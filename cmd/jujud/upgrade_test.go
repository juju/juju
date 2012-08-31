package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"launchpad.net/tomb"
	"net"
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
	m, err := st.AddMachine()
	c.Assert(err, IsNil)
	_, as, upgraderDone := startUpgrader(m)
	// We have no installed tools, so the logic should set the agent
	// tools anyway, but with no URL.
	assertEvent(c, as.event, fmt.Sprintf("SetAgentTools %s ", version.Current))
	assertEvent(c, as.event, "WatchProposedTools")

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
	m, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	u, as, upgraderDone := startUpgrader(m)
	assertEvent(c, as.event, fmt.Sprintf("SetAgentTools %s ", version.Current))
	assertEvent(c, as.event, "WatchProposedTools")

	err = u.Stop()
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
func startUpgrader(m *state.Machine) (u *Upgrader, as *testAgentState, upgraderDone <-chan error) {
	as = newTestAgentState(m)
	u = NewUpgrader("testagent", as)
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

func (s *upgraderSuite) TestUpgrader(c *C) {
	// Set up the current version and tools.
	v1 := &state.Tools{
		URL:    "http://oldurl.tgz",
		Binary: version.MustParseBinary("1.0.1-foo-bar"),
	}
	version.Current = v1.Binary
	// unpack the "current" version of the tools.
	v1tools := coretesting.TarGz(
		coretesting.NewTarFile("juju", 0777, "juju contents v1"),
		coretesting.NewTarFile("jujuc", 0777, "jujuc contents v1"),
		coretesting.NewTarFile("jujud", 0777, "jujud contents v1"),
	)
	err := environs.UnpackTools(v1, bytes.NewReader(v1tools))
	c.Assert(err, IsNil)

	// Upload a new version of the tools to the environ's storage.
	// We'll test upgrading to these tools.
	v2tools := coretesting.TarGz(
		coretesting.NewTarFile("juju", 0777, "juju contents v2"),
		coretesting.NewTarFile("jujuc", 0777, "jujuc contents v2"),
		coretesting.NewTarFile("jujud", 0777, "jujud contents v2"),
	)
	v2 := &state.Tools{
		Binary: version.MustParseBinary("1.0.2-foo-bar"),
	}
	storage := s.Conn.Environ.Storage()
	err = storage.Put(environs.ToolsStoragePath(v2.Binary), bytes.NewReader(v2tools), int64(len(v2tools)))
	c.Assert(err, IsNil)
	v2.URL, err = s.Conn.Environ.Storage().URL(environs.ToolsStoragePath(v2.Binary))
	c.Assert(err, IsNil)

	m, err := s.State.AddMachine()
	c.Assert(err, IsNil)

	// Start the upgrader going and check that the tools are those
	// that we set up.
	_, as, upgraderDone := startUpgrader(m)
	assertEvent(c, as.event, "SetAgentTools 1.0.1-foo-bar http://oldurl.tgz")

	// Propose some invalid tools then check that
	// the URL is fetched and that nothing happens.
	delayedURL, started := delayedFetch()
	s.proposeVersion(c, v2.Binary.Number)
	<-started

	s.proposeVersion(c, v2.Binary.Number)
	assertNoEvent(c, as.event)

	select {
	case err := <-upgraderDone:
		c.Assert(err, DeepEquals, &UpgradedError{v2})
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("upgrader did not stop as expected")
	}

	// Check that the upgraded version was really downloaded.
	data, err := ioutil.ReadFile(filepath.Join(environs.ToolsDir(v2.Binary), "jujud"))
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, "jujud contents v2")

	_, as, upgraderDone = startUpgrader(m)
	assertEvent(c, as.event, "SetAgentTools 1.0.1-foo-bar http://oldurl.tgz")
	assertEvent(c, as.event, "WatchProposedTools")

	// Use delayedURL but don't make it respond - if the upgrade
	// succeeds then we know that it has (correctly) not tried to
	// fetch the URL
	as.proposeTools(&state.Tools{
		URL:    delayedURL,
		Binary: v2.Binary,
	})
	assertNoEvent(c, as.event)
	select {
	case tools := <-upgraderDone:
		c.Assert(tools, DeepEquals, &UpgradedError{v2})
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("upgrader did not stop as expected")
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

func assertNoEvent(c *C, event <-chan string) {
	select {
	case got := <-event:
		c.Fatalf("expected no event; got %q", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func delayedFetch() (url string, started chan bool) {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		panic(err)
	}
	started = make(chan bool)
	go http.Serve(l, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- true
		http.NotFound(w, r)
		r.Body.Close()
	}))
	return fmt.Sprintf("http://%s/delayed", l.Addr()), started
}

type testAgentState struct {
	tomb.Tomb
	event chan string
	m     *state.Machine
}

func newTestAgentState(m *state.Machine) *testAgentState {
	return &testAgentState{
		event: make(chan string),
		m:     m,
	}
}

func (t *testAgentState) SetAgentTools(tools *state.Tools) error {
	t.event <- fmt.Sprintf("SetAgentTools %v %s", tools.Binary, tools.URL)
	return t.m.SetAgentTools(tools)
}

func (t *testAgentState) proposeTools(tools *state.Tools) {
	err := t.m.ProposeAgentTools(tools)
	if err != nil {
		panic(err)
	}
}
