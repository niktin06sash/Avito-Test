package logger

import (
	"os"

	"github.com/sirupsen/logrus"
)

func InitMain() *logrus.Logger {
	log := logrus.New()
	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)
	log.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	return log
}

func Service(root *logrus.Logger) *logrus.Entry {
	return root.WithField("layer", "service")
}

func Handler(root *logrus.Logger) *logrus.Entry {
	return root.WithField("layer", "handler")
}
