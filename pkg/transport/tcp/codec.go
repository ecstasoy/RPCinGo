// Kunhua Huang 2026

package tcp

import (
	"fmt"
	"io"

	"github.com/ecstasoy/RPCinGo/pkg/codec"
	"github.com/ecstasoy/RPCinGo/pkg/protocol"
)

// ProtocolCodec handles RPC protocol framing with protocol.Header.
//
// Framing responsibility:
//   - ProtocolCodec: Uses protocol.Header (20-byte fixed header + BodyLength field)
//   - codec.Codec: Encodes/decodes Request/Response to/from bytes (no framing)
//   - codec.StreamCodec: Independent framing (NOT used by ProtocolCodec)
//
// DO NOT mix ProtocolCodec with codec.StreamCodec's framing.
type ProtocolCodec struct {
	codec        codec.Codec
	compressor   codec.Compressor
	codecType    protocol.CodecType
	compressType protocol.CompressType
}

// NewProtocolCodec returns a ProtocolCodec configured for the supplied body
// codec and compression type.
func NewProtocolCodec(codecType protocol.CodecType, compressorType protocol.CompressType) *ProtocolCodec {
	return &ProtocolCodec{
		codec:        codec.GetOrDefault(codecType),
		compressor:   codec.GetCompressorOrNone(compressorType),
		codecType:    codecType,
		compressType: compressorType,
	}
}

// EncodeRequest serializes req into header-plus-body wire bytes.
func (pc *ProtocolCodec) EncodeRequest(req *protocol.Request) ([]byte, error) {
	bodyData, err := pc.codec.Encode(req)
	if err != nil {
		return nil, fmt.Errorf("encode request error: %w", err)
	}

	compressedBodyData, err := pc.compressor.Compress(bodyData)
	if err != nil {
		return nil, fmt.Errorf("compress request error: %w", err)
	}

	header := protocol.NewHeader(
		protocol.MsgTypeRequest,
		pc.codecType,
		req.ID,
		uint32(len(compressedBodyData)),
	)

	header.Compress = pc.compressType

	headerBytes := header.Encode()

	result := make([]byte, len(headerBytes)+len(compressedBodyData))
	copy(result[0:], headerBytes)
	copy(result[len(headerBytes):], compressedBodyData)

	return result, nil
}

// EncodeResponse serializes resp into header-plus-body wire bytes.
func (pc *ProtocolCodec) EncodeResponse(resp *protocol.Response) ([]byte, error) {
	bodyData, err := pc.codec.Encode(resp)
	if err != nil {
		return nil, fmt.Errorf("encode response error: %w", err)
	}

	compressedBodyData, err := pc.compressor.Compress(bodyData)
	if err != nil {
		return nil, fmt.Errorf("compress response error: %w", err)
	}

	header := protocol.NewHeader(
		protocol.MsgTypeResponse,
		pc.codecType,
		resp.ID,
		uint32(len(compressedBodyData)),
	)

	header.Compress = pc.compressType

	headerBytes := header.Encode()

	result := make([]byte, len(headerBytes)+len(compressedBodyData))
	copy(result[0:], headerBytes)
	copy(result[len(headerBytes):], compressedBodyData)

	return result, nil
}

// DecodeFromReader reads one full frame, validates the header, and
// decompresses the body.
func (pc *ProtocolCodec) DecodeFromReader(r io.Reader) (*protocol.Header, []byte, error) {
	headerBytes := make([]byte, protocol.HeaderLength)
	if _, err := io.ReadFull(r, headerBytes); err != nil {
		return nil, nil, fmt.Errorf("read header error: %w", err)
	}

	header := &protocol.Header{}
	if err := header.Decode(headerBytes); err != nil {
		return nil, nil, fmt.Errorf("decode header error: %w", err)
	}

	bodyBytes := make([]byte, header.BodyLength)
	if _, err := io.ReadFull(r, bodyBytes); err != nil {
		return nil, nil, fmt.Errorf("read body error: %w", err)
	}

	decompressedBodyData, err := pc.getCompressorByType(header.Compress).Decompress(bodyBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("decompress body error: %w", err)
	}

	return header, decompressedBodyData, nil
}

