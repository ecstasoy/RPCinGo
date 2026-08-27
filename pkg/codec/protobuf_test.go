package codec

import (
	"bytes"
	"encoding/binary"
	"testing"

	"RPCinGo/pkg/protocol"
)

// TestProtobufCodecName 测试名称
func TestProtobufCodecName(t *testing.T) {
	codec := NewProtobufCodec()

	name := codec.Name()
	if name != "protobuf" {
		t.Errorf("Name 错误: 期望 protobuf, 实际 %s", name)
	}

	t.Logf("编解码器名称: %s", name)
}

// TestProtobufCodecAutoRegister 测试自动注册
func TestProtobufCodecAutoRegister(t *testing.T) {
	// Protobuf 编解码器应该已自动注册
	codec := Get(protocol.CodecTypeProtobuf)
	if codec == nil {
		t.Fatal("Protobuf 编解码器应该已自动注册")
	}

	if codec.Name() != "protobuf" {
		t.Errorf("注册的编解码器名称错误: %s", codec.Name())
	}

	t.Log("Protobuf 编解码器自动注册成功")
}

// TestProtobufCodec_EncodeDecodeRequest 测试 Request 编解码
func TestProtobufCodec_EncodeDecodeRequest(t *testing.T) {
	codec := NewProtobufCodec()

	// 1. 创建原始 Request
	original := protocol.NewRequest("UserService", "GetUser", map[string]interface{}{
		"id":   123,
		"name": "Alice",
	})
	original.ServiceVersion = "v1.0.0"
	original.SetTimeout(5000)
	original.SetMetadata(protocol.MetaKeyTraceID, "trace-123")
	original.SetMetadata(protocol.MetaKeyUserID, "user-456")

	t.Logf("原始 Request: %s", original)

	// 2. 编码
	data, err := codec.Encode(original)
	if err != nil {
		t.Fatalf("编码失败: %v", err)
	}

	t.Logf("编码成功: %d bytes", len(data))

	// 3. 解码
	var decoded protocol.Request
	err = codec.Decode(data, &decoded)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}

	t.Logf("解码 Request: %s", &decoded)

	// 4. 验证基础字段
	if decoded.ID != original.ID {
		t.Errorf("ID 不一致: %d != %d", decoded.ID, original.ID)
	}

	if decoded.Service != original.Service {
		t.Errorf("Service 不一致: %s != %s", decoded.Service, original.Service)
	}

	if decoded.Method != original.Method {
		t.Errorf("Method 不一致: %s != %s", decoded.Method, original.Method)
	}

	if decoded.ServiceVersion != original.ServiceVersion {
		t.Errorf("ServiceVersion 不一致: %s != %s",
			decoded.ServiceVersion, original.ServiceVersion)
	}

	if decoded.Timeout != original.Timeout {
		t.Errorf("Timeout 不一致: %d != %d", decoded.Timeout, original.Timeout)
	}

	// 5. 验证 Metadata
	traceID, ok := decoded.GetMetadata(protocol.MetaKeyTraceID)
	if !ok || traceID != "trace-123" {
		t.Errorf("TraceID 不一致: %s", traceID)
	}

	userID, ok := decoded.GetMetadata(protocol.MetaKeyUserID)
	if !ok || userID != "user-456" {
		t.Errorf("UserID 不一致: %s", userID)
	}

	// 6. 验证 Args
	if decoded.Args == nil {
		t.Fatal("Args 不应该为 nil")
	}

	// 验证 ArgsCodec
	if decoded.ArgsCodec != protocol.PayloadCodecJSON {
		t.Errorf("ArgsCodec 应该是 JSON, 实际: %v", decoded.ArgsCodec)
	} else {
		t.Logf("✅ ArgsCodec 正确: JSON")
	}

	// Args 解码为 []byte
	argsBytes, ok := decoded.Args.([]byte)
	if !ok {
		t.Fatalf("Args 类型错误: %T (应该是 []byte)", decoded.Args)
	}

	t.Logf("Args bytes length: %d", len(argsBytes))
	t.Log("✅ Request 编解码完全正确")
}

