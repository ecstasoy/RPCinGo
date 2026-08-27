package protocol

import (
	"testing"
)

// TestNewHeader 测试创建协议头
func TestNewHeader(t *testing.T) {
	header := NewHeader(MsgTypeRequest, CodecTypeJSON, 123, 456)

	// 验证字段
	if header.Magic != ProtocolMagic {
		t.Errorf("Magic错误: 期望 0x%X, 实际 0x%X", ProtocolMagic, header.Magic)
	}

	if header.Version != ProtocolVersion {
		t.Errorf("Version错误: 期望 %d, 实际 %d", ProtocolVersion, header.Version)
	}

	if header.MsgType != MsgTypeRequest {
		t.Errorf("MsgType错误")
	}

	if header.Codec != CodecTypeJSON {
		t.Errorf("Codec错误")
	}

	if header.RequestID != 123 {
		t.Errorf("RequestID错误")
	}

	if header.BodyLength != 456 {
		t.Errorf("BodyLength错误")
	}

	t.Logf("Header创建成功: %s", header)
}

// TestHeaderEncode 测试编码
func TestHeaderEncode(t *testing.T) {
	header := NewHeader(MsgTypeRequest, CodecTypeJSON, 100, 200)

	// 编码
	data := header.Encode()

	// 验证长度
	if len(data) != HeaderLength {
		t.Fatalf("编码长度错误: 期望 %d, 实际 %d", HeaderLength, len(data))
	}

	// 验证魔数（前两个字节）
	if data[0] != 0xCA || data[1] != 0xFE {
		t.Errorf("魔数编码错误: [0x%X, 0x%X]", data[0], data[1])
	}

	t.Logf("编码成功: %d 字节", len(data))
}

// TestHeaderDecode 测试解码
func TestHeaderDecode(t *testing.T) {
	// 1. 先编码
	original := NewHeader(MsgTypeResponse, CodecTypeProtobuf, 999, 1024)
	data := original.Encode()

	// 2. 再解码
	decoded := &Header{}
	err := decoded.Decode(data)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}

	// 3. 验证
	if decoded.Magic != original.Magic {
		t.Errorf("Magic不一致")
	}
	if decoded.RequestID != original.RequestID {
		t.Errorf("RequestID不一致")
	}
	if decoded.BodyLength != original.BodyLength {
		t.Errorf("BodyLength不一致")
	}

	t.Logf("解码成功: %s", decoded)
}

// TestHeaderDecodeInvalidMagic 测试无效魔数
func TestHeaderDecodeInvalidMagic(t *testing.T) {
	data := make([]byte, HeaderLength)
	data[0] = 0x00 // 错误的魔数
	data[1] = 0x00

	header := &Header{}
	err := header.Decode(data)

	// 应该返回错误
	if err == nil {
		t.Error("应该返回错误，但没有")
	}

	t.Logf("正确检测到无效魔数: %v", err)
}
