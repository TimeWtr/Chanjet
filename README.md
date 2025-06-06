# Chanjet
<div align="center">
<a title="Build Status" target="_blank" href="https://github.com/TimeWtr/Chanjet/actions?query=workflow%3ATests"><img>
<img src="https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go&logoColor=white" alt="Go Version">
<img src="https://img.shields.io/badge/license-Apache2.0-blue" alt="License">
<img src="https://img.shields.io/badge/performance-optimized-brightgreen" alt="Performance">
<a title="Tag" target="_blank" href="https://github.com/TimeWtr/Chanjet/tags"><img src="https://img.shields.io/github/v/tag/TimeWtr/Chanjet?color=%23ff8936&logo=fitbit&style=flat-square" /></a>
<br/>
<a title="Doc for Poolx" target="_blank" href="https://pkg.go.dev/github.com/TimeWtr/Chanjet?tab=doc"><img src="https://img.shields.io/badge/go.dev-doc-007d9c?style=flat-square&logo=read-the-docs" /></a>
</div>

基于Go Channel、Buffer Pool和零拷贝实现的高性能双缓冲通道。

## 📊 Features
🔹 **双写缓冲区设计**：active通道用于实时接收数据，passive通道用于异步处理数据，当active满足
切换逻辑执行通道轮转后，passive转换为active实时写入通道接收数据，active转换成passive异步处理通道。

🔹 **消息全局有序特性**：每条消息的按照顺序写入到active通道中，当通道切换时active转换为passive，
切换程序会为passive分配一个全局唯一单调递增的序列号，并将passive添加到小顶堆中，小顶堆根据序列号
进行排序，消费数据时会按照序列号顺序来消息。

🔹 **复合通道轮转策略**：
- 当active通道中数据条数超过容量限制，立即轮转。
- 当达到一定时间，即当前时间距离上次轮转时间较长，超过了时间窗口周期立即进行轮转，防止因长期没有新数据写入
导致接收方无法获取通道内数据的问题，也尽可能减少数据丢失的风险。
- 综合策略，时间因子(40%)、数据条数因子(60%)，根据综合因子判断是否进行通道轮转。

🔹 **无锁化设计**：双缓冲通道不使用加锁保护通道切换，使用原子状态实现并发安全的通道切换，大大提升了性能。

🔹 **缓冲池设计**：使用缓冲池设计，通道切换时复用池中可用通道，防止出现频繁的通道创建和销毁的开销。

🔹 **完善监控的设计**：完善的监控指标设计，支持Prometheus和OpenTelemetry，目前已支持Prometheus指标，抽象批量上报接口，
上报指标数据定时批量刷新到底层指标采集器，3700000条数据写入单条指标上报总耗时1.5秒，批量刷新指标总耗时<1.1秒，耗时减少400毫秒
左右，大大提升了性能。
> 总耗时：数据写入缓冲区+异步写入readq读取通道+指标上报


## 🚀 Performance
- **系统架构**: darwin/arm64
- **处理器**: Apple M4
- **测试配置**: `-cpu=1 -count=5 -benchmem`
- **测试命令**: `go test -bench="^BenchmarkBuffer_Write$" -run="^$" -cpu=1 -count=5 -benchmem`

### 128B 测试结果
| 测试用例                   | 操作次数   | 单次耗时 (ns/op) | 内存分配 (B/op) | 分配次数 |
|---------------------------|-----------|------------------|-----------------|----------|
| BenchmarkBuffer_Write/128B | 9,593,606 | 138.6            | 157             | 0        |
| BenchmarkBuffer_Write/128B | 8,721,645 | 128.9            | 173             | 0        |
| BenchmarkBuffer_Write/128B | 8,517,952 | 133.1            | 118             | 0        |
| BenchmarkBuffer_Write/128B | 8,861,853 | 155.2            | 113             | 0        |
| BenchmarkBuffer_Write/128B | 8,608,171 | 152.7            | 116             | 0        |

