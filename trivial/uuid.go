package trivial

import (
	"crypto/rand"
	"fmt"
	"io"
)

// UUID represent a universal identifier with 16 octets.
type UUID [16]byte

// NewUUID generates a new version 4 UUID relying only on random numbers.
func NewUUID() (UUID, error) {
	uuid := UUID{}
	if _, err := io.ReadFull(rand.Reader, []byte(uuid[0:16])); err != nil {
		return UUID{}, err
	}
	// Set version (4) and variant (2) according to RfC 4122.
	var version byte = 4 << 4
	var variant byte = 8 << 4
	uuid[6] = version | (uuid[6] & 15)
	uuid[8] = variant | (uuid[8] & 15)
	return uuid, nil
}

// Copy returns a copy of the UUID.
func (uuid UUID) Copy() UUID {
	raw := uuid.Raw()
	return UUID(raw)
}

// Raw returns a copy of the UUID bytes.
func (uuid UUID) Raw() [16]byte {
	var raw [16]byte
	copy([]byte(raw[0:16]), []byte(uuid[0:16]))
	return raw
}

// String returns a hexadecimal string representation with
// standardized separators.
func (uuid UUID) String() string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}
