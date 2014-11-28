// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/presence"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

func runStatus(c *gc.C, args ...string) (code int, stdout, stderr []byte) {
	ctx := coretesting.Context(c)
	code = cmd.Main(envcmd.Wrap(&StatusCommand{}), ctx, args)
	stdout = ctx.Stdout.(*bytes.Buffer).Bytes()
	stderr = ctx.Stderr.(*bytes.Buffer).Bytes()
	return
}

type StatusSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&StatusSuite{})

type M map[string]interface{}

type L []interface{}

type testCase struct {
	summary string
	steps   []stepper
}

func test(summary string, steps ...stepper) testCase {
	return testCase{summary, steps}
}

type stepper interface {
	step(c *gc.C, ctx *context)
}

//
// context
//

func newContext(c *gc.C, st *state.State, env environs.Environ, adminUserTag string) *context {
	// We make changes in the API server's state so that
	// our changes to presence are immediately noticed
	// in the status.
	return &context{
		st:           st,
		env:          env,
		charms:       make(map[string]*state.Charm),
		pingers:      make(map[string]*presence.Pinger),
		adminUserTag: adminUserTag,
	}
}

type context struct {
	st           *state.State
	env          environs.Environ
	charms       map[string]*state.Charm
	pingers      map[string]*presence.Pinger
	adminUserTag string // A string repr of the tag.
}

func (ctx *context) reset(c *gc.C) {
	for _, up := range ctx.pingers {
		err := up.Kill()
		c.Check(err, jc.ErrorIsNil)
	}
}

func (ctx *context) run(c *gc.C, steps []stepper) {
	for i, s := range steps {
		c.Logf("step %d", i)
		c.Logf("%#v", s)
		s.step(c, ctx)
	}
}

func (ctx *context) setAgentPresence(c *gc.C, p presence.Presencer) *presence.Pinger {
	pinger, err := p.SetAgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	ctx.st.StartSync()
	err = p.WaitAgentPresence(coretesting.LongWait)
	c.Assert(err, jc.ErrorIsNil)
	agentPresence, err := p.AgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(agentPresence, jc.IsTrue)
	return pinger
}

func (s *StatusSuite) newContext(c *gc.C) *context {
	st := s.Environ.(testing.GetStater).GetStateInAPIServer()
	// We make changes in the API server's state so that
	// our changes to presence are immediately noticed
	// in the status.
	return newContext(c, st, s.Environ, s.AdminUserTag(c).String())
}

func (s *StatusSuite) resetContext(c *gc.C, ctx *context) {
	ctx.reset(c)
	s.JujuConnSuite.Reset(c)
}

// shortcuts for expected output.
var (
	machine0 = M{
		"agent-state":                "started",
		"dns-name":                   "dummyenv-0.dns",
		"instance-id":                "dummyenv-0",
		"series":                     "quantal",
		"hardware":                   "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
		"state-server-member-status": "adding-vote",
	}
	machine1 = M{
		"agent-state": "started",
		"dns-name":    "dummyenv-1.dns",
		"instance-id": "dummyenv-1",
		"series":      "quantal",
		"hardware":    "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
	}
	machine2 = M{
		"agent-state": "started",
		"dns-name":    "dummyenv-2.dns",
		"instance-id": "dummyenv-2",
		"series":      "quantal",
		"hardware":    "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
	}
	machine3 = M{
		"agent-state": "started",
		"dns-name":    "dummyenv-3.dns",
		"instance-id": "dummyenv-3",
		"series":      "quantal",
		"hardware":    "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
	}
	machine4 = M{
		"agent-state": "started",
		"dns-name":    "dummyenv-4.dns",
		"instance-id": "dummyenv-4",
		"series":      "quantal",
		"hardware":    "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
	}
	machine1WithContainers = M{
		"agent-state": "started",
		"containers": M{
			"1/lxc/0": M{
				"agent-state": "started",
				"containers": M{
					"1/lxc/0/lxc/0": M{
						"agent-state": "started",
						"dns-name":    "dummyenv-3.dns",
						"instance-id": "dummyenv-3",
						"series":      "quantal",
					},
				},
				"dns-name":    "dummyenv-2.dns",
				"instance-id": "dummyenv-2",
				"series":      "quantal",
			},
			"1/lxc/1": M{
				"instance-id": "pending",
				"series":      "quantal",
			},
		},
		"dns-name":    "dummyenv-1.dns",
		"instance-id": "dummyenv-1",
		"series":      "quantal",
		"hardware":    "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
	}
	machine1WithContainersScoped = M{
		"agent-state": "started",
		"containers": M{
			"1/lxc/0": M{
				"agent-state": "started",
				"dns-name":    "dummyenv-2.dns",
				"instance-id": "dummyenv-2",
				"series":      "quantal",
			},
		},
		"dns-name":    "dummyenv-1.dns",
		"instance-id": "dummyenv-1",
		"series":      "quantal",
		"hardware":    "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
	}
	unexposedService = M{
		"charm":   "cs:quantal/dummy-1",
		"exposed": false,
	}
	exposedService = M{
		"charm":   "cs:quantal/dummy-1",
		"exposed": true,
	}
)

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

var machineCons = constraints.MustParse("cpu-cores=2 mem=8G root-disk=8G")

