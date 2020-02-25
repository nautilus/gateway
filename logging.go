package gateway

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/vektah/gqlparser/v2/ast"
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

	log.Info(l.FormatSelectionSet(step.SelectionSet))
}

func (l *Logger) indentPrefix(level int) string {
	acc := "\n"
	// build up the prefix
	for i := 0; i <= level; i++ {
		acc += "    "
	}

	return acc
}
func (l *Logger) selectionSelectionSet(level int, selectionSet ast.SelectionSet) string {
	acc := " {"
	// and any sub selection
	acc += l.selection(level+1, selectionSet)
	acc += l.indentPrefix(level) + "}"

	return acc
}

func (l *Logger) selection(level int, selectionSet ast.SelectionSet) string {
	acc := ""

	for _, selection := range selectionSet {
		acc += l.indentPrefix(level)
		switch selection := selection.(type) {
		case *ast.Field:
			// add the field name
			acc += selection.Name
			if len(selection.SelectionSet) > 0 {
				acc += l.selectionSelectionSet(level, selection.SelectionSet)
			}
		case *ast.InlineFragment:
			// print the fragment name
			acc += fmt.Sprintf("... on %v", selection.TypeCondition) +
				l.selectionSelectionSet(level, selection.SelectionSet)
		case *ast.FragmentSpread:
			// print the fragment name
			acc += "..." + selection.Name
		}
	}

	return acc
}

// FormatSelectionSet returns a pretty printed version of a selection set
func (l *Logger) FormatSelectionSet(selection ast.SelectionSet) string {
	acc := "{"

	insides := l.selection(0, selection)

	if strings.TrimSpace(insides) != "" {
		acc += insides + "\n}"
	} else {
		acc += "}"
	}

	return acc
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
