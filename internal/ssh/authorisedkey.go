// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"fmt"

	"golang.org/x/crypto/ssh"
)

// PublicKey represents a single authorised key line that would commonly be
// found in a authorized_keys file. http://man.he.net/man5/authorized_keys
type PublicKey struct {
	// Key holds the parse key data for the public key.
	Key ssh.PublicKey

	// Comment is the comment string attached to the authorised key.
	Comment string
}

// Fingerprint returns the SHA256 fingerprint of the public key.
func (a *PublicKey) Fingerprint() string {
	return ssh.FingerprintSHA256(a.Key)
}

// ParsePublicKey parses a single line from an authorised keys file
// returning a [PublicKey] representation of the data.
// [ssh.ParseAuthorizedKey] is used to perform the underlying validating and
// parsing.
func ParsePublicKey(key string) (PublicKey, error) {
	parsedKey, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(key))
	if err != nil {
		return PublicKey{}, fmt.Errorf("parsing public key %q: %w", key, err)
	}

	return PublicKey{
		Key:     parsedKey,
		Comment: comment,
	}, nil
}
