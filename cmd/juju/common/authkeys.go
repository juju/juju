// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils/v4"
	"github.com/juju/utils/v4/ssh"

	"github.com/juju/juju/internal/cmd"
)

// ErrNoAuthorizedKeys is returned by ReadAuthorizedKeys when no
// authorized_keys files are found.
var ErrNoAuthorizedKeys = errors.New("no public ssh keys found")

// ReadAuthorizedKeys implements the standard juju behaviour for finding
// authorized_keys. It returns a set of keys in authorized_keys format
// (see sshd(8) for a description).  If path is non-empty, it names the
// file to use; otherwise the user's .ssh directory will be searched.
// Home directory expansion will be performed on the path if it starts with
// a ~; if the expanded path is relative, it will be interpreted relative
// to $HOME/.ssh.
//
// The result of utils/ssh.PublicKeyFiles will always be prepended to the
// result. In practice, this means ReadAuthorizedKeys never returns an
// error when the call originates in the CLI.
//
// If no SSH keys are found, ReadAuthorizedKeys returns
// ErrNoAuthorizedKeys.
func ReadAuthorizedKeys(ctx *cmd.Context, path string) (string, error) {
	files := ssh.PublicKeyFiles()
	if path == "" {
		files = append(files, "id_ed25519.pub", "id_ecdsa.pub", "id_rsa.pub", "identity.pub")
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
			f = filepath.Join(utils.Home(), ".ssh", f)
		}
		data, err := os.ReadFile(f)
		if err != nil {
			if firstError == nil && !os.IsNotExist(err) {
				firstError = err
			}
			continue
		}
		keyData = append(keyData, bytes.Trim(data, "\n")...)
		keyData = append(keyData, '\n')
		ctx.Verbosef("Adding contents of %q to authorized-keys", f)
	}
	if len(keyData) == 0 {
		if firstError == nil {
			firstError = ErrNoAuthorizedKeys
		}
		return "", firstError
	}
	return string(keyData), nil

}
