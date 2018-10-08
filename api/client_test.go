// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/errors"
	"github.com/juju/httprequest"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/featureflag"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/common"
	servercommon "github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/feature"
	jujunames "github.com/juju/juju/juju/names"
	"github.com/juju/juju/juju/osenv"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type clientSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&clientSuite{})

// TODO(jam) 2013-08-27 http://pad.lv/1217282
// Right now most of the direct tests for api.Client behavior are in
// apiserver/client/*_test.go
func (s *clientSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	err := os.Setenv(osenv.JujuFeatureFlagEnvKey, feature.LXDProfile)
	c.Assert(err, jc.ErrorIsNil)
	defer os.Unsetenv(osenv.JujuFeatureFlagEnvKey)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

func (s *clientSuite) TestCloseMultipleOk(c *gc.C) {
	client := s.APIState.Client()
	c.Assert(client.Close(), gc.IsNil)
	c.Assert(client.Close(), gc.IsNil)
	c.Assert(client.Close(), gc.IsNil)
}

func (s *clientSuite) TestUploadToolsOtherModel(c *gc.C) {
	otherSt, otherAPISt := s.otherModel(c)
	defer otherSt.Close()
	defer otherAPISt.Close()
	client := otherAPISt.Client()
	newVersion := version.MustParseBinary("5.4.3-quantal-amd64")
	var called bool

	// build fake tools
	expectedTools, _ := coretesting.TarGz(
		coretesting.NewTarFile(jujunames.Jujud, 0777, "jujud contents "+newVersion.String()))

	// UploadTools does not use the facades, so instead of patching the
	// facade call, we set up a fake endpoint to test.
	defer fakeAPIEndpoint(c, client, modelEndpoint(c, otherAPISt, "tools"), "POST",
		func(w http.ResponseWriter, r *http.Request) {
			called = true

			c.Assert(r.URL.Query(), gc.DeepEquals, url.Values{
				"binaryVersion": []string{"5.4.3-quantal-amd64"},
				"series":        []string{""},
			})
			defer r.Body.Close()
			obtainedTools, err := ioutil.ReadAll(r.Body)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(obtainedTools, gc.DeepEquals, expectedTools)
		},
	).Close()

	// We don't test the error or tools results as we only wish to assert that
	// the API client POSTs the tools archive to the correct endpoint.
	client.UploadTools(bytes.NewReader(expectedTools), newVersion)
	c.Assert(called, jc.IsTrue)
}

func (s *clientSuite) TestZipHasHooks(c *gc.C) {
	ch := testcharms.Repo.CharmDir("storage-filesystem-subordinate") // has hooks
	tempFile, err := ioutil.TempFile(c.MkDir(), "charm")
	c.Assert(err, jc.ErrorIsNil)
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())
	err = ch.ArchiveTo(tempFile)
	c.Assert(err, jc.ErrorIsNil)
	f := *api.HasHooks
	hasHooks, err := f(tempFile.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hasHooks, jc.IsTrue)
}

func (s *clientSuite) TestZipHasNoHooks(c *gc.C) {
	ch := testcharms.Repo.CharmDir("category") // has no hooks
	tempFile, err := ioutil.TempFile(c.MkDir(), "charm")
	c.Assert(err, jc.ErrorIsNil)
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())
	err = ch.ArchiveTo(tempFile)
	c.Assert(err, jc.ErrorIsNil)
	f := *api.HasHooks
	hasHooks, err := f(tempFile.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hasHooks, jc.IsFalse)
}

func (s *clientSuite) TestAddLocalCharm(c *gc.C) {
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	client := s.APIState.Client()

	// Test the sanity checks first.
	_, err := client.AddLocalCharm(charm.MustParseURL("cs:quantal/wordpress-1"), nil, false)
	c.Assert(err, gc.ErrorMatches, `expected charm URL with local: schema, got "cs:quantal/wordpress-1"`)

	// Upload an archive with its original revision.
	savedURL, err := client.AddLocalCharm(curl, charmArchive, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), gc.Equals, curl.String())

	// Upload a charm directory with changed revision.
	charmDir := testcharms.Repo.ClonedDir(c.MkDir(), "dummy")
	charmDir.SetDiskRevision(42)
	savedURL, err = client.AddLocalCharm(curl, charmDir, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.Revision, gc.Equals, 42)

	// Upload a charm directory again, revision should be bumped.
	savedURL, err = client.AddLocalCharm(curl, charmDir, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), gc.Equals, curl.WithRevision(43).String())
}

