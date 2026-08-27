package protocol

import (
	"testing"
	"time"
)

// TestNewRequest 测试创建请求
func TestNewRequest(t *testing.T) {
	req := NewRequest("UserService", "GetUser", map[string]interface{}{"id": 123})

	// 验证基础字段
	if req.Service != "UserService" {
		t.Errorf("Service 错误")
	}

	if req.Method != "GetUser" {
		t.Errorf("Method 错误")
	}

	// ID is owned by the transport multiplexer, not NewRequest. A freshly
	// constructed request carries ID 0 until it is sent.
	if req.ID != 0 {
		t.Errorf("ID 应在发送前为 0（由传输层分配），实际 %d", req.ID)
	}

	if req.CreatedAt == 0 {
		t.Error("CreatedAt 应该自动设置")
	}

	if req.Metadata == nil {
		t.Error("Metadata 应该初始化")
	}

	t.Logf("创建请求成功: %s", req)
}

// TestRequestTimeout 测试超时设置
func TestRequestTimeout(t *testing.T) {
	req := NewRequest("Service", "Method", nil)

	// 设置超时
	req.SetTimeout(3 * time.Second)

	// 验证
	if req.Timeout != 3000 {
		t.Errorf("Timeout 应该是 3000ms, 实际 %d", req.Timeout)
	}

	// 获取超时
	timeout := req.GetTimeout()
	if timeout != 3*time.Second {
		t.Errorf("GetTimeout 错误")
	}

	t.Log("超时设置成功")
}

// TestRequestMetadata 测试元数据
func TestRequestMetadata(t *testing.T) {
	req := NewRequest("Service", "Method", nil)

	// 设置元数据
	req.SetMetadata(MetaKeyTraceID, "trace-123")
	req.SetMetadata(MetaKeyUserID, "user-456")

	// 获取元数据
	traceID, ok := req.GetMetadata(MetaKeyTraceID)
	if !ok || traceID != "trace-123" {
		t.Error("元数据设置/获取失败")
	}

	t.Log("元数据操作成功")
}

// TestNewResponse 测试创建响应
func TestNewResponse(t *testing.T) {
	// 成功响应
	resp := NewSuccessResponse(123, map[string]interface{}{"name": "Alice"})

	if resp.ID != 123 {
		t.Error("ID 不匹配")
	}

	if !resp.IsSuccess() {
		t.Error("应该是成功响应")
	}

	if resp.IsError() {
		t.Error("不应该有错误")
	}

	t.Logf("成功响应: %s", resp)
}

// TestErrorResponse 测试错误响应
func TestErrorResponse(t *testing.T) {
	// 错误响应
	err := NewError(ErrorCodeNotFound, "service not found")
	resp := NewErrorResponse(456, err)

	if resp.ID != 456 {
		t.Error("ID 不匹配")
	}

	if resp.IsSuccess() {
		t.Error("不应该是成功响应")
	}

	if !resp.IsError() {
		t.Error("应该有错误")
	}

	if resp.Error.Code != ErrorCodeNotFound {
		t.Error("错误码不匹配")
	}

	t.Logf("错误响应: %s", resp)
}

// TestNewRequestDoesNotAssignID documents the ownership contract: NewRequest no
// longer assigns a competing global ID. The multiplexing ID is assigned by the
// transport at send time, so two freshly constructed requests both carry 0.
// Per-connection ID uniqueness and response routing are covered at the transport
// layer (see pkg/transport/tcp/client_test.go).
func TestNewRequestDoesNotAssignID(t *testing.T) {
	req1 := NewRequest("S1", "M1", nil)
	req2 := NewRequest("S2", "M2", nil)

	if req1.ID != 0 || req2.ID != 0 {
		t.Errorf("NewRequest 不应分配 ID，实际 req1.ID=%d req2.ID=%d", req1.ID, req2.ID)
	}
}

// TestMetadataClone 测试元数据克隆
func TestMetadataClone(t *testing.T) {
	meta := NewMetadata()
	meta.Set("key1", "value1")
	meta.Set("key2", "value2")

	// 克隆
	clone := meta.Clone()

	// 修改克隆
	clone.Set("key1", "modified")

	// 验证原始未变
	val, _ := meta.Get("key1")
	if val != "value1" {
		t.Error("原始元数据被修改了")
	}

	// 验证克隆已变
	val, _ = clone.Get("key1")
	if val != "modified" {
		t.Error("克隆修改失败")
	}

	t.Log("元数据克隆成功")
}
