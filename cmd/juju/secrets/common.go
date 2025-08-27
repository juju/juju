// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/core/secrets"
)

// SecretUpsertContentCommand is the helper base command to create or update a secret.
type SecretUpsertContentCommand struct {
	Data        map[string]string
	Description string
	FileName    string
}

// SetFlags implements cmd.Command.
func (c *SecretUpsertContentCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.Description, "info", "", "The secret description")
	f.StringVar(&c.FileName, "file", "", "A YAML file containing secret key values")
}

// Init implements cmd.Command.
func (c *SecretUpsertContentCommand) Init(args []string) error {
	var err error
	c.Data, err = secrets.CreateSecretData(args)
	if err != nil {
		return errors.Trace(err)
	}
	if c.FileName == "" {
		return nil
	}
	dataFromFile, err := secrets.ReadSecretData(c.FileName)
	if err != nil {
		return errors.Trace(err)
	}
	for k, v := range dataFromFile {
		c.Data[k] = v
	}
	return nil
}
