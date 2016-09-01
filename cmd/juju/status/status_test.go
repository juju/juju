// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/presence"
	"github.com/juju/juju/status"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	coreversion "github.com/juju/juju/version"
)

var (
	currentVersion = version.Number{Major: 1, Minor: 2, Patch: 3}
	nextVersion    = version.Number{Major: 1, Minor: 2, Patch: 4}
)

func runStatus(c *gc.C, args ...string) (code int, stdout, stderr []byte) {
	ctx := coretesting.Context(c)
	code = cmd.Main(NewStatusCommand(), ctx, args)
	stdout = ctx.Stdout.(*bytes.Buffer).Bytes()
	stderr = ctx.Stderr.(*bytes.Buffer).Bytes()
	return
}

type StatusSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&StatusSuite{})

func (s *StatusSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
	s.PatchValue(&coreversion.Current, currentVersion)
}

func (s *StatusSuite) SetUpTest(c *gc.C) {
	s.ConfigAttrs = map[string]interface{}{
		"agent-version": currentVersion.String(),
	}
	s.JujuConnSuite.SetUpTest(c)
}

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

func newContext(st *state.State, env environs.Environ, adminUserTag string) *context {
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
	st            *state.State
	env           environs.Environ
	charms        map[string]*state.Charm
	pingers       map[string]*presence.Pinger
	adminUserTag  string // A string repr of the tag.
	expectIsoTime bool
}

