// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/utils/v3"
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

	// directoryUserLocalSSH is the directory name inside of a users home
	// directory where we expect to find ssh keys and configuration.
	directoryUserLocalSSH = ".ssh"
)

// commonPublicKeyFileNames returns a set of common filenames for public keys
// that can be found inside of a Juju users .ssh directory.
func commonPublicKeyFileNames() []string {
	return []string{
		"id_ed25519.pub",
		"id_ecdsa.pub",
		"id_rsa.pub",
		"identity.pub",
	}
}

// GetFileSystemPublicKeys will attempt to find all ssh public keys at the root
// of the file system and read them all into a slice of public ssh keys. No
// attempt is made to assert if a file contains a valid public key.
//
// Public keys are found based on the file in the file system ending in a ".pub"
// suffix. If a file is read and a permission error or file not found error
// occurs this function will simply move on and not report the problem upwards.
// This is a best effort approach.
//
// This function is useful for reading the public keys found in a users juju ssh
// directory. See [github.com/juju/juju/juju/osenv.JujuXDGDataSSHFS].
func GetFileSystemPublicKeys(
	ctx context.Context,
	fileSystem fs.FS,
) ([]string, error) {
	pattern := "*" + publicKeyFileSuffix
	fileNames, err := fs.Glob(fileSystem, pattern)
	if err != nil {
		return nil, fmt.Errorf(
			"finding all files in file system that matches %q: %w",
			pattern,
			err,
		)
	}

	keys := []string{}
	for _, fileName := range fileNames {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		key, err := readPublicKeyFile(fileSystem, fileName)
		if errors.Is(err, fs.ErrPermission) ||
			errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("attempting to read public key %q from filesystem", fileName)
		}

		keys = append(keys, key)
	}

	return keys, nil
}

// readKeyFile is responsible for reading the file pointed at by file name from
// the file system and processing the data as an ssh public key.
//
// No attempt is made to verify the contents of the file is a valid ssh public
// key.
func readPublicKeyFile(fileSystem fs.FS, fileName string) (string, error) {
	data, err := fs.ReadFile(fileSystem, fileName)
	if err != nil {
		return "", fmt.Errorf("reading public key file %q: %w", fileName, err)
	}

	return trimPublicKey(string(data), ""), nil
}

// GetCommonUserPublicKeys is responsible for attempting to load common public
// key files from the supplied file system. See [LocalUserSSHFileSystem] for
// accessing the users ssh file system.
//
// The files target by this function are:
// - id_ed25519.pub
// - id_ecdsa.pub
// - id_rsa.pub
// - identity.pub
//
// No attempt is made to verify the contents of each file is a valid ssh public
// key. Access errors to files such as permission or not found are ignored. Any
// other read based errors are returned.
func GetCommonUserPublicKeys(
	ctx context.Context,
	fileSystem fs.FS,
) ([]string, error) {
	keys := []string{}
	for _, fileName := range commonPublicKeyFileNames() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		key, err := readPublicKeyFile(fileSystem, fileName)
		if errors.Is(err, fs.ErrPermission) ||
			errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("attempting to read public key %q from filesystem", fileName)
		}

		keys = append(keys, key)
	}

	return keys, nil
}

// LocalUserSSHFileSystem returns a file system rooted at the local users .ssh
// directory.
func LocalUserSSHFileSystem() fs.FS {
	return os.DirFS(filepath.Join(utils.Home(), directoryUserLocalSSH))
}

// trimPublicKey is responsible for removing supurfulus data from public keys
// read from files. extra runes to trim can be supplied in the extra argument.
func trimPublicKey(key string, extra string) string {
	return strings.Trim(key, " 	\r\n"+extra)
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
