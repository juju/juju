// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"io"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/core/secrets"

	jujucmd "github.com/juju/juju/cmd"
)

type secretGetCommand struct {
	cmd.CommandBase
	ctx Context
	out cmd.Output

	secretUrl *secrets.URL
	asBase64  bool
}

// NewSecretGetCommand returns a command to get a secret value.
func NewSecretGetCommand(ctx Context) (cmd.Command, error) {
	return &secretGetCommand{ctx: ctx}, nil
}

// Info implements cmd.Command.
func (c *secretGetCommand) Info() *cmd.Info {
	doc := `
Get the value of a secret with a given secret ID.
For secrets with a singular value, the decoded string
is printed, unless --base64 is specified, in which case
the encoded base64 value is printed.
Multiple key value secrets are printed as YAML.
`
	return jujucmd.Info(&cmd.Info{
		Name:    "secret-get",
		Args:    "<ID>",
		Purpose: "get the value of a secret",
		Doc:     doc,
	})
}

// SetFlags implements cmd.Command.
func (c *secretGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "plain", map[string]cmd.Formatter{
		"yaml":  cmd.FormatYaml,
		"json":  cmd.FormatJson,
		"plain": printPlainOutput,
	})
	f.BoolVar(&c.asBase64, "base64", false,
		`print the singular secret value as the base64 encoded string`)
}

func printPlainOutput(writer io.Writer, val interface{}) error {
	var str string
	switch v := val.(type) {
	case string:
		str = v
	default:
		return cmd.FormatYaml(writer, val)
	}
	fmt.Fprintf(writer, str)
	return nil
}

// Init implements cmd.Command.
func (c *secretGetCommand) Init(args []string) (err error) {
	if len(args) < 1 {
		return errors.New("missing secret name")
	}
	c.secretUrl, err = secrets.ParseURL(args[0])
	if err != nil {
		return errors.NotValidf("secret URL %q", args[0])
	}
	return cmd.CheckEmpty(args[1:])
}

// Run implements cmd.Command.
func (c *secretGetCommand) Run(ctx *cmd.Context) error {
	value, err := c.ctx.GetSecret(c.secretUrl.String())
	if err != nil {
		return err
	}

	var val interface{}
	if c.asBase64 {
		if value.Singular() {
			val, _ = value.EncodedValue()
		} else {
			valMap := value.EncodedValues()
			if c.secretUrl.Attribute != "" {
				val = valMap[c.secretUrl.Attribute]
			} else {
				val = valMap
			}
		}
	} else {
		if value.Singular() {
			val, err = value.Value()
			if err != nil {
				return err
			}
		} else {
			valMap, err := value.Values()
			if err != nil {
				return err
			}
			if c.secretUrl.Attribute != "" {
				val = valMap[c.secretUrl.Attribute]
			} else {
				val = valMap
			}
		}
	}
	return c.out.Write(ctx, val)
}
