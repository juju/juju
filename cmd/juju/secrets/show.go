// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	apisecrets "github.com/juju/juju/api/client/secrets"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	coresecrets "github.com/juju/juju/core/secrets"
)

type showSecretsCommand struct {
	modelcmd.ModelCommandBase
	out cmd.Output

	listSecretsAPIFunc func() (ListSecretsAPI, error)
	uri                *coresecrets.URI
	name               string
	revealSecrets      bool
	revisions          bool
	revision           int
}

var showSecretsDoc = `
Displays the details of a specified secret.

For controller/model admins, the actual secret content is exposed
with the ` + "`--reveal`" + ` option in the ` + "`json`" + ` or ` + "`yaml`" + ` formats.

Use ` + "`--revision`" + ` to inspect a particular revision, else latest is used.
Use ` + "`--revisions`" + ` to see the metadata for each revision.
`

const showSecretsExamples = `
    juju show-secret my-secret
    juju show-secret 9m4e2mr0ui3e8a215n4g
    juju show-secret secret:9m4e2mr0ui3e8a215n4g --revision 2
    juju show-secret 9m4e2mr0ui3e8a215n4g --revision 2 --reveal
    juju show-secret 9m4e2mr0ui3e8a215n4g --revisions
    juju show-secret 9m4e2mr0ui3e8a215n4g --reveal
`

// NewShowSecretsCommand returns a command to list secrets metadata.
func NewShowSecretsCommand() cmd.Command {
	c := &showSecretsCommand{}
	c.listSecretsAPIFunc = c.secretsAPI

	return modelcmd.Wrap(c)
}

func (c *showSecretsCommand) secretsAPI() (ListSecretsAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apisecrets.NewClient(root), nil

}

// Info implements cmd.Info.
func (c *showSecretsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "show-secret",
		Args:     "<ID>|<name>",
		Purpose:  "Shows details for a specific secret.",
		Doc:      showSecretsDoc,
		Examples: showSecretsExamples,
		SeeAlso: []string{
			"add-secret",
			"update-secret",
			"remove-secret",
		},
	})
}

// SetFlags implements cmd.SetFlags.
func (c *showSecretsCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.revealSecrets, "reveal", false, "Reveal secret values, applicable to yaml or json formats only")
	f.BoolVar(&c.revisions, "revisions", false, "Show the secret revisions metadata")
	f.IntVar(&c.revision, "revision", 0, "Show a specific revision (defaults to latest)")
	f.IntVar(&c.revision, "r", 0, "")
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Init implements cmd.Init.
func (c *showSecretsCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.Errorf("secret ID is required")
	}
	uri, err := coresecrets.ParseURI(args[0])
	if err != nil {
		c.name = args[0]
	}
	c.uri = uri
	if c.revisions {
		if c.revealSecrets {
			return errors.New("specify either --revisions or --reveal but not both")
		}
		if c.revision > 0 {
			return errors.New("specify either --revisions or --revision but not both")
		}
	}
	if c.revision < 0 {
		return errors.New("revision must be a positive integer")
	}
	return cmd.CheckEmpty(args[1:])
}

// Run implements cmd.Run.
func (c *showSecretsCommand) Run(ctxt *cmd.Context) error {
	if c.revealSecrets && c.out.Name() == "tabular" {
		ctxt.Infof("secret values are not shown in tabular format")
		c.revealSecrets = false
	}

	api, err := c.listSecretsAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()

	filter := coresecrets.Filter{
		URI: c.uri,
	}
	if c.revision > 0 {
		filter.Revision = &c.revision
	}
	if c.name != "" {
		filter.Label = &c.name
	}
	result, err := api.ListSecrets(c.revealSecrets, filter)
	if err != nil {
		return errors.Trace(err)
	}
	details := gatherSecretInfo(result, c.revealSecrets, c.revisions, true)
	if len(details) == 0 {
		if c.uri != nil {
			return errors.NotFoundf("secret %q", c.uri.ID)
		}
		return errors.NotFoundf("secret %q", c.name)
	}

	return c.out.Write(ctxt, details)
}
