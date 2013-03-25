package main

import (
	"bytes"
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/agent"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/worker"
	"launchpad.net/tomb"
	"net/http"
	"time"
)

var _ = Suite(&toolSuite{})

type toolSuite struct {
	coretesting.LoggingSuite
}

func assertDead(c *C, tasks []*testTask) {
	for _, t := range tasks {
		c.Assert(t.Dead(), Equals, true)
	}
}

func (*toolSuite) TestRunTasksAllSuccess(c *C) {
	tasks := newTestTasks(4)
	for _, t := range tasks {
		t.kill <- nil
	}
	err := runTasks(make(chan struct{}), taskSlice(tasks)...)
	c.Assert(err, IsNil)
	assertDead(c, tasks)
}

func (*toolSuite) TestOneTaskError(c *C) {
	tasks := newTestTasks(4)
	for i, t := range tasks {
		if i == 2 {
			t.kill <- fmt.Errorf("kill")
		}
	}
	err := runTasks(make(chan struct{}), taskSlice(tasks)...)
	c.Assert(err, ErrorMatches, "kill")
	assertDead(c, tasks)
}

func (*toolSuite) TestTaskStop(c *C) {
	tasks := newTestTasks(4)
	tasks[2].stopErr = fmt.Errorf("stop")
	stop := make(chan struct{})
	close(stop)
	err := runTasks(stop, taskSlice(tasks)...)
	c.Assert(err, ErrorMatches, "stop")
	assertDead(c, tasks)
}

func (*toolSuite) TestUpgradeGetsPrecedence(c *C) {
	tasks := newTestTasks(2)
	tasks[1].stopErr = &UpgradeReadyError{}
	go func() {
		time.Sleep(10 * time.Millisecond)
		tasks[0].kill <- fmt.Errorf("stop")
	}()
	err := runTasks(nil, taskSlice(tasks)...)
	c.Assert(err, Equals, tasks[1].stopErr)
	assertDead(c, tasks)
}

func (*toolSuite) TestDeadGetsPrecedence(c *C) {
	tasks := newTestTasks(3)
	tasks[1].stopErr = &UpgradeReadyError{}
	tasks[2].stopErr = worker.ErrDead
	go func() {
		time.Sleep(10 * time.Millisecond)
		tasks[0].kill <- fmt.Errorf("stop")
	}()
	err := runTasks(nil, taskSlice(tasks)...)
	c.Assert(err, Equals, tasks[2].stopErr)
	assertDead(c, tasks)
}

func mkTools(s string) *state.Tools {
	return &state.Tools{
		Binary: version.MustParseBinary(s + "-foo-bar"),
	}
}

func (*toolSuite) TestUpgradeErrorLog(c *C) {
	tasks := newTestTasks(7)
	tasks[0].stopErr = fmt.Errorf("zero")
	tasks[1].stopErr = fmt.Errorf("one")
	tasks[2].stopErr = &UpgradeReadyError{NewTools: mkTools("1.1.1")}
	tasks[3].kill <- fmt.Errorf("three")
	tasks[4].stopErr = fmt.Errorf("four")
	tasks[5].stopErr = &UpgradeReadyError{NewTools: mkTools("2.2.2")}
	tasks[6].stopErr = fmt.Errorf("six")

	expectLog := `
(.|\n)*task3: three
.*task0: zero
.*task1: one
.*task2: must restart: an agent upgrade is available
.*task4: four
.*task5: must restart: an agent upgrade is available
.*task6: six
(.|\n)*`[1:]

	err := runTasks(nil, taskSlice(tasks)...)
	c.Assert(err, Equals, tasks[2].stopErr)
	c.Assert(c.GetTestLog(), Matches, expectLog)
}

type testTask struct {
	name string
	tomb.Tomb
	kill    chan error
	stopErr error
}

func (t *testTask) Stop() error {
	t.Kill(nil)
	return t.Wait()
}

func (t *testTask) Dead() bool {
	select {
	case <-t.Tomb.Dead():
		return true
	default:
	}
	return false
}

