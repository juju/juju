// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	// "io"
	// "sort"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	// apisecrets "github.com/juju/juju/api/client/secrets"
	// jujucmd "github.com/juju/juju/cmd"
	// "github.com/juju/juju/cmd/juju/common"
	// "github.com/juju/juju/cmd/modelcmd"
	// "github.com/juju/juju/cmd/output"
	"github.com/juju/juju/core/secrets"
)

// SecretCreateArgs specifies args used to create a secret.
// Nil values are not included in the create.
type SecretCreateArgs struct {
	SecretUpdateArgs

	OwnerTag names.Tag
}

// SecretUpdateArgs specifies args used to update a secret.
// Nil values are not included in the update.
type SecretUpdateArgs struct {
	// Value is the new secret value or nil to not update.
	Value secrets.SecretValue

	RotatePolicy *secrets.RotatePolicy
	ExpireTime   *time.Time

	Description *string
	Label       *string
}

// SecretUpsertCommand is the helper base command to create or update a secret.
type SecretUpsertCommand struct {
	cmd.CommandBase

	Owner string
	Data  map[string]string

	rotatePolicy string
	description  string
	label        string
	fileName     string

	expireSpec string
	expireTime time.Time
}

// SetFlags implements cmd.Command.
func (c *SecretUpsertCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.expireSpec, "expire", "", "either a duration or time when the secret should expire")
	f.StringVar(&c.rotatePolicy, "rotate", "", "the secret rotation policy")
	f.StringVar(&c.description, "description", "", "the secret description")
	f.StringVar(&c.label, "label", "", "a label used to identify the secret in hooks")
	f.StringVar(&c.fileName, "file", "", "a YAML file containing secret key values")
	f.StringVar(&c.Owner, "owner", "application", "the owner of the secret, either the application or unit")
}

const rcf3339NoTZ = "2006-01-02T15:04:05"

// Init implements cmd.Command.
func (c *SecretUpsertCommand) Init(args []string) error {
	if c.expireSpec != "" {
		expireTime, err := time.Parse(time.RFC3339, c.expireSpec)
		if err != nil {
			expireTime, err = time.Parse(rcf3339NoTZ, c.expireSpec)
		}
		if err != nil {
			d, err := time.ParseDuration(c.expireSpec)
			if err != nil {
				return errors.NotValidf("expire time or duration %q", c.expireSpec)
			}
			if d <= 0 {
				return errors.NotValidf("negative expire duration %q", c.expireSpec)
			}
			expireTime = time.Now().Add(d)
		}
		c.expireTime = expireTime.UTC()
	}
	if c.rotatePolicy != "" && !secrets.RotatePolicy(c.rotatePolicy).IsValid() {
		return errors.NotValidf("rotate policy %q", c.rotatePolicy)
	}
	if c.Owner != "application" && c.Owner != "unit" {
		return errors.NotValidf("secret owner %q", c.Owner)
	}

	var err error
	c.Data, err = secrets.CreateSecretData(args)
	if err != nil {
		return errors.Trace(err)
	}
	if c.fileName == "" {
		return nil
	}
	dataFromFile, err := secrets.ReadSecretData(c.fileName)
	if err != nil {
		return errors.Trace(err)
	}
	for k, v := range dataFromFile {
		c.Data[k] = v
	}
	return nil
}

// MarshallArg returns the args to create or update a secret.
func (c *SecretUpsertCommand) MarshallArg() *SecretUpdateArgs {
	value := secrets.NewSecretValue(c.Data)
	arg := &SecretUpdateArgs{
		Value: value,
	}
	if c.rotatePolicy != "" {
		p := secrets.RotatePolicy(c.rotatePolicy)
		arg.RotatePolicy = &p
	}
	if !c.expireTime.IsZero() {
		arg.ExpireTime = &c.expireTime
	}
	if c.description != "" {
		arg.Description = &c.description
	}
	if c.label != "" {
		arg.Label = &c.label
	}
	return arg
}
