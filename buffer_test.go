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
	"bytes"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/TimeWtr/Chanjet/_const"
	"github.com/stretchr/testify/assert"
)

func TestNewBuffer(t *testing.T) {
	bf, err := NewBuffer(5000, WithMetrics(_const.PrometheusCollector))
	assert.NoError(t, err)

	ch := bf.Register()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()

		counter := 0
		for range ch {
			counter++
		}
		t.Log("通道关闭")
		t.Logf("接收到日志数据条数: %d", counter)
	}()

	go func() {
		defer wg.Done()
		defer bf.Close()

		// 64KB数据，每秒钟的传输量约为250万条
		template := "2025-05-12 12:12:00 [Info] 日志写入测试，当前的序号为: %d\n"
		for i := 0; i < 3700000; i++ {
			err = bf.Write([]byte(fmt.Sprintf(template, i)))
			if err != nil {
				t.Logf("写入日志失败，错误：%s\n", err.Error())
				continue
			}
		}
		t.Log("结束了")
	}()

	wg.Wait()
	t.Log("写入成功")
}

func generateData(size int) ([]byte, error) {
	data := make([]byte, size)
	n, err := rand.Read(data)
	if err != nil {
		return nil, fmt.Errorf("生成随机数据失败，错误：%w", err)
	}
	if n != size {
		return nil, fmt.Errorf("生成随机数据的长度不一致")
	}

	return data, nil
}

func TestGenerateData(t *testing.T) {
	testCases := []struct {
		name    string
		size    int
		wantErr error
	}{
		{
			name:    "1B",
			size:    1,
			wantErr: nil,
		},
		{
			name:    "100B",
			size:    100,
			wantErr: nil,
		},
		{
			name:    "1KB",
			size:    1024,
			wantErr: nil,
		},
		{
			name:    "10KB",
			size:    10 * 1024,
			wantErr: nil,
		},
		{
			name:    "1MB",
			size:    1024 * 1024,
			wantErr: nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			data, err := generateData(testCase.size)
			assert.Equal(t, testCase.wantErr, err)
			if err != nil {
				assert.Equal(t, testCase.size, len(data))
			}
		})
	}
}

// TestNewBuffer_1B_5000 1Byte大小的数据，缓冲区容量设置为5000条，1s的测试传输数据量为310万
func TestNewBuffer_1B_5000(t *testing.T) {
	bf, err := NewBuffer(5000)
	assert.NoError(t, err)

	data, err := generateData(1)
	assert.NoError(t, err)

	ch := bf.Register()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()

		counter := 0
		for range ch {
			counter++
		}
		t.Log("通道关闭")
		t.Logf("接收到日志数据条数: %d", counter)
	}()

	go func(data []byte) {
		defer wg.Done()
		defer bf.Close()

		for i := 0; i < 3100000; i++ {
			err := bf.Write(data)
			if err != nil {
				t.Logf("写入日志失败，错误：%s\n", err.Error())
				continue
			}
		}
		t.Log("结束了")
	}(data)

	wg.Wait()
	t.Log("写入成功")
}

// TestNewBuffer_1B_6000 1Byte大小的数据，缓冲区容量设置为6000条，1s的测试传输数据量为310万
func TestNewBuffer_1B_6000(t *testing.T) {
	bf, err := NewBuffer(6000)
	assert.NoError(t, err)

	data, err := generateData(1)
	assert.NoError(t, err)

	ch := bf.Register()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()

		counter := 0
		for range ch {
			counter++
		}
		t.Log("通道关闭")
		t.Logf("接收到日志数据条数: %d", counter)
	}()

	go func(data []byte) {
		defer wg.Done()
		defer bf.Close()

		for i := 0; i < 3200000; i++ {
			err := bf.Write(data)
			if err != nil {
				t.Logf("写入日志失败，错误：%s\n", err.Error())
				continue
			}
		}
		t.Log("结束了")
	}(data)

	wg.Wait()
	t.Log("写入成功")
}

// TestNewBuffer_100B 100Byte大小的数据，缓冲区容量设置为5000条，1s的测试传输数据量为310万
func TestNewBuffer_100B_5000(t *testing.T) {
	bf, err := NewBuffer(5000)
	assert.NoError(t, err)

	data, err := generateData(100)
	assert.NoError(t, err)

	ch := bf.Register()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()

		counter := 0
		for range ch {
			counter++
		}
		t.Log("通道关闭")
		t.Logf("接收到日志数据条数: %d", counter)
	}()

	go func(data []byte) {
		defer wg.Done()
		defer bf.Close()

		for i := 0; i < 3200000; i++ {
			err := bf.Write(data)
			if err != nil {
				t.Logf("写入日志失败，错误：%s\n", err.Error())
				continue
			}
		}
		t.Log("结束了")
	}(data)

	wg.Wait()
	t.Log("写入成功")
}

// TestNewBuffer_1KB_5000 1KB大小的数据，缓冲区容量设置为5000条，1s的测试传输数据量为310万
func TestNewBuffer_1KB_5000(t *testing.T) {
	bf, err := NewBuffer(5000)
	assert.NoError(t, err)

	data, err := generateData(1024 * 10)
	assert.NoError(t, err)

	ch := bf.Register()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()

		counter := 0
		for range ch {
			counter++
		}
		t.Log("通道关闭")
		t.Logf("接收到日志数据条数: %d", counter)
	}()

	go func(data []byte) {
		defer wg.Done()
		defer bf.Close()

		for i := 0; i < 3200000; i++ {
			err := bf.Write(data)
			if err != nil {
				t.Logf("写入日志失败，错误：%s\n", err.Error())
				continue
			}
		}
		t.Log("结束了")
	}(data)

	wg.Wait()
	t.Log("写入成功")
}