func (s *clientSuite) TestAddLocalCharmFindingHooksError(c *gc.C) {
	s.assertAddLocalCharmFailed(c,
		func(string) (bool, error) {
			return true, fmt.Errorf("bad zip")
		},
		`bad zip`)
}

func (s *clientSuite) TestAddLocalCharmNoHooks(c *gc.C) {
	s.assertAddLocalCharmFailed(c,
		func(string) (bool, error) {
			return false, nil
		},
		`invalid charm \"dummy\": has no hooks`)
}

func (s *clientSuite) TestAddLocalCharmWithLXDProfile(c *gc.C) {
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "lxd-profile")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	client := s.APIState.Client()

	// Upload an archive with its original revision.
	savedURL, err := client.AddLocalCharm(curl, charmArchive, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), gc.Equals, curl.String())

	// Upload a charm directory with changed revision.
	charmDir := testcharms.Repo.ClonedDir(c.MkDir(), "lxd-profile")
	charmDir.SetDiskRevision(42)
	savedURL, err = client.AddLocalCharm(curl, charmDir, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.Revision, gc.Equals, 42)

	// Upload a charm directory again, revision should be bumped.
	savedURL, err = client.AddLocalCharm(curl, charmDir, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), gc.Equals, curl.WithRevision(43).String())
}

func (s *clientSuite) TestAddLocalCharmWithInvalidLXDProfile(c *gc.C) {
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "lxd-profile-fail")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	client := s.APIState.Client()

	// Upload an archive with its original revision.
	_, err := client.AddLocalCharm(curl, charmArchive, false)
	c.Assert(err, gc.ErrorMatches, "invalid lxd-profile.yaml: contains device type \"unix-disk\"")
}

func (s *clientSuite) TestAddLocalCharmWithValidLXDProfileWithForceSucceeds(c *gc.C) {
	s.testAddLocalCharmWithWithForceSucceeds("lxd-profile", c)
}

func (s *clientSuite) TestAddLocalCharmWithInvalidLXDProfileWithForceSucceeds(c *gc.C) {
	s.testAddLocalCharmWithWithForceSucceeds("lxd-profile-fail", c)
}

func (s *clientSuite) testAddLocalCharmWithWithForceSucceeds(name string, c *gc.C) {
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), name)
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	client := s.APIState.Client()

	// Upload an archive with its original revision.
	savedURL, err := client.AddLocalCharm(curl, charmArchive, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), gc.Equals, curl.String())

	// Upload a charm directory with changed revision.
	charmDir := testcharms.Repo.ClonedDir(c.MkDir(), name)
	charmDir.SetDiskRevision(42)
	savedURL, err = client.AddLocalCharm(curl, charmDir, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.Revision, gc.Equals, 42)

	// Upload a charm directory again, revision should be bumped.
	savedURL, err = client.AddLocalCharm(curl, charmDir, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), gc.Equals, curl.WithRevision(43).String())
}

func (s *clientSuite) assertAddLocalCharmFailed(c *gc.C, f func(string) (bool, error), msg string) {
	curl, ch := s.testCharm(c)
	s.PatchValue(api.HasHooks, f)
	_, err := s.APIState.Client().AddLocalCharm(curl, ch, false)
	c.Assert(err, gc.ErrorMatches, msg)
}

func (s *clientSuite) TestAddLocalCharmDefinetelyWithHooks(c *gc.C) {
	curl, ch := s.testCharm(c)
	s.PatchValue(api.HasHooks, func(string) (bool, error) {
		return true, nil
	})
	savedCURL, err := s.APIState.Client().AddLocalCharm(curl, ch, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedCURL.String(), gc.Equals, curl.String())
}

