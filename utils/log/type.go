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

import "time"

type Level int8

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
	LevelPanic
	LevelInvalid
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	case LevelFatal:
		return "fatal"
	default:
		return "unknown"
	}
}

func (l Level) valid() bool {
	switch l {
	case LevelDebug, LevelInfo, LevelWarn, LevelError, LevelFatal, LevelPanic:
		return true
	default:
		return false
	}
}

func (l Level) UpperString() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

type Logger interface {
	Core
	Sync() error
	With(fields ...Field) Logger
	SetLevel(level Level) error
}

type Core interface {
	Debug(format string, args ...Field)
	Info(format string, args ...Field)
	Warn(format string, args ...Field)
	Error(format string, args ...Field)
	Fatal(format string, args ...Field)
	Panic(format string, args ...Field)
}

type Field struct {
	Key string
	Val any
}

func StringField(key, val string) Field {
	return Field{Key: key, Val: val}
}

func IntField(key string, val int) Field {
	return Field{Key: key, Val: val}
}

func BoolField(key string, val bool) Field {
	return Field{Key: key, Val: val}
}

func ErrorField(err error) Field {
	return Field{Key: "error", Val: err}
}

func DurationFiled(key string, val time.Duration) Field {
	return Field{Key: key, Val: val}
}

func TimeFiled(key string, val time.Time) Field {
	return Field{Key: key, Val: val}
}

type LoggerType string

const (
	ZapLoggerType     LoggerType = "zap"
	LogrusLoggerType  LoggerType = "logrus"
	ZerologLoggerType LoggerType = "zerolog"
	LogstashLogger    LoggerType = "logstash"
)

func (l LoggerType) valid() bool {
	switch l {
	case ZapLoggerType, LogrusLoggerType, LogstashLogger, ZerologLoggerType:
		return true
	default:
		return false
	}
}
