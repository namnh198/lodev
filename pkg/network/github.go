package network

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v81/github"
)

var (
	githubContext      context.Context // githubContext is the Go context used for GitHub API requests
	githubClient       *github.Client  // githubClient is the singleton instance of Client
	githubClientNoAuth *github.Client  // githubClientNoAuth is the singleton instance of Client without authentication
	githubClientOnce   sync.Once       // githubClientOnce ensures githubClient is initialized only once
)

// GetGithubClient returns a GitHub client with authentication if a GitHub token is available in environment variables, otherwise it returns a client without authentication.
func GetGithubClient(withAuth bool) (context.Context, *github.Client) {
	githubClientOnce.Do(func() {
		githubContext = context.Background()
		githubClientNoAuth = github.NewClientWithEnvProxy()
		githubClient = githubClientNoAuth
		githubToken, _ := GetGithubToken()

		if githubToken != "" {
			githubClient = githubClient.WithAuthToken(githubToken)
		}
	})

	if withAuth {
		return githubContext, githubClient
	}

	return githubContext, githubClientNoAuth
}

// GetGithubRelease gets the tarball URL and version for a GitHub repository release
func GetGithubRelease(owner, repo, requestedVersion string) (tarballURL, downloadedRelease string, err error) {
	ctx, client := GetGithubClient(true)

	releases, resp, err := client.Repositories.ListReleases(ctx, owner, repo, &github.ListOptions{PerPage: 100})
	var tokenErr error
	if err != nil {
		if tokenErr = HasInvalidGithubToken(resp); tokenErr != nil {
			ctx, client = GetGithubClient(false)
			releasesNoAuth, respNoAuth, errNoAuth := client.Repositories.ListReleases(ctx, owner, repo, &github.ListOptions{PerPage: 100})
			if errNoAuth == nil {
				releases = releasesNoAuth
				resp = respNoAuth
				err = errNoAuth
			}
		}
	}
	if err != nil {
		errorDetail := ""
		if resp != nil {
			rate := resp.Rate
			if rate.Limit != 0 {
				resetIn := time.Until(rate.Reset.Time).Round(time.Second)
				errorDetail += fmt.Sprintf("\nGitHub API Rate Limit: %d/%d remaining (resets in %s)", rate.Remaining, rate.Limit, resetIn)
			}
			if tokenErr != nil {
				errorDetail += "\nError: " + tokenErr.Error()
			}
		}
		return "", "", fmt.Errorf("unable to get releases for %v: %w%s", repo, err, errorDetail)
	}
	if len(releases) == 0 {
		return "", "", fmt.Errorf("no releases found for %v", repo)
	}

	releaseItem := 0
	releaseFound := false
	if requestedVersion != "" {
		for i, release := range releases {
			if release.GetTagName() == requestedVersion {
				releaseItem = i
				releaseFound = true
				break
			}
		}
		if !releaseFound {
			return "", "", fmt.Errorf("no release found for %v with tag %v", repo, requestedVersion)
		}
	}

	tarballURL = releases[releaseItem].GetTarballURL()
	downloadedRelease = releases[releaseItem].GetTagName()
	return tarballURL, downloadedRelease, nil
}

// isGithubUrl checks if the given URL is a GitHub URL.
func isGithubUrl(requestUrl string) bool {
	if requestUrl == "" {
		return false
	}
	u, err := url.Parse(requestUrl)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Host)
	return host == "github.com" || strings.HasSuffix(host, ".github.com")
}

// GetGithubHeaders returns the headers needed for GitHub API requests, including the Authorization header if a GitHub token is available in environment variables.
func GetGithubHeaders(requestUrl string) (headers map[string]string) {
	headers = make(map[string]string)
	if !isGithubUrl(requestUrl) {
		return headers
	}

	if githubToken, _ := GetGithubToken(); githubToken != "" {
		headers["Authorization"] = "Bearer " + githubToken
		headers["X-Github-Api-Version"] = "2022-11-28"
		headers["User-Agent"] = "D-Stack"
	}

	return headers
}

// GetGithubToken checks for GitHub token in environment variables and returns the token and the variable name used.
func GetGithubToken() (string, string) {
	for _, tokenEnv := range []string{"DSTACK_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"} {
		if githubToken := os.Getenv(tokenEnv); githubToken != "" {
			return githubToken, tokenEnv
		}
	}
	return "", ""
}

// HasInvalidGithubToken checks if the response indicates an invalid GitHub token.
// This is possible if we get 401 or 404 response.
func HasInvalidGithubToken(response any) error {
	var httpResp *http.Response
	switch r := response.(type) {
	case *github.Response:
		if r == nil {
			return nil
		}
		httpResp = r.Response
	case *http.Response:
		httpResp = r
	default:
		return nil
	}
	if httpResp == nil || httpResp.Request == nil {
		return nil
	}
	if !isGithubUrl(httpResp.Request.URL.String()) {
		return nil
	}
	_, tokenVariable := GetGithubToken()
	if tokenVariable == "" {
		return nil
	}
	if httpResp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("using %s, which is invalid or lacks required permissions", tokenVariable)
	}
	if httpResp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("using %s, which may be invalid or lack permissions", tokenVariable)
	}
	return nil
}