func (ctx *context) reset(c *gc.C) {
	for _, up := range ctx.pingers {
		err := up.KillForTesting()
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

func (ctx *context) setAgentPresence(c *gc.C, p presence.Agent) *presence.Pinger {
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
	return newContext(st, s.Environ, s.AdminUserTag(c).String())
}

func (s *StatusSuite) resetContext(c *gc.C, ctx *context) {
	ctx.reset(c)
	s.JujuConnSuite.Reset(c)
}

// shortcuts for expected output.
var (
	model = M{
		"name":       "controller",
		"controller": "kontroll",
		"cloud":      "dummy",
		"region":     "dummy-region",
		"version":    "1.2.3",
	}

	machine0 = M{
		"juju-status": M{
			"current": "started",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"dns-name":    "controller-0.dns",
		"instance-id": "controller-0",
		"machine-status": M{
			"current": "pending",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"series":                   "quantal",
		"hardware":                 "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
		"controller-member-status": "adding-vote",
	}
	machine1 = M{
		"juju-status": M{
			"current": "started",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"dns-name":    "controller-1.dns",
		"instance-id": "controller-1",
		"machine-status": M{
			"current": "pending",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"series":   "quantal",
		"hardware": "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
	}
	machine2 = M{
		"juju-status": M{
			"current": "started",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"dns-name":    "controller-2.dns",
		"instance-id": "controller-2",
		"machine-status": M{
			"current": "pending",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"series":   "quantal",
		"hardware": "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
	}
	machine3 = M{
		"juju-status": M{
			"current": "started",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"dns-name":    "controller-3.dns",
		"instance-id": "controller-3",
		"machine-status": M{
			"current": "pending",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"series":   "quantal",
		"hardware": "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
	}
	machine4 = M{
		"juju-status": M{
			"current": "started",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"dns-name":    "controller-4.dns",
		"instance-id": "controller-4",
		"machine-status": M{
			"current": "pending",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"series":   "quantal",
		"hardware": "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
	}
	machine1WithContainers = M{
		"juju-status": M{
			"current": "started",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"containers": M{
			"1/lxd/0": M{
				"juju-status": M{
					"current": "started",
					"since":   "01 Apr 15 01:23+10:00",
				},
				"containers": M{
					"1/lxd/0/lxd/0": M{
						"juju-status": M{
							"current": "started",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"dns-name":    "controller-3.dns",
						"instance-id": "controller-3",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series": "quantal",
					},
				},
				"dns-name":    "controller-2.dns",
				"instance-id": "controller-2",
				"machine-status": M{
					"current": "pending",
					"since":   "01 Apr 15 01:23+10:00",
				},
				"series": "quantal",
			},
			"1/lxd/1": M{
				"juju-status": M{
					"current": "pending",
					"since":   "01 Apr 15 01:23+10:00",
				},
				"instance-id": "pending",
				"machine-status": M{
					"current": "pending",
					"since":   "01 Apr 15 01:23+10:00",
				},
				"series": "quantal",
			},
		},
		"dns-name":    "controller-1.dns",
		"instance-id": "controller-1",
		"machine-status": M{
			"current": "pending",
			"since":   "01 Apr 15 01:23+10:00",
		},

		"series":   "quantal",
		"hardware": "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
	}
	unexposedService = dummyCharm(M{
		"application-status": M{
			"current": "unknown",
			"message": "Waiting for agent initialization to finish",
			"since":   "01 Apr 15 01:23+10:00",
		},
	})
	exposedService = dummyCharm(M{
		"application-status": M{
			"current": "unknown",
			"message": "Waiting for agent initialization to finish",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"exposed": true,
	})
	loggingCharm = M{
		"charm":              "cs:quantal/logging-1",
		"charm-origin":       "jujucharms",
		"charm-name":         "logging",
		"charm-rev":          1,
		"series":             "quantal",
		"os":                 "ubuntu",
		"exposed":            true,
		"application-status": M{},
		"relations": M{
			"logging-directory": L{"wordpress"},
			"info":              L{"mysql"},
		},
		"subordinate-to": L{"mysql", "wordpress"},
	}
)

type outputFormat struct {
	name      string
	marshal   func(v interface{}) ([]byte, error)
	unmarshal func(data []byte, v interface{}) error
}

// statusFormats list all output formats that can be marshalled as structured data,
// supported by status command.
var statusFormats = []outputFormat{
	{"yaml", goyaml.Marshal, goyaml.Unmarshal},
	{"json", json.Marshal, json.Unmarshal},
}

var machineCons = constraints.MustParse("cpu-cores=2 mem=8G root-disk=8G")

var statusTests = []testCase{
	// Status tests
	test( // 0
		"bootstrap and starting a single instance",

		addMachine{machineId: "0", job: state.JobManageModel},
		expect{
			"simulate juju bootstrap by adding machine/0 to the state",
			M{
				"model": model,
				"machines": M{
					"0": M{
						"juju-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"instance-id": "pending",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series":                   "quantal",
						"controller-member-status": "adding-vote",
					},
				},
				"applications": M{},
			},
		},

		startAliveMachine{"0"},
		setAddresses{"0", []network.Address{
			network.NewAddress("10.0.0.1"),
			network.NewScopedAddress("controller-0.dns", network.ScopePublic),
		}},
		expect{
			"simulate the PA starting an instance in response to the state change",
			M{
				"model": model,
				"machines": M{
					"0": M{
						"juju-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"dns-name":    "controller-0.dns",
						"instance-id": "controller-0",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series":                   "quantal",
						"hardware":                 "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
						"controller-member-status": "adding-vote",
					},
				},
				"applications": M{},
			},
		},

		setMachineStatus{"0", status.StatusStarted, ""},
		expect{
			"simulate the MA started and set the machine status",
			M{
				"model": model,
				"machines": M{
					"0": machine0,
				},
				"applications": M{},
			},
		},

		setTools{"0", version.MustParseBinary("1.2.3-trusty-ppc")},
		expect{
			"simulate the MA setting the version",
			M{
				"model": model,
				"machines": M{
					"0": M{
						"dns-name":    "controller-0.dns",
						"instance-id": "controller-0",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"juju-status": M{
							"current": "started",
							"since":   "01 Apr 15 01:23+10:00",
							"version": "1.2.3",
						},
						"series":                   "quantal",
						"hardware":                 "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
						"controller-member-status": "adding-vote",
					},
				},
				"applications": M{},
			},
		},
	),
	test( // 1
		"instance with different hardware characteristics",
		addMachine{machineId: "0", cons: machineCons, job: state.JobManageModel},
		setAddresses{"0", []network.Address{
			network.NewAddress("10.0.0.1"),
			network.NewScopedAddress("controller-0.dns", network.ScopePublic),
		}},
		startAliveMachine{"0"},
		setMachineStatus{"0", status.StatusStarted, ""},
		expect{
			"machine 0 has specific hardware characteristics",
			M{
				"model": model,
				"machines": M{
					"0": M{
						"juju-status": M{
							"current": "started",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"dns-name":    "controller-0.dns",
						"instance-id": "controller-0",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series":                   "quantal",
						"hardware":                 "arch=amd64 cpu-cores=2 mem=8192M root-disk=8192M",
						"controller-member-status": "adding-vote",
					},
				},
				"applications": M{},
			},
		},
	),
	test( // 2
		"instance without addresses",
		addMachine{machineId: "0", cons: machineCons, job: state.JobManageModel},
		startAliveMachine{"0"},
		setMachineStatus{"0", status.StatusStarted, ""},
		expect{
			"machine 0 has no dns-name",
			M{
				"model": model,
				"machines": M{
					"0": M{
						"juju-status": M{
							"current": "started",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"instance-id": "controller-0",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series":                   "quantal",
						"hardware":                 "arch=amd64 cpu-cores=2 mem=8192M root-disk=8192M",
						"controller-member-status": "adding-vote",
					},
				},
				"applications": M{},
			},
		},
	),
	test( // 3
		"test pending and missing machines",
		addMachine{machineId: "0", job: state.JobManageModel},
		expect{
			"machine 0 reports pending",
			M{
				"model": model,
				"machines": M{
					"0": M{
						"juju-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"instance-id": "pending",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series":                   "quantal",
						"controller-member-status": "adding-vote",
					},
				},
				"applications": M{},
			},
		},

		startMissingMachine{"0"},
		expect{
			"machine 0 reports missing",
			M{
				"model": model,
				"machines": M{
					"0": M{
						"instance-id": "i-missing",
						"juju-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"machine-status": M{
							"current": "unknown",
							"message": "missing",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series":                   "quantal",
						"hardware":                 "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
						"controller-member-status": "adding-vote",
					},
				},
				"applications": M{},
			},
		},
	),
	test( // 4
		"add two services and expose one, then add 2 more machines and some units",
		// step 0
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("controller-0.dns")},
		startAliveMachine{"0"},
		setMachineStatus{"0", status.StatusStarted, ""},
		addCharm{"dummy"},
		addService{name: "dummy-application", charm: "dummy"},
		addService{name: "exposed-application", charm: "dummy"},
		expect{
			"no applications exposed yet",
			M{
				"model": model,
				"machines": M{
					"0": machine0,
				},
				"applications": M{
					"dummy-application":   unexposedService,
					"exposed-application": unexposedService,
				},
			},
		},

		// step 8
		setServiceExposed{"exposed-application", true},
		expect{
			"one exposed application",
			M{
				"model": model,
				"machines": M{
					"0": machine0,
				},
				"applications": M{
					"dummy-application":   unexposedService,
					"exposed-application": exposedService,
				},
			},
		},

		// step 10
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("controller-1.dns")},
		startAliveMachine{"1"},
		setMachineStatus{"1", status.StatusStarted, ""},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", network.NewAddresses("controller-2.dns")},
		startAliveMachine{"2"},
		setMachineStatus{"2", status.StatusStarted, ""},
		expect{
			"two more machines added",
			M{
				"model": model,
				"machines": M{
					"0": machine0,
					"1": machine1,
					"2": machine2,
				},
				"applications": M{
					"dummy-application":   unexposedService,
					"exposed-application": exposedService,
				},
			},
		},

		// step 19
		addAliveUnit{"dummy-application", "1"},
		addAliveUnit{"exposed-application", "2"},
		setAgentStatus{"exposed-application/0", status.StatusError, "You Require More Vespene Gas", nil},
		// Open multiple ports with different protocols,
		// ensure they're sorted on protocol, then number.
		openUnitPort{"exposed-application/0", "udp", 10},
		openUnitPort{"exposed-application/0", "udp", 2},
		openUnitPort{"exposed-application/0", "tcp", 3},
		openUnitPort{"exposed-application/0", "tcp", 2},
		// Simulate some status with no info, while the agent is down.
		// Status used to be down, we no longer support said state.
		// now is one of: pending, started, error.
		setUnitStatus{"dummy-application/0", status.StatusTerminated, "", nil},
		setAgentStatus{"dummy-application/0", status.StatusIdle, "", nil},

		expect{
			"add two units, one alive (in error state), one started",
			M{
				"model": model,
				"machines": M{
					"0": machine0,
					"1": machine1,
					"2": machine2,
				},
				"applications": M{
					"exposed-application": dummyCharm(M{
						"exposed": true,
						"application-status": M{
							"current": "error",
							"message": "You Require More Vespene Gas",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"exposed-application/0": M{
								"machine": "2",
								"workload-status": M{
									"current": "error",
									"message": "You Require More Vespene Gas",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"open-ports": L{
									"2/tcp", "3/tcp", "2/udp", "10/udp",
								},
								"public-address": "controller-2.dns",
							},
						},
					}),
					"dummy-application": dummyCharm(M{
						"application-status": M{
							"current": "terminated",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"dummy-application/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "terminated",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-1.dns",
							},
						},
					}),
				},
			},
		},

		// step 29
		addMachine{machineId: "3", job: state.JobHostUnits},
		startMachine{"3"},
		// Simulate some status with info, while the agent is down.
		setAddresses{"3", network.NewAddresses("controller-3.dns")},
		setMachineStatus{"3", status.StatusStopped, "Really?"},
		addMachine{machineId: "4", job: state.JobHostUnits},
		setAddresses{"4", network.NewAddresses("controller-4.dns")},
		startAliveMachine{"4"},
		setMachineStatus{"4", status.StatusError, "Beware the red toys"},
		ensureDyingUnit{"dummy-application/0"},
		addMachine{machineId: "5", job: state.JobHostUnits},
		ensureDeadMachine{"5"},
		expect{
			"add three more machine, one with a dead agent, one in error state and one dead itself; also one dying unit",
			M{
				"model": model,
				"machines": M{
					"0": machine0,
					"1": machine1,
					"2": machine2,
					"3": M{
						"dns-name":    "controller-3.dns",
						"instance-id": "controller-3",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"juju-status": M{
							"current": "stopped",
							"message": "Really?",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series":   "quantal",
						"hardware": "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
					},
					"4": M{
						"dns-name":    "controller-4.dns",
						"instance-id": "controller-4",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"juju-status": M{
							"current": "error",
							"message": "Beware the red toys",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series":   "quantal",
						"hardware": "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
					},
					"5": M{
						"juju-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
							"life":    "dead",
						},
						"instance-id": "pending",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series": "quantal",
					},
				},
				"applications": M{
					"exposed-application": dummyCharm(M{
						"exposed": true,
						"application-status": M{
							"current": "error",
							"message": "You Require More Vespene Gas",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"exposed-application/0": M{
								"machine": "2",
								"workload-status": M{
									"current": "error",
									"message": "You Require More Vespene Gas",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"open-ports": L{
									"2/tcp", "3/tcp", "2/udp", "10/udp",
								},
								"public-address": "controller-2.dns",
							},
						},
					}),
					"dummy-application": dummyCharm(M{
						"application-status": M{
							"current": "terminated",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"dummy-application/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "terminated",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-1.dns",
							},
						},
					}),
				},
			},
		},

		// step 41
		scopedExpect{
			"scope status on dummy-application/0 unit",
			[]string{"dummy-application/0"},
			M{
				"model": model,
				"machines": M{
					"1": machine1,
				},
				"applications": M{
					"dummy-application": dummyCharm(M{
						"application-status": M{
							"current": "terminated",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"dummy-application/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "terminated",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-1.dns",
							},
						},
					}),
				},
			},
		},
		scopedExpect{
			"scope status on exposed-application application",
			[]string{"exposed-application"},
			M{
				"model": model,
				"machines": M{
					"2": machine2,
				},
				"applications": M{
					"exposed-application": dummyCharm(M{
						"exposed": true,
						"application-status": M{
							"current": "error",
							"message": "You Require More Vespene Gas",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"exposed-application/0": M{
								"machine": "2",
								"workload-status": M{
									"current": "error",
									"message": "You Require More Vespene Gas",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"open-ports": L{
									"2/tcp", "3/tcp", "2/udp", "10/udp",
								},
								"public-address": "controller-2.dns",
							},
						},
					}),
				},
			},
		},
		scopedExpect{
			"scope status on application pattern",
			[]string{"d*-application"},
			M{
				"model": model,
				"machines": M{
					"1": machine1,
				},
				"applications": M{
					"dummy-application": dummyCharm(M{
						"application-status": M{
							"current": "terminated",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"dummy-application/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "terminated",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-1.dns",
							},
						},
					}),
				},
			},
		},
		scopedExpect{
			"scope status on unit pattern",
			[]string{"e*posed-application/*"},
			M{
				"model": model,
				"machines": M{
					"2": machine2,
				},
				"applications": M{
					"exposed-application": dummyCharm(M{
						"exposed": true,
						"application-status": M{
							"current": "error",
							"message": "You Require More Vespene Gas",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"exposed-application/0": M{
								"machine": "2",
								"workload-status": M{
									"current": "error",
									"message": "You Require More Vespene Gas",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"open-ports": L{
									"2/tcp", "3/tcp", "2/udp", "10/udp",
								},
								"public-address": "controller-2.dns",
							},
						},
					}),
				},
			},
		},
		scopedExpect{
			"scope status on combination of application and unit patterns",
			[]string{"exposed-application", "dummy-application", "e*posed-application/*", "dummy-application/*"},
			M{
				"model": model,
				"machines": M{
					"1": machine1,
					"2": machine2,
				},
				"applications": M{
					"dummy-application": dummyCharm(M{
						"application-status": M{
							"current": "terminated",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"dummy-application/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "terminated",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-1.dns",
							},
						},
					}),
					"exposed-application": dummyCharm(M{
						"exposed": true,
						"application-status": M{
							"current": "error",
							"message": "You Require More Vespene Gas",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"exposed-application/0": M{
								"machine": "2",
								"workload-status": M{
									"current": "error",
									"message": "You Require More Vespene Gas",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"open-ports": L{
									"2/tcp", "3/tcp", "2/udp", "10/udp",
								},
								"public-address": "controller-2.dns",
							},
						},
					}),
				},
			},
		},
	),
	test( // 5
		"a unit with a hook relation error",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("controller-0.dns")},
		startAliveMachine{"0"},
		setMachineStatus{"0", status.StatusStarted, ""},

		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("controller-1.dns")},
		startAliveMachine{"1"},
		setMachineStatus{"1", status.StatusStarted, ""},

		addCharm{"wordpress"},
		addService{name: "wordpress", charm: "wordpress"},
		addAliveUnit{"wordpress", "1"},

		addCharm{"mysql"},
		addService{name: "mysql", charm: "mysql"},
		addAliveUnit{"mysql", "1"},

		relateServices{"wordpress", "mysql"},

		setAgentStatus{"wordpress/0", status.StatusError,
			"hook failed: some-relation-changed",
			map[string]interface{}{"relation-id": 0}},

		expect{
			"a unit with a hook relation error",
			M{
				"model": model,
				"machines": M{
					"0": machine0,
					"1": machine1,
				},
				"applications": M{
					"wordpress": wordpressCharm(M{
						"relations": M{
							"db": L{"mysql"},
						},
						"application-status": M{
							"current": "error",
							"message": "hook failed: some-relation-changed",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"wordpress/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "error",
									"message": "hook failed: some-relation-changed for mysql:server",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-1.dns",
							},
						},
					}),
					"mysql": mysqlCharm(M{
						"relations": M{
							"server": L{"wordpress"},
						},
						"application-status": M{
							"current": "unknown",
							"message": "Waiting for agent initialization to finish",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"mysql/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "unknown",
									"message": "Waiting for agent initialization to finish",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "allocating",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-1.dns",
							},
						},
					}),
				},
			},
		},
	),
	test( // 6
		"a unit with a hook relation error when the agent is down",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("controller-0.dns")},
		startAliveMachine{"0"},
		setMachineStatus{"0", status.StatusStarted, ""},

		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("controller-1.dns")},
		startAliveMachine{"1"},
		setMachineStatus{"1", status.StatusStarted, ""},

		addCharm{"wordpress"},
		addService{name: "wordpress", charm: "wordpress"},
		addAliveUnit{"wordpress", "1"},

		addCharm{"mysql"},
		addService{name: "mysql", charm: "mysql"},
		addAliveUnit{"mysql", "1"},

		relateServices{"wordpress", "mysql"},

		setAgentStatus{"wordpress/0", status.StatusError,
			"hook failed: some-relation-changed",
			map[string]interface{}{"relation-id": 0}},

		expect{
			"a unit with a hook relation error when the agent is down",
			M{
				"model": model,
				"machines": M{
					"0": machine0,
					"1": machine1,
				},
				"applications": M{
					"wordpress": wordpressCharm(M{
						"relations": M{
							"db": L{"mysql"},
						},
						"application-status": M{
							"current": "error",
							"message": "hook failed: some-relation-changed",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"wordpress/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "error",
									"message": "hook failed: some-relation-changed for mysql:server",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-1.dns",
							},
						},
					}),
					"mysql": mysqlCharm(M{
						"relations": M{
							"server": L{"wordpress"},
						},
						"application-status": M{
							"current": "unknown",
							"message": "Waiting for agent initialization to finish",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"mysql/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "unknown",
									"message": "Waiting for agent initialization to finish",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "allocating",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-1.dns",
							},
						},
					}),
				},
			},
		},
	),
	test( // 7
		"add a dying application",
		addCharm{"dummy"},
		addService{name: "dummy-application", charm: "dummy"},
		addMachine{machineId: "0", job: state.JobHostUnits},
		addAliveUnit{"dummy-application", "0"},
		ensureDyingService{"dummy-application"},
		expect{
			"application shows life==dying",
			M{
				"model": model,
				"machines": M{
					"0": M{
						"juju-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"instance-id": "pending",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},

						"series": "quantal",
					},
				},
				"applications": M{
					"dummy-application": dummyCharm(M{
						"life": "dying",
						"application-status": M{
							"current": "unknown",
							"message": "Waiting for agent initialization to finish",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"dummy-application/0": M{
								"machine": "0",
								"workload-status": M{
									"current": "unknown",
									"message": "Waiting for agent initialization to finish",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "allocating",
									"since":   "01 Apr 15 01:23+10:00",
								},
							},
						},
					}),
				},
			},
		},
	),
	test( // 8
		"a unit where the agent is down shows as lost",
		addCharm{"dummy"},
		addService{name: "dummy-application", charm: "dummy"},
		addMachine{machineId: "0", job: state.JobHostUnits},
		startAliveMachine{"0"},
		setMachineStatus{"0", status.StatusStarted, ""},
		addUnit{"dummy-application", "0"},
		setAgentStatus{"dummy-application/0", status.StatusIdle, "", nil},
		setUnitStatus{"dummy-application/0", status.StatusActive, "", nil},
		expect{
			"unit shows that agent is lost",
			M{
				"model": model,
				"machines": M{
					"0": M{
						"juju-status": M{
							"current": "started",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"instance-id": "controller-0",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},

						"series":   "quantal",
						"hardware": "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
					},
				},
				"applications": M{
					"dummy-application": dummyCharm(M{
						"application-status": M{
							"current": "active",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"dummy-application/0": M{
								"machine": "0",
								"workload-status": M{
									"current": "unknown",
									"message": "agent lost, see 'juju status-history dummy-application/0'",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "lost",
									"message": "agent is not communicating with the server",
									"since":   "01 Apr 15 01:23+10:00",
								},
							},
						},
					}),
				},
			},
		},
	),

	// Relation tests
	test( // 9
		"complex scenario with multiple related services",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("controller-0.dns")},
		startAliveMachine{"0"},
		setMachineStatus{"0", status.StatusStarted, ""},
		addCharm{"wordpress"},
		addCharm{"mysql"},
		addCharm{"varnish"},

		addService{name: "project", charm: "wordpress"},
		setServiceExposed{"project", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("controller-1.dns")},
		startAliveMachine{"1"},
		setMachineStatus{"1", status.StatusStarted, ""},
		addAliveUnit{"project", "1"},
		setAgentStatus{"project/0", status.StatusIdle, "", nil},
		setUnitStatus{"project/0", status.StatusActive, "", nil},

		addService{name: "mysql", charm: "mysql"},
		setServiceExposed{"mysql", true},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", network.NewAddresses("controller-2.dns")},
		startAliveMachine{"2"},
		setMachineStatus{"2", status.StatusStarted, ""},
		addAliveUnit{"mysql", "2"},
		setAgentStatus{"mysql/0", status.StatusIdle, "", nil},
		setUnitStatus{"mysql/0", status.StatusActive, "", nil},

		addService{name: "varnish", charm: "varnish"},
		setServiceExposed{"varnish", true},
		addMachine{machineId: "3", job: state.JobHostUnits},
		setAddresses{"3", network.NewAddresses("controller-3.dns")},
		startAliveMachine{"3"},
		setMachineStatus{"3", status.StatusStarted, ""},
		addAliveUnit{"varnish", "3"},

		addService{name: "private", charm: "wordpress"},
		setServiceExposed{"private", true},
		addMachine{machineId: "4", job: state.JobHostUnits},
		setAddresses{"4", network.NewAddresses("controller-4.dns")},
		startAliveMachine{"4"},
		setMachineStatus{"4", status.StatusStarted, ""},
		addAliveUnit{"private", "4"},

		relateServices{"project", "mysql"},
		relateServices{"project", "varnish"},
		relateServices{"private", "mysql"},

		expect{
			"multiples services with relations between some of them",
			M{
				"model": model,
				"machines": M{
					"0": machine0,
					"1": machine1,
					"2": machine2,
					"3": machine3,
					"4": machine4,
				},
				"applications": M{
					"project": wordpressCharm(M{
						"exposed": true,
						"application-status": M{
							"current": "active",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"project/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "active",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-1.dns",
							},
						},
						"relations": M{
							"db":    L{"mysql"},
							"cache": L{"varnish"},
						},
					}),
					"mysql": mysqlCharm(M{
						"exposed": true,
						"application-status": M{
							"current": "active",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"mysql/0": M{
								"machine": "2",
								"workload-status": M{
									"current": "active",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-2.dns",
							},
						},
						"relations": M{
							"server": L{"private", "project"},
						},
					}),
					"varnish": M{
						"charm":        "cs:quantal/varnish-1",
						"charm-origin": "jujucharms",
						"charm-name":   "varnish",
						"charm-rev":    1,
						"series":       "quantal",
						"os":           "ubuntu",
						"exposed":      true,
						"application-status": M{
							"current": "unknown",
							"message": "Waiting for agent initialization to finish",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"varnish/0": M{
								"machine": "3",
								"workload-status": M{
									"current": "unknown",
									"message": "Waiting for agent initialization to finish",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "allocating",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-3.dns",
							},
						},
						"relations": M{
							"webcache": L{"project"},
						},
					},
					"private": wordpressCharm(M{
						"exposed": true,
						"application-status": M{
							"current": "unknown",
							"message": "Waiting for agent initialization to finish",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"private/0": M{
								"machine": "4",
								"workload-status": M{
									"current": "unknown",
									"message": "Waiting for agent initialization to finish",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "allocating",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-4.dns",
							},
						},
						"relations": M{
							"db": L{"mysql"},
						},
					}),
				},
			},
		},
	),
	test( // 10
		"simple peer scenario",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("controller-0.dns")},
		startAliveMachine{"0"},
		setMachineStatus{"0", status.StatusStarted, ""},
		addCharm{"riak"},
		addCharm{"wordpress"},

		addService{name: "riak", charm: "riak"},
		setServiceExposed{"riak", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("controller-1.dns")},
		startAliveMachine{"1"},
		setMachineStatus{"1", status.StatusStarted, ""},
		addAliveUnit{"riak", "1"},
		setAgentStatus{"riak/0", status.StatusIdle, "", nil},
		setUnitStatus{"riak/0", status.StatusActive, "", nil},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", network.NewAddresses("controller-2.dns")},
		startAliveMachine{"2"},
		setMachineStatus{"2", status.StatusStarted, ""},
		addAliveUnit{"riak", "2"},
		setAgentStatus{"riak/1", status.StatusIdle, "", nil},
		setUnitStatus{"riak/1", status.StatusActive, "", nil},
		addMachine{machineId: "3", job: state.JobHostUnits},
		setAddresses{"3", network.NewAddresses("controller-3.dns")},
		startAliveMachine{"3"},
		setMachineStatus{"3", status.StatusStarted, ""},
		addAliveUnit{"riak", "3"},
		setAgentStatus{"riak/2", status.StatusIdle, "", nil},
		setUnitStatus{"riak/2", status.StatusActive, "", nil},

		expect{
			"multiples related peer units",
			M{
				"model": model,
				"machines": M{
					"0": machine0,
					"1": machine1,
					"2": machine2,
					"3": machine3,
				},
				"applications": M{
					"riak": M{
						"charm":        "cs:quantal/riak-7",
						"charm-origin": "jujucharms",
						"charm-name":   "riak",
						"charm-rev":    7,
						"series":       "quantal",
						"os":           "ubuntu",
						"exposed":      true,
						"application-status": M{
							"current": "active",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"riak/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "active",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-1.dns",
							},
							"riak/1": M{
								"machine": "2",
								"workload-status": M{
									"current": "active",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-2.dns",
							},
							"riak/2": M{
								"machine": "3",
								"workload-status": M{
									"current": "active",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-3.dns",
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
	test( // 11
		"one application with one subordinate application",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("controller-0.dns")},
		startAliveMachine{"0"},
		setMachineStatus{"0", status.StatusStarted, ""},
		addCharm{"wordpress"},
		addCharm{"mysql"},
		addCharm{"logging"},

		addService{name: "wordpress", charm: "wordpress"},
		setServiceExposed{"wordpress", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("controller-1.dns")},
		startAliveMachine{"1"},
		setMachineStatus{"1", status.StatusStarted, ""},
		addAliveUnit{"wordpress", "1"},
		setAgentStatus{"wordpress/0", status.StatusIdle, "", nil},
		setUnitStatus{"wordpress/0", status.StatusActive, "", nil},

		addService{name: "mysql", charm: "mysql"},
		setServiceExposed{"mysql", true},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", network.NewAddresses("controller-2.dns")},
		startAliveMachine{"2"},
		setMachineStatus{"2", status.StatusStarted, ""},
		addAliveUnit{"mysql", "2"},
		setAgentStatus{"mysql/0", status.StatusIdle, "", nil},
		setUnitStatus{"mysql/0", status.StatusActive, "", nil},

		addService{name: "logging", charm: "logging"},
		setServiceExposed{"logging", true},

		relateServices{"wordpress", "mysql"},
		relateServices{"wordpress", "logging"},
		relateServices{"mysql", "logging"},

		addSubordinate{"wordpress/0", "logging"},
		addSubordinate{"mysql/0", "logging"},

		setUnitsAlive{"logging"},
		setAgentStatus{"logging/0", status.StatusIdle, "", nil},
		setUnitStatus{"logging/0", status.StatusActive, "", nil},
		setAgentStatus{"logging/1", status.StatusError, "somehow lost in all those logs", nil},

		expect{
			"multiples related peer units",
			M{
				"model": model,
				"machines": M{
					"0": machine0,
					"1": machine1,
					"2": machine2,
				},
				"applications": M{
					"wordpress": wordpressCharm(M{
						"exposed": true,
						"application-status": M{
							"current": "active",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"wordpress/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "active",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"subordinates": M{
									"logging/0": M{
										"workload-status": M{
											"current": "active",
											"since":   "01 Apr 15 01:23+10:00",
										},
										"juju-status": M{
											"current": "idle",
											"since":   "01 Apr 15 01:23+10:00",
										},
										"public-address": "controller-1.dns",
									},
								},
								"public-address": "controller-1.dns",
							},
						},
						"relations": M{
							"db":          L{"mysql"},
							"logging-dir": L{"logging"},
						},
					}),
					"mysql": mysqlCharm(M{
						"exposed": true,
						"application-status": M{
							"current": "active",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"mysql/0": M{
								"machine": "2",
								"workload-status": M{
									"current": "active",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"subordinates": M{
									"logging/1": M{
										"workload-status": M{
											"current": "error",
											"message": "somehow lost in all those logs",
											"since":   "01 Apr 15 01:23+10:00",
										},
										"juju-status": M{
											"current": "idle",
											"since":   "01 Apr 15 01:23+10:00",
										},
										"public-address": "controller-2.dns",
									},
								},
								"public-address": "controller-2.dns",
							},
						},
						"relations": M{
							"server":    L{"wordpress"},
							"juju-info": L{"logging"},
						},
					}),
					"logging": loggingCharm,
				},
			},
		},

		// scoped on 'logging'
		scopedExpect{
			"subordinates scoped on logging",
			[]string{"logging"},
			M{
				"model": model,
				"machines": M{
					"1": machine1,
					"2": machine2,
				},
				"applications": M{
					"wordpress": wordpressCharm(M{
						"exposed": true,
						"application-status": M{
							"current": "active",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"wordpress/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "active",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"subordinates": M{
									"logging/0": M{
										"workload-status": M{
											"current": "active",
											"since":   "01 Apr 15 01:23+10:00",
										},
										"juju-status": M{
											"current": "idle",
											"since":   "01 Apr 15 01:23+10:00",
										},
										"public-address": "controller-1.dns",
									},
								},
								"public-address": "controller-1.dns",
							},
						},
						"relations": M{
							"db":          L{"mysql"},
							"logging-dir": L{"logging"},
						},
					}),
					"mysql": mysqlCharm(M{
						"exposed": true,
						"application-status": M{
							"current": "active",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"mysql/0": M{
								"machine": "2",
								"workload-status": M{
									"current": "active",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"subordinates": M{
									"logging/1": M{
										"workload-status": M{
											"current": "error",
											"message": "somehow lost in all those logs",
											"since":   "01 Apr 15 01:23+10:00",
										},
										"juju-status": M{
											"current": "idle",
											"since":   "01 Apr 15 01:23+10:00",
										},
										"public-address": "controller-2.dns",
									},
								},
								"public-address": "controller-2.dns",
							},
						},
						"relations": M{
							"server":    L{"wordpress"},
							"juju-info": L{"logging"},
						},
					}),
					"logging": loggingCharm,
				},
			},
		},

		// scoped on wordpress/0
		scopedExpect{
			"subordinates scoped on logging",
			[]string{"wordpress/0"},
			M{
				"model": model,
				"machines": M{
					"1": machine1,
				},
				"applications": M{
					"wordpress": wordpressCharm(M{
						"exposed": true,
						"application-status": M{
							"current": "active",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"wordpress/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "active",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"subordinates": M{
									"logging/0": M{
										"workload-status": M{
											"current": "active",
											"since":   "01 Apr 15 01:23+10:00",
										},
										"juju-status": M{
											"current": "idle",
											"since":   "01 Apr 15 01:23+10:00",
										},
										"public-address": "controller-1.dns",
									},
								},
								"public-address": "controller-1.dns",
							},
						},
						"relations": M{
							"db":          L{"mysql"},
							"logging-dir": L{"logging"},
						},
					}),
					"logging": loggingCharm,
				},
			},
		},
	),
	test( // 12
		"machines with containers",
		// step 0
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("controller-0.dns")},
		startAliveMachine{"0"},
		setMachineStatus{"0", status.StatusStarted, ""},
		addCharm{"mysql"},
		addService{name: "mysql", charm: "mysql"},
		setServiceExposed{"mysql", true},

		// step 7
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("controller-1.dns")},
		startAliveMachine{"1"},
		setMachineStatus{"1", status.StatusStarted, ""},
		addAliveUnit{"mysql", "1"},
		setAgentStatus{"mysql/0", status.StatusIdle, "", nil},
		setUnitStatus{"mysql/0", status.StatusActive, "", nil},

		// step 14: A container on machine 1.
		addContainer{"1", "1/lxd/0", state.JobHostUnits},
		setAddresses{"1/lxd/0", network.NewAddresses("controller-2.dns")},
		startAliveMachine{"1/lxd/0"},
		setMachineStatus{"1/lxd/0", status.StatusStarted, ""},
		addAliveUnit{"mysql", "1/lxd/0"},
		setAgentStatus{"mysql/1", status.StatusIdle, "", nil},
		setUnitStatus{"mysql/1", status.StatusActive, "", nil},
		addContainer{"1", "1/lxd/1", state.JobHostUnits},

		// step 22: A nested container.
		addContainer{"1/lxd/0", "1/lxd/0/lxd/0", state.JobHostUnits},
		setAddresses{"1/lxd/0/lxd/0", network.NewAddresses("controller-3.dns")},
		startAliveMachine{"1/lxd/0/lxd/0"},
		setMachineStatus{"1/lxd/0/lxd/0", status.StatusStarted, ""},

		expect{
			"machines with nested containers",
			M{
				"model": model,
				"machines": M{
					"0": machine0,
					"1": machine1WithContainers,
				},
				"applications": M{
					"mysql": mysqlCharm(M{
						"exposed": true,
						"application-status": M{
							"current": "active",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"mysql/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "active",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-1.dns",
							},
							"mysql/1": M{
								"machine": "1/lxd/0",
								"workload-status": M{
									"current": "active",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-2.dns",
							},
						},
					}),
				},
			},
		},

		// step 27: once again, with a scope on mysql/1
		scopedExpect{
			"machines with nested containers 2",
			[]string{"mysql/1"},
			M{
				"model": model,
				"machines": M{
					"1": M{
						"juju-status": M{
							"current": "started",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"containers": M{
							"1/lxd/0": M{
								"juju-status": M{
									"current": "started",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"dns-name":    "controller-2.dns",
								"instance-id": "controller-2",
								"machine-status": M{
									"current": "pending",
									"since":   "01 Apr 15 01:23+10:00",
								},

								"series": "quantal",
							},
						},
						"dns-name":    "controller-1.dns",
						"instance-id": "controller-1",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},

						"series":   "quantal",
						"hardware": "arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M",
					},
				},
				"applications": M{
					"mysql": mysqlCharm(M{
						"exposed": true,
						"application-status": M{
							"current": "active",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"mysql/1": M{
								"machine": "1/lxd/0",
								"workload-status": M{
									"current": "active",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-2.dns",
							},
						},
					}),
				},
			},
		},
	),
	test( // 13
		"application with out of date charm",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("controller-0.dns")},
		startAliveMachine{"0"},
		setMachineStatus{"0", status.StatusStarted, ""},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("controller-1.dns")},
		startAliveMachine{"1"},
		setMachineStatus{"1", status.StatusStarted, ""},
		addCharm{"mysql"},
		addService{name: "mysql", charm: "mysql"},
		setServiceExposed{"mysql", true},
		addCharmPlaceholder{"mysql", 23},
		addAliveUnit{"mysql", "1"},

		expect{
			"services and units with correct charm status",
			M{
				"model": model,
				"machines": M{
					"0": machine0,
					"1": machine1,
				},
				"applications": M{
					"mysql": mysqlCharm(M{
						"can-upgrade-to": "cs:quantal/mysql-23",
						"exposed":        true,
						"application-status": M{
							"current": "unknown",
							"message": "Waiting for agent initialization to finish",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"mysql/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "unknown",
									"message": "Waiting for agent initialization to finish",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "allocating",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-1.dns",
							},
						},
					}),
				},
			},
		},
	),
	test( // 14
		"unit with out of date charm",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("controller-0.dns")},
		startAliveMachine{"0"},
		setMachineStatus{"0", status.StatusStarted, ""},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("controller-1.dns")},
		startAliveMachine{"1"},
		setMachineStatus{"1", status.StatusStarted, ""},
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
				"model": model,
				"machines": M{
					"0": machine0,
					"1": machine1,
				},
				"applications": M{
					"mysql": mysqlCharm(M{
						"charm":        "local:quantal/mysql-1",
						"charm-origin": "local",
						"exposed":      true,
						"application-status": M{
							"current": "active",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"mysql/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "active",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"upgrading-from": "cs:quantal/mysql-1",
								"public-address": "controller-1.dns",
							},
						},
					}),
				},
			},
		},
	),
	test( // 15
		"application and unit with out of date charms",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("controller-0.dns")},
		startAliveMachine{"0"},
		setMachineStatus{"0", status.StatusStarted, ""},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("controller-1.dns")},
		startAliveMachine{"1"},
		setMachineStatus{"1", status.StatusStarted, ""},
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
				"model": model,
				"machines": M{
					"0": machine0,
					"1": machine1,
				},
				"applications": M{
					"mysql": mysqlCharm(M{
						"charm":          "cs:quantal/mysql-2",
						"charm-rev":      2,
						"can-upgrade-to": "cs:quantal/mysql-23",
						"exposed":        true,
						"application-status": M{
							"current": "active",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"mysql/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "active",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"upgrading-from": "cs:quantal/mysql-1",
								"public-address": "controller-1.dns",
							},
						},
					}),
				},
			},
		},
	),
	test( // 16
		"application with local charm not shown as out of date",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("controller-0.dns")},
		startAliveMachine{"0"},
		setMachineStatus{"0", status.StatusStarted, ""},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("controller-1.dns")},
		startAliveMachine{"1"},
		setMachineStatus{"1", status.StatusStarted, ""},
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
				"model": model,
				"machines": M{
					"0": machine0,
					"1": machine1,
				},
				"applications": M{
					"mysql": mysqlCharm(M{
						"charm":        "local:quantal/mysql-1",
						"charm-origin": "local",
						"exposed":      true,
						"application-status": M{
							"current": "active",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"mysql/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "active",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"upgrading-from": "cs:quantal/mysql-1",
								"public-address": "controller-1.dns",
							},
						},
					}),
				},
			},
		},
	),
	test( // 17
		"deploy two services; set meter statuses on one",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("controller-0.dns")},
		startAliveMachine{"0"},
		setMachineStatus{"0", status.StatusStarted, ""},

		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("controller-1.dns")},
		startAliveMachine{"1"},
		setMachineStatus{"1", status.StatusStarted, ""},

		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", network.NewAddresses("controller-2.dns")},
		startAliveMachine{"2"},
		setMachineStatus{"2", status.StatusStarted, ""},

		addMachine{machineId: "3", job: state.JobHostUnits},
		setAddresses{"3", network.NewAddresses("controller-3.dns")},
		startAliveMachine{"3"},
		setMachineStatus{"3", status.StatusStarted, ""},

		addMachine{machineId: "4", job: state.JobHostUnits},
		setAddresses{"4", network.NewAddresses("controller-4.dns")},
		startAliveMachine{"4"},
		setMachineStatus{"4", status.StatusStarted, ""},

		addCharm{"mysql"},
		addService{name: "mysql", charm: "mysql"},
		setServiceExposed{"mysql", true},

		addService{name: "servicewithmeterstatus", charm: "mysql"},

		addAliveUnit{"mysql", "1"},
		addAliveUnit{"servicewithmeterstatus", "2"},
		addAliveUnit{"servicewithmeterstatus", "3"},
		addAliveUnit{"servicewithmeterstatus", "4"},

		setServiceExposed{"mysql", true},

		setAgentStatus{"mysql/0", status.StatusIdle, "", nil},
		setUnitStatus{"mysql/0", status.StatusActive, "", nil},
		setAgentStatus{"servicewithmeterstatus/0", status.StatusIdle, "", nil},
		setUnitStatus{"servicewithmeterstatus/0", status.StatusActive, "", nil},
		setAgentStatus{"servicewithmeterstatus/1", status.StatusIdle, "", nil},
		setUnitStatus{"servicewithmeterstatus/1", status.StatusActive, "", nil},
		setAgentStatus{"servicewithmeterstatus/2", status.StatusIdle, "", nil},
		setUnitStatus{"servicewithmeterstatus/2", status.StatusActive, "", nil},

		setUnitMeterStatus{"servicewithmeterstatus/1", "GREEN", "test green status"},
		setUnitMeterStatus{"servicewithmeterstatus/2", "RED", "test red status"},

		expect{
			"simulate just the two services and a bootstrap node",
			M{
				"model": model,
				"machines": M{
					"0": machine0,
					"1": machine1,
					"2": machine2,
					"3": machine3,
					"4": machine4,
				},
				"applications": M{
					"mysql": mysqlCharm(M{
						"exposed": true,
						"application-status": M{
							"current": "active",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"mysql/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "active",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-1.dns",
							},
						},
					}),

					"servicewithmeterstatus": mysqlCharm(M{
						"application-status": M{
							"current": "active",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"servicewithmeterstatus/0": M{
								"machine": "2",
								"workload-status": M{
									"current": "active",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-2.dns",
							},
							"servicewithmeterstatus/1": M{
								"machine": "3",
								"workload-status": M{
									"current": "active",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"meter-status": M{
									"color":   "green",
									"message": "test green status",
								},
								"public-address": "controller-3.dns",
							},
							"servicewithmeterstatus/2": M{
								"machine": "4",
								"workload-status": M{
									"current": "active",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"meter-status": M{
									"color":   "red",
									"message": "test red status",
								},
								"public-address": "controller-4.dns",
							},
						},
					}),
				},
			},
		},
	),
	test( // 18
		"upgrade available",
		setToolsUpgradeAvailable{},
		expect{
			"upgrade availability should be shown in model-status",
			M{
				"model": M{
					"name":              "controller",
					"controller":        "kontroll",
					"cloud":             "dummy",
					"region":            "dummy-region",
					"version":           "1.2.3",
					"upgrade-available": "1.2.4",
				},
				"machines":     M{},
				"applications": M{},
			},
		},
	),
	test( // 19
		"consistent workload version",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("controller-0.dns")},
		startAliveMachine{"0"},
		setMachineStatus{"0", status.StatusStarted, ""},

		addCharm{"mysql"},
		addService{name: "mysql", charm: "mysql"},

		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("controller-1.dns")},
		startAliveMachine{"1"},
		setMachineStatus{"1", status.StatusStarted, ""},
		addAliveUnit{"mysql", "1"},
		setUnitWorkloadVersion{"mysql/0", "the best!"},

		expect{
			"application and unit with correct workload version",
			M{
				"model": model,
				"machines": M{
					"0": machine0,
					"1": machine1,
				},
				"applications": M{
					"mysql": mysqlCharm(M{
						"version": "the best!",
						"application-status": M{
							"current": "unknown",
							"message": "Waiting for agent initialization to finish",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"mysql/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "unknown",
									"message": "Waiting for agent initialization to finish",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "allocating",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-1.dns",
							},
						},
					}),
				},
			},
		},
	),
	test( // 20
		"mixed workload version",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("controller-0.dns")},
		startAliveMachine{"0"},
		setMachineStatus{"0", status.StatusStarted, ""},

		addCharm{"mysql"},
		addService{name: "mysql", charm: "mysql"},

		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("controller-1.dns")},
		startAliveMachine{"1"},
		setMachineStatus{"1", status.StatusStarted, ""},
		addAliveUnit{"mysql", "1"},
		setUnitWorkloadVersion{"mysql/0", "the best!"},

		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", network.NewAddresses("controller-2.dns")},
		startAliveMachine{"2"},
		setMachineStatus{"2", status.StatusStarted, ""},
		addAliveUnit{"mysql", "2"},
		setUnitWorkloadVersion{"mysql/1", "not as good"},

		expect{
			"application and unit with correct workload version",
			M{
				"model": model,
				"machines": M{
					"0": machine0,
					"1": machine1,
					"2": machine2,
				},
				"applications": M{
					"mysql": mysqlCharm(M{
						"version": "not as good",
						"application-status": M{
							"current": "unknown",
							"message": "Waiting for agent initialization to finish",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"mysql/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "unknown",
									"message": "Waiting for agent initialization to finish",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "allocating",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-1.dns",
							},
							"mysql/1": M{
								"machine": "2",
								"workload-status": M{
									"current": "unknown",
									"message": "Waiting for agent initialization to finish",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "allocating",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "controller-2.dns",
							},
						},
					}),
				},
			},
		},
	),
}

