// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/client/secretbackends"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	_ "github.com/juju/juju/secrets/provider/all"
)

type updateSecretBackendCommand struct {
	modelcmd.ControllerCommandBase

	UpdateSecretBackendsAPIFunc func() (UpdateSecretBackendsAPI, error)

	Name  string
	Force bool

	// Attributes from a file.
	ConfigFile cmd.FileVar
	// Attributes from key value args.
	KeyValueAttrs map[string]interface{}
	Reset         []string
}

var updateSecretBackendsDoc = `
Updates a new secret backend for storing secret content.

You must specify a name for the backend to update,
followed by any necessary backend specific config values.
Config may be specified as key values ot read from a file.
Any key values override file content if both are specified.

Config attributes may be reset back to the default value using ` + "`--reset`" + `.

To rotate the backend access credential/token (if specified), use
the ` + "`token-rotate`" + ` config and supply a duration. To reset any existing
token rotation period, supply a value of ` + "`0`" + `.

`

const updateSecretBackendsExamples = `
    juju update-secret-backend myvault --config /path/to/cfg.yaml
    juju update-secret-backend myvault name=myvault2
    juju update-secret-backend myvault token-rotate=10m --config /path/to/cfg.yaml
    juju update-secret-backend myvault endpoint=https://vault.io:8200 token=s.1wshwhw
    juju update-secret-backend myvault token-rotate=0
    juju update-secret-backend myvault --reset namespace,ca-cert
`

// UpdateSecretBackendsAPI is the secrets client API.
type UpdateSecretBackendsAPI interface {
	UpdateSecretBackend(secretbackends.UpdateSecretBackend, bool) error
	Close() error
}

// NewUpdateSecretBackendCommand returns a command to update a secret backend.
func NewUpdateSecretBackendCommand() cmd.Command {
	c := &updateSecretBackendCommand{}
	c.UpdateSecretBackendsAPIFunc = c.secretBackendsAPI

	return modelcmd.WrapController(c)
}

func (c *updateSecretBackendCommand) secretBackendsAPI() (UpdateSecretBackendsAPI, error) {
	root, err := c.NewAPIRoot()
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
		Args:     "<backend-name>",
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
		"Reset the provided comma delimited config keys")
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
	for k, v := range c.KeyValueAttrs {
		attrs[k] = v
	}

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

	api, err := c.UpdateSecretBackendsAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()

	err = api.UpdateSecretBackend(backend, c.Force)
	return errors.Trace(err)
}
