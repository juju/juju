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

	stdssh "code.google.com/p/go.crypto/ssh"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/utils"
)

var logger = loggo.GetLogger("juju.ssh")

type ListMode bool

var (
	FullKeys    ListMode = true
	KeyComments ListMode = false
)

const (
	authKeysDir  = "~/.ssh"
	authKeysFile = "authorized_keys"
)

func readAuthorisedKeys() ([]string, error) {
	sshKeyFile := utils.NormalizePath(filepath.Join(authKeysDir, authKeysFile))
	keyData, err := ioutil.ReadFile(sshKeyFile)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
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

func parseKey(key string) (string, error) {
	if _, comment, _, _, ok := stdssh.ParseAuthorizedKey([]byte(key)); !ok {
		return "", fmt.Errorf("invalid ssh key: %v", key)
	} else {
		return comment, nil
	}
}

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
		comment, err := parseKey(newKey)
		if err != nil {
			return err
		}
		if comment == "" {
			return fmt.Errorf("cannot add ssh key without comment")
		}
		for _, key := range existingKeys {
			existingComment, err := parseKey(key)
			if err != nil {
				logger.Warningf("invalid existing ssh key %q: %v", key, err)
				continue
			}
			if existingComment == comment {
				return fmt.Errorf("cannot add duplicate ssh key: %v", comment)
			}
		}
	}
	sshKeys := append(existingKeys, newKeys...)
	return writeAuthorisedKeys(sshKeys)
}

// DeleteKeys removes the ssh keys with the given comments from the authorized ssh keys file.
// Returns an error if there is an issue with *any* of the keys to delete.
func DeleteKeys(comments ...string) error {
	mutex.Lock()
	defer mutex.Unlock()
	existingKeyData, err := readAuthorisedKeys()
	if err != nil {
		return err
	}
	var keysToWrite []string
	var sshKeys = make(map[string]string)
	for _, key := range existingKeyData {
		comment, err := parseKey(key)
		if err != nil || comment == "" {
			logger.Debugf("keeping unrecognised existing ssh key %q: %v", key, err)
			keysToWrite = append(keysToWrite, key)
			continue
		}
		sshKeys[comment] = key
	}
	for _, comment := range comments {
		_, ok := sshKeys[comment]
		if !ok {
			return fmt.Errorf("cannot delete non existent key: %v", comment)
		}
		delete(sshKeys, comment)
	}
	for _, key := range sshKeys {
		keysToWrite = append(keysToWrite, key)
	}
	if len(keysToWrite) == 0 {
		return fmt.Errorf("cannot delete all keys")
	}
	return writeAuthorisedKeys(keysToWrite)
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
		comment, err := parseKey(key)
		if err != nil {
			logger.Warningf("ignoring invalid ssh key %q: %v", key, err)
			continue
		}
		if mode == FullKeys || comment == "" {
			keys = append(keys, key)
		} else {
			keys = append(keys, comment)
		}
	}
	return keys, nil
}
