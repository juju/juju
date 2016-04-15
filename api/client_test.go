// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/httprequest"
	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	"golang.org/x/net/websocket"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	jujunames "github.com/juju/juju/juju/names"
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

func (s *clientSuite) TestCloseMultipleOk(c *gc.C) {
	client := s.APIState.Client()
	c.Assert(client.Close(), gc.IsNil)
	c.Assert(client.Close(), gc.IsNil)
	c.Assert(client.Close(), gc.IsNil)
}

func (s *clientSuite) TestUploadToolsOtherEnvironment(c *gc.C) {
	otherSt, otherAPISt := s.otherEnviron(c)
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
	defer fakeAPIEndpoint(c, client, envEndpoint(c, otherAPISt, "tools"), "POST",
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

func (s *clientSuite) TestAddLocalCharm(c *gc.C) {
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	client := s.APIState.Client()

	// Test the sanity checks first.
	_, err := client.AddLocalCharm(charm.MustParseURL("cs:quantal/wordpress-1"), nil)
	c.Assert(err, gc.ErrorMatches, `expected charm URL with local: schema, got "cs:quantal/wordpress-1"`)

	// Upload an archive with its original revision.
	savedURL, err := client.AddLocalCharm(curl, charmArchive)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), gc.Equals, curl.String())

	// Upload a charm directory with changed revision.
	charmDir := testcharms.Repo.ClonedDir(c.MkDir(), "dummy")
	charmDir.SetDiskRevision(42)
	savedURL, err = client.AddLocalCharm(curl, charmDir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.Revision, gc.Equals, 42)

	// Upload a charm directory again, revision should be bumped.
	savedURL, err = client.AddLocalCharm(curl, charmDir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), gc.Equals, curl.WithRevision(43).String())
}

func (s *clientSuite) TestAddLocalCharmOtherEnvironment(c *gc.C) {
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)

	otherSt, otherAPISt := s.otherEnviron(c)
	defer otherSt.Close()
	defer otherAPISt.Close()
	client := otherAPISt.Client()

	// Upload an archive
	savedURL, err := client.AddLocalCharm(curl, charmArchive)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), gc.Equals, curl.String())

	charm, err := otherSt.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charm.String(), gc.Equals, curl.String())
}

func (s *clientSuite) otherEnviron(c *gc.C) (*state.State, api.Connection) {
	otherSt := s.Factory.MakeModel(c, nil)
	info := s.APIInfo(c)
	info.ModelTag = otherSt.ModelTag()
	apiState, err := api.Open(info, api.DefaultDialOpts())
	c.Assert(err, jc.ErrorIsNil)
	return otherSt, apiState
}

func (s *clientSuite) TestAddLocalCharmError(c *gc.C) {
	client := s.APIState.Client()

	// AddLocalCharm does not use the facades, so instead of patching the
	// facade call, we set up a fake endpoint to test.
	defer fakeAPIEndpoint(c, client, envEndpoint(c, s.APIState, "charms"), "POST",
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

	_, err := client.AddLocalCharm(curl, charmArchive)
	c.Assert(err, gc.ErrorMatches, `POST http://.*/model/deadbeef-0bad-400d-8000-4b1d0d06f00d/charms\?series=quantal: the POST method is not allowed`)
}

func (s *clientSuite) TestMinVersionLocalCharm(c *gc.C) {
	tests := []minverTest{
		{"2.0.0", "1.0.0", true},
		{"1.0.0", "2.0.0", false},
		{"1.25.0", "1.24.0", true},
		{"1.24.0", "1.25.0", false},
		{"1.25.1", "1.25.0", true},
		{"1.25.0", "1.25.1", false},
		{"1.25.0", "1.25.0", true},
		{"1.25.0", "1.25-alpha1", true},
		{"1.25-alpha1", "1.25.0", false},
	}
	client := s.APIState.Client()
	for _, t := range tests {
		testMinVer(client, t, c)
	}
}

type minverTest struct {
	juju  string
	charm string
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

	_, err := client.AddLocalCharm(curl, charmArchive)

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

func (s *clientSuite) TestOpenCharmFound(c *gc.C) {
	client := s.APIState.Client()
	curl := addLocalCharm(c, client, "dummy")

	reader, err := client.OpenCharm(curl)
	defer reader.Close()
	c.Assert(err, jc.ErrorIsNil)

	data, err := ioutil.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(data), gc.Equals, `{"ErrorInfo":null,"Files":["actions.yaml","config.yaml","empty","empty/.gitkeep","hooks","hooks/install","metadata.yaml","revision","src","src/hello.c"]}`)
}

func (s *clientSuite) TestOpenCharmMissing(c *gc.C) {
	curl := charm.MustParseURL("cs:quantal/spam-3")
	client := s.APIState.Client()

	_, err := client.OpenCharm(curl)

	c.Check(err, gc.ErrorMatches, `.*unable to retrieve and save the charm: cannot get charm from state: charm "cs:quantal/spam-3" not found`)
}

func addLocalCharm(c *gc.C, client *api.Client, name string) *charm.URL {
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), name)
	curl := charm.MustParseURL(fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()))
	_, err := client.AddLocalCharm(curl, charmArchive)
	c.Assert(err, jc.ErrorIsNil)
	return curl
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

