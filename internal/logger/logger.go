package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	mu      sync.Mutex
	logFile *os.File
	logPath string
)

// Init ensures the log file at ~/.config/telegram-feed/telegram-feed.log is open and ready.
func Init() string {
	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		return logPath
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}

	dir := filepath.Join(home, ".config", "telegram-feed")
	_ = os.MkdirAll(dir, 0755)

	logPath = filepath.Join(dir, "telegram-feed.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		logFile = f
		_, _ = logFile.WriteString(fmt.Sprintf("%s [INFO] === telegram-feed session started ===\n", time.Now().Format("2006-01-02 15:04:05")))
	}

	return logPath
}

// GetLogPath returns the path to the active log file.
func GetLogPath() string {
	if logPath == "" {
		return Init()
	}
	return logPath
}

func logMessage(level, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("%s [%s] %s\n", timestamp, level, msg)

	mu.Lock()
	defer mu.Unlock()

	if logFile == nil {
		mu.Unlock()
		Init()
		mu.Lock()
	}

	if logFile != nil {
		_, _ = logFile.WriteString(line)
	}
}

// Infof records an informational log line.
func Infof(format string, args ...any) {
	logMessage("INFO", format, args...)
}

// Warnf records a warning log line.
func Warnf(format string, args ...any) {
	logMessage("WARN", format, args...)
}

// Errorf records an error log line.
func Errorf(format string, args ...any) {
	logMessage("ERROR", format, args...)
}
