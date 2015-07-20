// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/testing"
)

type APIInfoSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&APIInfoSuite{})

func (s *APIInfoSuite) TestArgParsing(c *gc.C) {
	for i, test := range []struct {
		message  string
		args     []string
		refresh  bool
		user     bool
		password bool
		cacert   bool
		servers  bool
		envuuid  bool
		srvuuid  bool
		errMatch string
	}{
		{
			message: "no args skips password",
			user:    true,
			cacert:  true,
			servers: true,
			envuuid: true,
			srvuuid: true,
		}, {
			message:  "password shown if user specifies",
			args:     []string{"--password"},
			user:     true,
			password: true,
			cacert:   true,
			servers:  true,
			envuuid:  true,
			srvuuid:  true,
		}, {
			message: "refresh the cache",
			args:    []string{"--refresh"},
			refresh: true,
			user:    true,
			cacert:  true,
			servers: true,
			envuuid: true,
			srvuuid: true,
		}, {
			message: "just show the user field",
			args:    []string{"user"},
			user:    true,
		}, {
			message:  "just show the password field",
			args:     []string{"password"},
			password: true,
		}, {
			message: "just show the cacert field",
			args:    []string{"ca-cert"},
			cacert:  true,
		}, {
			message: "just show the servers field",
			args:    []string{"state-servers"},
			servers: true,
		}, {
			message: "just show the envuuid field",
			args:    []string{"environ-uuid"},
			envuuid: true,
		}, {
			message: "just show the srvuuid field",
			args:    []string{"server-uuid"},
			srvuuid: true,
		}, {
			message:  "show the user and password field",
			args:     []string{"user", "password"},
			user:     true,
			password: true,
		}, {
			message:  "unknown field field",
			args:     []string{"foo"},
			errMatch: `unknown fields: "foo"`,
		}, {
			message:  "multiple unknown fields",
			args:     []string{"user", "pwd", "foo"},
			errMatch: `unknown fields: "pwd", "foo"`,
		},
	} {
		c.Logf("test %v: %s", i, test.message)
		command := &APIInfoCommand{}
		err := testing.InitCommand(envcmd.Wrap(command), test.args)
		if test.errMatch == "" {
			c.Check(err, jc.ErrorIsNil)
			c.Check(command.refresh, gc.Equals, test.refresh)
			c.Check(command.user, gc.Equals, test.user)
			c.Check(command.password, gc.Equals, test.password)
			c.Check(command.cacert, gc.Equals, test.cacert)
			c.Check(command.servers, gc.Equals, test.servers)
			c.Check(command.envuuid, gc.Equals, test.envuuid)
			c.Check(command.srvuuid, gc.Equals, test.srvuuid)
		} else {
			c.Check(err, gc.ErrorMatches, test.errMatch)
		}
	}
}

func (s *APIInfoSuite) TestOutput(c *gc.C) {
	s.PatchValue(&endpoint, func(c envcmd.EnvCommandBase, refresh bool) (configstore.APIEndpoint, error) {
		return configstore.APIEndpoint{
			Addresses:   []string{"localhost:12345", "10.0.3.1:12345"},
			CACert:      "this is the cacert",
			EnvironUUID: "deadbeef-dead-beef-dead-deaddeaddead",
			ServerUUID:  "bad0f00d-dead-beef-0000-01234567899a",
		}, nil
	})
	s.PatchValue(&creds, func(c envcmd.EnvCommandBase) (configstore.APICredentials, error) {
		return configstore.APICredentials{
			User:     "tester",
			Password: "sekrit",
		}, nil
	})

	for i, test := range []struct {
		args     []string
		output   string
		errMatch string
	}{
		{
			output: "" +
				"user: tester\n" +
				"environ-uuid: deadbeef-dead-beef-dead-deaddeaddead\n" +
				"server-uuid: bad0f00d-dead-beef-0000-01234567899a\n" +
				"state-servers:\n" +
				"- localhost:12345\n" +
				"- 10.0.3.1:12345\n" +
				"ca-cert: this is the cacert\n",
		}, {
			args: []string{"--password"},
			output: "" +
				"user: tester\n" +
				"password: sekrit\n" +
				"environ-uuid: deadbeef-dead-beef-dead-deaddeaddead\n" +
				"server-uuid: bad0f00d-dead-beef-0000-01234567899a\n" +
				"state-servers:\n" +
				"- localhost:12345\n" +
				"- 10.0.3.1:12345\n" +
				"ca-cert: this is the cacert\n",
		}, {
			args: []string{"--format=yaml"},
			output: "" +
				"user: tester\n" +
				"environ-uuid: deadbeef-dead-beef-dead-deaddeaddead\n" +
				"server-uuid: bad0f00d-dead-beef-0000-01234567899a\n" +
				"state-servers:\n" +
				"- localhost:12345\n" +
				"- 10.0.3.1:12345\n" +
				"ca-cert: this is the cacert\n",
		}, {
			args: []string{"--format=json"},
			output: `{"user":"tester",` +
				`"environ-uuid":"deadbeef-dead-beef-dead-deaddeaddead",` +
				`"server-uuid":"bad0f00d-dead-beef-0000-01234567899a",` +
				`"state-servers":["localhost:12345","10.0.3.1:12345"],` +
				`"ca-cert":"this is the cacert"}` + "\n",
		}, {
			args:   []string{"user"},
			output: "tester\n",
		}, {
			args: []string{"user", "password"},
			output: "" +
				"user: tester\n" +
				"password: sekrit\n",
		}, {
			args: []string{"state-servers"},
			output: "" +
				"localhost:12345\n" +
				"10.0.3.1:12345\n",
		}, {
			args:   []string{"--format=yaml", "user"},
			output: "user: tester\n",
		}, {
			args: []string{"--format=yaml", "user", "password"},
			output: "" +
				"user: tester\n" +
				"password: sekrit\n",
		}, {
			args: []string{"--format=yaml", "state-servers"},
			output: "" +
				"state-servers:\n" +
				"- localhost:12345\n" +
				"- 10.0.3.1:12345\n",
		}, {
			args:   []string{"--format=json", "user"},
			output: `{"user":"tester"}` + "\n",
		}, {
			args:   []string{"--format=json", "user", "password"},
			output: `{"user":"tester","password":"sekrit"}` + "\n",
		}, {
			args:   []string{"--format=json", "state-servers"},
			output: `{"state-servers":["localhost:12345","10.0.3.1:12345"]}` + "\n",
		},
	} {
		c.Logf("test %v: %v", i, test.args)
		command := &APIInfoCommand{}
		ctx, err := testing.RunCommand(c, envcmd.Wrap(command), test.args...)
		if test.errMatch == "" {
			c.Check(err, jc.ErrorIsNil)
			c.Check(testing.Stdout(ctx), gc.Equals, test.output)
		} else {
			c.Check(err, gc.ErrorMatches, test.errMatch)
		}
	}
}

