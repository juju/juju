// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/utils/v4"
	"github.com/juju/utils/v4/ssh"

	"github.com/juju/juju/environs/config"
)

var ErrNoAuthorizedKeys = errors.New("no public ssh keys found")

// FinalizeAuthorizedKeys takes a set of configuration attributes and
// ensures that it has an authorized-keys setting, or returns
// ErrNoAuthorizedKeys if it cannot.
//
// If the attributes contains a non-empty value for "authorized-keys",
// then it is left alone. If there is an "authorized-keys-path" setting,
// its contents will be loaded into "authorized-keys". Otherwise, the
// contents of standard public keys will be used: ~/.ssh/id_dsa.pub,
// ~/.ssh/id_rsa.pub, and ~/.ssh/identity.pub.
func FinalizeAuthorizedKeys(ctx *cmd.Context, attrs map[string]interface{}) error {
	const authorizedKeysPathKey = "authorized-keys-path"
	checker := schema.FieldMap(schema.Fields{
		config.AuthorizedKeysKey: schema.String(),
		authorizedKeysPathKey:    schema.String(),
	}, schema.Defaults{
		config.AuthorizedKeysKey: schema.Omit,
		authorizedKeysPathKey:    schema.Omit,
	})
	coerced, err := checker.Coerce(attrs, nil)
	if err != nil {
		return errors.Trace(err)
	}
	coercedAttrs := coerced.(map[string]interface{})

	_, haveAuthorizedKeys := coercedAttrs[config.AuthorizedKeysKey].(string)
	authorizedKeysPath, haveAuthorizedKeysPath := coercedAttrs[authorizedKeysPathKey].(string)
	if haveAuthorizedKeys && haveAuthorizedKeysPath {
		return errors.Errorf(
			"%q and %q may not both be specified",
			config.AuthorizedKeysKey, authorizedKeysPathKey,
		)
	}
	if haveAuthorizedKeys {
		// We have authorized-keys already; nothing to do.
		return nil
	}

	authorizedKeys, err := ReadAuthorizedKeys(ctx, authorizedKeysPath)
	if err != nil {
		return errors.Annotate(err, "reading authorized-keys")
	}
	if haveAuthorizedKeysPath {
		delete(attrs, authorizedKeysPathKey)
	}
	attrs[config.AuthorizedKeysKey] = authorizedKeys
	return nil
}

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
