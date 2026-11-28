// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"context"
	"fmt"
	"maps"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/client/secretbackends"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	_ "github.com/juju/juju/internal/secrets/provider/all"
)

type updateSecretBackendCommand struct {
	modelcmd.ControllerCommandBase

	UpdateSecretBackendsAPIFunc func(ctx context.Context) (UpdateSecretBackendsAPI, error)

	Name  string
	Force bool

	// Attributes from a file.
	ConfigFile cmd.FileVar
	// Attributes from key value args.
	KeyValueAttrs map[string]any
	Reset         []string
}

var updateSecretBackendsDoc = `
Updates a backend by its current name. Supply a YAML mapping with ` + "`--config`" + `
(use - for stdin), key=value arguments, or ` + "`--reset`" + `. Arguments override file
entries. Repeat ` + "`--reset`" + ` once per provider key; commas do not separate keys.
Resets are applied last. To disable backend token refresh, use token-rotate=0,
not ` + "`--reset token-rotate`" + `. Use name=NEW to rename the registration.

On success the command prints nothing; use show-secret-backend with the
resulting name to inspect the change.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/secret-backend/
`

const updateSecretBackendsExamples = `
Apply the settings in a private YAML file to the existing Vault backend ` + "`myvault`" + `:

    juju update-secret-backend myvault --config vault_config.yaml

Enable token refresh and request a two-hour lifetime for Juju's Vault access token:

    juju update-secret-backend myvault token-rotate=2h

Disable automatic refresh of the Vault access token:

    juju update-secret-backend myvault token-rotate=0

Clear the configured TLS client certificate and its private key:

    juju update-secret-backend myvault --reset client-cert --reset client-key

Rename the registration from ` + "`myvault`" + ` to ` + "`team-vault`" + `:

    juju update-secret-backend myvault name=team-vault
`

// UpdateSecretBackendsAPI is the secrets client API.
type UpdateSecretBackendsAPI interface {
	UpdateSecretBackend(context.Context, secretbackends.UpdateSecretBackend, bool) error
	Close() error
}

// NewUpdateSecretBackendCommand returns a command to update a secret backend.
func NewUpdateSecretBackendCommand() cmd.Command {
	c := &updateSecretBackendCommand{}
	c.UpdateSecretBackendsAPIFunc = c.secretBackendsAPI

	return modelcmd.WrapController(c)
}

func (c *updateSecretBackendCommand) secretBackendsAPI(ctx context.Context) (UpdateSecretBackendsAPI, error) {
	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return secretbackends.NewClient(root), nil

}

// Info implements cmd.Info.
func (c *updateSecretBackendCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "update-secret-backend",
		Purpose:  "Update an existing secret backend on the controller.",
		Doc:      updateSecretBackendsDoc,
		Args:     "<backend-name> (--config <file> | <key>=<value> | --reset <key>) ...",
		Examples: updateSecretBackendsExamples,
		SeeAlso: []string{
			"add-secret-backend",
			"secret-backends",
			"remove-secret-backend",
			"show-secret-backend",
		},
	})
}

// SetFlags implements cmd.SetFlags.
func (c *updateSecretBackendCommand) SetFlags(f *gnuflag.FlagSet) {
	f.Var(&c.ConfigFile, "config", "Path to yaml-formatted configuration file")
	f.Var(cmd.NewAppendStringsValue(&c.Reset), "reset",
		"Reset one provider configuration key; repeat for each key")
	f.BoolVar(&c.Force, "force", false, "Force update even if the backend is unreachable")
}

func (c *updateSecretBackendCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("must specify backend name")
	}
	c.Name = args[0]
	// The remaining arguments are divided into keys to set.
	var err error
	if c.KeyValueAttrs, err = parseArgs(args[1:]); err != nil {
		return errors.Trace(err)
	}
	for _, key := range c.Reset {
		delete(c.KeyValueAttrs, key)
	}

	if len(c.KeyValueAttrs) == 0 && len(c.Reset) == 0 && c.ConfigFile.Path == "" {
		return errors.New("must specify a config file or key/reset values")
	}
	return nil
}

// Run implements cmd.Run.
func (c *updateSecretBackendCommand) Run(ctxt *cmd.Context) error {
	attrs, err := readFile(ctxt, c.ConfigFile)
	if err != nil {
		return errors.Trace(err)
	}
	maps.Copy(attrs, c.KeyValueAttrs)

	tokenRotateInterval, err := parseTokenRotate(attrs, true)
	if err != nil {
		return errors.Trace(err)
	}

	var nameChange string
	if n, ok := attrs["name"]; ok {
		delete(attrs, "name")
		nameChange = fmt.Sprintf("%v", n)
	}
	backend := secretbackends.UpdateSecretBackend{
		Name:                c.Name,
		TokenRotateInterval: tokenRotateInterval,
		Config:              attrs,
		Reset:               c.Reset,
	}
	if nameChange != "" {
		backend.NameChange = &nameChange
	}

	api, err := c.UpdateSecretBackendsAPIFunc(ctxt)
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()

	err = api.UpdateSecretBackend(ctxt, backend, c.Force)
	return errors.Trace(err)
}
