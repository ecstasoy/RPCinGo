package transport

import (
	"testing"
	"time"
)

// TestDefaultClientOptions 测试默认客户端配置
func TestDefaultClientOptions(t *testing.T) {
	opts := DefaultClientOptions()

	if opts.DialTimeout == 0 {
		t.Error("DialTimeout 应该有默认值")
	}

	if opts.ReadTimeout == 0 {
		t.Error("ReadTimeout 应该有默认值")
	}

	if !opts.KeepAlive {
		t.Error("KeepAlive 应该默认启用")
	}

	t.Logf("默认配置: DialTimeout=%v, ReadTimeout=%v, KeepAlive=%v",
		opts.DialTimeout, opts.ReadTimeout, opts.KeepAlive)
}

// TestClientOptions 测试客户端选项
func TestClientOptions(t *testing.T) {
	// 模拟构造函数应用选项
	opts := DefaultClientOptions()

	// 应用自定义选项
	options := []ClientOption{
		WithDialTimeout(3 * time.Second),
		WithReadTimeout(5 * time.Second),
		WithRetry(5, 200*time.Millisecond),
	}

	for _, opt := range options {
		opt(opts)
	}

	// 验证
	if opts.DialTimeout != 3*time.Second {
		t.Errorf("DialTimeout 应该是 3s, 实际 %v", opts.DialTimeout)
	}

	if opts.ReadTimeout != 5*time.Second {
		t.Errorf("ReadTimeout 应该是 5s, 实际 %v", opts.ReadTimeout)
	}

	if opts.MaxRetries != 5 {
		t.Errorf("MaxRetries 应该是 5, 实际 %d", opts.MaxRetries)
	}

	t.Log("✅ 选项配置测试通过")
}

// TestDefaultServerOptions 测试默认服务端配置
func TestDefaultServerOptions(t *testing.T) {
	opts := DefaultServerOptions()

	if opts.ReadTimeout == 0 {
		t.Error("ReadTimeout 应该有默认值")
	}

	if opts.WorkerPoolSize == 0 {
		t.Error("WorkerPoolSize 应该有默认值")
	}

	if opts.MaxRequestBodySize == 0 {
		t.Error("MaxRequestBodySize 应该有默认值")
	}

	t.Logf("默认配置: WorkerPool=%d, MaxBodySize=%d",
		opts.WorkerPoolSize, opts.MaxRequestBodySize)
}

// TestServerOptions 测试服务端选项
func TestServerOptions(t *testing.T) {
	opts := DefaultServerOptions()

	// 应用选项
	options := []ServerOption{
		WithWorkerPool(16),
		WithMaxConcurrentRequests(1000),
		WithMaxRequestBodySize(20 * 1024 * 1024),
	}

	for _, opt := range options {
		opt(opts)
	}

	// 验证
	if opts.WorkerPoolSize != 16 {
		t.Error("WorkerPoolSize 配置错误")
	}

	if opts.MaxConcurrentRequests != 1000 {
		t.Error("MaxConcurrentRequests 配置错误")
	}

	if opts.MaxRequestBodySize != 20*1024*1024 {
		t.Error("MaxRequestBodySize 配置错误")
	}

	t.Log("✅ 服务端选项配置测试通过")
}
