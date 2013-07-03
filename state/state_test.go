// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	statetesting "launchpad.net/juju-core/state/testing"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/version"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type D []bson.DocElem

// preventUnitDestroyRemove sets a non-pending status on the unit, and hence
// prevents it from being unceremoniously removed from state on Destroy. This
// is useful because several tests go through a unit's lifecycle step by step,
// asserting the behaviour of a given method in each state, and the unit quick-
// remove change caused many of these to fail.
func preventUnitDestroyRemove(c *C, u *state.Unit) {
	err := u.SetStatus(params.StatusStarted, "")
	c.Assert(err, IsNil)
}

type StateSuite struct {
	ConnSuite
}

var _ = Suite(&StateSuite{})

func (s *StateSuite) TestDialAgain(c *C) {
	// Ensure idempotent operations on Dial are working fine.
	for i := 0; i < 2; i++ {
		st, err := state.Open(state.TestingStateInfo(), state.TestingDialOpts())
		c.Assert(err, IsNil)
		c.Assert(st.Close(), IsNil)
	}
}

func (s *StateSuite) TestStateInfo(c *C) {
	info := state.TestingStateInfo()
	stateAddr, err := s.State.Addresses()
	c.Assert(err, IsNil)
	c.Assert(stateAddr, DeepEquals, info.Addrs)
	c.Assert(s.State.CACert(), DeepEquals, info.CACert)
}

func (s *StateSuite) TestAPIAddresses(c *C) {
	config, err := s.State.EnvironConfig()
	c.Assert(err, IsNil)
	apiPort := strconv.Itoa(config.APIPort())
	info := state.TestingStateInfo()
	expectedAddrs := make([]string, 0, len(info.Addrs))
	for _, stateAddr := range info.Addrs {
		domain := strings.Split(stateAddr, ":")[0]
		expectedAddr := strings.Join([]string{domain, apiPort}, ":")
		expectedAddrs = append(expectedAddrs, expectedAddr)
	}
	apiAddrs, err := s.State.APIAddresses()
	c.Assert(err, IsNil)
	c.Assert(apiAddrs, DeepEquals, expectedAddrs)
}

func (s *StateSuite) TestIsNotFound(c *C) {
	err1 := fmt.Errorf("unrelated error")
	err2 := errors.NotFoundf("foo")
	c.Assert(err1, Not(checkers.Satisfies), errors.IsNotFoundError)
	c.Assert(err2, checkers.Satisfies, errors.IsNotFoundError)
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
	{state.JobManageState, "JobManageState"},
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
		s.assertMachineContainers(c, m, nil)
	}
	check(m0, "0", "series", oneJob)
	m0, err = s.State.Machine("0")
	c.Assert(err, IsNil)
	check(m0, "0", "series", oneJob)

	allJobs := []state.MachineJob{
		state.JobHostUnits,
		state.JobManageEnviron,
		state.JobManageState,
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

func (s *StateSuite) TestAddMachineExtraConstraints(c *C) {
	err := s.State.SetEnvironConstraints(constraints.MustParse("mem=4G"))
	c.Assert(err, IsNil)
	oneJob := []state.MachineJob{state.JobHostUnits}
	extraCons := constraints.MustParse("cpu-cores=4")
	params := state.AddMachineParams{
		Series:      "series",
		Constraints: extraCons,
		Jobs:        oneJob,
	}
	m, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, IsNil)
	c.Assert(m.Id(), Equals, "0")
	c.Assert(m.Series(), Equals, "series")
	c.Assert(m.Jobs(), DeepEquals, oneJob)
	expectedCons := constraints.MustParse("cpu-cores=4 mem=4G")
	mcons, err := m.Constraints()
	c.Assert(err, IsNil)
	c.Assert(mcons, DeepEquals, expectedCons)
}

var emptyCons = constraints.Value{}

func (s *StateSuite) assertMachineContainers(c *C, m *state.Machine, containers []string) {
	mc, err := m.Containers()
	c.Assert(err, IsNil)
	c.Assert(mc, DeepEquals, containers)
}

