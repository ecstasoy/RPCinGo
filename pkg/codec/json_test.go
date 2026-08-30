package codec

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ecstasoy/RPCinGo/pkg/protocol"
)

// 测试用的结构体
type TestStruct struct {
	ID   int      `json:"id"`
	Name string   `json:"name"`
	Tags []string `json:"tags,omitempty"`
}

// TestJSONCodecEncode 测试 JSON 编码
func TestJSONCodecEncode(t *testing.T) {
	codec := NewJSONCodec()

	obj := TestStruct{
		ID:   123,
		Name: "Alice",
		Tags: []string{"go", "rpc"},
	}

	// 编码
	data, err := codec.Encode(obj)
	if err != nil {
		t.Fatalf("编码失败: %v", err)
	}

	// 检查结果
	if len(data) == 0 {
		t.Fatal("编码结果为空")
	}

	t.Logf("编码成功: %s", string(data))
	// 预期: {"id":123,"name":"Alice","tags":["go","rpc"]}
}

// TestJSONCodecDecode 测试 JSON 解码
func TestJSONCodecDecode(t *testing.T) {
	codec := NewJSONCodec()

	// JSON 数据
	data := []byte(`{"id":456,"name":"Bob","tags":["python"]}`)

	// 解码
	var obj TestStruct
	err := codec.Decode(data, &obj)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}

	// 验证字段
	if obj.ID != 456 {
		t.Errorf("ID 错误: 期望 456, 实际 %d", obj.ID)
	}

	if obj.Name != "Bob" {
		t.Errorf("Name 错误: 期望 Bob, 实际 %s", obj.Name)
	}

	if len(obj.Tags) != 1 {
		t.Errorf("Tags 长度错误: 期望 1, 实际 %d", len(obj.Tags))
	}

	t.Logf("解码成功: %+v", obj)
}

// TestJSONCodecRoundTrip 测试编码解码往返
func TestJSONCodecRoundTrip(t *testing.T) {
	codec := NewJSONCodec()

	// 原始对象
	original := TestStruct{
		ID:   789,
		Name: "Charlie",
		Tags: []string{"admin", "user"},
	}

	// 编码
	data, err := codec.Encode(original)
	if err != nil {
		t.Fatalf("编码失败: %v", err)
	}

	// 解码
	var decoded TestStruct
	err = codec.Decode(data, &decoded)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}

	// 验证数据一致性
	if original.ID != decoded.ID {
		t.Errorf("ID 不一致: %d != %d", original.ID, decoded.ID)
	}
	if original.Name != decoded.Name {
		t.Errorf("Name 不一致: %s != %s", original.Name, decoded.Name)
	}
	if len(original.Tags) != len(decoded.Tags) {
		t.Errorf("Tags 长度不一致")
	}

	t.Logf("往返测试成功")
	t.Logf("原始: %+v", original)
	t.Logf("解码: %+v", decoded)
}

// TestJSONCodecName 测试 Name 方法
func TestJSONCodecName(t *testing.T) {
	codec := NewJSONCodec()

	name := codec.Name()
	if name != "json" {
		t.Errorf("Name 错误: 期望 json, 实际 %s", name)
	}

	t.Logf("编解码器名称: %s", name)
}

// TestJSONCodecAutoRegister 测试自动注册
func TestJSONCodecAutoRegister(t *testing.T) {
	// JSON 编解码器应该已自动注册
	codec := Get(protocol.CodecTypeJSON)
	if codec == nil {
		t.Fatal("JSON 编解码器应该已自动注册")
	}

	if codec.Name() != "json" {
		t.Errorf("注册的编解码器名称错误: %s", codec.Name())
	}

	t.Log("JSON 编解码器自动注册成功")
}

