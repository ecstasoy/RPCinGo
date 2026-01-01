// Kunhua Huang 2025

package codec

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"

	"github.com/ecstasoy/RPCinGo/pkg/protocol"
)

type Compressor interface {
	Compress(data []byte) ([]byte, error)
	Decompress(data []byte) ([]byte, error)
	Name() string
}

type NoneCompressor struct{}

var _ Compressor = (*NoneCompressor)(nil)

func NewNoneCompressor() Compressor {
	return &NoneCompressor{}
}

func (c *NoneCompressor) Compress(data []byte) ([]byte, error) {
	return data, nil
}

func (c *NoneCompressor) Decompress(data []byte) ([]byte, error) {
	return data, nil
}

func (c *NoneCompressor) Name() string {
	return "none"
}

// ------------------ Gzip Compressor ------------------

type GzipCompressor struct {
	Level int
}

var _ Compressor = (*GzipCompressor)(nil)

func NewGzipCompressor(level int) Compressor {
	return &GzipCompressor{Level: level}
}

func (c *GzipCompressor) Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer

	writer, err := gzip.NewWriterLevel(&buf, c.Level)
	if err != nil {
		return nil, fmt.Errorf("create gzip writer failed: %w", err)
	}

	if _, err := writer.Write(data); err != nil {
		writer.Close()
		return nil, fmt.Errorf("gzip write failed: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("gzip close failed: %w", err)
	}

	return buf.Bytes(), nil
}

func (c *GzipCompressor) Decompress(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create gzip reader failed: %w", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("gzip read failed: %w", err)
	}

	return decompressed, nil
}

func (c *GzipCompressor) Name() string {
	return "gzip"
}

// ----------------- Registry -----------------

var compressorRegistry = make(map[protocol.CompressType]Compressor)

func RegisterCompressor(typ protocol.CompressType, compressor Compressor) {
	if compressor == nil {
		panic(fmt.Sprintf("compressor: Register compressor is nil for type %s", typ))
	}

	if _, exists := compressorRegistry[typ]; exists {
		panic(fmt.Sprintf("compressor: Register called twice for type %s", typ))
	}

	compressorRegistry[typ] = compressor
}

func GetCompressor(typ protocol.CompressType) Compressor {
	return compressorRegistry[typ]
}

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