func (s *StateSuite) TestAddContainerToNewMachine(c *C) {
	oneJob := []state.MachineJob{state.JobHostUnits}

	params := state.AddMachineParams{
		ContainerType: instance.LXC,
		Series:        "series",
		Jobs:          oneJob,
	}
	m, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, IsNil)
	c.Assert(m.Id(), Equals, "0/lxc/0")
	c.Assert(m.Series(), Equals, "series")
	c.Assert(m.ContainerType(), Equals, instance.LXC)
	mcons, err := m.Constraints()
	c.Assert(err, IsNil)
	c.Assert(mcons, DeepEquals, emptyCons)
	c.Assert(m.Jobs(), DeepEquals, oneJob)

	m, err = s.State.Machine("0")
	c.Assert(err, IsNil)
	s.assertMachineContainers(c, m, []string{"0/lxc/0"})
	m, err = s.State.Machine("0/lxc/0")
	c.Assert(err, IsNil)
	s.assertMachineContainers(c, m, nil)
}

func (s *StateSuite) TestAddContainerToExistingMachine(c *C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	m0, err := s.State.AddMachine("series", oneJob...)
	c.Assert(err, IsNil)
	m1, err := s.State.AddMachine("series", oneJob...)
	c.Assert(err, IsNil)

	// Add first container.
	params := state.AddMachineParams{
		ParentId:      "1",
		ContainerType: instance.LXC,
		Series:        "series",
		Jobs:          []state.MachineJob{state.JobHostUnits},
	}
	m, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, IsNil)
	c.Assert(m.Id(), Equals, "1/lxc/0")
	c.Assert(m.Series(), Equals, "series")
	c.Assert(m.ContainerType(), Equals, instance.LXC)
	mcons, err := m.Constraints()
	c.Assert(err, IsNil)
	c.Assert(mcons, DeepEquals, emptyCons)
	c.Assert(m.Jobs(), DeepEquals, oneJob)
	s.assertMachineContainers(c, m1, []string{"1/lxc/0"})

	s.assertMachineContainers(c, m0, nil)
	s.assertMachineContainers(c, m1, []string{"1/lxc/0"})
	m, err = s.State.Machine("1/lxc/0")
	c.Assert(err, IsNil)
	s.assertMachineContainers(c, m, nil)

	// Add second container.
	m, err = s.State.AddMachineWithConstraints(&params)
	c.Assert(err, IsNil)
	c.Assert(m.Id(), Equals, "1/lxc/1")
	c.Assert(m.Series(), Equals, "series")
	c.Assert(m.ContainerType(), Equals, instance.LXC)
	c.Assert(m.Jobs(), DeepEquals, oneJob)
	s.assertMachineContainers(c, m1, []string{"1/lxc/0", "1/lxc/1"})
}

func (s *StateSuite) TestAddContainerWithConstraints(c *C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	cons := constraints.MustParse("mem=4G")

	params := state.AddMachineParams{
		ParentId:      "",
		ContainerType: instance.LXC,
		Series:        "series",
		Constraints:   cons,
		Jobs:          oneJob,
	}
	m, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, IsNil)
	c.Assert(m.Id(), Equals, "0/lxc/0")
	c.Assert(m.Series(), Equals, "series")
	c.Assert(m.ContainerType(), Equals, instance.LXC)
	c.Assert(m.Jobs(), DeepEquals, oneJob)
	mcons, err := m.Constraints()
	c.Assert(err, IsNil)
	c.Assert(cons, DeepEquals, mcons)
}

func (s *StateSuite) TestAddContainerErrors(c *C) {
	oneJob := []state.MachineJob{state.JobHostUnits}

	params := state.AddMachineParams{
		ParentId:      "10",
		ContainerType: instance.LXC,
		Series:        "series",
		Jobs:          oneJob,
	}
	_, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, ErrorMatches, "cannot add a new container: machine 10 not found")
	params.ContainerType = ""
	_, err = s.State.AddMachineWithConstraints(&params)
	c.Assert(err, ErrorMatches, "cannot add a new container: no container type specified")
}

func (s *StateSuite) TestInjectMachineErrors(c *C) {
	_, err := s.State.InjectMachine("", emptyCons, instance.Id("i-minvalid"), state.JobHostUnits)
	c.Assert(err, ErrorMatches, "cannot add a new machine: no series specified")
	_, err = s.State.InjectMachine("series", emptyCons, instance.Id(""), state.JobHostUnits)
	c.Assert(err, ErrorMatches, "cannot inject a machine without an instance id")
	_, err = s.State.InjectMachine("series", emptyCons, instance.Id("i-mlazy"))
	c.Assert(err, ErrorMatches, "cannot add a new machine: no jobs specified")
}

