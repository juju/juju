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

	stubDeps *stubUploadDeps
}

func (s *UploadSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	stub := &testing.Stub{}
	s.stubDeps = &stubUploadDeps{
		stub:   stub,
		file:   &stubFile{stub: stub},
		client: &StubClient{Stub: stub},
	}
}

func (*UploadSuite) TestInitEmpty(c *gc.C) {
	u := UploadCommand{resources: map[string]string{}}

	err := u.Init([]string{})
	c.Assert(err, jc.Satisfies, errors.IsBadRequest)
}

func (*UploadSuite) TestInitOneArg(c *gc.C) {
	u := UploadCommand{resources: map[string]string{}}
	err := u.Init([]string{"foo"})
	c.Assert(err, jc.Satisfies, errors.IsBadRequest)
}

func (*UploadSuite) TestInitJustName(c *gc.C) {
	u := UploadCommand{resources: map[string]string{}}

	err := u.Init([]string{"foo", "bar"})
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (*UploadSuite) TestInitDuplicate(c *gc.C) {
	u := UploadCommand{resources: map[string]string{}}

	err := u.Init([]string{"foo", "foo=bar", "foo=baz"})
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsAlreadyExists)
}

func (*UploadSuite) TestInitNoName(c *gc.C) {
	u := UploadCommand{resources: map[string]string{}}

	err := u.Init([]string{"foo", "=foobar"})
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
}

func (*UploadSuite) TestInitNoPath(c *gc.C) {
	u := UploadCommand{resources: map[string]string{}}

	err := u.Init([]string{"foo", "foobar="})
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
}

func (*UploadSuite) TestInitGood(c *gc.C) {
	u := UploadCommand{resources: map[string]string{}}

	err := u.Init([]string{"foo", "bar=baz"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.resources, gc.DeepEquals, map[string]string{"bar": "baz"})
	c.Assert(u.service, gc.Equals, "foo")
}

func (*UploadSuite) TestInitTwoResources(c *gc.C) {
	u := UploadCommand{resources: map[string]string{}}

	err := u.Init([]string{"foo", "bar=baz", "fizz=buzz"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.resources, gc.DeepEquals, map[string]string{
		"bar":  "baz",
		"fizz": "buzz",
	})
	c.Assert(u.service, gc.Equals, "foo")
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
		deps: UploadDeps{
			NewClient:    s.stubDeps.NewClient,
			OpenResource: s.stubDeps.OpenResource,
		},
		resources: map[string]string{"foo": "bar", "baz": "bat"},
		service:   "svc",
	}

	err := u.Run(nil)
	c.Assert(err, jc.ErrorIsNil)

	checkCall(c, s.stubDeps.stub, "OpenResource", [][]interface{}{
		{"bar"},
		{"bat"},
	})
	checkCall(c, s.stubDeps.stub, "Upload", [][]interface{}{
		{"svc", "foo", s.stubDeps.file},
		{"svc", "baz", s.stubDeps.file},
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

type stubUploadDeps struct {
	stub   *testing.Stub
	file   *stubFile
	client *StubClient
}

func (s *stubUploadDeps) NewClient(c *UploadCommand) (UploadClient, error) {
	s.stub.AddCall("NewClient", c)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.client, nil
}

func (s *stubUploadDeps) OpenResource(path string) (io.ReadCloser, error) {
	s.stub.AddCall("OpenResource", path)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.file, nil
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
