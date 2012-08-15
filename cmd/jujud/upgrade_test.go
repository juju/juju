package main

var _ = Suite(&upgraderSuite{})

type upgraderSuite struct {
	testing.LoggingSuite
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

	upgraderDone := make(chan error, 1)
	go func() {
		upgraderDone <- NewUpgrader("testagent", newTestAgentState(m))
	}()
	c.Assert(recvTimeout(r.event

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