func (s *StateSuite) TestInjectMachine(c *C) {
	cons := constraints.MustParse("mem=4G")
	m, err := s.State.InjectMachine("series", cons, instance.Id("i-mindustrious"), state.JobHostUnits, state.JobManageEnviron)
	c.Assert(err, IsNil)
	c.Assert(m.Jobs(), DeepEquals, []state.MachineJob{state.JobHostUnits, state.JobManageEnviron})
	instanceId, err := m.InstanceId()
	c.Assert(err, IsNil)
	c.Assert(instanceId, Equals, instance.Id("i-mindustrious"))
	mcons, err := m.Constraints()
	c.Assert(err, IsNil)
	c.Assert(cons, DeepEquals, mcons)

	// Make sure the bootstrap nonce value is set.
	c.Assert(m.CheckProvisioned(state.BootstrapNonce), Equals, true)
}

func (s *StateSuite) TestAddContainerToInjectedMachine(c *C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	m0, err := s.State.InjectMachine("series", emptyCons, instance.Id("i-mindustrious"), state.JobHostUnits, state.JobManageEnviron)
	c.Assert(err, IsNil)

	// Add first container.
	params := state.AddMachineParams{
		ParentId:      "0",
		ContainerType: instance.LXC,
		Series:        "series",
		Jobs:          []state.MachineJob{state.JobHostUnits},
	}
	m, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, IsNil)
	c.Assert(m.Id(), Equals, "0/lxc/0")
	c.Assert(m.Series(), Equals, "series")
	c.Assert(m.ContainerType(), Equals, instance.LXC)
	mcons, err := m.Constraints()
	c.Assert(err, IsNil)
	c.Assert(mcons, DeepEquals, emptyCons)
	c.Assert(m.Jobs(), DeepEquals, oneJob)
	s.assertMachineContainers(c, m0, []string{"0/lxc/0"})

	// Add second container.
	m, err = s.State.AddMachineWithConstraints(&params)
	c.Assert(err, IsNil)
	c.Assert(m.Id(), Equals, "0/lxc/1")
	c.Assert(m.Series(), Equals, "series")
	c.Assert(m.ContainerType(), Equals, instance.LXC)
	c.Assert(m.Jobs(), DeepEquals, oneJob)
	s.assertMachineContainers(c, m0, []string{"0/lxc/0", "0/lxc/1"})
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
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
}

func (s *StateSuite) TestMachineIdLessThan(c *C) {
	c.Assert(state.MachineIdLessThan("0", "0"), Equals, false)
	c.Assert(state.MachineIdLessThan("0", "1"), Equals, true)
	c.Assert(state.MachineIdLessThan("1", "0"), Equals, false)
	c.Assert(state.MachineIdLessThan("10", "2"), Equals, false)
	c.Assert(state.MachineIdLessThan("0", "0/lxc/0"), Equals, true)
	c.Assert(state.MachineIdLessThan("0/lxc/0", "0"), Equals, false)
	c.Assert(state.MachineIdLessThan("1", "0/lxc/0"), Equals, false)
	c.Assert(state.MachineIdLessThan("0/lxc/0", "1"), Equals, true)
	c.Assert(state.MachineIdLessThan("0/lxc/0/lxc/1", "0/lxc/0"), Equals, false)
	c.Assert(state.MachineIdLessThan("0/kvm/0", "0/lxc/0"), Equals, true)
}

