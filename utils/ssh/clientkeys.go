// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"code.google.com/p/go.crypto/ssh"

	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/set"
)

const clientKeyName = "juju_id_rsa"

var (
	clientKeysMutex sync.Mutex

	// clientKeys is a cached map of private key filenames
	// to ssh.Signers. The private keys are those loaded
	// from the client key directory, passed to LoadClientKeys.
	clientKeys map[string]ssh.Signer
)

// LoadClientKeys loads the client SSH keys from the
// specified directory, and caches them as a process-wide
// global. If the directory does not exist, it is created
// and populated with a new key pair.
//
// If the directory exists, then all pairs of files where one
// has the same name as the other + ".pub" will be loaded as
// private/public key pairs.
//
// Calls to LoadClientKeys will clear the previously loaded
// keys, and recompute the keys. If the dir specified is "",
// then the keys will be cleared and nil is returned without
// further action.
func LoadClientKeys(dir string) error {
	clientKeysMutex.Lock()
	defer clientKeysMutex.Unlock()
	if dir == "" {
		// Primarily for testing.
		clientKeys = nil
		return nil
	}
	dir, err := utils.NormalizePath(dir)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); err == nil {
		// If the directory exists without keys,
		// the user must remove the directory.
		keys, err := loadClientKeys(dir)
		if err == nil {
			clientKeys = keys
		}
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	keyfile, key, err := generateClientKey(dir)
	if err != nil {
		os.RemoveAll(dir)
		return err
	}
	clientKeys = map[string]ssh.Signer{keyfile: key}
	return nil
}

func generateClientKey(dir string) (keyfile string, key ssh.Signer, err error) {
	private, public, err := GenerateKey("juju-client-key")
	if err != nil {
		return "", nil, err
	}
	clientPrivateKey, err := ssh.ParsePrivateKey([]byte(private))
	if err != nil {
		return "", nil, err
	}
	privkeyFilename := filepath.Join(dir, clientKeyName)
	if err = ioutil.WriteFile(privkeyFilename, []byte(private), 0600); err != nil {
		return "", nil, err
	}
	if err := ioutil.WriteFile(privkeyFilename+".pub", []byte(public), 0600); err != nil {
		os.Remove(privkeyFilename)
		return "", nil, err
	}
	return privkeyFilename, clientPrivateKey, nil
}

func loadClientKeys(dir string) (map[string]ssh.Signer, error) {
	publicKeyFiles, err := publicKeyFiles(dir)
	if err != nil {
		return nil, err
	}
	keys := make(map[string]ssh.Signer, len(publicKeyFiles))
	for _, filename := range publicKeyFiles {
		filename = filename[:len(filename)-len(".pub")]
		data, err := ioutil.ReadFile(filename)
		if err != nil {
			return nil, err
		}
		keys[filename], err = ssh.ParsePrivateKey(data)
		if err != nil {
			return nil, fmt.Errorf("parsing key file %q: %v", filename, err)
		}
	}
	return keys, nil
}

// privateKeys returns the private keys loaded by LoadClientKeys.
func privateKeys() (signers []ssh.Signer) {
	clientKeysMutex.Lock()
	for _, key := range clientKeys {
		signers = append(signers, key)
	}
	clientKeysMutex.Unlock()
	return signers
}

// PrivateKeyFiles returns the filenames of private SSH keys loaded by
// LoadClientKeys.
func PrivateKeyFiles() []string {
	clientKeysMutex.Lock()
	defer clientKeysMutex.Unlock()
	keyfiles := make([]string, 0, len(clientKeys))
	for f := range clientKeys {
		keyfiles = append(keyfiles, f)
	}
	return keyfiles
}

// PublicKeyFiles returns the filenames of public SSH keys loaded by
// LoadClientKeys.
func PublicKeyFiles() []string {
	privkeys := PrivateKeyFiles()
	pubkeys := make([]string, len(privkeys))
	for i, priv := range privkeys {
		pubkeys[i] = priv + ".pub"
	}
	return pubkeys
}

// publicKeyFiles returns the filenames of public SSH keys
// in the specified directory (all the files ending with .pub).
func publicKeyFiles(clientKeysDir string) ([]string, error) {
	if clientKeysDir == "" {
		return nil, nil
	}
	var keys []string
	dir, err := os.Open(clientKeysDir)
	if err != nil {
		return nil, err
	}
	names, err := dir.Readdirnames(-1)
	dir.Close()
	if err != nil {
		return nil, err
	}
	candidates := set.NewStrings(names...)
	for _, name := range names {
		if !strings.HasSuffix(name, ".pub") {
			continue
		}
		// If the private key filename also exists, add the file.
		priv := name[:len(name)-len(".pub")]
		if candidates.Contains(priv) {
			keys = append(keys, filepath.Join(dir.Name(), name))
		}
	}
	return keys, nil
}
