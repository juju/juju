// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster_test

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/httprequest"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6/resource"
	"gopkg.in/juju/names.v3"
	"gopkg.in/macaroon.v2-unstable"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/migrationmaster"
	macapitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/resource"
)

type ClientSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&ClientSuite{})

func (s *ClientSuite) TestWatch(c *gc.C) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		*(result.(*params.NotifyWatchResult)) = params.NotifyWatchResult{
			NotifyWatcherId: "123",
		}
		return nil
	})
	expectWatch := &struct{ watcher.NotifyWatcher }{}
	newWatcher := func(caller base.APICaller, result params.NotifyWatchResult) watcher.NotifyWatcher {
		c.Check(caller, gc.NotNil)
		c.Check(result, jc.DeepEquals, params.NotifyWatchResult{NotifyWatcherId: "123"})
		return expectWatch
	}
	client := migrationmaster.NewClient(apiCaller, newWatcher)
	w, err := client.Watch()
	c.Check(err, jc.ErrorIsNil)
	c.Check(w, gc.Equals, expectWatch)
	stub.CheckCalls(c, []jujutesting.StubCall{{"MigrationMaster.Watch", []interface{}{"", nil}}})
}

func (s *ClientSuite) TestWatchCallError(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("boom")
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	_, err := client.Watch()
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *ClientSuite) TestMigrationStatus(c *gc.C) {
	mac, err := macaroon.New([]byte("secret"), []byte("id"), "location")
	c.Assert(err, jc.ErrorIsNil)
	macs := []macaroon.Slice{{mac}}
	macsJSON, err := json.Marshal(macs)
	c.Assert(err, jc.ErrorIsNil)

	modelUUID := utils.MustNewUUID().String()
	controllerUUID := utils.MustNewUUID().String()
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
				},
			},
			MigrationId:      "id",
			Phase:            "IMPORT",
			PhaseChangedTime: timestamp,
		}
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	status, err := client.MigrationStatus()
	c.Assert(err, jc.ErrorIsNil)
	// Extract macaroons so we can compare them separately
	// (as they can't be compared using DeepEquals due to 'UnmarshaledAs')
	statusMacs := status.TargetInfo.Macaroons
	status.TargetInfo.Macaroons = nil
	macapitesting.MacaroonEquals(c, statusMacs[0][0], mac)
	c.Assert(status, gc.DeepEquals, migration.MigrationStatus{
		MigrationId:      "id",
		ModelUUID:        modelUUID,
		Phase:            migration.IMPORT,
		PhaseChangedTime: timestamp,
		TargetInfo: migration.TargetInfo{
			ControllerTag: controllerTag,
			Addrs:         []string{"2.2.2.2:2"},
			CACert:        "cert",
			AuthTag:       names.NewUserTag("admin"),
			Password:      "secret",
		},
	})
}

func (s *ClientSuite) TestSetPhase(c *gc.C) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	err := client.SetPhase(migration.QUIESCE)
	c.Assert(err, jc.ErrorIsNil)
	expectedArg := params.SetMigrationPhaseArgs{Phase: "QUIESCE"}
	stub.CheckCalls(c, []jujutesting.StubCall{
		{"MigrationMaster.SetPhase", []interface{}{"", expectedArg}},
	})
}

func (s *ClientSuite) TestSetPhaseError(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(string, int, string, string, interface{}, interface{}) error {
		return errors.New("boom")
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	err := client.SetPhase(migration.QUIESCE)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *ClientSuite) TestSetStatusMessage(c *gc.C) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	err := client.SetStatusMessage("foo")
	c.Assert(err, jc.ErrorIsNil)
	expectedArg := params.SetMigrationStatusMessageArgs{Message: "foo"}
	stub.CheckCalls(c, []jujutesting.StubCall{
		{"MigrationMaster.SetStatusMessage", []interface{}{"", expectedArg}},
	})
}

