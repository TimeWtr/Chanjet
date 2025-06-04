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
	"sync"
	"time"

	"github.com/TimeWtr/Chanjet/errorx"
)

type SwitchConfig struct {
	version             int64
	SizeThreshold       int64
	PercentThreshold    int
	TimeThreshold       time.Duration
	timeThresholdMillis int64
}

type SwitchCondition struct {
	config  SwitchConfig
	notify  chan struct{}
	version int64
	mu      sync.RWMutex
}

func NewSwitchCondition(config SwitchConfig) (*SwitchCondition, error) {
	sw := &SwitchCondition{
		notify: make(chan struct{}, 1),
	}

	if err := sw.validate(config); err != nil {
		return nil, err
	}

	sw.version = 1
	config.timeThresholdMillis = config.TimeThreshold.Milliseconds()
	config.version = 1
	sw.config = config
	sw.notify <- struct{}{}

	return sw, nil
}

func (s *SwitchCondition) validate(sc SwitchConfig) error {
	if sc.SizeThreshold <= 0 {
		return errorx.ErrSizeThreshold
	}

	if sc.PercentThreshold < 0 || sc.PercentThreshold > 100 {
		return errorx.ErrPercentThreshold
	}

	if sc.TimeThreshold < 0 {
		return errorx.ErrTimeThreshold
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

	s.mu.Lock()
	defer s.mu.Unlock()
	s.version += 1
	s.config = sc
	s.config.version = s.version
	s.config.timeThresholdMillis = sc.TimeThreshold.Milliseconds()

	select {
	case s.notify <- struct{}{}:
	default:
	}

	return nil
}

func (s *SwitchCondition) GetConfig() SwitchConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.config
}
