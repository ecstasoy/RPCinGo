// Kunhua Huang 2025

package codec

import (
	"fmt"
	"io"
	"sync"

	"github.com/ecstasoy/RPCinGo/pkg/protocol"
)

// Codec encodes and decodes request or response payloads without transport
// framing.
type Codec interface {
	Encode(v interface{}) ([]byte, error)
	Decode(data []byte, v interface{}) error
	Name() string
}

// StreamCodec extends Codec-style serialization with its own reader/writer
// framing.
type StreamCodec interface {
	EncodeToWriter(w io.Writer, v interface{}) error
	DecodeFromReader(r io.Reader, v interface{}) error
}

var registry = struct {
	codecs map[protocol.CodecType]Codec
	sync.RWMutex
}{
	codecs: make(map[protocol.CodecType]Codec),
}

// Register installs codec for typ and panics if codec is nil or typ is already
// registered.
func Register(typ protocol.CodecType, codec Codec) {
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

// Get returns the codec registered for typ, or nil if none was registered.
func Get(typ protocol.CodecType) Codec {
	registry.RLock()
	defer registry.RUnlock()

	return registry.codecs[typ]
}

// GetOrDefault returns the codec registered for typ, falling back to the JSON
// codec when typ is unknown.
func GetOrDefault(typ protocol.CodecType) Codec {
	codec := Get(typ)
	if codec == nil {
		codec = Get(protocol.CodecTypeJSON)
	}
	return codec
}

// List returns the registered codec types.
func List() []protocol.CodecType {
	registry.RLock()
	defer registry.RUnlock()

	types := make([]protocol.CodecType, 0, len(registry.codecs))
	for typ := range registry.codecs {
		types = append(types, typ)
	}
	return types
}

// ----------------- Compressed Codec -----------------

// CompressedCodec wraps another Codec and applies a Compressor around encoded
// bytes.
type CompressedCodec struct {
	codec      Codec
	compressor Compressor
}

// NewCompressedCodec returns a Codec that compresses encoded bytes and
// decompresses them before decoding.
func NewCompressedCodec(codec Codec, compressor Compressor) Codec {
	return &CompressedCodec{
		codec:      codec,
		compressor: compressor,
	}
}

// Encode serializes v with the wrapped codec and then compresses the result.
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

// Decode decompresses data and then decodes it with the wrapped codec.
func (c *CompressedCodec) Decode(data []byte, v interface{}) error {
	decompressed, err := c.compressor.Decompress(data)
	if err != nil {
		return fmt.Errorf("decompress failed: %w", err)
	}

	return c.codec.Decode(decompressed, v)
}

// Name returns a combined codec/compressor name.
func (c *CompressedCodec) Name() string {
	return fmt.Sprintf("%s+%s", c.codec.Name(), c.compressor.Name())
}