func (t *testTask) run() {
	defer t.Done()
	select {
	case <-t.Dying():
		t.Kill(t.stopErr)
	case err := <-t.kill:
		t.Kill(err)
		return
	}
}

func (t *testTask) String() string {
	return t.name
}

func newTestTasks(n int) []*testTask {
	tasks := make([]*testTask, n)
	for i := range tasks {
		tasks[i] = &testTask{
			kill: make(chan error, 1),
			name: fmt.Sprintf("task%d", i),
		}
		go tasks[i].run()
	}
	return tasks
}

func taskSlice(tasks []*testTask) []task {
	r := make([]task, len(tasks))
	for i, t := range tasks {
		r[i] = t
	}
	return r
}

type acCreator func() (cmd.Command, *AgentConf)

// CheckAgentCommand is a utility function for verifying that common agent
// options are handled by a Command; it returns an instance of that
// command pre-parsed, with any mandatory flags added.
func CheckAgentCommand(c *C, create acCreator, args []string) cmd.Command {
	com, conf := create()
	err := coretesting.InitCommand(com, args)
	c.Assert(conf.dataDir, Equals, "/var/lib/juju")
	badArgs := append(args, "--data-dir", "")
	com, conf = create()
	err = coretesting.InitCommand(com, badArgs)
	c.Assert(err, ErrorMatches, "--data-dir option must be set")

	args = append(args, "--data-dir", "jd")
	com, conf = create()
	c.Assert(coretesting.InitCommand(com, args), IsNil)
	c.Assert(conf.dataDir, Equals, "jd")
	return com
}

// ParseAgentCommand is a utility function that inserts the always-required args
// before parsing an agent command and returning the result.
func ParseAgentCommand(ac cmd.Command, args []string) error {
	common := []string{
		"--data-dir", "jd",
	}
	return coretesting.InitCommand(ac, append(common, args...))
}

type runner interface {
	Run(*cmd.Context) error
	Stop() error
}

// runWithTimeout runs an agent and waits
// for it to complete within a reasonable time.
func runWithTimeout(r runner) error {
	done := make(chan error)
	go func() {
		done <- r.Run(nil)
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
	}
	err := r.Stop()
	return fmt.Errorf("timed out waiting for agent to finish; stop error: %v", err)
}

// runStop runs an agent, immediately stops it,
// and returns the resulting error status.
func runStop(r runner) error {
	done := make(chan error, 1)
	go func() {
		done <- r.Run(nil)
	}()
	go func() {
		done <- r.Stop()
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
	}
	return fmt.Errorf("timed out waiting for agent to finish")
}

type entity interface {
	state.Tagger
	SetMongoPassword(string) error
}

// agentSuite is a fixture to be used by agent test suites.
type agentSuite struct {
	testing.JujuConnSuite
}

// primeAgent writes the configuration file and tools
// for an agent with the given entity name.
// It returns the agent's configuration and the current tools.
func (s *agentSuite) primeAgent(c *C, entityName, password string) (*agent.Conf, *state.Tools) {
	tools := s.primeTools(c, version.Current)
	tools1, err := agent.ChangeAgentTools(s.DataDir(), entityName, version.Current)
	c.Assert(err, IsNil)
	c.Assert(tools1, DeepEquals, tools)

	conf := &agent.Conf{
		DataDir:     s.DataDir(),
		OldPassword: password,
		StateInfo:   s.StateInfo(c),
	}
	conf.StateInfo.EntityName = entityName
	err = conf.Write()
	c.Assert(err, IsNil)
	return conf, tools
}

// initAgent initialises the given agent command with additional
// arguments as provided.
func (s *agentSuite) initAgent(c *C, a cmd.Command, args ...string) {
	args = append([]string{"--data-dir", s.DataDir()}, args...)
	err := coretesting.InitCommand(a, args)
	c.Assert(err, IsNil)
}

