package util

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yarlson/tap"
)

// Failed will print a red error message and exit with failure
func Failed(text string, args ...any) {
	format := highlightF(tap.Red, text, args...)
	fmt.Fprintln(os.Stderr, format)
	os.Exit(0)
}

// Error will print a red error message but will not exit.
func Error(text string, args ...any) {
	format := highlightF(tap.Red, text, args...)
	fmt.Fprintln(os.Stderr, format)
}

// Success will indicate an operation succeeded with colored confirmation text.
func Success(text string, args ...any) {
	format := highlightF(tap.Green, text, args...)
	fmt.Fprintln(os.Stdout, format)
}

// Warning will present the user with warning text.
func Warning(text string, args ...any) {
	format := highlightF(tap.Yellow, text, args...)
	fmt.Fprintln(os.Stdout, format)
}

// Info will present the user with informational text.
func Info(text string, args ...any) {
	format := highlightF(tap.Gray, text, args...)
	fmt.Fprintln(os.Stdout, format)
}

// Debug will present the user with debug text, but only if the LODEV_VERBOSE environment variable is set to true.
func Debug(text string, args ...any) {
	if IsEnvFalse("LODEV_VERBOSE") {
		return
	}

	format := highlightF(tap.Gray, text, args...)
	fmt.Fprintln(os.Stdout, format)
}

// WarningMessage will print a warning message using tap.Message with yellow color.
func WarningMessage(text string, opts ...tap.MessageOptions) {
	format := highlight(tap.Yellow, text)
	tap.Message(format, opts...)
}

// WarningMessage will print a warning message using tap.Message with yellow color.
func InfoMessage(text string, opts ...tap.MessageOptions) {
	tap.Message(text, opts...)
}

// SuccessMessage will print a success message using tap.Message with green color.
func SuccessMessage(text string, opts ...tap.MessageOptions) {
	format := highlight(tap.Green, text)
	tap.Outro(format, opts...)
}

// ErrorMessage will print an error message using tap.Message with red color.
func ErrorMessage(text string, opts ...tap.MessageOptions) {
	tap.Cancel(text, opts...)
	os.Exit(0)
}

// ArrayToReadableOutput generates a printable list of items in a readable way
func ArrayToReadableOutput(slice []string) (string, error) {
	if len(slice) == 0 {
		return "", fmt.Errorf("empty slice")
	}
	var b strings.Builder
	b.WriteString("\n")
	for _, item := range slice {
		b.WriteString("  - " + item + "\n")
	}
	return b.String(), nil
}

// WaitTimer tracks elapsed time for wait operations with inline output
type WaitTimer struct {
	startTime   time.Time
	waitingName string
	Progress    *tap.Progress
}

// StartWait prints a waiting message (without newline) and returns a WaitTimer
// to track elapsed time. Call Complete() when the operation finishes.
func StartWait(message string, progress ...tap.Progress) *WaitTimer {
	var p *tap.Progress
	if len(progress) > 0 {
		p = &progress[0]
	} else {
		p = tap.NewProgress(tap.ProgressOptions{
			Style: "heavy",
			Max:   100,
			Size:  40,
		})
	}
	p.Start(message)
	return &WaitTimer{startTime: time.Now(), Progress: p}
}

// Complete prints the elapsed time on the same line as the wait message.
// If err is non-nil, just prints a newline.
func (w *WaitTimer) Complete(err error, message ...string) time.Duration {
	elapsed := time.Since(w.startTime)

	if err != nil {
		w.Progress.Stop("FAILED !!!", 2, tap.StopOptions{Hint: fmt.Sprintf("ERROR: %v", err)})
	} else if len(message) > 0 {
		w.Progress.Stop(fmt.Sprintf("%s [%.1fs]", message[0], elapsed.Seconds()), 0)
	} else {
		w.Progress.Stop(fmt.Sprintf("DONE [%.1fs]", elapsed.Seconds()), 0)
	}
	return elapsed
}

// highlight is a helper function to apply ANSI color codes to a string.
func highlightF(ansi, text string, arg ...any) string {
	return fmt.Sprintf("%s%s%s", ansi, fmt.Sprintf(text, arg...), tap.Reset)
}

func highlight(ansi, text string) string {
	return fmt.Sprintf("%s%s%s", ansi, fmt.Sprint(text), tap.Reset)
}
