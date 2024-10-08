// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"time"

	"github.com/juju/cmd/v4"
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
`
	examples := `
    secret-info-get secret:9m4e2mr0ui3e8a215n4g
    secret-info-get --label db-password
`
	return jujucmd.Info(&cmd.Info{
		Name:     "secret-info-get",
		Args:     "<ID>",
		Purpose:  "Get a secret's metadata info.",
		Doc:      doc,
		Examples: examples,
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
	Access           []accessInfo         `yaml:"access,omitempty" json:"access,omitempty"`
}

// accessInfo holds info about a secret access information.
type accessInfo struct {
	Target string             `yaml:"target" json:"target"`
	Scope  string             `yaml:"scope" json:"scope"`
	Role   secrets.SecretRole `yaml:"role" json:"role"`
}

func toAccessInfo(grants []secrets.AccessInfo) []accessInfo {
	result := make([]accessInfo, len(grants))
	for i, grant := range grants {
		result[i] = accessInfo{
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
				Owner:            string(md.Owner.Kind),
				Description:      md.Description,
				RotatePolicy:     md.RotatePolicy,
				LatestExpireTime: md.LatestExpireTime,
				NextRotateTime:   md.NextRotateTime,
				Access:           toAccessInfo(md.Access),
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
