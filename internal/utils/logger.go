// internal/utils/logger.go

package utils

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

type LogEntry struct {
	Timestamp string
	Level     string
	Function  string
	Message   string
	Color     string
}

var (
	LogHandler func(string)
	LogStore   []LogEntry
)

// SetLogHandler sets a function to be called with each log message
func SetLogHandler(handler func(string)) {
	LogHandler = handler
}

// Add these new functions
func GetLogs() []LogEntry {

	return LogStore
}

func ClearLogs() {
	LogStore = []LogEntry{}
}

// Log prints a standard log message with optional formatting
func Log(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	logWithOptionalLevel(message, "", 2, "\033[0m") // Reset
}

// Error logs a message with ERROR level and optional formatting
func Error(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	logWithOptionalLevel(message, "ERROR", 2, "\033[31m") // Red
}

// Debug logs a message with DEBUG level and optional formatting
func Debug(format string, args ...interface{}) {
	if os.Getenv("ENVIRONMENT") == "DEVELOPMENT" {
		message := fmt.Sprintf(format, args...)
		logWithOptionalLevel(message, "DEBUG", 2, "\033[34m") // Blue
	}
}

// Warn logs a message with WARN level and a [!] prefix
func Warn(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	logWithOptionalLevel("[!] "+message, "WARN", 2, "\033[33m") // Yellow
}

func Custom(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	logWithOptionalLevel(message, "CUSTOM", 2, "\033[35m") // Magenta
}

// Success logs a message with a [\u2713] prefix
func Success(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	logWithOptionalLevel("[\u2713] "+message, "", 2, "\033[32m") // Green
}

// Fatal logs a message and terminates the program
func Fatal(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	logWithOptionalLevel(message, "FATAL", 2, "\033[31m") // Red
	os.Exit(1)
}

func logWithOptionalLevel(message string, level string, skip int, color string) {
	// Get the caller's function name
	pc, _, _, ok := runtime.Caller(skip)
	functionName := "unknown"
	timestamp := time.Now().Format("15:04:05")

	if ok {
		fn := runtime.FuncForPC(pc)
		if fn != nil {
			// Get full function name
			fullName := fn.Name()
			// Split by "/" and get the last two parts (package.function)
			parts := strings.Split(fullName, "/")
			functionName = parts[len(parts)-1]
		}
	}

	resetColor := "\033[0m"

	// Store the log entry only once and only for specific levels
	if level == "FATAL" || level == "ERROR" || level == "CUSTOM" {
		logEntry := LogEntry{
			Timestamp: timestamp,
			Level:     level,
			Function:  functionName,
			Message:   message,
			Color:     color,
		}
		LogStore = append(LogStore, logEntry)
	}

	// Format the log message
	var logMessage string
	if level != "" {
		logMessage = fmt.Sprintf("%s[%s] [%s %s] %s%s\n", color, level, functionName, timestamp, message, resetColor)
	} else {
		logMessage = fmt.Sprintf("%s[%s %s] %s%s\n", color, functionName, timestamp, message, resetColor)
	}

	// Print to console
	fmt.Print(logMessage)

	// Call the log handler if set
	if LogHandler != nil {
		LogHandler(logMessage)
	}
}
