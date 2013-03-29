package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/presence"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"net/url"
	"sync"
	"time"
)

func runStatus(c *C, args ...string) (code int, stderr, stdout []byte) {
	ctx := coretesting.Context(c)
	code = cmd.Main(&StatusCommand{}, ctx, args)
	stderr = ctx.Stderr.(*bytes.Buffer).Bytes()
	stdout = ctx.Stdout.(*bytes.Buffer).Bytes()
	return
}

type StatusSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&StatusSuite{})

type M map[string]interface{}

type testCase struct {
	summary string
	steps   []stepper
}

func test(summary string, steps ...stepper) testCase {
	return testCase{summary, steps}
}

type stepper interface {
	step(c *C, ctx *context)
}

type context struct {
	st          *state.State
	conn        *juju.Conn
	charms      map[string]*state.Charm
	unitPingers map[string]*presence.Pinger
}

func (s *StatusSuite) newContext() *context {
	return &context{
		st:          s.State,
		conn:        s.Conn,
		charms:      make(map[string]*state.Charm),
		unitPingers: make(map[string]*presence.Pinger),
	}
}

func (s *StatusSuite) resetContext(c *C, ctx *context) {
	for _, up := range ctx.unitPingers {
		err := up.Kill()
		c.Check(err, IsNil)
	}
	s.JujuConnSuite.Reset(c)
}

func (ctx *context) run(c *C, steps []stepper) {
	for i, s := range steps {
		c.Logf("step %d", i)
		c.Logf("%#v", s)
		s.step(c, ctx)
	}
}

type outputFormat struct {
	name      string
	marshal   func(v interface{}) ([]byte, error)
	unmarshal func(data []byte, v interface{}) error
}

// statusFormats list all output formats supported by status command.
var statusFormats = []outputFormat{
	{"yaml", goyaml.Marshal, goyaml.Unmarshal},
	{"json", json.Marshal, json.Unmarshal},
}

var statusTests = []testCase{
	test(
		// unlikely, as you can't run juju status in real life without
		// machine/0 bootstrapped.
		"empty state",
		expect{M{
			"machines": M{},
			"services": M{},
		}},
	), test(
		"simulate juju bootstrap by adding machine/0 to the state",
		addMachine{"0", state.JobManageEnviron},
		expect{M{
			"machines": M{
				"0": M{
					"instance-id": "pending",
				},
			},
			"services": M{},
		}},
	), test(
		"simulate the PA starting an instance in response to the state change",
		addAndStartMachine{"0", state.JobManageEnviron},
		expect{M{
			"machines": M{
				"0": M{
					"dns-name":    "dummyenv-0.dns",
					"instance-id": "dummyenv-0",
				},
			},
			"services": M{},
		}},
	), test(
		"simulate the MA setting the version",
		addAndStartMachine{"0", state.JobManageEnviron},
		setTools{"0", &state.Tools{
			Binary: version.Binary{
				Number: version.MustParse("1.2.3"),
				Series: "gutsy",
				Arch:   "ppc",
			},
			URL: "http://canonical.com/",
		}},
		expect{M{
			"machines": M{
				"0": M{
					"dns-name":      "dummyenv-0.dns",
					"instance-id":   "dummyenv-0",
					"agent-version": "1.2.3",
				},
			},
			"services": M{},
		}},
	), test(
		"add two services and expose one",
		addAndStartMachine{"0", state.JobManageEnviron},
		addCharm{"dummy"},
		addServiceSetExposed{"dummy-service", "dummy", false},
		addServiceSetExposed{"exposed-service", "dummy", true},
		expect{M{
			"machines": M{
				"0": M{
					"dns-name":    "dummyenv-0.dns",
					"instance-id": "dummyenv-0",
				},
			},
			"services": M{
				"dummy-service": M{
					"charm":   "local:series/dummy-1",
					"exposed": false,
				},
				"exposed-service": M{
					"charm":   "local:series/dummy-1",
					"exposed": true,
				},
			},
		}},
	), test(
		"add three machines, two for units; also two services, one exposed",
		setupMachinesAndServices{},
		expect{M{
			"machines": M{
				"0": M{
					"dns-name":      "dummyenv-0.dns",
					"instance-id":   "dummyenv-0",
					"agent-version": "1.2.3",
				},
				"1": M{
					"dns-name":    "dummyenv-1.dns",
					"instance-id": "dummyenv-1",
				},
				"2": M{
					"dns-name":    "dummyenv-2.dns",
					"instance-id": "dummyenv-2",
				},
			},
			"services": M{
				"dummy-service": M{
					"charm":   "local:series/dummy-1",
					"exposed": false,
				},
				"exposed-service": M{
					"charm":   "local:series/dummy-1",
					"exposed": true,
				},
			},
		}},
	), test(
		"same scenario as above; add units for services, set status for both (one is down)",
		setupMachinesAndServices{},
		addUnit{"dummy-service", "1"},
		addAliveUnit{"exposed-service", "2"},
		setUnitStatus{"exposed-service/0", state.UnitError, "You Require More Vespene Gas"},
		// This will be ignored, because the unit is down.
		setUnitStatus{"dummy-service/0", state.UnitStarted, ""},
		expect{M{
			"machines": M{
				"0": M{
					"dns-name":      "dummyenv-0.dns",
					"instance-id":   "dummyenv-0",
					"agent-version": "1.2.3",
				},
				"1": M{
					"dns-name":    "dummyenv-1.dns",
					"instance-id": "dummyenv-1",
				},
				"2": M{
					"dns-name":    "dummyenv-2.dns",
					"instance-id": "dummyenv-2",
				},
			},
			"services": M{
				"exposed-service": M{
					"charm":   "local:series/dummy-1",
					"exposed": true,
					"units": M{
						"exposed-service/0": M{
							"machine":          "2",
							"agent-state":      "error",
							"agent-state-info": "You Require More Vespene Gas",
						},
					},
				},
				"dummy-service": M{
					"charm":   "local:series/dummy-1",
					"exposed": false,
					"units": M{
						"dummy-service/0": M{
							"machine":     "1",
							"agent-state": "down",
						},
					},
				},
			},
		}},
	),
}

