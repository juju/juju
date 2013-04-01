package state_test

import (
	"fmt"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/constraints"
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
		st, err := state.Open(state.TestingStateInfo(), state.TestingDialTimeout)
		c.Assert(err, IsNil)
		c.Assert(st.Close(), IsNil)
	}
}

func (s *StateSuite) TestStateInfo(c *C) {
	info := state.TestingStateInfo()
	c.Assert(s.State.Addresses(), DeepEquals, info.Addrs)
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

	// set that a nil charm is handled correctly
	_, err = s.State.AddService("umadbro", nil)
	c.Assert(err, ErrorMatches, `cannot add service "umadbro": charm is nil`)

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
			ServiceName: "rk1",
			Relation: charm.Relation{
				Name:      "ring",
				Interface: "riak",
				Limit:     1,
				Role:      charm.RolePeer,
				Scope:     charm.ScopeGlobal,
			},
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
			ServiceName: "ms",
			Relation: charm.Relation{
				Interface: "mysql",
				Name:      "dev",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeGlobal,
				Limit:     2,
			},
		}, {
			ServiceName: "wp",
			Relation: charm.Relation{
				Interface: "mysql",
				Name:      "db",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeGlobal,
				Limit:     1,
			},
		}},
	}, {
		summary: "explicit logging relation is preferred over implicit juju-info",
		inputs:  [][]string{{"lg", "wp"}},
		eps: []state.Endpoint{{
			ServiceName: "lg",
			Relation: charm.Relation{
				Interface: "logging",
				Name:      "logging-directory",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeContainer,
				Limit:     1,
			},
		}, {
			ServiceName: "wp",
			Relation: charm.Relation{
				Interface: "logging",
				Name:      "logging-dir",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeContainer,
			},
		}},
	}, {
		summary: "implict relations can be chosen explicitly",
		inputs: [][]string{
			{"lg:info", "wp"},
			{"lg", "wp:juju-info"},
			{"lg:info", "wp:juju-info"},
		},
		eps: []state.Endpoint{{
			ServiceName: "lg",
			Relation: charm.Relation{
				Interface: "juju-info",
				Name:      "info",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeContainer,
				Limit:     1,
			},
		}, {
			ServiceName: "wp",
			Relation: charm.Relation{
				Interface: "juju-info",
				Name:      "juju-info",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeGlobal,
			},
		}},
	}, {
		summary: "implicit relations will be chosen if there are no other options",
		inputs:  [][]string{{"lg", "ms"}},
		eps: []state.Endpoint{{
			ServiceName: "lg",
			Relation: charm.Relation{
				Interface: "juju-info",
				Name:      "info",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeContainer,
				Limit:     1,
			},
		}, {
			ServiceName: "ms",
			Relation: charm.Relation{
				Interface: "juju-info",
				Name:      "juju-info",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeGlobal,
			},
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
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, IsNil)
	change, err := cfg.Apply(map[string]interface{}{
		"authorized-keys": "different-keys",
		"arbitrary-key":   "shazam!",
	})
	c.Assert(err, IsNil)
	err = s.State.SetEnvironConfig(change)
	c.Assert(err, IsNil)
	cfg, err = s.State.EnvironConfig()
	c.Assert(err, IsNil)
	c.Assert(cfg.AllAttrs(), DeepEquals, change.AllAttrs())
}

func (s *StateSuite) TestEnvironConstraints(c *C) {
	// Environ constraints start out empty (for now).
	cons0 := constraints.Value{}
	cons1, err := s.State.EnvironConstraints()
	c.Assert(err, IsNil)
	c.Assert(cons1, DeepEquals, cons0)

	// Environ constraints can be set.
	cons2 := constraints.Value{Mem: uint64p(1024)}
	err = s.State.SetEnvironConstraints(cons2)
	c.Assert(err, IsNil)
	cons3, err := s.State.EnvironConstraints()
	c.Assert(err, IsNil)
	c.Assert(cons3, DeepEquals, cons2)
	c.Assert(cons3, Not(Equals), cons2)

	// Environ constraints are completely overwritten when re-set.
	cons4 := constraints.Value{CpuPower: uint64p(250)}
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
		// Check that anything that is considered a valid service name
		// is also (in)valid if a(n) (in)valid unit designator is added
		// to it.
		c.Assert(state.IsUnitName(s+"/0"), Equals, expect)
		c.Assert(state.IsUnitName(s+"/99"), Equals, expect)
		c.Assert(state.IsUnitName(s+"/-1"), Equals, false)
		c.Assert(state.IsUnitName(s+"/blah"), Equals, false)
		c.Assert(state.IsUnitName(s+"/"), Equals, false)
	}
	// Service names must be non-empty...
	assertService("", false)
	// must not consist entirely of numbers
	assertService("33", false)
	// may consist of a single word
	assertService("wordpress", true)
	// may contain hyphen-seperated words...
	assertService("super-duper-wordpress", true)
	// ...but those words must have at least one letter in them
	assertService("super-1234-wordpress", false)
	// may contain internal numbers.
	assertService("w0rd-pre55", true)
	// must not begin with a number
	assertService("3wordpress", false)
	// but internal, hyphen-sperated words can begin with numbers
	assertService("foo-2foo", true)
	// and may end with a number...
	assertService("foo2", true)
	// ...unless that number is all by itself
	assertService("foo-2", false)

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

func (s *StateSuite) TestWatchEnvironConfig(c *C) {
	w := s.State.WatchEnvironConfig()
	defer stop(c, w)

	assertNoChange := func() {
		s.State.StartSync()
		select {
		case got := <-w.Changes():
			c.Fatalf("got unexpected change: %#v", got)
		case <-time.After(50 * time.Millisecond):
		}
	}
	assertChange := func(change attrs) {
		cfg, err := s.State.EnvironConfig()
		c.Assert(err, IsNil)
		if change != nil {
			cfg, err = cfg.Apply(change)
			c.Assert(err, IsNil)
			err = s.State.SetEnvironConfig(cfg)
			c.Assert(err, IsNil)
		}
		s.State.Sync()
		select {
		case got, ok := <-w.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got.AllAttrs(), DeepEquals, cfg.AllAttrs())
		case <-time.After(500 * time.Millisecond):
			c.Fatalf("did not get change: %#v", change)
		}
		assertNoChange()
	}
	assertChange(nil)
	assertChange(attrs{"default-series": "another-series"})
	assertChange(attrs{"fancy-new-key": "arbitrary-value"})
}

