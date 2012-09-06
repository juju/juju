package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"launchpad.net/tomb"
	"net/http"
	"path/filepath"
	"time"
)

var _ = Suite(&upgraderSuite{})

type upgraderSuite struct {
	testing.JujuConnSuite
	oldVarDir string
}

func (s *upgraderSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	s.oldVarDir = environs.VarDir
	environs.VarDir = c.MkDir()
}

func (s *upgraderSuite) TearDownTest(c *C) {
	environs.VarDir = s.oldVarDir
	s.JujuConnSuite.TearDownTest(c)
}

func (s *upgraderSuite) TestUpgraderError(c *C) {
	st, err := state.Open(s.StateInfo(c))
	c.Assert(err, IsNil)
	_, as, upgraderDone := startUpgrader(st)
	// We have no installed tools, so the logic should set the agent
	// tools anyway, but with no URL.
	assertEvent(c, as.event, fmt.Sprintf("SetAgentTools %s ", version.Current))

	// Close the state under the watcher and check that the upgrader dies.
	st.Close()
	select {
	case err := <-upgraderDone:
		c.Assert(err, Not(FitsTypeOf), &UpgradedError{})
		c.Assert(err, NotNil)
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("upgrader did not stop as expected")
	}
}

func (s *upgraderSuite) TestUpgraderStop(c *C) {
	u, as, upgraderDone := startUpgrader(s.State)
	assertEvent(c, as.event, fmt.Sprintf("SetAgentTools %s ", version.Current))

	err := u.Stop()
	c.Assert(err, IsNil)

	select {
	case err := <-upgraderDone:
		c.Assert(err, IsNil)
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("upgrader did not stop as expected")
	}
}

// startUpgrader starts the upgrader using the given machine
// for observing and changing agent tools.
func startUpgrader(st *state.State) (u *Upgrader, as *testAgentState, upgraderDone <-chan error) {
	as = newTestAgentState()
	u = NewUpgrader(st, "testagent", as)
	done := make(chan error, 1)
	go func() {
		done <- u.Wait()
	}()
	upgraderDone = done
	return
}

func (s *upgraderSuite) proposeVersion(c *C, vers version.Number) {
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, IsNil)
	attrs := cfg.AllAttrs()
	attrs["agent-version"] = vers.String()
	newCfg, err := config.New(attrs)
	c.Assert(err, IsNil)
	err = s.State.SetEnvironConfig(newCfg)
	c.Assert(err, IsNil)
}

func (s *upgraderSuite) uploadTools(c *C, vers version.Binary) (path string, tools *state.Tools) {
	tgz := coretesting.TarGz(
		coretesting.NewTarFile("juju", 0777, "juju contents "+vers.String()),
		coretesting.NewTarFile("jujuc", 0777, "jujuc contents "+vers.String()),
		coretesting.NewTarFile("jujud", 0777, "jujud contents "+vers.String()),
	)
	storage := s.Conn.Environ.Storage()
	err := storage.Put(environs.ToolsStoragePath(vers), bytes.NewReader(tgz), int64(len(tgz)))
	c.Assert(err, IsNil)
	path = environs.ToolsStoragePath(vers)
	url, err := s.Conn.Environ.Storage().URL(path)
	c.Assert(err, IsNil)
	return path, &state.Tools{URL: url, Binary: vers}
}

func (s *upgraderSuite) TestUpgrader(c *C) {

	// Set up the current version and tools.
	version.Current = version.MustParseBinary("1.0.1-foo-bar")
	v1path, v1tools := s.uploadTools(c, version.Current)

	// Unpack the "current" version of the tools, and delete them from
	// the storage so that we're sure that the uploader isn't trying
	// to fetch them.
	resp, err := http.Get(v1tools.URL)
	c.Assert(err, IsNil)
	err = environs.UnpackTools(v1tools, resp.Body)
	c.Assert(err, IsNil)
	err = s.Conn.Environ.Storage().Remove(v1path)
	c.Assert(err, IsNil)

	// Start the upgrader going and check that the tools are those
	// that we set up.
	_, as, upgraderDone := startUpgrader(s.State)
	assertEvent(c, as.event, "SetAgentTools 1.0.1-foo-bar "+v1tools.URL)

	// Propose some tools that are not there.
	s.proposeVersion(c, version.MustParse("1.0.2"))
	assertNothingHappens(c, upgraderDone)

	// Upload the current tools again.
	v1path, v1tools = s.uploadTools(c, version.Current)
	s.proposeVersion(c, version.MustParse("1.0.3"))
	assertNothingHappens(c, upgraderDone)

	// Upload two new versions of the tools. We'll test upgrading to these tools.
	_, v5tools := s.uploadTools(c, version.MustParseBinary("1.0.5-foo-bar"))
	_, v6tools := s.uploadTools(c, version.MustParseBinary("1.0.6-foo-bar"))

	// Check that it won't choose tools with a greater version number.
	s.proposeVersion(c, version.MustParse("1.0.4"))
	assertNothingHappens(c, upgraderDone)

	s.proposeVersion(c, v6tools.Number)
	select {
	case err := <-upgraderDone:
		c.Assert(err, DeepEquals, &UpgradedError{v6tools})
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("upgrader did not stop as expected")
	}

	// Check that the upgraded version was really downloaded.
	data, err := ioutil.ReadFile(filepath.Join(environs.ToolsDir(v6tools.Binary), "jujud"))
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, "jujud contents 1.0.6-foo-bar")

	version.Current = v6tools.Binary
	// Check that we can start again.
	_, as, upgraderDone = startUpgrader(s.State)
	assertEvent(c, as.event, "SetAgentTools 1.0.6-foo-bar "+v6tools.URL)

	// Check that we can downgrade.
	s.proposeVersion(c, v5tools.Number)

	select {
	case tools := <-upgraderDone:
		c.Assert(tools, DeepEquals, &UpgradedError{v5tools})
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("upgrader did not stop as expected")
	}
}

func assertNothingHappens(c *C, upgraderDone <-chan error) {
	select {
	case got := <-upgraderDone:
		c.Fatalf("expected nothing to happen, got %v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func assertEvent(c *C, event <-chan string, want string) {
	select {
	case got := <-event:
		c.Assert(got, Equals, want)
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("no event received; expected %q", want)
	}
}

type testAgentState struct {
	tomb.Tomb
	event chan string
}

func newTestAgentState() *testAgentState {
	return &testAgentState{
		event: make(chan string),
	}
}

func (t *testAgentState) SetAgentTools(tools *state.Tools) error {
	t.event <- fmt.Sprintf("SetAgentTools %v %s", tools.Binary, tools.URL)
	return nil
}
