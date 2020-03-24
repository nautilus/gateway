package gateway

import (
	"os"

	"github.com/nautilus/graphql"
	"github.com/sirupsen/logrus"
)

// Logger handles the logging in the gateway library
type Logger struct {
	fields logrus.Fields
}

// LoggerFields is a wrapper over a map of key,value pairs to associate with the log
type LoggerFields map[string]interface{}

// Debug should be used for any logging that would be useful for debugging
func (l *Logger) Debug(args ...interface{}) {
	entry := newLogEntry()
	// if there are fields
	if l.fields != nil {
		entry = entry.WithFields(l.fields)
	}

	// finally log
	entry.Debug(args...)
}

// Info should be used for any logging that doesn't necessarily need attention but is nice to see by default
func (l *Logger) Info(args ...interface{}) {
	entry := newLogEntry()
	// if there are fields
	if l.fields != nil {
		entry = entry.WithFields(l.fields)
	}

	// finally log
	entry.Info(args...)
}

// Warn should be used for logging that needs attention
func (l *Logger) Warn(args ...interface{}) {
	entry := newLogEntry()
	// if there are fields
	if l.fields != nil {
		entry = entry.WithFields(l.fields)
	}

	// finally log
	entry.Warn(args...)
}

// WithFields adds the provided fields to the Log
func (l *Logger) WithFields(fields LoggerFields) *Logger {
	// build up the logrus fields
	logrusFields := logrus.Fields{}
	for key, value := range fields {
		logrusFields[key] = value
	}
	return &Logger{fields: logrusFields}
}

// QueryPlanStep formats and logs a query plan step for human consumption
func (l *Logger) QueryPlanStep(step *QueryPlanStep) {
	log.WithFields(LoggerFields{
		"id":              step.ParentID,
		"insertion point": step.InsertionPoint,
	}).Info(step.ParentType)

	log.Info(graphql.FormatSelectionSet(step.SelectionSet))
}

var log *Logger
var level logrus.Level

func newLogEntry() *logrus.Entry {
	entry := logrus.New()

	// only log the warning severity or above.
	entry.SetLevel(level)

	// configure the formatter
	entry.SetFormatter(&logrus.TextFormatter{
		DisableTimestamp:       true,
		ForceColors:            true,
		DisableLevelTruncation: true,
	})

	return logrus.NewEntry(entry)
}
func init() {
	log = &Logger{}

	switch os.Getenv("LOGLEVEL") {
	case "Debug":
		level = logrus.DebugLevel
	case "Info":
		level = logrus.InfoLevel
	default:
		level = logrus.WarnLevel
	}
}
