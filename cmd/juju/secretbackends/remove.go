// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/client/secretbackends"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	_ "github.com/juju/juju/secrets/provider/all"
)

type removeSecretBackendCommand struct {
	modelcmd.ControllerCommandBase

	RemoveSecretBackendsAPIFunc func() (RemoveSecretBackendsAPI, error)

	Name  string
	Force bool
}

var removeSecretBackendsDoc = `
Removes a secret backend, used for storing secret content.
If the backend is being used to store secrets currently in use,
the ` + "`--force`" + ` option can be supplied to force the removal, but be
warned, this will affect charms which use those secrets.
`

const removeSecretBackendExamples = `
    juju remove-secret-backend myvault
    juju remove-secret-backend myvault --force
`

// RemoveSecretBackendsAPI is the secrets client API.
type RemoveSecretBackendsAPI interface {
	RemoveSecretBackend(string, bool) error
	Close() error
}

// NewRemoveSecretBackendCommand returns a command to remove secret backends.
func NewRemoveSecretBackendCommand() cmd.Command {
	c := &removeSecretBackendCommand{}
	c.RemoveSecretBackendsAPIFunc = c.secretBackendsAPI

	return modelcmd.WrapController(c)
}

func (c *removeSecretBackendCommand) secretBackendsAPI() (RemoveSecretBackendsAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return secretbackends.NewClient(root), nil

}

// Info implements cmd.Info.
func (c *removeSecretBackendCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "remove-secret-backend",
		Purpose:  "Removes a secret backend from the controller.",
		Doc:      removeSecretBackendsDoc,
		Args:     "<backend-name>",
		Examples: removeSecretBackendExamples,
		SeeAlso: []string{
			"add-secret-backend",
			"secret-backends",
			"show-secret-backend",
			"update-secret-backend",
		},
	})
}

// SetFlags implements cmd.SetFlags.
func (c *removeSecretBackendCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.Force, "force", false, "Force removal even if the backend stores in-use secrets")
}

func (c *removeSecretBackendCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("must specify backend name")
	}
	c.Name = args[0]
	return cmd.CheckEmpty(args[1:])
}

// Run implements cmd.Run.
func (c *removeSecretBackendCommand) Run(ctxt *cmd.Context) error {
	api, err := c.RemoveSecretBackendsAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()

	err = api.RemoveSecretBackend(c.Name, c.Force)
	if errors.IsNotSupported(err) {
		cmd.WriteError(ctxt.Stderr, errors.Errorf("backend %q still contains secret content", c.Name))
		return cmd.ErrSilent
	}
	return errors.Trace(err)
}
