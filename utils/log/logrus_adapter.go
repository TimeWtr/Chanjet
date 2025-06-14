// Copyright 2025 TimeWtr
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package log

import (
	"errors"

	"github.com/sirupsen/logrus"
)

type LogrusAdapter struct {
	logger *logrus.Logger
	entry  *logrus.Entry
	level  Level
}

func NewLogrusAdapter(level Level, logger *logrus.Logger) Logger {
	return &LogrusAdapter{
		logger: logger,
		level:  level,
	}
}

func (l *LogrusAdapter) Debug(format string, args ...Field) {
	l.log(LevelDebug, format, args...)
}

func (l *LogrusAdapter) Info(format string, args ...Field) {
	l.log(LevelInfo, format, args...)
}

func (l *LogrusAdapter) Warn(format string, args ...Field) {
	l.log(LevelWarn, format, args...)
}

func (l *LogrusAdapter) Error(format string, args ...Field) {
	l.log(LevelError, format, args...)
}

func (l *LogrusAdapter) Fatal(format string, args ...Field) {
	l.log(LevelFatal, format, args...)
}

func (l *LogrusAdapter) Panic(format string, args ...Field) {
	l.log(LevelPanic, format, args...)
}

func (l *LogrusAdapter) Sync() error {
	return nil
}

func (l *LogrusAdapter) log(level Level, format string, fields ...Field) {
	if level < l.level {
		return
	}

	entry := l.entry
	if len(fields) > 0 {
		logrusFields := make(logrus.Fields, len(fields))
		for _, field := range fields {
			logrusFields[field.Key] = field.Val
		}
		entry.WithFields(logrusFields)
	}

	switch level {
	case LevelDebug:
		entry.Debug(format)
	case LevelInfo:
		entry.Info(format)
	case LevelWarn:
		entry.Warn(format)
	case LevelError:
		entry.Error(format)
	case LevelFatal:
		entry.Fatal(format)
	case LevelPanic:
		entry.Panic(format)
	default:
	}
}

func (l *LogrusAdapter) With(fields ...Field) Logger {
	logrusFields := make(logrus.Fields, len(fields))
	for _, field := range fields {
		logrusFields[field.Key] = field.Val
	}
	l.logger.WithFields(logrusFields)
	return &LogrusAdapter{}
}

func (l *LogrusAdapter) SetLevel(level Level) error {
	if !level.valid() {
		return errors.New("invalid log level")
	}
	l.level = level
	l.logger.SetLevel(l.convertLogrusLevel(level))
	return nil
}

func (l *LogrusAdapter) convertLogrusLevel(level Level) logrus.Level {
	switch level {
	case LevelDebug:
		return logrus.DebugLevel
	case LevelInfo:
		return logrus.InfoLevel
	case LevelWarn:
		return logrus.WarnLevel
	case LevelError:
		return logrus.ErrorLevel
	case LevelFatal:
		return logrus.FatalLevel
	case LevelPanic:
		return logrus.PanicLevel
	default:
		return logrus.InfoLevel
	}
}

//nolint:unused  // this will be called.
func (l *LogrusAdapter) covertLevel(level logrus.Level) Level {
	switch level {
	case logrus.DebugLevel:
		return LevelDebug
	case logrus.InfoLevel:
		return LevelInfo
	case logrus.WarnLevel:
		return LevelWarn
	case logrus.ErrorLevel:
		return LevelError
	case logrus.FatalLevel:
		return LevelFatal
	case logrus.PanicLevel:
		return LevelPanic
	default:
		return LevelInvalid
	}
}
