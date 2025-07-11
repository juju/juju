// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/description/v10"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"gopkg.in/httprequest.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/migrationmaster"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/watcher"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

type ClientSuite struct {
	testhelpers.IsolationSuite
}

func TestClientSuite(t *stdtesting.T) {
	tc.Run(t, &ClientSuite{})
}

func (s *ClientSuite) TestWatch(c *tc.C) {
	var stub testhelpers.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		*(result.(*params.NotifyWatchResult)) = params.NotifyWatchResult{
			NotifyWatcherId: "123",
		}
		return nil
	})
	expectWatch := &struct{ watcher.NotifyWatcher }{}
	newWatcher := func(caller base.APICaller, result params.NotifyWatchResult) watcher.NotifyWatcher {
		c.Check(caller, tc.NotNil)
		c.Check(result, tc.DeepEquals, params.NotifyWatchResult{NotifyWatcherId: "123"})
		return expectWatch
	}
	client := migrationmaster.NewClient(apiCaller, newWatcher)
	w, err := client.Watch(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(w, tc.Equals, expectWatch)
	stub.CheckCalls(c, []testhelpers.StubCall{{FuncName: "MigrationMaster.Watch", Args: []interface{}{"", nil}}})
}

func (s *ClientSuite) TestWatchCallError(c *tc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("boom")
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	_, err := client.Watch(c.Context())
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *ClientSuite) TestMigrationStatus(c *tc.C) {
	mac, err := macaroon.New([]byte("secret"), []byte("id"), "location", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	macs := []macaroon.Slice{{mac}}
	macsJSON, err := json.Marshal(macs)
	c.Assert(err, tc.ErrorIsNil)

	modelUUID := uuid.MustNewUUID().String()
	controllerUUID := uuid.MustNewUUID().String()
	controllerTag := names.NewControllerTag(controllerUUID)
	timestamp := time.Date(2016, 6, 22, 16, 42, 44, 0, time.UTC)
	apiCaller := apitesting.APICallerFunc(func(_ string, _ int, _, _ string, _, result interface{}) error {
		out := result.(*params.MasterMigrationStatus)
		*out = params.MasterMigrationStatus{
			Spec: params.MigrationSpec{
				ModelTag: names.NewModelTag(modelUUID).String(),
				TargetInfo: params.MigrationTargetInfo{
					ControllerTag: controllerTag.String(),
					Addrs:         []string{"2.2.2.2:2"},
					CACert:        "cert",
					AuthTag:       names.NewUserTag("admin").String(),
					Password:      "secret",
					Macaroons:     string(macsJSON),
					Token:         "token",
				},
			},
			MigrationId:      "id",
			Phase:            "IMPORT",
			PhaseChangedTime: timestamp,
		}
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	status, err := client.MigrationStatus(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	// Extract macaroons so we can compare them separately
	// (as they can't be compared using DeepEquals due to 'UnmarshaledAs')
	statusMacs := status.TargetInfo.Macaroons
	status.TargetInfo.Macaroons = nil
	testing.MacaroonEquals(c, statusMacs[0][0], mac)
	c.Assert(status, tc.DeepEquals, migration.MigrationStatus{
		MigrationId:      "id",
		ModelUUID:        modelUUID,
		Phase:            migration.IMPORT,
		PhaseChangedTime: timestamp,
		TargetInfo: migration.TargetInfo{
			ControllerUUID: controllerUUID,
			Addrs:          []string{"2.2.2.2:2"},
			CACert:         "cert",
			User:           "admin",
			Password:       "secret",
			Token:          "token",
		},
	})
}

func (s *ClientSuite) TestSetPhase(c *tc.C) {
	var stub testhelpers.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	err := client.SetPhase(c.Context(), migration.QUIESCE)
	c.Assert(err, tc.ErrorIsNil)
	expectedArg := params.SetMigrationPhaseArgs{Phase: "QUIESCE"}
	stub.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "MigrationMaster.SetPhase", Args: []interface{}{"", expectedArg}},
	})
}

func (s *ClientSuite) TestSetPhaseError(c *tc.C) {
	apiCaller := apitesting.APICallerFunc(func(string, int, string, string, interface{}, interface{}) error {
		return errors.New("boom")
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	err := client.SetPhase(c.Context(), migration.QUIESCE)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *ClientSuite) TestSetStatusMessage(c *tc.C) {
	var stub testhelpers.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	err := client.SetStatusMessage(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	expectedArg := params.SetMigrationStatusMessageArgs{Message: "foo"}
	stub.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "MigrationMaster.SetStatusMessage", Args: []interface{}{"", expectedArg}},
	})
}

func (s *ClientSuite) TestSetStatusMessageError(c *tc.C) {
	apiCaller := apitesting.APICallerFunc(func(string, int, string, string, interface{}, interface{}) error {
		return errors.New("boom")
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	err := client.SetStatusMessage(c.Context(), "foo")
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *ClientSuite) TestModelInfoWithoutModelDescription(c *tc.C) {
	var stub testhelpers.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, v int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		*(result.(*params.MigrationModelInfo)) = params.MigrationModelInfo{
			UUID:                   "uuid",
			Name:                   "name",
			Qualifier:              "prod",
			AgentVersion:           semversion.MustParse("1.2.3"),
			ControllerAgentVersion: semversion.MustParse("1.2.4"),
		}
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	model, err := client.ModelInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	stub.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "MigrationMaster.ModelInfo", Args: []interface{}{"", nil}},
	})
	c.Check(model, tc.DeepEquals, migration.ModelInfo{
		UUID:                   "uuid",
		Name:                   "name",
		Qualifier:              "prod",
		AgentVersion:           semversion.MustParse("1.2.3"),
		ControllerAgentVersion: semversion.MustParse("1.2.4"),
	})
}

