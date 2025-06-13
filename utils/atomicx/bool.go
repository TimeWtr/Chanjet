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

package atomicx

import "sync/atomic"

type Bool struct {
	value uint32
}

func NewBool() *Bool {
	return &Bool{value: 0}
}

func (b *Bool) SetTrue() {
	atomic.StoreUint32(&b.value, 1)
}

func (b *Bool) SetFalse() {
	atomic.StoreUint32(&b.value, 0)
}

func (b *Bool) Load() bool {
	return atomic.LoadUint32(&b.value) == 1
}

func (b *Bool) Store(val bool) {
	if val {
		atomic.StoreUint32(&b.value, 1)
	} else {
		atomic.StoreUint32(&b.value, 0)
	}
}

func (b *Bool) Swap(newVal bool) bool {
	var v uint32
	if newVal {
		v = 1
	}

	return atomic.SwapUint32(&b.value, v) == 1
}

func (b *Bool) CompareAndSwap(oldVal, newVal bool) bool {
	var (
		o uint32
		n uint32
	)
	if oldVal {
		o = 1
	}

	if newVal {
		n = 1
	}

	return atomic.CompareAndSwapUint32(&b.value, o, n)
}
