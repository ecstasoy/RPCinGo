// Kunhua Huang 2025

package codec

import (
	"encoding/json"
	"fmt"
	"io"

	"RPCinGo/pkg/protocol"
	"RPCinGo/pkg/protocol/pb"

	"google.golang.org/protobuf/proto"
)

type JSONCodec struct{}

var _ Codec = (*JSONCodec)(nil)
var _ StreamCodec = (*JSONCodec)(nil)

func NewJSONCodec() Codec {
	return &JSONCodec{}
}

func (c *JSONCodec) Encode(v interface{}) ([]byte, error) {
	if req, ok := v.(*protocol.Request); ok {
		return c.encodeRequest(req)
	}

	if resp, ok := v.(*protocol.Response); ok {
		return c.encodeResponse(resp)
	}

	return json.Marshal(v)
}

func (c *JSONCodec) Decode(data []byte, v interface{}) error {
	if req, ok := v.(*protocol.Request); ok {
		return c.decodeRequest(data, req)
	}

	if resp, ok := v.(*protocol.Response); ok {
		return c.decodeResponse(data, resp)
	}

	return json.Unmarshal(data, v)
}

func (c *JSONCodec) EncodeToWriter(w io.Writer, v interface{}) error {
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(v); err != nil {
		return fmt.Errorf("json codec: encode to writer failed: %w", err)
	}
	return nil
}

func (c *JSONCodec) DecodeFromReader(r io.Reader, v interface{}) error {
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(v); err != nil {
		return fmt.Errorf("json codec: decode from reader failed: %w", err)
	}
	return nil
}

func (c *JSONCodec) encodeRequest(req *protocol.Request) ([]byte, error) {
	var argsData []byte
	var argsCodec pb.PayloadCodec
	var err error

	if req.Args != nil {
		if req.ArgsCodec != protocol.PayloadCodecUnknown {
			argsCodec = req.ArgsCodec

			switch req.ArgsCodec {
			case protocol.PayloadCodecRaw:
				if argBytes, ok := req.Args.([]byte); ok {
					argsData = argBytes
				} else {
					return nil, fmt.Errorf("ArgsCodec is RAW but Args is not []byte (got %T)", req.Args)
				}
			case protocol.PayloadCodecJSON:
				if argBytes, ok := req.Args.([]byte); ok {
					argsData = argBytes
				} else {
					argsData, err = json.Marshal(req.Args)
					if err != nil {
						return nil, fmt.Errorf("marshal args as JSON failed: %w", err)
					}
				}
			case protocol.PayloadCodecProtobuf:
				if argBytes, ok := req.Args.([]byte); ok {
					argsData = argBytes
				} else if protoMsg, ok := req.Args.(proto.Message); ok {
					argsData, err = proto.Marshal(protoMsg)
					if err != nil {
						return nil, fmt.Errorf("marshal args as Protobuf failed: %w", err)
					}
				} else {
					return nil, fmt.Errorf("ArgsCodec is PROTOBUF but Args is not proto.Message or []byte (got %T)", req.Args)
				}
			default:
				return nil, fmt.Errorf("unsupported ArgsCodec: %v", req.ArgsCodec)
			}
		} else {
			if argBytes, ok := req.Args.([]byte); ok {
				argsData = argBytes
				argsCodec = pb.PayloadCodec_PAYLOAD_CODEC_RAW
			} else if protoMsg, ok := req.Args.(proto.Message); ok {
				argsData, err = proto.Marshal(protoMsg)
				if err != nil {
					return nil, fmt.Errorf("marshal args failed: %w", err)
				}
				argsCodec = pb.PayloadCodec_PAYLOAD_CODEC_PROTOBUF
			} else {
				argsData, err = json.Marshal(req.Args)
				if err != nil {
					return nil, fmt.Errorf("marshal args failed: %w", err)
				}
				argsCodec = pb.PayloadCodec_PAYLOAD_CODEC_JSON
			}
		}
	}

	wireReq := struct {
		ID             uint64            `json:"id"`
		Service        string            `json:"service"`
		Method         string            `json:"method"`
		ServiceVersion string            `json:"service_version,omitempty"`
		Args           []byte            `json:"args,omitempty"`
		Timeout        int64             `json:"timeout,omitempty"`
		IsStream       bool              `json:"is_stream,omitempty"`
		Metadata       map[string]string `json:"metadata,omitempty"`
		CreatedAt      int64             `json:"created_at,omitempty"`
		ArgsCodec      pb.PayloadCodec   `json:"args_codec,omitempty"`
	}{
		ID:             req.ID,
		Service:        req.Service,
		Method:         req.Method,
		ServiceVersion: req.ServiceVersion,
		Args:           argsData,
		Timeout:        req.Timeout,
		IsStream:       req.IsStream,
		CreatedAt:      req.CreatedAt,
		ArgsCodec:      argsCodec,
	}

	if req.Metadata != nil {
		wireReq.Metadata = req.Metadata.ToMap()
	}

	return json.Marshal(wireReq)
}

