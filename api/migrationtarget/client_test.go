// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget_test

import (
	"github.com/juju/errors"
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

func (s *ClientSuite) TestImport(c *gc.C) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		return errors.New("boom")
	})
	client := migrationtarget.NewClient(apiCaller)

	err := client.Import([]byte("foo"))

	expectedArg := params.SerializedModel{Bytes: []byte("foo")}
	stub.CheckCalls(c, []jujutesting.StubCall{
		{"MigrationTarget.Import", []interface{}{"", expectedArg}},
	})
	c.Assert(err, gc.ErrorMatches, "boom")
}
