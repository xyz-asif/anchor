package logger

import (
	"fmt"
	"log"
	"os"
	"time"
)

type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
	FATAL
)

var levelNames = map[Level]string{
	DEBUG: "DEBUG",
	INFO:  "INFO",
	WARN:  "WARN",
	ERROR: "ERROR",
	FATAL: "FATAL",
}

type Logger struct {
	level Level
	log   *log.Logger
}

func New(level Level) *Logger {
	return &Logger{
		level: level,
		log:   log.New(os.Stdout, "", log.LstdFlags),
	}
}

func (l *Logger) formatMessage(level Level, format string, v ...interface{}) string {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	levelName := levelNames[level]
	message := fmt.Sprintf(format, v...)
	return fmt.Sprintf("[%s] [%s] %s", timestamp, levelName, message)
}

func (l *Logger) Debug(format string, v ...interface{}) {
	if l.level <= DEBUG {
		l.log.Print(l.formatMessage(DEBUG, format, v...))
	}
}

func (l *Logger) Info(format string, v ...interface{}) {
	if l.level <= INFO {
		l.log.Print(l.formatMessage(INFO, format, v...))
	}
}

func (l *Logger) Warn(format string, v ...interface{}) {
	if l.level <= WARN {
		l.log.Print(l.formatMessage(WARN, format, v...))
	}
}

func (l *Logger) Error(format string, v ...interface{}) {
	if l.level <= ERROR {
		l.log.Print(l.formatMessage(ERROR, format, v...))
	}
}

func (l *Logger) Fatal(format string, v ...interface{}) {
	if l.level <= FATAL {
		l.log.Fatal(l.formatMessage(FATAL, format, v...))
	}
}

// SetLevel changes the logging level
func (l *Logger) SetLevel(level Level) {
	l.level = level
}

// GetLevel returns current logging level
func (l *Logger) GetLevel() Level {
	return l.level
}

// Global logger instance
var defaultLogger = New(INFO)

// Package-level functions for easy access
func Debug(format string, v ...interface{}) { defaultLogger.Debug(format, v...) }
func Info(format string, v ...interface{})  { defaultLogger.Info(format, v...) }
func Warn(format string, v ...interface{})  { defaultLogger.Warn(format, v...) }
func Error(format string, v ...interface{}) { defaultLogger.Error(format, v...) }
func Fatal(format string, v ...interface{}) { defaultLogger.Fatal(format, v...) }

// SetGlobalLevel sets the level for the global logger
func SetGlobalLevel(level Level) {
	defaultLogger.SetLevel(level)
}
