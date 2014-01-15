// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/utils"
)

var logger = loggo.GetLogger("juju.utils.ssh")

type ListMode bool

var (
	FullKeys     ListMode = true
	Fingerprints ListMode = false
)

const (
	authKeysDir  = "~%s/.ssh"
	authKeysFile = "authorized_keys"
)

// case-insensive public authorized_keys options
var validOptions = [...]string{
	"cert-authority",
	"command",
	"environment",
	"from",
	"no-agent",
	"no-port-forwarding",
	"no-pty",
	"no-user",
	"no-X11-forwarding",
	"permitopen",
	"principals",
	"tunnel",
}

var validKeytypes = [...]string{
	"ecdsa-sha2-nistp256",
	"ecdsa-sha2-nistp384",
	"ecdsa-sha2-nistp521",
	"ssh-dss",
	"ssh-rsa",
}

type AuthorisedKey struct {
	KeyType string
	Key     []byte
	Comment string
}

// skipOptions takes a non-comment line from an
// authorized_keys file, and returns the remainder
// of the line after skipping any options at the
// beginning of the line.
func skipOptions(line string) string {
	found := false
	lower := strings.ToLower(line)
	for _, o := range validOptions {
		if strings.HasPrefix(lower, o) {
			line = line[len(o):]
			found = true
			break
		}
	}
	if !found {
		return line
	}
	// Skip to the next unquoted whitespace, returning the remainder.
	// Double quotes may be escaped with \".
	var quoted bool
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case ' ', '\t':
			if !quoted {
				return strings.TrimLeft(line[i+1:], " \t")
			}
		case '\\':
			if i+1 < len(line) && line[i+1] == '"' {
				i++
			}
		case '"':
			quoted = !quoted
		}
	}
	return ""
}

// ParseAuthorisedKey parses a non-comment line from an
// authorized_keys file and returns the constituent parts.
// Based on description in "man sshd".
//
// TODO(axw) support version 1 format?
func ParseAuthorisedKey(line string) (*AuthorisedKey, error) {
	withoutOptions := skipOptions(line)
	var keytype, key, comment string
	if i := strings.IndexAny(withoutOptions, " \t"); i == -1 {
		// There must be at least two fields: keytype and key.
		return nil, fmt.Errorf("malformed line: %q", line)
	} else {
		keytype = withoutOptions[:i]
		key = strings.TrimSpace(withoutOptions[i+1:])
	}
	validKeytype := false
	for _, kt := range validKeytypes {
		if keytype == kt {
			validKeytype = true
			break
		}
	}
	if !validKeytype {
		return nil, fmt.Errorf("invalid keytype %q in line %q", keytype, line)
	}
	// Split key/comment (if any)
	if i := strings.IndexAny(key, " \t"); i != -1 {
		key, comment = key[:i], key[i+1:]
	}
	keyBytes, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return nil, err
	}
	return &AuthorisedKey{
		KeyType: keytype,
		Key:     keyBytes,
		Comment: comment,
	}, nil
}

