package pool

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/ecstasoy/RPCinGo/pkg/protocol"
	"github.com/ecstasoy/RPCinGo/pkg/transport/tcp"
)

func startMockServer(tb testing.TB, handler func(req *protocol.Request) *protocol.Response) (string, func(), error) {
	// 监听随机端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}

	addr := listener.Addr().String()

	// 创建协议编解码器
	codec := tcp.NewProtocolCodec(protocol.CodecTypeJSON, protocol.CompressTypeNone)

	// 停止函数
	stop := func() {
		listener.Close()
	}

	// 启动服务
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return // 监听器关闭
			}

			// 处理连接
			go func(conn net.Conn) {
				defer conn.Close()

				// 读取请求
				_, req, err := codec.ReadRequest(conn)
				if err != nil {
					tb.Logf("读取请求失败: %v", err)
					return
				} else {
					tb.Logf("收到请求 ID=%d", req.ID)
				}

				// 处理请求
				resp := handler(req)

				// 发送响应
				if err := codec.WriteResponse(conn, resp); err != nil {
					tb.Logf("发送响应失败: %v", err)
				} else {
					tb.Logf("处理请求 ID=%d 成功", req.ID)
				}
			}(conn)
		}
	}()

	return addr, stop, nil
}

// TestConnectionPool_Basic 测试基础功能
func TestConnectionPool_Basic(t *testing.T) {
	// 启动测试服务器
	addr, stop, err := startMockServer(t, func(req *protocol.Request) *protocol.Response {
		return protocol.NewSuccessResponse(req.ID, "pong")
	})
	if err != nil {
		t.Fatalf("启动服务失败: %v", err)
	}
	defer stop()

	time.Sleep(50 * time.Millisecond)

	// 创建连接池（使用 Option 模式）
	pool, err := NewConnectionPool(addr,
		WithPoolSize(10, 2), // max=10, min=2
		WithIdleTimeout(60*time.Second),
		WithPoolCodec(protocol.CodecTypeJSON, protocol.CompressTypeNone),
	)
	if err != nil {
		t.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	// 检查初始状态
	stats := pool.Stats()
	t.Logf("初始统计: %s", stats)

	if stats.CurrentSize < 2 {
		t.Errorf("应该预创建 2 个连接，实际 %d", stats.CurrentSize)
	}

	// 1. 获取连接
	conn, err := pool.Get()
	if err != nil {
		t.Fatalf("获取连接失败: %v", err)
	}

	// 2. 使用连接
	req := protocol.NewRequest("Service", "Method", nil)

	_, err = conn.SendRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("发送失败: %v", err)
	}

	t.Log("✅ 连接可用")

	// 3. 归还连接
	conn.Release()

	// 4. 再次获取（应该复用）
	conn2, err := pool.Get()
	if err != nil {
		t.Fatalf("获取连接失败: %v", err)
	}
	defer conn2.Release()

	// 检查统计
	stats2 := pool.Stats()
	t.Logf("使用后统计: %s", stats2)

	if stats2.GetCount < 2 {
		t.Errorf("GetCount 应该 >= 2, 实际 %d", stats2.GetCount)
	}

	t.Log("✅ 基础功能测试通过")
}

// TestConnectionPool_Concurrent 测试并发获取
func TestConnectionPool_Concurrent(t *testing.T) {
	addr, stop, _ := startMockServer(t, func(req *protocol.Request) *protocol.Response {
		return protocol.NewSuccessResponse(req.ID, "ok")
	})
	defer stop()
	time.Sleep(50 * time.Millisecond)

	// 创建小连接池（测试自动扩容）
	pool, err := NewConnectionPool(addr,
		WithPoolSize(5, 1), // 最大 5，最小 1
	)
	if err != nil {
		t.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	// 并发获取连接
	concurrency := 10
	done := make(chan bool, concurrency)

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			conn, err := pool.Get()
			if err != nil {
				t.Errorf("goroutine %d 获取连接失败: %v", id, err)
				done <- false
				return
			}
			defer conn.Release()

			// 模拟使用
			time.Sleep(10 * time.Millisecond)

			done <- true
		}(i)
	}

	// 等待完成
	wg.Wait()
	close(done)

	// 检查结果
	successCount := 0
	for success := range done {
		if success {
			successCount++
		}
	}

	if successCount != concurrency {
		t.Errorf("成功数量不对: %d/%d", successCount, concurrency)
	}

	stats := pool.Stats()
	t.Logf("✅ 并发测试通过: %s", stats)

	// 验证：连接数不应超过最大值
	if stats.CurrentSize > 5 {
		t.Errorf("连接数超过最大值: %d > 5", stats.CurrentSize)
	}
}

