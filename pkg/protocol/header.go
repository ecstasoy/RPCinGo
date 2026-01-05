// Kunhua Huang 2025

package protocol

import (
	"encoding/binary"
	"fmt"
)

const (
	HeaderLength         = 20
	ProtocolMagic        = 0xCAFE
	ProtocolVersion byte = 0x01
)

type MessageType byte

const (
	MsgTypeRequest  MessageType = 0x01
	MsgTypeResponse MessageType = 0x02
)

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

type CodecType byte

const (
	CodecTypeJSON     CodecType = 0x00
	CodecTypeProtobuf CodecType = 0x01
	CodecTypeMsgPack  CodecType = 0x02
)

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

type CompressType byte

const (
	CompressTypeNone   CompressType = 0x00
	CompressTypeGzip   CompressType = 0x01
	CompressTypeSnappy CompressType = 0x02
)

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

// Header Structure
// Fixed length: 20 bytes
//
// Byte layout:
//   0  1  2  3  4  5  6  7  8  9  10 11 12 13 14 15 16 17 18 19
//  +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//  |Magic |Ver|Typ|Cod|Cmp|Reserv |    Request ID     |BodyLen |
//  +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+

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
