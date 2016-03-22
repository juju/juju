// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/migrationtarget"
	"github.com/juju/juju/apiserver/params"
)

type ClientSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&ClientSuite{})

func (s *ClientSuite) getClientAndStub(c *gc.C) (migrationtarget.Client, *jujutesting.Stub) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		return errors.New("boom")
	})
	client := migrationtarget.NewClient(apiCaller)
	return client, &stub
}

func (s *ClientSuite) TestImport(c *gc.C) {
	client, stub := s.getClientAndStub(c)

	err := client.Import([]byte("foo"))

	expectedArg := params.SerializedModel{Bytes: []byte("foo")}
	stub.CheckCalls(c, []jujutesting.StubCall{
		{"MigrationTarget.Import", []interface{}{"", expectedArg}},
	})
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *ClientSuite) TestAbort(c *gc.C) {
	client, stub := s.getClientAndStub(c)

	tag := names.NewModelTag("fake-uuid")
	err := client.Abort(tag)
	s.AssertModelCall(c, stub, tag, "Abort", err)
}

func (s *ClientSuite) TestActivate(c *gc.C) {
	client, stub := s.getClientAndStub(c)

	tag := names.NewModelTag("fake-uuid")
	err := client.Activate(tag)
	s.AssertModelCall(c, stub, tag, "Activate", err)
}

func (s *ClientSuite) AssertModelCall(c *gc.C, stub *jujutesting.Stub, tag names.ModelTag, call string, err error) {
	expectedArg := params.ModelArgs{ModelTag: tag.String()}
	stub.CheckCalls(c, []jujutesting.StubCall{
		{"MigrationTarget." + call, []interface{}{"", expectedArg}},
	})
	c.Assert(err, gc.ErrorMatches, "boom")
}
