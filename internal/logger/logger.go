package logger

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

// ANSI color codes
const (
	reset = "\033[0m"
	bold  = "\033[1m"
	dim   = "\033[2m"

	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	blue    = "\033[34m"
	magenta = "\033[35m"
	cyan    = "\033[36m"
	white   = "\033[37m"
)

var useColors = false

func init() {
	// Check if colors are supported
	// Windows Terminal, PowerShell 7+, VS Code terminal support colors
	// Classic cmd.exe does NOT support ANSI by default

	if runtime.GOOS != "windows" {
		// Unix-like systems generally support colors
		useColors = true
		return
	}

	// On Windows, check for modern terminal indicators
	// WT_SESSION = Windows Terminal
	// TERM_PROGRAM = VS Code, etc.
	// ANSICON = ANSICON installed
	if os.Getenv("WT_SESSION") != "" ||
		os.Getenv("TERM_PROGRAM") != "" ||
		os.Getenv("ANSICON") != "" ||
		os.Getenv("ConEmuANSI") == "ON" {
		useColors = true
		return
	}

	// Try to enable VT mode on Windows 10+
	useColors = enableWindowsVT()
}

func colorize(color, text string) string {
	if !useColors {
		return text
	}
	return color + text + reset
}

func timestamp() string {
	t := time.Now().Format("15:04:05")
	return colorize(dim, t)
}

// Banner prints the startup banner
func Banner(version string) {
	if version == "" {
		version = "dev"
	}
	pad := 18 - len(version)
	if pad < 0 {
		pad = 0
	}
	fmt.Println()
	fmt.Println(colorize(cyan+bold, "  ╔═══════════════════════════════════════╗"))
	fmt.Println(colorize(cyan+bold, "  ║") + colorize(yellow+bold, "         EVE FLIPPER ") + colorize(dim, version) + colorize(cyan+bold, strings.Repeat(" ", pad)+"║"))
	fmt.Println(colorize(cyan+bold, "  ║") + colorize(dim, "      Market Analysis Tool           ") + colorize(cyan+bold, "║"))
	fmt.Println(colorize(cyan+bold, "  ╚═══════════════════════════════════════╝"))
	fmt.Println()
}

// Info prints an info message
func Info(tag, msg string) {
	icon := colorize(blue, "●")
	tagStr := colorize(cyan, fmt.Sprintf("[%s]", tag))
	fmt.Printf("%s %s %s %s\n", timestamp(), icon, tagStr, msg)
}

// Success prints a success message
func Success(tag, msg string) {
	icon := colorize(green, "✓")
	tagStr := colorize(green, fmt.Sprintf("[%s]", tag))
	fmt.Printf("%s %s %s %s\n", timestamp(), icon, tagStr, msg)
}

// Warn prints a warning message
func Warn(tag, msg string) {
	icon := colorize(yellow, "⚠")
	tagStr := colorize(yellow, fmt.Sprintf("[%s]", tag))
	fmt.Printf("%s %s %s %s\n", timestamp(), icon, tagStr, msg)
}

// Error prints an error message
func Error(tag, msg string) {
	icon := colorize(red, "✗")
	tagStr := colorize(red, fmt.Sprintf("[%s]", tag))
	fmt.Printf("%s %s %s %s\n", timestamp(), icon, tagStr, msg)
}

// Loading prints a loading message (without newline initially)
func Loading(tag, msg string) {
	icon := colorize(magenta, "◐")
	tagStr := colorize(magenta, fmt.Sprintf("[%s]", tag))
	fmt.Printf("%s %s %s %s", timestamp(), icon, tagStr, msg)
}

// Done completes a loading message
func Done(details string) {
	if details != "" {
		fmt.Printf(" %s\n", colorize(dim, details))
	} else {
		fmt.Println()
	}
}

// Server prints the server listening message
func Server(addr string) {
	fmt.Println()
	icon := colorize(green+bold, "►")
	fmt.Printf("%s %s Server running at %s\n", timestamp(), icon, colorize(cyan+bold, "http://"+addr))
	fmt.Printf("%s   %s\n", strings.Repeat(" ", 8), colorize(dim, "Press Ctrl+C to stop"))
	fmt.Println()
}

// Section prints a section header
func Section(title string) {
	fmt.Printf("\n%s %s\n", colorize(dim, "───"), colorize(white+bold, title))
}

// Stats prints statistics in a nice format
func Stats(label string, value interface{}) {
	fmt.Printf("    %s %s %v\n", colorize(dim, "•"), colorize(dim, label+":"), colorize(white, fmt.Sprint(value)))
}
