// Package tui holds the terminal output helpers.
package tui

import (
	"fmt"
	"os"
)

const (
	reset  = "\033[0m"
	cyan   = "\033[96m"
	green  = "\033[92m"
	yellow = "\033[93m"
	red    = "\033[91m"
)

var enabled bool

// Init enables colors on a terminal unless NO_COLOR is set.
func Init() {
	if os.Getenv("NO_COLOR") != "" {
		return
	}
	if fi, err := os.Stdout.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		enabled = true
	}
}

func c(code string) string {
	if enabled {
		return code
	}
	return ""
}

func Prompt() {
	fmt.Print(c(cyan) + "You: " + c(reset))
}

func AnswerPrefix() {
	fmt.Print(c(green) + "ch.at: " + c(reset))
}

func Write(p []byte) {
	_, _ = os.Stdout.Write(p)
}

func Warn(format string, a ...any) {
	fmt.Println(c(yellow) + fmt.Sprintf(format, a...) + c(reset))
}

func Error(err error) {
	fmt.Fprintln(os.Stderr, c(red)+"error: "+err.Error()+c(reset))
}