// TestProtobufCodec_EncodeDecodeResponse 测试 Response 编解码
func TestProtobufCodec_EncodeDecodeResponse(t *testing.T) {
	codec := NewProtobufCodec()

	// 1. 创建成功响应
	original := protocol.NewSuccessResponse(123, map[string]interface{}{
		"name": "Alice",
		"age":  25,
	})
	original.ServerTime = 1000000
	original.SetMetadata(protocol.MetaKeyTraceID, "trace-456")

	t.Logf("原始 Response: %s", original)

	// 2. 编码
	data, err := codec.Encode(original)
	if err != nil {
		t.Fatalf("编码失败: %v", err)
	}

	t.Logf("编码成功: %d bytes", len(data))

	// 3. 解码
	var decoded protocol.Response
	err = codec.Decode(data, &decoded)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}

	t.Logf("解码 Response: %s", &decoded)

	// 4. 验证基础字段
	if decoded.ID != original.ID {
		t.Errorf("ID 不一致")
	}

	if decoded.ServerTime != original.ServerTime {
		t.Errorf("ServerTime 不一致")
	}

	// 5. 验证 Metadata
	traceID, ok := decoded.GetMetadata(protocol.MetaKeyTraceID)
	if !ok || traceID != "trace-456" {
		t.Errorf("TraceID 不一致")
	}

	// 6. 验证 Data
	if decoded.Data == nil {
		t.Fatal("Data 不应该为 nil")
	}

	// 验证 DataCodec
	if decoded.DataCodec != protocol.PayloadCodecJSON {
		t.Errorf("DataCodec 应该是 JSON, 实际: %v", decoded.DataCodec)
	} else {
		t.Logf("✅ DataCodec 正确: JSON")
	}

	// Data 解码为 []byte
	dataBytes, ok := decoded.Data.([]byte)
	if !ok {
		t.Fatalf("Data 类型错误: %T (应该是 []byte)", decoded.Data)
	}

	t.Logf("Data bytes length: %d", len(dataBytes))
	t.Log("✅ Response 编解码完全正确")
}

// TestProtobufCodec_ErrorResponse 测试错误响应编解码
func TestProtobufCodec_ErrorResponse(t *testing.T) {
	codec := NewProtobufCodec()

	// 1. 创建错误响应
	err := protocol.NewError(protocol.ErrorCodeNotFound, "service not found")
	err.Details = "UserService is not registered"

	original := protocol.NewErrorResponse(789, err)
	original.SetMetadata(protocol.MetaKeyTraceID, "trace-789")

	t.Logf("原始错误响应: %s", original)

	// 2. 编码
	data, encErr := codec.Encode(original)
	if encErr != nil {
		t.Fatalf("编码失败: %v", encErr)
	}

	// 3. 解码
	var decoded protocol.Response
	decErr := codec.Decode(data, &decoded)
	if decErr != nil {
		t.Fatalf("解码失败: %v", decErr)
	}

	// 4. 验证
	if !decoded.IsError() {
		t.Error("应该是错误响应")
	}

	if decoded.Error.Code != protocol.ErrorCodeNotFound {
		t.Errorf("错误码不一致: %d != %d",
			decoded.Error.Code, protocol.ErrorCodeNotFound)
	}

	if decoded.Error.Message != "service not found" {
		t.Errorf("错误消息不一致")
	}

	if decoded.Error.Details != "UserService is not registered" {
		t.Errorf("错误详情不一致")
	}

	t.Log("✅ 错误响应编解码完全正确")
}

// TestProtobufCodec_ComplexArgs 测试复杂参数
func TestProtobufCodec_ComplexArgs(t *testing.T) {
	codec := NewProtobufCodec()

	// 复杂的参数：嵌套结构
	complexArgs := map[string]interface{}{
		"user": map[string]interface{}{
			"id":   123,
			"name": "Alice",
			"tags": []interface{}{"admin", "user"},
		},
		"options": map[string]interface{}{
			"verbose": true,
			"limit":   10,
		},
	}

	req := protocol.NewRequest("Service", "Method", complexArgs)

	// 编码
	data, err := codec.Encode(req)
	if err != nil {
		t.Fatalf("编码失败: %v", err)
	}

	// 解码
	var decoded protocol.Request
	err = codec.Decode(data, &decoded)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}

	// 验证 ArgsCodec
	if decoded.ArgsCodec != protocol.PayloadCodecJSON {
		t.Errorf("ArgsCodec 应该是 JSON, 实际: %v", decoded.ArgsCodec)
	}

	// Args 解码为 []byte
	if _, ok := decoded.Args.([]byte); !ok {
		t.Errorf("Args 类型错误: %T (应该是 []byte)", decoded.Args)
	}

	t.Log("✅ 复杂参数编解码成功")
}

// TestProtobufCodec_NilFields 测试 nil 字段处理
func TestProtobufCodec_NilFields(t *testing.T) {
	codec := NewProtobufCodec()

	// Request 没有 Args 和 Metadata
	req := &protocol.Request{
		ID:       1,
		Service:  "Service",
		Method:   "Method",
		Args:     nil, // ← nil
		Metadata: nil, // ← nil
	}

	// 编码
	data, err := codec.Encode(req)
	if err != nil {
		t.Fatalf("编码失败: %v", err)
	}

	// 解码
	var decoded protocol.Request
	err = codec.Decode(data, &decoded)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}

	// 验证：nil 字段应该保持为 nil 或被初始化为空
	if decoded.Args != nil {
		// 可以接受被初始化为 nil
		t.Logf("Args 被初始化: %v", decoded.Args)
	}

	if decoded.Metadata == nil {
		t.Error("Metadata 应该被初始化")
	}

	t.Log("✅ nil 字段处理正确")
}