func mysqlCharm(extras M) M {
	charm := M{
		"charm":        "cs:quantal/mysql-1",
		"charm-origin": "jujucharms",
		"charm-name":   "mysql",
		"charm-rev":    1,
		"series":       "quantal",
		"os":           "ubuntu",
		"exposed":      false,
	}
	for key, value := range extras {
		charm[key] = value
	}
	return charm
}

func dummyCharm(extras M) M {
	charm := M{
		"charm":        "cs:quantal/dummy-1",
		"charm-origin": "jujucharms",
		"charm-name":   "dummy",
		"charm-rev":    1,
		"series":       "quantal",
		"os":           "ubuntu",
		"exposed":      false,
	}
	for key, value := range extras {
		charm[key] = value
	}
	return charm
}

func wordpressCharm(extras M) M {
	charm := M{
		"charm":        "cs:quantal/wordpress-3",
		"charm-origin": "jujucharms",
		"charm-name":   "wordpress",
		"charm-rev":    3,
		"series":       "quantal",
		"os":           "ubuntu",
		"exposed":      false,
	}
	for key, value := range extras {
		charm[key] = value
	}
	return charm
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
	m, err := ctx.st.AddMachineInsideMachine(template, ac.parentId, instance.LXD)
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
	cfg, err := ctx.st.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	inst, hc := testing.AssertStartInstanceWithConstraints(c, ctx.env, cfg.ControllerUUID(), m.Id(), cons)
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
	cfg, err := ctx.st.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	_, hc := testing.AssertStartInstanceWithConstraints(c, ctx.env, cfg.ControllerUUID(), m.Id(), cons)
	err = m.SetProvisioned("i-missing", "fake_nonce", hc)
	c.Assert(err, jc.ErrorIsNil)
	// lp:1558657
	now := time.Now()
	s := status.StatusInfo{
		Status:  status.StatusUnknown,
		Message: "missing",
		Since:   &now,
	}
	err = m.SetInstanceStatus(s)
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
	cfg, err := ctx.st.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	inst, hc := testing.AssertStartInstanceWithConstraints(c, ctx.env, cfg.ControllerUUID(), m.Id(), cons)
	err = m.SetProvisioned(inst.Id(), "fake_nonce", hc)
	c.Assert(err, jc.ErrorIsNil)
	ctx.pingers[m.Id()] = pinger
}

