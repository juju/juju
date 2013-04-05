package trivial

import (
	"crypto/rand"
	"fmt"
	"io"
)

// UUID represent a universal identifier with 16 octets.
type UUID [16]byte

// NewUUID generates a new version 4 UUID relying only on random numbers.
func NewUUID() UUID {
	uuid := UUID{}
	_, err := io.ReadFull(rand.Reader, []byte(uuid[0:16]))
	if err != nil {
		panic(err)
	}
	// Set version (4) and variant (2) according to RfC 4122.
	var version byte = 4 << 4
	var variant byte = 8 << 4
	uuid[6] = version | (uuid[6] & 15)
	uuid[8] = variant | (uuid[8] & 15)
	return uuid
}

// Copy returns a copy of the UUID.
func (uuid UUID) Copy() UUID {
	raw := uuid.Raw()
	return UUID(raw)
}

// Raw returns a copy of the UUID bytes.
func (uuid UUID) Raw() [16]byte {
	var raw [16]byte
	for i := 0; i < 16; i++ {
		raw[i] = uuid[i]
	}
	return raw
}

// String returns a hexadecimal string representation with
// standardized separators.
func (uuid UUID) String() string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}
