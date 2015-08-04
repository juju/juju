// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system_test

import (
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/system"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/testing"
)

const (
	serverUUID = "0dbfe161-de6c-47ad-9283-5e3ea64e1dd3"
	env1UUID   = "ebf03329-cdad-44a5-9f10-fe318efda3ce"
	env2UUID   = "b366cdd5-82da-49a1-ac18-001f26bb59a3"
	env3UUID   = "fd0f57a3-eb94-4095-9ab0-d1f6042f942a"
	env4UUID   = "1e45141b-85cb-4a0a-96ef-0aa6bbeac45a"
)

type UseEnvironmentSuite struct {
	testing.FakeJujuHomeSuite
	api      *fakeEnvMgrAPIClient
	creds    configstore.APICredentials
	endpoint configstore.APIEndpoint
}

var _ = gc.Suite(&UseEnvironmentSuite{})

func (s *UseEnvironmentSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)

	err := envcmd.WriteCurrentSystem("fake")
	c.Assert(err, jc.ErrorIsNil)

	envs := []base.UserEnvironment{{
		Name:  "unique",
		Owner: "tester@local",
		UUID:  "some-uuid",
	}, {
		Name:  "test",
		Owner: "tester@local",
		UUID:  env1UUID,
	}, {
		Name:  "test",
		Owner: "bob@local",
		UUID:  env2UUID,
	}, {
		Name:  "other",
		Owner: "bob@local",
		UUID:  env3UUID,
	}, {
		Name:  "other",
		Owner: "bob@remote",
		UUID:  env4UUID,
	}}
	s.api = &fakeEnvMgrAPIClient{envs: envs}
	s.creds = configstore.APICredentials{User: "tester", Password: "password"}
	s.endpoint = configstore.APIEndpoint{
		Addresses:  []string{"127.0.0.1:12345"},
		Hostnames:  []string{"localhost:12345"},
		CACert:     testing.CACert,
		ServerUUID: serverUUID,
	}
}

func (s *UseEnvironmentSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command := system.NewUseEnvironmentCommand(s.api, &s.creds, &s.endpoint)
	return testing.RunCommand(c, envcmd.WrapSystem(command), args...)
}

func (s *UseEnvironmentSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		args        []string
		errorString string
		localName   string
		owner       string
		envName     string
		envUUID     string
	}{{
		errorString: "no environment supplied",
	}, {
		args:        []string{""},
		errorString: "no environment supplied",
	}, {
		args:    []string{"env-name"},
		envName: "env-name",
	}, {
		args:      []string{"env-name", "--name", "foo"},
		localName: "foo",
		envName:   "env-name",
	}, {
		args:    []string{"user/foobar"},
		envName: "foobar",
		owner:   "user",
	}, {
		args:    []string{"user@local/foobar"},
		envName: "foobar",
		owner:   "user@local",
	}, {
		args:    []string{"user@remote/foobar"},
		envName: "foobar",
		owner:   "user@remote",
	}, {
		args:        []string{"+user+name/foobar"},
		errorString: `"\+user\+name" is not a valid user`,
	}, {
		args:    []string{env1UUID},
		envUUID: env1UUID,
	}, {
		args:    []string{"user/" + env1UUID},
		owner:   "user",
		envUUID: env1UUID,
	}} {
		c.Logf("test %d", i)
		command := &system.UseEnvironmentCommand{}
		err := testing.InitCommand(command, test.args)
		if test.errorString == "" {
			c.Check(command.LocalName, gc.Equals, test.localName)
			c.Check(command.EnvName, gc.Equals, test.envName)
			c.Check(command.EnvUUID, gc.Equals, test.envUUID)
			c.Check(command.Owner, gc.Equals, test.owner)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorString)
		}
	}
}

func (s *UseEnvironmentSuite) TestEnvironmentsError(c *gc.C) {
	s.api.err = common.ErrPerm
	_, err := s.run(c, "ignored-but-needed")
	c.Assert(err, gc.ErrorMatches, "cannot list environments: permission denied")
}

func (s *UseEnvironmentSuite) TestNameNotFound(c *gc.C) {
	_, err := s.run(c, "missing")
	c.Assert(err, gc.ErrorMatches, "matching environment not found")
}

func (s *UseEnvironmentSuite) TestUUID(c *gc.C) {
	_, err := s.run(c, env3UUID)
	c.Assert(err, gc.IsNil)

	s.assertCurrentEnvironment(c, "bob-other", env3UUID)
}

func (s *UseEnvironmentSuite) TestUUIDCorrectOwner(c *gc.C) {
	_, err := s.run(c, "bob/"+env3UUID)
	c.Assert(err, gc.IsNil)

	s.assertCurrentEnvironment(c, "bob-other", env3UUID)
}

func (s *UseEnvironmentSuite) TestUUIDWrongOwner(c *gc.C) {
	ctx, err := s.run(c, "charles/"+env3UUID)
	c.Assert(err, gc.IsNil)
	expected := "Specified environment owned by bob@local, not charles@local"
	c.Assert(testing.Stderr(ctx), jc.Contains, expected)

	s.assertCurrentEnvironment(c, "bob-other", env3UUID)
}

