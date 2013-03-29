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

func (s *StatusSuite) SetUpSuite(c *C) {
	s.JujuConnSuite.SetUpSuite(c)
}

func (s *StatusSuite) TearDownSuite(c *C) {
	s.JujuConnSuite.TearDownSuite(c)
}

func (s *StatusSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
}

func (s *StatusSuite) TearDownTest(c *C) {
	s.JujuConnSuite.TearDownTest(c)
}

type M map[string]interface{}

type testCase struct {
	summary string
	steps   []stepper
	output  M
}

func test(summary string, output M, steps ...stepper) testCase {
	return testCase{summary, steps, output}
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

var statusTests = []testCase{
	test(
		// unlikely, as you can't run juju status in real life without
		// machine/0 bootstrapped.
		"empty state",
		M{
			"machines": M{},
			"services": M{},
		},
	), test(
		"simulate juju bootstrap by adding machine/0 to the state",
		M{
			"machines": M{
				"0": M{
					"instance-id": "pending",
				},
			},
			"services": M{},
		},
		addMachine{"0", state.JobManageEnviron},
	), test(
		"simulate the PA starting an instance in response to the state change",
		M{
			"machines": M{
				"0": M{
					"dns-name":    "dummyenv-0.dns",
					"instance-id": "dummyenv-0",
				},
			},
			"services": M{},
		},
		addAndStartMachine{"0", state.JobManageEnviron},
	), test(
		"simulate the MA setting the version",
		M{
			"machines": M{
				"0": M{
					"dns-name":      "dummyenv-0.dns",
					"instance-id":   "dummyenv-0",
					"agent-version": "1.2.3",
				},
			},
			"services": M{},
		},
		addAndStartMachine{"0", state.JobManageEnviron},
		setTools{"0", &state.Tools{
			Binary: version.Binary{
				Number: version.MustParse("1.2.3"),
				Series: "gutsy",
				Arch:   "ppc",
			},
			URL: "http://canonical.com/",
		}},
	), test(
		"add two services and expose one",
		M{
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
		},
		addAndStartMachine{"0", state.JobManageEnviron},
		addCharm{"dummy"},
		addServiceSetExposed{"dummy-service", "dummy", false},
		addServiceSetExposed{"exposed-service", "dummy", true},
	), test(
		"add three machines, two for units; also two services, one exposed",
		M{
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
		},
		setupMachinesAndServices{},
	), test(
		"same scenario as above; add units for services, set status for both (one is down)",
		M{
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
							"machine":     "2",
							"status":      "error",
							"status-info": "You Require More Vespene Gas",
						},
					},
				},
				"dummy-service": M{
					"charm":   "local:series/dummy-1",
					"exposed": false,
					"units": M{
						"dummy-service/0": M{
							"machine": "1",
							"status":  "down",
						},
					},
				},
			},
		},
		setupMachinesAndServices{},
		addUnit{"dummy-service", "1"},
		addAliveUnit{"exposed-service", "2"},
		setUnitStatus{"exposed-service/0", state.UnitError, "You Require More Vespene Gas"},
		// This will be ignored, because the unit is down.
		setUnitStatus{"dummy-service/0", state.UnitStarted, ""},
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

func (s *StatusSuite) runTestCasesWithFormat(c *C, tests []testCase, format string, marshal func(v interface{}) ([]byte, error), unmarshal func(data []byte, v interface{}) error) {
	for i, t := range tests {
		c.Log("test %d: %s", i, t.summary)
		func() {
			// Prepare context and run all steps to setup.
			ctx := s.newContext()
			defer s.resetContext(c, ctx)
			ctx.run(c, t.steps)

			// Run command with the required format.
			code, stderr, stdout := runStatus(c, "--format", format)
			c.Assert(code, Equals, 0)
			c.Assert(stderr, HasLen, 0)

			// Prepare the output in the same format.
			buf, err := marshal(t.output)
			c.Assert(err, IsNil)
			expected := make(map[string]interface{})
			err = unmarshal(buf, &expected)
			c.Assert(err, IsNil)

			// Check the output is as expected.
			actual := make(map[string]interface{})
			err = unmarshal(stdout, &actual)
			c.Assert(err, IsNil)
			c.Assert(actual, DeepEquals, expected)
		}()
	}
}

func (s *StatusSuite) TestYamlStatus(c *C) {
	s.runTestCasesWithFormat(c, statusTests, "yaml", goyaml.Marshal, goyaml.Unmarshal)
}

func (s *StatusSuite) TestJsonStatus(c *C) {
	s.runTestCasesWithFormat(c, statusTests, "json", json.Marshal, json.Unmarshal)
}