func (s *clientSuite) testCharm(c *gc.C) (*charm.URL, charm.Charm) {
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	return curl, charmArchive
}

func (s *clientSuite) TestAddLocalCharmOtherModel(c *gc.C) {
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)

	otherSt, otherAPISt := s.otherModel(c)
	defer otherSt.Close()
	defer otherAPISt.Close()
	client := otherAPISt.Client()

	// Upload an archive
	savedURL, err := client.AddLocalCharm(curl, charmArchive, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), gc.Equals, curl.String())

	charm, err := otherSt.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charm.String(), gc.Equals, curl.String())
}

func (s *clientSuite) otherModel(c *gc.C) (*state.State, api.Connection) {
	otherSt := s.Factory.MakeModel(c, nil)
	info := s.APIInfo(c)
	model, err := otherSt.Model()
	c.Assert(err, jc.ErrorIsNil)
	info.ModelTag = model.ModelTag()
	apiState, err := api.Open(info, api.DefaultDialOpts())
	c.Assert(err, jc.ErrorIsNil)
	return otherSt, apiState
}

func (s *clientSuite) TestAddLocalCharmError(c *gc.C) {
	client := s.APIState.Client()

	// AddLocalCharm does not use the facades, so instead of patching the
	// facade call, we set up a fake endpoint to test.
	defer fakeAPIEndpoint(c, client, modelEndpoint(c, s.APIState, "charms"), "POST",
		func(w http.ResponseWriter, r *http.Request) {
			httprequest.WriteJSON(w, http.StatusMethodNotAllowed, &params.CharmsResponse{
				Error:     "the POST method is not allowed",
				ErrorCode: params.CodeMethodNotAllowed,
			})
		},
	).Close()

	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)

	_, err := client.AddLocalCharm(curl, charmArchive, false)
	c.Assert(err, gc.ErrorMatches, `.*the POST method is not allowed$`)
}

func (s *clientSuite) TestMinVersionLocalCharm(c *gc.C) {
	tests := []minverTest{
		{"2.0.0", "1.0.0", false, true},
		{"1.0.0", "2.0.0", false, false},
		{"1.25.0", "1.24.0", false, true},
		{"1.24.0", "1.25.0", false, false},
		{"1.25.1", "1.25.0", false, true},
		{"1.25.0", "1.25.1", false, false},
		{"1.25.0", "1.25.0", false, true},
		{"1.25.0", "1.25-alpha1", false, true},
		{"1.25-alpha1", "1.25.0", false, false},
		{"2.0.0", "1.0.0", true, true},
		{"1.0.0", "2.0.0", true, false},
		{"1.25.0", "1.24.0", true, true},
		{"1.24.0", "1.25.0", true, false},
		{"1.25.1", "1.25.0", true, true},
		{"1.25.0", "1.25.1", true, false},
		{"1.25.0", "1.25.0", true, true},
		{"1.25.0", "1.25-alpha1", true, true},
		{"1.25-alpha1", "1.25.0", true, false},
	}
	client := s.APIState.Client()
	for _, t := range tests {
		testMinVer(client, t, c)
	}
}

type minverTest struct {
	juju  string
	charm string
	force bool
	ok    bool
}

func testMinVer(client *api.Client, t minverTest, c *gc.C) {
	charmMinVer := version.MustParse(t.charm)
	jujuVer := version.MustParse(t.juju)

	cleanup := api.PatchClientFacadeCall(client,
		func(request string, paramsIn interface{}, response interface{}) error {
			c.Assert(paramsIn, gc.IsNil)
			if response, ok := response.(*params.AgentVersionResult); ok {
				response.Version = jujuVer
			} else {
				c.Log("wrong output structure")
				c.Fail()
			}
			return nil
		},
	)
	defer cleanup()

	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	charmArchive.Meta().MinJujuVersion = charmMinVer

	_, err := client.AddLocalCharm(curl, charmArchive, t.force)

	if t.ok {
		if err != nil {
			c.Errorf("Unexpected non-nil error for jujuver %v, minver %v: %#v", t.juju, t.charm, err)
		}
	} else {
		if err == nil {
			c.Errorf("Unexpected nil error for jujuver %v, minver %v", t.juju, t.charm)
		} else if !api.IsMinVersionError(err) {
			c.Errorf("Wrong error for jujuver %v, minver %v: expected minVersionError, got: %#v", t.juju, t.charm, err)
		}
	}
}

