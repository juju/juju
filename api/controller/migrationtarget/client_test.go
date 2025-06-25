// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/migrationtarget"
	coremigration "github.com/juju/juju/core/migration"
	resourcetesting "github.com/juju/juju/core/resources/testing"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

type ClientSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&ClientSuite{})

func (s *ClientSuite) getClientAndStub(c *gc.C) (*migrationtarget.Client, *jujutesting.Stub) {
	var stub jujutesting.Stub
	apiCaller := apitesting.BestVersionCaller{APICallerFunc: apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		return errors.New("boom")
	}), BestVersion: 2}
	client := migrationtarget.NewClient(apiCaller)
	return client, &stub
}

func (s *ClientSuite) TestPrechecks(c *gc.C) {
	client, stub := s.getClientAndStub(c)

	ownerTag := names.NewUserTag("owner")
	vers := version.MustParse("1.2.3")
	controllerVers := version.MustParse("1.2.5")
	modelDescription := description.NewModel(description.ModelArgs{})

	bytes, err := description.Serialize(modelDescription)
	c.Assert(err, jc.ErrorIsNil)

	err = client.Prechecks(coremigration.ModelInfo{
		UUID:                   "uuid",
		Owner:                  ownerTag,
		Name:                   "name",
		AgentVersion:           vers,
		ControllerAgentVersion: controllerVers,
		ModelDescription:       modelDescription,
	})
	c.Assert(err, gc.ErrorMatches, "boom")

	expectedArg := params.MigrationModelInfo{
		UUID:                   "uuid",
		Name:                   "name",
		OwnerTag:               ownerTag.String(),
		AgentVersion:           vers,
		ControllerAgentVersion: controllerVers,
		ModelDescription:       bytes,
	}
	stub.CheckCallNames(c, "MigrationTarget.Prechecks")

	arg := stub.Calls()[0].Args[1].(params.MigrationModelInfo)

	mc := jc.NewMultiChecker()
	mc.AddExpr("_.FacadeVersions", gc.Not(gc.HasLen), 0)

	c.Assert(arg, mc, expectedArg)
}

