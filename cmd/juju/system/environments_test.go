// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system_test

import (
	"fmt"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/system"
	"github.com/juju/juju/cmd/syscmd"
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
	envs []string
}

func (f *fakeEnvMgrAPIClient) Close() error {
	return nil
}

func (f *fakeEnvMgrAPIClient) ListEnvironments(user string) ([]params.Environment, error) {
	if f.err != nil {
		return nil, f.err
	}

	f.user = user
	results := make([]params.Environment, len(f.envs))
	for i, envname := range f.envs {
		results[i] = params.Environment{
			Name:     envname,
			OwnerTag: "user-admin@local",
			UUID:     fmt.Sprintf("%s-UUID", envname),
		}
	}
	return results, nil
}

func (s *EnvironmentsSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.api = &fakeEnvMgrAPIClient{envs: []string{"test-env1", "test-env2", "test-env3"}}
	s.creds = &configstore.APICredentials{User: "admin@local", Password: "password"}
}

func (s *EnvironmentsSuite) newCommand() cmd.Command {
	command := system.NewEnvironmentsCommand(s.api, s.creds)
	return syscmd.Wrap(command)
}

func (s *EnvironmentsSuite) TestEnvironments(c *gc.C) {
	context, err := testing.RunCommand(c, s.newCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.user, gc.Equals, "admin@local")
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"NAME       OWNER\n"+
		"test-env1  user-admin@local\n"+
		"test-env2  user-admin@local\n"+
		"test-env3  user-admin@local\n"+
		"\n")
}

func (s *EnvironmentsSuite) TestEnvironmentsUUID(c *gc.C) {
	context, err := testing.RunCommand(c, s.newCommand(), "--uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.user, gc.Equals, "admin@local")
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"NAME       OWNER             ENVIRONMENT UUID\n"+
		"test-env1  user-admin@local  test-env1-UUID\n"+
		"test-env2  user-admin@local  test-env2-UUID\n"+
		"test-env3  user-admin@local  test-env3-UUID\n"+
		"\n")
}

func (s *EnvironmentsSuite) TestEnvironmentsForUser(c *gc.C) {
	context, err := testing.RunCommand(c, s.newCommand(), "--user", "bob")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.user, gc.Equals, "bob")
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"NAME       OWNER\n"+
		"test-env1  user-admin@local\n"+
		"test-env2  user-admin@local\n"+
		"test-env3  user-admin@local\n"+
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
