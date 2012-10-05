package trivial

import (
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"io"
	"launchpad.net/juju-core/thirdparty/pbkdf2"
)

var salt = []byte{0x75, 0x82, 0x81, 0xca, 0x4c, 0xed, 0x68, 0x5b, 0xf1, 0x69, 0x5c, 0xc6}

// RandomBytes returns n random bytes.
func RandomBytes(n int) ([]byte, error) {
	buf := make([]byte, n)
	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		return nil, fmt.Errorf("cannot read random bytes: %v", err)
	}
	return buf, nil
}

// PasswordHash returns base64-encoded one-way hash of the provided salt
// and password that is computationally hard to crack by iterating
// through possible passwords.
func PasswordHash(password string) string {
	h := pbkdf2.Key([]byte(password), salt, 8192, 76, sha512.New)
	return base64.StdEncoding.EncodeToString(h)
}