### 64KB 测试结果
| 测试用例                    | 操作次数  | 单次耗时 (ns/op) | 内存分配 (B/op) | 分配次数 |
|----------------------------|----------|------------------|-----------------|----------|
| BenchmarkBuffer_Write/64KB  | 7,249,588 | 171.6            | 208             | 0        |
| BenchmarkBuffer_Write/64KB  | 9,140,080 | 140.6            | 110             | 0        |
| BenchmarkBuffer_Write/64KB  | 7,035,427 | 161.3            | 214             | 0        |
| BenchmarkBuffer_Write/64KB  | 8,811,381 | 141.9            | 114             | 0        |
| BenchmarkBuffer_Write/64KB  | 6,769,135 | 187.4            | 297             | 0        |
| BenchmarkBuffer_Write/64KB#01 | 8,653,538 | 142.8            | 116             | 0        |
| BenchmarkBuffer_Write/64KB#01 | 7,285,388 | 168.3            | 276             | 0        |
| BenchmarkBuffer_Write/64KB#01 | 7,607,090 | 166.5            | 198             | 0        |
| BenchmarkBuffer_Write/64KB#01 | 7,639,873 | 146.3            | 131             | 0        |
| BenchmarkBuffer_Write/64KB#01 | 8,885,304 | 254.1            | 339             | 0        |

### 1MB 测试结果
| 测试用例                   | 操作次数  | 单次耗时 (ns/op) | 内存分配 (B/op) | 分配次数 |
|---------------------------|----------|------------------|-----------------|----------|
| BenchmarkBuffer_Write/1MB | 8,231,407 | 179.2            | 489             | 0        |
| BenchmarkBuffer_Write/1MB | 6,488,708 | 175.0            | 387             | 0        |
| BenchmarkBuffer_Write/1MB | 6,460,352 | 214.5            | 545             | 0        |
| BenchmarkBuffer_Write/1MB | 6,894,111 | 214.3            | 657             | 0        |
| BenchmarkBuffer_Write/1MB | 6,569,706 | 169.1            | 383             | 0        |