func (c *JSONCodec) decodeRequest(data []byte, req *protocol.Request) error {
	wireReq := struct {
		ID             uint64            `json:"id"`
		Service        string            `json:"service"`
		Method         string            `json:"method"`
		ServiceVersion string            `json:"service_version,omitempty"`
		Args           []byte            `json:"args,omitempty"`
		Timeout        int64             `json:"timeout,omitempty"`
		IsStream       bool              `json:"is_stream,omitempty"`
		Metadata       map[string]string `json:"metadata,omitempty"`
		CreatedAt      int64             `json:"created_at,omitempty"`
		ArgsCodec      pb.PayloadCodec   `json:"args_codec,omitempty"`
	}{}

	if err := json.Unmarshal(data, &wireReq); err != nil {
		return err
	}

	req.ID = wireReq.ID
	req.Service = wireReq.Service
	req.Method = wireReq.Method
	req.ServiceVersion = wireReq.ServiceVersion
	req.Timeout = wireReq.Timeout
	req.IsStream = wireReq.IsStream
	req.CreatedAt = wireReq.CreatedAt
	req.ArgsCodec = wireReq.ArgsCodec

	if len(wireReq.Args) > 0 {
		req.Args = wireReq.Args
	}

	if wireReq.Metadata != nil {
		req.Metadata = protocol.FromMap(wireReq.Metadata)
	} else {
		req.Metadata = protocol.NewMetadata()
	}

	return nil
}

func (c *JSONCodec) encodeResponse(resp *protocol.Response) ([]byte, error) {
	var dataBytes []byte
	var dataCodec pb.PayloadCodec
	var err error

	if resp.Data != nil {
		if resp.DataCodec != protocol.PayloadCodecUnknown {
			dataCodec = resp.DataCodec

			switch resp.DataCodec {
			case protocol.PayloadCodecRaw:
				if dataB, ok := resp.Data.([]byte); ok {
					dataBytes = dataB
				} else {
					return nil, fmt.Errorf("DataCodec is RAW but Data is not []byte (got %T)", resp.Data)
				}
			case protocol.PayloadCodecJSON:
				if dataB, ok := resp.Data.([]byte); ok {
					dataBytes = dataB
				} else {
					dataBytes, err = json.Marshal(resp.Data)
					if err != nil {
						return nil, fmt.Errorf("marshal data as JSON failed: %w", err)
					}
				}
			case protocol.PayloadCodecProtobuf:
				if dataB, ok := resp.Data.([]byte); ok {
					dataBytes = dataB
				} else if protoMsg, ok := resp.Data.(proto.Message); ok {
					dataBytes, err = proto.Marshal(protoMsg)
					if err != nil {
						return nil, fmt.Errorf("marshal data as Protobuf failed: %w", err)
					}
				} else {
					return nil, fmt.Errorf("DataCodec is PROTOBUF but Data is not proto.Message or []byte (got %T)", resp.Data)
				}
			default:
				return nil, fmt.Errorf("unsupported DataCodec: %v", resp.DataCodec)
			}
		} else {
			if dataB, ok := resp.Data.([]byte); ok {
				dataBytes = dataB
				dataCodec = pb.PayloadCodec_PAYLOAD_CODEC_RAW
			} else if protoMsg, ok := resp.Data.(proto.Message); ok {
				dataBytes, err = proto.Marshal(protoMsg)
				if err != nil {
					return nil, fmt.Errorf("marshal data failed: %w", err)
				}
				dataCodec = pb.PayloadCodec_PAYLOAD_CODEC_PROTOBUF
			} else {
				dataBytes, err = json.Marshal(resp.Data)
				if err != nil {
					return nil, fmt.Errorf("marshal data failed: %w", err)
				}
				dataCodec = pb.PayloadCodec_PAYLOAD_CODEC_JSON
			}
		}
	}

	wireResp := struct {
		ID         uint64            `json:"id"`
		Data       []byte            `json:"data,omitempty"`
		Error      *protocol.Error   `json:"error,omitempty"`
		Metadata   map[string]string `json:"metadata,omitempty"`
		ServerTime int64             `json:"server_time,omitempty"`
		DataCodec  pb.PayloadCodec   `json:"data_codec,omitempty"`
	}{
		ID:         resp.ID,
		Data:       dataBytes,
		Error:      resp.Error,
		ServerTime: resp.ServerTime,
		DataCodec:  dataCodec,
	}

	if resp.Metadata != nil {
		wireResp.Metadata = resp.Metadata.ToMap()
	}

	return json.Marshal(wireResp)
}

func (c *JSONCodec) decodeResponse(data []byte, resp *protocol.Response) error {
	wireResp := struct {
		ID         uint64            `json:"id"`
		Data       []byte            `json:"data,omitempty"`
		Error      *protocol.Error   `json:"error,omitempty"`
		Metadata   map[string]string `json:"metadata,omitempty"`
		ServerTime int64             `json:"server_time,omitempty"`
		DataCodec  pb.PayloadCodec   `json:"data_codec,omitempty"`
	}{}

	if err := json.Unmarshal(data, &wireResp); err != nil {
		return err
	}

	resp.ID = wireResp.ID
	resp.ServerTime = wireResp.ServerTime
	resp.Error = wireResp.Error
	resp.DataCodec = wireResp.DataCodec

	if len(wireResp.Data) > 0 {
		resp.Data = wireResp.Data
	}

	if wireResp.Metadata != nil {
		resp.Metadata = protocol.FromMap(wireResp.Metadata)
	} else {
		resp.Metadata = protocol.NewMetadata()
	}

	return nil
}

func (c *JSONCodec) Name() string {
	return "json"
}

func init() {
	Register(protocol.CodecTypeJSON, NewJSONCodec())
}
