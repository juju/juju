// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/configstore"
)

// APIInfoCommand returns the fields used to connect to an API server.
type APIInfoCommand struct {
	envcmd.EnvCommandBase
	out      cmd.Output
	refresh  bool
	user     bool
	password bool
	cacert   bool
	servers  bool
	envuuid  bool
	fields   []string
}

const apiInfoDoc = `
Returns the values of the various fields used to connect to an API server.

By default the password is not shown in the result.  If the password is specified
explicitly, or through the --password option, the value is included.

By specifying individual fields, the user is able to return just those fields.
The valid field options are:
  user
  password
  environ-uuid
  state-servers
  ca-cert


Examples:
  $ juju api-info user
  admin

  $ juju api-info user password
  user: admin
  password: sekrit


`

func (c *APIInfoCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "api-info",
		Args:    "",
		Purpose: "print the field values used to connect to the API",
		Doc:     apiInfoDoc,
	}
}

func (c *APIInfoCommand) Init(args []string) error {
	c.fields = args
	if len(args) > 0 {
		fields := set.NewStrings(args...)
		for _, name := range args {
			switch name {
			case "user":
				c.user = true
			case "password":
				c.password = true
			case "environ-uuid":
				c.envuuid = true
			case "state-servers":
				c.servers = true
			case "ca-cert":
				c.cacert = true
			default:
				continue
			}
			fields.Remove(name)
		}
		if fields.Size() > 0 {
			var quoted []string
			for _, field := range fields.SortedValues() {
				quoted = append(quoted, fmt.Sprintf("%q", field))
			}
			return errors.Errorf("unknown fields: %s", strings.Join(quoted, ", "))
		}

	} else {
		c.user = true
		c.envuuid = true
		c.servers = true
		c.cacert = true
	}
	return nil
}

func (c *APIInfoCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "default", map[string]cmd.Formatter{
		"default": c.format,
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
	})
	f.BoolVar(&c.refresh, "refresh", false, "connect to the API to ensure an up-to-date endpoint location")
	f.BoolVar(&c.password, "password", false, "include the password in the output fields")
}

var (
	endpoint = func(c envcmd.EnvCommandBase, refresh bool) (configstore.APIEndpoint, error) {
		return c.ConnectionEndpoint(refresh)
	}
	creds = func(c envcmd.EnvCommandBase) (configstore.APICredentials, error) {
		return c.ConnectionCredentials()
	}
)

// Print out the addresses of the API server endpoints.
func (c *APIInfoCommand) Run(ctx *cmd.Context) error {
	apiendpoint, err := endpoint(c.EnvCommandBase, c.refresh)
	if err != nil {
		return err
	}
	credentials, err := creds(c.EnvCommandBase)
	if err != nil {
		return err
	}

	var result InfoData
	if c.user {
		result.User = credentials.User
	}
	if c.password {
		result.Password = credentials.Password
	}
	if c.envuuid {
		result.EnvironUUID = apiendpoint.EnvironUUID
	}
	if c.servers {
		result.StateServers = apiendpoint.Addresses
	}
	if c.cacert {
		result.CACert = apiendpoint.CACert
	}

	return c.out.Write(ctx, result)
}

func (c *APIInfoCommand) format(value interface{}) ([]byte, error) {
	if count := len(c.fields); count == 1 {
		data := value.(InfoData)
		field, err := data.field(c.fields[0])
		if err != nil {
			return nil, err
		}
		if value, ok := field.([]string); ok {
			return []byte(strings.Join(value, "\n")), nil
		}
		return []byte(field.(string)), nil
	}

	return cmd.FormatYaml(value)
}

type InfoData struct {
	User         string   `json:"user,omitempty" yaml:",omitempty"`
	Password     string   `json:"password,omitempty" yaml:",omitempty"`
	EnvironUUID  string   `json:"environ-uuid,omitempty" yaml:"environ-uuid,omitempty"`
	StateServers []string `json:"state-servers,omitempty" yaml:"state-servers,omitempty"`
	CACert       string   `json:"ca-cert,omitempty" yaml:"ca-cert,omitempty"`
}

func (i *InfoData) field(name string) (interface{}, error) {
	switch name {
	case "user":
		return i.User, nil
	case "password":
		return i.Password, nil
	case "environ-uuid":
		return i.EnvironUUID, nil
	case "state-servers":
		return i.StateServers, nil
	case "ca-cert":
		return i.CACert, nil
	default:
		return "", errors.Errorf("unknown field %q", name)
	}
}
