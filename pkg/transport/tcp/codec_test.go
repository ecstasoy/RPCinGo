package tcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"RPCinGo/pkg/protocol"
)

// TestProtocolCodec_EncodeDecodeRequest 测试请求编解码
func TestProtocolCodec_EncodeDecodeRequest(t *testing.T) {
	// 创建编解码器（JSON + 不压缩）
	pc := NewProtocolCodec(protocol.CodecTypeJSON, protocol.CompressTypeNone)

	// 1. 创建请求
	original := protocol.NewRequest("UserService", "GetUser", map[string]interface{}{
		"id": 123,
	})
	original.SetMetadata(protocol.MetaKeyTraceID, "trace-test")

	t.Logf("原始请求: %s", original)

	// 2. 编码
	encoded, err := pc.EncodeRequest(original)
	if err != nil {
		t.Fatalf("编码失败: %v", err)
	}

	t.Logf("编码后大小: %d bytes", len(encoded))

	// 验证：应该有 Header (20 bytes) + Body
	if len(encoded) < protocol.HeaderLength {
		t.Fatal("编码数据太短，应该包含 Header")
	}

	// 3. 解码（从 bytes.Reader 模拟网络）
	reader := bytes.NewReader(encoded)
	header, decoded, err := pc.ReadRequest(reader)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}

	t.Logf("解码的 Header: %s", header)
	t.Logf("解码的 Request: %s", decoded)

	// 4. 验证 Header
	if header.Magic != protocol.ProtocolMagic {
		t.Errorf("Magic 错误: 0x%X", header.Magic)
	}

	if header.MsgType != protocol.MsgTypeRequest {
		t.Errorf("MsgType 错误: %s", header.MsgType)
	}

	if header.RequestID != original.ID {
		t.Errorf("RequestID 不一致: %d != %d", header.RequestID, original.ID)
	}

	// 5. 验证 Request
	if decoded.Service != original.Service {
		t.Errorf("Service 不一致")
	}

	if decoded.Method != original.Method {
		t.Errorf("Method 不一致")
	}

	traceID, ok := decoded.GetMetadata(protocol.MetaKeyTraceID)
	if !ok || traceID != "trace-test" {
		t.Errorf("Metadata 不一致")
	}

	t.Log("✅ Request 编解码测试通过")
}

// TestProtocolCodec_EncodeDecodeResponse 测试响应编解码
func TestProtocolCodec_EncodeDecodeResponse(t *testing.T) {
	pc := NewProtocolCodec(protocol.CodecTypeJSON, protocol.CompressTypeNone)

	// 1. 创建响应
	original := protocol.NewSuccessResponse(456, map[string]interface{}{
		"name": "Alice",
		"age":  25,
	})

	// 2. 编码
	encoded, err := pc.EncodeResponse(original)
	if err != nil {
		t.Fatalf("编码失败: %v", err)
	}

	t.Logf("编码后大小: %d bytes", len(encoded))

	// 3. 解码
	reader := bytes.NewReader(encoded)
	header, decoded, err := pc.ReadResponse(reader)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}

	// 4. 验证 Header
	if header.MsgType != protocol.MsgTypeResponse {
		t.Error("MsgType 应该是 Response")
	}

	if header.RequestID != original.ID {
		t.Error("RequestID 不一致")
	}

	// 5. 验证 Response
	if decoded.ID != original.ID {
		t.Error("ID 不一致")
	}

	if !decoded.IsSuccess() {
		t.Error("应该是成功响应")
	}

	dataBytes, ok := decoded.Data.([]byte)
	if !ok {
		t.Fatalf("Data 应该是 []byte, got %T", decoded.Data)
	}

	var dataMap map[string]interface{}
	if err := json.Unmarshal(dataBytes, &dataMap); err != nil {
		t.Fatalf("unmarshal data failed: %v", err)
	}

	if dataMap["name"].(string) != "Alice" {
		t.Error("Data 不一致")
	}

	t.Log("✅ Response 编解码测试通过")
}

