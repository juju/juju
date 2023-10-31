// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"

	apisecrets "github.com/juju/juju/api/client/secrets"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/secrets"
)

type grantSecretCommand struct {
	modelcmd.ModelCommandBase

	secretURI *secrets.URI
	apps      []string

	secretsAPIFunc func() (GrantRevokeSecretsAPI, error)
}

// GrantRevokeSecretsAPI is the secrets client API.
type GrantRevokeSecretsAPI interface {
	GrantSecret(*secrets.URI, []string) ([]error, error)
	RevokeSecret(*secrets.URI, []string) ([]error, error)
	Close() error
}

// NewGrantSecretCommand returns a command to grant view access of a secret to applications.
func NewGrantSecretCommand() cmd.Command {
	c := &grantSecretCommand{}
	c.secretsAPIFunc = c.secretsAPI
	return modelcmd.Wrap(c)
}

func (c *grantSecretCommand) secretsAPI() (GrantRevokeSecretsAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apisecrets.NewClient(root), nil
}

const (
	grantSecretDoc = `
Grant applications access to view the value of a specified secret.
`
	grantSecretExamples = `
    juju grant-secret 9m4e2mr0ui3e8a215n4g ubuntu-k8s
	juju grant-secret 9m4e2mr0ui3e8a215n4g ubuntu-k8s,prometheus-k8s
`
)

// Info implements cmd.Command.
func (c *grantSecretCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "grant-secret",
		Args:     "<secret-uri> <application>[,<application>...]",
		Purpose:  "Grant access to a secret.",
		Doc:      grantSecretDoc,
		Examples: grantSecretExamples,
	})
}

// Init implements cmd.Command.
func (c *grantSecretCommand) Init(args []string) error {
	if len(args) < 2 {
		return errors.New("missing secret URI or application name")
	}

	var err error
	if c.secretURI, err = secrets.ParseURI(args[0]); err != nil {
		return errors.Trace(err)
	}
	c.apps = strings.Split(args[1], ",")
	return cmd.CheckEmpty(args[2:])
}

// Run implements cmd.Command.
func (c *grantSecretCommand) Run(ctx *cmd.Context) error {
	secretsAPI, err := c.secretsAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer secretsAPI.Close()
	return processGrantRevokeErrors(secretsAPI.GrantSecret(c.secretURI, c.apps))
}

func processGrantRevokeErrors(errs []error, err error) error {
	if err != nil {
		return errors.Trace(err)
	}
	if len(errs) == 0 {
		return nil
	}
	var errStrings []string
	for _, err := range errs {
		if err == nil {
			continue
		}
		errStrings = append(errStrings, err.Error())
	}
	if len(errStrings) == 0 {
		return nil
	}
	return errors.Errorf("failed to grant/revoke secret: %s", strings.Join(errStrings, ", "))
}

type revokeSecretCommand struct {
	modelcmd.ModelCommandBase

	secretURI *secrets.URI
	apps      []string

	secretsAPIFunc func() (GrantRevokeSecretsAPI, error)
}

// NewRevokeSecretCommand returns a command to revoke view access of a secret to applications.
func NewRevokeSecretCommand() cmd.Command {
	c := &revokeSecretCommand{}
	c.secretsAPIFunc = c.secretsAPI
	return modelcmd.Wrap(c)
}

func (c *revokeSecretCommand) secretsAPI() (GrantRevokeSecretsAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apisecrets.NewClient(root), nil
}

const (
	revokeSecretDoc = `
Revoke applications' access to view the value of a specified secret.
`
	revokeSecretExamples = `
    juju revoke-secret 9m4e2mr0ui3e8a215n4g ubuntu-k8s
	juju revoke-secret 9m4e2mr0ui3e8a215n4g ubuntu-k8s,prometheus-k8s
`
)

// Info implements cmd.Command.
func (c *revokeSecretCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "revoke-secret",
		Args:     "<secret-uri> <application>[,<application>...]",
		Purpose:  "Revoke access to a secret.",
		Doc:      revokeSecretDoc,
		Examples: revokeSecretExamples,
	})
}

// Init implements cmd.Command.
func (c *revokeSecretCommand) Init(args []string) error {
	if len(args) < 2 {
		return errors.New("missing secret URI or application name")
	}

	var err error
	if c.secretURI, err = secrets.ParseURI(args[0]); err != nil {
		return errors.Trace(err)
	}
	c.apps = strings.Split(args[1], ",")
	return cmd.CheckEmpty(args[2:])
}

// Run implements cmd.Command.
func (c *revokeSecretCommand) Run(ctx *cmd.Context) error {
	secretsAPI, err := c.secretsAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer secretsAPI.Close()
	return processGrantRevokeErrors(secretsAPI.RevokeSecret(c.secretURI, c.apps))
}
