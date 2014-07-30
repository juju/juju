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
	"github.com/juju/charm"
	charmtesting "github.com/juju/charm/testing"
	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/constraints"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/api"
	"github.com/juju/juju/state/api/params"
)

type clientSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&clientSuite{})

// TODO(jam) 2013-08-27 http://pad.lv/1217282
// Right now most of the direct tests for api.Client behavior are in
// state/apiserver/client/*_test.go

func (s *clientSuite) TestCloseMultipleOk(c *gc.C) {
	client := s.APIState.Client()
	c.Assert(client.Close(), gc.IsNil)
	c.Assert(client.Close(), gc.IsNil)
	c.Assert(client.Close(), gc.IsNil)
}

func (s *clientSuite) TestAddLocalCharm(c *gc.C) {
	charmArchive := charmtesting.Charms.Bundle(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	client := s.APIState.Client()

	// Test the sanity checks first.
	_, err := client.AddLocalCharm(charm.MustParseURL("cs:quantal/wordpress-1"), nil)
	c.Assert(err, gc.ErrorMatches, `expected charm URL with local: schema, got "cs:quantal/wordpress-1"`)

	// Upload an archive with its original revision.
	savedURL, err := client.AddLocalCharm(curl, charmArchive)
	c.Assert(err, gc.IsNil)
	c.Assert(savedURL.String(), gc.Equals, curl.String())

	// Upload a charm directory with changed revision.
	charmDir := charmtesting.Charms.ClonedDir(c.MkDir(), "dummy")
	charmDir.SetDiskRevision(42)
	savedURL, err = client.AddLocalCharm(curl, charmDir)
	c.Assert(err, gc.IsNil)
	c.Assert(savedURL.Revision, gc.Equals, 42)

	// Upload a charm directory again, revision should be bumped.
	savedURL, err = client.AddLocalCharm(curl, charmDir)
	c.Assert(err, gc.IsNil)
	c.Assert(savedURL.String(), gc.Equals, curl.WithRevision(43).String())

	// Finally, try the NotImplementedError by mocking the server
	// address to a handler that returns 405 Method Not Allowed for
	// POST.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, gc.IsNil)
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

	api.SetServerRoot(client, url)
	_, err = client.AddLocalCharm(curl, charmArchive)
	c.Assert(err, jc.Satisfies, params.IsCodeNotImplemented)
}

func (s *clientSuite) TestClientEnvironmentUUID(c *gc.C) {
	environ, err := s.State.Environment()
	c.Assert(err, gc.IsNil)

	client := s.APIState.Client()
	c.Assert(client.EnvironmentUUID(), gc.Equals, environ.Tag().Id())
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
	c.Assert(err, gc.IsNil)

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
	info.EnvironTag = nil
	apistate, err := api.Open(info, api.DialOpts{})
	c.Assert(err, gc.IsNil)
	defer apistate.Close()
	reader, err := apistate.Client().WatchDebugLog(api.DebugLogParams{})
	c.Assert(err, gc.IsNil)
	connectURL := connectURLFromReader(c, reader)
	c.Assert(connectURL.Path, gc.Matches, "/log")
}

func (s *clientSuite) TestDebugLogAtUUIDLogPath(c *gc.C) {
	s.PatchValue(api.WebsocketDialConfig, echoURL(c))
	// If the server supports it, we should log at "/environment/UUID/log"
	environ, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	info := s.APIInfo(c)
	info.EnvironTag = environ.Tag()
	apistate, err := api.Open(info, api.DialOpts{})
	c.Assert(err, gc.IsNil)
	defer apistate.Close()
	reader, err := apistate.Client().WatchDebugLog(api.DebugLogParams{})
	c.Assert(err, gc.IsNil)
	connectURL := connectURLFromReader(c, reader)
	c.ExpectFailure("debug log always goes to /log for compatibility http://pad.lv/1326799")
	c.Assert(connectURL.Path, gc.Matches, fmt.Sprintf("/%s/log", environ.UUID()))
}

func (s *clientSuite) TestOpenUsesEnvironUUIDPaths(c *gc.C) {
	info := s.APIInfo(c)
	// Backwards compatibility, passing EnvironTag = "" should just work
	info.EnvironTag = nil
	apistate, err := api.Open(info, api.DialOpts{})
	c.Assert(err, gc.IsNil)
	apistate.Close()

	// Passing in the correct environment UUID should also work
	environ, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	info.EnvironTag = environ.Tag()
	apistate, err = api.Open(info, api.DialOpts{})
	c.Assert(err, gc.IsNil)
	apistate.Close()

	// Passing in a bad environment UUID should fail with a known error
	info.EnvironTag = names.NewEnvironTag("dead-beef-123456")
	apistate, err = api.Open(info, api.DialOpts{})
	c.Check(err, gc.ErrorMatches, `unknown environment: "dead-beef-123456"`)
	c.Check(err, jc.Satisfies, params.IsCodeNotFound)
	c.Assert(apistate, gc.IsNil)
}

func (s *clientSuite) TestClientEnsureAvailabilityFailsBadEnvTag(c *gc.C) {
	defer api.PatchEnvironTag(s.APIState, "bad-env-uuid")()
	emptyCons := constraints.Value{}
	defaultSeries := ""
	_, err := s.APIState.Client().EnsureAvailability(3, emptyCons, defaultSeries)
	c.Assert(err, gc.ErrorMatches,
		`invalid environment tag: "bad-env-uuid" is not a valid tag`)
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
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)
	connectURL, err := url.Parse(strings.TrimSpace(location))
	c.Assert(err, gc.IsNil)
	rc.Close()
	return connectURL
}
