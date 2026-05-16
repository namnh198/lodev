package network

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/namnh198/lodev/pkg/util"
	"github.com/yarlson/tap"
)

// ProgressWriter tracks download progress and updates the tap.Progress bar
type ProgressWriter struct {
	progress *tap.Progress
	fileName string
	total    int64
	current  int64
}

// Write implements io.Writer so it can be used with io.MultiWriter
func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.current += int64(n)

	var message string
	if pw.total > 0 {
		percentage := float64(pw.current) / float64(pw.total) * 100
		message = fmt.Sprintf("Downloading %s... %.1f%%", pw.fileName, percentage)
	} else {
		// Fallback if the server doesn't provide a Content-Length
		message = fmt.Sprintf("Downloading %s... %d bytes", pw.fileName, pw.current)
	}

	// Advance the progress bar by n bytes
	pw.progress.Advance(n, message)
	return n, nil
}

func createRetryHTTPClient(timeout time.Duration, timeoutMax time.Duration, retries int, attempt int) (*retryablehttp.Client, time.Duration) {
	client := retryablehttp.NewClient()
	client.RetryMax = retries
	client.RetryWaitMin = 500 * time.Millisecond
	client.RetryWaitMax = 5 * time.Second
	// "context deadline exceeded" error during file copying cannot be retried with retryablehttp
	// See https://github.com/hashicorp/go-retryablehttp/issues/167
	// We use manual retry logic (for loop) around the entire Get+Copy operation instead
	client.CheckRetry = retryablehttp.DefaultRetryPolicy
	client.Backoff = retryablehttp.DefaultBackoff
	client.Logger = nil
	// Double the timeout for each retry attempt, up to timeoutMax
	// 1st attempt = clientTimeout * 2^0
	// 2nd attempt = clientTimeout * 2^1
	// 3rd attempt = clientTimeout * 2^2
	timeout = min(timeout*time.Duration(1<<attempt), timeoutMax)
	// Timeout for the entire request
	client.HTTPClient.Timeout = timeout
	client.RequestLogHook = func(_ retryablehttp.Logger, req *http.Request, attempt int) {
		if attempt > 0 {
			// attempt==1 is the first retry, 2 the second, etc
			tap.Message("Retrying download...", tap.MessageOptions{
				Hint: fmt.Sprintf("Retrying download of %s try #%d", req.URL.String(), attempt),
			})
		}
	}
	return client, timeout
}

// DownloadFile retrieves a file with retry logic, optional progress bar, and SHA256 verification.
func DownloadFile(destPath string, fileURL string, progressBar bool, shaSumURL string) (err error) {
	return DownloadFileExtended(destPath, fileURL, progressBar, shaSumURL, 2, 20*time.Minute)
}

// DownloadFileExtended retrieves a file with retry logic, optional progress bar, and SHA256 verification.
// It allows specifying the number of retries and timeout duration.
func DownloadFileExtended(destPath string, fileURL string, progressBar bool, shasumURL string, retries int, timeout time.Duration) (err error) {
	const timeoutMax = 1 * time.Hour // 1 hour is a reasonable upper limit for timeout to prevent excessively long waits
	// Correct invalid input values for retries and timeout
	if retries <= 0 {
		retries = 1
	}

	if timeout <= 0 {
		timeout = 20 * time.Minute
	}

	tap.Intro(fmt.Sprintf("Starting download of %s...", filepath.Base(fileURL)), tap.MessageOptions{
		Hint: fmt.Sprintf("To: %s", destPath),
	})

	// Ensure destFile is cleaned up
	defer func() {
		if err != nil {
			_ = os.Remove(destPath)
		}
	}()

	var hasher hash.Hash

	fileName := filepath.Base(fileURL)

	// Downloading the file with retry logic
	for attempt := 0; attempt <= retries; attempt++ {
		outFile, createErr := os.Create(destPath)
		if createErr != nil {
			err = fmt.Errorf("Failed to create file %s: %w", destPath, createErr)
			return
		}

		client, currentTimeout := createRetryHTTPClient(timeout, timeoutMax, retries, attempt)
		tap.Message(fmt.Sprintf("Downloading file %s (attempt #%d, timeout: %s)...", fileName, attempt+1, currentTimeout), tap.MessageOptions{})
		req, reqErr := retryablehttp.NewRequest("GET", fileURL, nil)

		if reqErr != nil {
			_ = outFile.Close()
			err = fmt.Errorf("Failed to create request for file download: %w", reqErr)
			return
		}

		githubHeaders := GetGithubHeaders(fileURL)
		for key, value := range githubHeaders {
			req.Header.Set(key, value)
		}

		resp, respErr := client.Do(req)

		if tokenErr := HasInvalidGithubToken(resp); tokenErr != nil {
			util.Debug("Retrying download without token. ERR: %v", tokenErr)
			for key := range githubHeaders {
				req.Header.Del(key)
			}
			respNoAuth, err := client.Do(req)
			if err == nil {
				if resp != nil {
					checkClose(resp.Body)
				}
				resp = respNoAuth
				respErr = err
			}
		}

		if respErr != nil {
			_ = outFile.Close()
			err = fmt.Errorf("Failed to download file from %s: %w", fileURL, respErr)
			return
		}

		if resp.StatusCode != http.StatusOK {
			checkClose(resp.Body)
			_ = outFile.Close()
			err = fmt.Errorf("Failed to download file from %s: HTTP status %d", fileURL, resp.StatusCode)
			return
		}

		totalSize := resp.ContentLength
		maxVal := int(totalSize)
		if maxVal <= 0 {
			maxVal = 100 // Fallback max size
		}

		hasher = sha256.New()

		var writer io.Writer
		var progress *tap.Progress
		var pw *ProgressWriter

		if progressBar {
			progress = tap.NewProgress(tap.ProgressOptions{
				Style: "heavy",
				Max:   maxVal,
				Size:  40,
			})

			progress.Start(fmt.Sprintf("Downloading %s to %s...", fileName, destPath))

			pw = &ProgressWriter{
				fileName: fileName,
				progress: progress,
				total:    totalSize,
			}
			writer = io.MultiWriter(outFile, hasher, pw)
		} else {
			writer = io.MultiWriter(outFile, hasher)
		}

		_, copyErr := io.Copy(writer, resp.Body)
		checkClose(resp.Body)
		closeErr := outFile.Close()

		if copyErr != nil {
			if errors.Is(copyErr, context.DeadlineExceeded) && attempt < retries {
				util.Debug("Download attempt %d failed with timeout, retrying...", attempt+1)
				continue
			}
			err = fmt.Errorf("Failed to save downloaded file: %w", copyErr)
			return
		}

		if closeErr != nil {
			err = fmt.Errorf("Failed to close file after download: %w", closeErr)
			return
		}

		if progressBar {
			progress.Stop(fmt.Sprintf("Download %s completed!", fileName), 0, tap.StopOptions{
				Hint: fmt.Sprintf("Saved at %s: (%.2f MB)", destPath, float64(pw.total)/1000000),
			})
		}

		break
	}

	// Downloading and verifying SHA sum if provided with retry logic
	verified, verfiedErr := doVerifySHA256(fileURL, shasumURL, retries, hasher)

	if verfiedErr != nil {
		return verfiedErr
	}

	var outroMessage string
	if verified {
		outroMessage = "Downloaded (%s) and verified successfully!"
	} else {
		outroMessage = "Downloaded %s without verification (no shasum URL provided)"
	}

	tap.Outro(fmt.Sprintf(outroMessage, fileName), tap.MessageOptions{
		Hint: fmt.Sprintf("Saved at %s", destPath),
	})

	return
}

