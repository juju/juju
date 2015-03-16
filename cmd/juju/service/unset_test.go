// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	"bytes"
	"strings"

	"github.com/juju/cmd"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/service"
	coretesting "github.com/juju/juju/testing"
)

type UnsetSuite struct {
	coretesting.FakeJujuHomeSuite
	dir  string
	fake *fakeServiceAPI
}

var _ = gc.Suite(&UnsetSuite{})

func (s *UnsetSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.fake = &fakeServiceAPI{servName: "dummy-service", values: map[string]interface{}{
		"username": "hello",
		"outlook":  "hello@world.tld",
	}}

	s.dir = c.MkDir()
	setupConfigFile(c, s.dir)
}

func (s *UnsetSuite) TestUnsetCommandInit(c *gc.C) {
	// missing args
	err := coretesting.InitCommand(&service.UnsetCommand{}, []string{})
	c.Assert(err, gc.ErrorMatches, "no service name specified")
}

func (s *UnsetSuite) TestUnsetOptionOneByOneSuccess(c *gc.C) {
	// Unset one by one.
	s.assertUnsetSuccess(c, s.dir, []string{"username"}, map[string]interface{}{
		"outlook": "hello@world.tld",
	})
	s.assertUnsetSuccess(c, s.dir, []string{"outlook"}, make(map[string]interface{}))
}

func (s *UnsetSuite) TestBlockUnset(c *gc.C) {
	// Block operation
	s.fake.err = common.ErrOperationBlocked("TestBlockUnset")
	ctx := coretesting.ContextForDir(c, s.dir)
	code := cmd.Main(envcmd.Wrap(service.NewUnsetCommand(s.fake)), ctx, []string{
		"dummy-service",
		"username"})
	c.Check(code, gc.Equals, 1)

	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, gc.Matches, ".*TestBlockUnset.*")
}

func (s *UnsetSuite) TestUnsetOptionMultipleAtOnceSuccess(c *gc.C) {
	// Unset multiple options at once.
	s.assertUnsetSuccess(c, s.dir, []string{"username", "outlook"}, make(map[string]interface{}))
}

func (s *UnsetSuite) TestUnsetOptionFail(c *gc.C) {
	s.assertUnsetFail(c, s.dir, []string{}, "error: no configuration options specified\n")
	s.assertUnsetFail(c, s.dir, []string{"invalid"}, "error: unknown option \"invalid\"\n")
	s.assertUnsetFail(c, s.dir, []string{"username=bar"}, "error: unknown option \"username=bar\"\n")
	s.assertUnsetFail(c, s.dir, []string{
		"username",
		"outlook",
		"invalid",
	}, "error: unknown option \"invalid\"\n")
}

// assertUnsetSuccess unsets configuration options and checks the expected settings.
func (s *UnsetSuite) assertUnsetSuccess(c *gc.C, dir string, args []string, expect map[string]interface{}) {
	ctx := coretesting.ContextForDir(c, dir)
	code := cmd.Main(envcmd.Wrap(service.NewUnsetCommand(s.fake)), ctx, append([]string{"dummy-service"}, args...))
	c.Check(code, gc.Equals, 0)
	c.Assert(s.fake.values, gc.DeepEquals, expect)
}

// assertUnsetFail unsets configuration options and checks the expected error.
func (s *UnsetSuite) assertUnsetFail(c *gc.C, dir string, args []string, err string) {
	ctx := coretesting.ContextForDir(c, dir)
	code := cmd.Main(envcmd.Wrap(service.NewUnsetCommand(s.fake)), ctx, append([]string{"dummy-service"}, args...))
	c.Check(code, gc.Not(gc.Equals), 0)
	c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Matches, err)
}