func (s *StateSuite) TestWatchEnvironConfigCorruptConfig(c *C) {
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, IsNil)

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
	err = s.State.SetEnvironConfig(cfg)
	c.Assert(err, IsNil)
	fixed := cfg.AllAttrs()

	s.State.StartSync()
	select {
	case got := <-done:
		c.Assert(got.AllAttrs(), DeepEquals, fixed)
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
	st, err := state.Open(info, state.TestingDialTimeout)
	if err == nil {
		st.Close()
	}
	return err
}

func (s *StateSuite) TestOpenWithoutSetMongoPassword(c *C) {
	info := state.TestingStateInfo()
	info.Tag, info.Password = "arble", "bar"
	err := tryOpenState(info)
	c.Assert(state.IsUnauthorizedError(err), Equals, true)

	info.Tag, info.Password = "arble", ""
	err = tryOpenState(info)
	c.Assert(state.IsUnauthorizedError(err), Equals, true)

	info.Tag, info.Password = "", ""
	err = tryOpenState(info)
	c.Assert(err, IsNil)
}

func (s *StateSuite) TestOpenBadAddress(c *C) {
	info := state.TestingStateInfo()
	info.Addrs = []string{"0.1.2.3:1234"}
	st, err := state.Open(info, 1*time.Millisecond)
	if err == nil {
		st.Close()
	}
	c.Assert(err, ErrorMatches, "no reachable servers")
}

func testSetPassword(c *C, getEntity func() (state.Authenticator, error)) {
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
		testWhenDying(c, le, noErr, deadErr, func() error {
			return e.SetPassword("arble")
		})
	}
}

type entity interface {
	lifer
	state.TaggedAuthenticator
	SetMongoPassword(password string) error
}

