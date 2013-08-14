// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/presence"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
)

func runStatus(c *C, args ...string) (code int, stdout, stderr []byte) {
	ctx := coretesting.Context(c)
	code = cmd.Main(&StatusCommand{}, ctx, args)
	stdout = ctx.Stdout.(*bytes.Buffer).Bytes()
	stderr = ctx.Stderr.(*bytes.Buffer).Bytes()
	return
}

type StatusSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&StatusSuite{})

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
	step(c *C, ctx *context)
}

type context struct {
	st      *state.State
	conn    *juju.Conn
	charms  map[string]*state.Charm
	pingers map[string]*presence.Pinger
}

func (s *StatusSuite) newContext() *context {
	return &context{
		st:      s.State,
		conn:    s.Conn,
		charms:  make(map[string]*state.Charm),
		pingers: make(map[string]*presence.Pinger),
	}
}

func (s *StatusSuite) resetContext(c *C, ctx *context) {
	for _, up := range ctx.pingers {
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

// shortcuts for expected output.
var (
	machine0 = M{
		"agent-state": "started",
		"dns-name":    "dummyenv-0.dns",
		"instance-id": "dummyenv-0",
		"series":      "series",
		"hardware":    "arch=amd64 cpu-cores=1 mem=1024M os-disk=8192M",
	}
	machine1 = M{
		"agent-state": "started",
		"dns-name":    "dummyenv-1.dns",
		"instance-id": "dummyenv-1",
		"series":      "series",
		"hardware":    "arch=amd64 cpu-cores=1 mem=1024M os-disk=8192M",
	}
	machine2 = M{
		"agent-state": "started",
		"dns-name":    "dummyenv-2.dns",
		"instance-id": "dummyenv-2",
		"series":      "series",
		"hardware":    "arch=amd64 cpu-cores=1 mem=1024M os-disk=8192M",
	}
	machine3 = M{
		"agent-state": "started",
		"dns-name":    "dummyenv-3.dns",
		"instance-id": "dummyenv-3",
		"series":      "series",
		"hardware":    "arch=amd64 cpu-cores=1 mem=1024M os-disk=8192M",
	}
	machine4 = M{
		"agent-state": "started",
		"dns-name":    "dummyenv-4.dns",
		"instance-id": "dummyenv-4",
		"series":      "series",
		"hardware":    "arch=amd64 cpu-cores=1 mem=1024M os-disk=8192M",
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
						"series":      "series",
					},
				},
				"dns-name":    "dummyenv-2.dns",
				"instance-id": "dummyenv-2",
				"series":      "series",
			},
			"1/lxc/1": M{
				"instance-id": "pending",
				"series":      "series",
			},
		},
		"dns-name":    "dummyenv-1.dns",
		"instance-id": "dummyenv-1",
		"series":      "series",
		"hardware":    "arch=amd64 cpu-cores=1 mem=1024M os-disk=8192M",
	}
	machine1WithContainersScoped = M{
		"agent-state": "started",
		"containers": M{
			"1/lxc/0": M{
				"agent-state": "started",
				"dns-name":    "dummyenv-2.dns",
				"instance-id": "dummyenv-2",
				"series":      "series",
			},
		},
		"dns-name":    "dummyenv-1.dns",
		"instance-id": "dummyenv-1",
		"series":      "series",
		"hardware":    "arch=amd64 cpu-cores=1 mem=1024M os-disk=8192M",
	}
	unexposedService = M{
		"charm":   "local:series/dummy-1",
		"exposed": false,
	}
	exposedService = M{
		"charm":   "local:series/dummy-1",
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

var machineCons = constraints.MustParse("cpu-cores=2 mem=8G os-disk=8G")

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
						"instance-id": "pending",
						"series":      "series",
					},
				},
				"services": M{},
			},
		},

		startAliveMachine{"0"},
		expect{
			"simulate the PA starting an instance in response to the state change",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": M{
						"agent-state": "pending",
						"dns-name":    "dummyenv-0.dns",
						"instance-id": "dummyenv-0",
						"series":      "series",
						"hardware":    "arch=amd64 cpu-cores=1 mem=1024M os-disk=8192M",
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

		setTools{"0", &tools.Tools{
			Version: version.Binary{
				Number: version.MustParse("1.2.3"),
				Series: "gutsy",
				Arch:   "ppc",
			},
			URL: "http://canonical.com/",
		}},
		expect{
			"simulate the MA setting the version",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": M{
						"dns-name":      "dummyenv-0.dns",
						"instance-id":   "dummyenv-0",
						"agent-version": "1.2.3",
						"agent-state":   "started",
						"series":        "series",
						"hardware":      "arch=amd64 cpu-cores=1 mem=1024M os-disk=8192M",
					},
				},
				"services": M{},
			},
		},
	), test(
		"instance with different hardware characteristics",
		addMachine{"0", machineCons, state.JobManageEnviron},
		startAliveMachine{"0"},
		setMachineStatus{"0", params.StatusStarted, ""},
		expect{
			"machine 0 has specific hardware characteristics",
			M{
				"environment": "dummyenv",
				"machines": M{
					"0": M{
						"agent-state": "started",
						"dns-name":    "dummyenv-0.dns",
						"instance-id": "dummyenv-0",
						"series":      "series",
						"hardware":    "arch=amd64 cpu-cores=2 mem=8192M os-disk=8192M",
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
						"instance-id": "pending",
						"series":      "series",
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
						"instance-state": "missing",
						"instance-id":    "i-missing",
						"agent-state":    "pending",
						"series":         "series",
						"hardware":       "arch=amd64 cpu-cores=1 mem=1024M os-disk=8192M",
					},
				},
				"services": M{},
			},
		},
	), test(
		"add two services and expose one, then add 2 more machines and some units",
		addMachine{machineId: "0", job: state.JobManageEnviron},
		startAliveMachine{"0"},
		setMachineStatus{"0", params.StatusStarted, ""},
		addCharm{"dummy"},
		addService{"dummy-service", "dummy"},
		addService{"exposed-service", "dummy"},
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
		startAliveMachine{"1"},
		setMachineStatus{"1", params.StatusStarted, ""},
		addMachine{machineId: "2", job: state.JobHostUnits},
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
						"charm":   "local:series/dummy-1",
						"exposed": true,
						"units": M{
							"exposed-service/0": M{
								"machine":          "2",
								"agent-state":      "error",
								"agent-state-info": "You Require More Vespene Gas",
								"open-ports": L{
									"2/tcp", "3/tcp", "2/udp", "10/udp",
								},
							},
						},
					},
					"dummy-service": M{
						"charm":   "local:series/dummy-1",
						"exposed": false,
						"units": M{
							"dummy-service/0": M{
								"machine":          "1",
								"agent-state":      "down",
								"agent-state-info": "(started)",
							},
						},
					},
				},
			},
		},

		addMachine{machineId: "3", job: state.JobHostUnits},
		startMachine{"3"},
		// Simulate some status with info, while the agent is down.
		setMachineStatus{"3", params.StatusStopped, "Really?"},
		addMachine{machineId: "4", job: state.JobHostUnits},
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
						"series":           "series",
						"hardware":         "arch=amd64 cpu-cores=1 mem=1024M os-disk=8192M",
					},
					"4": M{
						"dns-name":         "dummyenv-4.dns",
						"instance-id":      "dummyenv-4",
						"agent-state":      "error",
						"agent-state-info": "Beware the red toys",
						"series":           "series",
						"hardware":         "arch=amd64 cpu-cores=1 mem=1024M os-disk=8192M",
					},
					"5": M{
						"life":        "dead",
						"instance-id": "pending",
						"series":      "series",
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
								"open-ports": L{
									"2/tcp", "3/tcp", "2/udp", "10/udp",
								},
							},
						},
					},
					"dummy-service": M{
						"charm":   "local:series/dummy-1",
						"exposed": false,
						"units": M{
							"dummy-service/0": M{
								"machine":          "1",
								"life":             "dying",
								"agent-state":      "down",
								"agent-state-info": "(started)",
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
						"charm":   "local:series/dummy-1",
						"exposed": false,
						"units": M{
							"dummy-service/0": M{
								"machine":          "1",
								"life":             "dying",
								"agent-state":      "down",
								"agent-state-info": "(started)",
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
						"charm":   "local:series/dummy-1",
						"exposed": true,
						"units": M{
							"exposed-service/0": M{
								"machine":          "2",
								"agent-state":      "error",
								"agent-state-info": "You Require More Vespene Gas",
								"open-ports": L{
									"2/tcp", "3/tcp", "2/udp", "10/udp",
								},
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
						"charm":   "local:series/dummy-1",
						"exposed": false,
						"units": M{
							"dummy-service/0": M{
								"machine":          "1",
								"life":             "dying",
								"agent-state":      "down",
								"agent-state-info": "(started)",
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
						"charm":   "local:series/dummy-1",
						"exposed": true,
						"units": M{
							"exposed-service/0": M{
								"machine":          "2",
								"agent-state":      "error",
								"agent-state-info": "You Require More Vespene Gas",
								"open-ports": L{
									"2/tcp", "3/tcp", "2/udp", "10/udp",
								},
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
						"charm":   "local:series/dummy-1",
						"exposed": false,
						"units": M{
							"dummy-service/0": M{
								"machine":          "1",
								"life":             "dying",
								"agent-state":      "down",
								"agent-state-info": "(started)",
							},
						},
					},
					"exposed-service": M{
						"charm":   "local:series/dummy-1",
						"exposed": true,
						"units": M{
							"exposed-service/0": M{
								"machine":          "2",
								"agent-state":      "error",
								"agent-state-info": "You Require More Vespene Gas",
								"open-ports": L{
									"2/tcp", "3/tcp", "2/udp", "10/udp",
								},
							},
						},
					},
				},
			},
		},
	),
	test(
		"add a dying service",
		addCharm{"dummy"},
		addService{"dummy-service", "dummy"},
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
						"series":      "series",
					},
				},
				"services": M{
					"dummy-service": M{
						"charm":   "local:series/dummy-1",
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
		startAliveMachine{"0"},
		setMachineStatus{"0", params.StatusStarted, ""},
		addCharm{"wordpress"},
		addCharm{"mysql"},
		addCharm{"varnish"},

		addService{"project", "wordpress"},
		setServiceExposed{"project", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		startAliveMachine{"1"},
		setMachineStatus{"1", params.StatusStarted, ""},
		addAliveUnit{"project", "1"},
		setUnitStatus{"project/0", params.StatusStarted, ""},

		addService{"mysql", "mysql"},
		setServiceExposed{"mysql", true},
		addMachine{machineId: "2", job: state.JobHostUnits},
		startAliveMachine{"2"},
		setMachineStatus{"2", params.StatusStarted, ""},
		addAliveUnit{"mysql", "2"},
		setUnitStatus{"mysql/0", params.StatusStarted, ""},

		addService{"varnish", "varnish"},
		setServiceExposed{"varnish", true},
		addMachine{machineId: "3", job: state.JobHostUnits},
		startAliveMachine{"3"},
		setMachineStatus{"3", params.StatusStarted, ""},
		addUnit{"varnish", "3"},

		addService{"private", "wordpress"},
		setServiceExposed{"private", true},
		addMachine{machineId: "4", job: state.JobHostUnits},
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
						"charm":   "local:series/wordpress-3",
						"exposed": true,
						"units": M{
							"project/0": M{
								"machine":     "1",
								"agent-state": "started",
							},
						},
						"relations": M{
							"db":    L{"mysql"},
							"cache": L{"varnish"},
						},
					},
					"mysql": M{
						"charm":   "local:series/mysql-1",
						"exposed": true,
						"units": M{
							"mysql/0": M{
								"machine":     "2",
								"agent-state": "started",
							},
						},
						"relations": M{
							"server": L{"private", "project"},
						},
					},
					"varnish": M{
						"charm":   "local:series/varnish-1",
						"exposed": true,
						"units": M{
							"varnish/0": M{
								"machine":     "3",
								"agent-state": "pending",
							},
						},
						"relations": M{
							"webcache": L{"project"},
						},
					},
					"private": M{
						"charm":   "local:series/wordpress-3",
						"exposed": true,
						"units": M{
							"private/0": M{
								"machine":     "4",
								"agent-state": "pending",
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
		startAliveMachine{"0"},
		setMachineStatus{"0", params.StatusStarted, ""},
		addCharm{"riak"},
		addCharm{"wordpress"},

		addService{"riak", "riak"},
		setServiceExposed{"riak", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		startAliveMachine{"1"},
		setMachineStatus{"1", params.StatusStarted, ""},
		addAliveUnit{"riak", "1"},
		setUnitStatus{"riak/0", params.StatusStarted, ""},
		addMachine{machineId: "2", job: state.JobHostUnits},
		startAliveMachine{"2"},
		setMachineStatus{"2", params.StatusStarted, ""},
		addAliveUnit{"riak", "2"},
		setUnitStatus{"riak/1", params.StatusStarted, ""},
		addMachine{machineId: "3", job: state.JobHostUnits},
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
						"charm":   "local:series/riak-7",
						"exposed": true,
						"units": M{
							"riak/0": M{
								"machine":     "1",
								"agent-state": "started",
							},
							"riak/1": M{
								"machine":     "2",
								"agent-state": "started",
							},
							"riak/2": M{
								"machine":     "3",
								"agent-state": "started",
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
		startAliveMachine{"0"},
		setMachineStatus{"0", params.StatusStarted, ""},
		addCharm{"wordpress"},
		addCharm{"mysql"},
		addCharm{"logging"},

		addService{"wordpress", "wordpress"},
		setServiceExposed{"wordpress", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		startAliveMachine{"1"},
		setMachineStatus{"1", params.StatusStarted, ""},
		addAliveUnit{"wordpress", "1"},
		setUnitStatus{"wordpress/0", params.StatusStarted, ""},

		addService{"mysql", "mysql"},
		setServiceExposed{"mysql", true},
		addMachine{machineId: "2", job: state.JobHostUnits},
		startAliveMachine{"2"},
		setMachineStatus{"2", params.StatusStarted, ""},
		addAliveUnit{"mysql", "2"},
		setUnitStatus{"mysql/0", params.StatusStarted, ""},

		addService{"logging", "logging"},
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
						"charm":   "local:series/wordpress-3",
						"exposed": true,
						"units": M{
							"wordpress/0": M{
								"machine":     "1",
								"agent-state": "started",
								"subordinates": M{
									"logging/0": M{
										"agent-state": "started",
									},
								},
							},
						},
						"relations": M{
							"db":          L{"mysql"},
							"logging-dir": L{"logging"},
						},
					},
					"mysql": M{
						"charm":   "local:series/mysql-1",
						"exposed": true,
						"units": M{
							"mysql/0": M{
								"machine":     "2",
								"agent-state": "started",
								"subordinates": M{
									"logging/1": M{
										"agent-state":      "error",
										"agent-state-info": "somehow lost in all those logs",
									},
								},
							},
						},
						"relations": M{
							"server":    L{"wordpress"},
							"juju-info": L{"logging"},
						},
					},
					"logging": M{
						"charm":   "local:series/logging-1",
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
						"charm":   "local:series/wordpress-3",
						"exposed": true,
						"units": M{
							"wordpress/0": M{
								"machine":     "1",
								"agent-state": "started",
								"subordinates": M{
									"logging/0": M{
										"agent-state": "started",
									},
								},
							},
						},
						"relations": M{
							"db":          L{"mysql"},
							"logging-dir": L{"logging"},
						},
					},
					"mysql": M{
						"charm":   "local:series/mysql-1",
						"exposed": true,
						"units": M{
							"mysql/0": M{
								"machine":     "2",
								"agent-state": "started",
								"subordinates": M{
									"logging/1": M{
										"agent-state":      "error",
										"agent-state-info": "somehow lost in all those logs",
									},
								},
							},
						},
						"relations": M{
							"server":    L{"wordpress"},
							"juju-info": L{"logging"},
						},
					},
					"logging": M{
						"charm":   "local:series/logging-1",
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
						"charm":   "local:series/wordpress-3",
						"exposed": true,
						"units": M{
							"wordpress/0": M{
								"machine":     "1",
								"agent-state": "started",
								"subordinates": M{
									"logging/0": M{
										"agent-state": "started",
									},
								},
							},
						},
						"relations": M{
							"db":          L{"mysql"},
							"logging-dir": L{"logging"},
						},
					},
					"logging": M{
						"charm":   "local:series/logging-1",
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

		addService{"wordpress", "wordpress"},
		setServiceExposed{"wordpress", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		startAliveMachine{"1"},
		setMachineStatus{"1", params.StatusStarted, ""},
		addAliveUnit{"wordpress", "1"},
		setUnitStatus{"wordpress/0", params.StatusStarted, ""},

		addService{"logging", "logging"},
		setServiceExposed{"logging", true},
		addService{"monitoring", "monitoring"},
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
						"charm":   "local:series/wordpress-3",
						"exposed": true,
						"units": M{
							"wordpress/0": M{
								"machine":     "1",
								"agent-state": "started",
								"subordinates": M{
									"monitoring/0": M{
										"agent-state": "started",
									},
								},
							},
						},
						"relations": M{
							"logging-dir":     L{"logging"},
							"monitoring-port": L{"monitoring"},
						},
					},
					"monitoring": M{
						"charm":   "local:series/monitoring-0",
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
		startAliveMachine{"0"},
		setMachineStatus{"0", params.StatusStarted, ""},
		addCharm{"mysql"},
		addService{"mysql", "mysql"},
		setServiceExposed{"mysql", true},

		addMachine{machineId: "1", job: state.JobHostUnits},
		startAliveMachine{"1"},
		setMachineStatus{"1", params.StatusStarted, ""},
		addAliveUnit{"mysql", "1"},
		setUnitStatus{"mysql/0", params.StatusStarted, ""},

		// A container on machine 1.
		addContainer{"1", "1/lxc/0", state.JobHostUnits},
		startAliveMachine{"1/lxc/0"},
		setMachineStatus{"1/lxc/0", params.StatusStarted, ""},
		addAliveUnit{"mysql", "1/lxc/0"},
		setUnitStatus{"mysql/1", params.StatusStarted, ""},
		addContainer{"1", "1/lxc/1", state.JobHostUnits},

		// A nested container.
		addContainer{"1/lxc/0", "1/lxc/0/lxc/0", state.JobHostUnits},
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
						"charm":   "local:series/mysql-1",
						"exposed": true,
						"units": M{
							"mysql/0": M{
								"machine":     "1",
								"agent-state": "started",
							},
							"mysql/1": M{
								"machine":     "1/lxc/0",
								"agent-state": "started",
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
						"charm":   "local:series/mysql-1",
						"exposed": true,
						"units": M{
							"mysql/1": M{
								"machine":     "1/lxc/0",
								"agent-state": "started",
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

func (am addMachine) step(c *C, ctx *context) {
	params := &state.AddMachineParams{
		Series:      "series",
		Constraints: am.cons,
		Jobs:        []state.MachineJob{am.job},
	}
	m, err := ctx.st.AddMachineWithConstraints(params)
	c.Assert(err, IsNil)
	c.Assert(m.Id(), Equals, am.machineId)
}

type addContainer struct {
	parentId  string
	machineId string
	job       state.MachineJob
}

func (ac addContainer) step(c *C, ctx *context) {
	params := &state.AddMachineParams{
		ParentId:      ac.parentId,
		ContainerType: instance.LXC,
		Series:        "series",
		Jobs:          []state.MachineJob{ac.job},
	}
	m, err := ctx.st.AddMachineWithConstraints(params)
	c.Assert(err, IsNil)
	c.Assert(m.Id(), Equals, ac.machineId)
}

type startMachine struct {
	machineId string
}

func (sm startMachine) step(c *C, ctx *context) {
	m, err := ctx.st.Machine(sm.machineId)
	c.Assert(err, IsNil)
	cons, err := m.Constraints()
	c.Assert(err, IsNil)
	inst, hc := testing.StartInstanceWithConstraints(c, ctx.conn.Environ, m.Id(), cons)
	err = m.SetProvisioned(inst.Id(), "fake_nonce", hc)
	c.Assert(err, IsNil)
}

type startMissingMachine struct {
	machineId string
}

func (sm startMissingMachine) step(c *C, ctx *context) {
	m, err := ctx.st.Machine(sm.machineId)
	c.Assert(err, IsNil)
	cons, err := m.Constraints()
	c.Assert(err, IsNil)
	_, hc := testing.StartInstanceWithConstraints(c, ctx.conn.Environ, m.Id(), cons)
	err = m.SetProvisioned("i-missing", "fake_nonce", hc)
	c.Assert(err, IsNil)
}

type startAliveMachine struct {
	machineId string
}

func (sam startAliveMachine) step(c *C, ctx *context) {
	m, err := ctx.st.Machine(sam.machineId)
	c.Assert(err, IsNil)
	pinger, err := m.SetAgentAlive()
	c.Assert(err, IsNil)
	ctx.st.StartSync()
	err = m.WaitAgentAlive(coretesting.LongWait)
	c.Assert(err, IsNil)
	agentAlive, err := m.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(agentAlive, Equals, true)
	cons, err := m.Constraints()
	c.Assert(err, IsNil)
	inst, hc := testing.StartInstanceWithConstraints(c, ctx.conn.Environ, m.Id(), cons)
	err = m.SetProvisioned(inst.Id(), "fake_nonce", hc)
	c.Assert(err, IsNil)
	ctx.pingers[m.Id()] = pinger
}

type setTools struct {
	machineId string
	tools     *tools.Tools
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
	bundleURL, err := url.Parse(fmt.Sprintf("http://bundles.testing.invalid/%s-%d", name, rev))
	c.Assert(err, IsNil)
	dummy, err := ctx.st.AddCharm(ch, curl, bundleURL, fmt.Sprintf("%s-%d-sha256", name, rev))
	c.Assert(err, IsNil)
	ctx.charms[ac.name] = dummy
}

type addService struct {
	name  string
	charm string
}

func (as addService) step(c *C, ctx *context) {
	ch, ok := ctx.charms[as.charm]
	c.Assert(ok, Equals, true)
	_, err := ctx.st.AddService(as.name, ch)
	c.Assert(err, IsNil)
}

type setServiceExposed struct {
	name    string
	exposed bool
}

func (sse setServiceExposed) step(c *C, ctx *context) {
	s, err := ctx.st.Service(sse.name)
	c.Assert(err, IsNil)
	if sse.exposed {
		err = s.SetExposed()
		c.Assert(err, IsNil)
	}
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
	err = u.WaitAgentAlive(coretesting.LongWait)
	c.Assert(err, IsNil)
	agentAlive, err := u.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(agentAlive, Equals, true)
	m, err := ctx.st.Machine(aau.machineId)
	c.Assert(err, IsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, IsNil)
	ctx.pingers[u.Name()] = pinger
}

type setUnitsAlive struct {
	serviceName string
}

func (sua setUnitsAlive) step(c *C, ctx *context) {
	s, err := ctx.st.Service(sua.serviceName)
	c.Assert(err, IsNil)
	us, err := s.AllUnits()
	c.Assert(err, IsNil)
	for _, u := range us {
		pinger, err := u.SetAgentAlive()
		c.Assert(err, IsNil)
		ctx.st.StartSync()
		err = u.WaitAgentAlive(coretesting.LongWait)
		c.Assert(err, IsNil)
		agentAlive, err := u.AgentAlive()
		c.Assert(err, IsNil)
		c.Assert(agentAlive, Equals, true)
		ctx.pingers[u.Name()] = pinger
	}
}

type setUnitStatus struct {
	unitName   string
	status     params.Status
	statusInfo string
}

func (sus setUnitStatus) step(c *C, ctx *context) {
	u, err := ctx.st.Unit(sus.unitName)
	c.Assert(err, IsNil)
	err = u.SetStatus(sus.status, sus.statusInfo)
	c.Assert(err, IsNil)
}

type openUnitPort struct {
	unitName string
	protocol string
	number   int
}

func (oup openUnitPort) step(c *C, ctx *context) {
	u, err := ctx.st.Unit(oup.unitName)
	c.Assert(err, IsNil)
	err = u.OpenPort(oup.protocol, oup.number)
	c.Assert(err, IsNil)
}

type ensureDyingUnit struct {
	unitName string
}

func (e ensureDyingUnit) step(c *C, ctx *context) {
	u, err := ctx.st.Unit(e.unitName)
	c.Assert(err, IsNil)
	err = u.Destroy()
	c.Assert(err, IsNil)
	c.Assert(u.Life(), Equals, state.Dying)
}

type ensureDyingService struct {
	serviceName string
}

func (e ensureDyingService) step(c *C, ctx *context) {
	svc, err := ctx.st.Service(e.serviceName)
	c.Assert(err, IsNil)
	err = svc.Destroy()
	c.Assert(err, IsNil)
	err = svc.Refresh()
	c.Assert(err, IsNil)
	c.Assert(svc.Life(), Equals, state.Dying)
}

type ensureDeadMachine struct {
	machineId string
}

func (e ensureDeadMachine) step(c *C, ctx *context) {
	m, err := ctx.st.Machine(e.machineId)
	c.Assert(err, IsNil)
	err = m.EnsureDead()
	c.Assert(err, IsNil)
	c.Assert(m.Life(), Equals, state.Dead)
}

type setMachineStatus struct {
	machineId  string
	status     params.Status
	statusInfo string
}

func (sms setMachineStatus) step(c *C, ctx *context) {
	m, err := ctx.st.Machine(sms.machineId)
	c.Assert(err, IsNil)
	err = m.SetStatus(sms.status, sms.statusInfo)
	c.Assert(err, IsNil)
}

type relateServices struct {
	ep1, ep2 string
}

func (rs relateServices) step(c *C, ctx *context) {
	eps, err := ctx.st.InferEndpoints([]string{rs.ep1, rs.ep2})
	c.Assert(err, IsNil)
	_, err = ctx.st.AddRelation(eps...)
	c.Assert(err, IsNil)
}

type addSubordinate struct {
	prinUnit   string
	subService string
}

func (as addSubordinate) step(c *C, ctx *context) {
	u, err := ctx.st.Unit(as.prinUnit)
	c.Assert(err, IsNil)
	eps, err := ctx.st.InferEndpoints([]string{u.ServiceName(), as.subService})
	c.Assert(err, IsNil)
	rel, err := ctx.st.EndpointsRelation(eps...)
	c.Assert(err, IsNil)
	ru, err := rel.Unit(u)
	c.Assert(err, IsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, IsNil)
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

func (e scopedExpect) step(c *C, ctx *context) {
	c.Logf("expect: %s %s", e.what, strings.Join(e.scope, " "))

	// Now execute the command for each format.
	for _, format := range statusFormats {
		c.Logf("format %q", format.name)
		// Run command with the required format.
		args := append([]string{"--format", format.name}, e.scope...)
		code, stdout, stderr := runStatus(c, args...)
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
}

func (e expect) step(c *C, ctx *context) {
	scopedExpect{e.what, nil, e.output}.step(c, ctx)
}

func (s *StatusSuite) TestStatusAllFormats(c *C) {
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

func (s *StatusSuite) TestStatusFilterErrors(c *C) {
	steps := []stepper{
		addMachine{machineId: "0", job: state.JobManageEnviron},
		addMachine{machineId: "1", job: state.JobHostUnits},
		addCharm{"mysql"},
		addService{"mysql", "mysql"},
		addAliveUnit{"mysql", "1"},
	}
	ctx := s.newContext()
	defer s.resetContext(c, ctx)
	ctx.run(c, steps)

	// Status filters can only fail if the patterns are invalid.
	code, _, stderr := runStatus(c, "[*")
	c.Assert(code, Not(Equals), 0)
	c.Assert(string(stderr), Equals, `error: pattern "[*" contains invalid characters`+"\n")

	code, _, stderr = runStatus(c, "//")
	c.Assert(code, Not(Equals), 0)
	c.Assert(string(stderr), Equals, `error: pattern "//" contains too many '/' characters`+"\n")

	// Pattern validity is checked eagerly; if a bad pattern
	// proceeds a valid, matching pattern, then the bad pattern
	// will still cause an error.
	code, _, stderr = runStatus(c, "*", "[*")
	c.Assert(code, Not(Equals), 0)
	c.Assert(string(stderr), Equals, `error: pattern "[*" contains invalid characters`+"\n")
}