func (s *ClientSuite) TestModelInfoWithModelDescription(c *tc.C) {
	modelDescription := description.NewModel(description.ModelArgs{
		Config: make(map[string]interface{}),
	})
	serialized, err := description.Serialize(modelDescription)
	c.Assert(err, tc.ErrorIsNil)

	var stub testhelpers.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, v int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		*(result.(*params.MigrationModelInfo)) = params.MigrationModelInfo{
			UUID:                   "uuid",
			Name:                   "name",
			Qualifier:              "prod",
			AgentVersion:           semversion.MustParse("1.2.3"),
			ControllerAgentVersion: semversion.MustParse("1.2.4"),
			ModelDescription:       serialized,
		}
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	model, err := client.ModelInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	stub.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "MigrationMaster.ModelInfo", Args: []interface{}{"", nil}},
	})
	c.Check(model, tc.DeepEquals, migration.ModelInfo{
		UUID:                   "uuid",
		Name:                   "name",
		Qualifier:              "prod",
		AgentVersion:           semversion.MustParse("1.2.3"),
		ControllerAgentVersion: semversion.MustParse("1.2.4"),
		ModelDescription:       modelDescription,
	})
}

func (s *ClientSuite) TestSourceControllerInfo(c *tc.C) {
	var stub testhelpers.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, v int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		*(result.(*params.MigrationSourceInfo)) = params.MigrationSourceInfo{
			LocalRelatedModels: []string{"related-model-uuid"},
			ControllerTag:      coretesting.ControllerTag.String(),
			ControllerAlias:    "mycontroller",
			Addrs:              []string{"source-addr"},
			CACert:             "cacert",
		}
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	info, relatedModels, err := client.SourceControllerInfo(c.Context())
	stub.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "MigrationMaster.SourceControllerInfo", Args: []interface{}{"", nil}},
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(info, tc.DeepEquals, migration.SourceControllerInfo{
		ControllerTag:   coretesting.ControllerTag,
		ControllerAlias: "mycontroller",
		Addrs:           []string{"source-addr"},
		CACert:          "cacert",
	})
	c.Assert(relatedModels, tc.SameContents, []string{"related-model-uuid"})
}

func (s *ClientSuite) TestPrechecks(c *tc.C) {
	var stub testhelpers.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		return errors.New("blam")
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	err := client.Prechecks(c.Context())
	c.Check(err, tc.ErrorMatches, "blam")
	expectedArg := params.PrechecksArgs{}
	stub.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "MigrationMaster.Prechecks", Args: []interface{}{"", expectedArg}},
	})
}

