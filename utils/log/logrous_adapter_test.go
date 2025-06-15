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
	"bytes"
	"errors"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestLogrusAdapter(t *testing.T) {
	buf := new(bytes.Buffer)
	logger := logrus.New()
	logger.SetOutput(buf)
	logger.SetFormatter(&logrus.TextFormatter{
		DisableTimestamp: true,
		DisableQuote:     true,
	})

	adapter, _ := NewLogrusAdapter(logger).(*LogrusAdapter)

	t.Run("Debug", func(t *testing.T) {
		buf.Reset()
		_ = adapter.SetLevel(LevelDebug)
		adapter.Debug("debug message", StringField("key", "value"))

		assert.Contains(t, buf.String(), "level=debug")
		assert.Contains(t, buf.String(), "msg=debug message")
		assert.Contains(t, buf.String(), "key=value")
	})

	t.Run("Info", func(t *testing.T) {
		buf.Reset()
		adapter.Info("info message", IntField("count", 42))
		assert.Contains(t, buf.String(), "level=info")
		assert.Contains(t, buf.String(), "info message")
		assert.Contains(t, buf.String(), "count=42")
	})

	t.Run("Warn", func(t *testing.T) {
		buf.Reset()
		adapter.Warn("warn message", BoolField("flag", true))
		assert.Contains(t, buf.String(), "level=warning")
		assert.Contains(t, buf.String(), "warn message")
		assert.Contains(t, buf.String(), "flag=true")
	})

	t.Run("Error", func(t *testing.T) {
		buf.Reset()
		adapter.Error("error message", ErrorField(errors.New("test error")))
		assert.Contains(t, buf.String(), "level=error")
		assert.Contains(t, buf.String(), "error message")
		assert.Contains(t, buf.String(), "error=test error")
	})

	t.Run("With", func(t *testing.T) {
		buf.Reset()
		child, _ := adapter.With(StringField("request", "123")).(*LogrusAdapter)
		child.Info("with fields")
		assert.Contains(t, buf.String(), "request=123")
	})

	t.Run("SetLevel", func(t *testing.T) {
		buf.Reset()
		assert.NoError(t, adapter.SetLevel(LevelError))
		adapter.Info("should not appear")
		assert.Empty(t, buf.String())
	})
}

func Benchmark_Logrus(b *testing.B) {
	buf := new(bytes.Buffer)
	logger := logrus.New()
	logger.SetOutput(buf)
	logger.SetFormatter(&logrus.TextFormatter{
		DisableTimestamp: true,
		DisableQuote:     true,
	})

	adapter, _ := NewLogrusAdapter(logger).(*LogrusAdapter)
	for i := 0; i < b.N; i++ {
		adapter.Info("test message", IntField("count", 42))
	}
}
