// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/network"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/presence"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
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

type context struct {
	st      *state.State
	conn    *juju.Conn
	charms  map[string]*state.Charm
	pingers map[string]*presence.Pinger
}

func (s *StatusSuite) newContext() *context {
	st := s.Conn.Environ.(testing.GetStater).GetStateInAPIServer()
	// We make changes in the API server's state so that
	// our changes to presence are immediately noticed
	// in the status.
	return &context{
		st:      st,
		conn:    s.Conn,
		charms:  make(map[string]*state.Charm),
		pingers: make(map[string]*presence.Pinger),
	}
}

func (s *StatusSuite) resetContext(c *gc.C, ctx *context) {
	for _, up := range ctx.pingers {
		err := up.Kill()
		c.Check(err, gc.IsNil)
	}
	s.JujuConnSuite.Reset(c)
}

func (ctx *context) run(c *gc.C, steps []stepper) {
	for i, s := range steps {
		c.Logf("step %d", i)
		c.Logf("%#v", s)
		s.step(c, ctx)
	}
}

type aliver interface {
	AgentAlive() (bool, error)
	SetAgentAlive() (*presence.Pinger, error)
	WaitAgentAlive(time.Duration) error
}

func (ctx *context) setAgentAlive(c *gc.C, a aliver) *presence.Pinger {
	pinger, err := a.SetAgentAlive()
	c.Assert(err, gc.IsNil)
	ctx.st.StartSync()
	err = a.WaitAgentAlive(coretesting.LongWait)
	c.Assert(err, gc.IsNil)
	agentAlive, err := a.AgentAlive()
	c.Assert(err, gc.IsNil)
	c.Assert(agentAlive, gc.Equals, true)
	return pinger
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

		// unlikely, as you can't run juju status in real life without
		// machine/0 bootstrapped.
		expect{
			"empty state",
			M{
				"environment": "dummyenv",
				"machines":    M{},
				"services":    M{},
			},
		},

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
		setAddresses{"0", []instance.Address{
			instance.NewAddress("10.0.0.1", instance.NetworkUnknown),
			instance.NewAddress("dummyenv-0.dns", instance.NetworkPublic),
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

		setMachineStatus{"0", params.StatusStarted, ""},
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

		setTools{"0", version.MustParseBinary("1.2.3-gutsy-ppc")},
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
		setMachineStatus{"0", params.StatusStarted, ""},
		setAddresses{"0", []instance.Address{
			instance.NewAddress("10.0.0.1", instance.NetworkUnknown),
			instance.NewAddress("dummyenv-0.dns", instance.NetworkPublic),
		}},
		addCharm{"dummy"},
		addService{
			name:            "networks-service",
			charm:           "dummy",
			withNetworks:    []string{"net1", "net2"},
			withoutNetworks: []string{"net3", "net4"},
		},
		addService{
			name:            "no-networks-service",
			charm:           "dummy",
			withoutNetworks: []string{"mynet"},
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
							"disabled": L{"net3", "net4"},
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
		setAddresses{"0", []instance.Address{
			instance.NewAddress("10.0.0.1", instance.NetworkUnknown),
			instance.NewAddress("dummyenv-0.dns", instance.NetworkPublic),
		}},
		startAliveMachine{"0"},
		setMachineStatus{"0", params.StatusStarted, ""},
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
		setMachineStatus{"0", params.StatusStarted, ""},
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
		setAddresses{"0", []instance.Address{instance.NewAddress("dummyenv-0.dns", instance.NetworkUnknown)}},
		startAliveMachine{"0"},
		setMachineStatus{"0", params.StatusStarted, ""},
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
		setAddresses{"1", []instance.Address{instance.NewAddress("dummyenv-1.dns", instance.NetworkUnknown)}},
		startAliveMachine{"1"},
		setMachineStatus{"1", params.StatusStarted, ""},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", []instance.Address{instance.NewAddress("dummyenv-2.dns", instance.NetworkUnknown)}},
		startAliveMachine{"2"},
		setMachineStatus{"2", params.StatusStarted, ""},
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
		setUnitStatus{"exposed-service/0", params.StatusError, "You Require More Vespene Gas"},
		// Open multiple ports with different protocols,
		// ensure they're sorted on protocol, then number.
		openUnitPort{"exposed-service/0", "udp", 10},
		openUnitPort{"exposed-service/0", "udp", 2},
		openUnitPort{"exposed-service/0", "tcp", 3},
		openUnitPort{"exposed-service/0", "tcp", 2},
		// Simulate some status with no info, while the agent is down.
		setUnitStatus{"dummy-service/0", params.StatusStarted, ""},
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
		setAddresses{"3", []instance.Address{instance.NewAddress("dummyenv-3.dns", instance.NetworkUnknown)}},
		setMachineStatus{"3", params.StatusStopped, "Really?"},
		addMachine{machineId: "4", job: state.JobHostUnits},
		setAddresses{"4", []instance.Address{instance.NewAddress("dummyenv-4.dns", instance.NetworkUnknown)}},
		startAliveMachine{"4"},
		setMachineStatus{"4", params.StatusError, "Beware the red toys"},
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
		setAddresses{"0", []instance.Address{instance.NewAddress("dummyenv-0.dns", instance.NetworkUnknown)}},
		startAliveMachine{"0"},
		setMachineStatus{"0", params.StatusStarted, ""},
		addCharm{"wordpress"},
		addCharm{"mysql"},
		addCharm{"varnish"},

		addService{name: "project", charm: "wordpress"},
		setServiceExposed{"project", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", []instance.Address{instance.NewAddress("dummyenv-1.dns", instance.NetworkUnknown)}},
		startAliveMachine{"1"},
		setMachineStatus{"1", params.StatusStarted, ""},
		addAliveUnit{"project", "1"},
		setUnitStatus{"project/0", params.StatusStarted, ""},

		addService{name: "mysql", charm: "mysql"},
		setServiceExposed{"mysql", true},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", []instance.Address{instance.NewAddress("dummyenv-2.dns", instance.NetworkUnknown)}},
		startAliveMachine{"2"},
		setMachineStatus{"2", params.StatusStarted, ""},
		addAliveUnit{"mysql", "2"},
		setUnitStatus{"mysql/0", params.StatusStarted, ""},

		addService{name: "varnish", charm: "varnish"},
		setServiceExposed{"varnish", true},
		addMachine{machineId: "3", job: state.JobHostUnits},
		setAddresses{"3", []instance.Address{instance.NewAddress("dummyenv-3.dns", instance.NetworkUnknown)}},
		startAliveMachine{"3"},
		setMachineStatus{"3", params.StatusStarted, ""},
		addUnit{"varnish", "3"},

		addService{name: "private", charm: "wordpress"},
		setServiceExposed{"private", true},
		addMachine{machineId: "4", job: state.JobHostUnits},
		setAddresses{"4", []instance.Address{instance.NewAddress("dummyenv-4.dns", instance.NetworkUnknown)}},
		startAliveMachine{"4"},
		setMachineStatus{"4", params.StatusStarted, ""},
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
		setAddresses{"0", []instance.Address{instance.NewAddress("dummyenv-0.dns", instance.NetworkUnknown)}},
		startAliveMachine{"0"},
		setMachineStatus{"0", params.StatusStarted, ""},
		addCharm{"riak"},
		addCharm{"wordpress"},

		addService{name: "riak", charm: "riak"},
		setServiceExposed{"riak", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", []instance.Address{instance.NewAddress("dummyenv-1.dns", instance.NetworkUnknown)}},
		startAliveMachine{"1"},
		setMachineStatus{"1", params.StatusStarted, ""},
		addAliveUnit{"riak", "1"},
		setUnitStatus{"riak/0", params.StatusStarted, ""},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", []instance.Address{instance.NewAddress("dummyenv-2.dns", instance.NetworkUnknown)}},
		startAliveMachine{"2"},
		setMachineStatus{"2", params.StatusStarted, ""},
		addAliveUnit{"riak", "2"},
		setUnitStatus{"riak/1", params.StatusStarted, ""},
		addMachine{machineId: "3", job: state.JobHostUnits},
		setAddresses{"3", []instance.Address{instance.NewAddress("dummyenv-3.dns", instance.NetworkUnknown)}},
		startAliveMachine{"3"},
		setMachineStatus{"3", params.StatusStarted, ""},
		addAliveUnit{"riak", "3"},
		setUnitStatus{"riak/2", params.StatusStarted, ""},

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
		setAddresses{"0", []instance.Address{instance.NewAddress("dummyenv-0.dns", instance.NetworkUnknown)}},
		startAliveMachine{"0"},
		setMachineStatus{"0", params.StatusStarted, ""},
		addCharm{"wordpress"},
		addCharm{"mysql"},
		addCharm{"logging"},

		addService{name: "wordpress", charm: "wordpress"},
		setServiceExposed{"wordpress", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", []instance.Address{instance.NewAddress("dummyenv-1.dns", instance.NetworkUnknown)}},
		startAliveMachine{"1"},
		setMachineStatus{"1", params.StatusStarted, ""},
		addAliveUnit{"wordpress", "1"},
		setUnitStatus{"wordpress/0", params.StatusStarted, ""},

		addService{name: "mysql", charm: "mysql"},
		setServiceExposed{"mysql", true},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", []instance.Address{instance.NewAddress("dummyenv-2.dns", instance.NetworkUnknown)}},
		startAliveMachine{"2"},
		setMachineStatus{"2", params.StatusStarted, ""},
		addAliveUnit{"mysql", "2"},
		setUnitStatus{"mysql/0", params.StatusStarted, ""},

		addService{name: "logging", charm: "logging"},
		setServiceExposed{"logging", true},

		relateServices{"wordpress", "mysql"},
		relateServices{"wordpress", "logging"},
		relateServices{"mysql", "logging"},

		addSubordinate{"wordpress/0", "logging"},
		addSubordinate{"mysql/0", "logging"},

		setUnitsAlive{"logging"},
		setUnitStatus{"logging/0", params.StatusStarted, ""},
		setUnitStatus{"logging/1", params.StatusError, "somehow lost in all those logs"},

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
	), test(
		"one service with two subordinate services",
		addMachine{machineId: "0", job: state.JobManageEnviron},
		startAliveMachine{"0"},
		setMachineStatus{"0", params.StatusStarted, ""},
		addCharm{"wordpress"},
		addCharm{"logging"},
		addCharm{"monitoring"},

		addService{name: "wordpress", charm: "wordpress"},
		setServiceExposed{"wordpress", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", []instance.Address{instance.NewAddress("dummyenv-1.dns", instance.NetworkUnknown)}},
		startAliveMachine{"1"},
		setMachineStatus{"1", params.StatusStarted, ""},
		addAliveUnit{"wordpress", "1"},
		setUnitStatus{"wordpress/0", params.StatusStarted, ""},

		addService{name: "logging", charm: "logging"},
		setServiceExposed{"logging", true},
		addService{name: "monitoring", charm: "monitoring"},
		setServiceExposed{"monitoring", true},

		relateServices{"wordpress", "logging"},
		relateServices{"wordpress", "monitoring"},

		addSubordinate{"wordpress/0", "logging"},
		addSubordinate{"wordpress/0", "monitoring"},

		setUnitsAlive{"logging"},
		setUnitStatus{"logging/0", params.StatusStarted, ""},

		setUnitsAlive{"monitoring"},
		setUnitStatus{"monitoring/0", params.StatusStarted, ""},

		// scoped on monitoring; make sure logging doesn't show up.
		scopedExpect{
			"subordinates scoped on:",
			[]string{"monitoring"},
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
									"monitoring/0": M{
										"agent-state":    "started",
										"public-address": "dummyenv-1.dns",
									},
								},
								"public-address": "dummyenv-1.dns",
							},
						},
						"relations": M{
							"logging-dir":     L{"logging"},
							"monitoring-port": L{"monitoring"},
						},
					},
					"monitoring": M{
						"charm":   "cs:quantal/monitoring-0",
						"exposed": true,
						"relations": M{
							"monitoring-port": L{"wordpress"},
						},
						"subordinate-to": L{"wordpress"},
					},
				},
			},
		},
	), test(
		"machines with containers",
		addMachine{machineId: "0", job: state.JobManageEnviron},
		setAddresses{"0", []instance.Address{instance.NewAddress("dummyenv-0.dns", instance.NetworkUnknown)}},
		startAliveMachine{"0"},
		setMachineStatus{"0", params.StatusStarted, ""},
		addCharm{"mysql"},
		addService{name: "mysql", charm: "mysql"},
		setServiceExposed{"mysql", true},

		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", []instance.Address{instance.NewAddress("dummyenv-1.dns", instance.NetworkUnknown)}},
		startAliveMachine{"1"},
		setMachineStatus{"1", params.StatusStarted, ""},
		addAliveUnit{"mysql", "1"},
		setUnitStatus{"mysql/0", params.StatusStarted, ""},

		// A container on machine 1.
		addContainer{"1", "1/lxc/0", state.JobHostUnits},
		setAddresses{"1/lxc/0", []instance.Address{instance.NewAddress("dummyenv-2.dns", instance.NetworkUnknown)}},
		startAliveMachine{"1/lxc/0"},
		setMachineStatus{"1/lxc/0", params.StatusStarted, ""},
		addAliveUnit{"mysql", "1/lxc/0"},
		setUnitStatus{"mysql/1", params.StatusStarted, ""},
		addContainer{"1", "1/lxc/1", state.JobHostUnits},

		// A nested container.
		addContainer{"1/lxc/0", "1/lxc/0/lxc/0", state.JobHostUnits},
		setAddresses{"1/lxc/0/lxc/0", []instance.Address{instance.NewAddress("dummyenv-3.dns", instance.NetworkUnknown)}},
		startAliveMachine{"1/lxc/0/lxc/0"},
		setMachineStatus{"1/lxc/0/lxc/0", params.StatusStarted, ""},

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
					"1": machine1WithContainersScoped,
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
		setAddresses{"0", []instance.Address{instance.NewAddress("dummyenv-0.dns", instance.NetworkUnknown)}},
		startAliveMachine{"0"},
		setMachineStatus{"0", params.StatusStarted, ""},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", []instance.Address{instance.NewAddress("dummyenv-1.dns", instance.NetworkUnknown)}},
		startAliveMachine{"1"},
		setMachineStatus{"1", params.StatusStarted, ""},
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
		setAddresses{"0", []instance.Address{instance.NewAddress("dummyenv-0.dns", instance.NetworkUnknown)}},
		startAliveMachine{"0"},
		setMachineStatus{"0", params.StatusStarted, ""},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", []instance.Address{instance.NewAddress("dummyenv-1.dns", instance.NetworkUnknown)}},
		startAliveMachine{"1"},
		setMachineStatus{"1", params.StatusStarted, ""},
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
		setAddresses{"0", []instance.Address{instance.NewAddress("dummyenv-0.dns", instance.NetworkUnknown)}},
		startAliveMachine{"0"},
		setMachineStatus{"0", params.StatusStarted, ""},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", []instance.Address{instance.NewAddress("dummyenv-1.dns", instance.NetworkUnknown)}},
		startAliveMachine{"1"},
		setMachineStatus{"1", params.StatusStarted, ""},
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
		setAddresses{"0", []instance.Address{instance.NewAddress("dummyenv-0.dns", instance.NetworkUnknown)}},
		startAliveMachine{"0"},
		setMachineStatus{"0", params.StatusStarted, ""},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", []instance.Address{instance.NewAddress("dummyenv-1.dns", instance.NetworkUnknown)}},
		startAliveMachine{"1"},
		setMachineStatus{"1", params.StatusStarted, ""},
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
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)
	c.Assert(m.Id(), gc.Equals, ac.machineId)
}

