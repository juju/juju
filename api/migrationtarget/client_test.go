// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget_test

import (
	"net/http"
	"net/textproto"
	"net/url"
	"time"

	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/migrationtarget"
	"github.com/juju/juju/apiserver/params"
	coremigration "github.com/juju/juju/core/migration"
	jujuversion "github.com/juju/juju/version"
)

type ClientSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&ClientSuite{})

func (s *ClientSuite) getClientAndStub(c *gc.C) (*migrationtarget.Client, *jujutesting.Stub) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		return errors.New("boom")
	})
	client := migrationtarget.NewClient(apiCaller)
	return client, &stub
}

func (s *ClientSuite) TestPrechecks(c *gc.C) {
	client, stub := s.getClientAndStub(c)

	ownerTag := names.NewUserTag("owner")
	vers := version.MustParse("1.2.3")

	err := client.Prechecks(coremigration.ModelInfo{
		UUID:         "uuid",
		Owner:        ownerTag,
		Name:         "name",
		AgentVersion: vers,
	})
	c.Assert(err, gc.ErrorMatches, "boom")

	expectedArg := params.MigrationModelInfo{
		UUID:         "uuid",
		Name:         "name",
		OwnerTag:     ownerTag.String(),
		AgentVersion: vers,
	}
	stub.CheckCalls(c, []jujutesting.StubCall{
		{"MigrationTarget.Prechecks", []interface{}{"", expectedArg}},
	})
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

	uuid := "fake"
	err := client.Abort(uuid)
	s.AssertModelCall(c, stub, names.NewModelTag(uuid), "Abort", err, true)
}

func (s *ClientSuite) TestActivate(c *gc.C) {
	client, stub := s.getClientAndStub(c)

	uuid := "fake"
	err := client.Activate(uuid)
	s.AssertModelCall(c, stub, names.NewModelTag(uuid), "Activate", err, true)
}

func (s *ClientSuite) TestOpenLogTransferStream(c *gc.C) {
	caller := fakeConnector{Stub: &jujutesting.Stub{}}
	client := migrationtarget.NewClient(caller)
	stream, err := client.OpenLogTransferStream("bad-dad")
	c.Assert(stream, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "sound hound")

	caller.Stub.CheckCall(c, 0, "ConnectControllerStream", "/migrate/logtransfer",
		url.Values{"jujuclientversion": {jujuversion.Current.String()}},
		http.Header{textproto.CanonicalMIMEHeaderKey(params.MigrationModelHTTPHeader): {"bad-dad"}},
	)
}

func (s *ClientSuite) TestLatestLogTime(c *gc.C) {
	var stub jujutesting.Stub
	t1 := time.Date(2016, 12, 1, 10, 31, 0, 0, time.UTC)

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		target, ok := result.(*time.Time)
		c.Assert(ok, jc.IsTrue)
		*target = t1
		stub.AddCall(objType+"."+request, id, arg)
		return nil
	})
	client := migrationtarget.NewClient(apiCaller)
	result, err := client.LatestLogTime("fake")

	c.Assert(result, gc.Equals, t1)
	s.AssertModelCall(c, &stub, names.NewModelTag("fake"), "LatestLogTime", err, false)
}

func (s *ClientSuite) TestLatestLogTimeError(c *gc.C) {
	client, stub := s.getClientAndStub(c)
	result, err := client.LatestLogTime("fake")

	c.Assert(result, gc.Equals, time.Time{})
	s.AssertModelCall(c, stub, names.NewModelTag("fake"), "LatestLogTime", err, true)
}

func (s *ClientSuite) AssertModelCall(c *gc.C, stub *jujutesting.Stub, tag names.ModelTag, call string, err error, expectError bool) {
	expectedArg := params.ModelArgs{ModelTag: tag.String()}
	stub.CheckCalls(c, []jujutesting.StubCall{
		{"MigrationTarget." + call, []interface{}{"", expectedArg}},
	})
	if expectError {
		c.Assert(err, gc.ErrorMatches, "boom")
	} else {
		c.Assert(err, jc.ErrorIsNil)
	}
}

type fakeConnector struct {
	base.APICaller

	*jujutesting.Stub
}

func (fakeConnector) BestFacadeVersion(string) int {
	return 0
}

func (c fakeConnector) ConnectControllerStream(path string, attrs url.Values, headers http.Header) (base.Stream, error) {
	c.Stub.AddCall("ConnectControllerStream", path, attrs, headers)
	return nil, errors.New("sound hound")
}
