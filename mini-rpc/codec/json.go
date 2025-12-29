//Kunhua Huang, 12/29/2025

package codec

import (
	"encoding/json"
	"io"
)

type JSONCodec struct{}

var _ Codec = (*JSONCodec)(nil)

func NewJSONCodec() Codec {
	return &JSONCodec{}
}

func (c *JSONCodec) Encode(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func (c *JSONCodec) Decode(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// ------------------- StreamCodec -------------------//
var _ StreamCodec = (*JSONCodec)(nil)

func (c *JSONCodec) EncodeToWriter(w io.Writer, v interface{}) error {
	encoder := json.NewEncoder(w)
	return encoder.Encode(v)
}

func (c *JSONCodec) DecodeFromReader(r io.Reader, v interface{}) error {
	decoder := json.NewDecoder(r)
	return decoder.Decode(v)
}

func init() {
	RegisterCodec(JSONCodecType, NewJSONCodec())
}
