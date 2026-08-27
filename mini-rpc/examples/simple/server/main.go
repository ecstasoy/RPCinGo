package main

import (
	"fmt"
	"mini-rpc/server"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	// 1. 创建服务端
	srv := server.NewServer(":8080")

	// 2. 注册 Calculator 服务
	srv.Register("Calculator", func(method string, args []interface{}) (interface{}, error) {
		fmt.Printf("📝 处理请求: %s(%v)\n", method, args)

		switch method {
		case "Add":
			a := int(args[0].(float64))
			b := int(args[1].(float64))
			result := a + b
			fmt.Printf("✅ %d + %d = %d\n", a, b, result)
			return result, nil

		case "Sub":
			a := int(args[0].(float64))
			b := int(args[1].(float64))
			result := a - b
			fmt.Printf("✅ %d - %d = %d\n", a, b, result)
			return result, nil

		case "Multiply":
			a := int(args[0].(float64))
			b := int(args[1].(float64))
			result := a * b
			fmt.Printf("✅ %d * %d = %d\n", a, b, result)
			return result, nil

		case "Divide":
			a := int(args[0].(float64))
			b := int(args[1].(float64))
			if b == 0 {
				return nil, fmt.Errorf("除数不能为 0")
			}
			result := a / b
			fmt.Printf("✅ %d / %d = %d\n", a, b, result)
			return result, nil

		default:
			return nil, fmt.Errorf("未知方法: %s", method)
		}
	})

	// 3. 注册 Greeter 服务
	srv.Register("Greeter", func(method string, args []interface{}) (interface{}, error) {
		fmt.Printf("📝 处理请求: %s(%v)\n", method, args)

		switch method {
		case "SayHello":
			name := args[0].(string)
			result := fmt.Sprintf("Hello, %s!", name)
			fmt.Printf("✅ 返回: %s\n", result)
			return result, nil

		default:
			return nil, fmt.Errorf("未知方法: %s", method)
		}
	})

	fmt.Println("🚀 RPC 服务端启动...")
	fmt.Println("📍 监听地址: :8080")
	fmt.Println("📦 已注册服务:")
	fmt.Println("   - Calculator (Add, Sub, Multiply, Divide)")
	fmt.Println("   - Greeter (SayHello)")
	fmt.Println("按 Ctrl+C 停止服务")
	fmt.Println(strings.Repeat("-", 50))

	// 4. 优雅关闭
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 在 goroutine 中启动服务
	errChan := make(chan error, 1)
	go func() {
		errChan <- srv.Start()
	}()

	// 等待信号或错误
	select {
	case err := <-errChan:
		if err != nil {
			fmt.Printf("❌ 服务端错误: %v\n", err)
		}
	case sig := <-sigChan:
		fmt.Printf("\n📢 收到信号: %v\n", sig)
		fmt.Println("🛑 正在关闭服务端...")
		srv.Stop()
		fmt.Println("✅ 服务端已关闭")
	}
}