// SplitAuthorisedKeys extracts a key slice from the specified key data,
// by splitting the key data into lines and ignoring comments and blank lines.
func SplitAuthorisedKeys(keyData string) []string {
	var keys []string
	for _, key := range strings.Split(string(keyData), "\n") {
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

func readAuthorisedKeys(username string) ([]string, error) {
	keyDir := fmt.Sprintf(authKeysDir, username)
	sshKeyFile, err := utils.NormalizePath(filepath.Join(keyDir, authKeysFile))
	if err != nil {
		return nil, err
	}
	logger.Debugf("reading authorised keys file %s", sshKeyFile)
	keyData, err := ioutil.ReadFile(sshKeyFile)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading ssh authorised keys file: %v", err)
	}
	var keys []string
	for _, key := range strings.Split(string(keyData), "\n") {
		if len(strings.Trim(key, " \r")) == 0 {
			continue
		}
		keys = append(keys, key)
	}
	return keys, nil
}

func writeAuthorisedKeys(username string, keys []string) error {
	keyDir := fmt.Sprintf(authKeysDir, username)
	keyDir, err := utils.NormalizePath(keyDir)
	if err != nil {
		return err
	}
	err = os.MkdirAll(keyDir, os.FileMode(0755))
	if err != nil {
		return fmt.Errorf("cannot create ssh key directory: %v", err)
	}
	keyData := strings.Join(keys, "\n") + "\n"

	// Get perms to use on auth keys file
	sshKeyFile := filepath.Join(keyDir, authKeysFile)
	perms := os.FileMode(0644)
	info, err := os.Stat(sshKeyFile)
	if err == nil {
		perms = info.Mode().Perm()
	}
	// Write the data to a temp file
	tempDir, err := ioutil.TempDir(keyDir, "")
	if err != nil {
		return err
	}
	tempFile := filepath.Join(tempDir, "newkeyfile")
	defer os.RemoveAll(tempDir)
	err = ioutil.WriteFile(tempFile, []byte(keyData), perms)
	if err != nil {
		return err
	}

	// Rename temp file to the final location and ensure its owner
	// is set correctly.
	logger.Debugf("writing authorised keys file %s", sshKeyFile)
	// TODO (wallyworld) - what to do on windows (if anything)
	if runtime.GOOS != "windows" {
		// Ensure the resulting authorised keys file has its ownership
		// set to the specified username.
		var u *user.User
		if username == "" {
			u, err = user.Current()
		} else {
			u, err = user.Lookup(username)
		}
		if err != nil {
			return err
		}
		// chown requires ints but user.User has strings for windows.
		uid, err := strconv.Atoi(u.Uid)
		if err != nil {
			return err
		}
		gid, err := strconv.Atoi(u.Gid)
		if err != nil {
			return err
		}
		err = os.Chown(tempFile, uid, gid)
		if err != nil {
			return err
		}
	}
	return os.Rename(tempFile, sshKeyFile)
}

// We need a mutex because updates to the authorised keys file are done by
// reading the contents, updating, and writing back out. So only one caller
// at a time can use either Add, Delete, List.
var mutex sync.Mutex

// AddKeys adds the specified ssh keys to the authorized_keys file for user.
// Returns an error if there is an issue with *any* of the supplied keys.
func AddKeys(user string, newKeys ...string) error {
	mutex.Lock()
	defer mutex.Unlock()
	existingKeys, err := readAuthorisedKeys(user)
	if err != nil {
		return err
	}
	for _, newKey := range newKeys {
		fingerprint, comment, err := KeyFingerprint(newKey)
		if err != nil {
			return err
		}
		if comment == "" {
			return fmt.Errorf("cannot add ssh key without comment")
		}
		for _, key := range existingKeys {
			existingFingerprint, existingComment, err := KeyFingerprint(key)
			if err != nil {
				// Only log a warning if the unrecognised key line is not a comment.
				if key[0] != '#' {
					logger.Warningf("invalid existing ssh key %q: %v", key, err)
				}
				continue
			}
			if existingFingerprint == fingerprint {
				return fmt.Errorf("cannot add duplicate ssh key: %v", fingerprint)
			}
			if existingComment == comment {
				return fmt.Errorf("cannot add ssh key with duplicate comment: %v", comment)
			}
		}
	}
	sshKeys := append(existingKeys, newKeys...)
	return writeAuthorisedKeys(user, sshKeys)
}

// DeleteKeys removes the specified ssh keys from the authorized ssh keys file for user.
// keyIds may be either key comments or fingerprints.
// Returns an error if there is an issue with *any* of the keys to delete.
func DeleteKeys(user string, keyIds ...string) error {
	mutex.Lock()
	defer mutex.Unlock()
	existingKeyData, err := readAuthorisedKeys(user)
	if err != nil {
		return err
	}
	// Build up a map of keys indexed by fingerprint, and fingerprints indexed by comment
	// so we can easily get the key represented by each keyId, which may be either a fingerprint
	// or comment.
	var keysToWrite []string
	var sshKeys = make(map[string]string)
	var keyComments = make(map[string]string)
	for _, key := range existingKeyData {
		fingerprint, comment, err := KeyFingerprint(key)
		if err != nil {
			logger.Debugf("keeping unrecognised existing ssh key %q: %v", key, err)
			keysToWrite = append(keysToWrite, key)
			continue
		}
		sshKeys[fingerprint] = key
		if comment != "" {
			keyComments[comment] = fingerprint
		}
	}
	for _, keyId := range keyIds {
		// assume keyId may be a fingerprint
		fingerprint := keyId
		_, ok := sshKeys[keyId]
		if !ok {
			// keyId is a comment
			fingerprint, ok = keyComments[keyId]
		}
		if !ok {
			return fmt.Errorf("cannot delete non existent key: %v", keyId)
		}
		delete(sshKeys, fingerprint)
	}
	for _, key := range sshKeys {
		keysToWrite = append(keysToWrite, key)
	}
	if len(keysToWrite) == 0 {
		return fmt.Errorf("cannot delete all keys")
	}
	return writeAuthorisedKeys(user, keysToWrite)
}

// ReplaceKeys writes the specified ssh keys to the authorized_keys file for user,
// replacing any that are already there.
// Returns an error if there is an issue with *any* of the supplied keys.
func ReplaceKeys(user string, newKeys ...string) error {
	mutex.Lock()
	defer mutex.Unlock()

	existingKeyData, err := readAuthorisedKeys(user)
	if err != nil {
		return err
	}
	var existingNonKeyLines []string
	for _, line := range existingKeyData {
		_, _, err := KeyFingerprint(line)
		if err != nil {
			existingNonKeyLines = append(existingNonKeyLines, line)
		}
	}
	for _, newKey := range newKeys {
		_, comment, err := KeyFingerprint(newKey)
		if err != nil {
			return err
		}
		if comment == "" {
			return fmt.Errorf("cannot add ssh key without comment")
		}
	}
	return writeAuthorisedKeys(user, append(existingNonKeyLines, newKeys...))
}

// ListKeys returns either the full keys or key comments from the authorized ssh keys file for user.
func ListKeys(user string, mode ListMode) ([]string, error) {
	mutex.Lock()
	defer mutex.Unlock()
	keyData, err := readAuthorisedKeys(user)
	if err != nil {
		return nil, err
	}
	var keys []string
	for _, key := range keyData {
		fingerprint, comment, err := KeyFingerprint(key)
		if err != nil {
			// Only log a warning if the unrecognised key line is not a comment.
			if key[0] != '#' {
				logger.Warningf("ignoring invalid ssh key %q: %v", key, err)
			}
			continue
		}
		if mode == FullKeys {
			keys = append(keys, key)
		} else {
			shortKey := fingerprint
			if comment != "" {
				shortKey += fmt.Sprintf(" (%s)", comment)
			}
			keys = append(keys, shortKey)
		}
	}
	return keys, nil
}

// Any ssh key added to the authorised keys list by Juju will have this prefix.
// This allows Juju to know which keys have been added externally and any such keys
// will always be retained by Juju when updating the authorised keys file.
const JujuCommentPrefix = "Juju:"

func EnsureJujuComment(key string) string {
	ak, err := ParseAuthorisedKey(key)
	// Just return an invalid key as is.
	if err != nil {
		logger.Warningf("invalid Juju ssh key %s: %v", key, err)
		return key
	}
	if ak.Comment == "" {
		return key + " " + JujuCommentPrefix + "sshkey"
	} else {
		// Add the Juju prefix to the comment if necessary.
		if !strings.HasPrefix(ak.Comment, JujuCommentPrefix) {
			commentIndex := strings.LastIndex(key, ak.Comment)
			return key[:commentIndex] + JujuCommentPrefix + ak.Comment
		}
	}
	return key
}
