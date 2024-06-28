// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"fmt"
	"strings"

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

// SplitAuthorisedKeys extracts a key slice from the specified key data,
// by splitting the key data into lines and ignoring comments and blank lines.
func SplitAuthorisedKeys(keyData string) []string {
	keys := []string{}
	for _, key := range strings.Split(keyData, "\n") {
		key = strings.Trim(key, " \r")
		if len(key) == 0 {
			continue
		}
		if key[0] == '#' {
			continue
		}
		keys = append(keys, key)
	}
	return keys
}