func (s *clientSuite) TestOpenURIFound(c *gc.C) {
	// Use tools download to test OpenURI
	const toolsVersion = "2.0.0-xenial-ppc64"
	s.AddToolsToState(c, version.MustParseBinary(toolsVersion))

	client := s.APIState.Client()
	reader, err := client.OpenURI("/tools/"+toolsVersion, nil)
	c.Assert(err, jc.ErrorIsNil)
	defer reader.Close()

	// The fake tools content will be the version number.
	content, err := ioutil.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, toolsVersion)
}

func (s *clientSuite) TestOpenURIError(c *gc.C) {
	client := s.APIState.Client()
	_, err := client.OpenURI("/tools/foobar", nil)
	c.Assert(err, gc.ErrorMatches, ".*error parsing version.+")
}

func (s *clientSuite) TestOpenCharmFound(c *gc.C) {
	client := s.APIState.Client()
	curl, ch := addLocalCharm(c, client, "dummy", false)
	expected, err := ioutil.ReadFile(ch.Path)
	c.Assert(err, jc.ErrorIsNil)

	reader, err := client.OpenCharm(curl)
	defer reader.Close()
	c.Assert(err, jc.ErrorIsNil)

	data, err := ioutil.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(data, jc.DeepEquals, expected)
}

func (s *clientSuite) TestOpenCharmFoundWithForceStillSucceeds(c *gc.C) {
	client := s.APIState.Client()
	curl, ch := addLocalCharm(c, client, "dummy", true)
	expected, err := ioutil.ReadFile(ch.Path)
	c.Assert(err, jc.ErrorIsNil)

	reader, err := client.OpenCharm(curl)
	defer reader.Close()
	c.Assert(err, jc.ErrorIsNil)

	data, err := ioutil.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(data, jc.DeepEquals, expected)
}

func (s *clientSuite) TestOpenCharmMissing(c *gc.C) {
	curl := charm.MustParseURL("cs:quantal/spam-3")
	client := s.APIState.Client()

	_, err := client.OpenCharm(curl)

	c.Check(err, gc.ErrorMatches, `.*cannot get charm from state: charm "cs:quantal/spam-3" not found`)
}

func addLocalCharm(c *gc.C, client *api.Client, name string, force bool) (*charm.URL, *charm.CharmArchive) {
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), name)
	curl := charm.MustParseURL(fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()))
	_, err := client.AddLocalCharm(curl, charmArchive, force)
	c.Assert(err, jc.ErrorIsNil)
	return curl, charmArchive
}

func fakeAPIEndpoint(c *gc.C, client *api.Client, address, method string, handle func(http.ResponseWriter, *http.Request)) net.Listener {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, jc.ErrorIsNil)

	mux := http.NewServeMux()
	mux.HandleFunc(address, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == method {
			handle(w, r)
		}
	})
	go func() {
		http.Serve(lis, mux)
	}()
	api.SetServerAddress(client, "http", lis.Addr().String())
	return lis
}

// modelEndpoint returns "/model/<model-uuid>/<destination>"
func modelEndpoint(c *gc.C, apiState api.Connection, destination string) string {
	modelTag, ok := apiState.ModelTag()
	c.Assert(ok, jc.IsTrue)
	return path.Join("/model", modelTag.Id(), destination)
}

func (s *clientSuite) TestClientModelUUID(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	client := s.APIState.Client()
	uuid, ok := client.ModelUUID()
	c.Assert(ok, jc.IsTrue)
	c.Assert(uuid, gc.Equals, model.Tag().Id())
}

