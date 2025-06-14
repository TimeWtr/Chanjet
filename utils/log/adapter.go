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
	"sync"
)

var (
	mu           sync.RWMutex
	adapters     = map[LoggerType]func() Core{}
	currentLevel = LevelInfo
)

func Register(tp LoggerType, adapter func() Core) error {
	mu.Lock()
	defer mu.Unlock()
	if tp.valid() {
		return errors.New("logger type is invalid")
	}

	adapters[tp] = adapter
	return nil
}

func New(tp LoggerType) (Core, error) {
	mu.Lock()
	defer mu.Unlock()

	if !tp.valid() {
		return nil, errors.New("logger type is invalid")
	}

	adapter, ok := adapters[tp]
	if !ok {
		return nil, errors.New("logger adapter not exist, please init adapter")
	}

	return adapter(), nil
}

func SetLevel(level Level) {
	mu.Lock()
	defer mu.Unlock()
	currentLevel = level
}

func getLevel() Level {
	mu.RLock()
	defer mu.RUnlock()
	return currentLevel
}
