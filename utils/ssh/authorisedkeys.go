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
	for _, key := range keys {
		if _, err := parseKey(key); err != nil {
			return err
		}
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

// AddKey adds the specified ssh key to the authorized_keys file.
func AddKey(newKey string) error {
	mutex.Lock()
	defer mutex.Unlock()
	comment, err := parseKey(newKey)
	if err != nil {
		return err
	}
	if comment == "" {
		return fmt.Errorf("cannot add ssh key without comment")
	}
	existingKeys, err := readAuthorisedKeys()
	if err != nil {
		return err
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
	sshKeys := append(existingKeys, newKey)
	return writeAuthorisedKeys(sshKeys)
}

// DeleteKey removes the ssh key with the given comment from the authorized ssh keys file.
func DeleteKey(comment string) error {
	mutex.Lock()
	defer mutex.Unlock()
	existingKeys, err := readAuthorisedKeys()
	if err != nil {
		return err
	}
	var newKeys []string
	found := false
	for _, key := range existingKeys {
		existingComment, err := parseKey(key)
		if err != nil {
			logger.Warningf("invalid existing ssh key %q: %v", key, err)
			continue
		}
		if existingComment == comment {
			found = true
		} else {
			newKeys = append(newKeys, key)
		}
	}
	if !found {
		return fmt.Errorf("cannot delete non existent key: %v", comment)
	}
	if len(newKeys) == 0 {
		return fmt.Errorf("cannot delete only key: %v", comment)
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