var statusTests = []testCase{
	// Status tests
	test(
		"bootstrap and starting a single instance",

		addMachine{machineId: "0", job: state.JobManageEnviron},
		expect{
			"simulate juju bootstrap by adding machine/0 to the state",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": M{
						"instance-id":                "pending",
						"series":                     "quantal",
						"state-server-member-status": "adding-vote",
					},
				},
				"services": M{},
			},
		},

		startAliveMachine{"0"},
		setAddresses{"0", []network.Address{
			network.NewAddress("10.0.0.1", network.ScopeUnknown),
			network.NewAddress("dummyenv-0.dns", network.ScopePublic),
		}},
		expect{
			"simulate the PA starting an instance in response to the state change",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": M{
						"agent-state":                "pending",
						"dns-name":                   "dummyenv-0.dns",
						"instance-id":                "dummyenv-0",
						"series":                     "quantal",
						"hardware":                   "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
						"state-server-member-status": "adding-vote",
					},
				},
				"services": M{},
			},
		},

		setMachineStatus{"0", state.StatusStarted, ""},
		expect{
			"simulate the MA started and set the machine status",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": machine0,
				},
				"services": M{},
			},
		},

		setTools{"0", version.MustParseBinary("1.2.3-trusty-ppc")},
		expect{
			"simulate the MA setting the version",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": M{
						"dns-name":                   "dummyenv-0.dns",
						"instance-id":                "dummyenv-0",
						"agent-version":              "1.2.3",
						"agent-state":                "started",
						"series":                     "quantal",
						"hardware":                   "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
						"state-server-member-status": "adding-vote",
					},
				},
				"services": M{},
			},
		},
	), test(
		"deploy two services and two networks",
		addMachine{machineId: "0", job: state.JobManageEnviron},
		startAliveMachine{"0"},
		setMachineStatus{"0", state.StatusStarted, ""},
		setAddresses{"0", []network.Address{
			network.NewAddress("10.0.0.1", network.ScopeUnknown),
			network.NewAddress("dummyenv-0.dns", network.ScopePublic),
		}},
		addCharm{"dummy"},
		addService{
			name:     "networks-service",
			charm:    "dummy",
			networks: []string{"net1", "net2"},
			cons:     constraints.MustParse("networks=foo,bar,^no,^good"),
		},
		addService{
			name:  "no-networks-service",
			charm: "dummy",
			cons:  constraints.MustParse("networks=^mynet"),
		},
		addNetwork{
			name:       "net1",
			providerId: network.Id("provider-net1"),
			cidr:       "0.1.2.0/24",
			vlanTag:    0,
		},
		addNetwork{
			name:       "net2",
			providerId: network.Id("provider-vlan42"),
			cidr:       "0.42.1.0/24",
			vlanTag:    42,
		},

		expect{
			"simulate just the two services and a bootstrap node",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": machine0,
				},
				"services": M{
					"networks-service": M{
						"charm":   "cs:quantal/dummy-1",
						"exposed": false,
						"networks": M{
							"enabled":  L{"net1", "net2"},
							"disabled": L{"foo", "bar", "no", "good"},
						},
					},
					"no-networks-service": M{
						"charm":   "cs:quantal/dummy-1",
						"exposed": false,
						"networks": M{
							"disabled": L{"mynet"},
						},
					},
				},
				"networks": M{
					"net1": M{
						"provider-id": "provider-net1",
						"cidr":        "0.1.2.0/24",
					},
					"net2": M{
						"provider-id": "provider-vlan42",
						"cidr":        "0.42.1.0/24",
						"vlan-tag":    42,
					},
				},
			},
		},
	), test(
		"instance with different hardware characteristics",
		addMachine{machineId: "0", cons: machineCons, job: state.JobManageEnviron},
		setAddresses{"0", []network.Address{
			network.NewAddress("10.0.0.1", network.ScopeUnknown),
			network.NewAddress("dummyenv-0.dns", network.ScopePublic),
		}},
		startAliveMachine{"0"},
		setMachineStatus{"0", state.StatusStarted, ""},
		expect{
			"machine 0 has specific hardware characteristics",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": M{
						"agent-state":                "started",
						"dns-name":                   "dummyenv-0.dns",
						"instance-id":                "dummyenv-0",
						"series":                     "quantal",
						"hardware":                   "arch=amd64 cpu-cores=2 mem=8192M root-disk=8192M",
						"state-server-member-status": "adding-vote",
					},
				},
				"services": M{},
			},
		},
	), test(
		"instance without addresses",
		addMachine{machineId: "0", cons: machineCons, job: state.JobManageEnviron},
		startAliveMachine{"0"},
		setMachineStatus{"0", state.StatusStarted, ""},
		expect{
			"machine 0 has no dns-name",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": M{
						"agent-state":                "started",
						"instance-id":                "dummyenv-0",
						"series":                     "quantal",
						"hardware":                   "arch=amd64 cpu-cores=2 mem=8192M root-disk=8192M",
						"state-server-member-status": "adding-vote",
					},
				},
				"services": M{},
			},
		},
	), test(
		"test pending and missing machines",
		addMachine{machineId: "0", job: state.JobManageEnviron},
		expect{
			"machine 0 reports pending",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": M{
						"instance-id":                "pending",
						"series":                     "quantal",
						"state-server-member-status": "adding-vote",
					},
				},
				"services": M{},
			},
		},

		startMissingMachine{"0"},
		expect{
			"machine 0 reports missing",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": M{
						"instance-state":             "missing",
						"instance-id":                "i-missing",
						"agent-state":                "pending",
						"series":                     "quantal",
						"hardware":                   "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
						"state-server-member-status": "adding-vote",
					},
				},
				"services": M{},
			},
		},
	), test(
		"add two services and expose one, then add 2 more machines and some units",
		addMachine{machineId: "0", job: state.JobManageEnviron},
		setAddresses{"0", []network.Address{network.NewAddress("dummyenv-0.dns", network.ScopeUnknown)}},
		startAliveMachine{"0"},
		setMachineStatus{"0", state.StatusStarted, ""},
		addCharm{"dummy"},
		addService{name: "dummy-service", charm: "dummy"},
		addService{name: "exposed-service", charm: "dummy"},
		expect{
			"no services exposed yet",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": machine0,
				},
				"services": M{
					"dummy-service":   unexposedService,
					"exposed-service": unexposedService,
				},
			},
		},

		setServiceExposed{"exposed-service", true},
		expect{
			"one exposed service",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": machine0,
				},
				"services": M{
					"dummy-service":   unexposedService,
					"exposed-service": exposedService,
				},
			},
		},

		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", []network.Address{network.NewAddress("dummyenv-1.dns", network.ScopeUnknown)}},
		startAliveMachine{"1"},
		setMachineStatus{"1", state.StatusStarted, ""},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", []network.Address{network.NewAddress("dummyenv-2.dns", network.ScopeUnknown)}},
		startAliveMachine{"2"},
		setMachineStatus{"2", state.StatusStarted, ""},
		expect{
			"two more machines added",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": machine0,
					"1": machine1,
					"2": machine2,
				},
				"services": M{
					"dummy-service":   unexposedService,
					"exposed-service": exposedService,
				},
			},
		},

		addUnit{"dummy-service", "1"},
		addAliveUnit{"exposed-service", "2"},
		setUnitStatus{"exposed-service/0", state.StatusError, "You Require More Vespene Gas", nil},
		// Open multiple ports with different protocols,
		// ensure they're sorted on protocol, then number.
		openUnitPort{"exposed-service/0", "udp", 10},
		openUnitPort{"exposed-service/0", "udp", 2},
		openUnitPort{"exposed-service/0", "tcp", 3},
		openUnitPort{"exposed-service/0", "tcp", 2},
		// Simulate some status with no info, while the agent is down.
		setUnitStatus{"dummy-service/0", state.StatusStarted, "", nil},
		expect{
			"add two units, one alive (in error state), one down",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": machine0,
					"1": machine1,
					"2": machine2,
				},
				"services": M{
					"exposed-service": M{
						"charm":   "cs:quantal/dummy-1",
						"exposed": true,
						"units": M{
							"exposed-service/0": M{
								"machine":          "2",
								"agent-state":      "error",
								"agent-state-info": "You Require More Vespene Gas",
								"open-ports": L{
									"2/tcp", "3/tcp", "2/udp", "10/udp",
								},
								"public-address": "dummyenv-2.dns",
							},
						},
					},
					"dummy-service": M{
						"charm":   "cs:quantal/dummy-1",
						"exposed": false,
						"units": M{
							"dummy-service/0": M{
								"machine":          "1",
								"agent-state":      "down",
								"agent-state-info": "(started)",
								"public-address":   "dummyenv-1.dns",
							},
						},
					},
				},
			},
		},

		addMachine{machineId: "3", job: state.JobHostUnits},
		startMachine{"3"},
		// Simulate some status with info, while the agent is down.
		setAddresses{"3", []network.Address{network.NewAddress("dummyenv-3.dns", network.ScopeUnknown)}},
		setMachineStatus{"3", state.StatusStopped, "Really?"},
		addMachine{machineId: "4", job: state.JobHostUnits},
		setAddresses{"4", []network.Address{network.NewAddress("dummyenv-4.dns", network.ScopeUnknown)}},
		startAliveMachine{"4"},
		setMachineStatus{"4", state.StatusError, "Beware the red toys"},
		ensureDyingUnit{"dummy-service/0"},
		addMachine{machineId: "5", job: state.JobHostUnits},
		ensureDeadMachine{"5"},
		expect{
			"add three more machine, one with a dead agent, one in error state and one dead itself; also one dying unit",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": machine0,
					"1": machine1,
					"2": machine2,
					"3": M{
						"dns-name":         "dummyenv-3.dns",
						"instance-id":      "dummyenv-3",
						"agent-state":      "down",
						"agent-state-info": "(stopped: Really?)",
						"series":           "quantal",
						"hardware":         "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
					},
					"4": M{
						"dns-name":         "dummyenv-4.dns",
						"instance-id":      "dummyenv-4",
						"agent-state":      "error",
						"agent-state-info": "Beware the red toys",
						"series":           "quantal",
						"hardware":         "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
					},
					"5": M{
						"life":        "dead",
						"instance-id": "pending",
						"series":      "quantal",
					},
				},
				"services": M{
					"exposed-service": M{
						"charm":   "cs:quantal/dummy-1",
						"exposed": true,
						"units": M{
							"exposed-service/0": M{
								"machine":          "2",
								"agent-state":      "error",
								"agent-state-info": "You Require More Vespene Gas",
								"open-ports": L{
									"2/tcp", "3/tcp", "2/udp", "10/udp",
								},
								"public-address": "dummyenv-2.dns",
							},
						},
					},
					"dummy-service": M{
						"charm":   "cs:quantal/dummy-1",
						"exposed": false,
						"units": M{
							"dummy-service/0": M{
								"machine":          "1",
								"life":             "dying",
								"agent-state":      "down",
								"agent-state-info": "(started)",
								"public-address":   "dummyenv-1.dns",
							},
						},
					},
				},
			},
		},

		scopedExpect{
			"scope status on dummy-service/0 unit",
			[]string{"dummy-service/0"},
			M{
				"environment": "dummyenv",
				"machines": M{
					"1": machine1,
				},
				"services": M{
					"dummy-service": M{
						"charm":   "cs:quantal/dummy-1",
						"exposed": false,
						"units": M{
							"dummy-service/0": M{
								"machine":          "1",
								"life":             "dying",
								"agent-state":      "down",
								"agent-state-info": "(started)",
								"public-address":   "dummyenv-1.dns",
							},
						},
					},
				},
			},
		},
		scopedExpect{
			"scope status on exposed-service service",
			[]string{"exposed-service"},
			M{
				"environment": "dummyenv",
				"machines": M{
					"2": machine2,
				},
				"services": M{
					"exposed-service": M{
						"charm":   "cs:quantal/dummy-1",
						"exposed": true,
						"units": M{
							"exposed-service/0": M{
								"machine":          "2",
								"agent-state":      "error",
								"agent-state-info": "You Require More Vespene Gas",
								"open-ports": L{
									"2/tcp", "3/tcp", "2/udp", "10/udp",
								},
								"public-address": "dummyenv-2.dns",
							},
						},
					},
				},
			},
		},
		scopedExpect{
			"scope status on service pattern",
			[]string{"d*-service"},
			M{
				"environment": "dummyenv",
				"machines": M{
					"1": machine1,
				},
				"services": M{
					"dummy-service": M{
						"charm":   "cs:quantal/dummy-1",
						"exposed": false,
						"units": M{
							"dummy-service/0": M{
								"machine":          "1",
								"life":             "dying",
								"agent-state":      "down",
								"agent-state-info": "(started)",
								"public-address":   "dummyenv-1.dns",
							},
						},
					},
				},
			},
		},
		scopedExpect{
			"scope status on unit pattern",
			[]string{"e*posed-service/*"},
			M{
				"environment": "dummyenv",
				"machines": M{
					"2": machine2,
				},
				"services": M{
					"exposed-service": M{
						"charm":   "cs:quantal/dummy-1",
						"exposed": true,
						"units": M{
							"exposed-service/0": M{
								"machine":          "2",
								"agent-state":      "error",
								"agent-state-info": "You Require More Vespene Gas",
								"open-ports": L{
									"2/tcp", "3/tcp", "2/udp", "10/udp",
								},
								"public-address": "dummyenv-2.dns",
							},
						},
					},
				},
			},
		},
		scopedExpect{
			"scope status on combination of service and unit patterns",
			[]string{"exposed-service", "dummy-service", "e*posed-service/*", "dummy-service/*"},
			M{
				"environment": "dummyenv",
				"machines": M{
					"1": machine1,
					"2": machine2,
				},
				"services": M{
					"dummy-service": M{
						"charm":   "cs:quantal/dummy-1",
						"exposed": false,
						"units": M{
							"dummy-service/0": M{
								"machine":          "1",
								"life":             "dying",
								"agent-state":      "down",
								"agent-state-info": "(started)",
								"public-address":   "dummyenv-1.dns",
							},
						},
					},
					"exposed-service": M{
						"charm":   "cs:quantal/dummy-1",
						"exposed": true,
						"units": M{
							"exposed-service/0": M{
								"machine":          "2",
								"agent-state":      "error",
								"agent-state-info": "You Require More Vespene Gas",
								"open-ports": L{
									"2/tcp", "3/tcp", "2/udp", "10/udp",
								},
								"public-address": "dummyenv-2.dns",
							},
						},
					},
				},
			},
		},
	), test(
		"a unit with a hook relation error",
		addMachine{machineId: "0", job: state.JobManageEnviron},
		setAddresses{"0", []network.Address{network.NewAddress("dummyenv-0.dns", network.ScopeUnknown)}},
		startAliveMachine{"0"},
		setMachineStatus{"0", state.StatusStarted, ""},

		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", []network.Address{network.NewAddress("dummyenv-1.dns", network.ScopeUnknown)}},
		startAliveMachine{"1"},
		setMachineStatus{"1", state.StatusStarted, ""},

		addCharm{"wordpress"},
		addService{name: "wordpress", charm: "wordpress"},
		addAliveUnit{"wordpress", "1"},

		addCharm{"mysql"},
		addService{name: "mysql", charm: "mysql"},
		addAliveUnit{"mysql", "1"},

		relateServices{"wordpress", "mysql"},

		setUnitStatus{"wordpress/0", state.StatusError,
			"hook failed: some-relation-changed",
			map[string]interface{}{"relation-id": 0}},

		expect{
			"a unit with a hook relation error",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": machine0,
					"1": machine1,
				},
				"services": M{
					"wordpress": M{
						"charm":   "cs:quantal/wordpress-3",
						"exposed": false,
						"relations": M{
							"db": L{"mysql"},
						},
						"units": M{
							"wordpress/0": M{
								"machine":          "1",
								"agent-state":      "error",
								"agent-state-info": "hook failed: some-relation-changed for mysql:server",
								"public-address":   "dummyenv-1.dns",
							},
						},
					},
					"mysql": M{
						"charm":   "cs:quantal/mysql-1",
						"exposed": false,
						"relations": M{
							"server": L{"wordpress"},
						},
						"units": M{
							"mysql/0": M{
								"machine":        "1",
								"agent-state":    "pending",
								"public-address": "dummyenv-1.dns",
							},
						},
					},
				},
			},
		},
	), test(
		"a unit with a hook relation error when the agent is down",
		addMachine{machineId: "0", job: state.JobManageEnviron},
		setAddresses{"0", []network.Address{network.NewAddress("dummyenv-0.dns", network.ScopeUnknown)}},
		startAliveMachine{"0"},
		setMachineStatus{"0", state.StatusStarted, ""},

		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", []network.Address{network.NewAddress("dummyenv-1.dns", network.ScopeUnknown)}},
		startAliveMachine{"1"},
		setMachineStatus{"1", state.StatusStarted, ""},

		addCharm{"wordpress"},
		addService{name: "wordpress", charm: "wordpress"},
		addUnit{"wordpress", "1"},

		addCharm{"mysql"},
		addService{name: "mysql", charm: "mysql"},
		addAliveUnit{"mysql", "1"},

		relateServices{"wordpress", "mysql"},

		setUnitStatus{"wordpress/0", state.StatusError,
			"hook failed: some-relation-changed",
			map[string]interface{}{"relation-id": 0}},

		expect{
			"a unit with a hook relation error when the agent is down",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": machine0,
					"1": machine1,
				},
				"services": M{
					"wordpress": M{
						"charm":   "cs:quantal/wordpress-3",
						"exposed": false,
						"relations": M{
							"db": L{"mysql"},
						},
						"units": M{
							"wordpress/0": M{
								"machine":          "1",
								"agent-state":      "down",
								"agent-state-info": "(error: hook failed: some-relation-changed for mysql:server)",
								"public-address":   "dummyenv-1.dns",
							},
						},
					},
					"mysql": M{
						"charm":   "cs:quantal/mysql-1",
						"exposed": false,
						"relations": M{
							"server": L{"wordpress"},
						},
						"units": M{
							"mysql/0": M{
								"machine":        "1",
								"agent-state":    "pending",
								"public-address": "dummyenv-1.dns",
							},
						},
					},
				},
			},
		},
	), test(
		"add a dying service",
		addCharm{"dummy"},
		addService{name: "dummy-service", charm: "dummy"},
		addMachine{machineId: "0", job: state.JobHostUnits},
		addUnit{"dummy-service", "0"},
		ensureDyingService{"dummy-service"},
		expect{
			"service shows life==dying",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": M{
						"instance-id": "pending",
						"series":      "quantal",
					},
				},
				"services": M{
					"dummy-service": M{
						"charm":   "cs:quantal/dummy-1",
						"exposed": false,
						"life":    "dying",
						"units": M{
							"dummy-service/0": M{
								"machine":     "0",
								"agent-state": "pending",
							},
						},
					},
				},
			},
		},
	),

	// Relation tests
	test(
		"complex scenario with multiple related services",
		addMachine{machineId: "0", job: state.JobManageEnviron},
		setAddresses{"0", []network.Address{network.NewAddress("dummyenv-0.dns", network.ScopeUnknown)}},
		startAliveMachine{"0"},
		setMachineStatus{"0", state.StatusStarted, ""},
		addCharm{"wordpress"},
		addCharm{"mysql"},
		addCharm{"varnish"},

		addService{name: "project", charm: "wordpress"},
		setServiceExposed{"project", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", []network.Address{network.NewAddress("dummyenv-1.dns", network.ScopeUnknown)}},
		startAliveMachine{"1"},
		setMachineStatus{"1", state.StatusStarted, ""},
		addAliveUnit{"project", "1"},
		setUnitStatus{"project/0", state.StatusStarted, "", nil},

		addService{name: "mysql", charm: "mysql"},
		setServiceExposed{"mysql", true},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", []network.Address{network.NewAddress("dummyenv-2.dns", network.ScopeUnknown)}},
		startAliveMachine{"2"},
		setMachineStatus{"2", state.StatusStarted, ""},
		addAliveUnit{"mysql", "2"},
		setUnitStatus{"mysql/0", state.StatusStarted, "", nil},

		addService{name: "varnish", charm: "varnish"},
		setServiceExposed{"varnish", true},
		addMachine{machineId: "3", job: state.JobHostUnits},
		setAddresses{"3", []network.Address{network.NewAddress("dummyenv-3.dns", network.ScopeUnknown)}},
		startAliveMachine{"3"},
		setMachineStatus{"3", state.StatusStarted, ""},
		addUnit{"varnish", "3"},

		addService{name: "private", charm: "wordpress"},
		setServiceExposed{"private", true},
		addMachine{machineId: "4", job: state.JobHostUnits},
		setAddresses{"4", []network.Address{network.NewAddress("dummyenv-4.dns", network.ScopeUnknown)}},
		startAliveMachine{"4"},
		setMachineStatus{"4", state.StatusStarted, ""},
		addUnit{"private", "4"},

		relateServices{"project", "mysql"},
		relateServices{"project", "varnish"},
		relateServices{"private", "mysql"},

		expect{
			"multiples services with relations between some of them",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": machine0,
					"1": machine1,
					"2": machine2,
					"3": machine3,
					"4": machine4,
				},
				"services": M{
					"project": M{
						"charm":   "cs:quantal/wordpress-3",
						"exposed": true,
						"units": M{
							"project/0": M{
								"machine":        "1",
								"agent-state":    "started",
								"public-address": "dummyenv-1.dns",
							},
						},
						"relations": M{
							"db":    L{"mysql"},
							"cache": L{"varnish"},
						},
					},
					"mysql": M{
						"charm":   "cs:quantal/mysql-1",
						"exposed": true,
						"units": M{
							"mysql/0": M{
								"machine":        "2",
								"agent-state":    "started",
								"public-address": "dummyenv-2.dns",
							},
						},
						"relations": M{
							"server": L{"private", "project"},
						},
					},
					"varnish": M{
						"charm":   "cs:quantal/varnish-1",
						"exposed": true,
						"units": M{
							"varnish/0": M{
								"machine":        "3",
								"agent-state":    "pending",
								"public-address": "dummyenv-3.dns",
							},
						},
						"relations": M{
							"webcache": L{"project"},
						},
					},
					"private": M{
						"charm":   "cs:quantal/wordpress-3",
						"exposed": true,
						"units": M{
							"private/0": M{
								"machine":        "4",
								"agent-state":    "pending",
								"public-address": "dummyenv-4.dns",
							},
						},
						"relations": M{
							"db": L{"mysql"},
						},
					},
				},
			},
		},
	), test(
		"simple peer scenario",
		addMachine{machineId: "0", job: state.JobManageEnviron},
		setAddresses{"0", []network.Address{network.NewAddress("dummyenv-0.dns", network.ScopeUnknown)}},
		startAliveMachine{"0"},
		setMachineStatus{"0", state.StatusStarted, ""},
		addCharm{"riak"},
		addCharm{"wordpress"},

		addService{name: "riak", charm: "riak"},
		setServiceExposed{"riak", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", []network.Address{network.NewAddress("dummyenv-1.dns", network.ScopeUnknown)}},
		startAliveMachine{"1"},
		setMachineStatus{"1", state.StatusStarted, ""},
		addAliveUnit{"riak", "1"},
		setUnitStatus{"riak/0", state.StatusStarted, "", nil},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", []network.Address{network.NewAddress("dummyenv-2.dns", network.ScopeUnknown)}},
		startAliveMachine{"2"},
		setMachineStatus{"2", state.StatusStarted, ""},
		addAliveUnit{"riak", "2"},
		setUnitStatus{"riak/1", state.StatusStarted, "", nil},
		addMachine{machineId: "3", job: state.JobHostUnits},
		setAddresses{"3", []network.Address{network.NewAddress("dummyenv-3.dns", network.ScopeUnknown)}},
		startAliveMachine{"3"},
		setMachineStatus{"3", state.StatusStarted, ""},
		addAliveUnit{"riak", "3"},
		setUnitStatus{"riak/2", state.StatusStarted, "", nil},

		expect{
			"multiples related peer units",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": machine0,
					"1": machine1,
					"2": machine2,
					"3": machine3,
				},
				"services": M{
					"riak": M{
						"charm":   "cs:quantal/riak-7",
						"exposed": true,
						"units": M{
							"riak/0": M{
								"machine":        "1",
								"agent-state":    "started",
								"public-address": "dummyenv-1.dns",
							},
							"riak/1": M{
								"machine":        "2",
								"agent-state":    "started",
								"public-address": "dummyenv-2.dns",
							},
							"riak/2": M{
								"machine":        "3",
								"agent-state":    "started",
								"public-address": "dummyenv-3.dns",
							},
						},
						"relations": M{
							"ring": L{"riak"},
						},
					},
				},
			},
		},
	),

	// Subordinate tests
	test(
		"one service with one subordinate service",
		addMachine{machineId: "0", job: state.JobManageEnviron},
		setAddresses{"0", []network.Address{network.NewAddress("dummyenv-0.dns", network.ScopeUnknown)}},
		startAliveMachine{"0"},
		setMachineStatus{"0", state.StatusStarted, ""},
		addCharm{"wordpress"},
		addCharm{"mysql"},
		addCharm{"logging"},

		addService{name: "wordpress", charm: "wordpress"},
		setServiceExposed{"wordpress", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", []network.Address{network.NewAddress("dummyenv-1.dns", network.ScopeUnknown)}},
		startAliveMachine{"1"},
		setMachineStatus{"1", state.StatusStarted, ""},
		addAliveUnit{"wordpress", "1"},
		setUnitStatus{"wordpress/0", state.StatusStarted, "", nil},

		addService{name: "mysql", charm: "mysql"},
		setServiceExposed{"mysql", true},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", []network.Address{network.NewAddress("dummyenv-2.dns", network.ScopeUnknown)}},
		startAliveMachine{"2"},
		setMachineStatus{"2", state.StatusStarted, ""},
		addAliveUnit{"mysql", "2"},
		setUnitStatus{"mysql/0", state.StatusStarted, "", nil},

		addService{name: "logging", charm: "logging"},
		setServiceExposed{"logging", true},

		relateServices{"wordpress", "mysql"},
		relateServices{"wordpress", "logging"},
		relateServices{"mysql", "logging"},

		addSubordinate{"wordpress/0", "logging"},
		addSubordinate{"mysql/0", "logging"},

		setUnitsAlive{"logging"},
		setUnitStatus{"logging/0", state.StatusStarted, "", nil},
		setUnitStatus{"logging/1", state.StatusError, "somehow lost in all those logs", nil},

		expect{
			"multiples related peer units",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": machine0,
					"1": machine1,
					"2": machine2,
				},
				"services": M{
					"wordpress": M{
						"charm":   "cs:quantal/wordpress-3",
						"exposed": true,
						"units": M{
							"wordpress/0": M{
								"machine":     "1",
								"agent-state": "started",
								"subordinates": M{
									"logging/0": M{
										"agent-state":    "started",
										"public-address": "dummyenv-1.dns",
									},
								},
								"public-address": "dummyenv-1.dns",
							},
						},
						"relations": M{
							"db":          L{"mysql"},
							"logging-dir": L{"logging"},
						},
					},
					"mysql": M{
						"charm":   "cs:quantal/mysql-1",
						"exposed": true,
						"units": M{
							"mysql/0": M{
								"machine":     "2",
								"agent-state": "started",
								"subordinates": M{
									"logging/1": M{
										"agent-state":      "error",
										"agent-state-info": "somehow lost in all those logs",
										"public-address":   "dummyenv-2.dns",
									},
								},
								"public-address": "dummyenv-2.dns",
							},
						},
						"relations": M{
							"server":    L{"wordpress"},
							"juju-info": L{"logging"},
						},
					},
					"logging": M{
						"charm":   "cs:quantal/logging-1",
						"exposed": true,
						"relations": M{
							"logging-directory": L{"wordpress"},
							"info":              L{"mysql"},
						},
						"subordinate-to": L{"mysql", "wordpress"},
					},
				},
			},
		},

		// scoped on 'logging'
		scopedExpect{
			"subordinates scoped on logging",
			[]string{"logging"},
			M{
				"environment": "dummyenv",
				"machines": M{
					"1": machine1,
					"2": machine2,
				},
				"services": M{
					"wordpress": M{
						"charm":   "cs:quantal/wordpress-3",
						"exposed": true,
						"units": M{
							"wordpress/0": M{
								"machine":     "1",
								"agent-state": "started",
								"subordinates": M{
									"logging/0": M{
										"agent-state":    "started",
										"public-address": "dummyenv-1.dns",
									},
								},
								"public-address": "dummyenv-1.dns",
							},
						},
						"relations": M{
							"db":          L{"mysql"},
							"logging-dir": L{"logging"},
						},
					},
					"mysql": M{
						"charm":   "cs:quantal/mysql-1",
						"exposed": true,
						"units": M{
							"mysql/0": M{
								"machine":     "2",
								"agent-state": "started",
								"subordinates": M{
									"logging/1": M{
										"agent-state":      "error",
										"agent-state-info": "somehow lost in all those logs",
										"public-address":   "dummyenv-2.dns",
									},
								},
								"public-address": "dummyenv-2.dns",
							},
						},
						"relations": M{
							"server":    L{"wordpress"},
							"juju-info": L{"logging"},
						},
					},
					"logging": M{
						"charm":   "cs:quantal/logging-1",
						"exposed": true,
						"relations": M{
							"logging-directory": L{"wordpress"},
							"info":              L{"mysql"},
						},
						"subordinate-to": L{"mysql", "wordpress"},
					},
				},
			},
		},

		// scoped on wordpress/0
		scopedExpect{
			"subordinates scoped on logging",
			[]string{"wordpress/0"},
			M{
				"environment": "dummyenv",
				"machines": M{
					"1": machine1,
				},
				"services": M{
					"wordpress": M{
						"charm":   "cs:quantal/wordpress-3",
						"exposed": true,
						"units": M{
							"wordpress/0": M{
								"machine":     "1",
								"agent-state": "started",
								"subordinates": M{
									"logging/0": M{
										"agent-state":    "started",
										"public-address": "dummyenv-1.dns",
									},
								},
								"public-address": "dummyenv-1.dns",
							},
						},
						"relations": M{
							"db":          L{"mysql"},
							"logging-dir": L{"logging"},
						},
					},
					"logging": M{
						"charm":   "cs:quantal/logging-1",
						"exposed": true,
						"relations": M{
							"logging-directory": L{"wordpress"},
							"info":              L{"mysql"},
						},
						"subordinate-to": L{"mysql", "wordpress"},
					},
				},
			},
		},
	),
	test(
		"machines with containers",
		addMachine{machineId: "0", job: state.JobManageEnviron},
		setAddresses{"0", []network.Address{network.NewAddress("dummyenv-0.dns", network.ScopeUnknown)}},
		startAliveMachine{"0"},
		setMachineStatus{"0", state.StatusStarted, ""},
		addCharm{"mysql"},
		addService{name: "mysql", charm: "mysql"},
		setServiceExposed{"mysql", true},

		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", []network.Address{network.NewAddress("dummyenv-1.dns", network.ScopeUnknown)}},
		startAliveMachine{"1"},
		setMachineStatus{"1", state.StatusStarted, ""},
		addAliveUnit{"mysql", "1"},
		setUnitStatus{"mysql/0", state.StatusStarted, "", nil},

		// A container on machine 1.
		addContainer{"1", "1/lxc/0", state.JobHostUnits},
		setAddresses{"1/lxc/0", []network.Address{network.NewAddress("dummyenv-2.dns", network.ScopeUnknown)}},
		startAliveMachine{"1/lxc/0"},
		setMachineStatus{"1/lxc/0", state.StatusStarted, ""},
		addAliveUnit{"mysql", "1/lxc/0"},
		setUnitStatus{"mysql/1", state.StatusStarted, "", nil},
		addContainer{"1", "1/lxc/1", state.JobHostUnits},

		// A nested container.
		addContainer{"1/lxc/0", "1/lxc/0/lxc/0", state.JobHostUnits},
		setAddresses{"1/lxc/0/lxc/0", []network.Address{network.NewAddress("dummyenv-3.dns", network.ScopeUnknown)}},
		startAliveMachine{"1/lxc/0/lxc/0"},
		setMachineStatus{"1/lxc/0/lxc/0", state.StatusStarted, ""},

		expect{
			"machines with nested containers",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": machine0,
					"1": machine1WithContainers,
				},
				"services": M{
					"mysql": M{
						"charm":   "cs:quantal/mysql-1",
						"exposed": true,
						"units": M{
							"mysql/0": M{
								"machine":        "1",
								"agent-state":    "started",
								"public-address": "dummyenv-1.dns",
							},
							"mysql/1": M{
								"machine":        "1/lxc/0",
								"agent-state":    "started",
								"public-address": "dummyenv-2.dns",
							},
						},
					},
				},
			},
		},

		// once again, with a scope on mysql/1
		scopedExpect{
			"machines with nested containers",
			[]string{"mysql/1"},
			M{
				"environment": "dummyenv",
				"machines": M{
					"1": M{
						"agent-state": "started",
						"containers": M{
							"1/lxc/0": M{
								"agent-state": "started",
								"dns-name":    "dummyenv-2.dns",
								"instance-id": "dummyenv-2",
								"series":      "quantal",
							},
						},
						"dns-name":    "dummyenv-1.dns",
						"instance-id": "dummyenv-1",
						"series":      "quantal",
						"hardware":    "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
					},
				},
				"services": M{
					"mysql": M{
						"charm":   "cs:quantal/mysql-1",
						"exposed": true,
						"units": M{
							"mysql/1": M{
								"machine":        "1/lxc/0",
								"agent-state":    "started",
								"public-address": "dummyenv-2.dns",
							},
						},
					},
				},
			},
		},
	), test(
		"service with out of date charm",
		addMachine{machineId: "0", job: state.JobManageEnviron},
		setAddresses{"0", []network.Address{network.NewAddress("dummyenv-0.dns", network.ScopeUnknown)}},
		startAliveMachine{"0"},
		setMachineStatus{"0", state.StatusStarted, ""},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", []network.Address{network.NewAddress("dummyenv-1.dns", network.ScopeUnknown)}},
		startAliveMachine{"1"},
		setMachineStatus{"1", state.StatusStarted, ""},
		addCharm{"mysql"},
		addService{name: "mysql", charm: "mysql"},
		setServiceExposed{"mysql", true},
		addCharmPlaceholder{"mysql", 23},
		addAliveUnit{"mysql", "1"},

		expect{
			"services and units with correct charm status",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": machine0,
					"1": machine1,
				},
				"services": M{
					"mysql": M{
						"charm":          "cs:quantal/mysql-1",
						"can-upgrade-to": "cs:quantal/mysql-23",
						"exposed":        true,
						"units": M{
							"mysql/0": M{
								"machine":        "1",
								"agent-state":    "pending",
								"public-address": "dummyenv-1.dns",
							},
						},
					},
				},
			},
		},
	), test(
		"unit with out of date charm",
		addMachine{machineId: "0", job: state.JobManageEnviron},
		setAddresses{"0", []network.Address{network.NewAddress("dummyenv-0.dns", network.ScopeUnknown)}},
		startAliveMachine{"0"},
		setMachineStatus{"0", state.StatusStarted, ""},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", []network.Address{network.NewAddress("dummyenv-1.dns", network.ScopeUnknown)}},
		startAliveMachine{"1"},
		setMachineStatus{"1", state.StatusStarted, ""},
		addCharm{"mysql"},
		addService{name: "mysql", charm: "mysql"},
		setServiceExposed{"mysql", true},
		addAliveUnit{"mysql", "1"},
		setUnitCharmURL{"mysql/0", "cs:quantal/mysql-1"},
		addCharmWithRevision{addCharm{"mysql"}, "local", 1},
		setServiceCharm{"mysql", "local:quantal/mysql-1"},

		expect{
			"services and units with correct charm status",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": machine0,
					"1": machine1,
				},
				"services": M{
					"mysql": M{
						"charm":   "local:quantal/mysql-1",
						"exposed": true,
						"units": M{
							"mysql/0": M{
								"machine":        "1",
								"agent-state":    "started",
								"upgrading-from": "cs:quantal/mysql-1",
								"public-address": "dummyenv-1.dns",
							},
						},
					},
				},
			},
		},
	), test(
		"service and unit with out of date charms",
		addMachine{machineId: "0", job: state.JobManageEnviron},
		setAddresses{"0", []network.Address{network.NewAddress("dummyenv-0.dns", network.ScopeUnknown)}},
		startAliveMachine{"0"},
		setMachineStatus{"0", state.StatusStarted, ""},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", []network.Address{network.NewAddress("dummyenv-1.dns", network.ScopeUnknown)}},
		startAliveMachine{"1"},
		setMachineStatus{"1", state.StatusStarted, ""},
		addCharm{"mysql"},
		addService{name: "mysql", charm: "mysql"},
		setServiceExposed{"mysql", true},
		addAliveUnit{"mysql", "1"},
		setUnitCharmURL{"mysql/0", "cs:quantal/mysql-1"},
		addCharmWithRevision{addCharm{"mysql"}, "cs", 2},
		setServiceCharm{"mysql", "cs:quantal/mysql-2"},
		addCharmPlaceholder{"mysql", 23},

		expect{
			"services and units with correct charm status",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": machine0,
					"1": machine1,
				},
				"services": M{
					"mysql": M{
						"charm":          "cs:quantal/mysql-2",
						"can-upgrade-to": "cs:quantal/mysql-23",
						"exposed":        true,
						"units": M{
							"mysql/0": M{
								"machine":        "1",
								"agent-state":    "started",
								"upgrading-from": "cs:quantal/mysql-1",
								"public-address": "dummyenv-1.dns",
							},
						},
					},
				},
			},
		},
	), test(
		"service with local charm not shown as out of date",
		addMachine{machineId: "0", job: state.JobManageEnviron},
		setAddresses{"0", []network.Address{network.NewAddress("dummyenv-0.dns", network.ScopeUnknown)}},
		startAliveMachine{"0"},
		setMachineStatus{"0", state.StatusStarted, ""},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", []network.Address{network.NewAddress("dummyenv-1.dns", network.ScopeUnknown)}},
		startAliveMachine{"1"},
		setMachineStatus{"1", state.StatusStarted, ""},
		addCharm{"mysql"},
		addService{name: "mysql", charm: "mysql"},
		setServiceExposed{"mysql", true},
		addAliveUnit{"mysql", "1"},
		setUnitCharmURL{"mysql/0", "cs:quantal/mysql-1"},
		addCharmWithRevision{addCharm{"mysql"}, "local", 1},
		setServiceCharm{"mysql", "local:quantal/mysql-1"},
		addCharmPlaceholder{"mysql", 23},

		expect{
			"services and units with correct charm status",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": machine0,
					"1": machine1,
				},
				"services": M{
					"mysql": M{
						"charm":   "local:quantal/mysql-1",
						"exposed": true,
						"units": M{
							"mysql/0": M{
								"machine":        "1",
								"agent-state":    "started",
								"upgrading-from": "cs:quantal/mysql-1",
								"public-address": "dummyenv-1.dns",
							},
						},
					},
				},
			},
		},
	),
}

