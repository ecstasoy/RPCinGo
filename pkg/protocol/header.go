// Kunhua Huang 2025

package protocol

import (
	"encoding/binary"
	"fmt"
)

// HeaderLength and related constants define the fixed RPC frame header layout.
const (
	HeaderLength         = 20
	ProtocolMagic        = 0xCAFE
	ProtocolVersion byte = 0x01
)

// MessageType identifies whether a frame carries a request or a response.
type MessageType byte

// MessageType values distinguish request and response frames.
const (
	MsgTypeRequest  MessageType = 0x01
	MsgTypeResponse MessageType = 0x02
)

// String returns the symbolic name of the message type.
func (t MessageType) String() string {
	switch t {
	case MsgTypeRequest:
		return "request"
	case MsgTypeResponse:
		return "response"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

// CodecType identifies the payload codec recorded in a frame header.
type CodecType byte

// CodecType values advertise the body encoding used for a frame payload.
const (
	CodecTypeJSON     CodecType = 0x00
	CodecTypeProtobuf CodecType = 0x01
	CodecTypeMsgPack  CodecType = 0x02
)

// String returns the symbolic name of the codec type.
func (t CodecType) String() string {
	switch t {
	case CodecTypeJSON:
		return "json"
	case CodecTypeProtobuf:
		return "protobuf"
	case CodecTypeMsgPack:
		return "msgpack"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

// CompressType identifies the compression algorithm recorded in a frame
// header.
type CompressType byte

// CompressType values advertise how the frame payload was compressed.
const (
	CompressTypeNone   CompressType = 0x00
	CompressTypeGzip   CompressType = 0x01
	CompressTypeSnappy CompressType = 0x02
)

// String returns the symbolic name of the compression type.
func (t CompressType) String() string {
	switch t {
	case CompressTypeNone:
		return "none"
	case CompressTypeGzip:
		return "gzip"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

// Header is the fixed-length prefix that precedes every encoded request and
// response body.
//
// Layout (20 bytes):
//
//	 0  1  2  3  4  5  6  7  8  9  10 11 12 13 14 15 16 17 18 19
//	+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//	|Magic |Ver|Typ|Cod|Cmp|Reserv |    Request ID     |BodyLen |
//	+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
type Header struct {
	Magic      uint16
	Version    byte
	MsgType    MessageType
	Codec      CodecType
	Compress   CompressType
	Reserved   [2]byte
	RequestID  uint64
	BodyLength uint32
}

// NewHeader constructs a Header initialized with the protocol magic, version,
// and the supplied routing fields.
func NewHeader(msgType MessageType, codec CodecType, requestID uint64, bodyLen uint32) *Header {
	return &Header{
		Magic:      ProtocolMagic,
		Version:    ProtocolVersion,
		MsgType:    msgType,
		Codec:      codec,
		Compress:   CompressTypeNone,
		Reserved:   [2]byte{0, 0},
		RequestID:  requestID,
		BodyLength: bodyLen,
	}
}

// Encode serializes the header into its fixed 20-byte wire representation.
func (h *Header) Encode() []byte {
	buf := make([]byte, HeaderLength)

	binary.BigEndian.PutUint16(buf[0:2], h.Magic)

	buf[2] = h.Version
	buf[3] = byte(h.MsgType)
	buf[4] = byte(h.Codec)
	buf[5] = byte(h.Compress)
	buf[6] = h.Reserved[0]
	buf[7] = h.Reserved[1]

	binary.BigEndian.PutUint64(buf[8:16], h.RequestID)
	binary.BigEndian.PutUint32(buf[16:20], h.BodyLength)

	return buf
}

// Decode parses the fixed-size wire representation into h and validates the
// magic number and protocol version.
func (h *Header) Decode(buf []byte) error {
	if len(buf) < HeaderLength {
		return fmt.Errorf("invalid header length: %d, expected: %d", len(buf), HeaderLength)
	}

	h.Magic = binary.BigEndian.Uint16(buf[0:2])
	if h.Magic != ProtocolMagic {
		return fmt.Errorf("invalid magic number: 0x%X, expected: 0x%X", h.Magic, ProtocolMagic)
	}

	h.Version = buf[2]
	if h.Version != ProtocolVersion {
		return fmt.Errorf("unsupported protocol version: %d", h.Version)
	}

	h.MsgType = MessageType(buf[3])
	h.Codec = CodecType(buf[4])
	h.Compress = CompressType(buf[5])
	h.Reserved[0] = buf[6]
	h.Reserved[1] = buf[7]
	h.RequestID = binary.BigEndian.Uint64(buf[8:16])
	h.BodyLength = binary.BigEndian.Uint32(buf[16:20])

	return nil
}

// String returns a compact human-readable summary of the header.
func (h *Header) String() string {
	return fmt.Sprintf(
		"Header{Magic=0x%X, Version=%d, Type=%d, Codec=%d, Compress=%d, RequestID=%d, BodyLen=%d}",
		h.Magic,
		h.Version,
		h.MsgType,
		h.Codec,
		h.Compress,
		h.RequestID,
		h.BodyLength,
	)
}