type startMachine struct {
	machineId string
}

func (sm startMachine) step(c *gc.C, ctx *context) {
	m, err := ctx.st.Machine(sm.machineId)
	c.Assert(err, gc.IsNil)
	cons, err := m.Constraints()
	c.Assert(err, gc.IsNil)
	inst, hc := testing.AssertStartInstanceWithConstraints(c, ctx.conn.Environ, m.Id(), cons)
	err = m.SetProvisioned(inst.Id(), "fake_nonce", hc)
	c.Assert(err, gc.IsNil)
}

type startMissingMachine struct {
	machineId string
}

func (sm startMissingMachine) step(c *gc.C, ctx *context) {
	m, err := ctx.st.Machine(sm.machineId)
	c.Assert(err, gc.IsNil)
	cons, err := m.Constraints()
	c.Assert(err, gc.IsNil)
	_, hc := testing.AssertStartInstanceWithConstraints(c, ctx.conn.Environ, m.Id(), cons)
	err = m.SetProvisioned("i-missing", "fake_nonce", hc)
	c.Assert(err, gc.IsNil)
	err = m.SetInstanceStatus("missing")
	c.Assert(err, gc.IsNil)
}

type startAliveMachine struct {
	machineId string
}

func (sam startAliveMachine) step(c *gc.C, ctx *context) {
	m, err := ctx.st.Machine(sam.machineId)
	c.Assert(err, gc.IsNil)
	pinger := ctx.setAgentAlive(c, m)
	cons, err := m.Constraints()
	c.Assert(err, gc.IsNil)
	inst, hc := testing.AssertStartInstanceWithConstraints(c, ctx.conn.Environ, m.Id(), cons)
	err = m.SetProvisioned(inst.Id(), "fake_nonce", hc)
	c.Assert(err, gc.IsNil)
	ctx.pingers[m.Id()] = pinger
}