func testSetMongoPassword(c *C, getEntity func(st *state.State) (entity, error)) {
	info := state.TestingStateInfo()
	st, err := state.Open(info, state.TestingDialTimeout)
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
	info.Tag = ent.Tag()
	info.Password = "bar"
	err = tryOpenState(info)
	c.Assert(state.IsUnauthorizedError(err), Equals, true)

	// Check that we can log in with the correct password.
	info.Password = "foo"
	st1, err := state.Open(info, state.TestingDialTimeout)
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
	info.Tag, info.Password = "", "admin-secret"
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

func (s *StateSuite) testEntity(c *C, getEntity func(string) (state.Tagger, error)) {
	bad := []string{"", "machine", "-foo", "foo-", "---", "machine-jim", "unit-123", "unit-foo", "service-", "service-foo/bar", "environment-foo"}
	for _, name := range bad {
		c.Logf(name)
		e, err := getEntity(name)
		c.Check(e, IsNil)
		c.Assert(err, ErrorMatches, `invalid entity tag ".*"`)
	}

	e, err := getEntity("machine-1234")
	c.Check(e, IsNil)
	c.Assert(err, ErrorMatches, `machine 1234 not found`)
	c.Assert(state.IsNotFound(err), Equals, true)

	e, err = getEntity("unit-foo-654")
	c.Check(e, IsNil)
	c.Assert(err, ErrorMatches, `unit "foo/654" not found`)
	c.Assert(state.IsNotFound(err), Equals, true)

	e, err = getEntity("unit-foo-bar-654")
	c.Check(e, IsNil)
	c.Assert(err, ErrorMatches, `unit "foo-bar/654" not found`)
	c.Assert(state.IsNotFound(err), Equals, true)

	m, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)

	e, err = getEntity(m.Tag())
	c.Assert(err, IsNil)
	c.Assert(e, FitsTypeOf, m)
	c.Assert(e.Tag(), Equals, m.Tag())

	svc, err := s.State.AddService("ser-vice2", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	u, err := svc.AddUnit()
	c.Assert(err, IsNil)

	e, err = getEntity(u.Tag())
	c.Assert(err, IsNil)
	c.Assert(e, FitsTypeOf, u)
	c.Assert(e.Tag(), Equals, u.Tag())

	m.Destroy()
	svc.Destroy()
}

func (s *StateSuite) TestAuthenticator(c *C) {
	getEntity := func(name string) (state.Tagger, error) {
		e, err := s.State.Authenticator(name)
		if err != nil {
			return nil, err
		}
		return e, nil
	}
	s.testEntity(c, getEntity)
	e, err := getEntity("user-arble")
	c.Check(e, IsNil)
	c.Assert(err, ErrorMatches, `user "arble" not found`)
	c.Assert(state.IsNotFound(err), Equals, true)

	user, err := s.State.AddUser("arble", "pass")
	c.Assert(err, IsNil)

	e, err = getEntity(user.Tag())
	c.Assert(err, IsNil)
	c.Assert(e, FitsTypeOf, user)
	c.Assert(e.Tag(), Equals, user.Tag())

	cfg, err := s.State.EnvironConfig()
	c.Assert(err, IsNil)
	_, err = getEntity("environment-" + cfg.Name())
	c.Assert(
		err,
		ErrorMatches,
		`entity "environment-.*" does not support authentication`,
	)
}

func (s *StateSuite) TestAnnotator(c *C) {
	getEntity := func(name string) (state.Tagger, error) {
		e, err := s.State.Annotator(name)
		if err != nil {
			return nil, err
		}
		return e, nil
	}
	s.testEntity(c, getEntity)
	svc, err := s.State.AddService("ser-vice1", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, IsNil)

	service, err := getEntity(svc.Tag())
	c.Assert(err, IsNil)
	c.Assert(service, FitsTypeOf, svc)
	c.Assert(service.Tag(), Equals, svc.Tag())

	cfg, err := s.State.EnvironConfig()
	c.Assert(err, IsNil)
	e, err := getEntity("environment-" + cfg.Name())
	c.Assert(err, IsNil)
	env, err := s.State.Environment()
	c.Assert(err, IsNil)
	c.Assert(e, FitsTypeOf, env)
	c.Assert(e.Tag(), Equals, env.Tag())

	user, err := s.State.AddUser("arble", "pass")
	c.Assert(err, IsNil)
	_, err = getEntity(user.Tag())
	c.Assert(
		err,
		ErrorMatches,
		`entity "user-arble" does not support annotations`,
	)
}

func (s *StateSuite) TestParseTag(c *C) {
	bad := []string{
		"",
		"machine",
		"-foo",
		"foo-",
		"---",
		"foo-bar",
		"environment-foo",
		"unit-foo",
	}
	for _, name := range bad {
		c.Logf(name)
		coll, id, err := s.State.ParseTag(name)
		c.Check(coll, Equals, "")
		c.Check(id, Equals, "")
		c.Assert(err, ErrorMatches, `invalid entity name ".*"`)
	}

	// Parse a machine entity name.
	m, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	coll, id, err := s.State.ParseTag(m.Tag())
	c.Assert(coll, Equals, "machines")
	c.Assert(id, Equals, m.Id())
	c.Assert(err, IsNil)

	// Parse a service entity name.
	svc, err := s.State.AddService("ser-vice2", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, IsNil)
	coll, id, err = s.State.ParseTag(svc.Tag())
	c.Assert(coll, Equals, "services")
	c.Assert(id, Equals, svc.Name())
	c.Assert(err, IsNil)

	// Parse a unit entity name.
	u, err := svc.AddUnit()
	c.Assert(err, IsNil)
	coll, id, err = s.State.ParseTag(u.Tag())
	c.Assert(coll, Equals, "units")
	c.Assert(id, Equals, u.Name())
	c.Assert(err, IsNil)

	// Parse a user entity name.
	user, err := s.State.AddUser("arble", "pass")
	c.Assert(err, IsNil)
	coll, id, err = s.State.ParseTag(user.Tag())
	c.Assert(coll, Equals, "users")
	c.Assert(id, Equals, user.Name())
	c.Assert(err, IsNil)
}
