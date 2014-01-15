// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/ssh"
)

// ReadAuthorizedKeys implements the standard juju behaviour for finding
// authorized_keys. It returns a set of keys in in authorized_keys format
// (see sshd(8) for a description).  If path is non-empty, it names the
// file to use; otherwise the user's .ssh directory will be searched.
// Home directory expansion will be performed on the path if it starts with
// a ~; if the expanded path is relative, it will be interpreted relative
// to $HOME/.ssh.
//
// The result of utils/ssh.PublicKeyFiles will always be prepended to the
// result. In practice, this means ReadAuthorizedKeys never returns an
// error when the call originates in the CLI.
func ReadAuthorizedKeys(path string) (string, error) {
	files := ssh.PublicKeyFiles()
	if path == "" {
		files = append(files, "id_dsa.pub", "id_rsa.pub", "identity.pub")
	} else {
		files = append(files, path)
	}
	var firstError error
	var keyData []byte
	for _, f := range files {
		f, err := utils.NormalizePath(f)
		if err != nil {
			if firstError == nil {
				firstError = err
			}
			continue
		}
		if !filepath.IsAbs(f) {
			f = filepath.Join(osenv.Home(), ".ssh", f)
		}
		data, err := ioutil.ReadFile(f)
		if err != nil {
			if firstError == nil && !os.IsNotExist(err) {
				firstError = err
			}
			continue
		}
		keyData = append(keyData, bytes.Trim(data, "\n")...)
		keyData = append(keyData, '\n')
	}
	if len(keyData) == 0 {
		if firstError == nil {
			firstError = fmt.Errorf("no public ssh keys found")
		}
		return "", firstError
	}
	return string(keyData), nil
}

// verifyKeyPair verifies that the certificate and key parse correctly.
// The key is optional - if it is provided, we also check that the key
// matches the certificate.
func verifyKeyPair(certb, key []byte) error {
	if key != nil {
		_, err := tls.X509KeyPair(certb, key)
		return err
	}
	_, err := cert.ParseCert(certb)
	return err
}