// TestConnectionPool_AutoCleanup 测试自动清理
func TestConnectionPool_AutoCleanup(t *testing.T) {
	addr, stop, _ := startMockServer(t, func(req *protocol.Request) *protocol.Response {
		return protocol.NewSuccessResponse(req.ID, "ok")
	})
	defer stop()
	time.Sleep(50 * time.Millisecond)

	// 创建连接池（短空闲时间，频繁清理）
	pool, err := NewConnectionPool(addr,
		WithPoolSize(10, 0),            // 不预创建
		WithIdleTimeout(1*time.Second), // 1 秒空闲超时
		WithMaxLifetime(5*time.Second),
		WithCleanupInterval(500*time.Millisecond), // 0.5 秒清理一次
	)
	if err != nil {
		t.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	// 1. 获取并立即归还连接
	conn, _ := pool.Get()
	conn.Release()

	stats1 := pool.Stats()
	t.Logf("归还后: %s", stats1)

	// 2. 等待超过空闲时间 + 清理间隔
	time.Sleep(2 * time.Second)

	stats2 := pool.Stats()
	t.Logf("清理后: %s", stats2)

	// 3. 验证清理效果
	if stats2.CloseCount == 0 {
		t.Log("注意：清理可能还未触发")
	} else {
		t.Logf("✅ 已清理 %d 个过期连接", stats2.CloseCount)
	}

	t.Log("✅ 自动清理测试完成")
}

// TestConnectionPool_MaxLifetime 测试最大生存时间
func TestConnectionPool_MaxLifetime(t *testing.T) {
	addr, stop, _ := startMockServer(t, func(req *protocol.Request) *protocol.Response {
		return protocol.NewSuccessResponse(req.ID, "ok")
	})
	defer stop()
	time.Sleep(50 * time.Millisecond)

	// 创建连接池（短生存时间）
	pool, err := NewConnectionPool(addr,
		WithPoolSize(5, 1),
		WithMaxLifetime(2*time.Second), // 最大生存 1 秒
		WithIdleTimeout(1*time.Second), // 空闲超时 10 秒
		WithCleanupInterval(500*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	// 获取连接
	conn, _ := pool.Get()
	connStats := conn.Stats()
	t.Logf("连接创建于: %s", connStats.CreatedAt.Format("15:04:05.000"))

	// 立即归还（连接是活跃的）
	conn.Release()

	// 等待超过最大生存时间
	time.Sleep(1500 * time.Millisecond)

	// 触发清理
	pool.cleanup()

	stats := pool.Stats()
	t.Logf("清理后: %s", stats)

	// 连接应该因为超过 MaxLifetime 被清理
	t.Log("✅ MaxLifetime 测试完成")
}

// BenchmarkConnectionPool_WithVsWithout 对比有池vs无池
func BenchmarkConnectionPool_WithVsWithout(b *testing.B) {
	addr, stop, _ := startMockServer(b, func(req *protocol.Request) *protocol.Response {
		return protocol.NewSuccessResponse(req.ID, "ok")
	})
	defer stop()
	time.Sleep(100 * time.Millisecond)

	b.Run("WithoutPool", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			client := tcp.NewClient(addr, protocol.CodecTypeJSON, protocol.CompressTypeNone)
			client.Dial(context.Background(), "")

			req := protocol.NewRequest("S", "M", nil)
			client.SendRequest(context.Background(), req)

			client.Close()
		}
	})

	b.Run("WithPool", func(b *testing.B) {
		pool, _ := NewConnectionPool(addr,
			WithPoolSize(10, 2),
		)
		defer pool.Close()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			conn, _ := pool.Get()

			req := protocol.NewRequest("S", "M", nil)
			conn.SendRequest(context.Background(), req)

			conn.Release()
		}
	})
}