type startMachineWithHardware struct {
	machineId string
	hc        instance.HardwareCharacteristics
}

func (sm startMachineWithHardware) step(c *gc.C, ctx *context) {
	m, err := ctx.st.Machine(sm.machineId)
	c.Assert(err, jc.ErrorIsNil)
	pinger := ctx.setAgentPresence(c, m)
	cons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := ctx.st.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	inst, _ := testing.AssertStartInstanceWithConstraints(c, ctx.env, cfg.ControllerUUID(), m.Id(), cons)
	err = m.SetProvisioned(inst.Id(), "fake_nonce", &sm.hc)
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
	err = m.SetProviderAddresses(sa.addresses...)
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

type setUnitTools struct {
	unitName string
	version  version.Binary
}

func (st setUnitTools) step(c *gc.C, ctx *context) {
	m, err := ctx.st.Unit(st.unitName)
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
	info := state.CharmInfo{
		Charm:       ch,
		ID:          curl,
		StoragePath: "dummy-path",
		SHA256:      fmt.Sprintf("%s-%d-sha256", name, rev),
	}
	dummy, err := ctx.st.AddCharm(info)
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
	name  string
	charm string
	cons  constraints.Value
}

func (as addService) step(c *gc.C, ctx *context) {
	ch, ok := ctx.charms[as.charm]
	c.Assert(ok, jc.IsTrue)
	svc, err := ctx.st.AddApplication(state.AddApplicationArgs{Name: as.name, Charm: ch})
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
	s, err := ctx.st.Application(sse.name)
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
	s, err := ctx.st.Application(ssc.name)
	c.Assert(err, jc.ErrorIsNil)
	cfg := state.SetCharmConfig{Charm: ch}
	err = s.SetCharm(cfg)
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
	s, err := ctx.st.Application(au.serviceName)
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
	s, err := ctx.st.Application(aau.serviceName)
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
	s, err := ctx.st.Application(sua.serviceName)
	c.Assert(err, jc.ErrorIsNil)
	us, err := s.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	for _, u := range us {
		ctx.pingers[u.Name()] = ctx.setAgentPresence(c, u)
	}
}

type setUnitMeterStatus struct {
	unitName string
	color    string
	message  string
}

func (s setUnitMeterStatus) step(c *gc.C, ctx *context) {
	u, err := ctx.st.Unit(s.unitName)
	c.Assert(err, jc.ErrorIsNil)
	err = u.SetMeterStatus(s.color, s.message)
	c.Assert(err, jc.ErrorIsNil)
}

type setUnitStatus struct {
	unitName   string
	status     status.Status
	statusInfo string
	statusData map[string]interface{}
}

func (sus setUnitStatus) step(c *gc.C, ctx *context) {
	u, err := ctx.st.Unit(sus.unitName)
	c.Assert(err, jc.ErrorIsNil)
	// lp:1558657
	now := time.Now()
	s := status.StatusInfo{
		Status:  sus.status,
		Message: sus.statusInfo,
		Data:    sus.statusData,
		Since:   &now,
	}
	err = u.SetStatus(s)
	c.Assert(err, jc.ErrorIsNil)
}

type setAgentStatus struct {
	unitName   string
	status     status.Status
	statusInfo string
	statusData map[string]interface{}
}

func (sus setAgentStatus) step(c *gc.C, ctx *context) {
	u, err := ctx.st.Unit(sus.unitName)
	c.Assert(err, jc.ErrorIsNil)
	// lp:1558657
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  sus.status,
		Message: sus.statusInfo,
		Data:    sus.statusData,
		Since:   &now,
	}
	err = u.SetAgentStatus(sInfo)
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
	// lp:1558657
	now := time.Now()
	s := status.StatusInfo{
		Status:  status.StatusActive,
		Message: "",
		Since:   &now,
	}
	err = u.SetStatus(s)
	c.Assert(err, jc.ErrorIsNil)
	sInfo := status.StatusInfo{
		Status:  status.StatusIdle,
		Message: "",
		Since:   &now,
	}
	err = u.SetAgentStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)

}

