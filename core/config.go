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

package core

import (
	"fmt"
	"sync/atomic"
	"time"
)

type SwitchConfig struct {
	version             int64
	SizeThreshold       int64
	PercentThreshold    int
	TimeThreshold       time.Duration
	timeThresholdMillis int64
}

type SwitchCondition struct {
	config  atomic.Value
	notify  chan struct{}
	version int64
}

func NewSwitchCondition(config SwitchConfig) (*SwitchCondition, error) {
	sw := &SwitchCondition{
		notify: make(chan struct{}, 1),
	}

	if err := sw.validate(config); err != nil {
		return nil, err
	}

	config.timeThresholdMillis = config.TimeThreshold.Milliseconds()
	config.version = atomic.AddInt64(&sw.version, 1)
	sw.config.Store(config)
	sw.notify <- struct{}{}

	return sw, nil
}

func (s *SwitchCondition) validate(sc SwitchConfig) error {
	if sc.SizeThreshold < 0 {
		return fmt.Errorf("size threshold cannot be negative")
	}

	if sc.PercentThreshold < 0 || sc.PercentThreshold > 100 {
		return fmt.Errorf("percent threshold must be between 0 and 100")
	}

	if sc.TimeThreshold < 0 {
		return fmt.Errorf("time threshold cannot be negative")
	}

	return nil
}

func (s *SwitchCondition) register() <-chan struct{} {
	return s.notify
}

func (s *SwitchCondition) UpdateConfig(sc SwitchConfig) error {
	if err := s.validate(sc); err != nil {
		return err
	}

	version := atomic.LoadInt64(&s.version)
	atomic.CompareAndSwapInt64(&s.version, version, version+1)
	atomic.CompareAndSwapInt64(&sc.version, version, version+1)
	sc.version = version + 1
	sc.timeThresholdMillis = sc.TimeThreshold.Milliseconds()
	s.config.Store(sc)

	select {
	case s.notify <- struct{}{}:
	default:
	}

	return nil
}

func (s *SwitchCondition) GetConfig() SwitchConfig {
	return s.config.Load().(SwitchConfig)
}
