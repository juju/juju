// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/client/secretbackends"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	_ "github.com/juju/juju/internal/secrets/provider/all"
)

type removeSecretBackendCommand struct {
	modelcmd.ControllerCommandBase

	RemoveSecretBackendsAPIFunc func(ctx context.Context) (RemoveSecretBackendsAPI, error)

	Name  string
	Force bool
}

var removeSecretBackendsDoc = `
Removes the named registration without a confirmation prompt. For the built-in
backends (internal and kubernetes), the message that the backend still contains
secret content means removal is not allowed, even when the backend is empty.
Leave these backends in place. For a user-added backend, the same message means
revisions still reference it. Check those references before retrying.

With ` + "`--force`" + `, Juju can lose access to user and charm secret revisions without
moving or erasing the remote values. Force cannot remove a built-in. On success
this command prints nothing; models selecting the removed backend revert to auto.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/secret-backend/
`

const removeSecretBackendExamples = `
Once no secret revisions reference ` + "`myvault`" + `, remove its registration from the controller:

    juju remove-secret-backend myvault
`

// RemoveSecretBackendsAPI is the secrets client API.
type RemoveSecretBackendsAPI interface {
	RemoveSecretBackend(context.Context, string, bool) error
	Close() error
}

// NewRemoveSecretBackendCommand returns a command to remove secret backends.
func NewRemoveSecretBackendCommand() cmd.Command {
	c := &removeSecretBackendCommand{}
	c.RemoveSecretBackendsAPIFunc = c.secretBackendsAPI

	return modelcmd.WrapController(c)
}

func (c *removeSecretBackendCommand) secretBackendsAPI(ctx context.Context) (RemoveSecretBackendsAPI, error) {
	root, err := c.NewAPIRoot(ctx)
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
	f.BoolVar(&c.Force, "force", false, "Remove a user-added backend even if revisions still reference it")
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
	api, err := c.RemoveSecretBackendsAPIFunc(ctxt)
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()

	err = api.RemoveSecretBackend(ctxt, c.Name, c.Force)
	if errors.Is(err, errors.NotSupported) {
		cmd.WriteError(ctxt.Stderr, errors.Errorf("backend %q still contains secret content", c.Name))
		return cmd.ErrSilent
	}
	return errors.Trace(err)
}
