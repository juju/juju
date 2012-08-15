package main
import (
	"launchpad.net/tomb"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
	"time"
)

var _ = Suite(&upgraderSuite{})

type upgraderSuite struct {
	coretesting.LoggingSuite
	testing.JujuConnSuite
	oldVarDir string
	// We use a Machine as a stand-in for anything that
	// we get get a ProposedAgentToolsWatcher from.
	m *state.Machine
}

func (s *upgraderSuite) TestUpgrader(c *C) {
	// Set up the current version and tools.
	v1 := &state.Tools{
		URL:    "http://oldurl.tgz",
		Binary: version.MustParseBinary("1.0.1-foo-bar"),
	}
	version.Current = v1.Binary
	// unpack the "current" version of the tools.
	v1tools := testing.Archive(
		testing.NewFile("juju", 0777, "juju contents v1"),
		testing.NewFile("jujuc", 0777, "jujuc contents v1"),
		testing.NewFile("jujud", 0777, "jujud contents v1"),
	)
	err := environs.UnpackTools(v1, bytes.NewReader(v1tools))
	c.Assert(err, IsNil)

	// Upload a new version of the tools to the environ's storage.
	// We'll test upgrading to these tools.
	v2tools := testing.Archive(
		testing.NewFile("juju", 0777, "juju contents v2"),
		testing.NewFile("jujuc", 0777, "jujuc contents v2"),
		testing.NewFile("jujud", 0777, "jujud contents v2"),
	)
	v2 := &state.Tools{
		Binary: version.MustParseBinary("1.0.2-foo-bar"),
	}
	err = s.environ.Storage().Put(environs.ToolsPath(v2.Binary), bytes.NewReader(v2tools), int64(len(v2tools)))
	c.Assert(err, IsNil)
	v2.URL, err = s.environ.Storage().URL(environs.ToolsPath(v2.Binary))
	c.Assert(err, IsNil)

	m, err := s.State.AddMachine()
	c.Assert(err, IsNil)

	as := newTestAgentState(m)
	upgraderDone := make(chan error, 1)
	go func() {
		upgraderDone <- NewUpgrader("testagent", as)
	}()
	assertEvent(c, as.event, "SetAgentTools 1.0.1-foo-bar http://oldurl.tgz")
	assertEvent(c, as.event, "WatchProposedTools")

	// Propose some tools but delay the fetching.
	delayedURL, start := delayedFetch()
	s.proposeTools(&state.Tools{
		URL:    delayedURL,
		Binary: v2.Binary,
	})
	<-start

	start <- true
	assertNoEvent(c, as.event)

	s.proposeTools(v2)
	assertNoEvent(c, as.event)

	select {
	case err := <-upgraderDone:
		c.Assert(tools, DeepEquals, &UpgraderError{v2})
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("upgrader did not stop as expected")
	}

	// Check that the upgraded version was really downloaded.
	data, err := ioutil.ReadFile(filepath.Join(environs.AgentToolsDir("testagent"), "jujud"))
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, "jujud contents v2")

	as = newTestAgentState(m)
	upgraderDone = make(chan error, 1)
	go func() {
		upgraderDone <- NewUpgrader("testagent", as)
	}()

	assertEvent(c, as.event, "SetAgentTools 1.0.1-foo-bar http://oldurl.tgz")
	assertEvent(c, as.event, "WatchProposedTools")

	// Use delayedURL but don't make it respond - if the upgrade
	// succeeds then we know that it has (correctly) not tried to
	// fetch the URL
	s.proposeTools(&state.Tools{
		URL:    delayedURL,
		Binary: v2.Binary,
	})
	assertNoEvent(c, r.event)
	select {
	case tools := <-runnerDone:
		c.Assert(tools, DeepEquals, &UpgraderError{v2})
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("upgrader did not stop as expected")
	}
}

type testAgentState struct {
	tomb.Tomb
	event chan string
	m *state.Machine
}

func newTestAgentState(m *state.Machine) *testAgentState {
	return &testAgentState{
		event: make(chan string),
		m: m,
	}
}

func (t *testAgentState) SetAgentTools(tools *state.Tools) error {
	// TODO(rog) add logic to make this return an error
	r.event <- fmt.Sprintf("SetAgentTools %v %s", tools.Binary, tools.URL)
	if err := t.m.SetAgentTools(tools); err != nil {
		panic(err)
	}
}

func (t *testAgentState) WatchProposedTools(tools *state.Tools) *state.AgentToolsWatcher {
	r.event <- "WatchProposedTools"
	return t.m.WatchProposedAgentTools()
}

func (t *testAgentState) proposeTools(tools *state.Tools) {
	err := t.m.ProposeAgentTools(tools)
	if err != nil {
		panic(err)
	}
}
