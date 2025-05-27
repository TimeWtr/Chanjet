# Chanjet
基于Go Channel、Buffer Pool和零拷贝实现的高性能双缓冲通道。

<div align="center">

<a title="Build Status" target="_blank" href="https://github.com/TimeWtr/Chanjet/actions?query=workflow%3ATests"><img>
<img src="https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go&logoColor=white" alt="Go Version">
<img src="https://img.shields.io/badge/license-Apache2.0-blue" alt="License">
<img src="https://img.shields.io/badge/performance-optimized-brightgreen" alt="Performance">
<a title="Tag" target="_blank" href="https://github.com/TimeWtr/Chanjet/tags"><img src="https://img.shields.io/github/v/tag/TimeWtr/Chanjet?color=%23ff8936&logo=fitbit&style=flat-square" /></a>
<br/>
<a title="Doc for Poolx" target="_blank" href="https://pkg.go.dev/github.com/TimeWtr/Chanjet?tab=doc"><img src="https://img.shields.io/badge/go.dev-doc-007d9c?style=flat-square&logo=read-the-docs" /></a>
</div>

## 📊 Features
🔹 **双写缓冲区设计**：active通道用于实时接收数据，passive通道用于异步处理数据，当active满足
切换逻辑执行通道轮转后，passive转换为active实时写入通道接收数据，active转换成passive异步处理通道。

🔹 **统一读取队列设计**：passive异步缓冲通道需要快速异步的将所有数据写入到统一的readq通道中，接收方(
比如文件写入)只从readq中获取即可，这样即使接收方消费受限也不会明显的阻塞passive通道的使用。

🔹 **复合通道轮转策略**：
    
- 当active通道中数据大小触发一定的阈值，比如最大容量的80%，立即进行轮转。
- 当达到一定时间，比如300MS还没有进行通道轮转时，后台定时程序立即触发通道轮转，防止因长期没有新数据写入
导致接收方无法获取通道内数据的问题，也尽可能减少数据丢失的风险。

🔹 **无锁化设计**：双缓冲通道不使用加锁保护通道切换，使用原子状态实现并发安全的通道切换，大大提升了性能。

## 🚀 Performance
- **系统架构**: darwin/arm64
- **处理器**: Apple M4
- **测试配置**: `-cpu=1 -count=5 -benchmem`
- **测试命令**: `go test -bench="^BenchmarkBuffer_Write$" -run="^$" -cpu=1 -count=5 -benchmem`

### 128B 测试结果
| 测试用例                  | 操作次数   | 单次耗时 (ns/op) | 内存分配 (B/op) | 分配次数 |
|--------------------------|-----------|------------------|-----------------|----------|
| BenchmarkBuffer_Write/128B | 28,357,896 | 46.98            | 35              | 0        |
| BenchmarkBuffer_Write/128B | 26,224,845 | 52.54            | 38              | 0        |
| BenchmarkBuffer_Write/128B | 22,811,332 | 54.09            | 44              | 0        |
| BenchmarkBuffer_Write/128B | 26,970,789 | 55.06            | 55              | 0        |
| BenchmarkBuffer_Write/128B | 23,115,716 | 66.27            | 87              | 0        |

### 64KB 测试结果
| 测试用例                   | 操作次数   | 单次耗时 (ns/op) | 内存分配 (B/op) | 分配次数 |
|---------------------------|-----------|------------------|-----------------|----------|
| BenchmarkBuffer_Write/64KB | 30,947,586 | 40.22            | 32              | 0        |
| BenchmarkBuffer_Write/64KB | 29,636,215 | 41.79            | 33              | 0        |
| BenchmarkBuffer_Write/64KB | 30,254,332 | 41.37            | 49              | 0        |
| BenchmarkBuffer_Write/64KB | 27,079,902 | 42.48            | 37              | 0        |
| BenchmarkBuffer_Write/64KB | 27,172,502 | 41.52            | 55              | 0        |
| BenchmarkBuffer_Write/64KB#01 | 27,797,377 | 40.25            | 36              | 0        |
| BenchmarkBuffer_Write/64KB#01 | 30,421,725 | 43.51            | 49              | 0        |
| BenchmarkBuffer_Write/64KB#01 | 29,338,390 | 43.51            | 51              | 0        |
| BenchmarkBuffer_Write/64KB#01 | 26,996,911 | 41.46            | 37              | 0        |
| BenchmarkBuffer_Write/64KB#01 | 27,486,892 | 45.02            | 36              | 0        |

### 1MB 测试结果
| 测试用例                   | 操作次数   | 单次耗时 (ns/op) | 内存分配 (B/op) | 分配次数 |
|---------------------------|-----------|------------------|-----------------|----------|
| BenchmarkBuffer_Write/1MB | 18,422,181 | 154.1            | 409             | 0        |
| BenchmarkBuffer_Write/1MB | 11,433,486 | 191.2            | 396             | 0        |
| BenchmarkBuffer_Write/1MB | 13,047,794 | 88.17            | 385             | 0        |
| BenchmarkBuffer_Write/1MB | 14,501,538 | 139.9            | 416             | 0        |
| BenchmarkBuffer_Write/1MB | 13,242,762 | 170.8            | 418             | 0        |

> 1. **操作次数**: 列显示测试框架自动计算的`b.N`值
> 2. **内存分配**: 包含通道操作和数据复制的总开销
> 3. **测试组数**: 所有测试结果均运行5次(`-count=5`)，其中64KB包含两组测试(#01标识)


## 📦 Installation
```bash
go get github.com/TimeWtr/Chanjet
```
## 🧩 Usage
```go
package main

import (
	"fmt"
	"sync"

	cj "github.com/TimeWtr/Chanjet"
)

func main() {
	bf, err := cj.NewBuffer(5000, 10)
	if err != nil {
		panic(err)
	}

	ch := bf.Register()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()

		counter := 0
		for range ch {
			counter++
		}
		fmt.Println("通道关闭")
		fmt.Printf("接收到日志数据条数: %d", counter)
	}()

	go func() {
		defer wg.Done()
		defer bf.Close()
		
		template := "2025-05-12 12:12:00 [Info] 日志写入测试，当前的序号为: %d\n"
		for i := 0; i < 2600000; i++ {
			err := bf.Write([]byte(fmt.Sprintf(template, i)))
			if err != nil {
				fmt.Printf("写入日志失败，错误：%s\n", err.Error())
				continue
			}
		}
		fmt.Println("结束了")
	}()

	wg.Wait()
	fmt.Println("写入成功")
}
```