type setAddresses struct {
	machineId string
	addresses []instance.Address
}

func (sa setAddresses) step(c *gc.C, ctx *context) {
	m, err := ctx.st.Machine(sa.machineId)
	c.Assert(err, gc.IsNil)
	err = m.SetAddresses(sa.addresses...)
	c.Assert(err, gc.IsNil)
}

type setTools struct {
	machineId string
	version   version.Binary
}

func (st setTools) step(c *gc.C, ctx *context) {
	m, err := ctx.st.Machine(st.machineId)
	c.Assert(err, gc.IsNil)
	err = m.SetAgentVersion(st.version)
	c.Assert(err, gc.IsNil)
}

type addCharm struct {
	name string
}

func (ac addCharm) addCharmStep(c *gc.C, ctx *context, scheme string, rev int) {
	ch := coretesting.Charms.Dir(ac.name)
	name := ch.Meta().Name
	curl := charm.MustParseURL(fmt.Sprintf("%s:quantal/%s-%d", scheme, name, rev))
	bundleURL, err := url.Parse(fmt.Sprintf("http://bundles.testing.invalid/%s-%d", name, rev))
	c.Assert(err, gc.IsNil)
	dummy, err := ctx.st.AddCharm(ch, curl, bundleURL, fmt.Sprintf("%s-%d-sha256", name, rev))
	c.Assert(err, gc.IsNil)
	ctx.charms[ac.name] = dummy
}