func (s *APIInfoSuite) TestOutputNoServerUUID(c *gc.C) {
	s.PatchValue(&endpoint, func(c envcmd.EnvCommandBase, refresh bool) (configstore.APIEndpoint, error) {
		return configstore.APIEndpoint{
			Addresses:   []string{"localhost:12345", "10.0.3.1:12345"},
			CACert:      "this is the cacert",
			EnvironUUID: "deadbeef-dead-beef-dead-deaddeaddead",
		}, nil
	})
	s.PatchValue(&creds, func(c envcmd.EnvCommandBase) (configstore.APICredentials, error) {
		return configstore.APICredentials{
			User:     "tester",
			Password: "sekrit",
		}, nil
	})

	expected := "" +
		"user: tester\n" +
		"environ-uuid: deadbeef-dead-beef-dead-deaddeaddead\n" +
		"state-servers:\n" +
		"- localhost:12345\n" +
		"- 10.0.3.1:12345\n" +
		"ca-cert: this is the cacert\n"
	command := &APIInfoCommand{}
	ctx, err := testing.RunCommand(c, envcmd.Wrap(command))
	c.Check(err, jc.ErrorIsNil)
	c.Check(testing.Stdout(ctx), gc.Equals, expected)
}

func (s *APIInfoSuite) TestEndpointError(c *gc.C) {
	s.PatchValue(&endpoint, func(c envcmd.EnvCommandBase, refresh bool) (configstore.APIEndpoint, error) {
		return configstore.APIEndpoint{}, fmt.Errorf("oops, no endpoint")
	})
	s.PatchValue(&creds, func(c envcmd.EnvCommandBase) (configstore.APICredentials, error) {
		return configstore.APICredentials{}, nil
	})
	command := &APIInfoCommand{}
	_, err := testing.RunCommand(c, envcmd.Wrap(command))
	c.Assert(err, gc.ErrorMatches, "oops, no endpoint")
}

func (s *APIInfoSuite) TestCredentialsError(c *gc.C) {
	s.PatchValue(&endpoint, func(c envcmd.EnvCommandBase, refresh bool) (configstore.APIEndpoint, error) {
		return configstore.APIEndpoint{}, nil
	})
	s.PatchValue(&creds, func(c envcmd.EnvCommandBase) (configstore.APICredentials, error) {
		return configstore.APICredentials{}, fmt.Errorf("oops, no creds")
	})
	command := &APIInfoCommand{}
	_, err := testing.RunCommand(c, envcmd.Wrap(command))
	c.Assert(err, gc.ErrorMatches, "oops, no creds")
}

func (s *APIInfoSuite) TestNoEnvironment(c *gc.C) {
	command := &APIInfoCommand{}
	_, err := testing.RunCommand(c, envcmd.Wrap(command))
	c.Assert(err, gc.ErrorMatches, `environment "erewhemos" not found`)
}