func (pc *ProtocolCodec) getCompressorByType(compressType protocol.CompressType) codec.Compressor {
	compressor := codec.GetCompressor(compressType)
	if compressor == nil {
		return codec.GetCompressor(protocol.CompressTypeNone)
	}
	return compressor
}

// DecodeRequest decodes one request body using the connection's static codec.
func (pc *ProtocolCodec) DecodeRequest(data []byte) (*protocol.Request, error) {
	return pc.DecodeRequestWith(pc.codecType, data)
}

// DecodeRequestWith decodes one request body using the codec advertised in the
// frame header. Honouring the header byte makes the on-wire Codec field
// meaningful rather than dead metadata, and lets a peer encode a frame with a
// codec other than the connection default.
func (pc *ProtocolCodec) DecodeRequestWith(codecType protocol.CodecType, data []byte) (*protocol.Request, error) {
	var req protocol.Request
	if err := codec.GetOrDefault(codecType).Decode(data, &req); err != nil {
		return nil, fmt.Errorf("decode request error: %w", err)
	}
	return &req, nil
}

// DecodeResponse decodes one response body using the connection's static codec.
func (pc *ProtocolCodec) DecodeResponse(data []byte) (*protocol.Response, error) {
	return pc.DecodeResponseWith(pc.codecType, data)
}

// DecodeResponseWith decodes one response body using the codec advertised in the
// frame header.
func (pc *ProtocolCodec) DecodeResponseWith(codecType protocol.CodecType, data []byte) (*protocol.Response, error) {
	var resp protocol.Response
	if err := codec.GetOrDefault(codecType).Decode(data, &resp); err != nil {
		return nil, fmt.Errorf("decode response error: %w", err)
	}
	return &resp, nil
}

// WriteRequest encodes and writes one framed request to w.
func (pc *ProtocolCodec) WriteRequest(w io.Writer, req *protocol.Request) error {
	data, err := pc.EncodeRequest(req)
	if err != nil {
		return fmt.Errorf("encode request error: %w", err)
	}

	if err := writeFull(w, data); err != nil {
		return fmt.Errorf("write request error: %w", err)
	}

	return nil
}

// WriteResponse encodes and writes one framed response to w.
func (pc *ProtocolCodec) WriteResponse(w io.Writer, resp *protocol.Response) error {
	data, err := pc.EncodeResponse(resp)
	if err != nil {
		return fmt.Errorf("encode response error: %w", err)
	}

	if err := writeFull(w, data); err != nil {
		return fmt.Errorf("write response error: %w", err)
	}

	return nil
}

// ReadRequest reads and decodes one framed request from r.
func (pc *ProtocolCodec) ReadRequest(r io.Reader) (*protocol.Header, *protocol.Request, error) {
	header, bodyData, err := pc.DecodeFromReader(r)
	if err != nil {
		return nil, nil, fmt.Errorf("decode request error: %w", err)
	}

	if header.MsgType != protocol.MsgTypeRequest {
		return nil, nil, fmt.Errorf("message type error, expected %s, got %s", protocol.MsgTypeRequest, header.MsgType)
	}

	req, err := pc.DecodeRequestWith(header.Codec, bodyData)
	if err != nil {
		return nil, nil, fmt.Errorf("decode request error: %w", err)
	}

	return header, req, nil
}

// ReadResponse reads and decodes one framed response from r.
func (pc *ProtocolCodec) ReadResponse(r io.Reader) (*protocol.Header, *protocol.Response, error) {
	header, bodyData, err := pc.DecodeFromReader(r)
	if err != nil {
		return nil, nil, fmt.Errorf("decode response error: %w", err)
	}

	if header.MsgType != protocol.MsgTypeResponse {
		return nil, nil, fmt.Errorf("message type error, expected %s, got %s", protocol.MsgTypeResponse, header.MsgType)
	}

	resp, err := pc.DecodeResponseWith(header.Codec, bodyData)
	if err != nil {
		return nil, nil, fmt.Errorf("decode response error: %w", err)
	}

	return header, resp, nil
}