func (ac addCharm) step(c *gc.C, ctx *context) {
	ch := coretesting.Charms.Dir(ac.name)
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
	name            string
	charm           string
	withNetworks    []string
	withoutNetworks []string
}

func (as addService) step(c *gc.C, ctx *context) {
	ch, ok := ctx.charms[as.charm]
	c.Assert(ok, gc.Equals, true)
	_, err := ctx.st.AddService(as.name, "user-admin", ch, as.withNetworks, as.withoutNetworks)
	c.Assert(err, gc.IsNil)
}

type setServiceExposed struct {
	name    string
	exposed bool
}

func (sse setServiceExposed) step(c *gc.C, ctx *context) {
	s, err := ctx.st.Service(sse.name)
	c.Assert(err, gc.IsNil)
	if sse.exposed {
		err = s.SetExposed()
		c.Assert(err, gc.IsNil)
	}
}

type setServiceCharm struct {
	name  string
	charm string
}

func (ssc setServiceCharm) step(c *gc.C, ctx *context) {
	ch, err := ctx.st.Charm(charm.MustParseURL(ssc.charm))
	c.Assert(err, gc.IsNil)
	s, err := ctx.st.Service(ssc.name)
	c.Assert(err, gc.IsNil)
	err = s.SetCharm(ch, false)
	c.Assert(err, gc.IsNil)
}

