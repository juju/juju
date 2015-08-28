// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system_test

import (
	"time"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/system"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/testing"
)

type EnvironmentsSuite struct {
	testing.FakeJujuHomeSuite
	api   *fakeEnvMgrAPIClient
	creds *configstore.APICredentials
}

var _ = gc.Suite(&EnvironmentsSuite{})

type fakeEnvMgrAPIClient struct {
	err  error
	user string
	envs []base.UserEnvironment
	all  bool
}

func (f *fakeEnvMgrAPIClient) Close() error {
	return nil
}

func (f *fakeEnvMgrAPIClient) ListEnvironments(user string) ([]base.UserEnvironment, error) {
	if f.err != nil {
		return nil, f.err
	}

	f.user = user
	return f.envs, nil
}

func (f *fakeEnvMgrAPIClient) AllEnvironments() ([]base.UserEnvironment, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.all = true
	return f.envs, nil
}

func (s *EnvironmentsSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)

	err := envcmd.WriteCurrentSystem("fake")
	c.Assert(err, jc.ErrorIsNil)

	last1 := time.Date(2015, 3, 20, 0, 0, 0, 0, time.UTC)
	last2 := time.Date(2015, 3, 1, 0, 0, 0, 0, time.UTC)

	envs := []base.UserEnvironment{
		{
			Name:           "test-env1",
			Owner:          "user-admin@local",
			UUID:           "test-env1-UUID",
			LastConnection: &last1,
		}, {
			Name:           "test-env2",
			Owner:          "user-admin@local",
			UUID:           "test-env2-UUID",
			LastConnection: &last2,
		}, {
			Name:  "test-env3",
			Owner: "user-admin@local",
			UUID:  "test-env3-UUID",
		},
	}
	s.api = &fakeEnvMgrAPIClient{envs: envs}
	s.creds = &configstore.APICredentials{User: "admin@local", Password: "password"}
}

func (s *EnvironmentsSuite) newCommand() cmd.Command {
	command := system.NewEnvironmentsCommand(s.api, s.api, s.creds)
	return envcmd.WrapSystem(command)
}

func (s *EnvironmentsSuite) checkSuccess(c *gc.C, user string, args ...string) {
	context, err := testing.RunCommand(c, s.newCommand(), args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.user, gc.Equals, user)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"NAME       OWNER             LAST CONNECTION\n"+
		"test-env1  user-admin@local  2015-03-20\n"+
		"test-env2  user-admin@local  2015-03-01\n"+
		"test-env3  user-admin@local  never connected\n"+
		"\n")
}

func (s *EnvironmentsSuite) TestEnvironments(c *gc.C) {
	s.checkSuccess(c, "admin@local")
	s.checkSuccess(c, "bob", "--user", "bob")
}

func (s *EnvironmentsSuite) TestAllEnvironments(c *gc.C) {
	context, err := testing.RunCommand(c, s.newCommand(), "--all")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.all, jc.IsTrue)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"NAME       OWNER             LAST CONNECTION\n"+
		"test-env1  user-admin@local  2015-03-20\n"+
		"test-env2  user-admin@local  2015-03-01\n"+
		"test-env3  user-admin@local  never connected\n"+
		"\n")
}

func (s *EnvironmentsSuite) TestEnvironmentsUUID(c *gc.C) {
	context, err := testing.RunCommand(c, s.newCommand(), "--uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.user, gc.Equals, "admin@local")
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"NAME       ENVIRONMENT UUID  OWNER             LAST CONNECTION\n"+
		"test-env1  test-env1-UUID    user-admin@local  2015-03-20\n"+
		"test-env2  test-env2-UUID    user-admin@local  2015-03-01\n"+
		"test-env3  test-env3-UUID    user-admin@local  never connected\n"+
		"\n")
}

func (s *EnvironmentsSuite) TestUnrecognizedArg(c *gc.C) {
	_, err := testing.RunCommand(c, s.newCommand(), "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}

func (s *EnvironmentsSuite) TestEnvironmentsError(c *gc.C) {
	s.api.err = common.ErrPerm
	_, err := testing.RunCommand(c, s.newCommand())
	c.Assert(err, gc.ErrorMatches, "cannot list environments: permission denied")
}