// TODO(dfc) test failing components by destructively mutating the state under the hood

type addMachine struct {
	machineId string
	cons      constraints.Value
	job       state.MachineJob
}

func (am addMachine) step(c *gc.C, ctx *context) {
	m, err := ctx.st.AddOneMachine(state.MachineTemplate{
		Series:      "quantal",
		Constraints: am.cons,
		Jobs:        []state.MachineJob{am.job},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, am.machineId)
}

type addNetwork struct {
	name       string
	providerId network.Id
	cidr       string
	vlanTag    int
}

func (an addNetwork) step(c *gc.C, ctx *context) {
	n, err := ctx.st.AddNetwork(state.NetworkInfo{
		Name:       an.name,
		ProviderId: an.providerId,
		CIDR:       an.cidr,
		VLANTag:    an.vlanTag,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n.Name(), gc.Equals, an.name)
}

type addContainer struct {
	parentId  string
	machineId string
	job       state.MachineJob
}

func (ac addContainer) step(c *gc.C, ctx *context) {
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{ac.job},
	}
	m, err := ctx.st.AddMachineInsideMachine(template, ac.parentId, instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, ac.machineId)
}

type startMachine struct {
	machineId string
}

func (sm startMachine) step(c *gc.C, ctx *context) {
	m, err := ctx.st.Machine(sm.machineId)
	c.Assert(err, jc.ErrorIsNil)
	cons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	inst, hc := testing.AssertStartInstanceWithConstraints(c, ctx.env, m.Id(), cons)
	err = m.SetProvisioned(inst.Id(), "fake_nonce", hc)
	c.Assert(err, jc.ErrorIsNil)
}

type startMissingMachine struct {
	machineId string
}

func (sm startMissingMachine) step(c *gc.C, ctx *context) {
	m, err := ctx.st.Machine(sm.machineId)
	c.Assert(err, jc.ErrorIsNil)
	cons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	_, hc := testing.AssertStartInstanceWithConstraints(c, ctx.env, m.Id(), cons)
	err = m.SetProvisioned("i-missing", "fake_nonce", hc)
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetInstanceStatus("missing")
	c.Assert(err, jc.ErrorIsNil)
}

type startAliveMachine struct {
	machineId string
}

func (sam startAliveMachine) step(c *gc.C, ctx *context) {
	m, err := ctx.st.Machine(sam.machineId)
	c.Assert(err, jc.ErrorIsNil)
	pinger := ctx.setAgentPresence(c, m)
	cons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	inst, hc := testing.AssertStartInstanceWithConstraints(c, ctx.env, m.Id(), cons)
	err = m.SetProvisioned(inst.Id(), "fake_nonce", hc)
	c.Assert(err, jc.ErrorIsNil)
	ctx.pingers[m.Id()] = pinger
}

type setAddresses struct {
	machineId string
	addresses []network.Address
}

func (sa setAddresses) step(c *gc.C, ctx *context) {
	m, err := ctx.st.Machine(sa.machineId)
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetAddresses(sa.addresses...)
	c.Assert(err, jc.ErrorIsNil)
}

type setTools struct {
	machineId string
	version   version.Binary
}

func (st setTools) step(c *gc.C, ctx *context) {
	m, err := ctx.st.Machine(st.machineId)
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetAgentVersion(st.version)
	c.Assert(err, jc.ErrorIsNil)
}

type addCharm struct {
	name string
}

func (ac addCharm) addCharmStep(c *gc.C, ctx *context, scheme string, rev int) {
	ch := testcharms.Repo.CharmDir(ac.name)
	name := ch.Meta().Name
	curl := charm.MustParseURL(fmt.Sprintf("%s:quantal/%s-%d", scheme, name, rev))
	dummy, err := ctx.st.AddCharm(ch, curl, "dummy-path", fmt.Sprintf("%s-%d-sha256", name, rev))
	c.Assert(err, jc.ErrorIsNil)
	ctx.charms[ac.name] = dummy
}

func (ac addCharm) step(c *gc.C, ctx *context) {
	ch := testcharms.Repo.CharmDir(ac.name)
	ac.addCharmStep(c, ctx, "cs", ch.Revision())
}

type addCharmWithRevision struct {
	addCharm
	scheme string
	rev    int
}

func (ac addCharmWithRevision) step(c *gc.C, ctx *context) {
	ac.addCharmStep(c, ctx, ac.scheme, ac.rev)
}

type addService struct {
	name     string
	charm    string
	networks []string
	cons     constraints.Value
}

func (as addService) step(c *gc.C, ctx *context) {
	ch, ok := ctx.charms[as.charm]
	c.Assert(ok, jc.IsTrue)
	svc, err := ctx.st.AddService(as.name, ctx.adminUserTag, ch, as.networks)
	c.Assert(err, jc.ErrorIsNil)
	if svc.IsPrincipal() {
		err = svc.SetConstraints(as.cons)
		c.Assert(err, jc.ErrorIsNil)
	}
}

type setServiceExposed struct {
	name    string
	exposed bool
}

func (sse setServiceExposed) step(c *gc.C, ctx *context) {
	s, err := ctx.st.Service(sse.name)
	c.Assert(err, jc.ErrorIsNil)
	err = s.ClearExposed()
	c.Assert(err, jc.ErrorIsNil)
	if sse.exposed {
		err = s.SetExposed()
		c.Assert(err, jc.ErrorIsNil)
	}
}

type setServiceCharm struct {
	name  string
	charm string
}

func (ssc setServiceCharm) step(c *gc.C, ctx *context) {
	ch, err := ctx.st.Charm(charm.MustParseURL(ssc.charm))
	c.Assert(err, jc.ErrorIsNil)
	s, err := ctx.st.Service(ssc.name)
	c.Assert(err, jc.ErrorIsNil)
	err = s.SetCharm(ch, false)
	c.Assert(err, jc.ErrorIsNil)
}

type addCharmPlaceholder struct {
	name string
	rev  int
}

func (ac addCharmPlaceholder) step(c *gc.C, ctx *context) {
	ch := testcharms.Repo.CharmDir(ac.name)
	name := ch.Meta().Name
	curl := charm.MustParseURL(fmt.Sprintf("cs:quantal/%s-%d", name, ac.rev))
	err := ctx.st.AddStoreCharmPlaceholder(curl)
	c.Assert(err, jc.ErrorIsNil)
}

type addUnit struct {
	serviceName string
	machineId   string
}

func (au addUnit) step(c *gc.C, ctx *context) {
	s, err := ctx.st.Service(au.serviceName)
	c.Assert(err, jc.ErrorIsNil)
	u, err := s.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	m, err := ctx.st.Machine(au.machineId)
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
}

type addAliveUnit struct {
	serviceName string
	machineId   string
}

func (aau addAliveUnit) step(c *gc.C, ctx *context) {
	s, err := ctx.st.Service(aau.serviceName)
	c.Assert(err, jc.ErrorIsNil)
	u, err := s.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	pinger := ctx.setAgentPresence(c, u)
	m, err := ctx.st.Machine(aau.machineId)
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	ctx.pingers[u.Name()] = pinger
}

type setUnitsAlive struct {
	serviceName string
}

func (sua setUnitsAlive) step(c *gc.C, ctx *context) {
	s, err := ctx.st.Service(sua.serviceName)
	c.Assert(err, jc.ErrorIsNil)
	us, err := s.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	for _, u := range us {
		ctx.pingers[u.Name()] = ctx.setAgentPresence(c, u)
	}
}

type setUnitStatus struct {
	unitName   string
	status     state.Status
	statusInfo string
	statusData map[string]interface{}
}

func (sus setUnitStatus) step(c *gc.C, ctx *context) {
	u, err := ctx.st.Unit(sus.unitName)
	c.Assert(err, jc.ErrorIsNil)
	err = u.SetStatus(sus.status, sus.statusInfo, sus.statusData)
	c.Assert(err, jc.ErrorIsNil)
}

type setUnitCharmURL struct {
	unitName string
	charm    string
}

func (uc setUnitCharmURL) step(c *gc.C, ctx *context) {
	u, err := ctx.st.Unit(uc.unitName)
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL(uc.charm)
	err = u.SetCharmURL(curl)
	c.Assert(err, jc.ErrorIsNil)
	err = u.SetStatus(state.StatusStarted, "", nil)
	c.Assert(err, jc.ErrorIsNil)
}

type openUnitPort struct {
	unitName string
	protocol string
	number   int
}

func (oup openUnitPort) step(c *gc.C, ctx *context) {
	u, err := ctx.st.Unit(oup.unitName)
	c.Assert(err, jc.ErrorIsNil)
	err = u.OpenPort(oup.protocol, oup.number)
	c.Assert(err, jc.ErrorIsNil)
}

type ensureDyingUnit struct {
	unitName string
}

func (e ensureDyingUnit) step(c *gc.C, ctx *context) {
	u, err := ctx.st.Unit(e.unitName)
	c.Assert(err, jc.ErrorIsNil)
	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.Life(), gc.Equals, state.Dying)
}