func (s *ClientSuite) TestProcessRelations(c *tc.C) {
	var stub testhelpers.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		return nil
	})

	client := migrationmaster.NewClient(apiCaller, nil)
	err := client.ProcessRelations(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ClientSuite) TestProcessRelationsError(c *tc.C) {
	apiCaller := apitesting.APICallerFunc(func(string, int, string, string, interface{}, interface{}) error {
		return errors.New("blam")
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	err := client.ProcessRelations(c.Context(), "foo")
	c.Assert(err, tc.ErrorMatches, "blam")
}

func (s *ClientSuite) TestExport(c *tc.C) {
	var stub testhelpers.Stub

	fpHash := charmresource.NewFingerprintHash()
	appFp := fpHash.Fingerprint()

	appTs := time.Now()

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		out := result.(*params.SerializedModel)
		*out = params.SerializedModel{
			Bytes:  []byte("foo"),
			Charms: []string{"ch:foo-1"},
			Tools: []params.SerializedModelTools{{
				Version: "2.0.0-ubuntu-amd64",
				URI:     "/tools/0",
				SHA256:  "439c9ea02f8561c5a152d7cf4818d72cd5f2916b555d82c5eee599f5e8f3d09e",
			}},
			Resources: []params.SerializedModelResource{{
				Application:    "fooapp",
				Name:           "bin",
				Revision:       2,
				Type:           "file",
				Origin:         "upload",
				FingerprintHex: appFp.Hex(),
				Size:           123,
				Timestamp:      appTs,
				Username:       "bob",
			}},
		}
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	out, err := client.Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	stub.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "MigrationMaster.Export", Args: []interface{}{"", nil}},
	})
	c.Assert(out, tc.DeepEquals, migration.SerializedModel{
		Bytes:  []byte("foo"),
		Charms: []string{"ch:foo-1"},
		Tools: map[string]semversion.Binary{
			"439c9ea02f8561c5a152d7cf4818d72cd5f2916b555d82c5eee599f5e8f3d09e": semversion.MustParseBinary("2.0.0-ubuntu-amd64"),
		},
		Resources: []resource.Resource{{
			Resource: charmresource.Resource{
				Meta: charmresource.Meta{
					Name: "bin",
					Type: charmresource.TypeFile,
				},
				Origin:      charmresource.OriginUpload,
				Revision:    2,
				Fingerprint: appFp,
				Size:        123,
			},
			ApplicationName: "fooapp",
			RetrievedBy:     "bob",
			Timestamp:       appTs,
		}},
	})
}

func (s *ClientSuite) TestExportError(c *tc.C) {
	apiCaller := apitesting.APICallerFunc(func(string, int, string, string, interface{}, interface{}) error {
		return errors.New("blam")
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	_, err := client.Export(c.Context())
	c.Assert(err, tc.ErrorMatches, "blam")
}

const resourceContent = "resourceful"

func setupFakeHTTP() (*migrationmaster.Client, *fakeDoer) {
	doer := &fakeDoer{
		response: &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(resourceContent)),
		},
	}
	caller := &fakeHTTPCaller{
		httpClient: &httprequest.Client{
			Doer: doer,
		},
	}
	return migrationmaster.NewClient(caller, nil), doer
}

func (s *ClientSuite) TestOpenResource(c *tc.C) {
	client, doer := setupFakeHTTP()
	r, err := client.OpenResource(c.Context(), "app", "blob")
	c.Assert(err, tc.ErrorIsNil)
	checkReader(c, r, "resourceful")
	c.Check(doer.method, tc.Equals, "GET")
	c.Check(doer.url, tc.Equals, "/applications/app/resources/blob")
}

func (s *ClientSuite) TestReap(c *tc.C) {
	var stub testhelpers.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	err := client.Reap(c.Context())
	c.Check(err, tc.ErrorIsNil)
	stub.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "MigrationMaster.Reap", Args: []interface{}{"", nil}},
	})
}

func (s *ClientSuite) TestReapError(c *tc.C) {
	apiCaller := apitesting.APICallerFunc(func(string, int, string, string, interface{}, interface{}) error {
		return errors.New("blam")
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	err := client.Reap(c.Context())
	c.Assert(err, tc.ErrorMatches, "blam")
}

func (s *ClientSuite) TestWatchMinionReports(c *tc.C) {
	var stub testhelpers.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		*(result.(*params.NotifyWatchResult)) = params.NotifyWatchResult{
			NotifyWatcherId: "123",
		}
		return nil
	})

	expectWatch := &struct{ watcher.NotifyWatcher }{}
	newWatcher := func(caller base.APICaller, result params.NotifyWatchResult) watcher.NotifyWatcher {
		c.Check(caller, tc.NotNil)
		c.Check(result, tc.DeepEquals, params.NotifyWatchResult{NotifyWatcherId: "123"})
		return expectWatch
	}
	client := migrationmaster.NewClient(apiCaller, newWatcher)
	w, err := client.WatchMinionReports(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(w, tc.Equals, expectWatch)
	stub.CheckCalls(c, []testhelpers.StubCall{{FuncName: "MigrationMaster.WatchMinionReports", Args: []interface{}{"", nil}}})
}

func (s *ClientSuite) TestWatchMinionReportsError(c *tc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("boom")
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	_, err := client.WatchMinionReports(c.Context())
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *ClientSuite) TestMinionReports(c *tc.C) {
	var stub testhelpers.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		out := result.(*params.MinionReports)
		*out = params.MinionReports{
			MigrationId:  "id",
			Phase:        "IMPORT",
			SuccessCount: 4,
			UnknownCount: 3,
			UnknownSample: []string{
				names.NewMachineTag("3").String(),
				names.NewMachineTag("4").String(),
				names.NewUnitTag("foo/0").String(),
				names.NewApplicationTag("bar").String(),
			},
			Failed: []string{
				names.NewMachineTag("5").String(),
				names.NewUnitTag("foo/1").String(),
				names.NewUnitTag("foo/2").String(),
				names.NewApplicationTag("foobar").String(),
			},
		}
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	out, err := client.MinionReports(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	stub.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "MigrationMaster.MinionReports", Args: []interface{}{"", nil}},
	})
	c.Assert(out, tc.DeepEquals, migration.MinionReports{
		MigrationId:             "id",
		Phase:                   migration.IMPORT,
		SuccessCount:            4,
		UnknownCount:            3,
		SomeUnknownMachines:     []string{"3", "4"},
		SomeUnknownUnits:        []string{"foo/0"},
		SomeUnknownApplications: []string{"bar"},
		FailedMachines:          []string{"5"},
		FailedUnits:             []string{"foo/1", "foo/2"},
		FailedApplications:      []string{"foobar"},
	})
}