// BenchmarkJSONEncode 性能测试：编码
func BenchmarkJSONEncode(b *testing.B) {
	codec := NewJSONCodec()
	obj := TestStruct{
		ID:   123,
		Name: "Benchmark",
		Tags: []string{"test"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = codec.Encode(obj)
	}
}

// BenchmarkJSONDecode 性能测试：解码
func BenchmarkJSONDecode(b *testing.B) {
	codec := NewJSONCodec()
	data := []byte(`{"id":123,"name":"Benchmark","tags":["test"]}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var obj TestStruct
		_ = codec.Decode(data, &obj)
	}
}

// ===== 流式编解码测试 =====

// TestJSONStreamEncode 测试流式编码
func TestJSONStreamEncode(t *testing.T) {
	codec := NewJSONCodec()

	obj := TestStruct{
		ID:   999,
		Name: "Stream",
		Tags: []string{"test"},
	}

	// 使用 bytes.Buffer 模拟网络连接
	var buf bytes.Buffer

	// 流式编码
	err := codec.(StreamCodec).EncodeToWriter(&buf, obj)
	if err != nil {
		t.Fatalf("流式编码失败: %v", err)
	}

	// 验证结果
	data := buf.Bytes()
	if len(data) == 0 {
		t.Fatal("流式编码结果为空")
	}

	t.Logf("流式编码成功: %s", string(data))

	// 验证可以正确解码
	var decoded TestStruct
	err = codec.Decode(data, &decoded)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}

	if decoded.ID != obj.ID {
		t.Error("流式编码数据不一致")
	}
}

// TestJSONStreamDecode 测试流式解码
func TestJSONStreamDecode(t *testing.T) {
	codec := NewJSONCodec()

	// JSON 数据
	jsonData := `{"id":888,"name":"StreamDecode","tags":["a","b"]}`

	// 使用 strings.Reader 模拟网络连接
	reader := strings.NewReader(jsonData)

	// 流式解码
	var obj TestStruct
	err := codec.(StreamCodec).DecodeFromReader(reader, &obj)
	if err != nil {
		t.Fatalf("流式解码失败: %v", err)
	}

	// 验证
	if obj.ID != 888 {
		t.Errorf("ID 错误: %d", obj.ID)
	}

	if obj.Name != "StreamDecode" {
		t.Errorf("Name 错误: %s", obj.Name)
	}

	t.Logf("流式解码成功: %+v", obj)
}

// TestJSONStreamRoundTrip 测试流式往返
func TestJSONStreamRoundTrip(t *testing.T) {
	codec := NewJSONCodec()

	original := TestStruct{
		ID:   777,
		Name: "RoundTrip",
		Tags: []string{"stream", "test"},
	}

	// 1. 流式编码到 Buffer
	var buf bytes.Buffer
	err := codec.(StreamCodec).EncodeToWriter(&buf, original)
	if err != nil {
		t.Fatalf("流式编码失败: %v", err)
	}

	// 2. 流式解码
	var decoded TestStruct
	err = codec.(StreamCodec).DecodeFromReader(&buf, &decoded)
	if err != nil {
		t.Fatalf("流式解码失败: %v", err)
	}

	// 3. 验证
	if decoded.ID != original.ID {
		t.Error("ID 不一致")
	}
	if decoded.Name != original.Name {
		t.Error("Name 不一致")
	}

	t.Log("✅ 流式往返测试成功")
}

// BenchmarkJSONStreamEncode 性能测试：流式编码
func BenchmarkJSONStreamEncode(b *testing.B) {
	codec := NewJSONCodec()
	obj := TestStruct{ID: 123, Name: "Benchmark", Tags: []string{"test"}}

	// 创建一个丢弃写入的 Writer（测试纯编码性能）
	w := &discardWriter{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = codec.(StreamCodec).EncodeToWriter(w, obj)
	}
}

// BenchmarkJSONStreamDecode 性能测试：流式解码
func BenchmarkJSONStreamDecode(b *testing.B) {
	codec := NewJSONCodec()
	jsonData := `{"id":123,"name":"Benchmark","tags":["test"]}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := strings.NewReader(jsonData)
		var obj TestStruct
		_ = codec.(StreamCodec).DecodeFromReader(reader, &obj)
	}
}

// discardWriter 丢弃写入的 Writer（用于性能测试）
type discardWriter struct{}

func (d *discardWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}