func (s *StateSuite) TestAllMachines(c *C) {
	numInserts := 42
	for i := 0; i < numInserts; i++ {
		m, err := s.State.AddMachine("series", state.JobHostUnits)
		c.Assert(err, IsNil)
		err = m.SetProvisioned(instance.Id(fmt.Sprintf("foo-%d", i)), "fake_nonce", nil)
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
		instId, err := m.InstanceId()
		c.Assert(err, IsNil)
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
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
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
	cons0 := emptyCons
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

func (s *StateSuite) TestWatchServicesBulkEvents(c *C) {
	// Alive service...
	dummyCharm := s.AddTestingCharm(c, "dummy")
	alive, err := s.State.AddService("service0", dummyCharm)
	c.Assert(err, IsNil)

	// Dying service...
	dying, err := s.State.AddService("service1", dummyCharm)
	c.Assert(err, IsNil)
	keepDying, err := dying.AddUnit()
	c.Assert(err, IsNil)
	err = dying.Destroy()
	c.Assert(err, IsNil)

	// Dead service (actually, gone, Dead == removed in this case).
	gone, err := s.State.AddService("service2", dummyCharm)
	c.Assert(err, IsNil)
	err = gone.Destroy()
	c.Assert(err, IsNil)

	// All except gone are reported in initial event.
	w := s.State.WatchServices()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.StringsWatcherC{c, s.State, w}
	wc.AssertOneChange(alive.Name(), dying.Name())

	// Remove them all; alive/dying changes reported.
	err = alive.Destroy()
	c.Assert(err, IsNil)
	err = keepDying.Destroy()
	c.Assert(err, IsNil)
	wc.AssertOneChange(alive.Name(), dying.Name())
}

func (s *StateSuite) TestWatchServicesLifecycle(c *C) {
	// Initial event is empty when no services.
	w := s.State.WatchServices()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.StringsWatcherC{c, s.State, w}
	wc.AssertOneChange()

	// Add a service: reported.
	service, err := s.State.AddService("service", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, IsNil)
	wc.AssertOneChange("service")

	// Change the service: not reported.
	keepDying, err := service.AddUnit()
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Make it Dying: reported.
	err = service.Destroy()
	c.Assert(err, IsNil)
	wc.AssertOneChange("service")

	// Make it Dead(/removed): reported.
	err = keepDying.Destroy()
	c.Assert(err, IsNil)
	wc.AssertOneChange("service")
}

func (s *StateSuite) TestWatchMachinesBulkEvents(c *C) {
	// Alive machine...
	alive, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)

	// Dying machine...
	dying, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = dying.SetProvisioned(instance.Id("i-blah"), "fake-nonce", nil)
	c.Assert(err, IsNil)
	err = dying.Destroy()
	c.Assert(err, IsNil)

	// Dead machine...
	dead, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = dead.EnsureDead()
	c.Assert(err, IsNil)

	// Gone machine.
	gone, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = gone.EnsureDead()
	c.Assert(err, IsNil)
	err = gone.Remove()
	c.Assert(err, IsNil)

	// All except gone machine are reported in initial event.
	w := s.State.WatchEnvironMachines()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.StringsWatcherC{c, s.State, w}
	wc.AssertOneChange(alive.Id(), dying.Id(), dead.Id())

	// Remove them all; alive/dying changes reported; dead never mentioned again.
	err = alive.Destroy()
	c.Assert(err, IsNil)
	err = dying.EnsureDead()
	c.Assert(err, IsNil)
	err = dying.Remove()
	c.Assert(err, IsNil)
	err = dead.Remove()
	c.Assert(err, IsNil)
	wc.AssertOneChange(alive.Id(), dying.Id())
}

func (s *StateSuite) TestWatchMachinesLifecycle(c *C) {
	// Initial event is empty when no machines.
	w := s.State.WatchEnvironMachines()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.StringsWatcherC{c, s.State, w}
	wc.AssertOneChange()

	// Add a machine: reported.
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	wc.AssertOneChange("0")

	// Change the machine: not reported.
	err = machine.SetProvisioned(instance.Id("i-blah"), "fake-nonce", nil)
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Make it Dying: reported.
	err = machine.Destroy()
	c.Assert(err, IsNil)
	wc.AssertOneChange("0")

	// Make it Dead: reported.
	err = machine.EnsureDead()
	c.Assert(err, IsNil)
	wc.AssertOneChange("0")

	// Remove it: not reported.
	err = machine.Remove()
	c.Assert(err, IsNil)
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchMachinesLifecycleIgnoresContainers(c *C) {
	// Initial event is empty when no machines.
	w := s.State.WatchEnvironMachines()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.StringsWatcherC{c, s.State, w}
	wc.AssertOneChange()

	// Add a machine: reported.
	params := state.AddMachineParams{
		Series: "series",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	machine, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, IsNil)
	wc.AssertOneChange("0")

	// Add a container: not reported.
	params.ParentId = machine.Id()
	params.ContainerType = instance.LXC
	m, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Make the container Dying: not reported.
	err = m.Destroy()
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Make the container Dead: not reported.
	err = m.EnsureDead()
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Remove the container: not reported.
	err = m.Remove()
	c.Assert(err, IsNil)
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchContainerLifecycle(c *C) {
	// Add a host machine.
	params := state.AddMachineParams{
		Series: "series",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	machine, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, IsNil)

	otherMachine, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, IsNil)

	// Initial event is empty when no containers.
	w := machine.WatchContainers(instance.LXC)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.StringsWatcherC{c, s.State, w}
	wc.AssertOneChange()

	// Add a container of the required type: reported.
	params.ParentId = machine.Id()
	params.ContainerType = instance.LXC
	m, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, IsNil)
	wc.AssertOneChange("0/lxc/0")

	// Add a container of a different type: not reported.
	params.ContainerType = instance.KVM
	m1, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Add a container of a different machine: not reported.
	params.ParentId = otherMachine.Id()
	params.ContainerType = instance.LXC
	m2, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Make the container Dying: reported.
	err = m.Destroy()
	c.Assert(err, IsNil)
	wc.AssertOneChange("0/lxc/0")

	// Make the other containers Dying: not reported.
	err = m1.Destroy()
	c.Assert(err, IsNil)
	err = m2.Destroy()
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Make the container Dead: reported.
	err = m.EnsureDead()
	c.Assert(err, IsNil)
	wc.AssertOneChange("0/lxc/0")

	// Make the other containers Dead: not reported.
	err = m1.EnsureDead()
	c.Assert(err, IsNil)
	err = m2.EnsureDead()
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Remove the container: not reported.
	err = m.Remove()
	c.Assert(err, IsNil)
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchMachineHardwareCharacteristics(c *C) {
	// Add a machine: reported.
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	w, err := machine.WatchHardwareCharacteristics()
	c.Assert(err, IsNil)
	defer statetesting.AssertStop(c, w)

	// Initial event.
	wc := statetesting.NotifyWatcherC{c, s.State, w}
	wc.AssertOneChange()

	// Provision a machine: reported.
	err = machine.SetProvisioned(instance.Id("i-blah"), "fake-nonce", nil)
	c.Assert(err, IsNil)
	wc.AssertOneChange()

	// Alter the machine: not reported.
	tools := &state.Tools{
		Binary: version.Binary{
			Number: version.MustParse("1.2.3"),
			Series: "gutsy",
			Arch:   "ppc",
		},
		URL: "http://canonical.com/",
	}
	err = machine.SetAgentTools(tools)
	c.Assert(err, IsNil)
	wc.AssertNoChange()
}

var sortPortsTests = []struct {
	have, want []instance.Port
}{
	{nil, []instance.Port{}},
	{[]instance.Port{{"b", 1}, {"a", 99}, {"a", 1}}, []instance.Port{{"a", 1}, {"a", 99}, {"b", 1}}},
}

func (*StateSuite) TestSortPorts(c *C) {
	for _, t := range sortPortsTests {
		p := make([]instance.Port, len(t.have))
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
	assertMachine("00", false)
	assertMachine("1", true)
	assertMachine("1000001", true)
	assertMachine("01", false)
	assertMachine("-1", false)
	assertMachine("", false)
	assertMachine("cantankerous", false)
	// And container specs
	assertMachine("0/", false)
	assertMachine("0/0", false)
	assertMachine("0/lxc", false)
	assertMachine("0/lxc/", false)
	assertMachine("0/lxc/0", true)
	assertMachine("0/lxc/0/", false)
	assertMachine("0/lxc/00", false)
	assertMachine("0/lxc/01", false)
	assertMachine("0/lxc/10", true)
	assertMachine("0/kvm/4", true)
	assertMachine("0/no-dash/0", false)
	assertMachine("0/lxc/1/embedded/2", true)
}

type attrs map[string]interface{}

func (s *StateSuite) TestWatchEnvironConfig(c *C) {
	w := s.State.WatchEnvironConfig()
	defer statetesting.AssertStop(c, w)

	// TODO(fwereade) just use a NotifyWatcher and NotifyWatcherC to test it.
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
	st, err := state.Open(info, state.TestingDialOpts())
	if err == nil {
		st.Close()
	}
	return err
}

func (s *StateSuite) TestOpenWithoutSetMongoPassword(c *C) {
	info := state.TestingStateInfo()
	info.Tag, info.Password = "arble", "bar"
	err := tryOpenState(info)
	c.Assert(err, checkers.Satisfies, errors.IsUnauthorizedError)

	info.Tag, info.Password = "arble", ""
	err = tryOpenState(info)
	c.Assert(err, checkers.Satisfies, errors.IsUnauthorizedError)

	info.Tag, info.Password = "", ""
	err = tryOpenState(info)
	c.Assert(err, IsNil)
}

func (s *StateSuite) TestOpenBadAddress(c *C) {
	info := state.TestingStateInfo()
	info.Addrs = []string{"0.1.2.3:1234"}
	st, err := state.Open(info, state.DialOpts{
		Timeout: 1 * time.Millisecond,
	})
	if err == nil {
		st.Close()
	}
	c.Assert(err, ErrorMatches, "no reachable servers")
}

func (s *StateSuite) TestOpenDelaysRetryBadAddress(c *C) {
	// Default mgo retry delay
	retryDelay := 500 * time.Millisecond
	info := state.TestingStateInfo()
	info.Addrs = []string{"0.1.2.3:1234"}

	t0 := time.Now()
	st, err := state.Open(info, state.DialOpts{
		Timeout: 1 * time.Millisecond,
	})
	if err == nil {
		st.Close()
	}
	c.Assert(err, ErrorMatches, "no reachable servers")
	// tryOpenState should have delayed for at least retryDelay
	// internally mgo will try three times in a row before returning
	// to the caller.
	if t1 := time.Since(t0); t1 < 3*retryDelay {
		c.Errorf("mgo.Dial only paused for %v, expected at least %v", t1, 3*retryDelay)
	}
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
	st, err := state.Open(info, state.TestingDialOpts())
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
	c.Assert(err, checkers.Satisfies, errors.IsUnauthorizedError)

	// Check that we can log in with the correct password.
	info.Password = "foo"
	st1, err := state.Open(info, state.TestingDialOpts())
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
	c.Assert(err, checkers.Satisfies, errors.IsUnauthorizedError)

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
	c.Assert(err, checkers.Satisfies, errors.IsUnauthorizedError)

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
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)

	e, err = getEntity("unit-foo-654")
	c.Check(e, IsNil)
	c.Assert(err, ErrorMatches, `unit "foo/654" not found`)
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)

	e, err = getEntity("unit-foo-bar-654")
	c.Check(e, IsNil)
	c.Assert(err, ErrorMatches, `unit "foo-bar/654" not found`)
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)

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
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)

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