func (s *ClientSuite) TestMinionReportsFailedCall(c *tc.C) {
	apiCaller := apitesting.APICallerFunc(func(string, int, string, string, interface{}, interface{}) error {
		return errors.New("blam")
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	_, err := client.MinionReports(c.Context())
	c.Assert(err, tc.ErrorMatches, "blam")
}

func (s *ClientSuite) TestMinionReportsInvalidPhase(c *tc.C) {
	apiCaller := apitesting.APICallerFunc(func(_ string, _ int, _ string, _ string, _ interface{}, result interface{}) error {
		out := result.(*params.MinionReports)
		*out = params.MinionReports{
			Phase: "BLARGH",
		}
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	_, err := client.MinionReports(c.Context())
	c.Assert(err, tc.ErrorMatches, `invalid phase: "BLARGH"`)
}

func (s *ClientSuite) TestMinionReportsBadUnknownTag(c *tc.C) {
	apiCaller := apitesting.APICallerFunc(func(_ string, _ int, _ string, _ string, _ interface{}, result interface{}) error {
		out := result.(*params.MinionReports)
		*out = params.MinionReports{
			Phase:         "IMPORT",
			UnknownSample: []string{"carl"},
		}
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	_, err := client.MinionReports(c.Context())
	c.Assert(err, tc.ErrorMatches, `processing unknown agents: "carl" is not a valid tag`)
}

func (s *ClientSuite) TestMinionReportsBadFailedTag(c *tc.C) {
	apiCaller := apitesting.APICallerFunc(func(_ string, _ int, _ string, _ string, _ interface{}, result interface{}) error {
		out := result.(*params.MinionReports)
		*out = params.MinionReports{
			Phase:  "IMPORT",
			Failed: []string{"dave"},
		}
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	_, err := client.MinionReports(c.Context())
	c.Assert(err, tc.ErrorMatches, `processing failed agents: "dave" is not a valid tag`)
}

func (s *ClientSuite) TestMinionReportTimeout(c *tc.C) {
	apiCaller := apitesting.APICallerFunc(func(facade string, _ int, _, method string, _ interface{}, result interface{}) error {
		c.Assert(facade, tc.Equals, "MigrationMaster")
		c.Assert(method, tc.Equals, "MinionReportTimeout")

		out := result.(*params.StringResult)
		*out = params.StringResult{
			Result: "30s",
		}
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	timeout, err := client.MinionReportTimeout(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(timeout, tc.Equals, 30*time.Second)
}

func (s *ClientSuite) TestStreamModelLogs(c *tc.C) {
	caller := fakeConnector{path: new(string), attrs: &url.Values{}}
	client := migrationmaster.NewClient(caller, nil)
	stream, err := client.StreamModelLog(c.Context(), time.Date(2016, 12, 2, 10, 24, 1, 1000000, time.UTC))
	c.Assert(stream, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "colonel abrams")

	c.Assert(*caller.path, tc.Equals, "/log")
	c.Assert(*caller.attrs, tc.DeepEquals, url.Values{
		"version":       {"2"},
		"replay":        {"true"},
		"noTail":        {"true"},
		"startTime":     {"2016-12-02T10:24:01.001Z"},
		"includeEntity": nil,
		"includeModule": nil,
		"excludeEntity": nil,
		"excludeModule": nil,
	})
}

type fakeConnector struct {
	base.APICaller

	path  *string
	attrs *url.Values
}

func (fakeConnector) BestFacadeVersion(string) int {
	return 0
}

func (c fakeConnector) ConnectStream(_ context.Context, path string, attrs url.Values) (base.Stream, error) {
	*c.path = path
	*c.attrs = attrs
	return nil, errors.New("colonel abrams")
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

type fakeDoer struct {
	response *http.Response
	method   string
	url      string
}

func (d *fakeDoer) Do(req *http.Request) (*http.Response, error) {
	d.method = req.Method
	d.url = req.URL.String()
	return d.response, nil
}

func checkReader(c *tc.C, r io.Reader, expected string) {
	actual, err := io.ReadAll(r)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(actual), tc.Equals, expected)
}
