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

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type ZapAdapter struct {
	logger *zap.Logger
	level  Level
}

func NewZapAdapter(logger *zap.Logger) Logger {
	return &ZapAdapter{
		logger: logger,
		level:  getLevel(),
	}
}

func (z *ZapAdapter) Debug(format string, args ...Field) {
	z.log(LevelDebug, format, args...)
}

func (z *ZapAdapter) Info(format string, args ...Field) {
	z.log(LevelInfo, format, args...)
}

func (z *ZapAdapter) Warn(format string, args ...Field) {
	z.log(LevelWarn, format, args...)
}

func (z *ZapAdapter) Error(format string, args ...Field) {
	z.log(LevelError, format, args...)
}

func (z *ZapAdapter) Fatal(format string, args ...Field) {
	z.log(LevelFatal, format, args...)
}

func (z *ZapAdapter) Panic(format string, args ...Field) {
	z.log(LevelPanic, format, args...)
}

func (z *ZapAdapter) Sync() error {
	return z.logger.Sync()
}

func (z *ZapAdapter) With(fields ...Field) Logger {
	if len(fields) == 0 {
		return z
	}

	zapFields := make([]zapcore.Field, 0, len(fields))
	for _, field := range fields {
		zapFields = append(zapFields, zap.Any(field.Key, field.Val))
	}

	return &ZapAdapter{
		logger: z.logger.With(zapFields...),
		level:  z.level,
	}
}

func (z *ZapAdapter) SetLevel(level Level) error {
	if !level.valid() {
		return errors.New("invalid log level")
	}
	z.level = level
	return nil
}

//nolint:unused // this will be called.
func (z *ZapAdapter) convertToZapLevel(level Level) zapcore.Level {
	switch level {
	case LevelDebug:
		return zapcore.DebugLevel
	case LevelInfo:
		return zapcore.InfoLevel
	case LevelWarn:
		return zapcore.WarnLevel
	case LevelError:
		return zapcore.ErrorLevel
	case LevelFatal:
		return zapcore.FatalLevel
	case LevelPanic:
		return zapcore.PanicLevel
	default:
		return zapcore.InvalidLevel
	}
}

//nolint:unused // this will be called.
func (z *ZapAdapter) convertToLevel(l zapcore.Level) Level {
	switch l {
	case zapcore.DebugLevel:
		return LevelDebug
	case zapcore.InfoLevel:
		return LevelInfo
	case zapcore.WarnLevel:
		return LevelWarn
	case zapcore.ErrorLevel:
		return LevelError
	case zapcore.FatalLevel:
		return LevelFatal
	case zapcore.PanicLevel:
		return LevelPanic
	default:
		return LevelInvalid
	}
}

func (z *ZapAdapter) log(level Level, format string, fields ...Field) {
	if z.level > level {
		return
	}

	zapFields := make([]zap.Field, 0, len(fields))
	for _, field := range fields {
		zapFields = append(zapFields, zap.Any(field.Key, field.Val))
	}

	switch level {
	case LevelDebug:
		z.logger.Debug(format, zapFields...)
	case LevelInfo:
		z.logger.Info(format, zapFields...)
	case LevelWarn:
		z.logger.Warn(format, zapFields...)
	case LevelError:
		z.logger.Error(format, zapFields...)
	case LevelFatal:
		z.logger.Fatal(format, zapFields...)
	case LevelPanic:
		z.logger.Panic(format, zapFields...)
	default:
	}
}
