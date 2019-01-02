package gateway

import (
	"github.com/sirupsen/logrus"
)

// Logger handles the logging in the gateway library
type Logger struct{}

// Debug should be used for any logging that would be useful for debugging
func (l *Logger) Debug(args ...interface{}) {
	logrus.Debug(args...)
}

// Warn should be used for logging that needs attention
func (l *Logger) Warn(args ...interface{}) {
	logrus.Warn(args...)
}

var log *Logger

func init() {
	log = &Logger{}

	// only log the warning severity or above.
	logrus.SetLevel(logrus.DebugLevel)

	// configure the formatter
	logrus.SetFormatter(&logrus.TextFormatter{
		DisableTimestamp:       true,
		ForceColors:            true,
		DisableLevelTruncation: true,
	})
}
