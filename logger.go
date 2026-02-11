package jsonql

import (
	"fmt"
	"log"
	"os"
)

// Logger defines the interface for JSONQL logging
type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// ConsoleLogger logs to stdout/stderr using the standard library logger
type ConsoleLogger struct {
	logger *log.Logger
	level  LogLevel
}

// LogLevel defines logging severity levels
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
	LogLevelSilent
)

// NewConsoleLogger creates a new ConsoleLogger with the given log level
func NewConsoleLogger(level LogLevel) *ConsoleLogger {
	return &ConsoleLogger{
		logger: log.New(os.Stdout, "[jsonql] ", log.LstdFlags),
		level:  level,
	}
}

func (l *ConsoleLogger) Debug(msg string, args ...interface{}) {
	if l.level <= LogLevelDebug {
		l.logger.Printf("DEBUG: "+msg, args...)
	}
}

func (l *ConsoleLogger) Info(msg string, args ...interface{}) {
	if l.level <= LogLevelInfo {
		l.logger.Printf("INFO: "+msg, args...)
	}
}

func (l *ConsoleLogger) Warn(msg string, args ...interface{}) {
	if l.level <= LogLevelWarn {
		l.logger.Printf("WARN: "+msg, args...)
	}
}

func (l *ConsoleLogger) Error(msg string, args ...interface{}) {
	if l.level <= LogLevelError {
		l.logger.Printf("ERROR: "+msg, args...)
	}
}

// NoOpLogger discards all log output
type NoOpLogger struct{}

func (NoOpLogger) Debug(string, ...interface{}) {}
func (NoOpLogger) Info(string, ...interface{})  {}
func (NoOpLogger) Warn(string, ...interface{})  {}
func (NoOpLogger) Error(string, ...interface{}) {}

// compile-time check
var _ Logger = (*ConsoleLogger)(nil)
var _ Logger = NoOpLogger{}

// Stringer for LogLevel
func (l LogLevel) String() string {
	switch l {
	case LogLevelDebug:
		return "debug"
	case LogLevelInfo:
		return "info"
	case LogLevelWarn:
		return "warn"
	case LogLevelError:
		return "error"
	case LogLevelSilent:
		return "silent"
	default:
		return fmt.Sprintf("LogLevel(%d)", l)
	}
}