type addCharmPlaceholder struct {
	name string
	rev  int
}

func (ac addCharmPlaceholder) step(c *gc.C, ctx *context) {
	ch := coretesting.Charms.Dir(ac.name)
	name := ch.Meta().Name
	curl := charm.MustParseURL(fmt.Sprintf("cs:quantal/%s-%d", name, ac.rev))
	err := ctx.st.AddStoreCharmPlaceholder(curl)
	c.Assert(err, gc.IsNil)
}

type addUnit struct {
	serviceName string
	machineId   string
}

func (au addUnit) step(c *gc.C, ctx *context) {
	s, err := ctx.st.Service(au.serviceName)
	c.Assert(err, gc.IsNil)
	u, err := s.AddUnit()
	c.Assert(err, gc.IsNil)
	m, err := ctx.st.Machine(au.machineId)
	c.Assert(err, gc.IsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, gc.IsNil)
}

type addAliveUnit struct {
	serviceName string
	machineId   string
}

func (aau addAliveUnit) step(c *gc.C, ctx *context) {
	s, err := ctx.st.Service(aau.serviceName)
	c.Assert(err, gc.IsNil)
	u, err := s.AddUnit()
	c.Assert(err, gc.IsNil)
	pinger := ctx.setAgentAlive(c, u)
	m, err := ctx.st.Machine(aau.machineId)
	c.Assert(err, gc.IsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, gc.IsNil)
	ctx.pingers[u.Name()] = pinger
}

