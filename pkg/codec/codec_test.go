package codec

import (
	"compress/gzip"
	"fmt"
	"strings"
	"testing"

	"RPCinGo/pkg/protocol"
)

// TestRegisterAndGet 测试注册和获取
func TestRegisterAndGet(t *testing.T) {
	// JSON 应该已自动注册
	codec := Get(protocol.CodecTypeJSON)
	if codec == nil {
		t.Fatal("JSON 编解码器应该已注册")
	}

	if codec.Name() != "json" {
		t.Errorf("编解码器名称错误: %s", codec.Name())
	}

	t.Logf("获取到编解码器: %s", codec.Name())
}

// TestGetOrDefault 测试默认值获取
func TestGetOrDefault(t *testing.T) {
	// 获取存在的
	codec := GetOrDefault(protocol.CodecTypeJSON)
	if codec == nil {
		t.Fatal("应该返回 JSON 编解码器")
	}

	// 获取不存在的（应该返回 JSON）
	codec = GetOrDefault(protocol.CodecTypeMsgPack) // 未实现
	if codec == nil {
		t.Fatal("应该返回默认的 JSON 编解码器")
	}

	if codec.Name() != "json" {
		t.Errorf("应该返回 JSON 编解码器，实际: %s", codec.Name())
	}

	t.Log("GetOrDefault 测试通过")
}

// TestList 测试列出所有编解码器
func TestList(t *testing.T) {
	types := List()

	if len(types) == 0 {
		t.Fatal("应该至少有一个编解码器")
	}

	// 应该包含 JSON
	hasJSON := false
	for _, typ := range types {
		if typ == protocol.CodecTypeJSON {
			hasJSON = true
			break
		}
	}

	if !hasJSON {
		t.Error("应该包含 JSON 编解码器")
	}

	t.Logf("已注册的编解码器: %d 个", len(types))
	for _, typ := range types {
		t.Logf("  - %s", typ)
	}
}

// TestCodecTypeString 测试类型字符串转换
func TestCodecTypeString(t *testing.T) {
	tests := []struct {
		typ  protocol.CodecType
		want string
	}{
		{protocol.CodecTypeJSON, "json"},
		{protocol.CodecTypeProtobuf, "protobuf"},
		{protocol.CodecTypeMsgPack, "msgpack"},
		{protocol.CodecType(99), "unknown(99)"},
	}

	for _, tt := range tests {
		got := tt.typ.String()
		if got != tt.want {
			t.Errorf("CodecType(%d).String() = %s, want %s",
				tt.typ, got, tt.want)
		}
	}

	t.Log("CodecType.String() 测试通过")
}

// TestCompressedCodec 测试带压缩的编解码器
func TestCompressedCodec(t *testing.T) {
	// 1. 创建带压缩的 JSON 编解码器
	jsonCodec := NewJSONCodec()
	gzipCompressor := NewGzipCompressor(gzip.DefaultCompression)
	compressedCodec := NewCompressedCodec(jsonCodec, gzipCompressor)

	// 2. 测试数据（可压缩的重复数据）
	data := make(map[string]interface{})
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key_%d", i)
		data[key] = "This is a repeated value that should compress well"
	}

	// 3. 不压缩编码
	normalData, _ := jsonCodec.Encode(data)
	normalSize := len(normalData)

	// 4. 压缩编码
	compressedData, err := compressedCodec.Encode(data)
	if err != nil {
		t.Fatalf("压缩编码失败: %v", err)
	}
	compressedSize := len(compressedData)

	// 5. 比较大小
	t.Logf("原始大小: %d bytes", normalSize)
	t.Logf("压缩后大小: %d bytes", compressedSize)

	ratio := float64(normalSize-compressedSize) / float64(normalSize) * 100
	t.Logf("压缩率: %.2f%%", ratio)

	// 验证：压缩后应该更小
	if compressedSize >= normalSize {
		t.Error("压缩后应该更小")
	}

	// 6. 解压解码
	var decoded map[string]interface{}
	err = compressedCodec.Decode(compressedData, &decoded)
	if err != nil {
		t.Fatalf("解压解码失败: %v", err)
	}

	// 7. 验证数据一致性
	if len(decoded) != len(data) {
		t.Errorf("数据长度不一致: %d != %d", len(decoded), len(data))
	}

	for key, val := range data {
		if decoded[key] != val {
			t.Errorf("key %s 的值不一致", key)
			break
		}
	}

	t.Log("✅ 带压缩的编解码器测试通过")
}

// TestCompressedProtobuf 测试带压缩的 Protobuf
func TestCompressedProtobuf(t *testing.T) {
	// Protobuf + Gzip
	protoCodec := NewProtobufCodec()
	gzipCompressor := NewGzipCompressor(gzip.BestSpeed)
	compressedCodec := NewCompressedCodec(protoCodec, gzipCompressor)

	// 测试数据
	req := protocol.NewRequest("Service", "Method", map[string]interface{}{
		"data": strings.Repeat("test ", 200), // 可压缩数据
	})

	// 编码
	compressed, err := compressedCodec.Encode(req)
	if err != nil {
		t.Fatalf("编码失败: %v", err)
	}

	t.Logf("压缩后大小: %d bytes", len(compressed))

	// 解码
	var decoded protocol.Request
	err = compressedCodec.Decode(compressed, &decoded)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}

	// 验证
	if decoded.Service != req.Service {
		t.Error("Service 不一致")
	}

	t.Logf("编解码器名称: %s", compressedCodec.Name())
	t.Log("✅ 带压缩的 Protobuf 测试通过")
}

// TestCompressorRegistry 测试压缩器注册表
func TestCompressorRegistry(t *testing.T) {
	// None 应该已注册
	none := GetCompressor(protocol.CompressTypeNone) // ← 使用 protocol.CompressTypeNone
	if none == nil {
		t.Fatal("None 压缩器应该已注册")
	}

	// Gzip 应该已注册
	gzipComp := GetCompressor(protocol.CompressTypeGzip) // ← 使用 protocol.CompressTypeGzip
	if gzipComp == nil {
		t.Fatal("Gzip 压缩器应该已注册")
	}

	t.Logf("已注册压缩器: %s, %s", none.Name(), gzipComp.Name())
}

// TestGetCompressorOrNone 测试默认压缩器
func TestGetCompressorOrNone(t *testing.T) {
	// 获取不存在的压缩器，应该返回 None
	compressor := GetCompressorOrNone(protocol.CompressTypeSnappy) // ← 使用 protocol 常量

	if compressor == nil {
		t.Fatal("应该返回默认的 None 压缩器")
	}

	if compressor.Name() != "none" {
		t.Errorf("应该返回 None 压缩器，实际: %s", compressor.Name())
	}
}
