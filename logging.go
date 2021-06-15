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
	entry := newLogEntry()
	// if there are fields
	if l.fields != nil {
		entry = entry.WithFields(l.fields)
	}
}

// Debug should be used for any logging that would be useful for debugging
func (l *DefaultLogger) Debug(args ...interface{}) {
	entry := newLogEntry()
	// if there are fields
	if l.fields != nil {
		entry = entry.WithFields(l.fields)
	}
}

// Info should be used for any logging that doesn't necessarily need attention but is nice to see by default
func (l *DefaultLogger) Info(args ...interface{}) {
	entry := newLogEntry()
	// if there are fields
	if l.fields != nil {
		entry = entry.WithFields(l.fields)
	}
}

// Warn should be used for logging that needs attention
func (l *DefaultLogger) Warn(args ...interface{}) {
	entry := newLogEntry()
	// if there are fields
	if l.fields != nil {
		entry = entry.WithFields(l.fields)
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
	l.WithFields(LoggerFields{
		"id":              step.ParentID,
		"insertion point": step.InsertionPoint,
	}).Info(step.ParentType)

	l.Info(graphql.FormatSelectionSet(step.SelectionSet))
}

var level logrus.Level
var log Logger = &DefaultLogger{}

func newLogEntry() *logrus.Entry {
	entry := logrus.New()

	// only log LOGLEVEL
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
		level = logrus.TraceLevel
	case "Debug":
		level = logrus.DebugLevel
	case "Info":
		level = logrus.InfoLevel
	default:
		level = logrus.WarnLevel
	}
}