type setUnitWorkloadVersion struct {
	unitName string
	version  string
}

func (wv setUnitWorkloadVersion) step(c *gc.C, ctx *context) {
	u, err := ctx.st.Unit(wv.unitName)
	c.Assert(err, jc.ErrorIsNil)
	err = u.SetWorkloadVersion(wv.version)
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
	svc, err := ctx.st.Application(e.serviceName)
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
	status     status.Status
	statusInfo string
}

func (sms setMachineStatus) step(c *gc.C, ctx *context) {
	// lp:1558657
	now := time.Now()
	m, err := ctx.st.Machine(sms.machineId)
	c.Assert(err, jc.ErrorIsNil)
	sInfo := status.StatusInfo{
		Status:  sms.status,
		Message: sms.statusInfo,
		Since:   &now,
	}
	err = m.SetStatus(sInfo)
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
	eps, err := ctx.st.InferEndpoints(u.ApplicationName(), as.subService)
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

// substituteFakeTime replaces all "since" values
// in actual status output with a known fake value.
func substituteFakeSinceTime(c *gc.C, in []byte, expectIsoTime bool) []byte {
	// This regexp will work for yaml and json.
	exp := regexp.MustCompile(`(?P<since>"?since"?:\ ?)(?P<quote>"?)(?P<timestamp>[^("|\n)]*)*"?`)
	// Before the substritution is done, check that the timestamp produced
	// by status is in the correct format.
	if matches := exp.FindStringSubmatch(string(in)); matches != nil {
		for i, name := range exp.SubexpNames() {
			if name != "timestamp" {
				continue
			}
			timeFormat := "02 Jan 2006 15:04:05Z07:00"
			if expectIsoTime {
				timeFormat = "2006-01-02 15:04:05Z"
			}
			_, err := time.Parse(timeFormat, matches[i])
			c.Assert(err, jc.ErrorIsNil)
		}
	}

	out := exp.ReplaceAllString(string(in), `$since$quote<timestamp>$quote`)
	// Substitute a made up time used in our expected output.
	out = strings.Replace(out, "<timestamp>", "01 Apr 15 01:23+10:00", -1)
	return []byte(out)
}

func (e scopedExpect) step(c *gc.C, ctx *context) {
	c.Logf("\nexpect: %s %s\n", e.what, strings.Join(e.scope, " "))

	// Now execute the command for each format.
	for _, format := range statusFormats {
		c.Logf("format %q", format.name)
		// Run command with the required format.
		args := []string{"--format", format.name}
		if ctx.expectIsoTime {
			args = append(args, "--utc")
		}
		args = append(args, e.scope...)
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
		out := substituteFakeSinceTime(c, stdout, ctx.expectIsoTime)
		err = format.unmarshal(out, &actual)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(actual, jc.DeepEquals, expected)
	}
}

func (e expect) step(c *gc.C, ctx *context) {
	scopedExpect{e.what, nil, e.output}.step(c, ctx)
}

type setToolsUpgradeAvailable struct{}

func (ua setToolsUpgradeAvailable) step(c *gc.C, ctx *context) {
	model, err := ctx.st.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.UpdateLatestToolsVersion(nextVersion)
	c.Assert(err, jc.ErrorIsNil)
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

func (s *StatusSuite) TestMigrationInProgress(c *gc.C) {
	// This test isn't part of statusTests because migrations can't be
	// run on controller models.
	st := s.setupMigrationTest(c)
	defer st.Close()

	expected := M{
		"model": M{
			"name":       "hosted",
			"controller": "kontroll",
			"cloud":      "dummy",
			"region":     "dummy-region",
			"version":    "1.2.3",
			"migration":  "foo bar",
		},
		"machines":     M{},
		"applications": M{},
	}

	for _, format := range statusFormats {
		code, stdout, stderr := runStatus(c, "-m", "hosted", "--format", format.name)
		c.Check(code, gc.Equals, 0)
		c.Assert(stderr, gc.HasLen, 0, gc.Commentf("status failed: %s", stderr))

		// Roundtrip expected through format so that types will match.
		buf, err := format.marshal(expected)
		c.Assert(err, jc.ErrorIsNil)
		var expectedForFormat M
		err = format.unmarshal(buf, &expectedForFormat)
		c.Assert(err, jc.ErrorIsNil)

		var actual M
		c.Assert(format.unmarshal(stdout, &actual), jc.ErrorIsNil)
		c.Check(actual, jc.DeepEquals, expectedForFormat)
	}
}

func (s *StatusSuite) TestMigrationInProgressTabular(c *gc.C) {
	expected := `
MODEL   CONTROLLER  CLOUD/REGION        VERSION  NOTES
hosted  kontroll    dummy/dummy-region  1.2.3    migrating: foo bar

APP  VERSION  STATUS  SCALE  CHARM  STORE  REV  OS  NOTES

UNIT  WORKLOAD  AGENT  MACHINE  PUBLIC-ADDRESS  PORTS  MESSAGE

MACHINE  STATE  DNS  INS-ID  SERIES  AZ

`[1:]

	st := s.setupMigrationTest(c)
	defer st.Close()
	code, stdout, stderr := runStatus(c, "-m", "hosted", "--format", "tabular")
	c.Check(code, gc.Equals, 0)
	c.Assert(stderr, gc.HasLen, 0, gc.Commentf("status failed: %s", stderr))
	c.Assert(string(stdout), gc.Equals, expected)
}

func (s *StatusSuite) TestMigrationInProgressAndUpgradeAvailable(c *gc.C) {
	expected := `
MODEL   CONTROLLER  CLOUD/REGION        VERSION  NOTES
hosted  kontroll    dummy/dummy-region  1.2.3    migrating: foo bar

APP  VERSION  STATUS  SCALE  CHARM  STORE  REV  OS  NOTES

UNIT  WORKLOAD  AGENT  MACHINE  PUBLIC-ADDRESS  PORTS  MESSAGE

MACHINE  STATE  DNS  INS-ID  SERIES  AZ

`[1:]

	st := s.setupMigrationTest(c)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.UpdateLatestToolsVersion(nextVersion)
	c.Assert(err, jc.ErrorIsNil)

	code, stdout, stderr := runStatus(c, "-m", "hosted", "--format", "tabular")
	c.Check(code, gc.Equals, 0)
	c.Assert(stderr, gc.HasLen, 0, gc.Commentf("status failed: %s", stderr))
	c.Assert(string(stdout), gc.Equals, expected)
}

func (s *StatusSuite) setupMigrationTest(c *gc.C) *state.State {
	const hostedModelName = "hosted"
	const statusText = "foo bar"

	f := factory.NewFactory(s.BackingState)
	hostedSt := f.MakeModel(c, &factory.ModelParams{
		Name: hostedModelName,
	})

	mig, err := hostedSt.CreateMigration(state.MigrationSpec{
		InitiatedBy: names.NewUserTag("admin"),
		TargetInfo: migration.TargetInfo{
			ControllerTag: names.NewModelTag(utils.MustNewUUID().String()),
			Addrs:         []string{"1.2.3.4:5555", "4.3.2.1:6666"},
			CACert:        "cert",
			AuthTag:       names.NewUserTag("user"),
			Password:      "password",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = mig.SetStatusMessage(statusText)
	c.Assert(err, jc.ErrorIsNil)

	return hostedSt
}

type fakeApiClient struct {
	statusReturn *params.FullStatus
	patternsUsed []string
	closeCalled  bool
}

func (a *fakeApiClient) Status(patterns []string) (*params.FullStatus, error) {
	a.patternsUsed = patterns
	return a.statusReturn, nil
}

func (a *fakeApiClient) Close() error {
	a.closeCalled = true
	return nil
}

func (s *StatusSuite) TestStatusWithFormatSummary(c *gc.C) {
	ctx := s.newContext(c)
	defer s.resetContext(c, ctx)
	steps := []stepper{
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("localhost")},
		startAliveMachine{"0"},
		setMachineStatus{"0", status.StatusStarted, ""},
		addCharm{"wordpress"},
		addCharm{"mysql"},
		addCharm{"logging"},
		addService{name: "wordpress", charm: "wordpress"},
		setServiceExposed{"wordpress", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("localhost")},
		startAliveMachine{"1"},
		setMachineStatus{"1", status.StatusStarted, ""},
		addAliveUnit{"wordpress", "1"},
		setAgentStatus{"wordpress/0", status.StatusIdle, "", nil},
		setUnitStatus{"wordpress/0", status.StatusActive, "", nil},
		addService{name: "mysql", charm: "mysql"},
		setServiceExposed{"mysql", true},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", network.NewAddresses("10.0.0.1")},
		startAliveMachine{"2"},
		setMachineStatus{"2", status.StatusStarted, ""},
		addAliveUnit{"mysql", "2"},
		setAgentStatus{"mysql/0", status.StatusIdle, "", nil},
		setUnitStatus{"mysql/0", status.StatusActive, "", nil},
		addService{name: "logging", charm: "logging"},
		setServiceExposed{"logging", true},
		relateServices{"wordpress", "mysql"},
		relateServices{"wordpress", "logging"},
		relateServices{"mysql", "logging"},
		addSubordinate{"wordpress/0", "logging"},
		addSubordinate{"mysql/0", "logging"},
		setUnitsAlive{"logging"},
		setAgentStatus{"logging/0", status.StatusIdle, "", nil},
		setUnitStatus{"logging/0", status.StatusActive, "", nil},
		setAgentStatus{"logging/1", status.StatusError, "somehow lost in all those logs", nil},
	}
	for _, s := range steps {
		s.step(c, ctx)
	}
	code, stdout, stderr := runStatus(c, "--format", "summary")
	c.Check(code, gc.Equals, 0)
	c.Check(string(stderr), gc.Equals, "")
	c.Assert(string(stdout), gc.Equals, `
Running on subnets:  127.0.0.1/8, 10.0.0.1/8  
 Utilizing ports:                             
      # MACHINES:  (3)
         started:   3 
                 
         # UNITS:  (4)
          active:   3 
           error:   1 
                 
  # APPLICATIONS:  (3)
          logging  1/1  exposed
            mysql  1/1  exposed
        wordpress  1/1  exposed

`[1:])
}
func (s *StatusSuite) TestStatusWithFormatOneline(c *gc.C) {
	ctx := s.newContext(c)
	defer s.resetContext(c, ctx)
	steps := []stepper{
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("controller-0.dns")},
		startAliveMachine{"0"},
		setMachineStatus{"0", status.StatusStarted, ""},
		addCharm{"wordpress"},
		addCharm{"mysql"},
		addCharm{"logging"},

		addService{name: "wordpress", charm: "wordpress"},
		setServiceExposed{"wordpress", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("controller-1.dns")},
		startAliveMachine{"1"},
		setMachineStatus{"1", status.StatusStarted, ""},
		addAliveUnit{"wordpress", "1"},
		setAgentStatus{"wordpress/0", status.StatusIdle, "", nil},
		setUnitStatus{"wordpress/0", status.StatusActive, "", nil},

		addService{name: "mysql", charm: "mysql"},
		setServiceExposed{"mysql", true},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", network.NewAddresses("controller-2.dns")},
		startAliveMachine{"2"},
		setMachineStatus{"2", status.StatusStarted, ""},
		addAliveUnit{"mysql", "2"},
		setAgentStatus{"mysql/0", status.StatusIdle, "", nil},
		setUnitStatus{"mysql/0", status.StatusActive, "", nil},

		addService{name: "logging", charm: "logging"},
		setServiceExposed{"logging", true},

		relateServices{"wordpress", "mysql"},
		relateServices{"wordpress", "logging"},
		relateServices{"mysql", "logging"},

		addSubordinate{"wordpress/0", "logging"},
		addSubordinate{"mysql/0", "logging"},

		setUnitsAlive{"logging"},
		setAgentStatus{"logging/0", status.StatusIdle, "", nil},
		setUnitStatus{"logging/0", status.StatusActive, "", nil},
		setAgentStatus{"logging/1", status.StatusError, "somehow lost in all those logs", nil},
	}

	ctx.run(c, steps)

	const expected = `
- mysql/0: controller-2.dns (agent:idle, workload:active)
  - logging/1: controller-2.dns (agent:idle, workload:error)
- wordpress/0: controller-1.dns (agent:idle, workload:active)
  - logging/0: controller-1.dns (agent:idle, workload:active)
`
	assertOneLineStatus(c, expected)
}

func assertOneLineStatus(c *gc.C, expected string) {
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

func (s *StatusSuite) prepareTabularData(c *gc.C) *context {
	ctx := s.newContext(c)
	steps := []stepper{
		setToolsUpgradeAvailable{},
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("controller-0.dns")},
		startMachineWithHardware{"0", instance.MustParseHardware("availability-zone=us-east-1a")},
		setMachineStatus{"0", status.StatusStarted, ""},
		addCharm{"wordpress"},
		addCharm{"mysql"},
		addCharm{"logging"},
		addService{name: "wordpress", charm: "wordpress"},
		setServiceExposed{"wordpress", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("controller-1.dns")},
		startAliveMachine{"1"},
		setMachineStatus{"1", status.StatusStarted, ""},
		addAliveUnit{"wordpress", "1"},
		setAgentStatus{"wordpress/0", status.StatusIdle, "", nil},
		setUnitStatus{"wordpress/0", status.StatusActive, "", nil},
		setUnitTools{"wordpress/0", version.MustParseBinary("1.2.3-trusty-ppc")},
		addService{name: "mysql", charm: "mysql"},
		setServiceExposed{"mysql", true},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", network.NewAddresses("controller-2.dns")},
		startAliveMachine{"2"},
		setMachineStatus{"2", status.StatusStarted, ""},
		addAliveUnit{"mysql", "2"},
		setAgentStatus{"mysql/0", status.StatusIdle, "", nil},
		setUnitStatus{
			"mysql/0",
			status.StatusMaintenance,
			"installing all the things", nil},
		setUnitTools{"mysql/0", version.MustParseBinary("1.2.3-trusty-ppc")},
		addService{name: "logging", charm: "logging"},
		setServiceExposed{"logging", true},
		relateServices{"wordpress", "mysql"},
		relateServices{"wordpress", "logging"},
		relateServices{"mysql", "logging"},
		addSubordinate{"wordpress/0", "logging"},
		addSubordinate{"mysql/0", "logging"},
		setUnitsAlive{"logging"},
		setAgentStatus{"logging/0", status.StatusIdle, "", nil},
		setUnitStatus{"logging/0", status.StatusActive, "", nil},
		setAgentStatus{"logging/1", status.StatusError, "somehow lost in all those logs", nil},
		setUnitWorkloadVersion{"logging/1", "a bit too long, really"},
		setUnitWorkloadVersion{"wordpress/0", "4.5.3"},
		setUnitWorkloadVersion{"mysql/0", "5.7.13"},
	}
	for _, s := range steps {
		s.step(c, ctx)
	}
	return ctx
}

func (s *StatusSuite) testStatusWithFormatTabular(c *gc.C, useFeatureFlag bool) {
	ctx := s.prepareTabularData(c)
	defer s.resetContext(c, ctx)
	var args []string
	if !useFeatureFlag {
		args = []string{"--format", "tabular"}
	}
	code, stdout, stderr := runStatus(c, args...)
	c.Check(code, gc.Equals, 0)
	c.Check(string(stderr), gc.Equals, "")
	expected := `
MODEL       CONTROLLER  CLOUD/REGION        VERSION  NOTES
controller  kontroll    dummy/dummy-region  1.2.3    upgrade available: 1.2.4

APP        VERSION  STATUS       SCALE  CHARM      STORE       REV  OS      NOTES
logging    a bi...               2/2    logging    jujucharms  1    ubuntu  exposed
mysql      5.7.13   maintenance  1/1    mysql      jujucharms  1    ubuntu  exposed
wordpress  4.5.3    active       1/1    wordpress  jujucharms  3    ubuntu  exposed

RELATION           PROVIDES   CONSUMES   TYPE
juju-info          logging    mysql      regular
logging-dir        logging    wordpress  regular
info               mysql      logging    subordinate
db                 mysql      wordpress  regular
logging-directory  wordpress  logging    subordinate

UNIT         WORKLOAD     AGENT  MACHINE  PUBLIC-ADDRESS    PORTS  MESSAGE
mysql/0      maintenance  idle   2        controller-2.dns         installing all the things
  logging/1  error        idle            controller-2.dns         somehow lost in all those logs
wordpress/0  active       idle   1        controller-1.dns         
  logging/0  active       idle            controller-1.dns         

MACHINE  STATE    DNS               INS-ID        SERIES   AZ
0        started  controller-0.dns  controller-0  quantal  us-east-1a
1        started  controller-1.dns  controller-1  quantal  
2        started  controller-2.dns  controller-2  quantal  

`[1:]
	c.Assert(string(stdout), gc.Equals, expected)
}

func (s *StatusSuite) TestStatusWithFormatTabular(c *gc.C) {
	s.testStatusWithFormatTabular(c, false)
}

func (s *StatusSuite) TestFormatTabularHookActionName(c *gc.C) {
	status := formattedStatus{
		Applications: map[string]applicationStatus{
			"foo": {
				Units: map[string]unitStatus{
					"foo/0": {
						JujuStatusInfo: statusInfoContents{
							Current: status.StatusExecuting,
							Message: "running config-changed hook",
						},
						WorkloadStatusInfo: statusInfoContents{
							Current: status.StatusMaintenance,
							Message: "doing some work",
						},
					},
					"foo/1": {
						JujuStatusInfo: statusInfoContents{
							Current: status.StatusExecuting,
							Message: "running action backup database",
						},
						WorkloadStatusInfo: statusInfoContents{
							Current: status.StatusMaintenance,
							Message: "doing some work",
						},
					},
				},
			},
		},
	}
	out := &bytes.Buffer{}
	err := FormatTabular(out, false, status)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out.String(), gc.Equals, `
MODEL  CONTROLLER  CLOUD/REGION  VERSION
                                 

APP  VERSION  STATUS  SCALE  CHARM  STORE  REV  OS  NOTES
foo                   2/2                  0        

UNIT   WORKLOAD     AGENT      MACHINE  PUBLIC-ADDRESS  PORTS  MESSAGE
foo/0  maintenance  executing                                  (config-changed) doing some work
foo/1  maintenance  executing                                  (backup database) doing some work

MACHINE  STATE  DNS  INS-ID  SERIES  AZ
`[1:])
}

func (s *StatusSuite) TestFormatTabularConsistentPeerRelationName(c *gc.C) {
	status := formattedStatus{
		Applications: map[string]applicationStatus{
			"foo": {
				Relations: map[string][]string{
					"coordinator":  {"foo"},
					"frobulator":   {"foo"},
					"encapsulator": {"foo"},
					"catchulator":  {"foo"},
					"perforator":   {"foo"},
					"deliverator":  {"foo"},
					"replicator":   {"foo"},
				},
			},
		},
	}
	out := &bytes.Buffer{}
	err := FormatTabular(out, false, status)
	c.Assert(err, jc.ErrorIsNil)
	sections, err := splitTableSections(out.Bytes())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sections["RELATION"], gc.DeepEquals, []string{
		"RELATION    PROVIDES  CONSUMES  TYPE",
		"replicator  foo       foo       peer",
	})
}

func (s *StatusSuite) TestStatusWithNilStatusApi(c *gc.C) {
	ctx := s.newContext(c)
	defer s.resetContext(c, ctx)
	steps := []stepper{
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("controller-0.dns")},
		startAliveMachine{"0"},
		setMachineStatus{"0", status.StatusStarted, ""},
	}

	for _, s := range steps {
		s.step(c, ctx)
	}

	client := fakeApiClient{}
	var status = client.Status
	s.PatchValue(&status, func(_ []string) (*params.FullStatus, error) {
		return nil, nil
	})
	s.PatchValue(&newApiClientForStatus, func(_ *statusCommand) (statusAPI, error) {
		return &client, nil
	})

	code, _, stderr := runStatus(c, "--format", "tabular")
	c.Check(code, gc.Equals, 1)
	c.Check(string(stderr), gc.Equals, "error: unable to obtain the current status\n")
}