type ensureDyingService struct {
	serviceName string
}

func (e ensureDyingService) step(c *gc.C, ctx *context) {
	svc, err := ctx.st.Service(e.serviceName)
	c.Assert(err, jc.ErrorIsNil)
	err = svc.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = svc.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(svc.Life(), gc.Equals, state.Dying)
}

type ensureDeadMachine struct {
	machineId string
}

func (e ensureDeadMachine) step(c *gc.C, ctx *context) {
	m, err := ctx.st.Machine(e.machineId)
	c.Assert(err, jc.ErrorIsNil)
	err = m.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, state.Dead)
}

type setMachineStatus struct {
	machineId  string
	status     state.Status
	statusInfo string
}

func (sms setMachineStatus) step(c *gc.C, ctx *context) {
	m, err := ctx.st.Machine(sms.machineId)
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetStatus(sms.status, sms.statusInfo, nil)
	c.Assert(err, jc.ErrorIsNil)
}

type relateServices struct {
	ep1, ep2 string
}

func (rs relateServices) step(c *gc.C, ctx *context) {
	eps, err := ctx.st.InferEndpoints(rs.ep1, rs.ep2)
	c.Assert(err, jc.ErrorIsNil)
	_, err = ctx.st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
}

type addSubordinate struct {
	prinUnit   string
	subService string
}

