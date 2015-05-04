// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system_test

import (
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
		results[i] = params.Environment{Name: envname}
	}
	return results, nil
}

func (s *EnvironmentsSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.api = &fakeEnvMgrAPIClient{envs: []string{"test-env1", "test-env2", "test-env3"}}
	s.creds = &configstore.APICredentials{User: "admin@local", Password: "password"}
}

func (s *EnvironmentsSuite) TestEnvironments(c *gc.C) {
	command := system.NewEnvironmentsCommand(s.api, s.creds)
	context, err := testing.RunCommand(c, syscmd.Wrap(command))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "test-env1\ntest-env2\ntest-env3\n")
}

func (s *EnvironmentsSuite) TestEnvironmentsForUser(c *gc.C) {
	command := system.NewEnvironmentsCommand(s.api, s.creds)
	context, err := testing.RunCommand(c, syscmd.Wrap(command), "--user", "bob")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "test-env1\ntest-env2\ntest-env3\n")
	c.Assert(s.api.user, gc.Equals, "bob")
}

func (s *EnvironmentsSuite) TestUnrecognizedArg(c *gc.C) {
	command := system.NewEnvironmentsCommand(s.api, s.creds)
	_, err := testing.RunCommand(c, syscmd.Wrap(command), "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}

func (s *EnvironmentsSuite) TestEnvironmentsError(c *gc.C) {
	s.api.err = common.ErrPerm
	command := system.NewEnvironmentsCommand(s.api, s.creds)
	_, err := testing.RunCommand(c, syscmd.Wrap(command))
	c.Assert(err, gc.ErrorMatches, "cannot list environments: permission denied")
}