func (s *StatusSuite) TestFormatTabularMetering(c *gc.C) {
	status := formattedStatus{
		Applications: map[string]applicationStatus{
			"foo": {
				Units: map[string]unitStatus{
					"foo/0": {
						MeterStatus: &meterStatus{
							Color:   "strange",
							Message: "warning: stable strangelets",
						},
					},
					"foo/1": {
						MeterStatus: &meterStatus{
							Color:   "up",
							Message: "things are looking up",
						},
					},
				},
			},
		},
	}
	out := &bytes.Buffer{}
	err := FormatTabular(out, false, status)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out.String(), gc.Equals, `
MODEL  CONTROLLER  CLOUD/REGION  VERSION
                                 

APP  VERSION  STATUS  SCALE  CHARM  STORE  REV  OS  NOTES
foo                   0/2                  0        

UNIT   WORKLOAD  AGENT  MACHINE  PUBLIC-ADDRESS  PORTS  MESSAGE
foo/0                                                   
foo/1                                                   

METER  STATUS   MESSAGE
foo/0  strange  warning: stable strangelets
foo/1  up       things are looking up

MACHINE  STATE  DNS  INS-ID  SERIES  AZ
`[1:])
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
		addMachine{machineId: "0", job: state.JobManageModel},
		startAliveMachine{"0"},
		setMachineStatus{"0", status.StatusStarted, ""},
		// And the machine's address is "controller-0.dns"
		setAddresses{"0", network.NewAddresses("controller-0.dns")},
		// And a container is started
		// And the container's ID is "0/lxd/0"
		addContainer{"0", "0/lxd/0", state.JobHostUnits},

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
		setMachineStatus{"1", status.StatusStarted, ""},
		// And the machine's address is "controller-1.dns"
		setAddresses{"1", network.NewAddresses("controller-1.dns")},
		// And a unit of "wordpress" is deployed to machine "1"
		addAliveUnit{"wordpress", "1"},
		// And the unit is started
		setAgentStatus{"wordpress/0", status.StatusIdle, "", nil},
		setUnitStatus{"wordpress/0", status.StatusActive, "", nil},
		// And a machine is started

		// And the machine's ID is "2"
		// And the machine's job is to host units
		addMachine{machineId: "2", job: state.JobHostUnits},
		startAliveMachine{"2"},
		setMachineStatus{"2", status.StatusStarted, ""},
		// And the machine's address is "controller-2.dns"
		setAddresses{"2", network.NewAddresses("controller-2.dns")},
		// And a unit of "mysql" is deployed to machine "2"
		addAliveUnit{"mysql", "2"},
		// And the unit is started
		setAgentStatus{"mysql/0", status.StatusIdle, "", nil},
		setUnitStatus{"mysql/0", status.StatusActive, "", nil},
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
		setAgentStatus{"logging/0", status.StatusIdle, "", nil},
		setUnitStatus{"logging/0", status.StatusActive, "", nil},
		// And the "logging" service is a subordinate to unit 0 of the "mysql" service
		addSubordinate{"mysql/0", "logging"},
		setAgentStatus{"logging/1", status.StatusIdle, "", nil},
		setUnitStatus{"logging/1", status.StatusActive, "", nil},
		setUnitsAlive{"logging"},
	}

	ctx.run(c, steps)
	return ctx
}