func (s *clientSuite) TestClientModelUsers(c *gc.C) {
	client := s.APIState.Client()
	cleanup := api.PatchClientFacadeCall(client,
		func(request string, paramsIn interface{}, response interface{}) error {
			c.Assert(paramsIn, gc.IsNil)
			if response, ok := response.(*params.ModelUserInfoResults); ok {
				response.Results = []params.ModelUserInfoResult{
					{Result: &params.ModelUserInfo{UserName: "one"}},
					{Result: &params.ModelUserInfo{UserName: "two"}},
					{Result: &params.ModelUserInfo{UserName: "three"}},
				}
			} else {
				c.Log("wrong output structure")
				c.Fail()
			}
			return nil
		},
	)
	defer cleanup()

	obtained, err := client.ModelUserInfo()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(obtained, jc.DeepEquals, []params.ModelUserInfo{
		{UserName: "one"},
		{UserName: "two"},
		{UserName: "three"},
	})
}

func (s *clientSuite) TestWatchDebugLogConnected(c *gc.C) {
	client := s.APIState.Client()
	// Use the no tail option so we don't try to start a tailing cursor
	// on the oplog when there is no oplog configured in mongo as the tests
	// don't set up mongo in replicaset mode.
	messages, err := client.WatchDebugLog(common.DebugLogParams{NoTail: true})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(messages, gc.NotNil)
}

func (s *clientSuite) TestConnectStreamRequiresSlashPathPrefix(c *gc.C) {
	reader, err := s.APIState.ConnectStream("foo", nil)
	c.Assert(err, gc.ErrorMatches, `cannot make API path from non-slash-prefixed path "foo"`)
	c.Assert(reader, gc.Equals, nil)
}

