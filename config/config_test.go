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

package config

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/TimeWtr/TurboStream/errorx"
	"github.com/stretchr/testify/assert"
)

func TestSwitchCondition_Initialization(t *testing.T) {
	t.Run("should initialize with valid config", func(t *testing.T) {
		config := SwitchConfig{
			PercentThreshold: 50,
			TimeThreshold:    time.Second,
		}

		sw, err := NewSwitchCondition(config)
		assert.NoError(t, err)
		assert.NotNil(t, sw)

		loadedConfig := sw.GetConfig()
		assert.Equal(t, 50, loadedConfig.PercentThreshold)
		assert.Equal(t, time.Second, loadedConfig.TimeThreshold)
		assert.Equal(t, time.Second.Milliseconds(), loadedConfig.timeThresholdMillis)
		assert.Equal(t, int64(1), loadedConfig.version)
	})
}

func TestSwitchCondition_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  SwitchConfig
		wantErr error
	}{
		{
			name: "valid config",
			config: SwitchConfig{
				PercentThreshold: 50,
				TimeThreshold:    time.Second,
			},
			wantErr: nil,
		},
		{
			name: "percent below zero",
			config: SwitchConfig{
				PercentThreshold: -10,
				TimeThreshold:    time.Second,
			},
			wantErr: errorx.ErrPercentThreshold,
		},
		{
			name: "percent above 100",
			config: SwitchConfig{
				PercentThreshold: 110,
				TimeThreshold:    time.Second,
			},
			wantErr: errorx.ErrPercentThreshold,
		},
		{
			name: "negative time threshold",
			config: SwitchConfig{
				PercentThreshold: 50,
				TimeThreshold:    -time.Second,
			},
			wantErr: errorx.ErrTimeThreshold,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sw, err := NewSwitchCondition(SwitchConfig{
				PercentThreshold: 50,
				TimeThreshold:    time.Second,
			})
			assert.NoError(t, err)
			err = sw.UpdateConfig(tt.config)

			if tt.wantErr != nil {
				assert.Error(t, err)
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSwitchCondition_UpdateConfig(t *testing.T) {
	t.Run("should update config successfully", func(t *testing.T) {
		initial := SwitchConfig{
			PercentThreshold: 50,
			TimeThreshold:    time.Second,
		}

		sw, err := NewSwitchCondition(initial)
		assert.NoError(t, err)

		notify := sw.Register()
		<-notify

		newConfig := SwitchConfig{
			PercentThreshold: 75,
			TimeThreshold:    2 * time.Second,
		}

		err = sw.UpdateConfig(newConfig)
		assert.NoError(t, err)

		loadedConfig := sw.GetConfig()
		assert.Equal(t, 75, loadedConfig.PercentThreshold)
		assert.Equal(t, 2*time.Second, loadedConfig.TimeThreshold)
		assert.Equal(t, int64(2000), loadedConfig.timeThresholdMillis)
		assert.Equal(t, int64(2), loadedConfig.version)
	})

	t.Run("should increment version on each update", func(t *testing.T) {
		sw, err := NewSwitchCondition(SwitchConfig{
			PercentThreshold: 50,
			TimeThreshold:    time.Second,
		})
		assert.NoError(t, err)

		for i := 1; i <= 5; i++ {
			config := SwitchConfig{
				PercentThreshold: i * 10,
				TimeThreshold:    time.Duration(i) * time.Second,
			}

			err := sw.UpdateConfig(config)
			assert.NoError(t, err)

			loadedConfig := sw.GetConfig()
			assert.Equal(t, int64(i+1), loadedConfig.version) // Initial version is 1
		}
	})

	t.Run("should handle concurrent updates safely", func(t *testing.T) {
		sw, err := NewSwitchCondition(SwitchConfig{
			PercentThreshold: 50,
			TimeThreshold:    time.Second,
		})
		assert.NoError(t, err)
		var wg sync.WaitGroup

		updates := 100
		wg.Add(updates)

		for i := 0; i < updates; i++ {
			go func(i int) {
				defer wg.Done()
				config := SwitchConfig{
					PercentThreshold: i,
					TimeThreshold:    time.Duration(i) * time.Millisecond,
				}
				_ = sw.UpdateConfig(config)
			}(i)
		}

		wg.Wait()

		finalConfig := sw.GetConfig()
		assert.Equal(t, int64(updates+1), finalConfig.version)
	})
}

func TestSwitchCondition_Notify(t *testing.T) {
	t.Run("should notify registered channel on update", func(t *testing.T) {
		sw, err := NewSwitchCondition(SwitchConfig{
			PercentThreshold: 50,
			TimeThreshold:    time.Second,
		})
		assert.NoError(t, err)
		notifyCh := sw.Register()

		newConfig := SwitchConfig{
			PercentThreshold: 75,
			TimeThreshold:    2 * time.Second,
		}

		err = sw.UpdateConfig(newConfig)
		assert.NoError(t, err)

		select {
		case <-notifyCh:
		case <-time.After(100 * time.Millisecond):
			assert.Fail(t, "expected notification but didn't receive one")
		}
	})
}

func TestSwitchCondition_EdgeCases(t *testing.T) {
	t.Run("max values", func(t *testing.T) {
		config := SwitchConfig{
			PercentThreshold: 100,
			TimeThreshold:    time.Duration(1<<63 - 1),
		}

		sw, err := NewSwitchCondition(config)
		assert.NoError(t, err)
		loadedConfig := sw.GetConfig()

		assert.Equal(t, 100, loadedConfig.PercentThreshold)
		assert.Equal(t, time.Duration(1<<63-1), loadedConfig.TimeThreshold)
	})
}

func TestSwitchCondition_Metrics(t *testing.T) {
	tests := []struct {
		size     int64
		percent  int
		duration time.Duration
	}{
		{1, 1, 1},
		{10, 5, 10 * time.Millisecond},
		{100, 50, 500 * time.Millisecond},
		{1000, 75, time.Second},
		{10000, 100, 10 * time.Second},
	}

	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			config := SwitchConfig{
				PercentThreshold: tt.percent,
				TimeThreshold:    tt.duration,
			}

			sw, err := NewSwitchCondition(config)
			assert.NoError(t, err)
			loadedConfig := sw.GetConfig()
			assert.Equal(t, tt.percent, loadedConfig.PercentThreshold)
			assert.Equal(t, tt.duration, loadedConfig.TimeThreshold)
			assert.Equal(t, tt.duration.Milliseconds(), loadedConfig.timeThresholdMillis)
		})
	}
}
