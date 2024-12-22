// internal/utils/logger.go

package utils

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

// Log prints a standard log message with optional formatting
func Log(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	logWithOptionalLevel(message, "", 2)
}

// Error logs a message with ERROR level and optional formatting
func Error(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	logWithOptionalLevel(message, "ERROR", 2)
}

// Debug logs a message with DEBUG level and optional formatting
func Debug(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	logWithOptionalLevel(message, "DEBUG", 2)
}

// Warn logs a message with WARN level and a [!] prefix
func Warn(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	logWithOptionalLevel("[!] "+message, "WARN", 2)
}

// Success logs a message with a [\u2713] prefix
func Success(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	logWithOptionalLevel("[\u2713] "+message, "", 2)
}


// Fatal logs a message and terminates the program
func Fatal(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	logWithOptionalLevel(message, "FATAL", 2)
	os.Exit(1)
}

func logWithOptionalLevel(message string, level string, skip int) {
	// Get the caller's function name
	pc, _, _, ok := runtime.Caller(skip)
	functionName := "unknown"
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

	// Format timestamp
	timestamp := time.Now().Format("15:04:05")

	if level != "" {
		fmt.Printf("[%s %s %s] [%s] %s\n", level, functionName, timestamp, message)
	} else {
		fmt.Printf("[%s %s] %s\n", functionName, timestamp, message)
	}
}