func (s *agentSuite) proposeVersion(c *C, vers version.Number, development bool) {
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, IsNil)
	attrs := cfg.AllAttrs()
	attrs["agent-version"] = vers.String()
	attrs["development"] = development
	newCfg, err := config.New(attrs)
	c.Assert(err, IsNil)
	err = s.State.SetEnvironConfig(newCfg)
	c.Assert(err, IsNil)
}

func (s *agentSuite) uploadTools(c *C, vers version.Binary) *state.Tools {
	tgz := coretesting.TarGz(
		coretesting.NewTarFile("juju", 0777, "juju contents "+vers.String()),
		coretesting.NewTarFile("jujud", 0777, "jujud contents "+vers.String()),
	)
	storage := s.Conn.Environ.Storage()
	err := storage.Put(environs.ToolsStoragePath(vers), bytes.NewReader(tgz), int64(len(tgz)))
	c.Assert(err, IsNil)
	url, err := s.Conn.Environ.Storage().URL(environs.ToolsStoragePath(vers))
	c.Assert(err, IsNil)
	return &state.Tools{URL: url, Binary: vers}
}

// primeTools sets up the current version of the tools to vers and
// makes sure that they're available JujuConnSuite's DataDir.
func (s *agentSuite) primeTools(c *C, vers version.Binary) *state.Tools {
	// Set up the current version and tools.
	version.Current = vers
	tools := s.uploadTools(c, vers)
	resp, err := http.Get(tools.URL)
	c.Assert(err, IsNil)
	defer resp.Body.Close()
	err = agent.UnpackTools(s.DataDir(), tools, resp.Body)
	c.Assert(err, IsNil)
	return tools
}

func (s *agentSuite) testAgentPasswordChanging(c *C, ent entity, newAgent func() runner) {
	conf, err := agent.ReadConf(s.DataDir(), ent.Tag())
	c.Assert(err, IsNil)

	// Check that it starts initially and changes the password
	err = ent.SetMongoPassword("initial")
	c.Assert(err, IsNil)

	setOldPassword := func(password string) {
		conf.OldPassword = password
		err = conf.Write()
		c.Assert(err, IsNil)
	}

	setOldPassword("initial")
	err = runStop(newAgent())
	c.Assert(err, IsNil)

	// Check that we can no longer gain access with the initial password.
	info := s.StateInfo(c)
	info.EntityName = ent.Tag()
	info.Password = "initial"
	testOpenState(c, info, state.Unauthorizedf("unauth"))

	// Read the configuration and check that we can connect with it.
	c.Assert(refreshConfig(conf), IsNil)
	newPassword := conf.StateInfo.Password

	testOpenState(c, conf.StateInfo, nil)

	// Check that it starts again ok
	err = runStop(newAgent())
	c.Assert(err, IsNil)

	// Change the password in the configuration and check
	// that it falls back to using the initial password
	c.Assert(refreshConfig(conf), IsNil)
	conf.StateInfo.Password = "spurious"
	conf.OldPassword = newPassword
	c.Assert(conf.Write(), IsNil)

	err = runStop(newAgent())
	c.Assert(err, IsNil)

	// Check that it's changed the password again
	c.Assert(refreshConfig(conf), IsNil)
	c.Assert(conf.StateInfo.Password, Not(Equals), "spurious")
	c.Assert(conf.StateInfo.Password, Not(Equals), newPassword)

	info.Password = conf.StateInfo.Password
	testOpenState(c, info, nil)
}

func (s *agentSuite) testUpgrade(c *C, agent runner, currentTools *state.Tools) {
	newVers := version.Current
	newVers.Patch++
	newTools := s.uploadTools(c, newVers)
	s.proposeVersion(c, newVers.Number, true)
	err := runWithTimeout(agent)
	c.Assert(err, FitsTypeOf, &UpgradeReadyError{})
	ug := err.(*UpgradeReadyError)
	c.Assert(ug.NewTools, DeepEquals, newTools)
	c.Assert(ug.OldTools, DeepEquals, currentTools)
}

func refreshConfig(c *agent.Conf) error {
	nc, err := agent.ReadConf(c.DataDir, c.StateInfo.EntityName)
	if err != nil {
		return err
	}
	*c = *nc
	return nil
}
