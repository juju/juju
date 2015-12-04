// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"io"

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
	file   *stubFile
}

func (s *UploadSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.client = &StubClient{Stub: s.stub}
	s.file = &stubFile{stub: s.stub}
}

func (s *UploadSuite) NewClient(c *UploadCommand) (UploadClient, error) {
	s.stub.AddCall("NewClient", c)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.client, nil
}

func (s *UploadSuite) OpenResource(path string) (io.ReadCloser, error) {
	s.stub.AddCall("OpenResource", path)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.file, nil
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
		Args:    "service name=file [name2=file2 ...]",
		Purpose: "upload a file as a resource for a service",
		Doc: `
This command uploads a file from your local disk to the juju controller to be
used as a resource for a service.
`,
	})
}

func (s *UploadSuite) TestRun(c *gc.C) {
	u := UploadCommand{
		UploadDeps: UploadDeps{
			NewClient:    s.NewClient,
			OpenResource: s.OpenResource,
		},
		resources: map[string]string{"foo": "bar", "baz": "bat"},
		service:   "svc",
	}

	err := u.Run(nil)
	c.Assert(err, jc.ErrorIsNil)

	checkCall(c, s.stub, "OpenResource", [][]interface{}{
		{"bar"},
		{"bat"},
	})
	checkCall(c, s.stub, "Upload", [][]interface{}{
		{"svc", "foo", s.file},
		{"svc", "baz", s.file},
	})
}

// checkCall checks that the given function has been called exactly len(args)
// times, and that the args passed to the Nth call match args[N].
func checkCall(c *gc.C, stub *testing.Stub, funcname string, args [][]interface{}) {
	var actual [][]interface{}
	for _, call := range stub.Calls() {
		if call.FuncName == funcname {
			actual = append(actual, call.Args)
		}
	}
	c.Assert(actual, jc.DeepEquals, args)
}

type stubFile struct {
	// No one actually tries to read from this during tests.
	io.Reader
	stub *testing.Stub
}

func (s *stubFile) Close() error {
	s.stub.AddCall("FileClose")
	return errors.Trace(s.stub.NextErr())
}