func (s *ClientSuite) TestImport(c *gc.C) {
	client, stub := s.getClientAndStub(c)

	err := client.Import([]byte("foo"))

	expectedArg := params.SerializedModel{Bytes: []byte("foo")}
	stub.CheckCalls(c, []jujutesting.StubCall{
		{FuncName: "MigrationTarget.Import", Args: []interface{}{"", expectedArg}},
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
	sourceInfo := coremigration.SourceControllerInfo{
		ControllerTag:   coretesting.ControllerTag,
		ControllerAlias: "mycontroller",
		Addrs:           []string{"source-addr"},
		CACert:          "cacert",
	}
	relatedModels := []string{"related-model-uuid"}
	err := client.Activate(uuid, sourceInfo, relatedModels)
	expectedArg := params.ActivateModelArgs{
		ModelTag:        names.NewModelTag(uuid).String(),
		ControllerTag:   coretesting.ControllerTag.String(),
		ControllerAlias: "mycontroller",
		SourceAPIAddrs:  []string{"source-addr"},
		SourceCACert:    "cacert",
		CrossModelUUIDs: relatedModels,
	}
	stub.CheckCalls(c, []jujutesting.StubCall{
		{FuncName: "MigrationTarget.Activate", Args: []interface{}{"", expectedArg}},
	})
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *ClientSuite) TestOpenLogTransferStream(c *gc.C) {
	caller := fakeConnector{Stub: &jujutesting.Stub{}}
	client := migrationtarget.NewClient(caller)
	stream, err := client.OpenLogTransferStream("bad-dad")
	c.Assert(stream, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "sound hound")

	caller.Stub.CheckCall(c, 0, "ConnectControllerStream", "/migrate/logtransfer",
		url.Values{},
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

func (s *ClientSuite) TestCheckMachines(c *gc.C) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		target, ok := result.(*params.ErrorResults)
		c.Assert(ok, jc.IsTrue)
		*target = params.ErrorResults{Results: []params.ErrorResult{
			{Error: &params.Error{Message: "oops"}},
			{Error: &params.Error{Message: "oh no"}},
		}}
		stub.AddCall(objType+"."+request, id, arg)
		return nil
	})
	client := migrationtarget.NewClient(apiCaller)
	results, err := client.CheckMachines("django")
	c.Assert(results, gc.HasLen, 2)
	c.Assert(results[0], gc.ErrorMatches, "oops")
	c.Assert(results[1], gc.ErrorMatches, "oh no")
	s.AssertModelCall(c, &stub, names.NewModelTag("django"), "CheckMachines", err, false)
}

func (s *ClientSuite) TestUploadCharm(c *gc.C) {
	const charmBody = "charming"
	curl := "ch:foo-2"
	charmRef := "foo-abcdef0"
	doer := newFakeDoer(c, "", map[string]string{"Juju-Curl": curl})
	caller := &fakeHTTPCaller{
		httpClient: &httprequest.Client{Doer: doer},
	}
	client := migrationtarget.NewClient(caller)
	outCurl, err := client.UploadCharm("uuid", curl, charmRef, strings.NewReader(charmBody))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(outCurl, gc.DeepEquals, curl)
	c.Assert(doer.method, gc.Equals, "PUT")
	c.Assert(doer.url, gc.Equals, "/migrate/charms/foo-abcdef0")
	c.Assert(doer.headers.Get("Juju-Curl"), gc.Equals, curl)
	c.Assert(doer.body, gc.Equals, charmBody)
}

func (s *ClientSuite) TestUploadCharmHubCharm(c *gc.C) {
	const charmBody = "charming"
	curl := "ch:s390x/bionic/juju-qa-test-15"
	charmRef := "juju-qa-test-abcdef0"
	doer := newFakeDoer(c, "", map[string]string{"Juju-Curl": curl})
	caller := &fakeHTTPCaller{
		httpClient: &httprequest.Client{Doer: doer},
	}
	client := migrationtarget.NewClient(caller)
	outCurl, err := client.UploadCharm("uuid", curl, charmRef, strings.NewReader(charmBody))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(outCurl, gc.DeepEquals, curl)
	c.Assert(doer.method, gc.Equals, "PUT")
	c.Assert(doer.url, gc.Equals, "/migrate/charms/juju-qa-test-abcdef0")
	c.Assert(doer.headers.Get("Juju-Curl"), gc.Equals, curl)
	c.Assert(doer.body, gc.Equals, charmBody)
}

func (s *ClientSuite) TestUploadTools(c *gc.C) {
	const toolsBody = "toolie"
	vers := version.MustParseBinary("2.0.0-ubuntu-amd64")
	someTools := &tools.Tools{Version: vers}
	doer := newFakeDoer(c, params.ToolsResult{
		ToolsList: []*tools.Tools{someTools},
	}, nil)
	caller := &fakeHTTPCaller{
		httpClient: &httprequest.Client{Doer: doer},
	}
	client := migrationtarget.NewClient(caller)
	toolsList, err := client.UploadTools(
		"uuid",
		strings.NewReader(toolsBody),
		vers,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(toolsList, gc.HasLen, 1)
	c.Assert(toolsList[0], gc.DeepEquals, someTools)
	c.Assert(doer.method, gc.Equals, "POST")
	c.Assert(doer.url, gc.Equals, "/migrate/tools?binaryVersion=2.0.0-ubuntu-amd64")
	c.Assert(doer.body, gc.Equals, toolsBody)
}

func (s *ClientSuite) TestUploadResource(c *gc.C) {
	const resourceBody = "resourceful"
	doer := newFakeDoer(c, "", nil)
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
	doer := newFakeDoer(c, "", nil)
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
	doer := newFakeDoer(c, "", nil)
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

func (s *ClientSuite) TestCACert(c *gc.C) {
	call := func(objType string, version int, id, request string, args, response interface{}) error {
		c.Check(objType, gc.Equals, "MigrationTarget")
		c.Check(request, gc.Equals, "CACert")
		c.Check(args, gc.Equals, nil)
		c.Check(response, gc.FitsTypeOf, (*params.BytesResult)(nil))
		response.(*params.BytesResult).Result = []byte("foo cert")
		return nil
	}
	client := migrationtarget.NewClient(apitesting.APICallerFunc(call))
	r, err := client.CACert()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r, gc.Equals, "foo cert")
}

func (s *ClientSuite) AssertModelCall(c *gc.C, stub *jujutesting.Stub, tag names.ModelTag, call string, err error, expectError bool) {
	expectedArg := params.ModelArgs{ModelTag: tag.String()}
	stub.CheckCalls(c, []jujutesting.StubCall{
		{FuncName: "MigrationTarget." + call, Args: []interface{}{"", expectedArg}},
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

func (c fakeHTTPCaller) RootHTTPClient() (*httprequest.Client, error) {
	return c.httpClient, c.err
}

func (r *fakeHTTPCaller) Context() context.Context {
	return context.Background()
}

func newFakeDoer(c *gc.C, respBody interface{}, respHeaders map[string]string) *fakeDoer {
	body, err := json.Marshal(respBody)
	c.Assert(err, jc.ErrorIsNil)
	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	resp.Header = make(http.Header)
	resp.Header.Add("Content-Type", "application/json")
	for k, v := range respHeaders {
		resp.Header.Set(k, v)
	}
	return &fakeDoer{
		response: resp,
	}
}

type fakeDoer struct {
	response *http.Response

	method  string
	url     string
	body    string
	headers http.Header
}

func (d *fakeDoer) Do(req *http.Request) (*http.Response, error) {
	d.method = req.Method
	d.url = req.URL.String()
	d.headers = req.Header

	// If the body is nil, don't do anything about reading the req.Body
	// The underlying net http go library deals with nil bodies for requests,
	// so our fake stub should also mirror this.
	// https://golang.org/src/net/http/client.go?s=17323:17375#L587
	if req.Body == nil {
		return d.response, nil
	}

	// ReadAll the body if it's found.
	body, err := io.ReadAll(req.Body)
	if err != nil {
		panic(err)
	}
	d.body = string(body)
	return d.response, nil
}
