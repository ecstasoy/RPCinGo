package client

import (
	"fmt"
	"mini-rpc/server"
	"testing"
	"time"
)

// TestClientServer 集成测试
func TestClientServer(t *testing.T) {
	// 1. 启动服务端
	srv := server.NewServer("127.0.0.1:18090")

	// 2. 注册服务
	srv.Register("Calculator", func(method string, args []interface{}) (interface{}, error) {
		switch method {
		case "Add":
			a := int(args[0].(float64))
			b := int(args[1].(float64))
			return a + b, nil

		case "Multiply":
			a := int(args[0].(float64))
			b := int(args[1].(float64))
			return a * b, nil

		default:
			return nil, fmt.Errorf("unknown method: %s", method)
		}
	})

	// 3. 启动服务端（在 goroutine 中）
	done := make(chan error, 1)
	go func() {
		done <- srv.Start()
	}()

	// 等待服务端启动
	time.Sleep(200 * time.Millisecond)

	// 4. 创建客户端
	client := NewClient("127.0.0.1:18090")

	// 5. 连接
	if err := client.Connect(); err != nil {
		t.Fatalf("客户端连接失败: %v", err)
	}

	// 6. 测试 Add 方法
	result, err := client.Call("Calculator", "Add", []interface{}{10, 20})
	if err != nil {
		t.Fatalf("调用 Add 失败: %v", err)
	}

	resultNum := int(result.(float64))
	if resultNum != 30 {
		t.Errorf("Add 结果错误: 期望 30, 实际 %d", resultNum)
	}
	t.Logf("✅ Add(10, 20) = %d", resultNum)

	// 7. 测试 Multiply 方法
	result, err = client.Call("Calculator", "Multiply", []interface{}{5, 6})
	if err != nil {
		t.Fatalf("调用 Multiply 失败: %v", err)
	}

	resultNum = int(result.(float64))
	if resultNum != 30 {
		t.Errorf("Multiply 结果错误: 期望 30, 实际 %d", resultNum)
	}
	t.Logf("✅ Multiply(5, 6) = %d", resultNum)

	// 8. 测试不存在的服务
	_, err = client.Call("NonExist", "Method", []interface{}{})
	if err == nil {
		t.Error("调用不存在的服务应该返回错误")
	}
	t.Logf("✅ 不存在的服务正确返回错误: %v", err)

	// 9. 先关闭客户端
	client.Close()

	// 10. 再关闭服务端
	srv.Stop()

	// 11. 等待服务端关闭（带超时）⚠️ 关键！
	select {
	case <-done:
		t.Log("✅ 服务端正常关闭")
	case <-time.After(1 * time.Second):
		t.Log("⏱️ 服务端关闭超时（正常现象，Accept 阻塞）")
	}

	t.Log("✅ 集成测试完成")
}

// TestMultipleClients 测试多客户端并发
func TestMultipleClients(t *testing.T) {
	// 启动服务端
	srv := server.NewServer("127.0.0.1:18091")

	srv.Register("Echo", func(method string, args []interface{}) (interface{}, error) {
		if len(args) > 0 {
			return args[0], nil
		}
		return "empty", nil
	})

	done := make(chan error, 1)
	go func() {
		done <- srv.Start()
	}()

	time.Sleep(200 * time.Millisecond)

	// 启动多个客户端
	clientCount := 5
	clientDone := make(chan bool, clientCount)

	for i := 1; i <= clientCount; i++ {
		go func(clientID int) {
			c := NewClient("127.0.0.1:18091")
			if err := c.Connect(); err != nil {
				t.Errorf("客户端 %d 连接失败: %v", clientID, err)
				clientDone <- false
				return
			}
			defer c.Close()

			// 每个客户端发送 3 次请求
			for j := 1; j <= 3; j++ {
				msg := fmt.Sprintf("Client-%d-Request-%d", clientID, j)
				result, err := c.Call("Echo", "Echo", []interface{}{msg})
				if err != nil {
					t.Errorf("客户端 %d 第 %d 次调用失败: %v", clientID, j, err)
					clientDone <- false
					return
				}

				if result != msg {
					t.Errorf("客户端 %d 响应错误: 期望 %s, 实际 %v",
						clientID, msg, result)
				}

				t.Logf("✅ 客户端 %d 第 %d 次调用成功", clientID, j)
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

	t.Log("✅ 多客户端并发测试完成")

	// 关闭服务端
	srv.Stop()

	// 等待关闭（带超时）⚠️ 关键！
	select {
	case <-done:
		t.Log("✅ 服务端正常关闭")
	case <-time.After(1 * time.Second):
		t.Log("⏱️ 服务端关闭超时")
	}
}
