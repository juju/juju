// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"

	"code.google.com/p/go.net/websocket"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/version"
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

func (s *clientSuite) TestAddLocalCharmError(c *gc.C) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, jc.ErrorIsNil)
	defer lis.Close()
	url := fmt.Sprintf("http://%v", lis.Addr())
	http.HandleFunc("/charms", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	})
	go func() {
		http.Serve(lis, nil)
	}()

	client := s.APIState.Client()
	api.SetServerRoot(client, url)

	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	_, err = client.AddLocalCharm(curl, charmArchive)
	c.Assert(err, gc.ErrorMatches, "charm upload failed: 405 \\(Method Not Allowed\\)")
}

func (s *clientSuite) TestClientEnvironmentUUID(c *gc.C) {
	environ, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)

	client := s.APIState.Client()
	c.Assert(client.EnvironmentUUID(), gc.Equals, environ.Tag().Id())
}

func (s *clientSuite) TestShareEnvironmentExistingUser(c *gc.C) {
	client := s.APIState.Client()
	user := s.Factory.MakeEnvUser(c, nil)
	cleanup := api.PatchClientFacadeCall(client,
		func(request string, paramsIn interface{}, response interface{}) error {
			if users, ok := paramsIn.(params.ModifyEnvironUsers); ok {
				c.Assert(users.Changes, gc.HasLen, 1)
				c.Logf(string(users.Changes[0].Action), gc.Equals, string(params.AddEnvUser))
				c.Logf(users.Changes[0].UserTag, gc.Equals, user.UserTag().String())
			} else {
				c.Fatalf("wrong input structure")
			}
			if result, ok := response.(*params.ErrorResults); ok {
				err := &params.Error{Message: "failed to create environment user: env user already exists"}
				*result = params.ErrorResults{Results: []params.ErrorResult{{Error: err}}}
			} else {
				c.Fatalf("wrong input structure")
			}
			return nil
		},
	)
	defer cleanup()

	err := client.ShareEnvironment([]names.UserTag{user.UserTag()})
	c.Assert(err, gc.ErrorMatches, "failed to create environment user: env user already exists")
}

func (s *clientSuite) TestShareEnvironmentThreeUsers(c *gc.C) {
	client := s.APIState.Client()
	existingUser := s.Factory.MakeEnvUser(c, nil)
	localUser := s.Factory.MakeUser(c, nil)
	newUserTag := names.NewUserTag("foo@bar")
	cleanup := api.PatchClientFacadeCall(client,
		func(request string, paramsIn interface{}, response interface{}) error {
			if users, ok := paramsIn.(params.ModifyEnvironUsers); ok {
				c.Assert(users.Changes, gc.HasLen, 3)
				c.Logf(string(users.Changes[0].Action), gc.Equals, string(params.AddEnvUser))
				c.Logf(users.Changes[0].UserTag, gc.Equals, existingUser.UserTag().String())
				c.Logf(string(users.Changes[1].Action), gc.Equals, string(params.AddEnvUser))
				c.Logf(users.Changes[1].UserTag, gc.Equals, localUser.UserTag().String())
				c.Logf(string(users.Changes[1].Action), gc.Equals, string(params.AddEnvUser))
				c.Logf(users.Changes[1].UserTag, gc.Equals, newUserTag.String())
			} else {
				c.Log("wrong input structure")
				c.Fail()
			}
			if result, ok := response.(*params.ErrorResults); ok {
				err := &params.Error{Message: "existing user"}
				*result = params.ErrorResults{Results: []params.ErrorResult{{Error: err}, {Error: nil}, {Error: nil}}}
			} else {
				c.Log("wrong output structure")
				c.Fail()
			}
			return nil
		},
	)
	defer cleanup()

	err := client.ShareEnvironment([]names.UserTag{existingUser.UserTag(), localUser.UserTag(), newUserTag})
	c.Assert(err, gc.ErrorMatches, `existing user`)
}

