// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
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

const (
	// publicKeyFileSuffix is the suffix Juju expects public ssh keys to have.
	publicKeyFileSuffix = ".pub"
)

// PublicKeysForPrivateKeyFiles returns the public keys paired with the given
// private key files.
func PublicKeysForPrivateKeyFiles(privateKeyFiles []string) ([]string, error) {
	keys := make([]string, 0, len(privateKeyFiles))
	for _, privateKeyFile := range privateKeyFiles {
		key, err := os.ReadFile(privateKeyFile + publicKeyFileSuffix)
		if err != nil {
			return nil, fmt.Errorf("reading public key for %q: %w", privateKeyFile, err)
		}
		keys = append(keys, strings.TrimSpace(string(key)))
	}
	return keys, nil
}

// Fingerprint returns the SHA256 fingerprint of the public key.
func (a *PublicKey) Fingerprint() string {
	return ssh.FingerprintSHA256(a.Key)
}

// ParsePublicKey parses a single line from an authorised keys file
// returning a [PublicKey] representation of the data.
// [ssh.ParseAuthorizedKey] is used to perform the underlying validating and
// parsing.
// Data describing more than one key is rejected.
func ParsePublicKey(key string) (PublicKey, error) {
	parsedKey, comment, _, rest, err := ssh.ParseAuthorizedKey([]byte(key))
	if err != nil {
		return PublicKey{}, fmt.Errorf("parsing public key %q: %w", key, err)
	}

	// ParseAuthorizedKey stops at the first key it understands and hands back
	// everything after it. Callers keep the string they gave us and write it
	// out to authorized_keys verbatim, so trailing keys would be authorised
	// while the comment and fingerprint recorded here only describe the first.
	trailing, err := SplitAuthorizedKeysReader(bytes.NewReader(rest))
	if err != nil {
		return PublicKey{}, fmt.Errorf("parsing public key %q: %w", key, err)
	}
	if len(trailing) != 0 {
		return PublicKey{}, fmt.Errorf("public key %q describes more than one key", key)
	}

	return PublicKey{
		Key:     parsedKey,
		Comment: comment,
	}, nil
}

// SplitAuthorizedKeys extracts a key slice from the specified key data,
// by splitting the key data into lines and ignoring comments and blank lines.
//
// No validation is performed on the split keys to make sure they are compliant.
func SplitAuthorizedKeys(keyData string) ([]string, error) {
	return SplitAuthorizedKeysByDelimiter('\n', keyData)
}

// SplitAuthorizedKeysByDelimiter extracts a key slice from the specified key
// data, by splitting the key data into lines separated by delimiter and
// ignoring comments and blank lines.
//
// No validation is performed on the split keys to make sure they are compliant.
func SplitAuthorizedKeysByDelimiter(delimiter byte, keyData string) ([]string, error) {
	return SplitAuthorizedKeysReaderByDelimiter(delimiter, strings.NewReader(keyData))
}

// SplitAuthorizedKeysReaderByDelimiter is responsible for splitting up all of
// the authorized keys contained within the reader into a slice of keys.
//
// Keys in the reader are parsed in accordance with the open ssh authorized_keys
// file format.
//
// Any errors encountered when reading from the reader are returned to the
// caller with exception of [io.EOF].
//
// No validation is performed on the split keys to make sure they are compliant.
func SplitAuthorizedKeysReader(reader io.Reader) ([]string, error) {
	return SplitAuthorizedKeysReaderByDelimiter('\n', reader)
}

// SplitAuthorizedKeysReaderByDelimiter is responsible for splitting up all of
// the authorized keys contained within the reader into a slice of keys. The
// delimiter tells the parser what to use when considering a newline.
//
// Keys in the reader are parsed in accordance with the open ssh authorized_keys
// file format.
//
// Any errors encountered when reading from the reader are returned to the
// caller with exception of [io.EOF].
//
// No validation is performed on the split keys to make sure they are compliant.
func SplitAuthorizedKeysReaderByDelimiter(
	delimiter byte,
	reader io.Reader,
) ([]string, error) {
	// Each line of the file contains one key (empty lines and lines starting
	// with a '#' are ignored as comments)

	rd := bufio.NewReader(reader)

	var (
		err       error
		line      string
		lines     = []string{}
		lineCount = 0
	)

	for {
		if errors.Is(err, io.EOF) {
			break
		}

		lineCount++
		line, err = rd.ReadString(delimiter)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf(
				"reading authorized keys from reader at line %d: %w",
				lineCount,
				err,
			)
		}

		// We need to trim spaces, tabs, carrige returns and the delimiter from
		// the line.
		line = strings.Trim(line, " 	\r"+string(rune(delimiter)))
		if len(line) == 0 {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}

		lines = append(lines, line)
	}

	return lines, nil
}

// MakeAuthorizedKeysString is responsible for turning a slice of public ssh
// keys into a compliant authorized key file string. Keys are written in the
// order they are defined in the keys slice.
//
// No validation is performed on the keys to make sure they are public ssh keys.
func MakeAuthorizedKeysString(keys []string) string {
	builder := strings.Builder{}
	WriteAuthorizedKeys(&builder, keys)
	return builder.String()
}

// WriteAuthorizedKeys is responsible for writing a slice of authorized ssh
// public keys to a write as a standards compliant authorized keys file.
// Keys are written in the order they are defined in the keys slice.
//
// No validation is performed on the keys to make sure they are public ssh keys.
func WriteAuthorizedKeys(writer io.Writer, keys []string) {
	for _, key := range keys {
		fmt.Fprintln(writer, key)
	}
}
