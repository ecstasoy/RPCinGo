package tcp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/ecstasoy/RPCinGo/pkg/protocol"
)

// startMockServer 启动一个 Mock 服务端（用于测试）
// handler: 处理请求的函数
// 返回: 监听地址, 停止函数, 错误
func startMockServer(tb testing.TB, handler func(req *protocol.Request) *protocol.Response) (string, func(), error) {
	// 监听随机端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}

	addr := listener.Addr().String()

	// 创建协议编解码器
	codec := NewProtocolCodec(protocol.CodecTypeJSON, protocol.CompressTypeNone)

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

// TestClient_DialAndClose 测试连接和关闭
func TestClient_DialAndClose(t *testing.T) {
	// 启动 Mock 服务
	addr, stop, err := startMockServer(t, func(req *protocol.Request) *protocol.Response {
		return protocol.NewSuccessResponse(req.ID, "ok")
	})
	if err != nil {
		t.Fatalf("启动服务失败: %v", err)
	}
	defer stop()

	// 等待服务启动
	time.Sleep(50 * time.Millisecond)

	// 创建客户端
	client := NewClient(addr, protocol.CodecTypeJSON, protocol.CompressTypeNone)

	// 连接
	ctx := context.Background()
	if err := client.Dial(ctx, ""); err != nil {
		t.Fatalf("连接失败: %v", err)
	} else {
		t.Log("连接成功, 地址:", addr)
	}

	// 检查状态
	if !client.IsConnected() {
		t.Error("应该已连接")
	}

	if client.RemoteAddr() == nil {
		t.Error("RemoteAddr 不应该为 nil")
	} else {
		t.Logf("远程地址: %s", client.RemoteAddr().String())
	}

	t.Logf("已连接到: %s", client.RemoteAddr())

	// 关闭
	if err := client.Close(); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}

	// 检查状态
	if client.IsConnected() {
		t.Error("应该已断开")
	}

	t.Log("✅ 连接和关闭测试通过")
}

// startMuxMockServer 启动支持多路复用的 Mock 服务端：
// 每个连接上并发处理多个请求，响应通过 writeMu 串行写回。
// handler 可以睡眠来模拟乱序响应。
func startMuxMockServer(tb testing.TB, handler func(req *protocol.Request) *protocol.Response) (string, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}

	addr := listener.Addr().String()
	codec := NewProtocolCodec(protocol.CodecTypeJSON, protocol.CompressTypeNone)
	stop := func() { listener.Close() }

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				var writeMu sync.Mutex

				for {
					_, req, err := codec.ReadRequest(conn)
					if err != nil {
						return
					}
					go func(req *protocol.Request) {
						resp := handler(req)
						writeMu.Lock()
						codec.WriteResponse(conn, resp)
						writeMu.Unlock()
					}(req)
				}
			}(conn)
		}
	}()

	return addr, stop, nil
}

// TestClient_Send 测试发送请求
func TestClient_Send(t *testing.T) {
	// 启动 Echo 服务（回显）
	addr, stop, err := startMockServer(t, func(req *protocol.Request) *protocol.Response {
		// 回显请求参数
		return protocol.NewSuccessResponse(req.ID, req.Args)
	})
	if err != nil {
		t.Fatalf("启动服务失败: %v", err)
	}
	defer stop()

	time.Sleep(50 * time.Millisecond)

	// 创建客户端
	client := NewClient(addr, protocol.CodecTypeJSON, protocol.CompressTypeNone)

	// 连接
	if err := client.Dial(context.Background(), ""); err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer client.Close()

	// 创建请求
	req := protocol.NewRequest("TestService", "Echo", map[string]interface{}{
		"message": "Hello, RPC!",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.SendRequest(ctx, req)
	if err != nil {
		t.Fatalf("发送失败: %v", err)
	}

	// 验证
	if !resp.IsSuccess() {
		t.Errorf("应该成功: %v", resp.Error)
	}

	if resp.ID != req.ID {
		t.Error("响应 ID 不匹配")
	}

	t.Logf("✅ Send 测试通过，收到: %v", resp.Data)
}

// TestClient_Multiplex_Concurrent 验证单连接上多个 goroutine 并发发请求，
// 每个请求都能拿到正确的响应（RequestID 匹配）。
func TestClient_Multiplex_Concurrent(t *testing.T) {
	addr, stop, err := startMuxMockServer(t, func(req *protocol.Request) *protocol.Response {
		return protocol.NewSuccessResponse(req.ID, req.ID) // 把 RequestID 作为响应数据回传
	})
	if err != nil {
		t.Fatalf("启动服务失败: %v", err)
	}
	defer stop()
	time.Sleep(50 * time.Millisecond)

	client := NewClient(addr, protocol.CodecTypeJSON, protocol.CompressTypeNone)
	if err := client.Dial(context.Background(), ""); err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer client.Close()

	const n = 50
	errs := make(chan error, n)

	for i := range n {
		go func(i int) {
			req := protocol.NewRequest("Svc", fmt.Sprintf("M%d", i), nil)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			resp, err := client.SendRequest(ctx, req)
			if err != nil {
				errs <- fmt.Errorf("goroutine %d: %w", i, err)
				return
			}
			if resp.ID != req.ID {
				errs <- fmt.Errorf("goroutine %d: 响应 ID %d != 请求 ID %d", i, resp.ID, req.ID)
				return
			}
			errs <- nil
		}(i)
	}

	for range n {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}

	t.Logf("✅ 多路复用并发测试通过（%d 个并发请求）", n)
}

// TestClient_Multiplex_OutOfOrder 验证服务端乱序返回响应时，
// 客户端仍能按 RequestID 把响应正确路由回各自的调用方。
func TestClient_Multiplex_OutOfOrder(t *testing.T) {
	// 服务端对奇数 ID 的请求延迟响应，让偶数先返回，制造乱序
	addr, stop, err := startMuxMockServer(t, func(req *protocol.Request) *protocol.Response {
		if req.ID%2 == 1 {
			time.Sleep(50 * time.Millisecond)
		}
		return protocol.NewSuccessResponse(req.ID, req.ID)
	})
	if err != nil {
		t.Fatalf("启动服务失败: %v", err)
	}
	defer stop()
	time.Sleep(50 * time.Millisecond)

	client := NewClient(addr, protocol.CodecTypeJSON, protocol.CompressTypeNone)
	if err := client.Dial(context.Background(), ""); err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer client.Close()

	const n = 200
	var wg sync.WaitGroup
	wg.Add(n)
	failed := make(chan error, n)

	for range n {
		go func() {
			defer wg.Done()
			req := protocol.NewRequest("Svc", "M", nil)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			resp, err := client.SendRequest(ctx, req)
			if err != nil {
				failed <- fmt.Errorf("发送失败: %w，请求 ID %d", err, req.ID)
				return
			}
			if resp.ID != req.ID {
				failed <- fmt.Errorf("响应 ID %d != 请求 ID %d", resp.ID, req.ID)
			}
		}()
	}

	wg.Wait()
	close(failed)

	for err := range failed {
		t.Error(err)
	}

	t.Logf("✅ 乱序响应路由测试通过（%d 个请求，奇数 ID 延迟 50ms）", n)
}
