// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"code.google.com/p/go.crypto/ssh"
	"github.com/juju/loggo"

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

type AuthorisedKey struct {
	Key     []byte
	Comment string
}

// ParseAuthorisedKey parses a non-comment line from an
// authorized_keys file and returns the constituent parts.
// Based on description in "man sshd".
func ParseAuthorisedKey(line string) (*AuthorisedKey, error) {
	key, comment, _, _, ok := ssh.ParseAuthorizedKey([]byte(line))
	if !ok {
		return nil, fmt.Errorf("invalid authorized_key %q", line)
	}
	keyBytes := ssh.MarshalPublicKey(key)
	return &AuthorisedKey{
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

	logger.Debugf("writing authorised keys file %s", sshKeyFile)
	err = utils.AtomicWriteFile(sshKeyFile, []byte(keyData), perms)
	if err != nil {
		return err
	}

	// TODO (wallyworld) - what to do on windows (if anything)
	// TODO(dimitern) - no need to use user.Current() if username
	// is "" - it will use the current user anyway.
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
		err = os.Chown(sshKeyFile, uid, gid)
		if err != nil {
			return err
		}
	}
	return nil
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
