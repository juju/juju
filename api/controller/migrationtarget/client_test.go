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
	stdtesting "testing"
	"time"

	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/migrationtarget"
	coremigration "github.com/juju/juju/core/migration"
	resourcetesting "github.com/juju/juju/core/resource/testing"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
)

type ClientSuite struct {
	testhelpers.IsolationSuite
}

func TestClientSuite(t *stdtesting.T) { tc.Run(t, &ClientSuite{}) }
func (s *ClientSuite) getClientAndStub() (*migrationtarget.Client, *testhelpers.Stub) {
	var stub testhelpers.Stub
	apiCaller := apitesting.BestVersionCaller{APICallerFunc: apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result any) error {
		stub.AddCall(objType+"."+request, id, arg)
		return errors.New("boom")
	}), BestVersion: 2}
	client := migrationtarget.NewClient(apiCaller)
	return client, &stub
}

func (s *ClientSuite) TestPrechecks(c *tc.C) {
	client, stub := s.getClientAndStub()

	ownerTag := names.NewUserTag("owner")
	vers := semversion.MustParse("1.2.3")
	controllerVers := semversion.MustParse("1.2.5")
	modelDescription := description.NewModel(description.ModelArgs{})

	bytes, err := description.Serialize(modelDescription)
	c.Assert(err, tc.ErrorIsNil)

	err = client.Prechecks(c.Context(), coremigration.ModelInfo{
		UUID:                   "uuid",
		Owner:                  ownerTag,
		Name:                   "name",
		AgentVersion:           vers,
		ControllerAgentVersion: controllerVers,
		ModelDescription:       modelDescription,
	})
	c.Assert(err, tc.ErrorMatches, "boom")

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

	mc := tc.NewMultiChecker()
	mc.AddExpr("_.FacadeVersions", tc.Not(tc.HasLen), 0)

	c.Check(arg, mc, expectedArg)
}

func (s *ClientSuite) TestImport(c *tc.C) {
	client, stub := s.getClientAndStub()

	err := client.Import(c.Context(), []byte("foo"))

	expectedArg := params.SerializedModel{Bytes: []byte("foo")}
	stub.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "MigrationTarget.Import", Args: []any{"", expectedArg}},
	})
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *ClientSuite) TestAbort(c *tc.C) {
	client, stub := s.getClientAndStub()

	uuid := "fake"
	err := client.Abort(c.Context(), uuid)
	s.AssertModelCall(c, stub, names.NewModelTag(uuid), "Abort", err, true)
}

func (s *ClientSuite) TestActivate(c *tc.C) {
	client, stub := s.getClientAndStub()

	uuid := "fake"
	sourceInfo := coremigration.SourceControllerInfo{
		ControllerTag:   coretesting.ControllerTag,
		ControllerAlias: "mycontroller",
		Addrs:           []string{"source-addr"},
		CACert:          "cacert",
	}
	relatedModels := []string{"related-model-uuid"}
	err := client.Activate(c.Context(), uuid, sourceInfo, relatedModels)
	expectedArg := params.ActivateModelArgs{
		ModelTag:        names.NewModelTag(uuid).String(),
		ControllerTag:   coretesting.ControllerTag.String(),
		ControllerAlias: "mycontroller",
		SourceAPIAddrs:  []string{"source-addr"},
		SourceCACert:    "cacert",
		CrossModelUUIDs: relatedModels,
	}
	stub.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "MigrationTarget.Activate", Args: []any{"", expectedArg}},
	})
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *ClientSuite) TestOpenLogTransferStream(c *tc.C) {
	caller := fakeConnector{Stub: &testhelpers.Stub{}}
	client := migrationtarget.NewClient(caller)
	stream, err := client.OpenLogTransferStream(c.Context(), "bad-dad")
	c.Assert(stream, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "sound hound")

	caller.Stub.CheckCall(c, 0, "ConnectControllerStream", "/migrate/logtransfer",
		url.Values{},
		http.Header{textproto.CanonicalMIMEHeaderKey(params.MigrationModelHTTPHeader): {"bad-dad"}},
	)
}

