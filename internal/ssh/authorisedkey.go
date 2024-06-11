package ssh

import (
	"fmt"

	"golang.org/x/crypto/ssh"
)

// AuthorisedKey represents a single authorised key line that would commonly be
// found in a authorized_keys file. http://man.he.net/man5/authorized_keys
type AuthorisedKey struct {
	// Key holds the parse key data for the public key.
	Key ssh.PublicKey

	// Comment is the comment string attached to the authorised key.
	Comment string
}

// Fingerprint returns the SHA256 fingerprint of the public key.
func (a *AuthorisedKey) Fingerprint() string {
	return ssh.FingerprintSHA256(a.Key)
}

// ParseAuthorisedKey parses a single line from an authorised keys file
// returning a [AuthorisedKey] representation of the data.
// [ssh.ParseAuthorizedKey] is used to perform the underlying validating and
// parsing.
func ParseAuthorisedKey(key string) (AuthorisedKey, error) {
	parsedKey, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(key))
	if err != nil {
		return AuthorisedKey{}, fmt.Errorf("parsing authorised key %q: %w", err)
	}

	return AuthorisedKey{
		Key:     parsedKey,
		Comment: comment,
	}, nil
}