type setUnitsAlive struct {
	serviceName string
}

func (sua setUnitsAlive) step(c *gc.C, ctx *context) {
	s, err := ctx.st.Service(sua.serviceName)
	c.Assert(err, gc.IsNil)
	us, err := s.AllUnits()
	c.Assert(err, gc.IsNil)
	for _, u := range us {
		ctx.pingers[u.Name()] = ctx.setAgentAlive(c, u)
	}
}

type setUnitStatus struct {
	unitName   string
	status     params.Status
	statusInfo string
}

func (sus setUnitStatus) step(c *gc.C, ctx *context) {
	u, err := ctx.st.Unit(sus.unitName)
	c.Assert(err, gc.IsNil)
	err = u.SetStatus(sus.status, sus.statusInfo, nil)
	c.Assert(err, gc.IsNil)
}

type setUnitCharmURL struct {
	unitName string
	charm    string
}

func (uc setUnitCharmURL) step(c *gc.C, ctx *context) {
	u, err := ctx.st.Unit(uc.unitName)
	c.Assert(err, gc.IsNil)
	curl := charm.MustParseURL(uc.charm)
	err = u.SetCharmURL(curl)
	c.Assert(err, gc.IsNil)
	err = u.SetStatus(params.StatusStarted, "", nil)
	c.Assert(err, gc.IsNil)
}

type openUnitPort struct {
	unitName string
	protocol string
	number   int
}

func (oup openUnitPort) step(c *gc.C, ctx *context) {
	u, err := ctx.st.Unit(oup.unitName)
	c.Assert(err, gc.IsNil)
	err = u.OpenPort(oup.protocol, oup.number)
	c.Assert(err, gc.IsNil)
}

type ensureDyingUnit struct {
	unitName string
}

func (e ensureDyingUnit) step(c *gc.C, ctx *context) {
	u, err := ctx.st.Unit(e.unitName)
	c.Assert(err, gc.IsNil)
	err = u.Destroy()
	c.Assert(err, gc.IsNil)
	c.Assert(u.Life(), gc.Equals, state.Dying)
}

type ensureDyingService struct {
	serviceName string
}

func (e ensureDyingService) step(c *gc.C, ctx *context) {
	svc, err := ctx.st.Service(e.serviceName)
	c.Assert(err, gc.IsNil)
	err = svc.Destroy()
	c.Assert(err, gc.IsNil)
	err = svc.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(svc.Life(), gc.Equals, state.Dying)
}

type ensureDeadMachine struct {
	machineId string
}