func (s *ClientSuite) TestSetStatusMessageError(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(string, int, string, string, interface{}, interface{}) error {
		return errors.New("boom")
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	err := client.SetStatusMessage("foo")
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *ClientSuite) TestModelInfo(c *gc.C) {
	var stub jujutesting.Stub
	owner := names.NewUserTag("owner")
	apiCaller := apitesting.APICallerFunc(func(objType string, v int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		*(result.(*params.MigrationModelInfo)) = params.MigrationModelInfo{
			UUID:                   "uuid",
			Name:                   "name",
			OwnerTag:               owner.String(),
			AgentVersion:           version.MustParse("1.2.3"),
			ControllerAgentVersion: version.MustParse("1.2.4"),
		}
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	model, err := client.ModelInfo()
	stub.CheckCalls(c, []jujutesting.StubCall{
		{"MigrationMaster.ModelInfo", []interface{}{"", nil}},
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(model, jc.DeepEquals, migration.ModelInfo{
		UUID:                   "uuid",
		Name:                   "name",
		Owner:                  owner,
		AgentVersion:           version.MustParse("1.2.3"),
		ControllerAgentVersion: version.MustParse("1.2.4"),
	})
}

func (s *ClientSuite) TestPrechecks(c *gc.C) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		return errors.New("blam")
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	err := client.Prechecks()
	c.Check(err, gc.ErrorMatches, "blam")
	stub.CheckCalls(c, []jujutesting.StubCall{
		{"MigrationMaster.Prechecks", []interface{}{"", nil}},
	})
}

func (s *ClientSuite) TestProcessRelations(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})

	client := migrationmaster.NewClient(apiCaller, nil)
	err := client.ProcessRelations("")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ClientSuite) TestExport(c *gc.C) {
	var stub jujutesting.Stub

	fpHash := charmresource.NewFingerprintHash()
	appFp := fpHash.Fingerprint()
	unitFp := fpHash.Fingerprint()

	appTs := time.Now()
	unitTs := appTs.Add(time.Hour)

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		out := result.(*params.SerializedModel)
		*out = params.SerializedModel{
			Bytes:  []byte("foo"),
			Charms: []string{"cs:foo-1"},
			Tools: []params.SerializedModelTools{{
				Version: "2.0.0-trusty-amd64",
				URI:     "/tools/0",
			}},
			Resources: []params.SerializedModelResource{{
				Application: "fooapp",
				Name:        "bin",
				ApplicationRevision: params.SerializedModelResourceRevision{
					Revision:       2,
					Type:           "file",
					Path:           "bin.tar.gz",
					Description:    "who knows",
					Origin:         "upload",
					FingerprintHex: appFp.Hex(),
					Size:           123,
					Timestamp:      appTs,
					Username:       "bob",
				},
				CharmStoreRevision: params.SerializedModelResourceRevision{
					// Imitate a placeholder for the test by having no Timestamp
					// and an empty Fingerpritn
					Revision:    3,
					Type:        "file",
					Path:        "fink.tar.gz",
					Description: "knows who",
					Origin:      "store",
					Size:        321,
					Username:    "xena",
				},
				UnitRevisions: map[string]params.SerializedModelResourceRevision{
					"fooapp/0": {
						Revision:       1,
						Type:           "file",
						Path:           "blink.tar.gz",
						Description:    "bo knows",
						Origin:         "store",
						FingerprintHex: unitFp.Hex(),
						Size:           222,
						Timestamp:      unitTs,
						Username:       "bambam",
					},
				},
			}},
		}
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	out, err := client.Export()
	c.Assert(err, jc.ErrorIsNil)
	stub.CheckCalls(c, []jujutesting.StubCall{
		{"MigrationMaster.Export", []interface{}{"", nil}},
	})
	c.Assert(out, gc.DeepEquals, migration.SerializedModel{
		Bytes:  []byte("foo"),
		Charms: []string{"cs:foo-1"},
		Tools: map[version.Binary]string{
			version.MustParseBinary("2.0.0-trusty-amd64"): "/tools/0",
		},
		Resources: []migration.SerializedModelResource{{
			ApplicationRevision: resource.Resource{
				Resource: charmresource.Resource{
					Meta: charmresource.Meta{
						Name:        "bin",
						Type:        charmresource.TypeFile,
						Path:        "bin.tar.gz",
						Description: "who knows",
					},
					Origin:      charmresource.OriginUpload,
					Revision:    2,
					Fingerprint: appFp,
					Size:        123,
				},
				ApplicationID: "fooapp",
				Username:      "bob",
				Timestamp:     appTs,
			},
			CharmStoreRevision: resource.Resource{
				Resource: charmresource.Resource{
					Meta: charmresource.Meta{
						Name:        "bin",
						Type:        charmresource.TypeFile,
						Path:        "fink.tar.gz",
						Description: "knows who",
					},
					Origin:   charmresource.OriginStore,
					Revision: 3,
					Size:     321,
				},
				ApplicationID: "fooapp",
				Username:      "xena",
			},
			UnitRevisions: map[string]resource.Resource{
				"fooapp/0": {
					Resource: charmresource.Resource{
						Meta: charmresource.Meta{
							Name:        "bin",
							Type:        charmresource.TypeFile,
							Path:        "blink.tar.gz",
							Description: "bo knows",
						},
						Origin:      charmresource.OriginStore,
						Revision:    1,
						Fingerprint: unitFp,
						Size:        222,
					},
					ApplicationID: "fooapp",
					Username:      "bambam",
					Timestamp:     unitTs,
				},
			},
		}},
	})
}

func (s *ClientSuite) TestExportError(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(string, int, string, string, interface{}, interface{}) error {
		return errors.New("blam")
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	_, err := client.Export()
	c.Assert(err, gc.ErrorMatches, "blam")
}

const resourceContent = "resourceful"

func setupFakeHTTP() (*migrationmaster.Client, *fakeDoer) {
	doer := &fakeDoer{
		response: &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(strings.NewReader(resourceContent)),
		},
	}
	caller := &fakeHTTPCaller{
		httpClient: &httprequest.Client{
			Doer: doer,
		},
	}
	return migrationmaster.NewClient(caller, nil), doer
}

func (s *ClientSuite) TestOpenResource(c *gc.C) {
	client, doer := setupFakeHTTP()
	r, err := client.OpenResource("app", "blob")
	c.Assert(err, jc.ErrorIsNil)
	checkReader(c, r, "resourceful")
	c.Check(doer.method, gc.Equals, "GET")
	c.Check(doer.url, gc.Equals, "/applications/app/resources/blob")
}

func (s *ClientSuite) TestReap(c *gc.C) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	err := client.Reap()
	c.Check(err, jc.ErrorIsNil)
	stub.CheckCalls(c, []jujutesting.StubCall{
		{"MigrationMaster.Reap", []interface{}{"", nil}},
	})
}

func (s *ClientSuite) TestReapError(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(string, int, string, string, interface{}, interface{}) error {
		return errors.New("blam")
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	err := client.Reap()
	c.Assert(err, gc.ErrorMatches, "blam")
}

func (s *ClientSuite) TestWatchMinionReports(c *gc.C) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		*(result.(*params.NotifyWatchResult)) = params.NotifyWatchResult{
			NotifyWatcherId: "123",
		}
		return nil
	})

	expectWatch := &struct{ watcher.NotifyWatcher }{}
	newWatcher := func(caller base.APICaller, result params.NotifyWatchResult) watcher.NotifyWatcher {
		c.Check(caller, gc.NotNil)
		c.Check(result, jc.DeepEquals, params.NotifyWatchResult{NotifyWatcherId: "123"})
		return expectWatch
	}
	client := migrationmaster.NewClient(apiCaller, newWatcher)
	w, err := client.WatchMinionReports()
	c.Check(err, jc.ErrorIsNil)
	c.Check(w, gc.Equals, expectWatch)
	stub.CheckCalls(c, []jujutesting.StubCall{{"MigrationMaster.WatchMinionReports", []interface{}{"", nil}}})
}

