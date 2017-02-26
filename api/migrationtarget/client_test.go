// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/httprequest"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/migrationtarget"
	"github.com/juju/juju/apiserver/params"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/resource/resourcetesting"
	"github.com/juju/juju/tools"
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
	controllerVers := version.MustParse("1.2.5")

	err := client.Prechecks(coremigration.ModelInfo{
		UUID:                   "uuid",
		Owner:                  ownerTag,
		Name:                   "name",
		AgentVersion:           vers,
		ControllerAgentVersion: controllerVers,
	})
	c.Assert(err, gc.ErrorMatches, "boom")

	expectedArg := params.MigrationModelInfo{
		UUID:                   "uuid",
		Name:                   "name",
		OwnerTag:               ownerTag.String(),
		AgentVersion:           vers,
		ControllerAgentVersion: controllerVers,
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

func (s *ClientSuite) TestAdoptResources(c *gc.C) {
	client, stub := s.getClientAndStub(c)
	err := client.AdoptResources("the-model")
	c.Assert(err, gc.ErrorMatches, "boom")
	stub.CheckCall(c, 0, "MigrationTarget.AdoptResources", "", params.AdoptResourcesArgs{
		ModelTag:                "model-the-model",
		SourceControllerVersion: jujuversion.Current,
	})
}

func (s *ClientSuite) TestUploadCharm(c *gc.C) {
	const charmBody = "charming"
	curl := charm.MustParseURL("cs:~user/foo-2")
	doer := newFakeDoer(c, params.CharmsResponse{
		CharmURL: curl.String(),
	})
	caller := &fakeHTTPCaller{
		httpClient: &httprequest.Client{Doer: doer},
	}
	client := migrationtarget.NewClient(caller)
	outCurl, err := client.UploadCharm("uuid", curl, strings.NewReader(charmBody))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(outCurl, gc.DeepEquals, curl)
	c.Assert(doer.method, gc.Equals, "POST")
	c.Assert(doer.url, gc.Equals, "/migrate/charms?revision=2&schema=cs&series=&user=user")
	c.Assert(doer.body, gc.Equals, charmBody)
}

func (s *ClientSuite) TestUploadTools(c *gc.C) {
	const toolsBody = "toolie"
	vers := version.MustParseBinary("2.0.0-xenial-amd64")
	someTools := &tools.Tools{Version: vers}
	doer := newFakeDoer(c, params.ToolsResult{
		ToolsList: []*tools.Tools{someTools},
	})
	caller := &fakeHTTPCaller{
		httpClient: &httprequest.Client{Doer: doer},
	}
	client := migrationtarget.NewClient(caller)
	toolsList, err := client.UploadTools(
		"uuid",
		strings.NewReader(toolsBody),
		vers,
		"trusty", "warty",
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(toolsList, gc.HasLen, 1)
	c.Assert(toolsList[0], gc.DeepEquals, someTools)
	c.Assert(doer.method, gc.Equals, "POST")
	c.Assert(doer.url, gc.Equals, "/migrate/tools?binaryVersion=2.0.0-xenial-amd64&series=trusty,warty")
	c.Assert(doer.body, gc.Equals, toolsBody)
}

func (s *ClientSuite) TestUploadResource(c *gc.C) {
	const resourceBody = "resourceful"
	doer := newFakeDoer(c, "")
	caller := &fakeHTTPCaller{
		httpClient: &httprequest.Client{Doer: doer},
	}
	client := migrationtarget.NewClient(caller)

	res := resourcetesting.NewResource(c, nil, "blob", "app", resourceBody).Resource
	res.Revision = 1

	err := client.UploadResource("uuid", res, strings.NewReader(resourceBody))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(doer.method, gc.Equals, "POST")
	expectedURL := fmt.Sprintf("/migrate/resources?application=app&description=blob+description&fingerprint=%s&name=blob&origin=upload&path=blob.tgz&revision=1&size=11&timestamp=%d&type=file&user=a-user", res.Fingerprint.Hex(), res.Timestamp.UnixNano())
	c.Assert(doer.url, gc.Equals, expectedURL)
	c.Assert(doer.body, gc.Equals, resourceBody)
}

func (s *ClientSuite) TestSetUnitResource(c *gc.C) {
	const resourceBody = "resourceful"
	doer := newFakeDoer(c, "")
	caller := &fakeHTTPCaller{
		httpClient: &httprequest.Client{Doer: doer},
	}
	client := migrationtarget.NewClient(caller)

	res := resourcetesting.NewResource(c, nil, "blob", "app", resourceBody).Resource
	res.Revision = 2

	err := client.SetUnitResource("uuid", "app/0", res)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(doer.method, gc.Equals, "POST")
	expectedURL := fmt.Sprintf("/migrate/resources?description=blob+description&fingerprint=%s&name=blob&origin=upload&path=blob.tgz&revision=2&size=11&timestamp=%d&type=file&unit=app%%2F0&user=a-user", res.Fingerprint.Hex(), res.Timestamp.UnixNano())
	c.Assert(doer.url, gc.Equals, expectedURL)
	c.Assert(doer.body, gc.Equals, "")
}

func (s *ClientSuite) TestPlaceholderResource(c *gc.C) {
	doer := newFakeDoer(c, "")
	caller := &fakeHTTPCaller{
		httpClient: &httprequest.Client{Doer: doer},
	}
	client := migrationtarget.NewClient(caller)

	res := resourcetesting.NewPlaceholderResource(c, "blob", "app")
	res.Revision = 3
	res.Size = 123

	err := client.SetPlaceholderResource("uuid", res)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(doer.method, gc.Equals, "POST")
	expectedURL := fmt.Sprintf("/migrate/resources?application=app&description=blob+description&fingerprint=%s&name=blob&origin=upload&path=blob.tgz&revision=3&size=123&type=file", res.Fingerprint.Hex())
	c.Assert(doer.url, gc.Equals, expectedURL)
	c.Assert(doer.body, gc.Equals, "")
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

type fakeHTTPCaller struct {
	base.APICaller
	httpClient *httprequest.Client
	err        error
}

func (fakeHTTPCaller) BestFacadeVersion(string) int {
	return 0
}

func (c fakeHTTPCaller) HTTPClient() (*httprequest.Client, error) {
	return c.httpClient, c.err
}

func newFakeDoer(c *gc.C, respBody interface{}) *fakeDoer {
	body, err := json.Marshal(respBody)
	c.Assert(err, jc.ErrorIsNil)
	resp := &http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(bytes.NewReader(body)),
	}
	resp.Header = make(http.Header)
	resp.Header.Add("Content-Type", "application/json")
	return &fakeDoer{
		response: resp,
	}
}

type fakeDoer struct {
	response *http.Response

	method string
	url    string
	body   string
}

func (d *fakeDoer) Do(req *http.Request) (*http.Response, error) {
	d.method = req.Method
	d.url = req.URL.String()
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		panic(err)
	}
	d.body = string(body)
	return d.response, nil
}