// TestNewBuffer_10KB_5000 1KB大小的数据，缓冲区容量设置为5000条，1s的测试传输数据量为310万
func TestNewBuffer_10KB_5000(t *testing.T) {
	bf, err := NewBuffer(10000)
	assert.NoError(t, err)

	data, err := generateData(1024)
	assert.NoError(t, err)

	ch := bf.Register()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()

		counter := 0
		for range ch {
			counter++
		}
		t.Log("通道关闭")
		t.Logf("接收到日志数据条数: %d", counter)
	}()

	go func(data []byte) {
		defer wg.Done()
		defer bf.Close()

		for i := 0; i < 3200000; i++ {
			err := bf.Write(data)
			if err != nil {
				t.Logf("写入日志失败，错误：%s\n", err.Error())
				continue
			}
		}
		t.Log("结束了")
	}(data)

	wg.Wait()
	t.Log("写入成功")
}

func BenchmarkNewBuffer(b *testing.B) {
	bf, err := NewBuffer(5000, WithMetrics(_const.PrometheusCollector))
	assert.NoError(b, err)

	ch := bf.Register()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()

		counter := 0
		for {
			select {
			case data, ok := <-ch:
				if !ok {
					b.Log("通道关闭")
					b.Logf("接收到日志数据条数: %d", counter)
					return
				}
				b.Logf("【读取日志】日志内容为：%s", data)
				counter++
			default:

			}
		}
	}()

	go func() {
		defer wg.Done()
		defer bf.Close()

		template := "2025-05-12 12:12:00 [Info] 日志写入测试，当前的序号为: "
		for i := 0; i < b.N; i++ {
			var builder strings.Builder
			builder.WriteString(template)
			builder.WriteString(strconv.Itoa(i))
			builder.WriteString("\n")
			err := bf.Write([]byte(builder.String()))
			if err != nil {
				b.Logf("写入日志失败，错误：%s\n", err.Error())
				continue
			}
			b.Logf("日志%d写入成功\n", i)
		}
	}()

	wg.Wait()
	b.Log("写入成功")
}

func BenchmarkNewBuffer_No_Log(b *testing.B) {
	bf, err := NewBuffer(5000)
	assert.NoError(b, err)

	ch := bf.Register()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()

		counter := 0
		for {
			select {
			case _, ok := <-ch:
				if !ok {
					b.Log("收到日志条数: ", counter)
					return
				}
				counter++
			default:

			}
		}
	}()

	go func() {
		defer wg.Done()
		defer bf.Close()

		template := []byte("2025-05-12 12:12:00 [Info] 日志写入测试，当前的序号 1234535453534")
		for i := 0; i < b.N; i++ {
			err = bf.Write(template)
			if err != nil {
				b.Logf("写入日志失败，错误：%s\n", err.Error())
				continue
			}
		}
	}()

	wg.Wait()
	b.Log("写入成功")
}

// 基准测试组配置
var testCases = []struct {
	name      string
	batchSize int
	dataSize  int
}{
	{"Small-1KBx100", 100, 1024},
	{"Medium-10KBx1k", 1000, 10240},
	{"Large-100KBx10k", 10000, 102400},
}

// 主基准测试函数
func BenchmarkBuffer(b *testing.B) {
	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			buffer, _ := NewBuffer(1024 * 1024)
			defer buffer.Close()

			dataPool := make([][]byte, tc.batchSize)
			for i := 0; i < tc.batchSize; i++ {
				dataPool[i], _ = generateData(tc.dataSize)
			}

			b.ResetTimer()
			b.SetParallelism(8) // 8个并发写入goroutine

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					for _, data := range dataPool {
						if err := buffer.Write(data); err != nil {
							b.Fatal(err)
						}
					}
				}
			})

			// 验证数据完整性
			var total int
			for d := range buffer.Register() {
				total += len(d)
			}
			b.Logf("数据校验: 写入量 %d MB", total/(1024*1024))
		})
	}
}

// 测试不同数据大小的基准测试
func BenchmarkBuffer_Write(b *testing.B) {
	sizes := []int{
		128,     // 小数据 (128B)
		1024,    // 中数据 (1KB)
		65536,   // 大数据 (64KB)
		1 << 20, // 超大数据 (1MB)
	}

	for _, size := range sizes {
		b.Run(formatSize(size), func(b *testing.B) {
			buf, _ := NewBuffer(20*1024*1024, WithMetrics(_const.PrometheusCollector))
			defer buf.Close()

			// 预填充测试数据
			data := bytes.Repeat([]byte("x"), size)

			b.ResetTimer()
			b.SetParallelism(8) // 模拟并发写入
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_ = buf.Write(data)
				}
			})
		})
	}
}

func formatSize(bytes int) string {
	switch {
	case bytes >= 1<<20:
		return "1MB"
	case bytes >= 1<<10:
		return "64KB"
	default:
		return "128B"
	}
}