func (s *clientSuite) TestShareEnvironmentRealAPIServer(c *gc.C) {
	client := s.APIState.Client()
	user := names.NewUserTag("foo@ubuntuone")
	err := client.ShareEnvironment([]names.UserTag{user})
	c.Assert(err, jc.ErrorIsNil)

	envUser, err := s.State.EnvironmentUser(user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envUser.UserName(), gc.Equals, user.Username())
	c.Assert(envUser.CreatedBy(), gc.Equals, s.AdminUserTag(c).Username())
	c.Assert(envUser.LastConnection(), gc.IsNil)
}

func (s *clientSuite) TestUnshareEnvironmentRealAPIServer(c *gc.C) {
	client := s.APIState.Client()
	user := names.NewUserTag("foo@ubuntuone")
	err := client.ShareEnvironment([]names.UserTag{user})
	c.Assert(err, jc.ErrorIsNil)

	envUser, err := s.State.EnvironmentUser(user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envUser.UserName(), gc.Equals, user.Username())

	err = client.UnshareEnvironment([]names.UserTag{user})
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.EnvironmentUser(user)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *clientSuite) TestWatchDebugLogConnected(c *gc.C) {
	// Shows both the unmarshalling of a real error, and
	// that the api server is connected.
	client := s.APIState.Client()
	reader, err := client.WatchDebugLog(api.DebugLogParams{})
	c.Assert(err, gc.ErrorMatches, "cannot open log file: .*")
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectionErrorBadConnection(c *gc.C) {
	s.PatchValue(api.WebsocketDialConfig, func(_ *websocket.Config) (io.ReadCloser, error) {
		return nil, fmt.Errorf("bad connection")
	})
	client := s.APIState.Client()
	reader, err := client.WatchDebugLog(api.DebugLogParams{})
	c.Assert(err, gc.ErrorMatches, "bad connection")
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectionErrorNoData(c *gc.C) {
	s.PatchValue(api.WebsocketDialConfig, func(_ *websocket.Config) (io.ReadCloser, error) {
		return ioutil.NopCloser(&bytes.Buffer{}), nil
	})
	client := s.APIState.Client()
	reader, err := client.WatchDebugLog(api.DebugLogParams{})
	c.Assert(err, gc.ErrorMatches, "unable to read initial response: EOF")
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectionErrorBadData(c *gc.C) {
	s.PatchValue(api.WebsocketDialConfig, func(_ *websocket.Config) (io.ReadCloser, error) {
		junk := strings.NewReader("junk\n")
		return ioutil.NopCloser(junk), nil
	})
	client := s.APIState.Client()
	reader, err := client.WatchDebugLog(api.DebugLogParams{})
	c.Assert(err, gc.ErrorMatches, "unable to unmarshal initial response: .*")
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectionErrorReadError(c *gc.C) {
	s.PatchValue(api.WebsocketDialConfig, func(_ *websocket.Config) (io.ReadCloser, error) {
		err := fmt.Errorf("bad read")
		return ioutil.NopCloser(&badReader{err}), nil
	})
	client := s.APIState.Client()
	reader, err := client.WatchDebugLog(api.DebugLogParams{})
	c.Assert(err, gc.ErrorMatches, "unable to read initial response: bad read")
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestParamsEncoded(c *gc.C) {
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
	}

	client := s.APIState.Client()
	reader, err := client.WatchDebugLog(params)
	c.Assert(err, jc.ErrorIsNil)

	connectURL := connectURLFromReader(c, reader)

	c.Assert(connectURL.Path, gc.Matches, "/log")
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
	})
}

func (s *clientSuite) TestDebugLogRootPath(c *gc.C) {
	s.PatchValue(api.WebsocketDialConfig, echoURL(c))

	// If the server is old, we log at "/log"
	info := s.APIInfo(c)
	info.EnvironTag = names.NewEnvironTag("")
	apistate, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer apistate.Close()
	reader, err := apistate.Client().WatchDebugLog(api.DebugLogParams{})
	c.Assert(err, jc.ErrorIsNil)
	connectURL := connectURLFromReader(c, reader)
	c.Assert(connectURL.Path, gc.Matches, "/log")
}

func (s *clientSuite) TestDebugLogAtUUIDLogPath(c *gc.C) {
	s.PatchValue(api.WebsocketDialConfig, echoURL(c))
	// If the server supports it, we should log at "/environment/UUID/log"
	environ, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	info := s.APIInfo(c)
	info.EnvironTag = environ.EnvironTag()
	apistate, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer apistate.Close()
	reader, err := apistate.Client().WatchDebugLog(api.DebugLogParams{})
	c.Assert(err, jc.ErrorIsNil)
	connectURL := connectURLFromReader(c, reader)
	c.ExpectFailure("debug log always goes to /log for compatibility http://pad.lv/1326799")
	c.Assert(connectURL.Path, gc.Matches, fmt.Sprintf("/%s/log", environ.UUID()))
}

func (s *clientSuite) TestOpenUsesEnvironUUIDPaths(c *gc.C) {
	info := s.APIInfo(c)
	// Backwards compatibility, passing EnvironTag = "" should just work
	info.EnvironTag = names.NewEnvironTag("")
	apistate, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	apistate.Close()

	// Passing in the correct environment UUID should also work
	environ, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	info.EnvironTag = environ.EnvironTag()
	apistate, err = api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	apistate.Close()

	// Passing in a bad environment UUID should fail with a known error
	info.EnvironTag = names.NewEnvironTag("dead-beef-123456")
	apistate, err = api.Open(info, api.DialOpts{})
	c.Check(err, gc.ErrorMatches, `unknown environment: "dead-beef-123456"`)
	c.Check(err, jc.Satisfies, params.IsCodeNotFound)
	c.Assert(apistate, gc.IsNil)
}

func (s *clientSuite) TestSetEnvironAgentVersionDuringUpgrade(c *gc.C) {
	// This is an integration test which ensure that a test with the
	// correct error code is seen by the client from the
	// SetEnvironAgentVersion call when an upgrade is in progress.
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := envConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageEnviron},
	})
	err = machine.SetAgentVersion(version.MustParseBinary(agentVersion.String() + "-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	nextVersion := version.MustParse("9.8.7")
	_, err = s.State.EnsureUpgradeInfo(machine.Id(), agentVersion, nextVersion)
	c.Assert(err, jc.ErrorIsNil)

	err = s.APIState.Client().SetEnvironAgentVersion(nextVersion)

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
	env, err := client.EnvironmentGet()
	c.Assert(err, jc.ErrorIsNil)
	// Check a known value, just checking that there is something there.
	c.Assert(env["type"], gc.Equals, "dummy")
}

func (s *clientSuite) TestEnvironmentSet(c *gc.C) {
	client := s.APIState.Client()
	err := client.EnvironmentSet(map[string]interface{}{
		"some-name":  "value",
		"other-name": true,
	})
	c.Assert(err, jc.ErrorIsNil)
	// Check them using EnvironmentGet.
	env, err := client.EnvironmentGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env["some-name"], gc.Equals, "value")
	c.Assert(env["other-name"], gc.Equals, true)
}

func (s *clientSuite) TestEnvironmentUnset(c *gc.C) {
	client := s.APIState.Client()
	err := client.EnvironmentSet(map[string]interface{}{
		"some-name": "value",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Now unset it and make sure it isn't there.
	err = client.EnvironmentUnset("some-name")
	c.Assert(err, jc.ErrorIsNil)

	env, err := client.EnvironmentGet()
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

func echoURL(c *gc.C) func(*websocket.Config) (io.ReadCloser, error) {
	response := &params.ErrorResult{}
	message, err := json.Marshal(response)
	c.Assert(err, jc.ErrorIsNil)
	return func(config *websocket.Config) (io.ReadCloser, error) {
		pr, pw := io.Pipe()
		go func() {
			fmt.Fprintf(pw, "%s\n", message)
			fmt.Fprintf(pw, "%s\n", config.Location)
		}()
		return pr, nil
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
