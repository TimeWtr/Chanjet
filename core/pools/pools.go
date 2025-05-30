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

import "sync"

const (
	initializeBufSize = 512       // 初始化的对象
	maxRecycleSize    = 16 * 1024 // 池中最大的回收对象为16KB
)

const (
	bytesPoolPreloads = 200
)

var bytesPool = sync.Pool{
	New: func() interface{} {
		// 复制指针，避免slice Header复制
		buf := make([]byte, 0, initializeBufSize)
		return &buf
	},
}

func GetBytesPool(size int64) *[]byte {
	ptr := bytesPool.Get().(*[]byte)
	buf := *ptr

	if int64(cap(buf)) < size {
		*ptr = make([]byte, size)
		return ptr
	}

	*ptr = buf[:size]
	return ptr
}

func PutBytesPool(buf []byte) {
	buf = buf[:0]
	if cap(buf) > maxRecycleSize {
		return
	}

	bytesPool.Put(&buf)
}
