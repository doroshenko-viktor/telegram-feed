package logger

import (
	"os"
	"strings"
	"testing"
)

func TestLogger(t *testing.T) {
	path := Init()
	if path == "" {
		t.Fatal("expected non-empty log path")
	}

	testMsg := "Unit test verification log message"
	Infof("%s", testMsg)
	Warnf("Warning message for %s", testMsg)
	Errorf("Error message for %s", testMsg)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	str := string(content)
	if !strings.Contains(str, testMsg) {
		t.Errorf("log file did not contain test message: %s", str)
	}
	if !strings.Contains(str, "[INFO]") || !strings.Contains(str, "[WARN]") || !strings.Contains(str, "[ERROR]") {
		t.Errorf("log file missing severity tags: %s", str)
	}
}