func (s *ClientSuite) TestLatestLogTime(c *tc.C) {
	var stub testhelpers.Stub
	t1 := time.Date(2016, 12, 1, 10, 31, 0, 0, time.UTC)

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result any) error {
		target, ok := result.(*time.Time)
		c.Assert(ok, tc.IsTrue)
		*target = t1
		stub.AddCall(objType+"."+request, id, arg)
		return nil
	})
	client := migrationtarget.NewClient(apiCaller)
	result, err := client.LatestLogTime(c.Context(), "fake")

	c.Assert(result, tc.Equals, t1)
	s.AssertModelCall(c, &stub, names.NewModelTag("fake"), "LatestLogTime", err, false)
}

func (s *ClientSuite) TestLatestLogTimeError(c *tc.C) {
	client, stub := s.getClientAndStub()
	result, err := client.LatestLogTime(c.Context(), "fake")

	c.Assert(result, tc.Equals, time.Time{})
	s.AssertModelCall(c, stub, names.NewModelTag("fake"), "LatestLogTime", err, true)
}

func (s *ClientSuite) TestAdoptResources(c *tc.C) {
	client, stub := s.getClientAndStub()
	err := client.AdoptResources(c.Context(), "the-model")
	c.Assert(err, tc.ErrorMatches, "boom")
	stub.CheckCall(c, 0, "MigrationTarget.AdoptResources", "", params.AdoptResourcesArgs{
		ModelTag:                "model-the-model",
		SourceControllerVersion: jujuversion.Current,
	})
}

func (s *ClientSuite) TestCheckMachines(c *tc.C) {
	var stub testhelpers.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result any) error {
		target, ok := result.(*params.ErrorResults)
		c.Assert(ok, tc.IsTrue)
		*target = params.ErrorResults{Results: []params.ErrorResult{
			{Error: &params.Error{Message: "oops"}},
			{Error: &params.Error{Message: "oh no"}},
		}}
		stub.AddCall(objType+"."+request, id, arg)
		return nil
	})
	client := migrationtarget.NewClient(apiCaller)
	results, err := client.CheckMachines(c.Context(), "django")
	c.Assert(results, tc.HasLen, 2)
	c.Assert(results[0], tc.ErrorMatches, "oops")
	c.Assert(results[1], tc.ErrorMatches, "oh no")
	s.AssertModelCall(c, &stub, names.NewModelTag("django"), "CheckMachines", err, false)
}

func (s *ClientSuite) TestUploadCharm(c *tc.C) {
	const charmBody = "charming"
	curl := "ch:foo-2"
	charmRef := "foo-abcdef0"
	doer := newFakeDoer(c, "", map[string]string{params.JujuCharmURLHeader: curl})
	caller := &fakeHTTPCaller{
		c:          c,
		httpClient: &httprequest.Client{Doer: doer},
	}
	client := migrationtarget.NewClient(caller)
	outCurl, err := client.UploadCharm(c.Context(), "uuid", curl, charmRef, strings.NewReader(charmBody))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(outCurl, tc.DeepEquals, curl)
	c.Assert(doer.method, tc.Equals, "PUT")
	c.Assert(doer.url, tc.Equals, "/migrate/charms/foo-abcdef0")
	c.Assert(doer.headers.Get(params.JujuCharmURLHeader), tc.Equals, curl)
	c.Assert(doer.body, tc.Equals, charmBody)
}

func (s *ClientSuite) TestUploadCharmHubCharm(c *tc.C) {
	const charmBody = "charming"
	curl := "ch:s390x/bionic/juju-qa-test-15"
	charmRef := "juju-qa-test-abcdef0"
	doer := newFakeDoer(c, "", map[string]string{params.JujuCharmURLHeader: curl})
	caller := &fakeHTTPCaller{
		c:          c,
		httpClient: &httprequest.Client{Doer: doer},
	}
	client := migrationtarget.NewClient(caller)
	outCurl, err := client.UploadCharm(c.Context(), "uuid", curl, charmRef, strings.NewReader(charmBody))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(outCurl, tc.DeepEquals, curl)
	c.Assert(doer.method, tc.Equals, "PUT")
	c.Assert(doer.url, tc.Equals, "/migrate/charms/juju-qa-test-abcdef0")
	c.Assert(doer.headers.Get(params.JujuCharmURLHeader), tc.Equals, curl)
	c.Assert(doer.body, tc.Equals, charmBody)
}