func (s *ClientSuite) TestWatchMinionReportsError(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("boom")
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	_, err := client.WatchMinionReports()
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *ClientSuite) TestMinionReports(c *gc.C) {
	var stub jujutesting.Stub
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
	out, err := client.MinionReports()
	c.Assert(err, jc.ErrorIsNil)
	stub.CheckCalls(c, []jujutesting.StubCall{
		{"MigrationMaster.MinionReports", []interface{}{"", nil}},
	})
	c.Assert(out, gc.DeepEquals, migration.MinionReports{
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

func (s *ClientSuite) TestMinionReportsFailedCall(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(string, int, string, string, interface{}, interface{}) error {
		return errors.New("blam")
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	_, err := client.MinionReports()
	c.Assert(err, gc.ErrorMatches, "blam")
}

func (s *ClientSuite) TestMinionReportsInvalidPhase(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(_ string, _ int, _ string, _ string, _ interface{}, result interface{}) error {
		out := result.(*params.MinionReports)
		*out = params.MinionReports{
			Phase: "BLARGH",
		}
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	_, err := client.MinionReports()
	c.Assert(err, gc.ErrorMatches, `invalid phase: "BLARGH"`)
}

func (s *ClientSuite) TestMinionReportsBadUnknownTag(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(_ string, _ int, _ string, _ string, _ interface{}, result interface{}) error {
		out := result.(*params.MinionReports)
		*out = params.MinionReports{
			Phase:         "IMPORT",
			UnknownSample: []string{"carl"},
		}
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	_, err := client.MinionReports()
	c.Assert(err, gc.ErrorMatches, `processing unknown agents: "carl" is not a valid tag`)
}

func (s *ClientSuite) TestMinionReportsBadFailedTag(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(_ string, _ int, _ string, _ string, _ interface{}, result interface{}) error {
		out := result.(*params.MinionReports)
		*out = params.MinionReports{
			Phase:  "IMPORT",
			Failed: []string{"dave"},
		}
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	_, err := client.MinionReports()
	c.Assert(err, gc.ErrorMatches, `processing failed agents: "dave" is not a valid tag`)
}

func (s *ClientSuite) TestStreamModelLogs(c *gc.C) {
	caller := fakeConnector{path: new(string), attrs: &url.Values{}}
	client := migrationmaster.NewClient(caller, nil)
	stream, err := client.StreamModelLog(time.Date(2016, 12, 2, 10, 24, 1, 1000000, time.UTC))
	c.Assert(stream, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "colonel abrams")

	c.Assert(*caller.path, gc.Equals, "/log")
	c.Assert(*caller.attrs, gc.DeepEquals, url.Values{
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

func (c fakeConnector) ConnectStream(path string, attrs url.Values) (base.Stream, error) {
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

func checkReader(c *gc.C, r io.Reader, expected string) {
	actual, err := ioutil.ReadAll(r)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(actual), gc.Equals, expected)
}