// TODO(dfc) test failing components by destructively mutating the state under the hood

type addMachine struct {
	machineId string
	job       state.MachineJob
}

func (am addMachine) step(c *C, ctx *context) {
	m, err := ctx.st.AddMachine("series", am.job)
	c.Assert(err, IsNil)
	c.Assert(m.Id(), Equals, am.machineId)
}

type addAndStartMachine struct {
	machineId string
	job       state.MachineJob
}

func (asm addAndStartMachine) step(c *C, ctx *context) {
	am := &addMachine{asm.machineId, asm.job}
	am.step(c, ctx)
	m, err := ctx.st.Machine(asm.machineId)
	c.Assert(err, IsNil)
	inst := testing.StartInstance(c, ctx.conn.Environ, m.Id())
	err = m.SetInstanceId(inst.Id())
	c.Assert(err, IsNil)
}

type setTools struct {
	machineId string
	tools     *state.Tools
}

func (st setTools) step(c *C, ctx *context) {
	m, err := ctx.st.Machine(st.machineId)
	c.Assert(err, IsNil)
	err = m.SetAgentTools(st.tools)
	c.Assert(err, IsNil)
}

type addCharm struct {
	name string
}

func (ac addCharm) step(c *C, ctx *context) {
	ch := coretesting.Charms.Dir(ac.name)
	name, rev := ch.Meta().Name, ch.Revision()
	curl := charm.MustParseURL(fmt.Sprintf("local:series/%s-%d", name, rev))
	bundleURL, err := url.Parse(fmt.Sprintf("http://bundles.example.com/%s-%d", name, rev))
	c.Assert(err, IsNil)
	dummy, err := ctx.st.AddCharm(ch, curl, bundleURL, fmt.Sprintf("%s-%d-sha256", name, rev))
	c.Assert(err, IsNil)
	ctx.charms[ac.name] = dummy
}

type addServiceSetExposed struct {
	name    string
	charm   string
	exposed bool
}

