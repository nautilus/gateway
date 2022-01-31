package gateway

import (
	"os"

	"github.com/nautilus/graphql"
	"github.com/sirupsen/logrus"
)

// Logger logs messages
type Logger interface {
	Trace(args ...interface{})
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

func (l *DefaultLogger) Trace(args ...interface{}) {
	if globalLogLevel >= logrus.TraceLevel {
		entry := newLogEntry(logrus.TraceLevel)
		// if there are fields
		if l.fields != nil {
			entry = entry.WithFields(l.fields)
		}
		entry.Trace(args...)
	}
}

// Debug should be used for any logging that would be useful for debugging
func (l *DefaultLogger) Debug(args ...interface{}) {
	if globalLogLevel >= logrus.DebugLevel {
		entry := newLogEntry(logrus.TraceLevel)
		// if there are fields
		if l.fields != nil {
			entry = entry.WithFields(l.fields)
		}
		entry.Debug(args...)
	}
}

// Info should be used for any logging that doesn't necessarily need attention but is nice to see by default
func (l *DefaultLogger) Info(args ...interface{}) {
	if globalLogLevel >= logrus.InfoLevel {
		entry := newLogEntry(logrus.InfoLevel)
		// if there are fields
		if l.fields != nil {
			entry = entry.WithFields(l.fields)
		}
		entry.Info(args...)
	}
}

// Warn should be used for logging that needs attention
func (l *DefaultLogger) Warn(args ...interface{}) {
	if globalLogLevel >= logrus.WarnLevel {
		entry := newLogEntry(logrus.WarnLevel)
		// if there are fields
		if l.fields != nil {
			entry = entry.WithFields(l.fields)
		}
		entry.Warn(args...)
	}
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
	if globalLogLevel >= logrus.DebugLevel {
		l.WithFields(LoggerFields{
			"id":              step.ParentID,
			"insertion point": step.InsertionPoint,
		}).Debug("QueryPlanStep.ParentType: ", step.ParentType)

		l.Debug("QueryPlanStep.SelectionSet: ", graphql.FormatSelectionSet(step.SelectionSet))
	}
}

var globalLogLevel logrus.Level
var log Logger = &DefaultLogger{}

func newLogEntry(level logrus.Level) *logrus.Entry {
	entry := logrus.New()

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
	switch os.Getenv("LOGLEVEL") {
	case "Trace":
		globalLogLevel = logrus.TraceLevel
	case "Debug":
		globalLogLevel = logrus.DebugLevel
	case "Info":
		globalLogLevel = logrus.InfoLevel
	default:
		globalLogLevel = logrus.WarnLevel
	}
}
