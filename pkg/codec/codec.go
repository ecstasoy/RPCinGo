// Kunhua Huang 2025

package codec

import (
	"fmt"
	"io"
	"sync"
)

type Codec interface {
	Encode(v interface{}) ([]byte, error)
	Decode(data []byte, v interface{}) error
	Name() string
}

type StreamCodec interface {
	EncodeToWriter(w io.Writer, v interface{}) error
	DecodeFromReader(r io.Reader, v interface{}) error
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

var registry = struct {
	codecs map[CodecType]Codec
	sync.RWMutex
}{
	codecs: make(map[CodecType]Codec),
}

func Register(typ CodecType, codec Codec) {
	registry.Lock()
	defer registry.Unlock()

	if codec == nil {
		panic(fmt.Sprintf("codec: Register codec is nil for type %d", typ))
	}

	if _, exists := registry.codecs[typ]; exists {
		panic(fmt.Sprintf("codec: Register called twice for type %d", typ))
	}

	registry.codecs[typ] = codec
}

func Get(typ CodecType) Codec {
	registry.RLock()
	defer registry.RUnlock()

	return registry.codecs[typ]
}

func GetOrDefault(typ CodecType) Codec {
	codec := Get(typ)
	if codec == nil {
		codec = Get(CodecTypeJSON)
	}
	return codec
}

func List() []CodecType {
	registry.RLock()
	defer registry.RUnlock()

	types := make([]CodecType, 0, len(registry.codecs))
	for typ := range registry.codecs {
		types = append(types, typ)
	}
	return types
}

// ----------------- Compressed Codec -----------------

type CompressedCodec struct {
	codec      Codec
	compressor Compressor
}

func NewCompressedCodec(codec Codec, compressor Compressor) Codec {
	return &CompressedCodec{
		codec:      codec,
		compressor: compressor,
	}
}

func (c *CompressedCodec) Encode(v interface{}) ([]byte, error) {
	data, err := c.codec.Encode(v)
	if err != nil {
		return nil, err
	}

	compressed, err := c.compressor.Compress(data)
	if err != nil {
		return nil, fmt.Errorf("compress failed: %w", err)
	}

	return compressed, nil
}

func (c *CompressedCodec) Decode(data []byte, v interface{}) error {
	decompressed, err := c.compressor.Decompress(data)
	if err != nil {
		return fmt.Errorf("decompress failed: %w", err)
	}

	return c.codec.Decode(decompressed, v)
}

func (c *CompressedCodec) Name() string {
	return fmt.Sprintf("%s+%s", c.codec.Name(), c.compressor.Name())
}