> 1. **操作次数**: 列显示测试框架自动计算的`b.N`值
> 2. **内存分配**: 包含通道操作和数据复制的总开销
> 3. **测试组数**: 所有测试结果均运行5次(`-count=5`)，其中64KB包含两组测试(#01标识)

## 监控指标说明

### 全局命名空间
所有指标均以 `Chanjet_` 作为命名空间前缀

---

### 写入相关指标
| 指标名称                           | 类型       | 标签/维度       | 描述                                                                 |
|------------------------------------|------------|-----------------|----------------------------------------------------------------------|
| `Chanjet_write_counts_total`       | CounterVec | `result`        | 写入操作总数（标签值：`success` 成功 / `failure` 失败）               |
| `Chanjet_write_sizes_total`        | Counter    | -               | 已写入数据的总字节数（单位：字节）                                   |
| `Chanjet_write_errors_total`       | Counter    | -               | 写入失败的次数（含网络错误、校验失败等场景）                         |

---

### 读取相关指标
| 指标名称                           | 类型       | 标签/维度       | 描述                                                                 |
|------------------------------------|------------|-----------------|----------------------------------------------------------------------|
| `Chanjet_read_counts_total`        | CounterVec | `result`        | 读取操作总数（标签值：`success` 成功 / `failure` 失败）               |
| `Chanjet_read_sizes_total`         | Counter    | -               | 已读取数据的总字节数（单位：字节）                                   |
| `Chanjet_read_errors_total`        | Counter    | -               | 读取失败的次数（含超时、校验失败等场景）                             |

---

### 缓冲区切换指标
| 指标名称                           | 类型       | 描述                                                                 |
|------------------------------------|------------|----------------------------------------------------------------------|
| `Chanjet_switch_counts_total`      | Counter    | 缓冲区切换操作总次数                                                 |
| `Chanjet_switch_latency`           | Histogram  | 切换延迟分布（单位：秒，预设桶边界：[0.001, 0.005, 0.01, 0.05, 0.1]）|
| `Chanjet_skip_switch_counts_total` | Counter    | 定时任务跳过切换的次数（未达到切换条件时计数）                       |

---

### 异步处理指标
| 指标名称                           | 类型       | 描述                                                                 |
|------------------------------------|------------|----------------------------------------------------------------------|
| `Chanjet_async_workers`            | Gauge      | 当前活跃的异步工作协程数量                                           |

---

### 缓冲池指标
| 指标名称                           | 类型       | 描述                                                                 |
|------------------------------------|------------|----------------------------------------------------------------------|
| `Chanjet_pool_alloc_total`         | Counter    | 对象池内存分配次数                                                   |

---

### 通道状态指标
| 指标名称                           | 类型       | 描述                                                                 |
|------------------------------------|------------|----------------------------------------------------------------------|
| `Chanjet_active_channel_data_counts` | Gauge    | 当前活跃通道中未处理的数据条目数量                                   |
| `Chanjet_active_channel_data_sizes`  | Gauge    | 当前活跃通道中未处理的数据总大小（单位：字节）                       |

---

### 指标类型说明
| 类型        | 特性                                                                 |
|-------------|----------------------------------------------------------------------|
| **Counter**   | 只增不减的累积计数器，适用于请求数、错误数等统计                     |
| **Gauge**     | 可任意变化的瞬时值，适用于实时资源用量（如内存、协程数）             |
| **Histogram** | 测量观测值的分布，自动计算分位数，适用于延迟、响应大小等指标         |
| **CounterVec**| 带标签的计数器，支持多维细分统计（如按成功/失败状态分类）            |

---

## 示例 PromQL 查询
```promql
# 计算写入吞吐量（次/秒）
rate(Chanjet_write_counts_total[1m])

# 获取活跃通道数据积压告警（>1MB 持续5分钟）
Chanjet_active_channel_data_sizes > 1e6

# 统计切换延迟的P99值
histogram_quantile(0.99, sum(rate(Chanjet_switch_latency_bucket[5m])) by (le))
```

## 📦 Installation
```bash
go get github.com/TimeWtr/Chanjet
```
## 🧩 Usage
```go
package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	cj "github.com/TimeWtr/Chanjet"
	"github.com/TimeWtr/Chanjet/_const"
	"github.com/TimeWtr/Chanjet/metrics"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func main() {
	ser := gin.Default()
	bf, err := cj.NewBuffer(1025*1024*100,
		cj.WithMetrics(_const.PrometheusCollector))
	if err != nil {
		panic(err)
	}
	ser.GET("/metrics", gin.WrapH(metrics.GetHandler()))

	ch := bf.Register()
	exitChan := make(chan struct{}, 1)
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()

		counter := 0
		for data := range ch {
			fmt.Println("[收到数据]: ", string(data))
			counter++
		}
		fmt.Println("通道关闭")
		fmt.Printf("接收到日志数据条数: %d", counter)
	}()

	go func() {
		defer wg.Done()
		defer bf.Close()

		template := "2025-05-12 12:12:00 [Info] 日志写入测试，当前的序号为: %d\n"
		for i := 0; i < 3100000000; i++ {
			err := bf.Write([]byte(fmt.Sprintf(template, i)))
			if err != nil {
				fmt.Printf("写入日志失败，错误：%s\n", err.Error())
				continue
			}
		}
		fmt.Println("结束了")
	}()

	// HTTP 服务协程
	go func() {
		defer wg.Done()

		srv := &http.Server{
			Addr:    ":8080",
			Handler: ser,
		}

		// 优雅关闭处理
		go func() {
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
			<-sigChan

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := srv.Shutdown(ctx); err != nil {
				fmt.Printf("服务关闭异常: %v\n", err)
			}
			exitChan <- struct{}{} // 发送退出信号
		}()

		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			fmt.Printf("服务启动失败: %v\n", err)
			exitChan <- struct{}{}
		}
	}()

	wg.Wait()
	fmt.Println("写入成功")
}

```