// envEndpoint returns "/model/<model-uuid>/<destination>"
func envEndpoint(c *gc.C, apiState api.Connection, destination string) string {
	modelTag, err := apiState.ModelTag()
	c.Assert(err, jc.ErrorIsNil)
	return path.Join("/model", modelTag.Id(), destination)
}

func (s *clientSuite) TestClientEnvironmentUUID(c *gc.C) {
	environ, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	client := s.APIState.Client()
	c.Assert(client.ModelUUID(), gc.Equals, environ.Tag().Id())
}

func (s *clientSuite) TestClientEnvironmentUsers(c *gc.C) {
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

func (s *clientSuite) TestDestroyEnvironment(c *gc.C) {
	client := s.APIState.Client()
	var called bool
	cleanup := api.PatchClientFacadeCall(client,
		func(req string, args interface{}, resp interface{}) error {
			c.Assert(req, gc.Equals, "DestroyModel")
			called = true
			return nil
		})
	defer cleanup()

	err := client.DestroyModel()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *clientSuite) TestWatchDebugLogConnected(c *gc.C) {
	client := s.APIState.Client()
	// Use the no tail option so we don't try to start a tailing cursor
	// on the oplog when there is no oplog configured in mongo as the tests
	// don't set up mongo in replicaset mode.
	reader, err := client.WatchDebugLog(api.DebugLogParams{NoTail: true})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(reader, gc.NotNil)
	reader.Close()
}

func (s *clientSuite) TestConnectStreamRequiresSlashPathPrefix(c *gc.C) {
	reader, err := s.APIState.ConnectStream("foo", nil)
	c.Assert(err, gc.ErrorMatches, `path must start with "/"`)
	c.Assert(reader, gc.Equals, nil)
}

func (s *clientSuite) TestConnectStreamErrorBadConnection(c *gc.C) {
	s.PatchValue(api.WebsocketDialConfig, func(_ *websocket.Config) (base.Stream, error) {
		return nil, fmt.Errorf("bad connection")
	})
	reader, err := s.APIState.ConnectStream("/", nil)
	c.Assert(err, gc.ErrorMatches, "bad connection")
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectStreamErrorNoData(c *gc.C) {
	s.PatchValue(api.WebsocketDialConfig, func(_ *websocket.Config) (base.Stream, error) {
		return fakeStreamReader{&bytes.Buffer{}}, nil
	})
	reader, err := s.APIState.ConnectStream("/", nil)
	c.Assert(err, gc.ErrorMatches, "unable to read initial response: EOF")
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectStreamErrorBadData(c *gc.C) {
	s.PatchValue(api.WebsocketDialConfig, func(_ *websocket.Config) (base.Stream, error) {
		return fakeStreamReader{strings.NewReader("junk\n")}, nil
	})
	reader, err := s.APIState.ConnectStream("/", nil)
	c.Assert(err, gc.ErrorMatches, "unable to unmarshal initial response: .*")
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectStreamErrorReadError(c *gc.C) {
	s.PatchValue(api.WebsocketDialConfig, func(_ *websocket.Config) (base.Stream, error) {
		err := fmt.Errorf("bad read")
		return fakeStreamReader{&badReader{err}}, nil
	})
	reader, err := s.APIState.ConnectStream("/", nil)
	c.Assert(err, gc.ErrorMatches, "unable to read initial response: bad read")
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestWatchDebugLogParamsEncoded(c *gc.C) {
	s.PatchValue(api.WebsocketDialConfig, echoURL(c))

	params := api.DebugLogParams{
		IncludeEntity: []string{"a", "b"},
		IncludeModule: []string{"c", "d"},
		ExcludeEntity: []string{"e", "f"},
		ExcludeModule: []string{"g", "h"},
		Limit:         100,
		Backlog:       200,
		Level:         loggo.ERROR,
		Replay:        true,
		NoTail:        true,
	}

	client := s.APIState.Client()
	reader, err := client.WatchDebugLog(params)
	c.Assert(err, jc.ErrorIsNil)

	connectURL := connectURLFromReader(c, reader)
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
	})
}

func (s *clientSuite) TestConnectStreamAtUUIDPath(c *gc.C) {
	s.PatchValue(api.WebsocketDialConfig, echoURL(c))
	// If the server supports it, we should log at "/model/UUID/log"
	environ, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	info := s.APIInfo(c)
	info.ModelTag = environ.ModelTag()
	apistate, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer apistate.Close()
	reader, err := apistate.ConnectStream("/path", nil)
	c.Assert(err, jc.ErrorIsNil)
	connectURL := connectURLFromReader(c, reader)
	c.Assert(connectURL.Path, gc.Matches, fmt.Sprintf("/model/%s/path", environ.UUID()))
}

func (s *clientSuite) TestOpenUsesEnvironUUIDPaths(c *gc.C) {
	info := s.APIInfo(c)

	// Passing in the correct model UUID should work
	environ, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	info.ModelTag = environ.ModelTag()
	apistate, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	apistate.Close()

	// Passing in a bad model UUID should fail with a known error
	info.ModelTag = names.NewModelTag("dead-beef-123456")
	apistate, err = api.Open(info, api.DialOpts{})
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `unknown model: "dead-beef-123456"`,
		Code:    "not found",
	})
	c.Check(err, jc.Satisfies, params.IsCodeNotFound)
	c.Assert(apistate, gc.IsNil)
}

func (s *clientSuite) TestSetEnvironAgentVersionDuringUpgrade(c *gc.C) {
	// This is an integration test which ensure that a test with the
	// correct error code is seen by the client from the
	// SetModelAgentVersion call when an upgrade is in progress.
	envConfig, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := envConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageModel},
	})
	err = machine.SetAgentVersion(version.MustParseBinary(agentVersion.String() + "-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	nextVersion := version.MustParse("9.8.7")
	_, err = s.State.EnsureUpgradeInfo(machine.Id(), agentVersion, nextVersion)
	c.Assert(err, jc.ErrorIsNil)

	err = s.APIState.Client().SetModelAgentVersion(nextVersion)

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

func (s *clientSuite) TestEnvironmentGet(c *gc.C) {
	client := s.APIState.Client()
	env, err := client.ModelGet()
	c.Assert(err, jc.ErrorIsNil)
	// Check a known value, just checking that there is something there.
	c.Assert(env["type"], gc.Equals, "dummy")
}

func (s *clientSuite) TestEnvironmentSet(c *gc.C) {
	client := s.APIState.Client()
	err := client.ModelSet(map[string]interface{}{
		"some-name":  "value",
		"other-name": true,
	})
	c.Assert(err, jc.ErrorIsNil)
	// Check them using ModelGet.
	env, err := client.ModelGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env["some-name"], gc.Equals, "value")
	c.Assert(env["other-name"], gc.Equals, true)
}

func (s *clientSuite) TestEnvironmentUnset(c *gc.C) {
	client := s.APIState.Client()
	err := client.ModelSet(map[string]interface{}{
		"some-name": "value",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Now unset it and make sure it isn't there.
	err = client.ModelUnset("some-name")
	c.Assert(err, jc.ErrorIsNil)

	env, err := client.ModelGet()
	c.Assert(err, jc.ErrorIsNil)
	_, found := env["some-name"]
	c.Assert(found, jc.IsFalse)
}

// badReader raises err when Read is called.
type badReader struct {
	err error
}

func (r *badReader) Read(p []byte) (n int, err error) {
	return 0, r.err
}

func echoURL(c *gc.C) func(*websocket.Config) (base.Stream, error) {
	return func(config *websocket.Config) (base.Stream, error) {
		pr, pw := io.Pipe()
		go func() {
			fmt.Fprintf(pw, "null\n")
			fmt.Fprintf(pw, "%s\n", config.Location)
		}()
		return fakeStreamReader{pr}, nil
	}
}

func connectURLFromReader(c *gc.C, rc io.ReadCloser) *url.URL {
	bufReader := bufio.NewReader(rc)
	location, err := bufReader.ReadString('\n')
	c.Assert(err, jc.ErrorIsNil)
	connectURL, err := url.Parse(strings.TrimSpace(location))
	c.Assert(err, jc.ErrorIsNil)
	rc.Close()
	return connectURL
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

func (s fakeStreamReader) Write([]byte) (int, error) {
	panic("not implemented")
}

func (s fakeStreamReader) ReadJSON(v interface{}) error {
	panic("not implemented")
}

func (s fakeStreamReader) WriteJSON(v interface{}) error {
	panic("not implemented")
}
