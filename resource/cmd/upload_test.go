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
	var u UploadCommand

	err := u.Init([]string{})
	c.Assert(err, jc.Satisfies, errors.IsBadRequest)
}

func (*UploadSuite) TestInitOneArg(c *gc.C) {
	var u UploadCommand
	err := u.Init([]string{"foo"})
	c.Assert(err, jc.Satisfies, errors.IsBadRequest)
}

func (*UploadSuite) TestInitJustName(c *gc.C) {
	var u UploadCommand

	err := u.Init([]string{"foo", "bar"})
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (*UploadSuite) TestInitNoName(c *gc.C) {
	var u UploadCommand

	err := u.Init([]string{"foo", "=foobar"})
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
}

func (*UploadSuite) TestInitNoPath(c *gc.C) {
	var u UploadCommand

	err := u.Init([]string{"foo", "foobar="})
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
}

func (*UploadSuite) TestInitGood(c *gc.C) {
	var u UploadCommand

	err := u.Init([]string{"foo", "bar=baz"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.resourceFile, gc.DeepEquals, resourceFile{
		service:  "foo",
		name:     "bar",
		filename: "baz",
	})
	c.Assert(u.service, gc.Equals, "foo")
}

func (*UploadSuite) TestInitTwoResources(c *gc.C) {
	var u UploadCommand

	err := u.Init([]string{"foo", "bar=baz", "fizz=buzz"})
	c.Assert(err, jc.Satisfies, errors.IsBadRequest)
}

func (s *UploadSuite) TestInfo(c *gc.C) {
	var command UploadCommand
	info := command.Info()

	c.Check(info, jc.DeepEquals, &jujucmd.Info{
		Name:    "push-resource",
		Args:    "service name=file",
		Purpose: "upload a file as a resource for a service",
		Doc: `
This command uploads a file from your local disk to the juju controller to be
used as a resource for a service.
`,
	})
}

func (s *UploadSuite) TestRun(c *gc.C) {
	file := &stubFile{stub: s.stub}
	s.stubDeps.file = file
	u := UploadCommand{
		deps: UploadDeps{
			NewClient:    s.stubDeps.NewClient,
			OpenResource: s.stubDeps.OpenResource,
		},
		resourceFile: resourceFile{
			service:  "svc",
			name:     "foo",
			filename: "bar",
		},
		service: "svc",
	}

	err := u.Run(nil)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"NewClient",
		"OpenResource",
		"Upload",
		"FileClose",
		"Close",
	)
	s.stub.CheckCall(c, 1, "OpenResource", "bar")
	s.stub.CheckCall(c, 2, "Upload", "svc", "foo", file)
}

type stubUploadDeps struct {
	stub   *testing.Stub
	file   ReadSeekCloser
	client UploadClient
}

func (s *stubUploadDeps) NewClient(c *UploadCommand) (UploadClient, error) {
	s.stub.AddCall("NewClient", c)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.client, nil
}

func (s *stubUploadDeps) OpenResource(path string) (ReadSeekCloser, error) {
	s.stub.AddCall("OpenResource", path)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.file, nil
}
