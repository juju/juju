// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/configstore"
)

// APIInfoCommand returns the fields used to connect to the API server.
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
Returns the address of the current API server formatted as host:port.

Examples:
  $ juju api-endpoints
  10.0.3.1:17070
  $
`

func (c *APIInfoCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "api-info",
		Args:    "",
		Purpose: "Print the API server address",
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
			return errors.Errorf("unknown fields: %v", fields.SortedValues())
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
		return cmd.FormatYaml(field)
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
	}
	return "", errors.Errorf("unknown field %q", name)
}