// doVerifySHA256 verifies the SHA256 checksum of the downloaded file against the expected value from the shasumURL.
func doVerifySHA256(fileURL string, shasumURL string, retries int, hasher hash.Hash) (bool, error) {
	// If no shasumURL is provided, skip verification
	if shasumURL == "" {
		return false, nil
	}

	var expectedSHA string

	shasumTimeout := 30 * time.Second

	for attempt := 0; attempt <= retries; attempt++ {
		client, currentTimeout := createRetryHTTPClient(shasumTimeout, shasumTimeout*4, retries, attempt)
		tap.Message(fmt.Sprintf("Downloading SHA256 sum (attempt #%d, timeout: %s)...", attempt+1, currentTimeout), tap.MessageOptions{})
		req, reqErr := retryablehttp.NewRequest("GET", shasumURL, nil)

		if reqErr != nil {
			return true, fmt.Errorf("Failed to create request for SHA256 sum: %w", reqErr)
		}

		githubHeader := GetGithubHeaders(shasumURL)
		for key, value := range githubHeader {
			req.Header.Set(key, value)
		}

		resp, respErr := client.Do(req)

		if tokenErr := HasInvalidGithubToken(resp); tokenErr != nil {
			tap.Message("Retrying download the SHASUM without token", tap.MessageOptions{
				Hint: fmt.Sprintf("ERR: %v", tokenErr),
			})
			for key := range githubHeader {
				req.Header.Del(key)
			}
			respNoAuth, err := client.Do(req)
			if err == nil {
				if resp != nil {
					checkClose(resp.Body)
				}
				resp = respNoAuth
				respErr = err
			}
		}

		if respErr != nil {
			return true, fmt.Errorf("Failed to download SHASUM URL %s: %w", shasumURL, respErr)
		}

		if resp.StatusCode != http.StatusOK {
			checkClose(resp.Body)
			return true, fmt.Errorf("Failed to download SHASUM URL %s: HTTP status %d", shasumURL, resp.StatusCode)
		}

		body, readErr := io.ReadAll(resp.Body)
		checkClose(resp.Body)
		if readErr != nil {
			if errors.Is(readErr, context.DeadlineExceeded) && attempt < retries {
				tap.Message(fmt.Sprintf("SHASUM read attempt %d failed with timeout, retrying...", attempt+1), tap.MessageOptions{})
				continue
			}
			return true, fmt.Errorf("Failed to read SHASUM response body: %w", readErr)
		}

		expectedSHA = strings.TrimSpace(string(body))

		break
	}

	if expectedSHA == "" {
		return false, nil
	}

	baseName := filepath.Base(fileURL)

	var matchedSHA string
	for line := range strings.SplitSeq(expectedSHA, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		filename := strings.TrimPrefix(fields[1], "*")
		if filename == baseName {
			matchedSHA = fields[0]
			break
		}
	}

	tap.Message(fmt.Sprintf("Verifying (%s) SHA256 checksum...", baseName), tap.MessageOptions{
		Hint: fmt.Sprintf("SHA256: %s", matchedSHA),
	})

	if matchedSHA == "" {
		return true, fmt.Errorf("no matching SHA256 found for %s in shaSum file", baseName)
	}
	actualSHA := fmt.Sprintf("%x", hasher.Sum(nil))
	if actualSHA != matchedSHA {
		return true, fmt.Errorf("SHA256 mismatch: expected %s, got %s", matchedSHA, actualSHA)
	}

	return true, nil
}

// CheckClose is used to check the return from Close in a defer statement.
// From https://groups.google.com/d/msg/golang-nuts/-eo7navkp10/BY3ym_vMhRcJ
func checkClose(c io.Closer) {
	err := c.Close()
	if err != nil {
		util.Error("Failed to close deferred io.Closer, ERR: %v", err)
	}
}
