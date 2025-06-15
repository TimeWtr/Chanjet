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
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLevel(t *testing.T) {
	tests := []struct {
		level    Level
		str      string
		upperStr string
		valid    bool
	}{
		{LevelDebug, "debug", "DEBUG", true},
		{LevelInfo, "info", "INFO", true},
		{LevelWarn, "warn", "WARN", true},
		{LevelError, "error", "ERROR", true},
		{LevelFatal, "fatal", "FATAL", true},
		{LevelPanic, "panic", "PANIC", true},
		{LevelInvalid, "unknown", "UNKNOWN", false},
	}

	for _, tt := range tests {
		t.Run(tt.str, func(t *testing.T) {
			assert.Equal(t, tt.str, tt.level.String())
			assert.Equal(t, tt.upperStr, tt.level.UpperString())
			assert.Equal(t, tt.valid, tt.level.valid())
		})
	}
}

func TestFields(t *testing.T) {
	t.Run("StringField", func(t *testing.T) {
		f := StringField("key", "value")
		assert.Equal(t, "key", f.Key)
		assert.Equal(t, "value", f.Val)
	})

	t.Run("IntField", func(t *testing.T) {
		f := IntField("count", 42)
		assert.Equal(t, "count", f.Key)
		assert.Equal(t, 42, f.Val)
	})

	t.Run("ErrorField", func(t *testing.T) {
		err := errors.New("test")
		f := ErrorField(err)
		assert.Equal(t, "error", f.Key)
		assert.Equal(t, err, f.Val)
	})

	t.Run("DurationField", func(t *testing.T) {
		d := time.Second
		f := DurationField("latency", d)
		assert.Equal(t, "latency", f.Key)
		assert.Equal(t, d, f.Val)
	})

	t.Run("TimeField", func(t *testing.T) {
		now := time.Now()
		f := TimeField("timestamp", now)
		assert.Equal(t, "timestamp", f.Key)
		assert.Equal(t, now, f.Val)
	})
}
