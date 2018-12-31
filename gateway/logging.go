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

// Info should be used for tracking a noteworthy event
func (l *Logger) Info(args ...interface{}) {
	logrus.Info(args...)
}

// Warn should be used to log something that is worth some attention
func (l *Logger) Warn(args ...interface{}) {
	logrus.Warn(args...)
}

// Error should be used to log something that definitely wants attention
func (l *Logger) Error(args ...interface{}) {
	logrus.Error(args...)
}

var log *Logger

func init() {
	log = &Logger{}

	// only log the warning severity or above.
	logrus.SetLevel(logrus.WarnLevel)

	// configure the formatter
	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true, ForceColors: true, DisableLevelTruncation: true})
}
