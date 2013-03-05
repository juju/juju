package state_test

import (
	"fmt"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"net/url"
	"sort"
	"strconv"
	"time"
)

type D []bson.DocElem

type StateSuite struct {
	ConnSuite
}

var _ = Suite(&StateSuite{})

func (s *StateSuite) TestDialAgain(c *C) {
	// Ensure idempotent operations on Dial are working fine.
	for i := 0; i < 2; i++ {
		st, err := state.Open(state.TestingStateInfo())
		c.Assert(err, IsNil)
		c.Assert(st.Close(), IsNil)
	}
}

func (s *StateSuite) TestStateInfo(c *C) {
	info := state.TestingStateInfo()
	c.Assert(s.State.Addrs(), DeepEquals, info.Addrs)
	c.Assert(s.State.CACert(), DeepEquals, info.CACert)
}

func (s *StateSuite) TestIsNotFound(c *C) {
	err1 := fmt.Errorf("unrelated error")
	err2 := state.NotFoundf("foo")
	c.Assert(state.IsNotFound(err1), Equals, false)
	c.Assert(state.IsNotFound(err2), Equals, true)
}

func (s *StateSuite) TestAddCharm(c *C) {
	// Check that adding charms from scratch works correctly.
	ch := testing.Charms.Dir("dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:series/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	bundleURL, err := url.Parse("http://bundles.example.com/dummy-1")
	c.Assert(err, IsNil)
	dummy, err := s.State.AddCharm(ch, curl, bundleURL, "dummy-1-sha256")
	c.Assert(err, IsNil)
	c.Assert(dummy.URL().String(), Equals, curl.String())

	doc := state.CharmDoc{}
	err = s.charms.FindId(curl).One(&doc)
	c.Assert(err, IsNil)
	c.Logf("%#v", doc)
	c.Assert(doc.URL, DeepEquals, curl)
}

func (s *StateSuite) AssertMachineCount(c *C, expect int) {
	ms, err := s.State.AllMachines()
	c.Assert(err, IsNil)
	c.Assert(len(ms), Equals, expect)
}

var jobStringTests = []struct {
	job state.MachineJob
	s   string
}{
	{state.JobHostUnits, "JobHostUnits"},
	{state.JobManageEnviron, "JobManageEnviron"},
	{state.JobServeAPI, "JobServeAPI"},
	{0, "<unknown job 0>"},
	{5, "<unknown job 5>"},
}

func (s *StateSuite) TestJobString(c *C) {
	for _, t := range jobStringTests {
		c.Check(t.job.String(), Equals, t.s)
	}
}

func (s *StateSuite) TestAddMachineErrors(c *C) {
	_, err := s.State.AddMachine("")
	c.Assert(err, ErrorMatches, "cannot add a new machine: no series specified")
	_, err = s.State.AddMachine("series")
	c.Assert(err, ErrorMatches, "cannot add a new machine: no jobs specified")
	_, err = s.State.AddMachine("series", state.JobHostUnits, state.JobHostUnits)
	c.Assert(err, ErrorMatches, "cannot add a new machine: duplicate job: .*")
}

func (s *StateSuite) TestAddMachines(c *C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	m0, err := s.State.AddMachine("series", oneJob...)
	c.Assert(err, IsNil)
	check := func(m *state.Machine, id, series string, jobs []state.MachineJob) {
		c.Assert(m.Id(), Equals, id)
		c.Assert(m.Series(), Equals, series)
		c.Assert(m.Jobs(), DeepEquals, jobs)
	}
	check(m0, "0", "series", oneJob)
	m0, err = s.State.Machine("0")
	c.Assert(err, IsNil)
	check(m0, "0", "series", oneJob)

	allJobs := []state.MachineJob{
		state.JobHostUnits,
		state.JobManageEnviron,
		state.JobServeAPI,
	}
	m1, err := s.State.AddMachine("blahblah", allJobs...)
	c.Assert(err, IsNil)
	check(m1, "1", "blahblah", allJobs)

	m1, err = s.State.Machine("1")
	c.Assert(err, IsNil)
	check(m1, "1", "blahblah", allJobs)

	m, err := s.State.AllMachines()
	c.Assert(err, IsNil)
	c.Assert(m, HasLen, 2)
	check(m[0], "0", "series", oneJob)
	check(m[1], "1", "blahblah", allJobs)
}

func (s *StateSuite) TestInjectMachineErrors(c *C) {
	_, err := s.State.InjectMachine("", state.InstanceId("i-minvalid"), state.JobHostUnits)
	c.Assert(err, ErrorMatches, "cannot add a new machine: no series specified")
	_, err = s.State.InjectMachine("series", state.InstanceId(""), state.JobHostUnits)
	c.Assert(err, ErrorMatches, "cannot inject a machine without an instance id")
	_, err = s.State.InjectMachine("series", state.InstanceId("i-mlazy"))
	c.Assert(err, ErrorMatches, "cannot add a new machine: no jobs specified")
}

func (s *StateSuite) TestInjectMachine(c *C) {
	m, err := s.State.InjectMachine("series", state.InstanceId("i-mindustrious"), state.JobHostUnits, state.JobManageEnviron)
	c.Assert(err, IsNil)
	c.Assert(m.Jobs(), DeepEquals, []state.MachineJob{state.JobHostUnits, state.JobManageEnviron})
	instanceId, ok := m.InstanceId()
	c.Assert(ok, Equals, true)
	c.Assert(instanceId, Equals, state.InstanceId("i-mindustrious"))
}

func (s *StateSuite) TestReadMachine(c *C) {
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	expectedId := machine.Id()
	machine, err = s.State.Machine(expectedId)
	c.Assert(err, IsNil)
	c.Assert(machine.Id(), Equals, expectedId)
}

func (s *StateSuite) TestMachineNotFound(c *C) {
	_, err := s.State.Machine("0")
	c.Assert(err, ErrorMatches, "machine 0 not found")
	c.Assert(state.IsNotFound(err), Equals, true)
}

func (s *StateSuite) TestAllMachines(c *C) {
	numInserts := 42
	for i := 0; i < numInserts; i++ {
		m, err := s.State.AddMachine("series", state.JobHostUnits)
		c.Assert(err, IsNil)
		err = m.SetInstanceId(state.InstanceId(fmt.Sprintf("foo-%d", i)))
		c.Assert(err, IsNil)
		err = m.SetAgentTools(newTools("7.8.9-foo-bar", "http://arble.tgz"))
		c.Assert(err, IsNil)
		err = m.Destroy()
		c.Assert(err, IsNil)
	}
	s.AssertMachineCount(c, numInserts)
	ms, _ := s.State.AllMachines()
	for i, m := range ms {
		c.Assert(m.Id(), Equals, strconv.Itoa(i))
		instId, ok := m.InstanceId()
		c.Assert(ok, Equals, true)
		c.Assert(string(instId), Equals, fmt.Sprintf("foo-%d", i))
		tools, err := m.AgentTools()
		c.Check(err, IsNil)
		c.Check(tools, DeepEquals, newTools("7.8.9-foo-bar", "http://arble.tgz"))
		c.Assert(m.Life(), Equals, state.Dying)
	}
}

func (s *StateSuite) TestAddService(c *C) {
	charm := s.AddTestingCharm(c, "dummy")
	_, err := s.State.AddService("haha/borken", charm)
	c.Assert(err, ErrorMatches, `cannot add service "haha/borken": invalid name`)
	_, err = s.State.Service("haha/borken")
	c.Assert(err, ErrorMatches, `"haha/borken" is not a valid service name`)

	wordpress, err := s.State.AddService("wordpress", charm)
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")
	mysql, err := s.State.AddService("mysql", charm)
	c.Assert(err, IsNil)
	c.Assert(mysql.Name(), Equals, "mysql")

	// Check that retrieving the new created services works correctly.
	wordpress, err = s.State.Service("wordpress")
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")
	ch, _, err := wordpress.Charm()
	c.Assert(err, IsNil)
	c.Assert(ch.URL(), DeepEquals, charm.URL())
	mysql, err = s.State.Service("mysql")
	c.Assert(err, IsNil)
	c.Assert(mysql.Name(), Equals, "mysql")
	ch, _, err = mysql.Charm()
	c.Assert(err, IsNil)
	c.Assert(ch.URL(), DeepEquals, charm.URL())
}

func (s *StateSuite) TestServiceNotFound(c *C) {
	_, err := s.State.Service("bummer")
	c.Assert(err, ErrorMatches, `service "bummer" not found`)
	c.Assert(state.IsNotFound(err), Equals, true)
}

func (s *StateSuite) TestAllServices(c *C) {
	charm := s.AddTestingCharm(c, "dummy")
	services, err := s.State.AllServices()
	c.Assert(err, IsNil)
	c.Assert(len(services), Equals, 0)

	// Check that after adding services the result is ok.
	_, err = s.State.AddService("wordpress", charm)
	c.Assert(err, IsNil)
	services, err = s.State.AllServices()
	c.Assert(err, IsNil)
	c.Assert(len(services), Equals, 1)

	_, err = s.State.AddService("mysql", charm)
	c.Assert(err, IsNil)
	services, err = s.State.AllServices()
	c.Assert(err, IsNil)
	c.Assert(len(services), Equals, 2)

	// Check the returned service, order is defined by sorted keys.
	c.Assert(services[0].Name(), Equals, "wordpress")
	c.Assert(services[1].Name(), Equals, "mysql")
}

var inferEndpointsTests = []struct {
	summary string
	inputs  [][]string
	eps     []state.Endpoint
	err     string
}{
	{
		summary: "insane args",
		inputs:  [][]string{nil},
		err:     `cannot relate 0 endpoints`,
	}, {
		summary: "insane args",
		inputs:  [][]string{{"blah", "blur", "bleurgh"}},
		err:     `cannot relate 3 endpoints`,
	}, {
		summary: "invalid args",
		inputs: [][]string{
			{"ping:"},
			{":pong"},
			{":"},
		},
		err: `invalid endpoint ".*"`,
	}, {
		summary: "unknown service",
		inputs:  [][]string{{"wooble"}},
		err:     `service "wooble" not found`,
	}, {
		summary: "invalid relations",
		inputs: [][]string{
			{"lg", "lg"},
			{"ms", "ms"},
			{"wp", "wp"},
			{"rk1", "rk1"},
			{"rk1", "rk2"},
		},
		err: `no relations found`,
	}, {
		summary: "valid peer relation",
		inputs: [][]string{
			{"rk1"},
			{"rk1:ring"},
		},
		eps: []state.Endpoint{{
			ServiceName:   "rk1",
			Interface:     "riak",
			RelationName:  "ring",
			RelationRole:  state.RolePeer,
			RelationScope: charm.ScopeGlobal,
		}},
	}, {
		summary: "ambiguous provider/requirer relation",
		inputs: [][]string{
			{"ms", "wp"},
			{"ms", "wp:db"},
		},
		err: `ambiguous relation: ".*" could refer to "wp:db ms:dev"; "wp:db ms:prod"`,
	}, {
		summary: "unambiguous provider/requirer relation",
		inputs: [][]string{
			{"ms:dev", "wp"},
			{"ms:dev", "wp:db"},
		},
		eps: []state.Endpoint{{
			ServiceName:   "ms",
			Interface:     "mysql",
			RelationName:  "dev",
			RelationRole:  state.RoleProvider,
			RelationScope: charm.ScopeGlobal,
		}, {
			ServiceName:   "wp",
			Interface:     "mysql",
			RelationName:  "db",
			RelationRole:  state.RoleRequirer,
			RelationScope: charm.ScopeGlobal,
		}},
	}, {
		summary: "explicit logging relation is preferred over implicit juju-info",
		inputs:  [][]string{{"lg", "wp"}},
		eps: []state.Endpoint{{
			ServiceName:   "lg",
			Interface:     "logging",
			RelationName:  "logging-directory",
			RelationRole:  state.RoleRequirer,
			RelationScope: charm.ScopeContainer,
		}, {
			ServiceName:   "wp",
			Interface:     "logging",
			RelationName:  "logging-dir",
			RelationRole:  state.RoleProvider,
			RelationScope: charm.ScopeContainer,
		}},
	}, {
		summary: "implict relations can be chosen explicitly",
		inputs: [][]string{
			{"lg:info", "wp"},
			{"lg", "wp:juju-info"},
			{"lg:info", "wp:juju-info"},
		},
		eps: []state.Endpoint{{
			ServiceName:   "lg",
			Interface:     "juju-info",
			RelationName:  "info",
			RelationRole:  state.RoleRequirer,
			RelationScope: charm.ScopeContainer,
		}, {
			ServiceName:   "wp",
			Interface:     "juju-info",
			RelationName:  "juju-info",
			RelationRole:  state.RoleProvider,
			RelationScope: charm.ScopeGlobal,
		}},
	}, {
		summary: "implicit relations will be chosen if there are no other options",
		inputs:  [][]string{{"lg", "ms"}},
		eps: []state.Endpoint{{
			ServiceName:   "lg",
			Interface:     "juju-info",
			RelationName:  "info",
			RelationRole:  state.RoleRequirer,
			RelationScope: charm.ScopeContainer,
		}, {
			ServiceName:   "ms",
			Interface:     "juju-info",
			RelationName:  "juju-info",
			RelationRole:  state.RoleProvider,
			RelationScope: charm.ScopeGlobal,
		}},
	},
}

func (s *StateSuite) TestInferEndpoints(c *C) {
	_, err := s.State.AddService("ms", s.AddTestingCharm(c, "mysql-alternative"))
	c.Assert(err, IsNil)
	_, err = s.State.AddService("wp", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	_, err = s.State.AddService("lg", s.AddTestingCharm(c, "logging"))
	c.Assert(err, IsNil)
	riak := s.AddTestingCharm(c, "riak")
	_, err = s.State.AddService("rk1", riak)
	c.Assert(err, IsNil)
	_, err = s.State.AddService("rk2", riak)
	c.Assert(err, IsNil)

	for i, t := range inferEndpointsTests {
		c.Logf("test %d", i)
		for j, input := range t.inputs {
			c.Logf("  input %d", j)
			eps, err := s.State.InferEndpoints(input)
			if t.err == "" {
				c.Assert(err, IsNil)
				c.Assert(eps, DeepEquals, t.eps)
			} else {
				c.Assert(err, ErrorMatches, t.err)
			}
		}
	}
}

func (s *StateSuite) TestEnvironConfig(c *C) {
	initial := map[string]interface{}{
		"name":                      "test",
		"type":                      "test",
		"authorized-keys":           "i-am-a-key",
		"default-series":            "precise",
		"agent-version":             "1.2.3",
		"development":               true,
		"firewall-mode":             "",
		"admin-secret":              "",
		"ca-cert":                   testing.CACert,
		"ca-private-key":            "",
		"ssl-hostname-verification": true,
	}
	cfg, err := config.New(initial)
	c.Assert(err, IsNil)
	st, err := state.Initialize(state.TestingStateInfo(), cfg)
	c.Assert(err, IsNil)
	st.Close()
	c.Assert(err, IsNil)
	cfg, err = s.State.EnvironConfig()
	c.Assert(err, IsNil)
	current := cfg.AllAttrs()
	c.Assert(current, DeepEquals, initial)

	current["authorized-keys"] = "i-am-a-new-key"
	cfg, err = config.New(current)
	c.Assert(err, IsNil)
	err = s.State.SetEnvironConfig(cfg)
	c.Assert(err, IsNil)
	cfg, err = s.State.EnvironConfig()
	c.Assert(err, IsNil)
	final := cfg.AllAttrs()
	c.Assert(final, DeepEquals, current)
}

func (s *StateSuite) TestEnvironConfigWithAdminSecret(c *C) {
	attrs := map[string]interface{}{
		"name":            "test",
		"type":            "test",
		"authorized-keys": "i-am-a-key",
		"default-series":  "precise",
		"development":     true,
		"admin-secret":    "foo",
		"ca-cert":         testing.CACert,
		"ca-private-key":  "",
	}
	cfg, err := config.New(attrs)
	c.Assert(err, IsNil)
	_, err = state.Initialize(state.TestingStateInfo(), cfg)
	c.Assert(err, ErrorMatches, "admin-secret should never be written to the state")

	delete(attrs, "admin-secret")
	cfg, err = config.New(attrs)
	st, err := state.Initialize(state.TestingStateInfo(), cfg)
	c.Assert(err, IsNil)
	st.Close()

	cfg, err = cfg.Apply(map[string]interface{}{"admin-secret": "foo"})
	err = s.State.SetEnvironConfig(cfg)
	c.Assert(err, ErrorMatches, "admin-secret should never be written to the state")
}

func (s *StateSuite) TestEnvironConstraints(c *C) {
	// Environ constraints are not available before initialization.
	_, err := s.State.EnvironConstraints()
	c.Assert(state.IsNotFound(err), Equals, true)
	m := map[string]interface{}{
		"type":            "dummy",
		"name":            "lisboa",
		"authorized-keys": "i-am-a-key",
		"ca-cert":         testing.CACert,
		"ca-private-key":  "",
	}
	cfg, err := config.New(m)
	c.Assert(err, IsNil)
	st, err := state.Initialize(state.TestingStateInfo(), cfg)
	c.Assert(err, IsNil)
	st.Close()

	// Environ constraints start out empty (for now).
	cons0 := state.Constraints{}
	cons1, err := s.State.EnvironConstraints()
	c.Assert(err, IsNil)
	c.Assert(cons1, DeepEquals, cons0)

	// Environ constraints can be set.
	cons2 := state.Constraints{Mem: uint64p(1024)}
	err = s.State.SetEnvironConstraints(cons2)
	c.Assert(err, IsNil)
	cons3, err := s.State.EnvironConstraints()
	c.Assert(err, IsNil)
	c.Assert(cons3, DeepEquals, cons2)
	c.Assert(cons3, Not(Equals), cons2)

	// Environ constraints are completely overwritten when re-set.
	cons4 := state.Constraints{CpuPower: uint64p(250)}
	err = s.State.SetEnvironConstraints(cons4)
	c.Assert(err, IsNil)
	cons5, err := s.State.EnvironConstraints()
	c.Assert(err, IsNil)
	c.Assert(cons5, DeepEquals, cons4)
	c.Assert(cons5, Not(Equals), cons4)
}

var machinesWatchTests = []struct {
	summary string
	test    func(*C, *state.State)
	changes []string
}{
	{
		"Do nothing",
		func(_ *C, _ *state.State) {},
		nil,
	}, {
		"Add a machine",
		func(c *C, s *state.State) {
			_, err := s.AddMachine("series", state.JobHostUnits)
			c.Assert(err, IsNil)
		},
		[]string{"0"},
	}, {
		"Ignore unrelated changes",
		func(c *C, s *state.State) {
			_, err := s.AddMachine("series", state.JobHostUnits)
			c.Assert(err, IsNil)
			m0, err := s.Machine("0")
			c.Assert(err, IsNil)
			err = m0.SetInstanceId("spam")
			c.Assert(err, IsNil)
		},
		[]string{"1"},
	}, {
		"Add two machines at once",
		func(c *C, s *state.State) {
			_, err := s.AddMachine("series", state.JobHostUnits)
			c.Assert(err, IsNil)
			_, err = s.AddMachine("series", state.JobHostUnits)
			c.Assert(err, IsNil)
		},
		[]string{"2", "3"},
	}, {
		"Report machines that become Dying",
		func(c *C, s *state.State) {
			m3, err := s.Machine("3")
			c.Assert(err, IsNil)
			err = m3.Destroy()
			c.Assert(err, IsNil)
		},
		[]string{"3"},
	}, {
		"Report machines that become Dead",
		func(c *C, s *state.State) {
			m3, err := s.Machine("3")
			c.Assert(err, IsNil)
			err = m3.EnsureDead()
			c.Assert(err, IsNil)
		},
		[]string{"3"},
	}, {
		"Do not report Dead machines that are removed",
		func(c *C, s *state.State) {
			m0, err := s.Machine("0")
			c.Assert(err, IsNil)
			err = m0.Destroy()
			c.Assert(err, IsNil)
			m3, err := s.Machine("3")
			c.Assert(err, IsNil)
			err = m3.Remove()
			c.Assert(err, IsNil)
		},
		[]string{"0"},
	}, {
		"Report previously known machines that are removed",
		func(c *C, s *state.State) {
			m0, err := s.Machine("0")
			c.Assert(err, IsNil)
			err = m0.EnsureDead()
			c.Assert(err, IsNil)
			m2, err := s.Machine("2")
			c.Assert(err, IsNil)
			err = m2.EnsureDead()
			c.Assert(err, IsNil)
			err = m2.Remove()
			c.Assert(err, IsNil)
		},
		[]string{"0", "2"},
	}, {
		"Added and Dead machines at once",
		func(c *C, s *state.State) {
			_, err := s.AddMachine("series", state.JobHostUnits)
			c.Assert(err, IsNil)
			m1, err := s.Machine("1")
			c.Assert(err, IsNil)
			err = m1.EnsureDead()
			c.Assert(err, IsNil)
		},
		[]string{"1", "4"},
	}, {
		"Add many, change many, and remove many at once",
		func(c *C, s *state.State) {
			machines := [20]*state.Machine{}
			var err error
			for i := 0; i < len(machines); i++ {
				machines[i], err = s.AddMachine("series", state.JobHostUnits)
				c.Assert(err, IsNil)
			}
			for i := 0; i < len(machines); i++ {
				err = machines[i].SetInstanceId(state.InstanceId("spam" + fmt.Sprint(i)))
				c.Assert(err, IsNil)
			}
			for i := 10; i < len(machines); i++ {
				err = machines[i].EnsureDead()
				c.Assert(err, IsNil)
				err = machines[i].Remove()
				c.Assert(err, IsNil)
			}
		},
		[]string{"5", "6", "7", "8", "9", "10", "11", "12", "13", "14"},
	}, {
		"Do not report never-seen and removed or dead",
		func(c *C, s *state.State) {
			m, err := s.AddMachine("series", state.JobHostUnits)
			c.Assert(err, IsNil)
			err = m.EnsureDead()
			c.Assert(err, IsNil)

			m, err = s.AddMachine("series", state.JobHostUnits)
			c.Assert(err, IsNil)
			err = m.EnsureDead()
			c.Assert(err, IsNil)
			err = m.Remove()
			c.Assert(err, IsNil)

			_, err = s.AddMachine("series", state.JobHostUnits)
			c.Assert(err, IsNil)
		},
		[]string{"27"},
	}, {
		"Take into account what's already in the queue",
		func(c *C, s *state.State) {
			m, err := s.AddMachine("series", state.JobHostUnits)
			c.Assert(err, IsNil)
			s.Sync()
			err = m.EnsureDead()
			c.Assert(err, IsNil)
			s.Sync()
			err = m.Remove()
			c.Assert(err, IsNil)
			s.Sync()
		},
		[]string{"28"},
	},
}

func (s *StateSuite) TestWatchMachines(c *C) {
	machineWatcher := s.State.WatchMachines()
	defer func() {
		c.Assert(machineWatcher.Stop(), IsNil)
	}()
	for i, test := range machinesWatchTests {
		c.Logf("Test %d: %s", i, test.summary)
		test.test(c, s.State)
		s.State.StartSync()
		var got []string
		for {
			select {
			case ids, ok := <-machineWatcher.Changes():
				c.Assert(ok, Equals, true)
				got = append(got, ids...)
				if len(got) < len(test.changes) {
					continue
				}
				sort.Strings(got)
				sort.Strings(test.changes)
				c.Assert(got, DeepEquals, test.changes)
			case <-time.After(500 * time.Millisecond):
				c.Fatalf("did not get change: want %#v, got %#v", test.changes, got)
			}
			break
		}
	}
	select {
	case got := <-machineWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

var servicesWatchTests = []struct {
	summary string
	test    func(*C, *state.State, *state.Charm)
	changes []string
}{
	{
		"check initial empty event",
		func(_ *C, _ *state.State, _ *state.Charm) {},
		[]string(nil),
	},
	{
		"add a service",
		func(c *C, s *state.State, ch *state.Charm) {
			_, err := s.AddService("s0", ch)
			c.Assert(err, IsNil)
		},
		[]string{"s0"},
	},
	{
		"add a service and test unrelated change",
		func(c *C, s *state.State, ch *state.Charm) {
			svc, err := s.Service("s0")
			c.Assert(err, IsNil)
			err = svc.SetExposed()
			c.Assert(err, IsNil)
			_, err = s.AddService("s1", ch)
			c.Assert(err, IsNil)
		},
		[]string{"s1"},
	},
	{
		"add two services",
		func(c *C, s *state.State, ch *state.Charm) {
			_, err := s.AddService("s2", ch)
			c.Assert(err, IsNil)
			_, err = s.AddService("s3", ch)
			c.Assert(err, IsNil)
		},
		[]string{"s2", "s3"},
	},
	{
		"destroy a service",
		func(c *C, s *state.State, _ *state.Charm) {
			svc3, err := s.Service("s3")
			c.Assert(err, IsNil)
			err = svc3.Destroy()
			c.Assert(err, IsNil)
		},
		[]string{"s3"},
	},
	{
		"destroy one (to Dying); remove another",
		func(c *C, s *state.State, _ *state.Charm) {
			svc0, err := s.Service("s0")
			c.Assert(err, IsNil)
			_, err = svc0.AddUnit()
			c.Assert(err, IsNil)
			err = svc0.Destroy()
			c.Assert(err, IsNil)
			svc2, err := s.Service("s2")
			c.Assert(err, IsNil)
			err = svc2.Destroy()
			c.Assert(err, IsNil)
		},
		[]string{"s0", "s2"},
	},
	{
		"remove the Dying one",
		func(c *C, s *state.State, _ *state.Charm) {
			svc0, err := s.Service("s0")
			c.Assert(err, IsNil)
			removeAllUnits(c, svc0)
		},
		[]string{"s0"},
	},
	{
		"add and remove a service at the same time",
		func(c *C, s *state.State, ch *state.Charm) {
			_, err := s.AddService("s4", ch)
			c.Assert(err, IsNil)
			svc1, err := s.Service("s1")
			c.Assert(err, IsNil)
			err = svc1.Destroy()
			c.Assert(err, IsNil)
		},
		[]string{"s1", "s4"},
	},
	{
		"add and remove many services at once",
		func(c *C, s *state.State, ch *state.Charm) {
			services := [20]*state.Service{}
			var err error
			for i := 0; i < len(services); i++ {
				services[i], err = s.AddService("ss"+fmt.Sprint(i), ch)
				c.Assert(err, IsNil)
			}
			for i := 10; i < len(services); i++ {
				err = services[i].Destroy()
				c.Assert(err, IsNil)
			}
		},
		[]string{"ss0", "ss1", "ss2", "ss3", "ss4", "ss5", "ss6", "ss7", "ss8", "ss9"},
	},
	{
		"set exposed and change life at the same time",
		func(c *C, s *state.State, ch *state.Charm) {
			_, err := s.AddService("twenty-five", ch)
			c.Assert(err, IsNil)
			svc9, err := s.Service("ss9")
			c.Assert(err, IsNil)
			err = svc9.SetExposed()
			c.Assert(err, IsNil)
			err = svc9.Destroy()
			c.Assert(err, IsNil)
		},
		[]string{"twenty-five", "ss9"},
	},
}

func (s *StateSuite) TestWatchServices(c *C) {
	serviceWatcher := s.State.WatchServices()
	defer func() {
		c.Assert(serviceWatcher.Stop(), IsNil)
	}()
	charm := s.AddTestingCharm(c, "dummy")
	for i, test := range servicesWatchTests {
		c.Logf("test %d: %s", i, test.summary)
		test.test(c, s.State, charm)
		s.State.StartSync()
		var got []string
		for {
			select {
			case new, ok := <-serviceWatcher.Changes():
				c.Assert(ok, Equals, true)
				got = append(got, new...)
				if len(got) < len(test.changes) {
					continue
				}
				sort.Strings(got)
				sort.Strings(test.changes)
				c.Assert(got, DeepEquals, test.changes)
			case <-time.After(500 * time.Millisecond):
				c.Fatalf("did not get change, want: %#v, got: %#v", test.changes, got)
			}
			break
		}
	}
	select {
	case got := <-serviceWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func (s *StateSuite) TestInitialize(c *C) {
	m := map[string]interface{}{
		"type":                      "dummy",
		"name":                      "lisboa",
		"authorized-keys":           "i-am-a-key",
		"default-series":            "precise",
		"agent-version":             "1.2.3",
		"development":               true,
		"firewall-mode":             "",
		"admin-secret":              "",
		"ca-cert":                   testing.CACert,
		"ca-private-key":            "",
		"ssl-hostname-verification": true,
	}
	cfg, err := config.New(m)
	c.Assert(err, IsNil)
	st, err := state.Initialize(state.TestingStateInfo(), cfg)
	c.Assert(err, IsNil)
	c.Assert(st, NotNil)
	defer st.Close()
	env, err := st.EnvironConfig()
	c.Assert(env.AllAttrs(), DeepEquals, m)
}

func (s *StateSuite) TestDoubleInitialize(c *C) {
	m := map[string]interface{}{
		"type":                      "dummy",
		"name":                      "lisboa",
		"authorized-keys":           "i-am-a-key",
		"default-series":            "precise",
		"agent-version":             "1.2.3",
		"development":               true,
		"firewall-mode":             "",
		"admin-secret":              "",
		"ca-cert":                   testing.CACert,
		"ca-private-key":            "",
		"ssl-hostname-verification": true,
	}
	cfg, err := config.New(m)
	c.Assert(err, IsNil)
	st, err := state.Initialize(state.TestingStateInfo(), cfg)
	c.Assert(err, IsNil)
	c.Assert(st, NotNil)
	env1, err := st.EnvironConfig()
	st.Close()

	// initialize again, there should be no error and the
	// environ config should not change.
	m = map[string]interface{}{
		"type":                      "dummy",
		"name":                      "sydney",
		"authorized-keys":           "i-am-not-an-animal",
		"default-series":            "xanadu",
		"development":               false,
		"agent-version":             "3.4.5",
		"firewall-mode":             "",
		"admin-secret":              "",
		"ca-cert":                   testing.CACert,
		"ca-private-key":            "",
		"ssl-hostname-verification": false,
	}
	cfg, err = config.New(m)
	c.Assert(err, IsNil)
	st, err = state.Initialize(state.TestingStateInfo(), cfg)
	c.Assert(err, IsNil)
	c.Assert(st, NotNil)
	env2, err := st.EnvironConfig()
	st.Close()

	c.Assert(env1.AllAttrs(), DeepEquals, env2.AllAttrs())
}

var sortPortsTests = []struct {
	have, want []state.Port
}{
	{nil, []state.Port{}},
	{[]state.Port{{"b", 1}, {"a", 99}, {"a", 1}}, []state.Port{{"a", 1}, {"a", 99}, {"b", 1}}},
}

func (*StateSuite) TestSortPorts(c *C) {
	for _, t := range sortPortsTests {
		p := make([]state.Port, len(t.have))
		copy(p, t.have)
		state.SortPorts(p)
		c.Check(p, DeepEquals, t.want)
		state.SortPorts(p)
		c.Check(p, DeepEquals, t.want)
	}
}

func (*StateSuite) TestNameChecks(c *C) {
	assertService := func(s string, expect bool) {
		c.Assert(state.IsServiceName(s), Equals, expect)
		c.Assert(state.IsUnitName(s+"/0"), Equals, expect)
		c.Assert(state.IsUnitName(s+"/99"), Equals, expect)
		c.Assert(state.IsUnitName(s+"/-1"), Equals, false)
		c.Assert(state.IsUnitName(s+"/blah"), Equals, false)
	}
	assertService("", false)
	assertService("33", false)
	assertService("wordpress", true)
	assertService("w0rd-pre55", true)
	assertService("foo2", true)
	assertService("foo-2", false)
	assertService("foo-2foo", true)

	assertMachine := func(s string, expect bool) {
		c.Assert(state.IsMachineId(s), Equals, expect)
	}
	assertMachine("0", true)
	assertMachine("1", true)
	assertMachine("1000001", true)
	assertMachine("01", false)
	assertMachine("-1", false)
	assertMachine("", false)
	assertMachine("cantankerous", false)
}

type attrs map[string]interface{}

var watchEnvironConfigTests = []attrs{
	{
		"type":            "my-type",
		"name":            "my-name",
		"authorized-keys": "i-am-a-key",
		"ca-cert":         testing.CACert,
		"ca-private-key":  "",
	},
	{
		// Add an attribute.
		"type":            "my-type",
		"name":            "my-name",
		"default-series":  "my-series",
		"authorized-keys": "i-am-a-key",
		"ca-cert":         testing.CACert,
		"ca-private-key":  "",
	},
	{
		// Set a new attribute value.
		"type":            "my-type",
		"name":            "my-new-name",
		"default-series":  "my-series",
		"authorized-keys": "i-am-a-key",
		"ca-cert":         testing.CACert,
		"ca-private-key":  "",
	},
}

func (s *StateSuite) TestWatchEnvironConfig(c *C) {
	watcher := s.State.WatchEnvironConfig()
	defer func() {
		c.Assert(watcher.Stop(), IsNil)
	}()
	for i, test := range watchEnvironConfigTests {
		c.Logf("test %d", i)
		change, err := config.New(test)
		c.Assert(err, IsNil)
		if i == 0 {
			st, err := state.Initialize(state.TestingStateInfo(), change)
			c.Assert(err, IsNil)
			st.Close()
		} else {
			err = s.State.SetEnvironConfig(change)
			c.Assert(err, IsNil)
		}
		c.Assert(err, IsNil)
		s.State.StartSync()
		select {
		case got, ok := <-watcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got.AllAttrs(), DeepEquals, change.AllAttrs())
		case <-time.After(500 * time.Millisecond):
			c.Fatalf("did not get change: %#v", test)
		}
	}

	select {
	case got := <-watcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(50 * time.Millisecond):
	}
}

func (s *StateSuite) TestWatchEnvironConfigAfterCreation(c *C) {
	cfg, err := config.New(watchEnvironConfigTests[0])
	c.Assert(err, IsNil)
	st, err := state.Initialize(state.TestingStateInfo(), cfg)
	c.Assert(err, IsNil)
	st.Close()
	s.State.Sync()
	watcher := s.State.WatchEnvironConfig()
	defer watcher.Stop()
	select {
	case got, ok := <-watcher.Changes():
		c.Assert(ok, Equals, true)
		c.Assert(got.AllAttrs(), DeepEquals, cfg.AllAttrs())
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("did not get change")
	}
}

func (s *StateSuite) TestWatchEnvironConfigInvalidConfig(c *C) {
	m := map[string]interface{}{
		"type":            "dummy",
		"name":            "lisboa",
		"authorized-keys": "i-am-a-key",
		"ca-cert":         testing.CACert,
		"ca-private-key":  "",
	}
	cfg1, err := config.New(m)
	c.Assert(err, IsNil)
	st, err := state.Initialize(state.TestingStateInfo(), cfg1)
	c.Assert(err, IsNil)
	st.Close()

	// Corrupt the environment configuration.
	settings := s.Session.DB("juju").C("settings")
	err = settings.UpdateId("e", bson.D{{"$unset", bson.D{{"name", 1}}}})
	c.Assert(err, IsNil)

	s.State.Sync()

	// Start watching the configuration.
	watcher := s.State.WatchEnvironConfig()
	defer watcher.Stop()
	done := make(chan *config.Config)
	go func() {
		select {
		case cfg, ok := <-watcher.Changes():
			if !ok {
				c.Errorf("watcher channel closed")
			} else {
				done <- cfg
			}
		case <-time.After(5 * time.Second):
			c.Fatalf("no environment configuration observed")
		}
	}()

	s.State.Sync()

	// The invalid configuration must not have been generated.
	select {
	case <-done:
		c.Fatalf("configuration returned too soon")
	case <-time.After(100 * time.Millisecond):
	}

	// Fix the configuration.
	cfg2, err := config.New(map[string]interface{}{
		"type":            "dummy",
		"name":            "lisboa",
		"authorized-keys": "new-key",
		"ca-cert":         testing.CACert,
		"ca-private-key":  "",
	})
	c.Assert(err, IsNil)
	err = s.State.SetEnvironConfig(cfg2)
	c.Assert(err, IsNil)

	s.State.StartSync()
	select {
	case cfg3 := <-done:
		c.Assert(cfg3.AllAttrs(), DeepEquals, cfg2.AllAttrs())
	case <-time.After(5 * time.Second):
		c.Fatalf("no environment configuration observed")
	}
}

func (s *StateSuite) TestAddAndGetEquivalence(c *C) {
	// The equivalence tested here isn't necessarily correct, and
	// comparing private details is discouraged in the project.
	// The implementation might choose to cache information, or
	// to have different logic when adding or removing, and the
	// comparison might fail despite it being correct.
	// That said, we've had bugs with txn-revno being incorrect
	// before, so this testing at least ensures we're conscious
	// about such changes.

	m1, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	m2, err := s.State.Machine(m1.Id())
	c.Assert(m1, DeepEquals, m2)

	charm1 := s.AddTestingCharm(c, "wordpress")
	charm2, err := s.State.Charm(charm1.URL())
	c.Assert(err, IsNil)
	c.Assert(charm1, DeepEquals, charm2)

	wordpress1, err := s.State.AddService("wordpress", charm1)
	c.Assert(err, IsNil)
	wordpress2, err := s.State.Service("wordpress")
	c.Assert(err, IsNil)
	c.Assert(wordpress1, DeepEquals, wordpress2)

	unit1, err := wordpress1.AddUnit()
	c.Assert(err, IsNil)
	unit2, err := s.State.Unit("wordpress/0")
	c.Assert(err, IsNil)
	c.Assert(unit1, DeepEquals, unit2)

	_, err = s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, IsNil)
	relation1, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)
	relation2, err := s.State.EndpointsRelation(eps...)
	c.Assert(relation1, DeepEquals, relation2)
	relation3, err := s.State.Relation(relation1.Id())
	c.Assert(relation1, DeepEquals, relation3)
}

func tryOpenState(info *state.Info) error {
	st, err := state.Open(info)
	if err == nil {
		st.Close()
	}
	return err
}

func (s *StateSuite) TestOpenWithoutSetMongoPassword(c *C) {
	info := state.TestingStateInfo()
	info.EntityName, info.Password = "arble", "bar"
	err := tryOpenState(info)
	c.Assert(state.IsUnauthorizedError(err), Equals, true)

	info.EntityName, info.Password = "arble", ""
	err = tryOpenState(info)
	c.Assert(state.IsUnauthorizedError(err), Equals, true)

	info.EntityName, info.Password = "", ""
	err = tryOpenState(info)
	c.Assert(err, IsNil)
}

func (s *StateSuite) TestOpenBadAddress(c *C) {
	info := state.TestingStateInfo()
	info.Addrs = []string{"0.1.2.3:1234"}
	state.SetDialTimeout(1 * time.Millisecond)
	defer state.SetDialTimeout(0)

	err := tryOpenState(info)
	c.Assert(err, ErrorMatches, "no reachable servers")
}

func testSetPassword(c *C, getEntity func() (state.Entity, error)) {
	e, err := getEntity()
	c.Assert(err, IsNil)

	c.Assert(e.PasswordValid("foo"), Equals, false)
	err = e.SetPassword("foo")
	c.Assert(err, IsNil)
	c.Assert(e.PasswordValid("foo"), Equals, true)

	// Check a newly-fetched entity has the same password.
	e2, err := getEntity()
	c.Assert(err, IsNil)
	c.Assert(e2.PasswordValid("foo"), Equals, true)

	err = e.SetPassword("bar")
	c.Assert(err, IsNil)
	c.Assert(e.PasswordValid("foo"), Equals, false)
	c.Assert(e.PasswordValid("bar"), Equals, true)

	// Check that refreshing fetches the new password
	err = e2.Refresh()
	c.Assert(err, IsNil)
	c.Assert(e2.PasswordValid("bar"), Equals, true)

	if le, ok := e.(lifer); ok {
		testWhenDying(c, le, noErr, notAliveErr, func() error {
			return e.SetPassword("arble")
		})
	}
}

type entity interface {
	lifer
	EntityName() string
	SetMongoPassword(password string) error
	SetPassword(password string) error
	PasswordValid(password string) bool
	Refresh() error
}

func testSetMongoPassword(c *C, getEntity func(st *state.State) (entity, error)) {
	info := state.TestingStateInfo()
	st, err := state.Open(info)
	c.Assert(err, IsNil)
	defer st.Close()
	// Turn on fully-authenticated mode.
	err = st.SetAdminMongoPassword("admin-secret")
	c.Assert(err, IsNil)

	// Set the password for the entity
	ent, err := getEntity(st)
	c.Assert(err, IsNil)
	err = ent.SetMongoPassword("foo")
	c.Assert(err, IsNil)

	// Check that we cannot log in with the wrong password.
	info.EntityName = ent.EntityName()
	info.Password = "bar"
	err = tryOpenState(info)
	c.Assert(state.IsUnauthorizedError(err), Equals, true)

	// Check that we can log in with the correct password.
	info.Password = "foo"
	st1, err := state.Open(info)
	c.Assert(err, IsNil)
	defer st1.Close()

	// Change the password with an entity derived from the newly
	// opened and authenticated state.
	ent, err = getEntity(st)
	c.Assert(err, IsNil)
	err = ent.SetMongoPassword("bar")
	c.Assert(err, IsNil)

	// Check that we cannot log in with the old password.
	info.Password = "foo"
	err = tryOpenState(info)
	c.Assert(state.IsUnauthorizedError(err), Equals, true)

	// Check that we can log in with the correct password.
	info.Password = "bar"
	err = tryOpenState(info)
	c.Assert(err, IsNil)

	// Check that the administrator can still log in.
	info.EntityName, info.Password = "", "admin-secret"
	err = tryOpenState(info)
	c.Assert(err, IsNil)

	// Remove the admin password so that the test harness can reset the state.
	err = st.SetAdminMongoPassword("")
	c.Assert(err, IsNil)
}

func (s *StateSuite) TestSetAdminMongoPassword(c *C) {
	// Check that we can SetAdminMongoPassword to nothing when there's
	// no password currently set.
	err := s.State.SetAdminMongoPassword("")
	c.Assert(err, IsNil)

	err = s.State.SetAdminMongoPassword("foo")
	c.Assert(err, IsNil)
	defer s.State.SetAdminMongoPassword("")
	info := state.TestingStateInfo()
	err = tryOpenState(info)
	c.Assert(state.IsUnauthorizedError(err), Equals, true)

	info.Password = "foo"
	err = tryOpenState(info)
	c.Assert(err, IsNil)

	err = s.State.SetAdminMongoPassword("")
	c.Assert(err, IsNil)

	// Check that removing the password is idempotent.
	err = s.State.SetAdminMongoPassword("")
	c.Assert(err, IsNil)

	info.Password = ""
	err = tryOpenState(info)
	c.Assert(err, IsNil)
}

func (s *StateSuite) TestEntity(c *C) {
	bad := []string{"", "machine", "-foo", "foo-", "---", "machine-jim", "unit-123", "unit-foo"}
	for _, name := range bad {
		e, err := s.State.Entity(name)
		c.Check(e, IsNil)
		c.Assert(err, ErrorMatches, `invalid entity name ".*"`)
	}

	e, err := s.State.Entity("machine-1234")
	c.Check(e, IsNil)
	c.Assert(err, ErrorMatches, `machine 1234 not found`)
	c.Assert(state.IsNotFound(err), Equals, true)

	e, err = s.State.Entity("unit-foo-654")
	c.Check(e, IsNil)
	c.Assert(err, ErrorMatches, `unit "foo/654" not found`)
	c.Assert(state.IsNotFound(err), Equals, true)

	e, err = s.State.Entity("unit-foo-bar-654")
	c.Check(e, IsNil)
	c.Assert(err, ErrorMatches, `unit "foo-bar/654" not found`)
	c.Assert(state.IsNotFound(err), Equals, true)

	e, err = s.State.Entity("user-arble")
	c.Check(e, IsNil)
	c.Assert(err, ErrorMatches, `user "arble" not found`)
	c.Assert(state.IsNotFound(err), Equals, true)

	m, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)

	e, err = s.State.Entity(m.EntityName())
	c.Assert(err, IsNil)
	c.Assert(e, FitsTypeOf, m)
	c.Assert(e.EntityName(), Equals, m.EntityName())

	svc, err := s.State.AddService("ser-vice1", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, IsNil)
	u, err := svc.AddUnit()
	c.Assert(err, IsNil)

	e, err = s.State.Entity(u.EntityName())
	c.Assert(err, IsNil)
	c.Assert(e, FitsTypeOf, u)
	c.Assert(e.EntityName(), Equals, u.EntityName())

	user, err := s.State.AddUser("arble", "pass")
	c.Assert(err, IsNil)

	e, err = s.State.Entity(user.EntityName())
	c.Assert(err, IsNil)
	c.Assert(e, FitsTypeOf, user)
	c.Assert(e.EntityName(), Equals, user.EntityName())
}
