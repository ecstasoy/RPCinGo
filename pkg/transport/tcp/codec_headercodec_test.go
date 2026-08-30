package tcp

import (
	"bytes"
	"testing"

	"github.com/ecstasoy/RPCinGo/pkg/protocol"
)

// TestReadRequestHonoursHeaderCodec proves the on-wire Codec byte is meaningful:
// a frame encoded with JSON is decoded correctly even when the receiver's static
// codec is Protobuf. Protobuf decoding of a protocol.Request would fail, so a
// successful decode demonstrates the header codec — not the connection default —
// drove the decode.
func TestReadRequestHonoursHeaderCodec(t *testing.T) {
	sender := NewProtocolCodec(protocol.CodecTypeJSON, protocol.CompressTypeNone)
	req := protocol.NewRequest("Svc", "M", map[string]interface{}{"a": 1})
	req.ID = 42

	frame, err := sender.EncodeRequest(req)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	// Receiver's static codec is Protobuf; only by honouring the JSON header
	// byte can it decode a non-proto Request body.
	receiver := NewProtocolCodec(protocol.CodecTypeProtobuf, protocol.CompressTypeNone)

	header, got, err := receiver.ReadRequest(bytes.NewReader(frame))
	if err != nil {
		t.Fatalf("ReadRequest honouring header codec failed: %v", err)
	}
	if header.Codec != protocol.CodecTypeJSON {
		t.Errorf("header.Codec = %v, want json", header.Codec)
	}
	if got.Service != "Svc" || got.Method != "M" || got.ID != 42 {
		t.Errorf("decoded request mismatch: %+v", got)
	}
}
