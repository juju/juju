// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	jujucmd "github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	resourcecmd "github.com/juju/juju/resource/cmd"
)

var _ = gc.Suite(&UploadSuite{})

type UploadSuite struct {
	testing.IsolationSuite

	stub     *testing.Stub
	stubDeps *stubUploadDeps
}

func (s *UploadSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.stubDeps = &stubUploadDeps{
		stub:   s.stub,
		client: &stubAPIClient{stub: s.stub},
	}
}

func (*UploadSuite) TestInitEmpty(c *gc.C) {
	var u resourcecmd.UploadCommand

	err := u.Init([]string{})
	c.Assert(err, jc.Satisfies, errors.IsBadRequest)
}

func (*UploadSuite) TestInitOneArg(c *gc.C) {
	var u resourcecmd.UploadCommand
	err := u.Init([]string{"foo"})
	c.Assert(err, jc.Satisfies, errors.IsBadRequest)
}

func (*UploadSuite) TestInitJustName(c *gc.C) {
	var u resourcecmd.UploadCommand

	err := u.Init([]string{"foo", "bar"})
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (*UploadSuite) TestInitNoName(c *gc.C) {
	var u resourcecmd.UploadCommand

	err := u.Init([]string{"foo", "=foobar"})
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
}

func (*UploadSuite) TestInitNoPath(c *gc.C) {
	var u resourcecmd.UploadCommand

	err := u.Init([]string{"foo", "foobar="})
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
}

func (*UploadSuite) TestInitGood(c *gc.C) {
	var u resourcecmd.UploadCommand

	err := u.Init([]string{"foo", "bar=baz"})
	c.Assert(err, jc.ErrorIsNil)
	svc, name, filename := resourcecmd.UploadCommandResourceFile(&u)
	c.Assert(svc, gc.Equals, "foo")
	c.Assert(name, gc.Equals, "bar")
	c.Assert(filename, gc.Equals, "baz")
	c.Assert(resourcecmd.UploadCommandService(&u), gc.Equals, "foo")
}

func (*UploadSuite) TestInitTwoResources(c *gc.C) {
	var u resourcecmd.UploadCommand

	err := u.Init([]string{"foo", "bar=baz", "fizz=buzz"})
	c.Assert(err, jc.Satisfies, errors.IsBadRequest)
}

func (s *UploadSuite) TestInfo(c *gc.C) {
	var command resourcecmd.UploadCommand
	info := command.Info()

	c.Check(info, jc.DeepEquals, &jujucmd.Info{
		Name:    "attach",
		Args:    "application name=file",
		Purpose: "Upload a file as a resource for an application.",
		Doc: `
This command uploads a file from your local disk to the juju controller to be
used as a resource for an application.
`,
	})
}

func (s *UploadSuite) TestRun(c *gc.C) {
	file := &stubFile{stub: s.stub}
	s.stubDeps.file = file
	u := resourcecmd.NewUploadCommand(resourcecmd.UploadDeps{
		NewClient:    s.stubDeps.NewClient,
		OpenResource: s.stubDeps.OpenResource,
	},
	)
	err := u.Init([]string{"svc", "foo=bar"})
	c.Assert(err, jc.ErrorIsNil)

	err = u.Run(nil)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"NewClient",
		"OpenResource",
		"Upload",
		"FileClose",
		"Close",
	)
	s.stub.CheckCall(c, 1, "OpenResource", "bar")
	s.stub.CheckCall(c, 2, "Upload", "svc", "foo", "bar", file)
}

type stubUploadDeps struct {
	stub   *testing.Stub
	file   resourcecmd.ReadSeekCloser
	client resourcecmd.UploadClient
}

func (s *stubUploadDeps) NewClient(c *resourcecmd.UploadCommand) (resourcecmd.UploadClient, error) {
	s.stub.AddCall("NewClient", c)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.client, nil
}

func (s *stubUploadDeps) OpenResource(path string) (resourcecmd.ReadSeekCloser, error) {
	s.stub.AddCall("OpenResource", path)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.file, nil
}