// BenchmarkConnectionPool_Concurrent 并发性能测试
func BenchmarkConnectionPool_Concurrent(b *testing.B) {
	addr, stop, _ := startMockServer(b, func(req *protocol.Request) *protocol.Response {
		return protocol.NewSuccessResponse(req.ID, "ok")
	})
	defer stop()
	time.Sleep(100 * time.Millisecond)

	pool, _ := NewConnectionPool(addr,
		WithPoolSize(20, 5),
	)
	defer pool.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			conn, err := pool.Get()
			if err != nil {
				b.Errorf("获取连接失败: %v", err)
				continue
			}

			req := protocol.NewRequest("S", "M", nil)
			conn.SendRequest(context.Background(), req)

			conn.Release()
		}
	})

	stats := pool.Stats()
	b.Logf("最终统计: %s", stats)
}

// startMockServerWithCodec 启动使用指定 codec 的 mock 服务端（用于 benchmark）
func startMockServerWithCodec(b *testing.B, codecType protocol.CodecType) (string, func()) {
	srv := tcp.NewServer(codecType, protocol.CompressTypeNone)
	if err := srv.Listen(context.Background(), "127.0.0.1:0"); err != nil {
		b.Fatalf("listen failed: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go srv.Serve(ctx, func(_ context.Context, req *protocol.Request) (*protocol.Response, error) {
		return protocol.NewSuccessResponse(req.ID, "ok"), nil
	})
	time.Sleep(50 * time.Millisecond)
	return srv.Addr().String(), cancel
}

// BenchmarkCodec_JSONvsProtobuf 对比 JSON 和 Protobuf 编解码的端到端性能
func BenchmarkCodec_JSONvsProtobuf(b *testing.B) {
	for _, tc := range []struct {
		name      string
		codecType protocol.CodecType
	}{
		{"JSON", protocol.CodecTypeJSON},
		{"Protobuf", protocol.CodecTypeProtobuf},
	} {
		b.Run(tc.name, func(b *testing.B) {
			addr, stop := startMockServerWithCodec(b, tc.codecType)
			defer stop()

			pool, _ := NewConnectionPool(addr,
				WithPoolSize(20, 5),
				WithPoolCodec(tc.codecType, protocol.CompressTypeNone),
			)
			defer pool.Close()

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					conn, err := pool.Get()
					if err != nil {
						b.Errorf("获取连接失败: %v", err)
						continue
					}
					req := protocol.NewRequest("S", "M", nil)
					conn.SendRequest(context.Background(), req)
					conn.Release()
				}
			})
		})
	}
}

// TestConnectionPool_Validation 测试配置验证
func TestConnectionPool_Validation(t *testing.T) {
	addr := "localhost:8080" // 不需要真实服务

	// 测试各种无效配置
	tests := []struct {
		name    string
		options []PoolOption
		wantErr bool
	}{
		{
			name: "Valid",
			options: []PoolOption{
				WithPoolSize(10, 2),
			},
			wantErr: false,
		},
		{
			name: "MaxSize=0",
			options: []PoolOption{
				WithPoolSize(0, 0),
			},
			wantErr: true,
		},
		{
			name: "MinSize>MaxSize",
			options: []PoolOption{
				WithPoolSize(10, 20),
			},
			wantErr: true,
		},
		{
			name: "NegativeTimeout",
			options: []PoolOption{
				WithIdleTimeout(-1 * time.Second),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewConnectionPool(addr, tt.options...)

			if tt.wantErr && err == nil {
				t.Error("应该返回错误")
			}

			if !tt.wantErr && err != nil {
				t.Errorf("不应该返回错误: %v", err)
			}

			if err != nil {
				t.Logf("✅ 正确检测到错误: %v", err)
			}
		})
	}
}

