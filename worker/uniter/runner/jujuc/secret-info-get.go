// Copyright 2022 Canonical Ltd.
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

type secretInfoGetCommand struct {
	cmd.CommandBase
	ctx Context
	out cmd.Output

	secretUri *secrets.URI
	label     string
}

// NewSecretInfoGetCommand returns a command to get secret metadata.
func NewSecretInfoGetCommand(ctx Context) (cmd.Command, error) {
	return &secretInfoGetCommand{ctx: ctx}, nil
}

// Info implements cmd.Command.
func (c *secretInfoGetCommand) Info() *cmd.Info {
	doc := `
Get the metadata of a secret with a given secret ID.
Either the ID or label can be used to identify the secret.

Examples
    secret-info-get secret:9m4e2mr0ui3e8a215n4g
    secret-info-get --label db-password
`
	return jujucmd.Info(&cmd.Info{
		Name:    "secret-info-get",
		Args:    "<ID>",
		Purpose: "get a secret's metadata info",
		Doc:     doc,
	})
}

// SetFlags implements cmd.Command.
func (c *secretInfoGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
	f.StringVar(&c.label, "label", "", "a label used to identify the secret")
}

// Init implements cmd.Command.
func (c *secretInfoGetCommand) Init(args []string) (err error) {
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
	if c.secretUri != nil && c.label != "" {
		return errors.New("specify either a secret URI or label but not both")
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
	Grants           []GrantInfo          `yaml:"grants,omitempty" json:"grants,omitempty"`
}

// GrantInfo holds info about a secret grant.
type GrantInfo struct {
	Target string             `json:"target"`
	Scope  string             `json:"scope"`
	Role   secrets.SecretRole `json:"role"`
}

func toGrantInfo(grants []secrets.GrantInfo) []GrantInfo {
	result := make([]GrantInfo, len(grants))
	for i, grant := range grants {
		result[i] = GrantInfo{
			Target: grant.Target,
			Scope:  grant.Scope,
			Role:   grant.Role,
		}
	}
	return result
}

// Run implements cmd.Command.
func (c *secretInfoGetCommand) Run(ctx *cmd.Context) error {
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
				Grants:           toGrantInfo(md.Grants),
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