// Scenario: One unit is in an errored state and user filters to active
func (s *StatusSuite) TestFilterToActive(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	// Given unit 1 of the "logging" service has an error
	setAgentStatus{"logging/1", status.StatusError, "mock error", nil}.step(c, ctx)
	// And unit 0 of the "mysql" service has an error
	setAgentStatus{"mysql/0", status.StatusError, "mock error", nil}.step(c, ctx)
	// When I run juju status --format oneline started
	_, stdout, stderr := runStatus(c, "--format", "oneline", "active")
	c.Assert(string(stderr), gc.Equals, "")
	// Then I should receive output prefixed with:
	const expected = `

- wordpress/0: controller-1.dns (agent:idle, workload:active)
  - logging/0: controller-1.dns (agent:idle, workload:active)
`
	c.Assert(string(stdout), gc.Equals, expected[1:])
}

// Scenario: user filters to a single machine
func (s *StatusSuite) TestFilterToMachine(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	// When I run juju status --format oneline 1
	_, stdout, stderr := runStatus(c, "--format", "oneline", "1")
	c.Assert(string(stderr), gc.Equals, "")
	// Then I should receive output prefixed with:
	const expected = `

- wordpress/0: controller-1.dns (agent:idle, workload:active)
  - logging/0: controller-1.dns (agent:idle, workload:active)
`
	c.Assert(string(stdout), gc.Equals, expected[1:])
}