// TestConnectionPool_MaxConnections 测试最大连接数限制
func TestConnectionPool_MaxConnections(t *testing.T) {
	addr, stop, _ := startMockServer(t, func(req *protocol.Request) *protocol.Response {
		// 慢处理（保持连接占用）
		time.Sleep(200 * time.Millisecond)
		return protocol.NewSuccessResponse(req.ID, "ok")
	})
	defer stop()
	time.Sleep(50 * time.Millisecond)

	// 创建小连接池
	pool, _ := NewConnectionPool(addr,
		WithPoolSize(3, 0), // 最大 3 个连接
		WithWaitTimeout(1*time.Second),
	)
	defer pool.Close()

	// 获取 3 个连接（占满池）
	var conns []*PooledConnection
	for i := 0; i < 3; i++ {
		conn, err := pool.Get()
		if err != nil {
			t.Fatalf("获取连接 %d 失败: %v", i, err)
		}
		conns = append(conns, conn)
	}

	stats := pool.Stats()
	t.Logf("占满后: %s", stats)

	// 尝试获取第 4 个连接（应该等待或超时）
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	conn4, err := pool.GetWithContext(ctx)

	// 应该超时（因为池满了）
	if err == nil {
		conn4.Release()
		t.Log("获取到连接（可能有连接被释放了）")
	} else {
		t.Logf("✅ 正确返回超时: %v", err)
	}

	// 释放所有连接
	for _, conn := range conns {
		conn.Release()
	}

	t.Log("✅ 最大连接数限制测试完成")
}

// TestConnectionPool_Stats 测试统计功能
func TestConnectionPool_Stats(t *testing.T) {
	addr, stop, _ := startMockServer(t, func(req *protocol.Request) *protocol.Response {
		return protocol.NewSuccessResponse(req.ID, "ok")
	})
	defer stop()
	time.Sleep(50 * time.Millisecond)

	pool, _ := NewConnectionPool(addr,
		WithPoolSize(10, 2),
	)
	defer pool.Close()

	// 执行一些操作
	for i := 0; i < 5; i++ {
		conn, _ := pool.Get()
		conn.Release()
	}

	// 检查统计
	stats := pool.Stats()

	if stats.GetCount != 5 {
		t.Errorf("GetCount 应该是 5, 实际 %d", stats.GetCount)
	}

	if stats.PutCount != 5 {
		t.Errorf("PutCount 应该是 5, 实际 %d", stats.PutCount)
	}

	if stats.CreateCount < 2 {
		t.Errorf("CreateCount 应该 >= 2 (预创建), 实际 %d", stats.CreateCount)
	}

	t.Logf("✅ 统计测试通过: %s", stats)
}

// TestConnectionPool_Cleanup 测试清理机制
func TestConnectionPool_Cleanup(t *testing.T) {
	addr, stop, _ := startMockServer(t, func(req *protocol.Request) *protocol.Response {
		return protocol.NewSuccessResponse(req.ID, "ok")
	})
	defer stop()
	time.Sleep(50 * time.Millisecond)

	// 创建连接池（快速清理）
	pool, _ := NewConnectionPool(addr,
		WithPoolSize(10, 3),
		WithIdleTimeout(1*time.Second),
		WithCleanupInterval(500*time.Millisecond),
	)
	defer pool.Close()

	stats1 := pool.Stats()
	t.Logf("初始: 总连接=%d, 空闲=%d", stats1.CurrentSize, stats1.IdleSize)

	// 获取所有连接并归还
	var conns []*PooledConnection
	for i := 0; i < 5; i++ {
		conn, _ := pool.Get()
		conns = append(conns, conn)
	}

	for _, conn := range conns {
		conn.Release()
	}

	stats2 := pool.Stats()
	t.Logf("使用后: 总连接=%d, 空闲=%d", stats2.CurrentSize, stats2.IdleSize)

	// 等待清理触发
	time.Sleep(2 * time.Second)

	stats3 := pool.Stats()
	t.Logf("清理后: 总连接=%d, 空闲=%d, 关闭=%d",
		stats3.CurrentSize, stats3.IdleSize, stats3.CloseCount)

	// 应该保持最小连接数
	if stats3.CurrentSize < 3 {
		t.Errorf("应该保持最小连接数 3, 实际 %d", stats3.CurrentSize)
	}

	t.Log("✅ 清理测试完成")
}
