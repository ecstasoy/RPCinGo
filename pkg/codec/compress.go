// Kunhua Huang 2025

package codec

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"

	"github.com/ecstasoy/RPCinGo/pkg/protocol"
)

// Compressor compresses and decompresses raw payload bytes.
type Compressor interface {
	Compress(data []byte) ([]byte, error)
	Decompress(data []byte) ([]byte, error)
	Name() string
}

// NoneCompressor is a pass-through Compressor that leaves bytes unchanged.
type NoneCompressor struct{}

var _ Compressor = (*NoneCompressor)(nil)

// NewNoneCompressor returns a Compressor that performs no compression.
func NewNoneCompressor() Compressor {
	return &NoneCompressor{}
}

// Compress returns data unchanged.
func (c *NoneCompressor) Compress(data []byte) ([]byte, error) {
	return data, nil
}

// Decompress returns data unchanged.
func (c *NoneCompressor) Decompress(data []byte) ([]byte, error) {
	return data, nil
}

// Name returns the compressor name.
func (c *NoneCompressor) Name() string {
	return "none"
}

// ------------------ Gzip Compressor ------------------

// GzipCompressor compresses payloads with gzip at the configured level.
type GzipCompressor struct {
	Level int
}

var _ Compressor = (*GzipCompressor)(nil)

// NewGzipCompressor returns a gzip-backed Compressor using level.
func NewGzipCompressor(level int) Compressor {
	return &GzipCompressor{Level: level}
}

// Compress gzips data.
func (c *GzipCompressor) Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer

	writer, err := gzip.NewWriterLevel(&buf, c.Level)
	if err != nil {
		return nil, fmt.Errorf("create gzip writer failed: %w", err)
	}

	if _, err := writer.Write(data); err != nil {
		_ = writer.Close()
		return nil, fmt.Errorf("gzip write failed: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("gzip close failed: %w", err)
	}

	return buf.Bytes(), nil
}

// Decompress ungzips data.
func (c *GzipCompressor) Decompress(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create gzip reader failed: %w", err)
	}
	defer func() { _ = reader.Close() }()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("gzip read failed: %w", err)
	}

	return decompressed, nil
}

// Name returns the compressor name.
func (c *GzipCompressor) Name() string {
	return "gzip"
}

// ----------------- Registry -----------------

var compressorRegistry = make(map[protocol.CompressType]Compressor)

// RegisterCompressor installs compressor for typ and panics if compressor is
// nil or typ is already registered.
func RegisterCompressor(typ protocol.CompressType, compressor Compressor) {
	if compressor == nil {
		panic(fmt.Sprintf("compressor: Register compressor is nil for type %s", typ))
	}

	if _, exists := compressorRegistry[typ]; exists {
		panic(fmt.Sprintf("compressor: Register called twice for type %s", typ))
	}

	compressorRegistry[typ] = compressor
}

// GetCompressor returns the compressor registered for typ, or nil if none was
// registered.
func GetCompressor(typ protocol.CompressType) Compressor {
	return compressorRegistry[typ]
}

// GetCompressorOrNone returns the registered compressor for typ or the no-op
// compressor when typ is unknown.
func GetCompressorOrNone(typ protocol.CompressType) Compressor {
	compressor := GetCompressor(typ)
	if compressor == nil {
		compressor = GetCompressor(protocol.CompressTypeNone)
	}
	return compressor
}

func init() {
	RegisterCompressor(protocol.CompressTypeNone, NewNoneCompressor())
	RegisterCompressor(protocol.CompressTypeGzip, NewGzipCompressor(gzip.DefaultCompression))
}
