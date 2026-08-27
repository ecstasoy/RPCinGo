package main

import (
	"fmt"
	"mini-rpc/client"
	"os"
	"strings"
)

func main() {
	// 1. 创建客户端
	c := client.NewClient("localhost:8080")

	fmt.Println("🚀 RPC 客户端启动...")

	// 2. 连接到服务端
	if err := c.Connect(); err != nil {
		fmt.Printf("❌ 连接失败: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	fmt.Println("✅ 已连接到服务端: localhost:8080")
	fmt.Println(strings.Repeat("-", 50))

	// 3. 测试 Calculator 服务
	fmt.Println("\n📊 测试 Calculator 服务:")

	// Add
	result, err := c.Call("Calculator", "Add", []interface{}{10, 20})
	if err != nil {
		fmt.Printf("❌ Add 调用失败: %v\n", err)
	} else {
		fmt.Printf("✅ 10 + 20 = %.0f\n", result.(float64))
	}

	// Sub
	result, err = c.Call("Calculator", "Sub", []interface{}{100, 35})
	if err != nil {
		fmt.Printf("❌ Sub 调用失败: %v\n", err)
	} else {
		fmt.Printf("✅ 100 - 35 = %.0f\n", result.(float64))
	}

	// Multiply
	result, err = c.Call("Calculator", "Multiply", []interface{}{7, 8})
	if err != nil {
		fmt.Printf("❌ Multiply 调用失败: %v\n", err)
	} else {
		fmt.Printf("✅ 7 * 8 = %.0f\n", result.(float64))
	}

	// Divide
	result, err = c.Call("Calculator", "Divide", []interface{}{100, 4})
	if err != nil {
		fmt.Printf("❌ Divide 调用失败: %v\n", err)
	} else {
		fmt.Printf("✅ 100 / 4 = %.0f\n", result.(float64))
	}

	// Divide by zero (测试错误处理)
	_, err = c.Call("Calculator", "Divide", []interface{}{10, 0})
	if err != nil {
		fmt.Printf("✅ 除以 0 正确返回错误: %v\n", err)
	}

	// 4. 测试 Greeter 服务
	fmt.Println("\n👋 测试 Greeter 服务:")

	result, err = c.Call("Greeter", "SayHello", []interface{}{"Alice"})
	if err != nil {
		fmt.Printf("❌ SayHello 调用失败: %v\n", err)
	} else {
		fmt.Printf("✅ %s\n", result.(string))
	}

	result, err = c.Call("Greeter", "SayHello", []interface{}{"Bob"})
	if err != nil {
		fmt.Printf("❌ SayHello 调用失败: %v\n", err)
	} else {
		fmt.Printf("✅ %s\n", result.(string))
	}

	// 5. 测试不存在的服务
	fmt.Println("\n🧪 测试错误处理:")

	_, err = c.Call("NonExistService", "Method", []interface{}{})
	if err != nil {
		fmt.Printf("✅ 不存在的服务正确返回错误: %v\n", err)
	}

	fmt.Println("\n✅ 所有测试完成!")
}
