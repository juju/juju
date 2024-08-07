// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"bufio"
	"errors"
	"fmt"
	"io"
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