func (asse addServiceSetExposed) step(c *C, ctx *context) {
	ch, ok := ctx.charms[asse.charm]
	c.Assert(ok, Equals, true)
	s, err := ctx.st.AddService(asse.name, ch)
	c.Assert(err, IsNil)
	if asse.exposed {
		err = s.SetExposed()
		c.Assert(err, IsNil)
	}
}

type setupMachinesAndServices struct{}

func (smas setupMachinesAndServices) step(c *C, ctx *context) {
	addAndStartMachine{"0", state.JobManageEnviron}.step(c, ctx)
	setTools{"0", &state.Tools{
		Binary: version.Binary{
			Number: version.MustParse("1.2.3"),
			Series: "gutsy",
			Arch:   "ppc",
		},
		URL: "http://canonical.com/",
	}}.step(c, ctx)
	addAndStartMachine{"1", state.JobHostUnits}.step(c, ctx)
	addAndStartMachine{"2", state.JobHostUnits}.step(c, ctx)
	addCharm{"dummy"}.step(c, ctx)
	addServiceSetExposed{"dummy-service", "dummy", false}.step(c, ctx)
	addServiceSetExposed{"exposed-service", "dummy", true}.step(c, ctx)
}

type addUnit struct {
	serviceName string
	machineId   string
}

func (au addUnit) step(c *C, ctx *context) {
	s, err := ctx.st.Service(au.serviceName)
	c.Assert(err, IsNil)
	u, err := s.AddUnit()
	c.Assert(err, IsNil)
	m, err := ctx.st.Machine(au.machineId)
	c.Assert(err, IsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, IsNil)
}

type addAliveUnit struct {
	serviceName string
	machineId   string
}

func (aau addAliveUnit) step(c *C, ctx *context) {
	s, err := ctx.st.Service(aau.serviceName)
	c.Assert(err, IsNil)
	u, err := s.AddUnit()
	c.Assert(err, IsNil)
	pinger, err := u.SetAgentAlive()
	c.Assert(err, IsNil)
	ctx.st.StartSync()
	err = u.WaitAgentAlive(200 * time.Millisecond)
	c.Assert(err, IsNil)
	agentAlive, err := u.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(agentAlive, Equals, true)
	m, err := ctx.st.Machine(aau.machineId)
	c.Assert(err, IsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, IsNil)
	ctx.unitPingers[u.Name()] = pinger
}

type setUnitStatus struct {
	unitName   string
	status     state.UnitStatus
	statusInfo string
}

func (sus setUnitStatus) step(c *C, ctx *context) {
	u, err := ctx.st.Unit(sus.unitName)
	err = u.SetStatus(sus.status, sus.statusInfo)
	c.Assert(err, IsNil)
}

type expect struct {
	output M
}

func (e expect) step(c *C, ctx *context) {
	var wg sync.WaitGroup
	testFormat := func(format outputFormat, start chan bool) {
		defer wg.Done()

		<-start
		c.Logf("format %q", format.name)
		// Run command with the required format.
		code, stderr, stdout := runStatus(c, "--format", format.name)
		c.Assert(code, Equals, 0)
		c.Assert(stderr, HasLen, 0)

		// Prepare the output in the same format.
		buf, err := format.marshal(e.output)
		c.Assert(err, IsNil)
		expected := make(M)
		err = format.unmarshal(buf, &expected)
		c.Assert(err, IsNil)

		// Check the output is as expected.
		actual := make(M)
		err = format.unmarshal(stdout, &actual)
		c.Assert(err, IsNil)
		c.Assert(actual, DeepEquals, expected)
	}

	// Now execute the command concurrently for each format.
	start := make(chan bool)
	for _, format := range statusFormats {
		wg.Add(1)
		go testFormat(format, start)
		start <- true
	}
	wg.Wait()
}

func (s *StatusSuite) TestStatusAllFormats(c *C) {
	for i, t := range statusTests {
		c.Log("test %d: %s", i, t.summary)
		func() {
			// Prepare context and run all steps to setup.
			ctx := s.newContext()
			defer s.resetContext(c, ctx)
			ctx.run(c, t.steps)
		}()
	}
}
