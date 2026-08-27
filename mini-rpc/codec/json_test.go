package codec

import (
	"bytes"
	"testing"
)

// 测试用的结构体
type TestStruct struct {
	ID   int      `json:"id"`
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// TestJSONCodec_Encode 测试 JSON 编码
func TestJSONCodec_Encode(t *testing.T) {
	codec := NewJSONCodec()

	obj := TestStruct{
		ID:   123,
		Name: "Alice",
		Tags: []string{"student", "golang"},
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

	t.Logf("编码结果: %s", string(data))
	// 预期输出: {"id":123,"name":"Alice","tags":["student","golang"]}
}

// TestJSONCodec_Decode 测试 JSON 解码
func TestJSONCodec_Decode(t *testing.T) {
	codec := NewJSONCodec()

	// JSON 数据
	data := []byte(`{"id":456,"name":"Bob","tags":["teacher","python"]}`)

	// 解码
	var obj TestStruct
	err := codec.Decode(data, &obj) // 注意：传指针
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}

	// 验证字段
	if obj.ID != 456 {
		t.Errorf("期望 ID=456, 实际=%d", obj.ID)
	}
	if obj.Name != "Bob" {
		t.Errorf("期望 Name=Bob, 实际=%s", obj.Name)
	}
	if len(obj.Tags) != 2 {
		t.Errorf("期望 Tags 长度=2, 实际=%d", len(obj.Tags))
	}

	t.Logf("解码结果: %+v", obj)
	// %+v 会打印字段名: {ID:456 Name:Bob Tags:[teacher python]}
}

// TestJSONCodec_EncodeDecode 测试编码解码往返
func TestJSONCodec_EncodeDecode(t *testing.T) {
	codec := NewJSONCodec()

	// 原始对象
	original := TestStruct{
		ID:   789,
		Name: "Charlie",
		Tags: []string{"admin"},
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

	t.Logf("原始: %+v", original)
	t.Logf("解码: %+v", decoded)
}

// TestJSONCodec_StreamEncode 测试流式编码
func TestJSONCodec_StreamEncode(t *testing.T) {
	codec := &JSONCodec{} // 直接创建，测试 StreamCodec 方法

	obj := TestStruct{ID: 999, Name: "Stream"}

	// 创建一个 Buffer 作为 Writer
	var buf bytes.Buffer

	// 流式编码
	err := codec.EncodeToWriter(&buf, obj)
	if err != nil {
		t.Fatalf("流式编码失败: %v", err)
	}

	// 检查结果
	data := buf.Bytes()
	if len(data) == 0 {
		t.Fatal("流式编码结果为空")
	}

	t.Logf("流式编码结果: %s", string(data))
}

// TestJSONCodec_StreamDecode 测试流式解码
func TestJSONCodec_StreamDecode(t *testing.T) {
	codec := &JSONCodec{}

	// JSON 数据
	data := []byte(`{"id":888,"name":"Stream"}`)

	// 创建一个 Buffer 作为 Reader
	buf := bytes.NewBuffer(data)

	// 流式解码
	var obj TestStruct
	err := codec.DecodeFromReader(buf, &obj)
	if err != nil {
		t.Fatalf("流式解码失败: %v", err)
	}

	// 验证
	if obj.ID != 888 {
		t.Errorf("期望 ID=888, 实际=%d", obj.ID)
	}

	t.Logf("流式解码结果: %+v", obj)
}

// TestGetCodec 测试获取注册的编解码器
func TestGetCodec(t *testing.T) {
	// 获取 JSON 编解码器
	codec := GetCodec(JSONCodecType)
	if codec == nil {
		t.Fatal("获取 JSON 编解码器失败")
	}

	// 测试使用
	obj := TestStruct{ID: 1, Name: "Test"}
	data, err := codec.Encode(obj)
	if err != nil {
		t.Fatalf("编码失败: %v", err)
	}

	t.Logf("通过注册表获取的编解码器工作正常: %s", string(data))
}

// BenchmarkJSONCodec_Encode 性能测试：编码
func BenchmarkJSONCodec_Encode(b *testing.B) {
	codec := NewJSONCodec()
	obj := TestStruct{ID: 123, Name: "Benchmark", Tags: []string{"test"}}

	b.ResetTimer() // 重置计时器（忽略初始化时间）
	for i := 0; i < b.N; i++ {
		_, _ = codec.Encode(obj)
	}
}

// BenchmarkJSONCodec_Decode 性能测试：解码
func BenchmarkJSONCodec_Decode(b *testing.B) {
	codec := NewJSONCodec()
	data := []byte(`{"id":123,"name":"Benchmark","tags":["test"]}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var obj TestStruct
		_ = codec.Decode(data, &obj)
	}
}
