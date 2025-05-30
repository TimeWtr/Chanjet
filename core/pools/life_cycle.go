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

package pools

import (
	"sync"
	"time"
)

const (
	MediumPoolTTL = 2 * time.Minute
)

// LifeCycleManager 缓存池生命周期管理器，用于管理小、中、大三种级别大小
// 对象的缓冲池，小对象缓冲池直接调用sync.Pool，中对象带过期时间的缓冲
// 池，后台程序定时清除过期对象，大对象管理器存储每一个对象的引用计数，
// 管理器不进行后台的隐式删除，而是提供方法给调用方显式删除。
type LifeCycleManager struct {
	SmallPool   sync.Pool    // 小对象缓冲池
	MediumPool  *MediumPool  // 中对象缓冲池
	BigDataPool *BigDataPool // 大对象缓冲池
}

func NewLifeCycleManager() *LifeCycleManager {
	return &LifeCycleManager{
		SmallPool: sync.Pool{
			New: func() interface{} {
				return make([]byte, 0, initializeBufSize)
			},
		},
		MediumPool: &MediumPool{
			cache: make(map[uintptr]time.Time),
			mu:    sync.RWMutex{},
			stop:  make(chan struct{}),
		},
		BigDataPool: &BigDataPool{
			pool: make(map[uintptr]BigDataEntry),
			mu:   sync.RWMutex{},
		},
	}
}

func (l *LifeCycleManager) Cleanup() {
	l.MediumPool.cleanup()
	l.BigDataPool.cleanup()
}

type MediumPool struct {
	cache map[uintptr]time.Time // 对象与时间的映射关系
	mu    sync.RWMutex          // 读写锁保护Cache
	stop  chan struct{}         // 停止信号
}

func (m *MediumPool) Set(ptr uintptr, t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cache[ptr] = t
}

// IsValid 判断是ptr是否合法
func (m *MediumPool) IsValid(ptr uintptr) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	t, ok := m.cache[ptr]
	if !ok {
		return false
	}

	return time.Since(t) < MediumPoolTTL
}

// Release 释放资源
func (m *MediumPool) Release(ptr uintptr) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.cache, ptr)
}

func (m *MediumPool) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.cache) == 0 {
		return
	}

	for ptr, t := range m.cache {
		if time.Since(t) > time.Minute {
			delete(m.cache, ptr)
		}
	}
}

type BigDataPool struct {
	pool map[uintptr]BigDataEntry // 对象与Entry的映射关系
	mu   sync.RWMutex             // 读写锁保护
}

func (b *BigDataPool) cleanup() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.pool) == 0 {
		return
	}

	for ptr, bd := range b.pool {
		if bd.ref != 0 {
			continue
		}

		delete(b.pool, ptr)
		bd.data = nil // 释放底层数组
	}
}

func (b *BigDataPool) Put(ptr uintptr, data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.pool[ptr] = BigDataEntry{
		data: data,
		size: len(data),
		ref:  1,
	}
}

// Release 释放资源，减少ptr的引用计数
func (b *BigDataPool) Release(ptr uintptr) {
	b.mu.Lock()
	defer b.mu.Unlock()

	entry, exist := b.pool[ptr]
	if !exist {
		return
	}
	entry.ref--
	b.pool[ptr] = entry
}

type BigDataEntry struct {
	size int    // 数据的大小
	data []byte // 数据
	ref  int    // 引用计数
}
