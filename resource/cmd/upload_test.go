// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"io"
	"reflect"

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
		file:   &stubFile{stub: s.stub},
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

func (*UploadSuite) TestInitDuplicate(c *gc.C) {
	var u UploadCommand

	err := u.Init([]string{"foo", "foo=bar", "foo=baz"})
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsAlreadyExists)
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
	c.Assert(u.resourceFiles, gc.DeepEquals, []resourceFile{{
		service:  "foo",
		name:     "bar",
		filename: "baz",
	}})
	c.Assert(u.service, gc.Equals, "foo")
}

func (*UploadSuite) TestInitTwoResources(c *gc.C) {
	var u UploadCommand

	err := u.Init([]string{"foo", "bar=baz", "fizz=buzz"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.resourceFiles, gc.DeepEquals, []resourceFile{{
		service:  "foo",
		name:     "bar",
		filename: "baz",
	}, {
		service:  "foo",
		name:     "fizz",
		filename: "buzz",
	}})
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
		resourceFiles: []resourceFile{{
			service:  "svc",
			name:     "foo",
			filename: "bar",
		}, {
			service:  "svc",
			name:     "baz",
			filename: "bat",
		}},
		service: "svc",
	}

	err := u.Run(nil)
	c.Assert(err, jc.ErrorIsNil)

	checkCall(c, s.stub, "OpenResource", [][]interface{}{
		{"bar"},
		{"bat"},
	})
	checkCall(c, s.stub, "Upload", [][]interface{}{
		{"svc", "foo", s.stubDeps.file},
		{"svc", "baz", s.stubDeps.file},
	})
}

// checkCall checks that the given function has been called exactly len(args)
// times, and that the args passed to the Nth call match expected[N].
func checkCall(c *gc.C, stub *testing.Stub, funcname string, expected [][]interface{}) {
	var actual [][]interface{}
	for _, call := range stub.Calls() {
		if call.FuncName == funcname {
			actual = append(actual, call.Args)
		}
	}
	checkSameContent(c, actual, expected)
}

func checkSameContent(c *gc.C, actual, expected [][]interface{}) {
	for i, args := range actual {
		if len(expected) == 0 {
			c.Check(actual[i:], gc.HasLen, 0, gc.Commentf("unexpected call"))
			break
		}
		matched := false
		for j, expect := range expected {
			if reflect.DeepEqual(args, expect) {
				expected = append(expected[:j], expected[j+1:]...)
				matched = true
				break
			}
		}
		c.Check(matched, jc.IsTrue, gc.Commentf("extra call %#v", args))
	}
	c.Check(expected, gc.HasLen, 0, gc.Commentf("unmatched calls %#v", expected))
}

type stubUploadDeps struct {
	stub   *testing.Stub
	file   io.ReadCloser
	client UploadClient
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
