package logger

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"
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

	levelWidth     = 7
	tagWidth       = 16
	minBannerWidth = 62
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

func icon(color, symbol, ascii string) string {
	if useColors {
		return colorize(color, symbol)
	}
	return ascii
}

func separator() string {
	if useColors {
		return colorize(dim, strings.Repeat("─", 12))
	}
	return strings.Repeat("-", 12)
}

func timestamp() string {
	t := "[" + time.Now().Format("15:04:05") + "]"
	return colorize(dim, t)
}

func columnSeparator() string {
	if useColors {
		return colorize(dim, "│")
	}
	return "|"
}

func messageSeparator() string {
	if useColors {
		return colorize(dim, "›")
	}
	return ">"
}

func fitText(text string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(text)
	if len(r) > width {
		if width == 1 {
			return string(r[:1])
		}
		return string(r[:width-1]) + "…"
	}
	return string(r) + strings.Repeat(" ", width-len(r))
}

func visualWidth(text string) int {
	return utf8.RuneCountInString(text)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func sanitizeLevel(level string) string {
	level = strings.TrimSpace(level)
	if level == "" {
		return "INFO"
	}
	return strings.ToUpper(level)
}

func sanitizeTag(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "CORE"
	}
	tag = strings.Join(strings.Fields(tag), "_")
	return strings.ToUpper(tag)
}

func logPrefix(level, tag, levelColor, symbol, ascii string) string {
	levelBadge := "[" + fitText(sanitizeLevel(level), levelWidth) + "]"
	tagCol := fitText(sanitizeTag(tag), tagWidth)
	if useColors {
		levelBadge = colorize(levelColor+bold, levelBadge)
		tagCol = colorize(cyan, tagCol)
	}
	marker := icon(levelColor, symbol, ascii)
	return fmt.Sprintf("%s %s %s %s %s", timestamp(), columnSeparator(), marker+" "+levelBadge, columnSeparator(), tagCol)
}

func printLog(level, tag, msg, levelColor, symbol, ascii string) {
	msgLines := strings.Split(msg, "\n")
	if len(msgLines) == 0 {
		msgLines = []string{""}
	}
	prefix := logPrefix(level, tag, levelColor, symbol, ascii)
	fmt.Printf("%s %s %s\n", prefix, messageSeparator(), msgLines[0])
	if len(msgLines) == 1 {
		return
	}

	contPrefix := fmt.Sprintf(
		"%s %s %s %s %s",
		strings.Repeat(" ", len("[15:04:05]")),
		columnSeparator(),
		strings.Repeat(" ", levelWidth+4),
		columnSeparator(),
		fitText("", tagWidth),
	)
	for _, line := range msgLines[1:] {
		fmt.Printf("%s %s %s\n", contPrefix, messageSeparator(), line)
	}
}

// Banner prints the startup banner
func Banner(version string) {
	if version == "" {
		version = "dev"
	}
	lines := []string{
		"EVE FLIPPER TERMINAL",
		"Market analysis stack for EVE Online operators",
		"Build " + version + "   |   Local-first runtime",
	}
	width := minBannerWidth
	for _, line := range lines {
		width = maxInt(width, visualWidth(line))
	}

	fmt.Println()
	if !useColors {
		horizontal := strings.Repeat("-", width+2)
		fmt.Printf("  +%s+\n", horizontal)
		for _, line := range lines {
			fmt.Printf("  | %s |\n", fitText(line, width))
		}
		fmt.Printf("  | %s |\n", fitText("Status: ready", width))
		fmt.Printf("  +%s+\n", horizontal)
		fmt.Println()
		return
	}

	fmt.Println(colorize(cyan+bold, "  ╭"+strings.Repeat("─", width+2)+"╮"))
	for i, line := range lines {
		padded := " " + fitText(line, width) + " "
		lineColor := dim
		switch i {
		case 0:
			lineColor = yellow + bold
		case 1:
			lineColor = white
		default:
			lineColor = dim
		}
		fmt.Println(colorize(cyan+bold, "  │") + colorize(lineColor, padded) + colorize(cyan+bold, "│"))
	}

	statusText := "● core online   ● scanners ready   ● cache warm"
	statusLine := " " + colorize(dim, fitText(statusText, width)) + " "
	fmt.Println(colorize(cyan+bold, "  ├"+strings.Repeat("─", width+2)+"┤"))
	fmt.Println(colorize(cyan+bold, "  │") + statusLine + colorize(cyan+bold, "│"))
	fmt.Println(colorize(cyan+bold, "  ╰"+strings.Repeat("─", width+2)+"╯"))
	fmt.Println()
}

// Info prints an info message
func Info(tag, msg string) {
	printLog("INFO", tag, msg, blue, "●", "*")
}

// Success prints a success message
func Success(tag, msg string) {
	printLog("SUCCESS", tag, msg, green, "✓", "+")
}

// Warn prints a warning message
func Warn(tag, msg string) {
	printLog("WARN", tag, msg, yellow, "⚠", "!")
}

// Error prints an error message
func Error(tag, msg string) {
	printLog("ERROR", tag, msg, red, "✗", "x")
}

// Loading prints a loading message (without newline initially)
func Loading(tag, msg string) {
	fmt.Printf("%s %s %s", logPrefix("LOADING", tag, magenta, "◌", "..."), messageSeparator(), msg)
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
	Success("SERVER", "Listening on "+colorize(cyan+bold, "http://"+addr))
	fmt.Printf("%s %s %s\n", strings.Repeat(" ", 12), messageSeparator(), colorize(dim, "Press Ctrl+C to stop"))
	fmt.Println()
}

// Section prints a section header
func Section(title string) {
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		cleanTitle = "Section"
	}
	cleanTitle = strings.ToUpper(cleanTitle)
	if useColors {
		fmt.Printf("\n%s %s %s\n", colorize(cyan, "┌"), colorize(white+bold, cleanTitle), separator())
		return
	}
	fmt.Printf("\n%s %s %s\n", "+", cleanTitle, separator())
}

// Stats prints statistics in a nice format
func Stats(label string, value interface{}) {
	labelCol := fitText(strings.TrimSpace(label), 18)
	fmt.Printf("    %s %s %v\n", icon(dim, "•", "-"), colorize(dim, labelCol+":"), colorize(white, fmt.Sprint(value)))
}
