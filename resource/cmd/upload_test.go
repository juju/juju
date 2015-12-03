// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	jujucmd "github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

var _ = gc.Suite(&UploadSuite{})

type UploadSuite struct {
	testing.IsolationSuite

	stub   *testing.Stub
	client *StubClient
}

func (s *UploadSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.client = &StubClient{Stub: s.stub}
}

func (s *UploadSuite) newAPIClient(c *ShowCommand) (ShowAPI, error) {
	s.stub.AddCall("newAPIClient", c)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.client, nil
}

func (*UploadSuite) TestAddResource(c *gc.C) {
	u := UploadCommand{resources: map[string]string{}}

	err := u.addResource("foo=bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.resources, gc.DeepEquals, map[string]string{"foo": "bar"})

	err = u.addResource("foo=bar")
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsAlreadyExists)

	err = u.addResource("foobar")
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsNotValid)

	err = u.addResource("=foobar")
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsNotValid)

	err = u.addResource("foobar=")
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsNotValid)

	err = u.addResource("baz=bat")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.resources, gc.DeepEquals, map[string]string{
		"foo": "bar",
		"baz": "bat",
	})
}

func (*UploadSuite) TestInit(c *gc.C) {
	u := UploadCommand{resources: map[string]string{}}

	err := u.Init([]string{})
	c.Assert(err, jc.Satisfies, errors.IsBadRequest)

	err = u.Init([]string{"foo"})
	c.Assert(err, jc.Satisfies, errors.IsBadRequest)

	// full testing of bad resources is tested in TestAddResource, this just
	// tests that we're actually passing the errors through.
	err = u.Init([]string{"foo", "bar"})
	c.Assert(err, jc.Satisfies, errors.IsNotValid)

	err = u.Init([]string{"foo", "bar=baz"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.resources, gc.DeepEquals, map[string]string{"bar": "baz"})
	c.Assert(u.service, gc.Equals, "foo")

	u = UploadCommand{resources: map[string]string{}}

	err = u.Init([]string{"foo", "bar=baz", "fizz=buzz"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.resources, gc.DeepEquals, map[string]string{
		"bar":  "baz",
		"fizz": "buzz",
	})
}

func (s *UploadSuite) TestInfo(c *gc.C) {
	var command UploadCommand
	info := command.Info()

	c.Check(info, jc.DeepEquals, &jujucmd.Info{
		Name:    "upload",
		Args:    "service-name",
		Purpose: "upload a file as a resource for a service",
		Doc: `
This command uploads a file from your local disk to the juju controller to be
used as a resource for a service.
`,
	})
}
