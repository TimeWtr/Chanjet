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
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestZapAdapter(t *testing.T) {
	logger, err := zap.NewDevelopment(zap.WithCaller(true))
	assert.NoError(t, err)

	adapter, _ := NewZapAdapter(logger).(*ZapAdapter)

	t.Run("Debug", func(_ *testing.T) {
		adapter.Debug("", StringField("key", "value"))
	})

	t.Run("Info", func(_ *testing.T) {
		adapter.Info("info message", IntField("count", 42))
	})

	t.Run("Warn", func(_ *testing.T) {
		adapter.Warn("warn message", BoolField("flag", true))
	})

	t.Run("Error", func(_ *testing.T) {
		adapter.Error("error message", ErrorField(errors.New("test error")))
	})

	t.Run("With", func(_ *testing.T) {
		child, _ := adapter.With(StringField("request", "123")).(*ZapAdapter)
		child.Info("with fields")
	})

	t.Run("SetLevel", func(_ *testing.T) {
		assert.NoError(t, adapter.SetLevel(LevelError))
		adapter.Info("should not appear")
	})
}

func Benchmark_Zap_Adapter(b *testing.B) {
	logger, err := zap.NewDevelopment()
	assert.NoError(b, err)

	adapter, _ := NewZapAdapter(logger).(*ZapAdapter)
	for i := 0; i < b.N; i++ {
		adapter.Debug("test debug log message",
			StringField("key", "value"),
			IntField("count", 42))
	}
}
