package transport

import (
	"fmt"
	"testing"
	"time"
)

// TestTCPTransport 测试 TCP 传输
func TestTCPTransport(t *testing.T) {
	server := NewTCPServerTransport()
	addr := "127.0.0.1:18080"

	if err := server.Listen(addr); err != nil {
		t.Fatalf("服务端监听失败: %v", err)
	}

	// 启动服务，用 channel 接收结果
	done := make(chan error, 1)
	go func() {
		handler := func(data []byte) []byte {
			return []byte(fmt.Sprintf("Echo: %s", string(data)))
		}
		done <- server.Serve(handler)
	}()

	time.Sleep(100 * time.Millisecond)

	// 创建客户端
	client := NewTCPClientTransport(addr)
	if err := client.Connect(); err != nil {
		t.Fatalf("客户端连接失败: %v", err)
	}

	// 发送请求
	request := []byte("Hello, RPC!")
	response, err := client.Send(request)
	if err != nil {
		t.Fatalf("发送请求失败: %v", err)
	}

	// 验证响应
	expected := "Echo: Hello, RPC!"
	if string(response) != expected {
		t.Errorf("响应不符合预期\n期望: %s\n实际: %s", expected, string(response))
	}

	t.Logf("测试成功！响应: %s", string(response))

	// 清理资源
	client.Close()
	server.Close()

	// 等待服务端关闭（带超时）
	select {
	case <-done:
		t.Log("服务端已关闭")
	case <-time.After(1 * time.Second):
		t.Log("服务端关闭超时（正常现象）")
	}
}

// TestTCPTransport_MultipleRequests 测试多次请求
func TestTCPTransport_MultipleRequests(t *testing.T) {
	server := NewTCPServerTransport()
	addr := "127.0.0.1:18081"

	if err := server.Listen(addr); err != nil {
		t.Fatalf("监听失败: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- server.Serve(func(data []byte) []byte {
			return []byte(fmt.Sprintf("Response-%s", string(data)))
		})
	}()

	time.Sleep(100 * time.Millisecond)

	// 创建客户端
	client := NewTCPClientTransport(addr)
	if err := client.Connect(); err != nil {
		t.Fatalf("连接失败: %v", err)
	}

	// 发送多次请求
	for i := 1; i <= 5; i++ {
		request := []byte(fmt.Sprintf("Request-%d", i))
		response, err := client.Send(request)
		if err != nil {
			t.Fatalf("第 %d 次请求失败: %v", i, err)
		}

		expected := fmt.Sprintf("Response-Request-%d", i)
		if string(response) != expected {
			t.Errorf("第 %d 次响应错误\n期望: %s\n实际: %s",
				i, expected, string(response))
		}

		t.Logf("第 %d 次请求成功: %s", i, string(response))
	}

	// 清理
	client.Close()
	server.Close()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Log("服务端关闭超时")
	}
}

// TestTCPTransport_MultipleClients 测试多个客户端
func TestTCPTransport_MultipleClients(t *testing.T) {
	server := NewTCPServerTransport()
	addr := "127.0.0.1:18082"

	if err := server.Listen(addr); err != nil {
		t.Fatalf("监听失败: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- server.Serve(func(data []byte) []byte {
			return data
		})
	}()

	time.Sleep(100 * time.Millisecond)

	// 启动多个客户端
	clientCount := 3
	clientDone := make(chan bool, clientCount)

	for i := 1; i <= clientCount; i++ {
		go func(clientID int) {
			client := NewTCPClientTransport(addr)
			if err := client.Connect(); err != nil {
				t.Errorf("客户端 %d 连接失败: %v", clientID, err)
				clientDone <- false
				return
			}
			defer client.Close()

			// 每个客户端发送 3 次请求
			for j := 1; j <= 3; j++ {
				msg := fmt.Sprintf("Client-%d-Request-%d", clientID, j)
				resp, err := client.Send([]byte(msg))
				if err != nil {
					t.Errorf("客户端 %d 第 %d 次请求失败: %v", clientID, j, err)
					clientDone <- false
					return
				}

				if string(resp) != msg {
					t.Errorf("客户端 %d 响应错误: %s", clientID, string(resp))
				}

				t.Logf("客户端 %d 第 %d 次请求成功", clientID, j)
			}

			clientDone <- true
		}(i)
	}

	// 等待所有客户端完成
	for i := 0; i < clientCount; i++ {
		success := <-clientDone
		if !success {
			t.Fatal("有客户端测试失败")
		}
	}

	t.Log("多客户端测试成功！")

	// 清理
	server.Close()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Log("服务端关闭超时")
	}
}
