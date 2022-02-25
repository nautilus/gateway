package gateway

import (
	"github.com/nautilus/graphql"
	"github.com/sirupsen/logrus"
)

// Logger logs messages
type Logger interface {
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})

	WithFields(fields LoggerFields) Logger
	QueryPlanStep(step *QueryPlanStep)
}

// DefaultLogger handles the logging in the gateway library
type DefaultLogger struct {
	fields logrus.Fields
}

// LoggerFields is a wrapper over a map of key,value pairs to associate with the log
type LoggerFields map[string]interface{}

// Debug should be used for any logging that would be useful for debugging
func (l *DefaultLogger) Debug(args ...interface{}) {
	entry := newLogEntry()
	// if there are fields
	if l.fields != nil {
		entry = entry.WithFields(l.fields)
	}

	// finally log
	entry.Debug(args...)
}

// Info should be used for any logging that doesn't necessarily need attention but is nice to see by default
func (l *DefaultLogger) Info(args ...interface{}) {
	entry := newLogEntry()
	// if there are fields
	if l.fields != nil {
		entry = entry.WithFields(l.fields)
	}

	// finally log
	entry.Info(args...)
}

// Warn should be used for logging that needs attention
func (l *DefaultLogger) Warn(args ...interface{}) {
	entry := newLogEntry()
	// if there are fields
	if l.fields != nil {
		entry = entry.WithFields(l.fields)
	}

	// finally log
	entry.Warn(args...)
}

// WithFields adds the provided fields to the Log
func (l *DefaultLogger) WithFields(fields LoggerFields) Logger {
	// build up the logrus fields
	logrusFields := logrus.Fields{}
	for key, value := range fields {
		logrusFields[key] = value
	}
	return &DefaultLogger{fields: logrusFields}
}

// QueryPlanStep formats and logs a query plan step for human consumption
func (l *DefaultLogger) QueryPlanStep(step *QueryPlanStep) {
	l.WithFields(LoggerFields{
		"id":              step.ParentID,
		"insertion point": step.InsertionPoint,
	}).Info(step.ParentType)

	l.Info(graphql.FormatSelectionSet(step.SelectionSet))
}

func newLogEntry() *logrus.Entry {
	entry := logrus.New()

	// only log the warning severity or above.
	entry.SetLevel(logrus.WarnLevel)

	// configure the formatter
	entry.SetFormatter(&logrus.TextFormatter{
		DisableTimestamp:       true,
		ForceColors:            true,
		DisableLevelTruncation: true,
	})

	return logrus.NewEntry(entry)
}