func (s *clientSuite) TestConnectStreamErrorBadConnection(c *gc.C) {
	s.PatchValue(api.WebsocketDial, func(_ api.WebsocketDialer, _ string, _ http.Header) (base.Stream, error) {
		return nil, fmt.Errorf("bad connection")
	})
	reader, err := s.APIState.ConnectStream("/", nil)
	c.Assert(err, gc.ErrorMatches, "bad connection")
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectStreamErrorNoData(c *gc.C) {
	s.PatchValue(api.WebsocketDial, func(_ api.WebsocketDialer, _ string, _ http.Header) (base.Stream, error) {
		return fakeStreamReader{&bytes.Buffer{}}, nil
	})
	reader, err := s.APIState.ConnectStream("/", nil)
	c.Assert(err, gc.ErrorMatches, "unable to read initial response: EOF")
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectStreamErrorBadData(c *gc.C) {
	s.PatchValue(api.WebsocketDial, func(_ api.WebsocketDialer, _ string, _ http.Header) (base.Stream, error) {
		return fakeStreamReader{strings.NewReader("junk\n")}, nil
	})
	reader, err := s.APIState.ConnectStream("/", nil)
	c.Assert(err, gc.ErrorMatches, "unable to unmarshal initial response: .*")
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectStreamErrorReadError(c *gc.C) {
	s.PatchValue(api.WebsocketDial, func(_ api.WebsocketDialer, _ string, _ http.Header) (base.Stream, error) {
		err := fmt.Errorf("bad read")
		return fakeStreamReader{&badReader{err}}, nil
	})
	reader, err := s.APIState.ConnectStream("/", nil)
	c.Assert(err, gc.ErrorMatches, "unable to read initial response: bad read")
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectControllerStreamRejectsRelativePaths(c *gc.C) {
	reader, err := s.APIState.ConnectControllerStream("foo", nil, nil)
	c.Assert(err, gc.ErrorMatches, `path "foo" is not absolute`)
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectControllerStreamRejectsModelPaths(c *gc.C) {
	reader, err := s.APIState.ConnectControllerStream("/model/foo", nil, nil)
	c.Assert(err, gc.ErrorMatches, `path "/model/foo" is model-specific`)
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectControllerStreamAppliesHeaders(c *gc.C) {
	catcher := urlCatcher{}
	headers := http.Header{}
	headers.Add("thomas", "cromwell")
	headers.Add("anne", "boleyn")
	s.PatchValue(api.WebsocketDial, catcher.recordLocation)

	_, err := s.APIState.ConnectControllerStream("/something", nil, headers)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(catcher.headers.Get("thomas"), gc.Equals, "cromwell")
	c.Assert(catcher.headers.Get("anne"), gc.Equals, "boleyn")
}

func (s *clientSuite) TestWatchDebugLogParamsEncoded(c *gc.C) {
	catcher := urlCatcher{}
	s.PatchValue(api.WebsocketDial, catcher.recordLocation)

	params := common.DebugLogParams{
		IncludeEntity: []string{"a", "b"},
		IncludeModule: []string{"c", "d"},
		ExcludeEntity: []string{"e", "f"},
		ExcludeModule: []string{"g", "h"},
		Limit:         100,
		Backlog:       200,
		Level:         loggo.ERROR,
		Replay:        true,
		NoTail:        true,
		StartTime:     time.Date(2016, 11, 30, 11, 48, 0, 100, time.UTC),
	}

	client := s.APIState.Client()
	_, err := client.WatchDebugLog(params)
	c.Assert(err, jc.ErrorIsNil)

	connectURL, err := url.Parse(catcher.location)
	c.Assert(err, jc.ErrorIsNil)

	values := connectURL.Query()
	c.Assert(values, jc.DeepEquals, url.Values{
		"includeEntity": params.IncludeEntity,
		"includeModule": params.IncludeModule,
		"excludeEntity": params.ExcludeEntity,
		"excludeModule": params.ExcludeModule,
		"maxLines":      {"100"},
		"backlog":       {"200"},
		"level":         {"ERROR"},
		"replay":        {"true"},
		"noTail":        {"true"},
		"startTime":     {"2016-11-30T11:48:00.0000001Z"},
	})
}

func (s *clientSuite) TestConnectStreamAtUUIDPath(c *gc.C) {
	catcher := urlCatcher{}
	s.PatchValue(api.WebsocketDial, catcher.recordLocation)
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	info := s.APIInfo(c)
	info.ModelTag = model.ModelTag()
	apistate, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer apistate.Close()
	_, err = apistate.ConnectStream("/path", nil)
	c.Assert(err, jc.ErrorIsNil)
	connectURL, err := url.Parse(catcher.location)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(connectURL.Path, gc.Matches, fmt.Sprintf("/model/%s/path", model.UUID()))
}

func (s *clientSuite) TestOpenUsesModelUUIDPaths(c *gc.C) {
	info := s.APIInfo(c)

	// Passing in the correct model UUID should work
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	info.ModelTag = model.ModelTag()
	apistate, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	apistate.Close()

	// Passing in an unknown model UUID should fail with a known error
	info.ModelTag = names.NewModelTag("1eaf1e55-70ad-face-b007-70ad57001999")
	apistate, err = api.Open(info, api.DialOpts{})
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `unknown model: "1eaf1e55-70ad-face-b007-70ad57001999"`,
		Code:    "model not found",
	})
	c.Check(err, jc.Satisfies, params.IsCodeModelNotFound)
	c.Assert(apistate, gc.IsNil)
}

func (s *clientSuite) TestSetModelAgentVersionDuringUpgrade(c *gc.C) {
	// This is an integration test which ensure that a test with the
	// correct error code is seen by the client from the
	// SetModelAgentVersion call when an upgrade is in progress.
	modelConfig, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := modelConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageModel},
	})
	err = machine.SetAgentVersion(version.MustParseBinary(agentVersion.String() + "-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	nextVersion := version.MustParse("9.8.7")
	_, err = s.State.EnsureUpgradeInfo(machine.Id(), agentVersion, nextVersion)
	c.Assert(err, jc.ErrorIsNil)

	err = s.APIState.Client().SetModelAgentVersion(nextVersion, false)

	// Expect an error with a error code that indicates this specific
	// situation. The client needs to be able to reliably identify
	// this error and handle it differently to other errors.
	c.Assert(params.IsCodeUpgradeInProgress(err), jc.IsTrue)
}

func (s *clientSuite) TestAbortCurrentUpgrade(c *gc.C) {
	client := s.APIState.Client()
	someErr := errors.New("random")
	cleanup := api.PatchClientFacadeCall(client,
		func(request string, args interface{}, response interface{}) error {
			c.Assert(request, gc.Equals, "AbortCurrentUpgrade")
			c.Assert(args, gc.IsNil)
			c.Assert(response, gc.IsNil)
			return someErr
		},
	)
	defer cleanup()

	err := client.AbortCurrentUpgrade()
	c.Assert(err, gc.Equals, someErr) // Confirms that the correct facade was called
}

func (s *clientSuite) TestWebsocketDialWithErrorsJSON(c *gc.C) {
	errorResult := params.ErrorResult{
		Error: servercommon.ServerError(errors.New("kablooie")),
	}
	data, err := json.Marshal(errorResult)
	c.Assert(err, jc.ErrorIsNil)
	cw := closeWatcher{Reader: bytes.NewReader(data)}
	d := fakeDialer{
		resp: &http.Response{
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
			Body: &cw,
		},
	}
	d.SetErrors(websocket.ErrBadHandshake)
	stream, err := api.WebsocketDialWithErrors(&d, "something", nil)
	c.Assert(stream, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "kablooie")
	c.Assert(cw.closed, gc.Equals, true)
}

func (s *clientSuite) TestWebsocketDialWithErrorsNoJSON(c *gc.C) {
	cw := closeWatcher{Reader: strings.NewReader("wowee zowee")}
	d := fakeDialer{
		resp: &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       &cw,
		},
	}
	d.SetErrors(websocket.ErrBadHandshake)
	stream, err := api.WebsocketDialWithErrors(&d, "something", nil)
	c.Assert(stream, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `wowee zowee \(Not Found\)`)
	c.Assert(cw.closed, gc.Equals, true)
}

func (s *clientSuite) TestWebsocketDialWithErrorsOtherError(c *gc.C) {
	var d fakeDialer
	d.SetErrors(errors.New("jammy pac"))
	stream, err := api.WebsocketDialWithErrors(&d, "something", nil)
	c.Assert(stream, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "jammy pac")
}

// badReader raises err when Read is called.
type badReader struct {
	err error
}

func (r *badReader) Read(p []byte) (n int, err error) {
	return 0, r.err
}

type urlCatcher struct {
	location string
	headers  http.Header
}

func (u *urlCatcher) recordLocation(d api.WebsocketDialer, urlStr string, header http.Header) (base.Stream, error) {
	u.location = urlStr
	u.headers = header
	pr, pw := io.Pipe()
	go func() {
		fmt.Fprintf(pw, "null\n")
	}()
	return fakeStreamReader{pr}, nil
}

type fakeStreamReader struct {
	io.Reader
}

func (s fakeStreamReader) Close() error {
	if c, ok := s.Reader.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func (s fakeStreamReader) NextReader() (messageType int, r io.Reader, err error) {
	return websocket.TextMessage, s.Reader, nil
}

func (s fakeStreamReader) Write([]byte) (int, error) {
	return 0, errors.NotImplementedf("Write")
}

func (s fakeStreamReader) ReadJSON(v interface{}) error {
	return errors.NotImplementedf("ReadJSON")
}

func (s fakeStreamReader) WriteJSON(v interface{}) error {
	return errors.NotImplementedf("WriteJSON")
}

type fakeDialer struct {
	testing.Stub

	conn *websocket.Conn
	resp *http.Response
}

func (d *fakeDialer) Dial(url string, header http.Header) (*websocket.Conn, *http.Response, error) {
	d.AddCall("Dial", url, header)
	return d.conn, d.resp, d.NextErr()
}

type closeWatcher struct {
	io.Reader
	closed bool
}

func (c *closeWatcher) Close() error {
	c.closed = true
	return nil
}

type IsolatedClientSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&IsolatedClientSuite{})

func (s *IsolatedClientSuite) TestFindAllErrorsOnOlderController(c *gc.C) {
	apiCaller := apitesting.BestVersionCaller{BestVersion: 1}
	client := api.APIClient(apiCaller)
	_, err := client.FindTools(0, 0, "", "", "proposed")
	c.Assert(err, gc.ErrorMatches, "passing agent-stream not supported by the controller")
}