func (s *StateSuite) TestCleanup(c *C) {
	needed, err := s.State.NeedsCleanup()
	c.Assert(err, IsNil)
	c.Assert(needed, Equals, false)

	_, err = s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	_, err = s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, IsNil)
	relM, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)

	needed, err = s.State.NeedsCleanup()
	c.Assert(err, IsNil)
	c.Assert(needed, Equals, false)

	err = relM.Destroy()
	c.Assert(err, IsNil)

	needed, err = s.State.NeedsCleanup()
	c.Assert(err, IsNil)
	c.Assert(needed, Equals, true)

	err = s.State.Cleanup()
	c.Assert(err, IsNil)

	needed, err = s.State.NeedsCleanup()
	c.Assert(err, IsNil)
	c.Assert(needed, Equals, false)
}

func (s *StateSuite) TestWatchCleanups(c *C) {
	// Check initial event.
	w := s.State.WatchCleanups()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NotifyWatcherC{c, s.State, w}
	wc.AssertOneChange()

	// Set up two relations for later use, check no events.
	_, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	_, err = s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, IsNil)
	relM, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)
	_, err = s.State.AddService("varnish", s.AddTestingCharm(c, "varnish"))
	c.Assert(err, IsNil)
	eps, err = s.State.InferEndpoints([]string{"wordpress", "varnish"})
	c.Assert(err, IsNil)
	relV, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Destroy one relation, check one change.
	err = relM.Destroy()
	c.Assert(err, IsNil)
	wc.AssertOneChange()

	// Handle that cleanup doc and create another, check one change.
	err = s.State.Cleanup()
	c.Assert(err, IsNil)
	err = relV.Destroy()
	c.Assert(err, IsNil)
	wc.AssertOneChange()

	// Clean up final doc, check change.
	err = s.State.Cleanup()
	c.Assert(err, IsNil)
	wc.AssertOneChange()

	// Stop watcher, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchCleanupsBulk(c *C) {
	// Check initial event.
	w := s.State.WatchCleanups()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NotifyWatcherC{c, s.State, w}
	wc.AssertOneChange()

	// Create two peer relations by creating their services.
	riak, err := s.State.AddService("riak", s.AddTestingCharm(c, "riak"))
	c.Assert(err, IsNil)
	_, err = riak.Endpoint("ring")
	c.Assert(err, IsNil)
	allHooks, err := s.State.AddService("all-hooks", s.AddTestingCharm(c, "all-hooks"))
	c.Assert(err, IsNil)
	_, err = allHooks.Endpoint("self")
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Destroy them both, check one change.
	err = riak.Destroy()
	c.Assert(err, IsNil)
	err = allHooks.Destroy()
	c.Assert(err, IsNil)
	wc.AssertOneChange()

	// Clean them both up, check one change.
	err = s.State.Cleanup()
	c.Assert(err, IsNil)
	wc.AssertOneChange()
}

