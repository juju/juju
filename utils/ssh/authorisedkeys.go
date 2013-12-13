// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/utils"
)

var logger = loggo.GetLogger("juju.ssh")

type ListMode bool

var (
	FullKeys     ListMode = true
	Fingerprints ListMode = false
)

const (
	authKeysDir  = "~/.ssh"
	authKeysFile = "authorized_keys"
)

// KeyFingerprint returns the fingerprint and comment for the specified key, using
// the OS dependent function keyFingerprint.
var KeyFingerprint = keyFingerprint

func readAuthorisedKeys() ([]string, error) {
	sshKeyFile := utils.NormalizePath(filepath.Join(authKeysDir, authKeysFile))
	keyData, err := ioutil.ReadFile(sshKeyFile)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading ssh authorised keys file: %v", err)
	}
	var keys []string
	for _, key := range strings.Split(string(keyData), "\n") {
		if len(strings.Trim(key, " ")) == 0 {
			continue
		}
		keys = append(keys, key)
	}
	return keys, nil
}

func writeAuthorisedKeys(keys []string) error {
	keyDir := utils.NormalizePath(authKeysDir)
	err := os.MkdirAll(keyDir, 0755)
	if err != nil {
		return fmt.Errorf("cannot create ssh key directory: %v", err)
	}
	keyData := strings.Join(keys, "\n") + "\n"
	sshKeyFile := filepath.Join(keyDir, authKeysFile)
	return ioutil.WriteFile(sshKeyFile, []byte(keyData), 0644)
}

// We need a mutex because updates to the authorised keys file are done by
// reading the contents, updating, and writing back out. So only one caller
// at a time can use either Add, Delete, List.
var mutex sync.Mutex

// AddKeys adds the specified ssh keys to the authorized_keys file.
// Returns an error if there is an issue with *any* of the supplied keys.
func AddKeys(newKeys ...string) error {
	mutex.Lock()
	defer mutex.Unlock()
	existingKeys, err := readAuthorisedKeys()
	if err != nil {
		return err
	}
	for _, newKey := range newKeys {
		fingerprint, comment, err := keyFingerprint(newKey)
		if err != nil {
			return err
		}
		if comment == "" {
			return fmt.Errorf("cannot add ssh key without comment")
		}
		for _, key := range existingKeys {
			existingFingerprint, existingComment, err := keyFingerprint(key)
			if err != nil {
				logger.Warningf("invalid existing ssh key %q: %v", key, err)
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
	return writeAuthorisedKeys(sshKeys)
}

// DeleteKeys removes the specified ssh keys from the authorized ssh keys file.
// keyIds may be either key comments or fingerprints.
// Returns an error if there is an issue with *any* of the keys to delete.
func DeleteKeys(keyIds ...string) error {
	mutex.Lock()
	defer mutex.Unlock()
	existingKeyData, err := readAuthorisedKeys()
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
		fingerprint, comment, err := keyFingerprint(key)
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
	return writeAuthorisedKeys(keysToWrite)
}

// ReplaceKeys writes the specified ssh keys to the authorized_keys file,
// replacing any that are already there.
// Returns an error if there is an issue with *any* of the supplied keys.
func ReplaceKeys(newKeys ...string) error {
	mutex.Lock()
	defer mutex.Unlock()
	for _, newKey := range newKeys {
		_, comment, err := keyFingerprint(newKey)
		if err != nil {
			return err
		}
		if comment == "" {
			return fmt.Errorf("cannot add ssh key without comment")
		}
	}
	return writeAuthorisedKeys(newKeys)
}

// ListKeys returns either the full keys or key comments from the authorized ssh keys file.
func ListKeys(mode ListMode) ([]string, error) {
	mutex.Lock()
	defer mutex.Unlock()
	keyData, err := readAuthorisedKeys()
	if err != nil {
		return nil, err
	}
	var keys []string
	for _, key := range keyData {
		fingerprint, comment, err := keyFingerprint(key)
		if err != nil {
			logger.Warningf("ignoring invalid ssh key %q: %v", key, err)
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
	_, comment, err := keyFingerprint(key)
	// Just return an invalid key as is.
	if err != nil {
		logger.Warningf("invalid Juju ssh key %s: %v", key, err)
		return key
	}
	if comment == "" {
		return key + " " + JujuCommentPrefix + "sshkey"
	} else {
		// Add the Juju prefix to the comment if necessary.
		if !strings.HasPrefix(comment, JujuCommentPrefix) {
			commentIndex := strings.LastIndex(key, comment)
			return key[:commentIndex] + JujuCommentPrefix + comment
		}
	}
	return key
}
