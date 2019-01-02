package gateway

import (
	"github.com/sirupsen/logrus"
	"github.com/vektah/gqlparser/ast"
)

// Logger handles the logging in the gateway library
type Logger struct{}

// Debug should be used for any logging that would be useful for debugging
func (l *Logger) Debug(args ...interface{}) {
	logrus.Debug(args...)
}

// Info should be used for any logging that doesn't necessarily need attention but is nice to see by default
func (l *Logger) Info(args ...interface{}) {
	logrus.Info(args...)
}

// Warn should be used for logging that needs attention
func (l *Logger) Warn(args ...interface{}) {
	logrus.Warn(args...)
}

// QueryPlanStep formats and logs a query plan step for human consumption
func (l *Logger) QueryPlanStep(step *QueryPlanStep) {
	log.Info(step.ParentType)
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