func (s *StateSuite) TestWatchMinUnits(c *C) {
	// Check initial event.
	w := s.State.WatchMinUnits()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.StringsWatcherC{c, s.State, w}
	wc.AssertOneChange()

	// Set up services for later use.
	wordpress, err := s.State.AddService(
		"wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	wordpressName := wordpress.Name()

	// Add service units for later use.
	wordpress0, err := wordpress.AddUnit()
	c.Assert(err, IsNil)
	wordpress1, err := wordpress.AddUnit()
	c.Assert(err, IsNil)
	mysql0, err := mysql.AddUnit()
	c.Assert(err, IsNil)
	// No events should occur.
	wc.AssertNoChange()

	// Add minimum units to a service; a single change should occur.
	err = wordpress.SetMinUnits(2)
	c.Assert(err, IsNil)
	wc.AssertOneChange(wordpressName)

	// Decrease minimum units for a service; expect no changes.
	err = wordpress.SetMinUnits(1)
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Increase minimum units for two services; a single change should occur.
	err = mysql.SetMinUnits(1)
	c.Assert(err, IsNil)
	err = wordpress.SetMinUnits(3)
	c.Assert(err, IsNil)
	wc.AssertOneChange(mysql.Name(), wordpressName)

	// Remove minimum units for a service; expect no changes.
	err = mysql.SetMinUnits(0)
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Destroy a unit of a service with required minimum units.
	// Also avoid the unit removal. A single change should occur.
	preventUnitDestroyRemove(c, wordpress0)
	err = wordpress0.Destroy()
	c.Assert(err, IsNil)
	wc.AssertOneChange(wordpressName)

	// Two actions: destroy a unit and increase minimum units for a service.
	// A single change should occur, and the service name should appear only
	// one time in the change.
	err = wordpress.SetMinUnits(5)
	c.Assert(err, IsNil)
	err = wordpress1.Destroy()
	c.Assert(err, IsNil)
	wc.AssertOneChange(wordpressName)

	// Destroy a unit of a service not requiring minimum units; expect no changes.
	err = mysql0.Destroy()
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Destroy a service with required minimum units; expect no changes.
	err = wordpress.Destroy()
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Destroy a service not requiring minimum units; expect no changes.
	err = mysql.Destroy()
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Stop watcher, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestNestingLevel(c *C) {
	c.Assert(state.NestingLevel("0"), Equals, 0)
	c.Assert(state.NestingLevel("0/lxc/1"), Equals, 1)
	c.Assert(state.NestingLevel("0/lxc/1/kvm/0"), Equals, 2)
}

func (s *StateSuite) TestTopParentId(c *C) {
	c.Assert(state.TopParentId("0"), Equals, "0")
	c.Assert(state.TopParentId("0/lxc/1"), Equals, "0")
	c.Assert(state.TopParentId("0/lxc/1/kvm/2"), Equals, "0")
}

func (s *StateSuite) TestParentId(c *C) {
	c.Assert(state.ParentId("0"), Equals, "")
	c.Assert(state.ParentId("0/lxc/1"), Equals, "0")
	c.Assert(state.ParentId("0/lxc/1/kvm/0"), Equals, "0/lxc/1")
}
