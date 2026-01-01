package codec

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"

	"github.com/ecstasoy/RPCinGo/pkg/protocol"
	"github.com/ecstasoy/RPCinGo/pkg/protocol/pb"
	"google.golang.org/protobuf/proto"
)

type ProtobufCodec struct{}

var _ Codec = (*ProtobufCodec)(nil)
var _ StreamCodec = (*ProtobufCodec)(nil)

func NewProtobufCodec() Codec {
	return &ProtobufCodec{}
}

func (c *ProtobufCodec) Encode(v interface{}) ([]byte, error) {
	if msg, ok := v.(proto.Message); ok {
		return proto.Marshal(msg)
	}

	if req, ok := v.(*protocol.Request); ok {
		pbReq, err := c.requestToProto(req)
		if err != nil {
			return nil, fmt.Errorf("convert request to proto failed: %w", err)
		}
		return proto.Marshal(pbReq)
	}

	if resp, ok := v.(*protocol.Response); ok {
		pbResp, err := c.responseToProto(resp)
		if err != nil {
			return nil, fmt.Errorf("convert response to proto failed: %w", err)
		}
		return proto.Marshal(pbResp)
	}

	return nil, fmt.Errorf("protobuf codec: unsupported type %T", v)
}

func (c *ProtobufCodec) Decode(data []byte, v interface{}) error {
	if msg, ok := v.(proto.Message); ok {
		return proto.Unmarshal(data, msg)
	}

	if req, ok := v.(*protocol.Request); ok {
		pbReq := &pb.Request{}
		if err := proto.Unmarshal(data, pbReq); err != nil {
			return fmt.Errorf("unmarshal proto response failed: %w", err)
		}

		if err := c.protoToRequest(pbReq, req); err != nil {
			return fmt.Errorf("convert proto to response failed: %w", err)
		}
		return nil
	}

	if resp, ok := v.(*protocol.Response); ok {
		pbResp := &pb.Response{}
		if err := proto.Unmarshal(data, pbResp); err != nil {
			return fmt.Errorf("unmarshal proto response failed: %w", err)
		}

		if err := c.protoToResponse(pbResp, resp); err != nil {
			return fmt.Errorf("convert proto to response failed: %w", err)
		}
		return nil
	}

	return fmt.Errorf("protobuf codec: undupported type %T", v)
}

func (c *ProtobufCodec) requestToProto(req *protocol.Request) (*pb.Request, error) {
	var argsData []byte
	var err error
	if req.Args != nil {
		argsData, err = json.Marshal(req.Args)
		if err != nil {
			return nil, fmt.Errorf("marshal args failed: %w", err)
		}
	}

	var metadata map[string]string
	if req.Metadata != nil {
		metadata = req.Metadata.ToMap()
	}

	return &pb.Request{
		Id:             req.ID,
		Service:        req.Service,
		Method:         req.Method,
		ServiceVersion: req.ServiceVersion,
		Args:           argsData,
		Timeout:        req.Timeout,
		IsStream:       req.IsStream,
		Metadata:       metadata,
		CreatedAt:      req.CreatedAt,
	}, nil
}

func (c *ProtobufCodec) protoToRequest(pbReq *pb.Request, req *protocol.Request) error {
	req.ID = pbReq.Id
	req.Service = pbReq.Service
	req.Method = pbReq.Method
	req.ServiceVersion = pbReq.ServiceVersion
	req.Timeout = pbReq.Timeout
	req.IsStream = pbReq.IsStream
	req.CreatedAt = pbReq.CreatedAt

	if len(pbReq.Args) > 0 {
		var args interface{}
		if err := json.Unmarshal(pbReq.Args, &args); err != nil {
			return fmt.Errorf("unmarshal args failed: %w", err)
		}
		req.Args = args
	}

	if pbReq.Metadata != nil {
		req.Metadata = protocol.FromMap(pbReq.Metadata)
	} else {
		req.Metadata = protocol.NewMetadata()
	}

	return nil
}

func (c *ProtobufCodec) responseToProto(resp *protocol.Response) (*pb.Response, error) {
	pbResp := &pb.Response{
		Id:         resp.ID,
		ServerTime: resp.ServerTime,
	}

	if resp.Data != nil {
		dataBytes, err := json.Marshal(resp.Data)
		if err != nil {
			return nil, fmt.Errorf("marshal data failed: %w", err)
		}
		pbResp.Data = dataBytes
	}

	if resp.Error != nil {
		pbResp.Error = &pb.Error{
			Code:    resp.Error.Code,
			Message: resp.Error.Message,
			Details: resp.Error.Details,
		}
	}

	if resp.Metadata != nil {
		pbResp.Metadata = resp.Metadata.ToMap()
	}

	return pbResp, nil
}

func (c *ProtobufCodec) protoToResponse(pbResp *pb.Response, resp *protocol.Response) error {
	resp.ID = pbResp.Id
	resp.ServerTime = pbResp.ServerTime

	if len(pbResp.Data) > 0 {
		var data interface{}
		if err := json.Unmarshal(pbResp.Data, &data); err != nil {
			return fmt.Errorf("unmarshal data failed: %w", err)
		}
		resp.Data = data
	}

	if pbResp.Error != nil {
		resp.Error = &protocol.Error{
			Code:    pbResp.Error.Code,
			Message: pbResp.Error.Message,
			Details: pbResp.Error.Details,
		}
	}

	if pbResp.Metadata != nil {
		resp.Metadata = protocol.FromMap(pbResp.Metadata)
	} else {
		resp.Metadata = protocol.NewMetadata()
	}

	return nil
}

func (c *ProtobufCodec) EncodeToWriter(w io.Writer, v interface{}) error {
	data, err := c.Encode(v)
	if err != nil {
		return fmt.Errorf("encode failed: %w", err)
	}

	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, uint32(len(data)))

	if _, err := w.Write(lengthBuf); err != nil {
		return fmt.Errorf("write length failed: %w", err)
	}

	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write data failed: %w", err)
	}

	return nil
}

func (c *ProtobufCodec) DecodeFromReader(r io.Reader, v interface{}) error {
	lengthBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lengthBuf); err != nil {
		return fmt.Errorf("read length failed: %w", err)
	}

	length := binary.BigEndian.Uint32(lengthBuf)

	if length > 100*1024*1024 {
		return fmt.Errorf("message too large: %d bytes", length)
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return fmt.Errorf("read data failed: %w", err)
	}

	return c.Decode(data, v)
}

func (c *ProtobufCodec) Name() string {
	return "protobuf"
}

func init() {
	Register(CodecTypeProtobuf, NewProtobufCodec())
}