func (s *ClientSuite) TestUploadTools(c *tc.C) {
	const toolsBody = "toolie"
	vers := semversion.MustParseBinary("2.0.0-ubuntu-amd64")
	someTools := &tools.Tools{Version: vers}
	doer := newFakeDoer(c, params.ToolsResult{
		ToolsList: []*tools.Tools{someTools},
	}, nil)
	caller := &fakeHTTPCaller{
		c:          c,
		httpClient: &httprequest.Client{Doer: doer},
	}
	client := migrationtarget.NewClient(caller)
	toolsList, err := client.UploadTools(
		c.Context(),
		"uuid",
		strings.NewReader(toolsBody),
		vers,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(toolsList, tc.HasLen, 1)
	c.Assert(toolsList[0], tc.DeepEquals, someTools)
	c.Assert(doer.method, tc.Equals, "POST")
	c.Assert(doer.url, tc.Equals, "/migrate/tools?binaryVersion=2.0.0-ubuntu-amd64")
	c.Assert(doer.body, tc.Equals, toolsBody)
}

func (s *ClientSuite) TestUploadResource(c *tc.C) {
	const resourceBody = "resourceful"
	doer := newFakeDoer(c, "", nil)
	caller := &fakeHTTPCaller{
		c:          c,
		httpClient: &httprequest.Client{Doer: doer},
	}
	client := migrationtarget.NewClient(caller)

	res := resourcetesting.NewResource(c, nil, "blob", "app", resourceBody).Resource
	res.Revision = 1

	err := client.UploadResource(c.Context(), "uuid", res, strings.NewReader(resourceBody))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(doer.method, tc.Equals, "POST")
	expectedURL := fmt.Sprintf("/migrate/resources?application=app&fingerprint=%s&name=blob&origin=upload&revision=1&size=11&timestamp=%d&type=file&user=a-user", res.Fingerprint.Hex(), res.Timestamp.UnixNano())
	c.Assert(doer.url, tc.Equals, expectedURL)
	c.Assert(doer.body, tc.Equals, resourceBody)
}

func (s *ClientSuite) TestCACert(c *tc.C) {
	call := func(objType string, version int, id, request string, args, response any) error {
		c.Check(objType, tc.Equals, "MigrationTarget")
		c.Check(request, tc.Equals, "CACert")
		c.Check(args, tc.Equals, nil)
		c.Check(response, tc.FitsTypeOf, (*params.BytesResult)(nil))
		response.(*params.BytesResult).Result = []byte("foo cert")
		return nil
	}
	client := migrationtarget.NewClient(apitesting.APICallerFunc(call))
	r, err := client.CACert(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(r, tc.Equals, "foo cert")
}

func (s *ClientSuite) AssertModelCall(c *tc.C, stub *testhelpers.Stub, tag names.ModelTag, call string, err error, expectError bool) {
	expectedArg := params.ModelArgs{ModelTag: tag.String()}
	stub.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "MigrationTarget." + call, Args: []any{"", expectedArg}},
	})
	if expectError {
		c.Assert(err, tc.ErrorMatches, "boom")
	} else {
		c.Assert(err, tc.ErrorIsNil)
	}
}

type fakeConnector struct {
	base.APICaller

	*testhelpers.Stub
}

func (fakeConnector) BestFacadeVersion(string) int {
	return 0
}

func (c fakeConnector) ConnectControllerStream(_ context.Context, path string, attrs url.Values, headers http.Header) (base.Stream, error) {
	c.Stub.AddCall("ConnectControllerStream", path, attrs, headers)
	return nil, errors.New("sound hound")
}

type fakeHTTPCaller struct {
	base.APICaller
	httpClient *httprequest.Client
	err        error
	c          *tc.C
}

func (fakeHTTPCaller) BestFacadeVersion(string) int {
	return 0
}

func (f *fakeHTTPCaller) RootHTTPClient() (*httprequest.Client, error) {
	return f.httpClient, f.err
}

func (f *fakeHTTPCaller) Context() context.Context {
	return f.c.Context()
}

func newFakeDoer(c *tc.C, respBody any, respHeaders map[string]string) *fakeDoer {
	body, err := json.Marshal(respBody)
	c.Assert(err, tc.ErrorIsNil)
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
