package logger

import (
	"os"

	"github.com/sirupsen/logrus"
)

func Setup() {
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.JSONFormatter{})
}
