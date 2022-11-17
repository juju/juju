// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/core/secrets"
)

type secretGetCommand struct {
	cmd.CommandBase
	ctx Context
	out cmd.Output

	secretUri *secrets.URI
	label     string
	key       string
	peek      bool
	update    bool

	metadata bool
}

// NewSecretGetCommand returns a command to get a secret value.
func NewSecretGetCommand(ctx Context) (cmd.Command, error) {
	return &secretGetCommand{ctx: ctx}, nil
}

// Info implements cmd.Command.
func (c *secretGetCommand) Info() *cmd.Info {
	doc := `
Get the value of a secret with a given secret ID.
The first time the value is fetched, the latest revision is used.
Subsequent calls will always return this same revision unless
--peek or --update are used.
Using --peek will fetch the latest revision just this time.
Using --update will fetch the latest revision and continue to
return the same revision next time unless --peek or --update is used.

Secret owners can also fetch the metadata for the secret using --metadata.
Either the ID or label can be used to identify the secret.

Examples
    secret-get secret:9m4e2mr0ui3e8a215n4g
    secret-get secret:9m4e2mr0ui3e8a215n4g token
    secret-get secret:9m4e2mr0ui3e8a215n4g token#base64
    secret-get secret:9m4e2mr0ui3e8a215n4g --format json
    secret-get secret:9m4e2mr0ui3e8a215n4g --peek
    secret-get secret:9m4e2mr0ui3e8a215n4g --update
    secret-get secret:9m4e2mr0ui3e8a215n4g --label db-password

    secret-get secret:9m4e2mr0ui3e8a215n4g --metadata label db-password
    secret-get --metadata --label db-password
`
	return jujucmd.Info(&cmd.Info{
		Name:    "secret-get",
		Args:    "<ID> [key[#base64]]",
		Purpose: "get the value of a secret",
		Doc:     doc,
	})
}

// SetFlags implements cmd.Command.
func (c *secretGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
	f.StringVar(&c.label, "label", "", "a label used to identify the secret in hooks")
	f.BoolVar(&c.peek, "peek", false,
		`get the latest revision just this time`)
	f.BoolVar(&c.update, "update", false,
		`get the latest revision and also get this same revision for subsequent calls`)
	f.BoolVar(&c.metadata, "metadata", false,
		`get just the secret metadata`)
}

// Init implements cmd.Command.
func (c *secretGetCommand) Init(args []string) (err error) {
	if len(args) > 0 {
		c.secretUri, err = secrets.ParseURI(args[0])
		if err != nil {
			return errors.NotValidf("secret URI %q", args[0])
		}
		args = args[1:]
	}

	if c.secretUri == nil && c.label == "" {
		return errors.New("require either a secret URI or label")
	}

	if c.metadata {
		if c.secretUri != nil && c.label != "" {
			return errors.New("specify either a secret URI or label but not both")
		}
		if c.peek || c.update {
			return errors.New("--peek and --update are not valid when fetching metadata")
		}
		return cmd.CheckEmpty(args)
	}

	if c.peek && c.update {
		return errors.New("specify one of --peek or --update but not both")
	}
	if len(args) > 0 {
		c.key = args[0]
		return cmd.CheckEmpty(args[1:])
	}
	return cmd.CheckEmpty(args)
}

type metadataDisplay struct {
	LatestRevision   int                  `yaml:"revision" json:"revision"`
	Label            string               `yaml:"label" json:"label"`
	Owner            string               `yaml:"owner" json:"owner"`
	Description      string               `yaml:"description,omitempty" json:"description,omitempty"`
	RotatePolicy     secrets.RotatePolicy `yaml:"rotation,omitempty" json:"rotation,omitempty"`
	LatestExpireTime *time.Time           `yaml:"expiry,omitempty" json:"expiry,omitempty"`
	NextRotateTime   *time.Time           `yaml:"rotates,omitempty" json:"rotates,omitempty"`
}

func (c *secretGetCommand) getMetadata(ctx *cmd.Context) error {
	all, err := c.ctx.SecretMetadata()
	if err != nil {
		return err
	}
	print := func(id string, md SecretMetadata) error {
		return c.out.Write(ctx, map[string]metadataDisplay{
			id: {
				LatestRevision:   md.LatestRevision,
				Label:            md.Label,
				Owner:            md.Owner.Kind(),
				Description:      md.Description,
				RotatePolicy:     md.RotatePolicy,
				LatestExpireTime: md.LatestExpireTime,
				NextRotateTime:   md.NextRotateTime,
			}})
	}
	var want string
	if c.secretUri != nil {
		want = c.secretUri.ID
		if md, found := all[want]; found {
			return print(want, md)
		}

	} else {
		want = c.label
		for id, md := range all {
			if md.Label == want {
				return print(id, md)
			}
		}
	}
	return errors.NotFoundf("secret %q", want)
}

// Run implements cmd.Command.
func (c *secretGetCommand) Run(ctx *cmd.Context) error {
	if c.metadata {
		return c.getMetadata(ctx)
	}

	value, err := c.ctx.GetSecret(c.secretUri, c.label, c.update, c.peek)
	if err != nil {
		return err
	}

	var val interface{}
	val, err = value.Values()
	if err != nil {
		return err
	}
	if c.key == "" {
		return c.out.Write(ctx, val)
	}

	val, err = value.KeyValue(c.key)
	if err != nil {
		return err
	}
	return c.out.Write(ctx, val)
}