// BenchmarkProtobufEncode 性能测试：编码
func BenchmarkProtobufEncode(b *testing.B) {
	codec := NewProtobufCodec()
	req := protocol.NewRequest("Service", "Method", map[string]interface{}{
		"id": 123,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = codec.Encode(req)
	}
}

// BenchmarkProtobufDecode 性能测试：解码
func BenchmarkProtobufDecode(b *testing.B) {
	codec := NewProtobufCodec()
	req := protocol.NewRequest("Service", "Method", map[string]interface{}{
		"id": 123,
	})
	data, _ := codec.Encode(req)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var decoded protocol.Request
		_ = codec.Decode(data, &decoded)
	}
}

// BenchmarkProtobufVsJSON 对比测试
func BenchmarkProtobufVsJSON(b *testing.B) {
	req := protocol.NewRequest("Service", "Method", map[string]interface{}{
		"id":   123,
		"name": "test",
	})

	b.Run("Protobuf", func(b *testing.B) {
		codec := NewProtobufCodec()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			data, _ := codec.Encode(req)
			var decoded protocol.Request
			_ = codec.Decode(data, &decoded)
		}
	})

	b.Run("JSON", func(b *testing.B) {
		codec := NewJSONCodec()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			data, _ := codec.Encode(req)
			var decoded protocol.Request
			_ = codec.Decode(data, &decoded)
		}
	})
}

// ===== 流式编解码测试 =====

// TestProtobufStreamEncode 测试流式编码
func TestProtobufStreamEncode(t *testing.T) {
	codec := NewProtobufCodec()

	req := protocol.NewRequest("Service", "Method", map[string]interface{}{
		"id": 123,
	})

	// 使用 Buffer 模拟网络连接
	var buf bytes.Buffer

	// 流式编码
	err := codec.(StreamCodec).EncodeToWriter(&buf, req)
	if err != nil {
		t.Fatalf("流式编码失败: %v", err)
	}

	// 验证：应该有长度前缀 + 数据
	data := buf.Bytes()
	if len(data) < 4 {
		t.Fatal("数据太短，应该包含长度前缀")
	}

	// 解析长度前缀
	length := binary.BigEndian.Uint32(data[0:4])
	t.Logf("长度前缀: %d bytes", length)

	// 验证长度
	if int(length) != len(data)-4 {
		t.Errorf("长度不匹配: 前缀=%d, 实际=%d", length, len(data)-4)
	}

	t.Log("✅ 流式编码成功")
}

// TestProtobufStreamDecode 测试流式解码
func TestProtobufStreamDecode(t *testing.T) {
	codec := NewProtobufCodec()

	// 1. 先编码
	req := protocol.NewRequest("UserService", "GetUser", map[string]interface{}{
		"id": 456,
	})

	var buf bytes.Buffer
	err := codec.(StreamCodec).EncodeToWriter(&buf, req)
	if err != nil {
		t.Fatalf("编码失败: %v", err)
	}

	// 2. 流式解码
	var decoded protocol.Request
	err = codec.(StreamCodec).DecodeFromReader(&buf, &decoded)
	if err != nil {
		t.Fatalf("流式解码失败: %v", err)
	}

	// 3. 验证
	if decoded.Service != "UserService" {
		t.Error("Service 不一致")
	}

	if decoded.Method != "GetUser" {
		t.Error("Method 不一致")
	}

	t.Log("✅ 流式解码成功")
}

// TestProtobufStreamRoundTrip 测试流式往返
func TestProtobufStreamRoundTrip(t *testing.T) {
	codec := NewProtobufCodec()

	original := protocol.NewSuccessResponse(789, map[string]interface{}{
		"result": "success",
		"count":  100,
	})

	// 1. 流式编码
	var buf bytes.Buffer
	err := codec.(StreamCodec).EncodeToWriter(&buf, original)
	if err != nil {
		t.Fatalf("编码失败: %v", err)
	}

	t.Logf("编码字节数: %d", buf.Len())

	// 2. 流式解码
	var decoded protocol.Response
	err = codec.(StreamCodec).DecodeFromReader(&buf, &decoded)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}

	// 3. 验证
	if decoded.ID != original.ID {
		t.Error("ID 不一致")
	}

	if decoded.DataCodec != protocol.PayloadCodecJSON {
		t.Errorf("DataCodec 应该是 JSON, 实际: %v", decoded.DataCodec)
	}

	if _, ok := decoded.Data.([]byte); !ok {
		t.Errorf("Data 类型错误: %T (应该是 []byte)", decoded.Data)
	}

	t.Log("✅ 流式往返测试成功")
}

// BenchmarkProtobufStream 性能对比：普通 vs 流式
func BenchmarkProtobufStream(b *testing.B) {
	codec := NewProtobufCodec()
	req := protocol.NewRequest("Service", "Method", map[string]interface{}{"id": 123})

	b.Run("Normal", func(b *testing.B) {
		var buf bytes.Buffer
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf.Reset()
			data, _ := codec.Encode(req)
			buf.Write(data)
		}
	})

	b.Run("Stream", func(b *testing.B) {
		var buf bytes.Buffer
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf.Reset()
			_ = codec.(StreamCodec).EncodeToWriter(&buf, req)
		}
	})
}