func (as addSubordinate) step(c *gc.C, ctx *context) {
	u, err := ctx.st.Unit(as.prinUnit)
	c.Assert(err, jc.ErrorIsNil)
	eps, err := ctx.st.InferEndpoints(u.ServiceName(), as.subService)
	c.Assert(err, jc.ErrorIsNil)
	rel, err := ctx.st.EndpointsRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(u)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
}

type scopedExpect struct {
	what   string
	scope  []string
	output M
}

type expect struct {
	what   string
	output M
}

func (e scopedExpect) step(c *gc.C, ctx *context) {
	c.Logf("\nexpect: %s %s\n", e.what, strings.Join(e.scope, " "))

	// Now execute the command for each format.
	for _, format := range statusFormats {
		c.Logf("format %q", format.name)
		// Run command with the required format.
		args := append([]string{"--format", format.name}, e.scope...)
		c.Logf("running status %s", strings.Join(args, " "))
		code, stdout, stderr := runStatus(c, args...)
		c.Assert(code, gc.Equals, 0)
		if !c.Check(stderr, gc.HasLen, 0) {
			c.Fatalf("status failed: %s", string(stderr))
		}

		// Prepare the output in the same format.
		buf, err := format.marshal(e.output)
		c.Assert(err, jc.ErrorIsNil)
		expected := make(M)
		err = format.unmarshal(buf, &expected)
		c.Assert(err, jc.ErrorIsNil)

		// Check the output is as expected.
		actual := make(M)
		err = format.unmarshal(stdout, &actual)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(actual, jc.DeepEquals, expected)
	}
}

