// bench 对 calculator server 做并发压测，输出 QPS、延迟分布。
// 用法：
//
//	# 先启动 server
//	go run examples/calculator/server/main.go
//
//	# 再跑压测（默认 8 并发、10 秒）
//	go run examples/calculator/bench/main.go
//	go run examples/calculator/bench/main.go -c 32 -d 30s
package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ecstasoy/RPCinGo/examples/proto/calculator"
	"github.com/ecstasoy/RPCinGo/pkg/client"
)

func main() {
	concurrency := flag.Int("c", 8, "并发 goroutine 数")
	duration := flag.Duration("d", 10*time.Second, "压测持续时间")
	addr := flag.String("addr", "127.0.0.1:8080", "server 地址")
	flag.Parse()

	fmt.Printf("压测目标: %s  并发: %d  时长: %s\n\n", *addr, *concurrency, *duration)

	// 每个 goroutine 独立一个 client（各自连接池）
	var (
		totalOps  int64
		totalErrs int64
		latencies []int64 // nanoseconds，用 sync.Mutex 保护
		latMu     sync.Mutex
	)

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			cli, err := client.NewClient(*addr)
			if err != nil {
				fmt.Fprintf(os.Stderr, "连接失败: %v\n", err)
				atomic.AddInt64(&totalErrs, 1)
				return
			}
			defer cli.Close()

			req := &calculator.AddRequest{A: 3, B: 4}
			resp := &calculator.AddResponse{}

			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				start := time.Now()
				callCtx, callCancel := context.WithTimeout(ctx, 3*time.Second)
				_, err := cli.CallTyped(callCtx, "Calculator", "Add", req, resp)
				callCancel()
				elapsed := time.Since(start).Nanoseconds()

				if err != nil {
					atomic.AddInt64(&totalErrs, 1)
				} else {
					atomic.AddInt64(&totalOps, 1)
					latMu.Lock()
					latencies = append(latencies, elapsed)
					latMu.Unlock()
				}
			}
		}()
	}

	// 每秒打印实时 QPS
	ticker := time.NewTicker(time.Second)
	go func() {
		prev := int64(0)
		for {
			select {
			case <-ticker.C:
				cur := atomic.LoadInt64(&totalOps)
				fmt.Printf("  QPS: %d  累计: %d  错误: %d\n",
					cur-prev, cur, atomic.LoadInt64(&totalErrs))
				prev = cur
			case <-ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()

	wg.Wait()

	// 汇总
	ops := atomic.LoadInt64(&totalOps)
	errs := atomic.LoadInt64(&totalErrs)
	qps := float64(ops) / duration.Seconds()

	fmt.Printf("\n========== 结果 ==========\n")
	fmt.Printf("总请求:   %d\n", ops)
	fmt.Printf("错误:     %d\n", errs)
	fmt.Printf("平均 QPS: %.0f\n", qps)

	if len(latencies) > 0 {
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		n := len(latencies)
		avg := mean(latencies)
		fmt.Printf("\n延迟分布（单位 µs）:\n")
		fmt.Printf("  avg: %.1f\n", avg/1000)
		fmt.Printf("  p50: %.1f\n", float64(latencies[n*50/100])/1000)
		fmt.Printf("  p90: %.1f\n", float64(latencies[n*90/100])/1000)
		fmt.Printf("  p99: %.1f\n", float64(latencies[n*99/100])/1000)
		fmt.Printf("  max: %.1f\n", float64(latencies[n-1])/1000)
		fmt.Printf("  std: %.1f\n", stddev(latencies)/1000)
	}
}

func mean(ns []int64) float64 {
	sum := int64(0)
	for _, v := range ns {
		sum += v
	}
	return float64(sum) / float64(len(ns))
}

func stddev(ns []int64) float64 {
	avg := mean(ns)
	sum := 0.0
	for _, v := range ns {
		d := float64(v) - avg
		sum += d * d
	}
	return math.Sqrt(sum / float64(len(ns)))
}
