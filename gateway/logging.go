package gateway

import (
	"github.com/sirupsen/logrus"
	"github.com/vektah/gqlparser/ast"
)

// Logger handles the logging in the gateway library
type Logger struct {
	fields logrus.Fields
}

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

	logPlanStep(0, step.SelectionSet)
}

func logPlanStep(level int, selectionSet ast.SelectionSet) {
	// build up the prefix
	prefix := ""
	for i := 0; i < level; i++ {
		prefix += "    "
	}
	prefix += "|- "
	for _, selection := range applyDirectives(selectionSet) {
		log.Info(prefix, selection.Name)
		logPlanStep(level+1, selection.SelectionSet)
	}
}

var log *Logger

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
func init() {
	log = &Logger{}

}