func (e expect) step(c *gc.C, ctx *context) {
	scopedExpect{e.what, nil, e.output}.step(c, ctx)
}

func (s *StatusSuite) TestStatusAllFormats(c *gc.C) {
	for i, t := range statusTests {
		c.Logf("test %d: %s", i, t.summary)
		func(t testCase) {
			// Prepare context and run all steps to setup.
			ctx := s.newContext(c)
			defer s.resetContext(c, ctx)
			ctx.run(c, t.steps)
		}(t)
	}
}

type fakeApiClient struct {
	statusReturn *api.Status
	patternsUsed []string
	closeCalled  bool
}

func newFakeApiClient(statusReturn *api.Status) fakeApiClient {
	return fakeApiClient{
		statusReturn: statusReturn,
	}
}

func (a *fakeApiClient) Status(patterns []string) (*api.Status, error) {
	a.patternsUsed = patterns
	return a.statusReturn, nil
}

func (a *fakeApiClient) Close() error {
	a.closeCalled = true
	return nil
}

// Check that the client works with an older server which doesn't
// return the top level Relations field nor the unit and machine level
// Agent field (they were introduced at the same time).
func (s *StatusSuite) TestStatusWithPreRelationsServer(c *gc.C) {
	// Construct an older style status response
	client := newFakeApiClient(&api.Status{
		EnvironmentName: "dummyenv",
		Machines: map[string]api.MachineStatus{
			"0": {
				// Agent field intentionally not set
				Id:             "0",
				InstanceId:     instance.Id("dummyenv-0"),
				AgentState:     "down",
				AgentStateInfo: "(started)",
				Series:         "quantal",
				Containers:     map[string]api.MachineStatus{},
				Jobs:           []multiwatcher.MachineJob{multiwatcher.JobManageEnviron},
				HasVote:        false,
				WantsVote:      true,
			},
			"1": {
				// Agent field intentionally not set
				Id:             "1",
				InstanceId:     instance.Id("dummyenv-1"),
				AgentState:     "started",
				AgentStateInfo: "hello",
				Series:         "quantal",
				Containers:     map[string]api.MachineStatus{},
				Jobs:           []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
				HasVote:        false,
				WantsVote:      false,
			},
		},
		Services: map[string]api.ServiceStatus{
			"mysql": api.ServiceStatus{
				Charm: "local:quantal/mysql-1",
				Relations: map[string][]string{
					"server": []string{"wordpress"},
				},
				Units: map[string]api.UnitStatus{
					"mysql/0": api.UnitStatus{
						// Agent field intentionally not set
						Machine:    "1",
						AgentState: "pending",
					},
				},
			},
			"wordpress": api.ServiceStatus{
				Charm: "local:quantal/wordpress-3",
				Relations: map[string][]string{
					"db": []string{"mysql"},
				},
				Units: map[string]api.UnitStatus{
					"wordpress/0": api.UnitStatus{
						// Agent field intentionally not set
						AgentState:     "error",
						AgentStateInfo: "blam",
						Machine:        "1",
					},
				},
			},
		},
		Networks: map[string]api.NetworkStatus{},
		// Relations field intentionally not set
	})
	s.PatchValue(&newApiClientForStatus, func(_ *StatusCommand) (statusAPI, error) {
		return &client, nil
	})

	expected := expect{
		"sane output with an older client that doesn't return Agent or Relations fields",
		M{
			"environment": "dummyenv",
			"machines": M{
				"0": M{
					"agent-state":                "down",
					"agent-state-info":           "(started)",
					"instance-id":                "dummyenv-0",
					"series":                     "quantal",
					"state-server-member-status": "adding-vote",
				},
				"1": M{
					"agent-state":      "started",
					"agent-state-info": "hello",
					"instance-id":      "dummyenv-1",
					"series":           "quantal",
				},
			},
			"services": M{
				"mysql": M{
					"charm":   "local:quantal/mysql-1",
					"exposed": false,
					"relations": M{
						"server": L{"wordpress"},
					},
					"units": M{
						"mysql/0": M{
							"machine":     "1",
							"agent-state": "pending",
						},
					},
				},
				"wordpress": M{
					"charm":   "local:quantal/wordpress-3",
					"exposed": false,
					"relations": M{
						"db": L{"mysql"},
					},
					"units": M{
						"wordpress/0": M{
							"machine":          "1",
							"agent-state":      "error",
							"agent-state-info": "blam",
						},
					},
				},
			},
		},
	}
	ctx := s.newContext(c)
	defer s.resetContext(c, ctx)
	ctx.run(c, []stepper{expected})
}

