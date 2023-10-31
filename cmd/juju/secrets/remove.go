// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	apisecrets "github.com/juju/juju/api/client/secrets"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/secrets"
)

type removeSecretCommand struct {
	modelcmd.ModelCommandBase

	secretsAPIFunc func() (RemoveSecretsAPI, error)

	secretURI *secrets.URI
	name      string
	revision  int
}

// RemoveSecretsAPI is the secrets client API.
type RemoveSecretsAPI interface {
	RemoveSecret(uri *secrets.URI, name string, revision *int) error
	Close() error
}

// NewRemoveSecretCommand returns a command to remove a secret.
func NewRemoveSecretCommand() cmd.Command {
	c := &removeSecretCommand{}
	c.secretsAPIFunc = c.secretsAPI
	return modelcmd.Wrap(c)
}

func (c *removeSecretCommand) secretsAPI() (RemoveSecretsAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apisecrets.NewClient(root), nil
}

const (
	removeSecretDoc = `
Remove all the revisions of a secret with the specified URI or remove the provided revision only.
`
	removeSecretExamples = `
    juju remove-secret my-secret
    juju remove-secret secret:9m4e2mr0ui3e8a215n4g
    juju remove-secret secret:9m4e2mr0ui3e8a215n4g --revision 4
`
)

// Info implements cmd.Command.
func (c *removeSecretCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "remove-secret",
		Args:     "<ID>|<name>",
		Purpose:  "Remove a existing secret.",
		Doc:      removeSecretDoc,
		Examples: removeSecretExamples,
	})
}

// SetFlags implements cmd.Command.
func (c *removeSecretCommand) SetFlags(f *gnuflag.FlagSet) {
	f.IntVar(&c.revision, "revision", 0, "remove the specified revision")
}

// Init implements cmd.Command.
func (c *removeSecretCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("missing secret URI")
	}
	var err error
	if c.secretURI, err = secrets.ParseURI(args[0]); err != nil {
		c.name = args[0]
	}
	return cmd.CheckEmpty(args[1:])
}

// Run implements cmd.Command.
func (c *removeSecretCommand) Run(ctx *cmd.Context) error {
	var rev *int
	if c.revision > 0 {
		rev = &c.revision
	}
	secretsAPI, err := c.secretsAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer secretsAPI.Close()
	return secretsAPI.RemoveSecret(c.secretURI, c.name, rev)
}
