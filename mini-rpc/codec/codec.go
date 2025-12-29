// Kunhua Huang, 12/29/2025

package codec

import "io"

type Codec interface {
	Encode(v interface{}) ([]byte, error)
	Decode(data []byte, v interface{}) error
}

type StreamCodec interface {
	EncodeToWriter(w io.Writer, v interface{}) error
	DecodeFromReader(r io.Reader, v interface{}) error
}

type CodecType string

const (
	JSONCodecType     CodecType = "json"
	ProtobufCodecType CodecType = "protobuf"
	// More...
)

var codecs = make(map[CodecType]Codec)

func RegisterCodec(codecType CodecType, codec Codec) {
	codecs[codecType] = codec
}

func GetCodec(codecType CodecType) Codec {
	return codecs[codecType]
}