func (s *StatusSuite) TestStatusWithFormatSummary(c *gc.C) {
	ctx := s.newContext(c)
	defer s.resetContext(c, ctx)
	steps := []stepper{
		addMachine{machineId: "0", job: state.JobManageEnviron},
		setAddresses{"0", []network.Address{network.NewAddress("localhost", network.ScopeUnknown)}},
		startAliveMachine{"0"},
		setMachineStatus{"0", state.StatusStarted, ""},
		addCharm{"wordpress"},
		addCharm{"mysql"},
		addCharm{"logging"},
		addService{name: "wordpress", charm: "wordpress"},
		setServiceExposed{"wordpress", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", []network.Address{network.NewAddress("localhost", network.ScopeUnknown)}},
		startAliveMachine{"1"},
		setMachineStatus{"1", state.StatusStarted, ""},
		addAliveUnit{"wordpress", "1"},
		setUnitStatus{"wordpress/0", state.StatusStarted, "", nil},
		addService{name: "mysql", charm: "mysql"},
		setServiceExposed{"mysql", true},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", []network.Address{network.NewAddress("10.0.0.1", network.ScopeUnknown)}},
		startAliveMachine{"2"},
		setMachineStatus{"2", state.StatusStarted, ""},
		addAliveUnit{"mysql", "2"},
		setUnitStatus{"mysql/0", state.StatusStarted, "", nil},
		addService{name: "logging", charm: "logging"},
		setServiceExposed{"logging", true},
		relateServices{"wordpress", "mysql"},
		relateServices{"wordpress", "logging"},
		relateServices{"mysql", "logging"},
		addSubordinate{"wordpress/0", "logging"},
		addSubordinate{"mysql/0", "logging"},
		setUnitsAlive{"logging"},
		setUnitStatus{"logging/0", state.StatusStarted, "", nil},
		setUnitStatus{"logging/1", state.StatusError, "somehow lost in all those logs", nil},
	}
	for _, s := range steps {
		s.step(c, ctx)
	}
	code, stdout, stderr := runStatus(c, "--format", "summary")
	c.Check(code, gc.Equals, 0)
	c.Check(string(stderr), gc.Equals, "")
	c.Assert(
		string(stdout),
		gc.Equals,
		"Running on subnets: 127.0.0.1/8, 10.0.0.1/8 \n"+
			"Utilizing ports:                            \n"+
			" # MACHINES: (3)\n"+
			"    started:  3 \n"+
			"            \n"+
			"    # UNITS: (4)\n"+
			"      error:  1 \n"+
			"    started:  3 \n"+
			"            \n"+
			" # SERVICES:  (3)\n"+
			"     logging  1/1 exposed\n"+
			"       mysql  1/1 exposed\n"+
			"   wordpress  1/1 exposed\n"+
			"\n",
	)
}
func (s *StatusSuite) TestStatusWithFormatOneline(c *gc.C) {
	ctx := s.newContext(c)
	defer s.resetContext(c, ctx)
	steps := []stepper{
		addMachine{machineId: "0", job: state.JobManageEnviron},
		setAddresses{"0", []network.Address{network.NewAddress("dummyenv-0.dns", network.ScopeUnknown)}},
		startAliveMachine{"0"},
		setMachineStatus{"0", state.StatusStarted, ""},
		addCharm{"wordpress"},
		addCharm{"mysql"},
		addCharm{"logging"},

		addService{name: "wordpress", charm: "wordpress"},
		setServiceExposed{"wordpress", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", []network.Address{network.NewAddress("dummyenv-1.dns", network.ScopeUnknown)}},
		startAliveMachine{"1"},
		setMachineStatus{"1", state.StatusStarted, ""},
		addAliveUnit{"wordpress", "1"},
		setUnitStatus{"wordpress/0", state.StatusStarted, "", nil},

		addService{name: "mysql", charm: "mysql"},
		setServiceExposed{"mysql", true},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", []network.Address{network.NewAddress("dummyenv-2.dns", network.ScopeUnknown)}},
		startAliveMachine{"2"},
		setMachineStatus{"2", state.StatusStarted, ""},
		addAliveUnit{"mysql", "2"},
		setUnitStatus{"mysql/0", state.StatusStarted, "", nil},

		addService{name: "logging", charm: "logging"},
		setServiceExposed{"logging", true},

		relateServices{"wordpress", "mysql"},
		relateServices{"wordpress", "logging"},
		relateServices{"mysql", "logging"},

		addSubordinate{"wordpress/0", "logging"},
		addSubordinate{"mysql/0", "logging"},

		setUnitsAlive{"logging"},
		setUnitStatus{"logging/0", state.StatusStarted, "", nil},
		setUnitStatus{"logging/1", state.StatusError, "somehow lost in all those logs", nil},
	}

	ctx.run(c, steps)

	const expected = `
- mysql/0: dummyenv-2.dns (started)
  - logging/1: dummyenv-2.dns (error)
- wordpress/0: dummyenv-1.dns (started)
  - logging/0: dummyenv-1.dns (started)
`

	code, stdout, stderr := runStatus(c, "--format", "oneline")
	c.Check(code, gc.Equals, 0)
	c.Check(string(stderr), gc.Equals, "")
	c.Assert(string(stdout), gc.Equals, expected)

	c.Log(`Check that "short" is an alias for oneline.`)
	code, stdout, stderr = runStatus(c, "--format", "short")
	c.Check(code, gc.Equals, 0)
	c.Check(string(stderr), gc.Equals, "")
	c.Assert(string(stdout), gc.Equals, expected)

	c.Log(`Check that "line" is an alias for oneline.`)
	code, stdout, stderr = runStatus(c, "--format", "line")
	c.Check(code, gc.Equals, 0)
	c.Check(string(stderr), gc.Equals, "")
	c.Assert(string(stdout), gc.Equals, expected)
}
func (s *StatusSuite) TestStatusWithFormatTabular(c *gc.C) {
	ctx := s.newContext(c)
	defer s.resetContext(c, ctx)
	steps := []stepper{
		addMachine{machineId: "0", job: state.JobManageEnviron},
		setAddresses{"0", []network.Address{network.NewAddress("dummyenv-0.dns", network.ScopeUnknown)}},
		startAliveMachine{"0"},
		setMachineStatus{"0", state.StatusStarted, ""},
		addCharm{"wordpress"},
		addCharm{"mysql"},
		addCharm{"logging"},
		addService{name: "wordpress", charm: "wordpress"},
		setServiceExposed{"wordpress", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", []network.Address{network.NewAddress("dummyenv-1.dns", network.ScopeUnknown)}},
		startAliveMachine{"1"},
		setMachineStatus{"1", state.StatusStarted, ""},
		addAliveUnit{"wordpress", "1"},
		setUnitStatus{"wordpress/0", state.StatusStarted, "", nil},
		addService{name: "mysql", charm: "mysql"},
		setServiceExposed{"mysql", true},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", []network.Address{network.NewAddress("dummyenv-2.dns", network.ScopeUnknown)}},
		startAliveMachine{"2"},
		setMachineStatus{"2", state.StatusStarted, ""},
		addAliveUnit{"mysql", "2"},
		setUnitStatus{"mysql/0", state.StatusStarted, "", nil},
		addService{name: "logging", charm: "logging"},
		setServiceExposed{"logging", true},
		relateServices{"wordpress", "mysql"},
		relateServices{"wordpress", "logging"},
		relateServices{"mysql", "logging"},
		addSubordinate{"wordpress/0", "logging"},
		addSubordinate{"mysql/0", "logging"},
		setUnitsAlive{"logging"},
		setUnitStatus{"logging/0", state.StatusStarted, "", nil},
		setUnitStatus{"logging/1", state.StatusError, "somehow lost in all those logs", nil},
	}
	for _, s := range steps {
		s.step(c, ctx)
	}
	code, stdout, stderr := runStatus(c, "--format", "tabular")
	c.Check(code, gc.Equals, 0)
	c.Check(string(stderr), gc.Equals, "")
	c.Assert(
		string(stdout),
		gc.Equals,
		"[Machines] \n"+
			"ID         STATE   VERSION DNS            INS-ID     SERIES  HARDWARE                                         \n"+
			"0          started         dummyenv-0.dns dummyenv-0 quantal arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M \n"+
			"1          started         dummyenv-1.dns dummyenv-1 quantal arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M \n"+
			"2          started         dummyenv-2.dns dummyenv-2 quantal arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M \n"+
			"\n"+
			"[Services] \n"+
			"NAME       EXPOSED CHARM                  \n"+
			"logging    true    cs:quantal/logging-1   \n"+
			"mysql      true    cs:quantal/mysql-1     \n"+
			"wordpress  true    cs:quantal/wordpress-3 \n"+
			"\n"+
			"[Units]     \n"+
			"ID          STATE   VERSION MACHINE PORTS PUBLIC-ADDRESS \n"+
			"mysql/0     started         2             dummyenv-2.dns \n"+
			"  logging/1 error                         dummyenv-2.dns \n"+
			"wordpress/0 started         1             dummyenv-1.dns \n"+
			"  logging/0 started                       dummyenv-1.dns \n"+
			"\n",
	)
}

func (s *StatusSuite) TestStatusWithNilStatusApi(c *gc.C) {
	ctx := s.newContext(c)
	defer s.resetContext(c, ctx)
	steps := []stepper{
		addMachine{machineId: "0", job: state.JobManageEnviron},
		setAddresses{"0", []network.Address{network.NewAddress("dummyenv-0.dns", network.ScopeUnknown)}},
		startAliveMachine{"0"},
		setMachineStatus{"0", state.StatusStarted, ""},
	}

	for _, s := range steps {
		s.step(c, ctx)
	}

	client := fakeApiClient{}
	var status = client.Status
	s.PatchValue(&status, func(_ []string) (*api.Status, error) {
		return nil, nil
	})
	s.PatchValue(&newApiClientForStatus, func(_ *StatusCommand) (statusAPI, error) {
		return &client, nil
	})

	code, _, stderr := runStatus(c, "--format", "tabular")
	c.Check(code, gc.Equals, 1)
	c.Check(string(stderr), gc.Equals, "error: unable to obtain the current status\n")
}

//
// Filtering Feature
//

func (s *StatusSuite) FilteringTestSetup(c *gc.C) *context {
	ctx := s.newContext(c)

	steps := []stepper{
		// Given a machine is started
		// And the machine's ID is "0"
		// And the machine's job is to manage the environment
		addMachine{machineId: "0", job: state.JobManageEnviron},
		startAliveMachine{"0"},
		setMachineStatus{"0", state.StatusStarted, ""},
		// And the machine's address is "dummyenv-0.dns"
		setAddresses{"0", []network.Address{network.NewAddress("dummyenv-0.dns", network.ScopeUnknown)}},
		// And the "wordpress" charm is available
		addCharm{"wordpress"},
		addService{name: "wordpress", charm: "wordpress"},
		// And the "mysql" charm is available
		addCharm{"mysql"},
		addService{name: "mysql", charm: "mysql"},
		// And the "logging" charm is available
		addCharm{"logging"},
		// And a machine is started
		// And the machine's ID is "1"
		// And the machine's job is to host units
		addMachine{machineId: "1", job: state.JobHostUnits},
		startAliveMachine{"1"},
		setMachineStatus{"1", state.StatusStarted, ""},
		// And the machine's address is "dummyenv-1.dns"
		setAddresses{"1", []network.Address{network.NewAddress("dummyenv-1.dns", network.ScopeUnknown)}},
		// And a unit of "wordpress" is deployed to machine "1"
		addAliveUnit{"wordpress", "1"},
		// And the unit is started
		setUnitStatus{"wordpress/0", state.StatusStarted, "", nil},
		// And a machine is started

		// And the machine's ID is "2"
		// And the machine's job is to host units
		addMachine{machineId: "2", job: state.JobHostUnits},
		startAliveMachine{"2"},
		setMachineStatus{"2", state.StatusStarted, ""},
		// And the machine's address is "dummyenv-2.dns"
		setAddresses{"2", []network.Address{network.NewAddress("dummyenv-2.dns", network.ScopeUnknown)}},
		// And a unit of "mysql" is deployed to machine "2"
		addAliveUnit{"mysql", "2"},
		// And the unit is started
		setUnitStatus{"mysql/0", state.StatusStarted, "", nil},
		// And the "logging" service is added
		addService{name: "logging", charm: "logging"},
		// And the service is exposed
		setServiceExposed{"logging", true},
		// And the "wordpress" service is related to the "mysql" service
		relateServices{"wordpress", "mysql"},
		// And the "wordpress" service is related to the "logging" service
		relateServices{"wordpress", "logging"},
		// And the "mysql" service is related to the "logging" service
		relateServices{"mysql", "logging"},
		// And the "logging" service is a subordinate to unit 0 of the "wordpress" service
		addSubordinate{"wordpress/0", "logging"},
		setUnitStatus{"logging/0", state.StatusStarted, "", nil},
		// And the "logging" service is a subordinate to unit 0 of the "mysql" service
		addSubordinate{"mysql/0", "logging"},
		setUnitStatus{"logging/1", state.StatusStarted, "", nil},
		setUnitsAlive{"logging"},
	}

	ctx.run(c, steps)
	return ctx
}

// Scenario: One unit is in an errored state and user filters to started
func (s *StatusSuite) TestFilterToStarted(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	// Given unit 1 of the "logging" service has an error
	setUnitStatus{"logging/1", state.StatusError, "mock error", nil}.step(c, ctx)
	// And unit 0 of the "mysql" service has an error
	setUnitStatus{"mysql/0", state.StatusError, "mock error", nil}.step(c, ctx)
	// When I run juju status --format oneline started
	_, stdout, stderr := runStatus(c, "--format", "oneline", "started")
	c.Assert(string(stderr), gc.Equals, "")
	// Then I should receive output prefixed with:
	const expected = `

- wordpress/0: dummyenv-1.dns (started)
  - logging/0: dummyenv-1.dns (started)
`

	c.Assert(string(stdout), gc.Equals, expected[1:])
}

// Scenario: One unit is in an errored state and user filters to errored
func (s *StatusSuite) TestFilterToErrored(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	// Given unit 1 of the "logging" service has an error
	setUnitStatus{"logging/1", state.StatusError, "mock error", nil}.step(c, ctx)
	// When I run juju status --format oneline error
	_, stdout, stderr := runStatus(c, "--format", "oneline", "error")
	c.Assert(stderr, gc.IsNil)
	// Then I should receive output prefixed with:
	const expected = `

- mysql/0: dummyenv-2.dns (started)
  - logging/1: dummyenv-2.dns (error)
`

	c.Assert(string(stdout), gc.Equals, expected[1:])
}

// Scenario: User filters to mysql service
func (s *StatusSuite) TestFilterToService(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	// When I run juju status --format oneline error
	_, stdout, stderr := runStatus(c, "--format", "oneline", "mysql")
	c.Assert(stderr, gc.IsNil)
	// Then I should receive output prefixed with:
	const expected = `

- mysql/0: dummyenv-2.dns (started)
  - logging/1: dummyenv-2.dns (started)
`

	c.Assert(string(stdout), gc.Equals, expected[1:])
}

// Scenario: User filters to exposed services
func (s *StatusSuite) TestFilterToExposedService(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	// Given unit 1 of the "mysql" service is exposed
	setServiceExposed{"mysql", true}.step(c, ctx)
	// And the logging service is not exposed
	setServiceExposed{"logging", false}.step(c, ctx)
	// And the wordpress service is not exposed
	setServiceExposed{"wordpress", false}.step(c, ctx)
	// When I run juju status --format oneline exposed
	_, stdout, stderr := runStatus(c, "--format", "oneline", "exposed")
	c.Assert(stderr, gc.IsNil)
	// Then I should receive output prefixed with:
	const expected = `

- mysql/0: dummyenv-2.dns (started)
  - logging/1: dummyenv-2.dns (started)
`

	c.Assert(string(stdout), gc.Equals, expected[1:])
}

// Scenario: User filters to non-exposed services
func (s *StatusSuite) TestFilterToNotExposedService(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	setServiceExposed{"mysql", true}.step(c, ctx)
	// When I run juju status --format oneline not exposed
	_, stdout, stderr := runStatus(c, "--format", "oneline", "not", "exposed")
	c.Assert(stderr, gc.IsNil)
	// Then I should receive output prefixed with:
	const expected = `

- wordpress/0: dummyenv-1.dns (started)
  - logging/0: dummyenv-1.dns (started)
`

	c.Assert(string(stdout), gc.Equals, expected[1:])
}

// Scenario: Filtering on Subnets
func (s *StatusSuite) TestFilterOnSubnet(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	// Given the address for machine "1" is "localhost"
	setAddresses{"1", []network.Address{network.NewAddress("localhost", network.ScopeUnknown)}}.step(c, ctx)
	// And the address for machine "2" is "10.0.0.1"
	setAddresses{"2", []network.Address{network.NewAddress("10.0.0.1", network.ScopeUnknown)}}.step(c, ctx)
	// When I run juju status --format oneline 127.0.0.1
	_, stdout, stderr := runStatus(c, "--format", "oneline", "127.0.0.1")
	c.Assert(stderr, gc.IsNil)
	// Then I should receive output prefixed with:
	const expected = `

- wordpress/0: localhost (started)
  - logging/0: localhost (started)
`

	c.Assert(string(stdout), gc.Equals, expected[1:])
}

// Scenario: Filtering on Ports
func (s *StatusSuite) TestFilterOnPorts(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	// Given the address for machine "1" is "localhost"
	setAddresses{"1", []network.Address{network.NewAddress("localhost", network.ScopeUnknown)}}.step(c, ctx)
	// And the address for machine "2" is "10.0.0.1"
	setAddresses{"2", []network.Address{network.NewAddress("10.0.0.1", network.ScopeUnknown)}}.step(c, ctx)
	openUnitPort{"wordpress/0", "tcp", 80}.step(c, ctx)
	// When I run juju status --format oneline 80/tcp
	_, stdout, stderr := runStatus(c, "--format", "oneline", "80/tcp")
	c.Assert(stderr, gc.IsNil)
	// Then I should receive output prefixed with:
	const expected = `

- wordpress/0: localhost (started) 80/tcp
  - logging/0: localhost (started)
`

	c.Assert(string(stdout), gc.Equals, expected[1:])
}

// Scenario: User filters out a parent, but not its subordinate
func (s *StatusSuite) TestFilterParentButNotSubordinate(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	// When I run juju status --format oneline 80/tcp
	_, stdout, stderr := runStatus(c, "--format", "oneline", "logging")
	c.Assert(stderr, gc.IsNil)
	// Then I should receive output prefixed with:
	const expected = `

- mysql/0: dummyenv-2.dns (started)
  - logging/1: dummyenv-2.dns (started)
- wordpress/0: dummyenv-1.dns (started)
  - logging/0: dummyenv-1.dns (started)
`

	c.Assert(string(stdout), gc.Equals, expected[1:])
}

// Scenario: User filters out a subordinate, but not its parent
func (s *StatusSuite) TestFilterSubordinateButNotParent(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	// Given the wordpress service is exposed
	setServiceExposed{"wordpress", true}.step(c, ctx)
	// When I run juju status --format oneline not exposed
	_, stdout, stderr := runStatus(c, "--format", "oneline", "not", "exposed")
	c.Assert(stderr, gc.IsNil)
	// Then I should receive output prefixed with:
	const expected = `

- mysql/0: dummyenv-2.dns (started)
  - logging/1: dummyenv-2.dns (started)
`

	c.Assert(string(stdout), gc.Equals, expected[1:])
}

func (s *StatusSuite) TestFilterMultipleHomogenousPatterns(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	_, stdout, stderr := runStatus(c, "--format", "oneline", "wordpress/0", "mysql/0")
	c.Assert(stderr, gc.IsNil)
	// Then I should receive output prefixed with:
	const expected = `

- mysql/0: dummyenv-2.dns (started)
  - logging/1: dummyenv-2.dns (started)
- wordpress/0: dummyenv-1.dns (started)
  - logging/0: dummyenv-1.dns (started)
`

	c.Assert(string(stdout), gc.Equals, expected[1:])
}

func (s *StatusSuite) TestFilterMultipleHeterogenousPatterns(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	_, stdout, stderr := runStatus(c, "--format", "oneline", "wordpress/0", "started")
	c.Assert(stderr, gc.IsNil)
	// Then I should receive output prefixed with:
	const expected = `

- mysql/0: dummyenv-2.dns (started)
  - logging/1: dummyenv-2.dns (started)
- wordpress/0: dummyenv-1.dns (started)
  - logging/0: dummyenv-1.dns (started)
`

	c.Assert(string(stdout), gc.Equals, expected[1:])
}
