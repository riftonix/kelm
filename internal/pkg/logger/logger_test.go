package logger

import (
	"bytes"
	"os"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestSetup(t *testing.T) {
	// Store old logrus configuration values
	oldOut := logrus.StandardLogger().Out
	oldLevel := logrus.GetLevel()
	oldFormatter := logrus.StandardLogger().Formatter

	// Create buffer to check output
	var buf bytes.Buffer
	logrus.SetOutput(&buf)

	// Call Setup, this will modify logrus default params, which stored in first step
	Setup()

	// Run tests
	if logrus.GetLevel() != logrus.DebugLevel {
		t.Errorf("Expected log level Debug, got %v", logrus.GetLevel())
	}
	_, ok := logrus.StandardLogger().Formatter.(*logrus.JSONFormatter)
	if !ok {
		t.Errorf("Expected JSONFormatter, got %T", logrus.StandardLogger().Formatter)
	}
	if logrus.StandardLogger().Out != os.Stdout {
		t.Errorf("Expected output to be os.Stdout")
	}

	// Restore old values
	logrus.SetOutput(oldOut)
	logrus.SetLevel(oldLevel)
	logrus.SetFormatter(oldFormatter)
}
