// Kunhua Huang 2026

package tcp

import (
	"fmt"
	"io"

	"RPCinGo/pkg/codec"
	"RPCinGo/pkg/protocol"
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

func NewProtocolCodec(codecType protocol.CodecType, compressorType protocol.CompressType) *ProtocolCodec {
	return &ProtocolCodec{
		codec:        codec.GetOrDefault(codecType),
		compressor:   codec.GetCompressorOrNone(compressorType),
		codecType:    codecType,
		compressType: compressorType,
	}
}

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

func (pc *ProtocolCodec) DecodeRequest(data []byte) (*protocol.Request, error) {
	var req protocol.Request
	if err := pc.codec.Decode(data, &req); err != nil {
		return nil, fmt.Errorf("decode request error: %w", err)
	}
	return &req, nil
}

func (pc *ProtocolCodec) DecodeResponse(data []byte) (*protocol.Response, error) {
	var resp protocol.Response
	if err := pc.codec.Decode(data, &resp); err != nil {
		return nil, fmt.Errorf("decode response error: %w", err)
	}
	return &resp, nil
}

func (pc *ProtocolCodec) WriteRequest(w io.Writer, req *protocol.Request) error {
	data, err := pc.EncodeRequest(req)
	if err != nil {
		return fmt.Errorf("encode request error: %w", err)
	}

	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write response error: %w", err)
	}

	return nil
}

func (pc *ProtocolCodec) WriteResponse(w io.Writer, resp *protocol.Response) error {
	data, err := pc.EncodeResponse(resp)
	if err != nil {
		return fmt.Errorf("encode response error: %w", err)
	}

	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write response error: %w", err)
	}

	return nil
}

func (pc *ProtocolCodec) ReadRequest(r io.Reader) (*protocol.Header, *protocol.Request, error) {
	header, bodyData, err := pc.DecodeFromReader(r)
	if err != nil {
		return nil, nil, fmt.Errorf("decode request error: %w", err)
	}

	if header.MsgType != protocol.MsgTypeRequest {
		return nil, nil, fmt.Errorf("message type error, expected %s, got %s", protocol.MsgTypeRequest, header.MsgType)
	}

	req, err := pc.DecodeRequest(bodyData)
	if err != nil {
		return nil, nil, fmt.Errorf("decode request error: %w", err)
	}

	return header, req, nil
}

func (pc *ProtocolCodec) ReadResponse(r io.Reader) (*protocol.Header, *protocol.Response, error) {
	header, bodyData, err := pc.DecodeFromReader(r)
	if err != nil {
		return nil, nil, fmt.Errorf("decode response error: %w", err)
	}

	if header.MsgType != protocol.MsgTypeResponse {
		return nil, nil, fmt.Errorf("message type error, expected %s, got %s", protocol.MsgTypeResponse, header.MsgType)
	}

	resp, err := pc.DecodeResponse(bodyData)
	if err != nil {
		return nil, nil, fmt.Errorf("decode response error: %w", err)
	}

	return header, resp, nil
}
