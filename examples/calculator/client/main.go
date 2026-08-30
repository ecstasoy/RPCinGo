package main

import (
	"github.com/ecstasoy/RPCinGo/pkg/interceptor"
	"github.com/ecstasoy/RPCinGo/pkg/tracing"
	"bufio"
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ecstasoy/RPCinGo/examples/proto/calculator"
	"github.com/ecstasoy/RPCinGo/pkg/client"
)

func main() {
	shutdown, err := tracing.InitTracerProvider("http://localhost:14268/api/traces", "calculator-client")
	if err != nil {
		fmt.Printf("init tracer failed: %v\n", err)
		os.Exit(1)
	}
	defer shutdown(context.Background())

	cli, err := client.NewClient("127.0.0.1:8080")
	if err != nil {
		fmt.Printf("Create client failed: %v\n", err)
		os.Exit(1)
	}
	cli.Use(interceptor.TracingClient(), interceptor.Logging(nil))
	defer cli.Close()

	fmt.Println("=== RPCinGo Calculator ===")
	fmt.Println("支持运算: add, sub, mul, div")
	fmt.Println("输入格式: <运算> <a> <b>，例如: add 10 20")
	fmt.Println("随机模式: rand <次数>，例如: rand 100")
	fmt.Println("输入 exit 退出")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "exit" {
			break
		}

		parts := strings.Fields(line)

		// 随机模式
		if len(parts) == 2 && parts[0] == "rand" {
			n, err := strconv.Atoi(parts[1])
			if err != nil || n <= 0 {
				fmt.Println("格式错误，示例: rand 100")
				continue
			}
			runRandom(cli, n)
			fmt.Println()
			continue
		}

		if len(parts) != 3 {
			fmt.Println("格式错误，示例: add 10 20")
			continue
		}

		op := parts[0]
		a, err1 := strconv.ParseInt(parts[1], 10, 32)
		b, err2 := strconv.ParseInt(parts[2], 10, 32)
		if err1 != nil || err2 != nil {
			fmt.Println("参数必须是整数")
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		switch op {
		case "add":
			req := &calculator.AddRequest{A: int32(a), B: int32(b)}
			resp := &calculator.AddResponse{}
			_, err := cli.CallTyped(ctx, "Calculator", "Add", req, resp)
			if err != nil {
				fmt.Printf("错误: %v\n", err)
			} else {
				fmt.Printf("%d + %d = %d\n", a, b, resp.Result)
			}
		case "sub":
			req := &calculator.SubtractRequest{A: int32(a), B: int32(b)}
			resp := &calculator.SubtractResponse{}
			_, err := cli.CallTyped(ctx, "Calculator", "Subtract", req, resp)
			if err != nil {
				fmt.Printf("错误: %v\n", err)
			} else {
				fmt.Printf("%d - %d = %d\n", a, b, resp.Result)
			}
		case "mul":
			req := &calculator.MultiplyRequest{A: int32(a), B: int32(b)}
			resp := &calculator.MultiplyResponse{}
			_, err := cli.CallTyped(ctx, "Calculator", "Multiply", req, resp)
			if err != nil {
				fmt.Printf("错误: %v\n", err)
			} else {
				fmt.Printf("%d * %d = %d\n", a, b, resp.Result)
			}
		case "div":
			if b == 0 {
				fmt.Println("错误: 除数不能为 0")
				cancel()
				continue
			}
			req := &calculator.DivideRequest{A: int32(a), B: int32(b)}
			resp := &calculator.DivideResponse{}
			_, err := cli.CallTyped(ctx, "Calculator", "Divide", req, resp)
			if err != nil {
				fmt.Printf("错误: %v\n", err)
			} else if resp.Remainder != 0 {
				fmt.Printf("%d / %d = %d 余 %d\n", a, b, resp.Result, resp.Remainder)
			} else {
				fmt.Printf("%d / %d = %d\n", a, b, resp.Result)
			}
		default:
			fmt.Printf("不支持的运算: %s（支持 add, sub, mul, div）\n", op)
		}

		cancel()
		fmt.Println()
	}

	fmt.Println("退出")
}

func runRandom(cli *client.Client, n int) {
	ops := []string{"add", "sub", "mul", "div"}
	success, failed := 0, 0

	for i := 0; i < n; i++ {
		op := ops[rand.Intn(len(ops))]
		a := rand.Int31n(1000) + 1
		b := rand.Int31n(1000) + 1

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		var err error
		switch op {
		case "add":
			resp := &calculator.AddResponse{}
			_, err = cli.CallTyped(ctx, "Calculator", "Add", &calculator.AddRequest{A: a, B: b}, resp)
			if err == nil {
				fmt.Printf("[%d] %d + %d = %d\n", i+1, a, b, resp.Result)
			}
		case "sub":
			resp := &calculator.SubtractResponse{}
			_, err = cli.CallTyped(ctx, "Calculator", "Subtract", &calculator.SubtractRequest{A: a, B: b}, resp)
			if err == nil {
				fmt.Printf("[%d] %d - %d = %d\n", i+1, a, b, resp.Result)
			}
		case "mul":
			resp := &calculator.MultiplyResponse{}
			_, err = cli.CallTyped(ctx, "Calculator", "Multiply", &calculator.MultiplyRequest{A: a, B: b}, resp)
			if err == nil {
				fmt.Printf("[%d] %d * %d = %d\n", i+1, a, b, resp.Result)
			}
		case "div":
			resp := &calculator.DivideResponse{}
			_, err = cli.CallTyped(ctx, "Calculator", "Divide", &calculator.DivideRequest{A: a, B: b}, resp)
			if err == nil && resp.Remainder != 0 {
				fmt.Printf("[%d] %d / %d = %d 余 %d\n", i+1, a, b, resp.Result, resp.Remainder)
			} else if err == nil {
				fmt.Printf("[%d] %d / %d = %d\n", i+1, a, b, resp.Result)
			}
		}

		cancel()
		if err != nil {
			fmt.Printf("[%d] %s 错误: %v\n", i+1, op, err)
			failed++
		} else {
			success++
		}
	}

	fmt.Printf("\n完成 %d 次计算：成功 %d，失败 %d\n", n, success, failed)
}