// TestProtocolCodec_WithCompression 测试带压缩的编解码
func TestProtocolCodec_WithCompression(t *testing.T) {
	// 使用 Gzip 压缩
	pc := NewProtocolCodec(protocol.CodecTypeJSON, protocol.CompressTypeGzip)

	// 创建大请求（可压缩）
	largeData := make(map[string]interface{})
	for i := 0; i < 100; i++ {
		largeData[fmt.Sprintf("key_%d", i)] = "This is a repeated value"
	}

	req := protocol.NewRequest("Service", "Method", largeData)

	// 编码（带压缩）
	compressed, err := pc.EncodeRequest(req)
	if err != nil {
		t.Fatalf("编码失败: %v", err)
	}

	// 对比：不压缩的大小
	pcNoCompress := NewProtocolCodec(protocol.CodecTypeJSON, protocol.CompressTypeNone)
	uncompressed, _ := pcNoCompress.EncodeRequest(req)

	t.Logf("不压缩: %d bytes", len(uncompressed))
	t.Logf("压缩后: %d bytes", len(compressed))

	ratio := float64(len(uncompressed)-len(compressed)) / float64(len(uncompressed)) * 100
	t.Logf("压缩率: %.2f%%", ratio)

	// 压缩后应该更小
	if len(compressed) >= len(uncompressed) {
		t.Error("压缩后应该更小")
	}

	// 解码
	reader := bytes.NewReader(compressed)
	header, decoded, err := pc.ReadRequest(reader)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}

	// 验证 Header 中的压缩标志
	if header.Compress != protocol.CompressTypeGzip {
		t.Errorf("Compress 应该是 Gzip, 实际: %s", header.Compress)
	}

	// 验证数据一致性
	if decoded.Service != req.Service {
		t.Error("数据不一致")
	}

	t.Log("✅ 压缩编解码测试通过")
}

// TestProtocolCodec_Protobuf 测试 Protobuf 编解码
func TestProtocolCodec_Protobuf(t *testing.T) {
	// 使用 Protobuf
	pc := NewProtocolCodec(protocol.CodecTypeProtobuf, protocol.CompressTypeNone)

	req := protocol.NewRequest("TestService", "TestMethod", map[string]interface{}{
		"id": 789,
	})

	// 编码
	encoded, err := pc.EncodeRequest(req)
	if err != nil {
		t.Fatalf("编码失败: %v", err)
	}

	// 解码
	reader := bytes.NewReader(encoded)
	header, decoded, err := pc.ReadRequest(reader)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}

	// 验证 Codec 类型
	if header.Codec != protocol.CodecTypeProtobuf {
		t.Errorf("Codec 类型错误: %s", header.Codec)
	}

	// 验证数据一致性
	if decoded.Service != req.Service {
		t.Error("Service 不一致")
	}

	t.Log("✅ Protobuf 编解码测试通过")
}

// TestProtocolCodec_ErrorResponse 测试错误响应
func TestProtocolCodec_ErrorResponse(t *testing.T) {
	pc := NewProtocolCodec(protocol.CodecTypeJSON, protocol.CompressTypeNone)

	// 创建错误响应
	err := protocol.NewError(protocol.ErrorCodeNotFound, "service not found")
	resp := protocol.NewErrorResponse(999, err)

	// 编码
	encoded, encErr := pc.EncodeResponse(resp)
	if encErr != nil {
		t.Fatalf("编码失败: %v", encErr)
	}

	// 解码
	reader := bytes.NewReader(encoded)
	_, decoded, decErr := pc.ReadResponse(reader)
	if decErr != nil {
		t.Fatalf("解码失败: %v", decErr)
	}

	// 验证
	if !decoded.IsError() {
		t.Error("应该是错误响应")
	}

	if decoded.Error.Code != protocol.ErrorCodeNotFound {
		t.Error("错误码不一致")
	}

	t.Log("✅ 错误响应编解码测试通过")
}
