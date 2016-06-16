// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/errors"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/testing"
)

type GetSharedConfigSuite struct {
	baseControllerSuite
}

var _ = gc.Suite(&GetSharedConfigSuite{})

func (s *GetSharedConfigSuite) SetUpTest(c *gc.C) {
	s.baseControllerSuite.SetUpTest(c)
	s.createTestClientStore(c)
}

func (s *GetSharedConfigSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command := controller.NewGetSharedConfigCommandForTest(&fakeControllerAPI{}, s.store)
	return testing.RunCommand(c, command, args...)
}

func (s *GetSharedConfigSuite) TestInit(c *gc.C) {
	// zero or one args is fine.
	err := testing.InitCommand(controller.NewGetSharedConfigCommandForTest(&fakeControllerAPI{}, s.store), nil)
	c.Check(err, jc.ErrorIsNil)
	err = testing.InitCommand(controller.NewGetSharedConfigCommandForTest(&fakeControllerAPI{}, s.store), []string{"one"})
	c.Check(err, jc.ErrorIsNil)
	// More than one is not allowed.
	err = testing.InitCommand(controller.NewGetSharedConfigCommandForTest(&fakeControllerAPI{}, s.store), []string{"one", "two"})
	c.Check(err, gc.ErrorMatches, `unrecognized args: \["two"\]`)
}

func (s *GetSharedConfigSuite) TestSingleValue(c *gc.C) {
	context, err := s.run(c, "apt-mirror")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(testing.Stdout(context))
	c.Assert(output, gc.Equals, "http://mirror")
}

func (s *GetSharedConfigSuite) TestSingleValueJSON(c *gc.C) {
	context, err := s.run(c, "--format=json", "apt-mirror")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(testing.Stdout(context))
	c.Assert(output, gc.Equals, `"http://mirror"`)
}

func (s *GetSharedConfigSuite) TestAllValues(c *gc.C) {
	context, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(testing.Stdout(context))
	expected := "" +
		"apt-mirror: http://mirror\n" +
		"http-proxy: http://proxy"
	c.Assert(output, gc.Equals, expected)
}

func (s *GetSharedConfigSuite) TestAllValuesJSON(c *gc.C) {
	context, err := s.run(c, "--format=json")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(testing.Stdout(context))
	expected := `{"apt-mirror":"http://mirror","http-proxy":"http://proxy"}`
	c.Assert(output, gc.Equals, expected)
}

func (s *GetSharedConfigSuite) TestError(c *gc.C) {
	command := controller.NewGetSharedConfigCommandForTest(&fakeControllerAPI{err: errors.New("error")}, s.store)
	_, err := testing.RunCommand(c, command)
	c.Assert(err, gc.ErrorMatches, "error")
}

func (f *fakeControllerAPI) DefaultModelConfig() (map[string]interface{}, error) {
	if f.err != nil {
		return nil, f.err
	}
	return map[string]interface{}{
		"apt-mirror": "http://mirror",
		"http-proxy": "http://proxy",
	}, nil
}
