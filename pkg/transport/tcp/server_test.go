package tcp

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ecstasoy/RPCinGo/pkg/protocol"
	"github.com/ecstasoy/RPCinGo/pkg/transport"
)

// TestServer_ListenAndClose 测试监听和关闭
func TestServer_ListenAndClose(t *testing.T) {
	server := NewServer(protocol.CodecTypeJSON, protocol.CompressTypeNone)

	ctx := context.Background()

	// 监听
	if err := server.Listen(ctx, "127.0.0.1:0"); err != nil {
		t.Fatalf("监听失败: %v", err)
	}

	// 检查地址
	addr := server.Addr()
	if addr == nil {
		t.Fatal("Addr 不应该为 nil")
	}

	t.Logf("监听地址: %s", addr)

	// 关闭
	if err := server.Close(); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}

	t.Log("✅ 监听和关闭测试通过")
}

// TestServer_ServeEcho 测试 Echo 服务
func TestServer_ServeEcho(t *testing.T) {
	// 创建服务端
	server := NewServer(protocol.CodecTypeJSON, protocol.CompressTypeNone)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 监听
	if err := server.Listen(ctx, "127.0.0.1:0"); err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	defer server.Close()

	addr := server.Addr().String()
	t.Logf("服务端地址: %s", addr)

	// Echo handler: 直接返回收到的请求数据
	handler := func(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
		return protocol.NewSuccessResponse(req.ID, req.Args), nil
	}

	// 启动服务（在 goroutine 中）
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.Serve(ctx, handler)
	}()

	// 等待服务启动
	time.Sleep(100 * time.Millisecond)

	// 创建客户端
	client := NewClient(addr, protocol.CodecTypeJSON, protocol.CompressTypeNone)

	if err := client.Dial(context.Background(), ""); err != nil {
		t.Fatalf("客户端连接失败: %v", err)
	}
	defer client.Close()

	// 创建请求
	req := protocol.NewRequest("EchoService", "Echo", map[string]interface{}{
		"message": "Hello, Server!",
	})

	sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sendCancel()

	resp, err := client.SendRequest(sendCtx, req)
	if err != nil {
		t.Fatalf("发送失败: %v", err)
	}

	// 验证
	if !resp.IsSuccess() {
		t.Errorf("应该成功: %v", resp.Error)
	}

	t.Logf("✅ Echo 测试通过，收到: %v", resp.Data)

	// 停止服务
	cancel()

	select {
	case <-serverDone:
	case <-time.After(2 * time.Second):
		t.Log("服务端停止超时")
	}
}

// TestServer_Concurrent 测试并发请求
func TestServer_Concurrent(t *testing.T) {
	// 创建服务端
	server := NewServer(protocol.CodecTypeJSON, protocol.CompressTypeNone,
		transport.WithMaxConcurrentRequests(100),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.Listen(ctx, "127.0.0.1:0"); err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	defer server.Close()

	addr := server.Addr().String()

	// Calculator handler
	handler := func(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
		// 处理 Add 方法
		if req.Method == "Add" {
			argsBytes, ok := req.Args.([]byte)
			if !ok {
				return protocol.NewErrorResponse(req.ID,
					protocol.NewError(protocol.ErrorCodeInvalidArgument, "args is not bytes")), nil
			}

			var argsMap map[string]interface{}
			if err := server.codec.codec.Decode(argsBytes, &argsMap); err != nil {
				return protocol.NewErrorResponse(req.ID,
					protocol.NewError(protocol.ErrorCodeInvalidArgument, "decode args failed")), nil
			}

			a := int(argsMap["a"].(float64))
			b := int(argsMap["b"].(float64))
			result := a + b

			return protocol.NewSuccessResponse(req.ID, result), nil
		}

		return protocol.NewErrorResponse(req.ID,
			protocol.NewError(protocol.ErrorCodeNotFound, fmt.Sprintf("unknown method: %s", req.Method))), nil
	}

	// 启动服务
	go server.Serve(ctx, handler)
	time.Sleep(100 * time.Millisecond)

	// 启动多个客户端并发请求
	clientCount := 10
	requestsPerClient := 5

	done := make(chan bool, clientCount)

	for i := 0; i < clientCount; i++ {
		go func(clientID int) {
			// 创建客户端
			client := NewClient(addr, protocol.CodecTypeJSON, protocol.CompressTypeNone)
			if err := client.Dial(context.Background(), ""); err != nil {
				t.Errorf("客户端 %d 连接失败: %v", clientID, err)
				done <- false
				return
			}
			defer client.Close()

			// 发送多个请求
			for j := 0; j < requestsPerClient; j++ {
				req := protocol.NewRequest("Calculator", "Add", map[string]interface{}{
					"a": clientID,
					"b": j,
				})

				sendCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				resp, err := client.SendRequest(sendCtx, req)
				cancel()

				if err != nil {
					t.Errorf("客户端 %d 请求 %d 失败: %v", clientID, j, err)
					done <- false
					return
				}

				if resp.IsError() {
					t.Errorf("客户端 %d 请求 %d 返回错误: %v", clientID, j, resp.Error)
					done <- false
					return
				}

				resultBytes, ok := resp.Data.([]byte)
				if !ok {
					t.Errorf("客户端 %d 请求 %d 响应数据类型错误: %T", clientID, j, resp.Data)
					done <- false
					return
				}

				var result interface{}
				if err := server.codec.codec.Decode(resultBytes, &result); err != nil {
					t.Errorf("客户端 %d 请求 %d 解码结果失败: %v", clientID, j, err)
					done <- false
					return
				}

				resultInt := int(result.(float64))
				expected := clientID + j
				if resultInt != expected {
					t.Errorf("客户端 %d 请求 %d 结果错误: 期望 %d, 实际 %d",
						clientID, j, expected, resultInt)
				}
			}

			done <- true
		}(i)
	}

	// 等待所有客户端完成
	for i := 0; i < clientCount; i++ {
		success := <-done
		if !success {
			t.Fatal("有客户端测试失败")
		}
	}

	// 检查统计
	stats := server.Stats()
	t.Logf("✅ 并发测试通过")
	t.Logf("总连接数: %d", stats.TotalConnections)
	t.Logf("当前活跃: %d", stats.ActiveConnections)
}

// TestServer_Timeout 测试超时控制
func TestServer_Timeout(t *testing.T) {
	server := NewServer(protocol.CodecTypeJSON, protocol.CompressTypeNone,
		transport.WithServerTimeout(1*time.Second, 1*time.Second),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.Listen(ctx, "127.0.0.1:0"); err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	defer server.Close()

	addr := server.Addr().String()

	// 慢 handler（模拟超时）
	handler := func(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
		// 等待 2 秒（超过服务端超时）
		time.Sleep(2 * time.Second)
		return protocol.NewSuccessResponse(req.ID, req.Args), nil
	}

	go server.Serve(ctx, handler)
	time.Sleep(100 * time.Millisecond)

	// 客户端连接
	client := NewClient(addr, protocol.CodecTypeJSON, protocol.CompressTypeNone,
		transport.WithReadTimeout(500*time.Millisecond), // 客户端也设短超时
	)

	if err := client.Dial(context.Background(), ""); err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer client.Close()

	// 发送请求
	req := protocol.NewRequest("Service", "SlowMethod", nil)

	sendCtx, sendCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer sendCancel()

	_, err := client.SendRequest(sendCtx, req)

	// 应该超时
	if err == nil {
		t.Error("应该超时")
	}

	t.Logf("✅ 超时测试通过: %v", err)
}