func (e ensureDeadMachine) step(c *gc.C, ctx *context) {
	m, err := ctx.st.Machine(e.machineId)
	c.Assert(err, gc.IsNil)
	err = m.EnsureDead()
	c.Assert(err, gc.IsNil)
	c.Assert(m.Life(), gc.Equals, state.Dead)
}

type setMachineStatus struct {
	machineId  string
	status     params.Status
	statusInfo string
}

func (sms setMachineStatus) step(c *gc.C, ctx *context) {
	m, err := ctx.st.Machine(sms.machineId)
	c.Assert(err, gc.IsNil)
	err = m.SetStatus(sms.status, sms.statusInfo, nil)
	c.Assert(err, gc.IsNil)
}

type relateServices struct {
	ep1, ep2 string
}

func (rs relateServices) step(c *gc.C, ctx *context) {
	eps, err := ctx.st.InferEndpoints([]string{rs.ep1, rs.ep2})
	c.Assert(err, gc.IsNil)
	_, err = ctx.st.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
}

type addSubordinate struct {
	prinUnit   string
	subService string
}

func (as addSubordinate) step(c *gc.C, ctx *context) {
	u, err := ctx.st.Unit(as.prinUnit)
	c.Assert(err, gc.IsNil)
	eps, err := ctx.st.InferEndpoints([]string{u.ServiceName(), as.subService})
	c.Assert(err, gc.IsNil)
	rel, err := ctx.st.EndpointsRelation(eps...)
	c.Assert(err, gc.IsNil)
	ru, err := rel.Unit(u)
	c.Assert(err, gc.IsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, gc.IsNil)
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
		code, stdout, stderr := runStatus(c, args...)
		c.Assert(code, gc.Equals, 0)
		if !c.Check(stderr, gc.HasLen, 0) {
			c.Fatalf("status failed: %s", string(stderr))
		}

		// Prepare the output in the same format.
		buf, err := format.marshal(e.output)
		c.Assert(err, gc.IsNil)
		expected := make(M)
		err = format.unmarshal(buf, &expected)
		c.Assert(err, gc.IsNil)

		// Check the output is as expected.
		actual := make(M)
		err = format.unmarshal(stdout, &actual)
		c.Assert(err, gc.IsNil)
		c.Assert(actual, jc.DeepEquals, expected)
	}
}

func (e expect) step(c *gc.C, ctx *context) {
	scopedExpect{e.what, nil, e.output}.step(c, ctx)
}

func (s *StatusSuite) TestStatusAllFormats(c *gc.C) {
	for i, t := range statusTests {
		c.Logf("test %d: %s", i, t.summary)
		func() {
			// Prepare context and run all steps to setup.
			ctx := s.newContext()
			defer s.resetContext(c, ctx)
			ctx.run(c, t.steps)
		}()
	}
}

func (s *StatusSuite) TestStatusFilterErrors(c *gc.C) {
	steps := []stepper{
		addMachine{machineId: "0", job: state.JobManageEnviron},
		addMachine{machineId: "1", job: state.JobHostUnits},
		addCharm{"mysql"},
		addService{name: "mysql", charm: "mysql"},
		addAliveUnit{"mysql", "1"},
	}
	ctx := s.newContext()
	defer s.resetContext(c, ctx)
	ctx.run(c, steps)

	// Status filters can only fail if the patterns are invalid.
	code, _, stderr := runStatus(c, "[*")
	c.Assert(code, gc.Not(gc.Equals), 0)
	c.Assert(string(stderr), gc.Equals, `error: pattern "[*" contains invalid characters`+"\n")

	code, _, stderr = runStatus(c, "//")
	c.Assert(code, gc.Not(gc.Equals), 0)
	c.Assert(string(stderr), gc.Equals, `error: pattern "//" contains too many '/' characters`+"\n")

	// Pattern validity is checked eagerly; if a bad pattern
	// proceeds a valid, matching pattern, then the bad pattern
	// will still cause an error.
	code, _, stderr = runStatus(c, "*", "[*")
	c.Assert(code, gc.Not(gc.Equals), 0)
	c.Assert(string(stderr), gc.Equals, `error: pattern "[*" contains invalid characters`+"\n")
}
