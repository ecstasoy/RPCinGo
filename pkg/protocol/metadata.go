package protocol

type Metadata map[string]string

// Metadata keys
const (
	MetaKeyTraceID = "trace-id"
	MetaKeySpanID  = "span-id"

	MetaKeyToken  = "auth-token"
	MetaKeyUserID = "user-id"

	MetaKeyRegion = "region"
	MetaKeyZone   = "zone"

	MetaKeyDebug = "debug"
)

func NewMetadata() Metadata {
	return make(Metadata)
}

func (m Metadata) Set(key, value string) {
	m[key] = value
}

func (m Metadata) Get(key string) (string, bool) {
	val, ok := m[key]
	return val, ok
}

// Clone prevents concurrent map r/w panic
func (m Metadata) Clone() Metadata {
	cloneMap := make(Metadata, len(m))
	for k, v := range m {
		cloneMap[k] = v
	}
	return cloneMap
}

func (m Metadata) ToMap() map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func FromMap(data map[string]string) Metadata {
	if data == nil {
		return nil
	}
	out := make(Metadata, len(data))
	for k, v := range data {
		out[k] = v
	}
	return out
}

func (m Metadata) Merge(other Metadata) {
	if other == nil {
		return
	}

	for k, v := range other {
		m[k] = v
	}
}