// Scenario: user filters to a machine, shows containers
func (s *StatusSuite) TestFilterToMachineShowsContainer(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	// When I run juju status --format yaml 0
	_, stdout, stderr := runStatus(c, "--format", "yaml", "0")
	c.Assert(string(stderr), gc.Equals, "")
	// Then I should receive output matching:
	const expected = "(.|\n)*machines:(.|\n)*\"0\"(.|\n)*0/lxd/0(.|\n)*"
	c.Assert(string(stdout), gc.Matches, expected)
}

// Scenario: user filters to a container
func (s *StatusSuite) TestFilterToContainer(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	// When I run juju status --format yaml 0/lxd/0
	_, stdout, stderr := runStatus(c, "--format", "yaml", "0/lxd/0")
	c.Assert(string(stderr), gc.Equals, "")
	out := substituteFakeSinceTime(c, stdout, ctx.expectIsoTime)
	const expected = "" +
		"model:\n" +
		"  name: controller\n" +
		"  controller: kontroll\n" +
		"  cloud: dummy\n" +
		"  region: dummy-region\n" +
		"  version: 1.2.3\n" +
		"machines:\n" +
		"  \"0\":\n" +
		"    juju-status:\n" +
		"      current: started\n" +
		"      since: 01 Apr 15 01:23+10:00\n" +
		"    dns-name: controller-0.dns\n" +
		"    instance-id: controller-0\n" +
		"    machine-status:\n" +
		"      current: pending\n" +
		"      since: 01 Apr 15 01:23+10:00\n" +
		"    series: quantal\n" +
		"    containers:\n" +
		"      0/lxd/0:\n" +
		"        juju-status:\n" +
		"          current: pending\n" +
		"          since: 01 Apr 15 01:23+10:00\n" +
		"        instance-id: pending\n" +
		"        machine-status:\n" +
		"          current: pending\n" +
		"          since: 01 Apr 15 01:23+10:00\n" +
		"        series: quantal\n" +
		"    hardware: arch=amd64 cpu-cores=1 mem=1024M root-disk=8192M\n" +
		"    controller-member-status: adding-vote\n" +
		"applications: {}\n"

	c.Assert(string(out), gc.Equals, expected)
}

// Scenario: One unit is in an errored state and user filters to errored
func (s *StatusSuite) TestFilterToErrored(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	// Given unit 1 of the "logging" service has an error
	setAgentStatus{"logging/1", status.StatusError, "mock error", nil}.step(c, ctx)
	// When I run juju status --format oneline error
	_, stdout, stderr := runStatus(c, "--format", "oneline", "error")
	c.Assert(stderr, gc.IsNil)
	// Then I should receive output prefixed with:
	const expected = `

- mysql/0: controller-2.dns (agent:idle, workload:active)
  - logging/1: controller-2.dns (agent:idle, workload:error)
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

- mysql/0: controller-2.dns (agent:idle, workload:active)
  - logging/1: controller-2.dns (agent:idle, workload:active)
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

- mysql/0: controller-2.dns (agent:idle, workload:active)
  - logging/1: controller-2.dns (agent:idle, workload:active)
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

- wordpress/0: controller-1.dns (agent:idle, workload:active)
  - logging/0: controller-1.dns (agent:idle, workload:active)
`
	c.Assert(string(stdout), gc.Equals, expected[1:])
}

// Scenario: Filtering on Subnets
func (s *StatusSuite) TestFilterOnSubnet(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	// Given the address for machine "1" is "localhost"
	setAddresses{"1", network.NewAddresses("localhost", "127.0.0.1")}.step(c, ctx)
	// And the address for machine "2" is "10.0.0.1"
	setAddresses{"2", network.NewAddresses("10.0.0.1")}.step(c, ctx)
	// When I run juju status --format oneline 127.0.0.1
	_, stdout, stderr := runStatus(c, "--format", "oneline", "127.0.0.1")
	c.Assert(stderr, gc.IsNil)
	// Then I should receive output prefixed with:
	const expected = `

- wordpress/0: localhost (agent:idle, workload:active)
  - logging/0: localhost (agent:idle, workload:active)
`
	c.Assert(string(stdout), gc.Equals, expected[1:])
}

// Scenario: Filtering on Ports
func (s *StatusSuite) TestFilterOnPorts(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	// Given the address for machine "1" is "localhost"
	setAddresses{"1", network.NewAddresses("localhost")}.step(c, ctx)
	// And the address for machine "2" is "10.0.0.1"
	setAddresses{"2", network.NewAddresses("10.0.0.1")}.step(c, ctx)
	openUnitPort{"wordpress/0", "tcp", 80}.step(c, ctx)
	// When I run juju status --format oneline 80/tcp
	_, stdout, stderr := runStatus(c, "--format", "oneline", "80/tcp")
	c.Assert(stderr, gc.IsNil)
	// Then I should receive output prefixed with:
	const expected = `

- wordpress/0: localhost (agent:idle, workload:active) 80/tcp
  - logging/0: localhost (agent:idle, workload:active)
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

- mysql/0: controller-2.dns (agent:idle, workload:active)
  - logging/1: controller-2.dns (agent:idle, workload:active)
- wordpress/0: controller-1.dns (agent:idle, workload:active)
  - logging/0: controller-1.dns (agent:idle, workload:active)
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

- mysql/0: controller-2.dns (agent:idle, workload:active)
  - logging/1: controller-2.dns (agent:idle, workload:active)
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

- mysql/0: controller-2.dns (agent:idle, workload:active)
  - logging/1: controller-2.dns (agent:idle, workload:active)
- wordpress/0: controller-1.dns (agent:idle, workload:active)
  - logging/0: controller-1.dns (agent:idle, workload:active)
`
	c.Assert(string(stdout), gc.Equals, expected[1:])
}

func (s *StatusSuite) TestFilterMultipleHeterogenousPatterns(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	_, stdout, stderr := runStatus(c, "--format", "oneline", "wordpress/0", "active")
	c.Assert(stderr, gc.IsNil)
	// Then I should receive output prefixed with:
	const expected = `

- mysql/0: controller-2.dns (agent:idle, workload:active)
  - logging/1: controller-2.dns (agent:idle, workload:active)
- wordpress/0: controller-1.dns (agent:idle, workload:active)
  - logging/0: controller-1.dns (agent:idle, workload:active)
`
	c.Assert(string(stdout), gc.Equals, expected[1:])
}

// TestSummaryStatusWithUnresolvableDns is result of bug# 1410320.
func (s *StatusSuite) TestSummaryStatusWithUnresolvableDns(c *gc.C) {
	formatter := &summaryFormatter{}
	formatter.resolveAndTrackIp("invalidDns")
	// Test should not panic.
}

func initStatusCommand(args ...string) (*statusCommand, error) {
	com := &statusCommand{}
	return com, coretesting.InitCommand(modelcmd.Wrap(com), args)
}

var statusInitTests = []struct {
	args    []string
	envVar  string
	isoTime bool
	err     string
}{
	{
		isoTime: false,
	}, {
		args:    []string{"--utc"},
		isoTime: true,
	}, {
		envVar:  "true",
		isoTime: true,
	}, {
		envVar: "foo",
		err:    "invalid JUJU_STATUS_ISO_TIME env var, expected true|false.*",
	},
}

func (*StatusSuite) TestStatusCommandInit(c *gc.C) {
	defer os.Setenv(osenv.JujuStatusIsoTimeEnvKey, os.Getenv(osenv.JujuStatusIsoTimeEnvKey))

	for i, t := range statusInitTests {
		c.Logf("test %d", i)
		os.Setenv(osenv.JujuStatusIsoTimeEnvKey, t.envVar)
		com, err := initStatusCommand(t.args...)
		if t.err != "" {
			c.Check(err, gc.ErrorMatches, t.err)
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
		c.Check(com.isoTime, gc.DeepEquals, t.isoTime)
	}
}

var statusTimeTest = test(
	"status generates timestamps as UTC in ISO format",
	addMachine{machineId: "0", job: state.JobManageModel},
	setAddresses{"0", network.NewAddresses("controller-0.dns")},
	startAliveMachine{"0"},
	setMachineStatus{"0", status.StatusStarted, ""},
	addCharm{"dummy"},
	addService{name: "dummy-application", charm: "dummy"},

	addMachine{machineId: "1", job: state.JobHostUnits},
	startAliveMachine{"1"},
	setAddresses{"1", network.NewAddresses("controller-1.dns")},
	setMachineStatus{"1", status.StatusStarted, ""},

	addAliveUnit{"dummy-application", "1"},
	expect{
		"add two units, one alive (in error state), one started",
		M{
			"model": M{
				"name":       "controller",
				"controller": "kontroll",
				"cloud":      "dummy",
				"region":     "dummy-region",
				"version":    "1.2.3",
			},
			"machines": M{
				"0": machine0,
				"1": machine1,
			},
			"applications": M{
				"dummy-application": dummyCharm(M{
					"application-status": M{
						"current": "unknown",
						"message": "Waiting for agent initialization to finish",
						"since":   "01 Apr 15 01:23+10:00",
					},
					"units": M{
						"dummy-application/0": M{
							"machine": "1",
							"workload-status": M{
								"current": "unknown",
								"message": "Waiting for agent initialization to finish",
								"since":   "01 Apr 15 01:23+10:00",
							},
							"juju-status": M{
								"current": "allocating",
								"since":   "01 Apr 15 01:23+10:00",
							},
							"public-address": "controller-1.dns",
						},
					},
				}),
			},
		},
	},
)

func (s *StatusSuite) TestIsoTimeFormat(c *gc.C) {
	func(t testCase) {
		// Prepare context and run all steps to setup.
		ctx := s.newContext(c)
		ctx.expectIsoTime = true
		defer s.resetContext(c, ctx)
		ctx.run(c, t.steps)
	}(statusTimeTest)
}

func (s *StatusSuite) TestFormatProvisioningError(c *gc.C) {
	status := &params.FullStatus{
		Machines: map[string]params.MachineStatus{
			"1": {
				AgentStatus: params.DetailedStatus{
					Status: "error",
					Info:   "<error while provisioning>",
				},
				InstanceId:     "pending",
				InstanceStatus: params.DetailedStatus{},
				Series:         "trusty",
				Id:             "1",
				Jobs:           []multiwatcher.MachineJob{"JobHostUnits"},
			},
		},
	}
	formatter := NewStatusFormatter(status, true)
	formatted := formatter.format()

	c.Check(formatted, jc.DeepEquals, formattedStatus{
		Machines: map[string]machineStatus{
			"1": {
				JujuStatus: statusInfoContents{Current: "error", Message: "<error while provisioning>"},
				InstanceId: "pending",
				Series:     "trusty",
				Id:         "1",
				Containers: map[string]machineStatus{},
			},
		},
		Applications: map[string]applicationStatus{},
	})
}

type tableSections map[string][]string

func sectionTitle(lines []string) string {
	return strings.SplitN(lines[0], " ", 2)[0]
}

func splitTableSections(tableData []byte) (tableSections, error) {
	scanner := bufio.NewScanner(bytes.NewReader(tableData))
	result := make(tableSections)
	var current []string
	for scanner.Scan() {
		if line := scanner.Text(); line == "" && current != nil {
			result[sectionTitle(current)] = current
			current = nil
		} else if line != "" {
			current = append(current, line)
		}
	}
	if scanner.Err() != nil {
		return nil, scanner.Err()
	}
	if current != nil {
		result[sectionTitle(current)] = current
	}
	return result, nil
}
