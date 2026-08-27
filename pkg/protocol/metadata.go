package protocol

// Metadata carries string key/value pairs alongside requests and responses.
type Metadata map[string]string

// MetaKeyTraceID and related constants define conventional metadata keys used
// by built-in tracing, auth, and routing features.
const (
	MetaKeyTraceID = "trace-id"
	MetaKeySpanID  = "span-id"

	MetaKeyToken  = "auth-token"
	MetaKeyUserID = "user-id"

	MetaKeyRegion = "region"
	MetaKeyZone   = "zone"

	// MetaKeyHashKey carries the load-balancer affinity key. When present, a
	// consistent-hash balancer routes the request by hashing this value, so the
	// same key reaches the same instance.
	MetaKeyHashKey = "lb-hash-key"

	MetaKeyDebug = "debug"
)

// NewMetadata returns an initialized Metadata map.
func NewMetadata() Metadata {
	return make(Metadata)
}

// Set stores a metadata key/value pair.
func (m Metadata) Set(key, value string) {
	m[key] = value
}

// Get retrieves a metadata value and whether it was present.
func (m Metadata) Get(key string) (string, bool) {
	val, ok := m[key]
	return val, ok
}

// Clone returns a shallow copy that can be mutated independently.
func (m Metadata) Clone() Metadata {
	cloneMap := make(Metadata, len(m))
	for k, v := range m {
		cloneMap[k] = v
	}
	return cloneMap
}

// ToMap returns a shallow copy as a plain map.
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

// FromMap copies data into a new Metadata value.
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

// Merge copies all entries from other into m.
func (m Metadata) Merge(other Metadata) {
	if other == nil {
		return
	}

	for k, v := range other {
		m[k] = v
	}
}