func (s *UseEnvironmentSuite) TestUniqueName(c *gc.C) {
	_, err := s.run(c, "unique")
	c.Assert(err, gc.IsNil)

	s.assertCurrentEnvironment(c, "unique", "some-uuid")
}

func (s *UseEnvironmentSuite) TestMultipleNameMatches(c *gc.C) {
	ctx, err := s.run(c, "test")
	c.Assert(err, gc.ErrorMatches, "multiple environments matched")

	message := strings.TrimSpace(testing.Stderr(ctx))
	lines := strings.Split(message, "\n")
	c.Assert(lines, gc.HasLen, 4)
	c.Assert(lines[0], gc.Equals, `Multiple environments matched name "test":`)
	c.Assert(lines[1], gc.Equals, "  "+env1UUID+", owned by tester@local")
	c.Assert(lines[2], gc.Equals, "  "+env2UUID+", owned by bob@local")
	c.Assert(lines[3], gc.Equals, `Please specify either the environment UUID or the owner to disambiguate.`)
}

func (s *UseEnvironmentSuite) TestUserOwnerOfEnvironment(c *gc.C) {
	_, err := s.run(c, "tester/test")
	c.Assert(err, gc.IsNil)

	s.assertCurrentEnvironment(c, "test", env1UUID)
}

func (s *UseEnvironmentSuite) TestOtherUsersEnvironment(c *gc.C) {
	_, err := s.run(c, "bob/test")
	c.Assert(err, gc.IsNil)

	s.assertCurrentEnvironment(c, "bob-test", env2UUID)
}

func (s *UseEnvironmentSuite) TestRemoteUsersEnvironmentName(c *gc.C) {
	_, err := s.run(c, "bob@remote/other")
	c.Assert(err, gc.IsNil)

	s.assertCurrentEnvironment(c, "bob-other", env4UUID)
}

func (s *UseEnvironmentSuite) TestDisambiguateWrongOwner(c *gc.C) {
	_, err := s.run(c, "wrong/test")
	c.Assert(err, gc.ErrorMatches, "matching environment not found")
}

func (s *UseEnvironmentSuite) TestUseEnvAlreadyExisting(c *gc.C) {
	s.makeLocalEnvironment(c, "unique", "", "")
	ctx, err := s.run(c, "unique")
	c.Assert(err, gc.ErrorMatches, "existing environment")
	expected := `You have an existing environment called "unique", use --name to specify a different local name.`
	c.Assert(testing.Stderr(ctx), jc.Contains, expected)
}

func (s *UseEnvironmentSuite) TestUseEnvAlreadyExistingSameEnv(c *gc.C) {
	s.makeLocalEnvironment(c, "unique", "some-uuid", "tester")
	ctx, err := s.run(c, "unique")
	c.Assert(err, gc.IsNil)

	message := strings.TrimSpace(testing.Stderr(ctx))
	lines := strings.Split(message, "\n")
	c.Assert(lines, gc.HasLen, 2)

	expected := `You already have environment details for "unique" cached locally.`
	c.Assert(lines[0], gc.Equals, expected)
	c.Assert(lines[1], gc.Equals, `fake (system) -> unique`)

	current, err := envcmd.ReadCurrentEnvironment()
	c.Assert(err, gc.IsNil)
	c.Assert(current, gc.Equals, "unique")
}

func (s *UseEnvironmentSuite) assertCurrentEnvironment(c *gc.C, name, uuid string) {
	current, err := envcmd.ReadCurrentEnvironment()
	c.Assert(err, gc.IsNil)
	c.Assert(current, gc.Equals, name)

	store, err := configstore.Default()
	c.Assert(err, gc.IsNil)

	info, err := store.ReadInfo(name)
	c.Assert(err, gc.IsNil)
	c.Assert(info.APIEndpoint(), jc.DeepEquals, configstore.APIEndpoint{
		Addresses:   []string{"127.0.0.1:12345"},
		Hostnames:   []string{"localhost:12345"},
		CACert:      testing.CACert,
		EnvironUUID: uuid,
		ServerUUID:  serverUUID,
	})
	c.Assert(info.APICredentials(), jc.DeepEquals, s.creds)
}

func (s *UseEnvironmentSuite) makeLocalEnvironment(c *gc.C, name, uuid, owner string) {
	store, err := configstore.Default()
	c.Assert(err, gc.IsNil)

	if uuid == "" {
		uuid = utils.MustNewUUID().String()
	}
	if owner == "" {
		owner = "random@person"
	}
	info := store.CreateInfo(name)
	info.SetAPIEndpoint(configstore.APIEndpoint{
		EnvironUUID: uuid,
	})
	info.SetAPICredentials(configstore.APICredentials{
		User: owner,
	})
	err = info.Write()
	c.Assert(err, gc.IsNil)
}
