// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/client/secretbackends"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/output"
)

type showSecretBackendCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	ShowSecretBackendsAPIFunc func() (ShowSecretBackendsAPI, error)
	backendName               string
	revealSecrets             bool
}

var showSecretBackendsDoc = `
Displays the specified secret backend.
`

const showSecretBackendsExamples = `
    juju show-secret-backend myvault
    juju secret-backends myvault --reveal
`

// ShowSecretBackendsAPI is the secrets client API.
type ShowSecretBackendsAPI interface {
	ListSecretBackends([]string, bool) ([]secretbackends.SecretBackend, error)
	Close() error
}

// NewShowSecretBackendCommand returns a command to show a secrets backend.
func NewShowSecretBackendCommand() cmd.Command {
	c := &showSecretBackendCommand{}
	c.ShowSecretBackendsAPIFunc = c.secretBackendsAPI

	return modelcmd.WrapController(c)
}

func (c *showSecretBackendCommand) secretBackendsAPI() (ShowSecretBackendsAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return secretbackends.NewClient(root), nil

}

// Info implements cmd.Info.
func (c *showSecretBackendCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "show-secret-backend",
		Purpose:  "Displays the specified secret backend.",
		Doc:      showSecretBackendsDoc,
		Args:     "<backend-name>",
		Examples: showSecretBackendsExamples,
		SeeAlso: []string{
			"add-secret-backend",
			"secret-backends",
			"remove-secret-backend",
			"update-secret-backend",
		},
	})
}

// SetFlags implements cmd.SetFlags.
func (c *showSecretBackendCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.revealSecrets, "reveal", false, "Include sensitive backend config content")
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
}

// Init implements cmd.Init.
func (c *showSecretBackendCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("must specify backend name")
	}
	c.backendName = args[0]
	return cmd.CheckEmpty(args[1:])
}

// Run implements cmd.Run.
func (c *showSecretBackendCommand) Run(ctxt *cmd.Context) error {
	api, err := c.ShowSecretBackendsAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()

	result, err := api.ListSecretBackends([]string{c.backendName}, c.revealSecrets)
	if err != nil {
		return errors.Trace(err)
	}
	details := gatherSecretBackendInfo(result)
	if len(details) == 0 {
		ctxt.Infof("no secret backends have been added to this controller\n")
		return nil
	}
	return c.out.Write(ctxt, details)
}
