// Kunhua Huang 2025

package codec

import (
	"encoding/json"
	"fmt"
	"io"
)

type JSONCodec struct{}

var _ Codec = (*JSONCodec)(nil)
var _ StreamCodec = (*JSONCodec)(nil)

func NewJSONCodec() Codec {
	return &JSONCodec{}
}

func (c *JSONCodec) Encode(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func (c *JSONCodec) Decode(data []byte, v interface{}) error {
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

func (c *JSONCodec) Name() string {
	return "json"
}

func init() {
	Register(CodecTypeJSON, NewJSONCodec())
}
