// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/version"
	"github.com/kr/pretty"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/migration"
	corenetwork "github.com/juju/juju/core/network"
	corepresence "github.com/juju/juju/core/presence"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	environscontext "github.com/juju/juju/environs/context"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/presence"
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
	ctx := cmdtesting.Context(c)
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

func newContext(st *state.State, pool *state.StatePool, env environs.Environ, adminUserTag string) *context {
	// We make changes in the API server's state so that
	// our changes to presence are immediately noticed
	// in the status.
	return &context{
		st:           st,
		pool:         pool,
		env:          env,
		statusSetter: env.(agentStatusSetter),
		charms:       make(map[string]*state.Charm),
		pingers:      make(map[string]*presence.Pinger),
		adminUserTag: adminUserTag,
	}
}

type agentStatusSetter interface {
	SetAgentStatus(agent string, status corepresence.Status)
}

type context struct {
	st            *state.State
	pool          *state.StatePool
	env           environs.Environ
	statusSetter  agentStatusSetter
	charms        map[string]*state.Charm
	pingers       map[string]*presence.Pinger
	adminUserTag  string // A string repr of the tag.
	expectIsoTime bool
	skipTest      bool
}

func (ctx *context) reset(c *gc.C) {
	for _, up := range ctx.pingers {
		err := up.KillForTesting()
		c.Check(err, jc.ErrorIsNil)
	}
}

func (ctx *context) run(c *gc.C, steps []stepper) {
	for i, s := range steps {
		if ctx.skipTest {
			c.Logf("skipping test %d", i)
			return
		}
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
	return newContext(st, s.StatePool, s.Environ, s.AdminUserTag(c).String())
}

func (s *StatusSuite) resetContext(c *gc.C, ctx *context) {
	ctx.reset(c)
	s.JujuConnSuite.Reset(c)
}

// shortcuts for expected output.
var (
	model = M{
		"name":       "controller",
		"type":       "iaas",
		"controller": "kontroll",
		"cloud":      "dummy",
		"region":     "dummy-region",
		"version":    "1.2.3",
		"model-status": M{
			"current": "available",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"sla": "unsupported",
	}

	machine0 = M{
		"juju-status": M{
			"current": "started",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"dns-name":     "10.0.0.1",
		"ip-addresses": []string{"10.0.0.1"},
		"instance-id":  "controller-0",
		"machine-status": M{
			"current": "pending",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"modification-status": M{
			"current": "idle",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"series": "quantal",
		"network-interfaces": M{
			"eth0": M{
				"ip-addresses": []string{"10.0.0.1"},
				"mac-address":  "aa:bb:cc:dd:ee:ff",
				"is-up":        true,
			},
		},
		"hardware":                 "arch=amd64 cores=1 mem=1024M root-disk=8192M",
		"controller-member-status": "adding-vote",
	}
	machine1 = M{
		"juju-status": M{
			"current": "started",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"dns-name":     "10.0.1.1",
		"ip-addresses": []string{"10.0.1.1"},
		"instance-id":  "controller-1",
		"machine-status": M{
			"current": "pending",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"modification-status": M{
			"current": "idle",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"series": "quantal",
		"network-interfaces": M{
			"eth0": M{
				"ip-addresses": []string{"10.0.1.1"},
				"mac-address":  "aa:bb:cc:dd:ee:ff",
				"is-up":        true,
			},
		},
		"hardware": "arch=amd64 cores=1 mem=1024M root-disk=8192M",
	}
	machine1WithLXDProfile = M{
		"juju-status": M{
			"current": "started",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"dns-name":     "10.0.1.1",
		"ip-addresses": []string{"10.0.1.1"},
		"instance-id":  "controller-1",
		"machine-status": M{
			"current": "pending",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"modification-status": M{
			"current": "idle",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"series": "quantal",
		"network-interfaces": M{
			"eth0": M{
				"ip-addresses": []string{"10.0.1.1"},
				"mac-address":  "aa:bb:cc:dd:ee:ff",
				"is-up":        true,
			},
		},
		"hardware": "arch=amd64 cores=1 mem=1024M root-disk=8192M",
		"lxd-profiles": M{
			"juju-controller-lxd-profile-1": M{
				"config": M{
					"environment.http_proxy": "",
					"linux.kernel_modules":   "openvswitch,nbd,ip_tables,ip6_tables",
					"security.nesting":       "true",
					"security.privileged":    "true",
				},
				"description": "lxd profile for testing, will pass validation",
				"devices": M{
					"bdisk": M{
						"source": "/dev/loop0",
						"type":   "unix-block",
					},
					"gpu": M{
						"type": "gpu",
					},
					"sony": M{
						"productid": "51da",
						"type":      "usb",
						"vendorid":  "0fce",
					},
					"tun": M{
						"path": "/dev/net/tun",
						"type": "unix-char",
					},
				},
			},
		},
	}
	machine2 = M{
		"juju-status": M{
			"current": "started",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"dns-name":     "10.0.2.1",
		"ip-addresses": []string{"10.0.2.1"},
		"instance-id":  "controller-2",
		"machine-status": M{
			"current": "pending",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"modification-status": M{
			"current": "idle",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"series": "quantal",
		"network-interfaces": M{
			"eth0": M{
				"ip-addresses": []string{"10.0.2.1"},
				"mac-address":  "aa:bb:cc:dd:ee:ff",
				"is-up":        true,
			},
		},
		"hardware": "arch=amd64 cores=1 mem=1024M root-disk=8192M",
	}
	machine3 = M{
		"juju-status": M{
			"current": "started",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"dns-name":     "10.0.3.1",
		"ip-addresses": []string{"10.0.3.1"},
		"instance-id":  "controller-3",
		"machine-status": M{
			"current": "started",
			"message": "I am number three",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"modification-status": M{
			"current": "idle",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"series": "quantal",
		"network-interfaces": M{
			"eth0": M{
				"ip-addresses": []string{"10.0.3.1"},
				"mac-address":  "aa:bb:cc:dd:ee:ff",
				"is-up":        true,
			},
		},
		"hardware": "arch=amd64 cores=1 mem=1024M root-disk=8192M",
	}
	machine4 = M{
		"juju-status": M{
			"current": "started",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"dns-name":     "10.0.4.1",
		"ip-addresses": []string{"10.0.4.1"},
		"instance-id":  "controller-4",
		"machine-status": M{
			"current": "pending",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"modification-status": M{
			"current": "idle",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"series": "quantal",
		"network-interfaces": M{
			"eth0": M{
				"ip-addresses": []string{"10.0.4.1"},
				"mac-address":  "aa:bb:cc:dd:ee:ff",
				"is-up":        true,
			},
		},
		"hardware": "arch=amd64 cores=1 mem=1024M root-disk=8192M",
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
						"dns-name":     "10.0.3.1",
						"ip-addresses": []string{"10.0.3.1"},
						"instance-id":  "controller-3",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"modification-status": M{
							"current": "idle",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series": "quantal",
						"network-interfaces": M{
							"eth0": M{
								"ip-addresses": []string{"10.0.3.1"},
								"mac-address":  "aa:bb:cc:dd:ee:ff",
								"is-up":        true,
							},
						},
					},
				},
				"dns-name":     "10.0.2.1",
				"ip-addresses": []string{"10.0.2.1"},
				"instance-id":  "controller-2",
				"machine-status": M{
					"current": "pending",
					"since":   "01 Apr 15 01:23+10:00",
				},
				"modification-status": M{
					"current": "idle",
					"since":   "01 Apr 15 01:23+10:00",
				},
				"series": "quantal",
				"network-interfaces": M{
					"eth0": M{
						"ip-addresses": []string{"10.0.2.1"},
						"mac-address":  "aa:bb:cc:dd:ee:ff",
						"is-up":        true,
					},
				},
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
				"modification-status": M{
					"current": "idle",
					"since":   "01 Apr 15 01:23+10:00",
				},
				"series": "quantal",
			},
		},
		"dns-name":     "10.0.1.1",
		"ip-addresses": []string{"10.0.1.1"},
		"instance-id":  "controller-1",
		"machine-status": M{
			"current": "pending",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"modification-status": M{
			"current": "idle",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"series": "quantal",
		"network-interfaces": M{
			"eth0": M{
				"ip-addresses": []string{"10.0.1.1"},
				"mac-address":  "aa:bb:cc:dd:ee:ff",
				"is-up":        true,
			},
		},
		"hardware": "arch=amd64 cores=1 mem=1024M root-disk=8192M",
	}
	unexposedApplication = dummyCharm(M{
		"application-status": M{
			"current": "waiting",
			"message": "waiting for machine",
			"since":   "01 Apr 15 01:23+10:00",
		},
	})
	exposedApplication = dummyCharm(M{
		"application-status": M{
			"current": "waiting",
			"message": "waiting for machine",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"exposed": true,
	})
	loggingCharm = M{
		"charm":        "cs:quantal/logging-1",
		"charm-origin": "jujucharms",
		"charm-name":   "logging",
		"charm-rev":    1,
		"series":       "quantal",
		"os":           "ubuntu",
		"exposed":      true,
		"application-status": M{
			"current": "error",
			"message": "somehow lost in all those logs",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"relations": M{
			"logging-directory": L{"wordpress"},
			"info":              L{"mysql"},
		},
		"endpoint-bindings": M{
			"info":              "",
			"logging-client":    "",
			"logging-directory": "",
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

var machineCons = constraints.MustParse("cores=2 mem=8G root-disk=8G")

var statusTests = []testCase{
	// Status tests
	test( // 0
		"bootstrap and starting a single instance",

		addMachine{machineId: "0", job: state.JobManageModel},
		expect{
			what: "simulate juju bootstrap by adding machine/0 to the state",
			output: M{
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
						"modification-status": M{
							"current": "idle",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series":                   "quantal",
						"controller-member-status": "adding-vote",
					},
				},
				"applications": M{},
				"storage":      M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},

		startAliveMachine{"0", ""},
		setAddresses{"0", []network.Address{
			network.NewScopedAddress("10.0.0.1", network.ScopePublic),
			network.NewAddress("10.0.0.2"),
		}},
		expect{
			what: "simulate the PA starting an instance in response to the state change",
			output: M{
				"model": model,
				"machines": M{
					"0": M{
						"juju-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"dns-name":     "10.0.0.1",
						"ip-addresses": []string{"10.0.0.1", "10.0.0.2"},
						"instance-id":  "controller-0",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"modification-status": M{
							"current": "idle",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series": "quantal",
						"network-interfaces": M{
							"eth0": M{
								"ip-addresses": []string{"10.0.0.1"},
								"mac-address":  "aa:bb:cc:dd:ee:ff",
								"is-up":        true,
							},
							"eth1": M{
								"ip-addresses": []string{"10.0.0.2"},
								"mac-address":  "aa:bb:cc:dd:ee:ff",
								"is-up":        true,
							},
						},
						"hardware":                 "arch=amd64 cores=1 mem=1024M root-disk=8192M",
						"controller-member-status": "adding-vote",
					},
				},
				"applications": M{},
				"storage":      M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},

		setMachineStatus{"0", status.Started, ""},
		expect{
			what: "simulate the MA started and set the machine status",
			output: M{
				"model": model,
				"machines": M{
					"0": M{
						"juju-status": M{
							"current": "started",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"dns-name":     "10.0.0.1",
						"ip-addresses": []string{"10.0.0.1", "10.0.0.2"},
						"instance-id":  "controller-0",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"modification-status": M{
							"current": "idle",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series": "quantal",
						"network-interfaces": M{
							"eth0": M{
								"ip-addresses": []string{"10.0.0.1"},
								"mac-address":  "aa:bb:cc:dd:ee:ff",
								"is-up":        true,
							},
							"eth1": M{
								"ip-addresses": []string{"10.0.0.2"},
								"mac-address":  "aa:bb:cc:dd:ee:ff",
								"is-up":        true,
							},
						},
						"hardware":                 "arch=amd64 cores=1 mem=1024M root-disk=8192M",
						"controller-member-status": "adding-vote",
					},
				},
				"applications": M{},
				"storage":      M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},

		setTools{"0", version.MustParseBinary("1.2.3-trusty-ppc")},
		expect{
			what: "simulate the MA setting the version",
			output: M{
				"model": model,
				"machines": M{
					"0": M{
						"dns-name":     "10.0.0.1",
						"ip-addresses": []string{"10.0.0.1", "10.0.0.2"},
						"instance-id":  "controller-0",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"modification-status": M{
							"current": "idle",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"juju-status": M{
							"current": "started",
							"since":   "01 Apr 15 01:23+10:00",
							"version": "1.2.3",
						},
						"series": "quantal",
						"network-interfaces": M{
							"eth0": M{
								"ip-addresses": []string{"10.0.0.1"},
								"mac-address":  "aa:bb:cc:dd:ee:ff",
								"is-up":        true,
							},
							"eth1": M{
								"ip-addresses": []string{"10.0.0.2"},
								"mac-address":  "aa:bb:cc:dd:ee:ff",
								"is-up":        true,
							},
						},
						"hardware":                 "arch=amd64 cores=1 mem=1024M root-disk=8192M",
						"controller-member-status": "adding-vote",
					},
				},
				"applications": M{},
				"storage":      M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
	),
	test( // 1
		"instance with different hardware characteristics",
		addMachine{machineId: "0", cons: machineCons, job: state.JobManageModel},
		setAddresses{"0", []network.Address{
			network.NewScopedAddress("10.0.0.1", network.ScopePublic),
			network.NewAddress("10.0.0.2"),
		}},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		expect{
			what: "machine 0 has specific hardware characteristics",
			output: M{
				"model": model,
				"machines": M{
					"0": M{
						"juju-status": M{
							"current": "started",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"dns-name":     "10.0.0.1",
						"ip-addresses": []string{"10.0.0.1", "10.0.0.2"},
						"instance-id":  "controller-0",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"modification-status": M{
							"current": "idle",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series": "quantal",
						"network-interfaces": M{
							"eth0": M{
								"ip-addresses": []string{"10.0.0.1"},
								"mac-address":  "aa:bb:cc:dd:ee:ff",
								"is-up":        true,
							},
							"eth1": M{
								"ip-addresses": []string{"10.0.0.2"},
								"mac-address":  "aa:bb:cc:dd:ee:ff",
								"is-up":        true,
							},
						},
						"constraints":              "cores=2 mem=8192M root-disk=8192M",
						"hardware":                 "arch=amd64 cores=2 mem=8192M root-disk=8192M",
						"controller-member-status": "adding-vote",
					},
				},
				"applications": M{},
				"storage":      M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
	),
	test( // 2
		"instance without addresses",
		addMachine{machineId: "0", cons: machineCons, job: state.JobManageModel},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		expect{
			what: "machine 0 has no dns-name",
			output: M{
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
						"modification-status": M{
							"current": "idle",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series":                   "quantal",
						"constraints":              "cores=2 mem=8192M root-disk=8192M",
						"hardware":                 "arch=amd64 cores=2 mem=8192M root-disk=8192M",
						"controller-member-status": "adding-vote",
					},
				},
				"applications": M{},
				"storage":      M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
	),
	test( // 3
		"test pending and missing machines",
		addMachine{machineId: "0", job: state.JobManageModel},
		expect{
			what: "machine 0 reports pending",
			output: M{
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
						"modification-status": M{
							"current": "idle",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series":                   "quantal",
						"controller-member-status": "adding-vote",
					},
				},
				"applications": M{},
				"storage":      M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
		startMissingMachine{"0"},
		expect{
			what: "machine 0 reports missing",
			output: M{
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
						"modification-status": M{
							"current": "idle",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series":                   "quantal",
						"hardware":                 "arch=amd64 cores=1 mem=1024M root-disk=8192M",
						"controller-member-status": "adding-vote",
					},
				},
				"applications": M{},
				"storage":      M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
	),
	test( // 4
		"add two applications and expose one, then add 2 more machines and some units",
		// step 0
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addCharm{"dummy"},
		addApplication{name: "dummy-application", charm: "dummy"},
		addApplication{name: "exposed-application", charm: "dummy"},
		expect{
			what: "no applications exposed yet",
			output: M{
				"model": model,
				"machines": M{
					"0": machine0,
				},
				"applications": M{
					"dummy-application":   unexposedApplication,
					"exposed-application": unexposedApplication,
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},

		// step 8
		setApplicationExposed{"exposed-application", true},
		expect{
			what: "one exposed application",
			output: M{
				"model": model,
				"machines": M{
					"0": machine0,
				},
				"applications": M{
					"dummy-application":   unexposedApplication,
					"exposed-application": exposedApplication,
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},

		// step 10
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", network.NewAddresses("10.0.2.1")},
		startAliveMachine{"2", ""},
		setMachineStatus{"2", status.Started, ""},
		expect{
			what: "two more machines added",
			output: M{
				"model": model,
				"machines": M{
					"0": machine0,
					"1": machine1,
					"2": machine2,
				},
				"applications": M{
					"dummy-application":   unexposedApplication,
					"exposed-application": exposedApplication,
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},

		// step 19
		addAliveUnit{"dummy-application", "1"},
		addAliveUnit{"exposed-application", "2"},
		setAgentStatus{"exposed-application/0", status.Error, "You Require More Vespene Gas", nil},
		// Open multiple ports with different protocols,
		// ensure they're sorted on protocol, then number.
		openUnitPort{"exposed-application/0", "udp", 10},
		openUnitPort{"exposed-application/0", "udp", 2},
		openUnitPort{"exposed-application/0", "tcp", 3},
		openUnitPort{"exposed-application/0", "tcp", 2},
		// Simulate some status with no info, while the agent is down.
		// Status used to be down, we no longer support said state.
		// now is one of: pending, started, error.
		setUnitStatus{"dummy-application/0", status.Terminated, "", nil},
		setAgentStatus{"dummy-application/0", status.Idle, "", nil},

		expect{
			what: "add two units, one alive (in error state), one started",
			output: M{
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
								"public-address": "10.0.2.1",
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
								"public-address": "10.0.1.1",
							},
						},
					}),
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},

		// step 29
		addMachine{machineId: "3", job: state.JobHostUnits},
		startMachine{"3"},
		// Simulate some status with info, while the agent is down.
		setAddresses{"3", network.NewAddresses("10.0.3.1")},
		setMachineStatus{"3", status.Stopped, "Really?"},
		addMachine{machineId: "4", job: state.JobHostUnits},
		setAddresses{"4", network.NewAddresses("10.0.4.1")},
		startAliveMachine{"4", ""},
		setMachineStatus{"4", status.Error, "Beware the red toys"},
		ensureDyingUnit{"dummy-application/0"},
		addMachine{machineId: "5", job: state.JobHostUnits},
		ensureDeadMachine{"5"},
		expect{
			what: "add three more machine, one with a dead agent, one in error state and one dead itself; also one dying unit",
			output: M{
				"model": model,
				"machines": M{
					"0": machine0,
					"1": machine1,
					"2": machine2,
					"3": M{
						"dns-name":     "10.0.3.1",
						"ip-addresses": []string{"10.0.3.1"},
						"instance-id":  "controller-3",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"modification-status": M{
							"current": "idle",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"juju-status": M{
							"current": "stopped",
							"message": "Really?",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series": "quantal",
						"network-interfaces": M{
							"eth0": M{
								"ip-addresses": []string{"10.0.3.1"},
								"mac-address":  "aa:bb:cc:dd:ee:ff",
								"is-up":        true,
							},
						},
						"hardware": "arch=amd64 cores=1 mem=1024M root-disk=8192M",
					},
					"4": M{
						"dns-name":     "10.0.4.1",
						"ip-addresses": []string{"10.0.4.1"},
						"instance-id":  "controller-4",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"modification-status": M{
							"current": "idle",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"juju-status": M{
							"current": "error",
							"message": "Beware the red toys",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series": "quantal",
						"network-interfaces": M{
							"eth0": M{
								"ip-addresses": []string{"10.0.4.1"},
								"mac-address":  "aa:bb:cc:dd:ee:ff",
								"is-up":        true,
							},
						},
						"hardware": "arch=amd64 cores=1 mem=1024M root-disk=8192M",
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
						"modification-status": M{
							"current": "idle",
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
								"public-address": "10.0.2.1",
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
								"public-address": "10.0.1.1",
							},
						},
					}),
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},

		// step 41
		scopedExpect{
			what:  "scope status on dummy-application/0 unit",
			scope: []string{"dummy-application/0"},
			output: M{
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
								"public-address": "10.0.1.1",
							},
						},
					}),
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
		scopedExpect{
			what:  "scope status on exposed-application application",
			scope: []string{"exposed-application"},
			output: M{
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
								"public-address": "10.0.2.1",
							},
						},
					}),
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
		scopedExpect{
			what:  "scope status on application pattern",
			scope: []string{"d*-application"},
			output: M{
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
								"public-address": "10.0.1.1",
							},
						},
					}),
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
		scopedExpect{
			what:  "scope status on unit pattern",
			scope: []string{"e*posed-application/*"},
			output: M{
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
								"public-address": "10.0.2.1",
							},
						},
					}),
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
		scopedExpect{
			what:  "scope status on combination of application and unit patterns",
			scope: []string{"exposed-application", "dummy-application", "e*posed-application/*", "dummy-application/*"},
			output: M{
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
								"public-address": "10.0.1.1",
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
								"public-address": "10.0.2.1",
							},
						},
					}),
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
	),
	test( // 5
		"a unit with a hook relation error",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},

		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},

		addCharm{"wordpress"},
		addApplication{name: "wordpress", charm: "wordpress"},
		addAliveUnit{"wordpress", "1"},

		addCharm{"mysql"},
		addApplication{name: "mysql", charm: "mysql"},
		addAliveUnit{"mysql", "1"},

		relateApplications{"wordpress", "mysql", ""},

		setAgentStatus{"wordpress/0", status.Error,
			"hook failed: some-relation-changed",
			map[string]interface{}{"relation-id": 0}},

		expect{
			what: "a unit with a hook relation error",
			output: M{
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
								"public-address": "10.0.1.1",
							},
						},
						"endpoint-bindings": M{
							"logging-dir":     "",
							"monitoring-port": "",
							"url":             "",
							"admin-api":       "",
							"cache":           "",
							"db":              "",
							"db-client":       "",
							"foo-bar":         "",
						},
					}),
					"mysql": mysqlCharm(M{
						"relations": M{
							"server": L{"wordpress"},
						},
						"application-status": M{
							"current": "waiting",
							"message": "waiting for machine",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"mysql/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "waiting",
									"message": "waiting for machine",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "allocating",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "10.0.1.1",
							},
						},
						"endpoint-bindings": M{
							"server":         "",
							"server-admin":   "",
							"metrics-client": "",
						},
					}),
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
	),
	test( // 6
		"a unit with a hook relation error when the agent is down",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},

		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},

		addCharm{"wordpress"},
		addApplication{name: "wordpress", charm: "wordpress"},
		addAliveUnit{"wordpress", "1"},

		addCharm{"mysql"},
		addApplication{name: "mysql", charm: "mysql"},
		addAliveUnit{"mysql", "1"},

		relateApplications{"wordpress", "mysql", ""},

		setAgentStatus{"wordpress/0", status.Error,
			"hook failed: some-relation-changed",
			map[string]interface{}{"relation-id": 0}},

		expect{
			what: "a unit with a hook relation error when the agent is down",
			output: M{
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
								"public-address": "10.0.1.1",
							},
						},
						"endpoint-bindings": M{
							"admin-api":       "",
							"cache":           "",
							"db":              "",
							"db-client":       "",
							"foo-bar":         "",
							"logging-dir":     "",
							"monitoring-port": "",
							"url":             "",
						},
					}),
					"mysql": mysqlCharm(M{
						"relations": M{
							"server": L{"wordpress"},
						},
						"application-status": M{
							"current": "waiting",
							"message": "waiting for machine",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"mysql/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "waiting",
									"message": "waiting for machine",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "allocating",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "10.0.1.1",
							},
						},
						"endpoint-bindings": M{
							"server":         "",
							"server-admin":   "",
							"metrics-client": "",
						},
					}),
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
	),
	test( // 7
		"add a dying application",
		addCharm{"dummy"},
		addApplication{name: "dummy-application", charm: "dummy"},
		addMachine{machineId: "0", job: state.JobHostUnits},
		addAliveUnit{"dummy-application", "0"},
		ensureDyingApplication{"dummy-application"},
		expect{
			what: "application shows life==dying",
			output: M{
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
						"modification-status": M{
							"current": "idle",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series": "quantal",
					},
				},
				"applications": M{
					"dummy-application": dummyCharm(M{
						"life": "dying",
						"application-status": M{
							"current": "waiting",
							"message": "waiting for machine",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"dummy-application/0": M{
								"machine": "0",
								"workload-status": M{
									"current": "waiting",
									"message": "waiting for machine",
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
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
	),
	test( // 8
		"a unit where the agent is down shows as lost",
		addCharm{"dummy"},
		addApplication{name: "dummy-application", charm: "dummy"},
		addMachine{machineId: "0", job: state.JobHostUnits},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addUnit{"dummy-application", "0"},
		setAgentStatus{"dummy-application/0", status.Idle, "", nil},
		setUnitStatus{"dummy-application/0", status.Active, "", nil},
		expect{
			what: "unit shows that agent is lost",
			output: M{
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
						"modification-status": M{
							"current": "idle",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series":   "quantal",
						"hardware": "arch=amd64 cores=1 mem=1024M root-disk=8192M",
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
									"message": "agent lost, see 'juju show-status-log dummy-application/0'",
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
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
	),
	// Relation tests
	test( // 9
		"complex scenario with multiple related applications",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addCharm{"wordpress"},
		addCharm{"mysql"},
		addCharm{"varnish"},

		addApplication{name: "project", charm: "wordpress"},
		setApplicationExposed{"project", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addAliveUnit{"project", "1"},
		setAgentStatus{"project/0", status.Idle, "", nil},
		setUnitStatus{"project/0", status.Active, "", nil},

		addApplication{name: "mysql", charm: "mysql"},
		setApplicationExposed{"mysql", true},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", network.NewAddresses("10.0.2.1")},
		startAliveMachine{"2", ""},
		setMachineStatus{"2", status.Started, ""},
		addAliveUnit{"mysql", "2"},
		setAgentStatus{"mysql/0", status.Idle, "", nil},
		setUnitStatus{"mysql/0", status.Active, "", nil},

		addApplication{name: "varnish", charm: "varnish"},
		setApplicationExposed{"varnish", true},
		addMachine{machineId: "3", job: state.JobHostUnits},
		setAddresses{"3", network.NewAddresses("10.0.3.1")},
		startAliveMachine{"3", ""},
		setMachineStatus{"3", status.Started, ""},
		setMachineInstanceStatus{"3", status.Started, "I am number three"},
		addAliveUnit{"varnish", "3"},

		addApplication{name: "private", charm: "wordpress"},
		setApplicationExposed{"private", true},
		addMachine{machineId: "4", job: state.JobHostUnits},
		setAddresses{"4", network.NewAddresses("10.0.4.1")},
		startAliveMachine{"4", ""},
		setMachineStatus{"4", status.Started, ""},
		addAliveUnit{"private", "4"},

		relateApplications{"project", "mysql", ""},
		relateApplications{"project", "varnish", ""},
		relateApplications{"private", "mysql", ""},

		expect{
			what: "multiples applications with relations between some of them",
			output: M{
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
								"public-address": "10.0.1.1",
							},
						},
						"endpoint-bindings": M{
							"db":              "",
							"db-client":       "",
							"foo-bar":         "",
							"logging-dir":     "",
							"monitoring-port": "",
							"url":             "",
							"admin-api":       "",
							"cache":           "",
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
								"public-address": "10.0.2.1",
							},
						},
						"endpoint-bindings": M{
							"server":         "",
							"server-admin":   "",
							"metrics-client": "",
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
							"current": "waiting",
							"message": "waiting for machine",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"varnish/0": M{
								"machine": "3",
								"workload-status": M{
									"current": "waiting",
									"message": "waiting for machine",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "allocating",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "10.0.3.1",
							},
						},
						"endpoint-bindings": M{
							"webcache": "",
						},
						"relations": M{
							"webcache": L{"project"},
						},
					},
					"private": wordpressCharm(M{
						"exposed": true,
						"application-status": M{
							"current": "waiting",
							"message": "waiting for machine",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"private/0": M{
								"machine": "4",
								"workload-status": M{
									"current": "waiting",
									"message": "waiting for machine",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "allocating",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "10.0.4.1",
							},
						},
						"endpoint-bindings": M{
							"logging-dir":     "",
							"monitoring-port": "",
							"url":             "",
							"admin-api":       "",
							"cache":           "",
							"db":              "",
							"db-client":       "",
							"foo-bar":         "",
						},
						"relations": M{
							"db": L{"mysql"},
						},
					}),
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
	),
	test( // 10
		"simple peer scenario with leader",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addCharm{"riak"},
		addCharm{"wordpress"},

		addApplication{name: "riak", charm: "riak"},
		setApplicationExposed{"riak", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addAliveUnit{"riak", "1"},
		setAgentStatus{"riak/0", status.Idle, "", nil},
		setUnitStatus{"riak/0", status.Active, "", nil},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", network.NewAddresses("10.0.2.1")},
		startAliveMachine{"2", ""},
		setMachineStatus{"2", status.Started, ""},
		addAliveUnit{"riak", "2"},
		setAgentStatus{"riak/1", status.Idle, "", nil},
		setUnitStatus{"riak/1", status.Active, "", nil},
		addMachine{machineId: "3", job: state.JobHostUnits},
		setAddresses{"3", network.NewAddresses("10.0.3.1")},
		startAliveMachine{"3", ""},
		setMachineStatus{"3", status.Started, ""},
		setMachineInstanceStatus{"3", status.Started, "I am number three"},
		addAliveUnit{"riak", "3"},
		setAgentStatus{"riak/2", status.Idle, "", nil},
		setUnitStatus{"riak/2", status.Active, "", nil},
		setUnitAsLeader{"riak/1"},

		expect{
			what: "multiples related peer units",
			output: M{
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
								"public-address": "10.0.1.1",
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
								"public-address": "10.0.2.1",
								"leader":         true,
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
								"public-address": "10.0.3.1",
							},
						},
						"endpoint-bindings": M{
							"admin":    "",
							"endpoint": "",
							"ring":     "",
						},
						"relations": M{
							"ring": L{"riak"},
						},
					},
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
	),
	// Subordinate tests
	test( // 11
		"one application with one subordinate application and leader",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addCharm{"wordpress"},
		addCharm{"mysql"},
		addCharm{"logging"},

		addApplication{name: "wordpress", charm: "wordpress"},
		setApplicationExposed{"wordpress", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addAliveUnit{"wordpress", "1"},
		setAgentStatus{"wordpress/0", status.Idle, "", nil},
		setUnitStatus{"wordpress/0", status.Active, "", nil},

		addApplication{name: "mysql", charm: "mysql"},
		setApplicationExposed{"mysql", true},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", network.NewAddresses("10.0.2.1")},
		startAliveMachine{"2", ""},
		setMachineStatus{"2", status.Started, ""},
		addAliveUnit{"mysql", "2"},
		setAgentStatus{"mysql/0", status.Idle, "", nil},
		setUnitStatus{"mysql/0", status.Active, "", nil},

		addApplication{name: "logging", charm: "logging"},
		setApplicationExposed{"logging", true},

		relateApplications{"wordpress", "mysql", ""},
		relateApplications{"wordpress", "logging", ""},
		relateApplications{"mysql", "logging", ""},

		addSubordinate{"wordpress/0", "logging"},
		addSubordinate{"mysql/0", "logging"},

		setUnitsAlive{"logging"},
		setAgentStatus{"logging/0", status.Idle, "", nil},
		setUnitStatus{"logging/0", status.Active, "", nil},
		setAgentStatus{"logging/1", status.Error, "somehow lost in all those logs", nil},

		setUnitAsLeader{"mysql/0"},
		setUnitAsLeader{"logging/1"},
		setUnitAsLeader{"wordpress/0"},

		expect{
			what: "multiples related peer units",
			output: M{
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
										"public-address": "10.0.1.1",
									},
								},
								"public-address": "10.0.1.1",
								"leader":         true,
							},
						},
						"endpoint-bindings": M{
							"monitoring-port": "",
							"url":             "",
							"admin-api":       "",
							"cache":           "",
							"db":              "",
							"db-client":       "",
							"foo-bar":         "",
							"logging-dir":     "",
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
										"public-address": "10.0.2.1",
										"leader":         true,
									},
								},
								"public-address": "10.0.2.1",
								"leader":         true,
							},
						},
						"endpoint-bindings": M{
							"server":         "",
							"server-admin":   "",
							"metrics-client": "",
						},
						"relations": M{
							"server":    L{"wordpress"},
							"juju-info": L{"logging"},
						},
					}),
					"logging": loggingCharm,
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},

		// scoped on 'logging'
		scopedExpect{
			what:  "subordinates scoped on logging",
			scope: []string{"logging"},
			output: M{
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
										"public-address": "10.0.1.1",
									},
								},
								"public-address": "10.0.1.1",
								"leader":         true,
							},
						},
						"endpoint-bindings": M{
							"monitoring-port": "",
							"url":             "",
							"admin-api":       "",
							"cache":           "",
							"db":              "",
							"db-client":       "",
							"foo-bar":         "",
							"logging-dir":     "",
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
										"public-address": "10.0.2.1",
										"leader":         true,
									},
								},
								"public-address": "10.0.2.1",
								"leader":         true,
							},
						},
						"endpoint-bindings": M{
							"server":         "",
							"server-admin":   "",
							"metrics-client": "",
						},
						"relations": M{
							"server":    L{"wordpress"},
							"juju-info": L{"logging"},
						},
					}),
					"logging": loggingCharm,
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},

		// scoped on wordpress/0
		scopedExpect{
			what:  "subordinates scoped on wordpress",
			scope: []string{"wordpress/0"},
			output: M{
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
										"public-address": "10.0.1.1",
									},
								},
								"public-address": "10.0.1.1",
								"leader":         true,
							},
						},
						"endpoint-bindings": M{
							"admin-api":       "",
							"cache":           "",
							"db":              "",
							"db-client":       "",
							"foo-bar":         "",
							"logging-dir":     "",
							"monitoring-port": "",
							"url":             "",
						},
						"relations": M{
							"db":          L{"mysql"},
							"logging-dir": L{"logging"},
						},
					}),
					"logging": loggingCharm,
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
	),
	test( // 12
		"machines with containers",
		// step 0
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addCharm{"mysql"},
		addApplication{name: "mysql", charm: "mysql"},
		setApplicationExposed{"mysql", true},

		// step 7
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addAliveUnit{"mysql", "1"},
		setAgentStatus{"mysql/0", status.Idle, "", nil},
		setUnitStatus{"mysql/0", status.Active, "", nil},

		// step 14: A container on machine 1.
		addContainer{"1", "1/lxd/0", state.JobHostUnits},
		setAddresses{"1/lxd/0", network.NewAddresses("10.0.2.1")},
		startAliveMachine{"1/lxd/0", ""},
		setMachineStatus{"1/lxd/0", status.Started, ""},
		addAliveUnit{"mysql", "1/lxd/0"},
		setAgentStatus{"mysql/1", status.Idle, "", nil},
		setUnitStatus{"mysql/1", status.Active, "", nil},
		addContainer{"1", "1/lxd/1", state.JobHostUnits},

		// step 22: A nested container.
		addContainer{"1/lxd/0", "1/lxd/0/lxd/0", state.JobHostUnits},
		setAddresses{"1/lxd/0/lxd/0", network.NewAddresses("10.0.3.1")},
		startAliveMachine{"1/lxd/0/lxd/0", ""},
		setMachineStatus{"1/lxd/0/lxd/0", status.Started, ""},

		expect{
			what: "machines with nested containers",
			output: M{
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
								"public-address": "10.0.1.1",
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
								"public-address": "10.0.2.1",
							},
						},
						"endpoint-bindings": M{
							"server":         "",
							"server-admin":   "",
							"metrics-client": "",
						},
					}),
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},

		// step 27: once again, with a scope on mysql/1
		scopedExpect{
			what:  "machines with nested containers 2",
			scope: []string{"mysql/1"},
			output: M{
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
								"dns-name":     "10.0.2.1",
								"ip-addresses": []string{"10.0.2.1"},
								"instance-id":  "controller-2",
								"machine-status": M{
									"current": "pending",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"modification-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"series": "quantal",
								"network-interfaces": M{
									"eth0": M{
										"ip-addresses": []string{"10.0.2.1"},
										"mac-address":  "aa:bb:cc:dd:ee:ff",
										"is-up":        true,
									},
								},
							},
						},
						"dns-name":     "10.0.1.1",
						"ip-addresses": []string{"10.0.1.1"},
						"instance-id":  "controller-1",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"modification-status": M{
							"current": "idle",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series": "quantal",
						"network-interfaces": M{
							"eth0": M{
								"ip-addresses": []string{"10.0.1.1"},
								"mac-address":  "aa:bb:cc:dd:ee:ff",
								"is-up":        true,
							},
						},
						"hardware": "arch=amd64 cores=1 mem=1024M root-disk=8192M",
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
								"public-address": "10.0.2.1",
							},
						},
						"endpoint-bindings": M{
							"server":         "",
							"server-admin":   "",
							"metrics-client": "",
						},
					}),
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
	),
	test( // 13
		"application with out of date charm",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addCharm{"mysql"},
		addApplication{name: "mysql", charm: "mysql"},
		setApplicationExposed{"mysql", true},
		addCharmPlaceholder{"mysql", 23},
		addAliveUnit{"mysql", "1"},

		expect{
			what: "applications and units with correct charm status",
			output: M{
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
							"current": "waiting",
							"message": "waiting for machine",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"mysql/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "waiting",
									"message": "waiting for machine",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "allocating",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "10.0.1.1",
							},
						},
						"endpoint-bindings": M{
							"server":         "",
							"server-admin":   "",
							"metrics-client": "",
						},
					}),
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
	),
	test( // 14
		"unit with out of date charm",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addCharm{"mysql"},
		addApplication{name: "mysql", charm: "mysql"},
		setApplicationExposed{"mysql", true},
		addAliveUnit{"mysql", "1"},
		setUnitCharmURL{"mysql/0", "cs:quantal/mysql-1"},
		addCharmWithRevision{addCharm{"mysql"}, "local", 1},
		setApplicationCharm{"mysql", "local:quantal/mysql-1"},

		expect{
			what: "applications and units with correct charm status",
			output: M{
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
								"public-address": "10.0.1.1",
							},
						},
						"endpoint-bindings": M{
							"server":         "",
							"server-admin":   "",
							"metrics-client": "",
						},
					}),
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
	),
	test( // 15
		"application and unit with out of date charms",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addCharm{"mysql"},
		addApplication{name: "mysql", charm: "mysql"},
		setApplicationExposed{"mysql", true},
		addAliveUnit{"mysql", "1"},
		setUnitCharmURL{"mysql/0", "cs:quantal/mysql-1"},
		addCharmWithRevision{addCharm{"mysql"}, "cs", 2},
		setApplicationCharm{"mysql", "cs:quantal/mysql-2"},
		addCharmPlaceholder{"mysql", 23},

		expect{
			what: "applications and units with correct charm status",
			output: M{
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
								"public-address": "10.0.1.1",
							},
						},
						"endpoint-bindings": M{
							"server":         "",
							"server-admin":   "",
							"metrics-client": "",
						},
					}),
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
	),
	test( // 16
		"application with local charm not shown as out of date",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addCharm{"mysql"},
		addApplication{name: "mysql", charm: "mysql"},
		setApplicationExposed{"mysql", true},
		addAliveUnit{"mysql", "1"},
		setUnitCharmURL{"mysql/0", "cs:quantal/mysql-1"},
		addCharmWithRevision{addCharm{"mysql"}, "local", 1},
		setApplicationCharm{"mysql", "local:quantal/mysql-1"},
		addCharmPlaceholder{"mysql", 23},

		expect{
			what: "applications and units with correct charm status",
			output: M{
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
								"public-address": "10.0.1.1",
							},
						},
						"endpoint-bindings": M{
							"server":         "",
							"server-admin":   "",
							"metrics-client": "",
						},
					}),
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
	),
	test( // 17
		"deploy two applications; set meter statuses on one",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},

		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},

		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", network.NewAddresses("10.0.2.1")},
		startAliveMachine{"2", ""},
		setMachineStatus{"2", status.Started, ""},

		addMachine{machineId: "3", job: state.JobHostUnits},
		setAddresses{"3", network.NewAddresses("10.0.3.1")},
		startAliveMachine{"3", ""},
		setMachineStatus{"3", status.Started, ""},
		setMachineInstanceStatus{"3", status.Started, "I am number three"},

		addMachine{machineId: "4", job: state.JobHostUnits},
		setAddresses{"4", network.NewAddresses("10.0.4.1")},
		startAliveMachine{"4", ""},
		setMachineStatus{"4", status.Started, ""},

		addCharm{"mysql"},
		addApplication{name: "mysql", charm: "mysql"},
		setApplicationExposed{"mysql", true},

		addCharm{"metered"},
		addApplication{name: "applicationwithmeterstatus", charm: "metered"},

		addAliveUnit{"mysql", "1"},
		addAliveUnit{"applicationwithmeterstatus", "2"},
		addAliveUnit{"applicationwithmeterstatus", "3"},
		addAliveUnit{"applicationwithmeterstatus", "4"},

		setApplicationExposed{"mysql", true},

		setAgentStatus{"mysql/0", status.Idle, "", nil},
		setUnitStatus{"mysql/0", status.Active, "", nil},
		setAgentStatus{"applicationwithmeterstatus/0", status.Idle, "", nil},
		setUnitStatus{"applicationwithmeterstatus/0", status.Active, "", nil},
		setAgentStatus{"applicationwithmeterstatus/1", status.Idle, "", nil},
		setUnitStatus{"applicationwithmeterstatus/1", status.Active, "", nil},
		setAgentStatus{"applicationwithmeterstatus/2", status.Idle, "", nil},
		setUnitStatus{"applicationwithmeterstatus/2", status.Active, "", nil},

		setUnitMeterStatus{"applicationwithmeterstatus/1", "GREEN", "test green status"},
		setUnitMeterStatus{"applicationwithmeterstatus/2", "RED", "test red status"},

		expect{
			what: "simulate just the two applications and a bootstrap node",
			output: M{
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
								"public-address": "10.0.1.1",
							},
						},
						"endpoint-bindings": M{
							"server":         "",
							"server-admin":   "",
							"metrics-client": "",
						},
					}),

					"applicationwithmeterstatus": meteredCharm(M{
						"application-status": M{
							"current": "active",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"applicationwithmeterstatus/0": M{
								"machine": "2",
								"workload-status": M{
									"current": "active",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "10.0.2.1",
							},
							"applicationwithmeterstatus/1": M{
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
								"public-address": "10.0.3.1",
							},
							"applicationwithmeterstatus/2": M{
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
								"public-address": "10.0.4.1",
							},
						},
					}),
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
	),
	test( // 18
		"upgrade available",
		setToolsUpgradeAvailable{},
		expect{
			what: "upgrade availability should be shown in model-status",
			output: M{
				"model": M{
					"name":              "controller",
					"type":              "iaas",
					"controller":        "kontroll",
					"cloud":             "dummy",
					"region":            "dummy-region",
					"version":           "1.2.3",
					"upgrade-available": "1.2.4",
					"model-status": M{
						"current": "available",
						"since":   "01 Apr 15 01:23+10:00",
					},
					"sla": "unsupported",
				},
				"machines":     M{},
				"applications": M{},
				"storage":      M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
			stderr: "Model \"controller\" is empty.\n",
		},
	),
	test( // 19
		"consistent workload version",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},

		addCharm{"mysql"},
		addApplication{name: "mysql", charm: "mysql"},

		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addAliveUnit{"mysql", "1"},
		setUnitWorkloadVersion{"mysql/0", "the best!"},

		expect{
			what: "application and unit with correct workload version",
			output: M{
				"model": model,
				"machines": M{
					"0": machine0,
					"1": machine1,
				},
				"applications": M{
					"mysql": mysqlCharm(M{
						"version": "the best!",
						"application-status": M{
							"current": "waiting",
							"message": "waiting for machine",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"mysql/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "waiting",
									"message": "waiting for machine",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "allocating",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "10.0.1.1",
							},
						},
						"endpoint-bindings": M{
							"server":         "",
							"server-admin":   "",
							"metrics-client": "",
						},
					}),
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
	),
	test( // 20
		"mixed workload version",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},

		addCharm{"mysql"},
		addApplication{name: "mysql", charm: "mysql"},

		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addAliveUnit{"mysql", "1"},
		setUnitWorkloadVersion{"mysql/0", "the best!"},

		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", network.NewAddresses("10.0.2.1")},
		startAliveMachine{"2", ""},
		setMachineStatus{"2", status.Started, ""},
		addAliveUnit{"mysql", "2"},
		setUnitWorkloadVersion{"mysql/1", "not as good"},

		expect{
			what: "application and unit with correct workload version",
			output: M{
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
							"current": "waiting",
							"message": "waiting for machine",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"mysql/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "waiting",
									"message": "waiting for machine",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "allocating",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "10.0.1.1",
							},
							"mysql/1": M{
								"machine": "2",
								"workload-status": M{
									"current": "waiting",
									"message": "waiting for machine",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "allocating",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "10.0.2.1",
							},
						},
						"endpoint-bindings": M{
							"server":         "",
							"server-admin":   "",
							"metrics-client": "",
						},
					}),
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
	),
	test( // 21
		"instance with localhost addresses",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", []network.Address{
			network.NewScopedAddress("10.0.0.1", network.ScopeCloudLocal),
			network.NewScopedAddress("127.0.0.1", network.ScopeMachineLocal),
			// TODO(macgreagoir) setAddresses step method needs to
			// set netmask correctly before we can test IPv6
			// loopback.
			// network.NewScopedAddress("::1", network.ScopeMachineLocal),
		}},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		expect{
			what: "machine 0 has localhost addresses that should not display",
			output: M{
				"model": model,
				"machines": M{
					"0": machine0,
				},
				"applications": M{},
				"storage":      M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
	),
	test( // 22
		"instance with IPv6 addresses",
		addMachine{machineId: "0", cons: machineCons, job: state.JobManageModel},
		setAddresses{"0", []network.Address{
			network.NewScopedAddress("2001:db8::1", network.ScopeCloudLocal),
			// TODO(macgreagoir) setAddresses step method needs to
			// set netmask correctly before we can test IPv6
			// loopback.
			// network.NewScopedAddress("::1", network.ScopeMachineLocal),
		}},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		expect{
			what: "machine 0 has an IPv6 address",
			output: M{
				"model": model,
				"machines": M{
					"0": M{
						"juju-status": M{
							"current": "started",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"dns-name":     "2001:db8::1",
						"ip-addresses": []string{"2001:db8::1"},
						"instance-id":  "controller-0",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"modification-status": M{
							"current": "idle",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series": "quantal",
						"network-interfaces": M{
							"eth0": M{
								"ip-addresses": []string{"2001:db8::1"},
								"mac-address":  "aa:bb:cc:dd:ee:ff",
								"is-up":        true,
							},
						},
						"constraints":              "cores=2 mem=8192M root-disk=8192M",
						"hardware":                 "arch=amd64 cores=2 mem=8192M root-disk=8192M",
						"controller-member-status": "adding-vote",
					},
				},
				"applications": M{},
				"storage":      M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
	),
	test( // 23
		"a remote application",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},

		addCharm{"wordpress"},
		addApplication{name: "wordpress", charm: "wordpress"},
		addAliveUnit{"wordpress", "1"},

		addCharm{"mysql"},
		addRemoteApplication{name: "hosted-mysql", url: "me/model.mysql", charm: "mysql", endpoints: []string{"server"}},
		relateApplications{"wordpress", "hosted-mysql", ""},

		expect{
			what: "a remote application",
			output: M{
				"model": model,
				"machines": M{
					"0": machine0,
					"1": machine1,
				},
				"application-endpoints": M{
					"hosted-mysql": M{
						"url": "me/model.mysql",
						"endpoints": M{
							"server": M{
								"interface": "mysql",
								"role":      "provider",
							},
						},
						"application-status": M{
							"current": "unknown",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"relations": M{
							"server": L{"wordpress"},
						},
					},
				},
				"applications": M{
					"wordpress": wordpressCharm(M{
						"application-status": M{
							"current": "waiting",
							"message": "waiting for machine",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"relations": M{
							"db": L{"hosted-mysql"},
						},
						"units": M{
							"wordpress/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "waiting",
									"message": "waiting for machine",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "allocating",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"public-address": "10.0.1.1",
							},
						},
						"endpoint-bindings": M{
							"monitoring-port": "",
							"url":             "",
							"admin-api":       "",
							"cache":           "",
							"db":              "",
							"db-client":       "",
							"foo-bar":         "",
							"logging-dir":     "",
						},
					}),
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
	),
	test( // 24
		"set meter status on the model",
		setSLA{"advanced"},
		setModelMeterStatus{"RED", "status message"},
		expect{
			what: "simulate just the two applications and a bootstrap node",
			output: M{
				"model": M{
					"name":       "controller",
					"type":       "iaas",
					"controller": "kontroll",
					"cloud":      "dummy",
					"region":     "dummy-region",
					"version":    "1.2.3",
					"model-status": M{
						"current": "available",
						"since":   "01 Apr 15 01:23+10:00",
					},
					"meter-status": M{
						"color":   "red",
						"message": "status message",
					},
					"sla": "advanced",
				},
				"machines":     M{},
				"applications": M{},
				"storage":      M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
			stderr: "Model \"controller\" is empty.\n",
		},
	),
	test( // 25
		"set sla on the model",
		setSLA{"advanced"},
		expect{
			what: "set sla on the model",
			output: M{
				"model": M{
					"name":       "controller",
					"type":       "iaas",
					"controller": "kontroll",
					"cloud":      "dummy",
					"region":     "dummy-region",
					"version":    "1.2.3",
					"model-status": M{
						"current": "available",
						"since":   "01 Apr 15 01:23+10:00",
					},
					"sla": "advanced",
				},
				"machines":     M{},
				"applications": M{},
				"storage":      M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
			stderr: "Model \"controller\" is empty.\n",
		},
	),
	test( //26
		"deploy application with endpoint bound to space",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},

		addSpace{"myspace1"},

		addCharm{"wordpress"},
		addApplication{name: "wordpress", charm: "wordpress", binding: map[string]string{"db-client": "", "logging-dir": "", "cache": "", "db": "myspace1", "monitoring-port": "", "url": "", "admin-api": "", "foo-bar": ""}},
		addAliveUnit{"wordpress", "1"},

		scopedExpect{
			output: M{
				"model": M{
					"region":  "dummy-region",
					"version": "1.2.3",
					"model-status": M{
						"current": "available",
						"since":   "01 Apr 15 01:23+10:00",
					},
					"type":       "iaas",
					"sla":        "unsupported",
					"name":       "controller",
					"controller": "kontroll",
					"cloud":      "dummy",
				},
				"machines": M{
					"1": M{
						"juju-status": M{
							"current": "started",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"dns-name":     "10.0.1.1",
						"ip-addresses": []string{"10.0.1.1"},
						"instance-id":  "controller-1",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"modification-status": M{
							"current": "idle",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"series": "quantal",
						"network-interfaces": M{
							"eth0": M{
								"ip-addresses": []string{"10.0.1.1"},
								"mac-address":  "aa:bb:cc:dd:ee:ff",
								"is-up":        bool(true),
							},
						},
						"hardware": "arch=amd64 cores=1 mem=1024M root-disk=8192M",
					},
					"0": M{
						"series": "quantal",
						"network-interfaces": M{
							"eth0": M{
								"ip-addresses": []string{"10.0.0.1"},
								"mac-address":  "aa:bb:cc:dd:ee:ff",
								"is-up":        bool(true),
							},
						},
						"controller-member-status": "adding-vote",
						"dns-name":                 "10.0.0.1",
						"ip-addresses":             []string{"10.0.0.1"},
						"instance-id":              "controller-0",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"modification-status": M{
							"current": "idle",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"juju-status": M{
							"current": "started",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"hardware": "arch=amd64 cores=1 mem=1024M root-disk=8192M",
					},
				},
				"applications": M{
					"wordpress": M{
						"series":     "quantal",
						"os":         "ubuntu",
						"charm-name": "wordpress",
						"exposed":    bool(false),
						"units": M{
							"wordpress/0": M{
								"public-address": "10.0.1.1",
								"workload-status": M{
									"current": "waiting",
									"message": "waiting for machine",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "allocating",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"machine": "1",
							},
						},
						"charm":        "cs:quantal/wordpress-3",
						"charm-origin": "jujucharms",
						"charm-rev":    int(3),
						"application-status": M{
							"current": "waiting",
							"message": "waiting for machine",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"endpoint-bindings": M{
							"cache":           "",
							"db":              "myspace1",
							"db-client":       "",
							"foo-bar":         "",
							"logging-dir":     "",
							"monitoring-port": "",
							"url":             "",
							"admin-api":       "",
						},
					},
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
		},
	),
	test( // 27
		"application with lxd profiles",
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		setCharmProfiles{"1", []string{"juju-controller-lxd-profile-1"}},
		addCharm{"lxd-profile"},
		addApplication{name: "lxd-profile", charm: "lxd-profile"},
		setApplicationExposed{"lxd-profile", true},
		addAliveUnit{"lxd-profile", "1"},
		setUnitCharmURL{"lxd-profile/0", "cs:quantal/lxd-profile-0"},
		addCharmWithRevision{addCharm{"lxd-profile"}, "local", 1},
		setApplicationCharm{"lxd-profile", "local:quantal/lxd-profile-1"},
		addCharmPlaceholder{"lxd-profile", 23},
		expect{
			what: "applications and units with correct lxd profile charm status",
			output: M{
				"model": model,
				"machines": M{
					"0": machine0,
					"1": machine1WithLXDProfile,
				},
				"applications": M{
					"lxd-profile": lxdProfileCharm(M{
						"charm":        "local:quantal/lxd-profile-1",
						"charm-origin": "local",
						"exposed":      true,
						"application-status": M{
							"current": "active",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"units": M{
							"lxd-profile/0": M{
								"machine": "1",
								"workload-status": M{
									"current": "active",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"juju-status": M{
									"current": "idle",
									"since":   "01 Apr 15 01:23+10:00",
								},
								"upgrading-from": "cs:quantal/lxd-profile-0",
								"public-address": "10.0.1.1",
							},
						},
						"endpoint-bindings": M{
							"another": "",
							"ubuntu":  "",
						},
					}),
				},
				"storage": M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
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
	return composeCharms(charm, extras)
}

func lxdProfileCharm(extras M) M {
	charm := M{
		"charm":         "cs:quantal/lxd-profile-0",
		"charm-origin":  "jujucharms",
		"charm-name":    "lxd-profile",
		"charm-rev":     1,
		"charm-profile": "juju-controller-lxd-profile-1",
		"series":        "quantal",
		"os":            "ubuntu",
		"exposed":       false,
	}
	return composeCharms(charm, extras)
}

func meteredCharm(extras M) M {
	charm := M{
		"charm":        "cs:quantal/metered-1",
		"charm-origin": "jujucharms",
		"charm-name":   "metered",
		"charm-rev":    1,
		"series":       "quantal",
		"os":           "ubuntu",
		"exposed":      false,
	}
	return composeCharms(charm, extras)
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
	return composeCharms(charm, extras)
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
	return composeCharms(charm, extras)
}

func composeCharms(origin, extras M) M {
	result := make(M, len(origin))
	for key, value := range origin {
		result[key] = value
	}
	for key, value := range extras {
		result[key] = value
	}
	return result
}

// TODO(dfc) test failing components by destructively mutating the state under the hood

// sometimes you just need to skip the tests for windows (environment variables etc)
type skipTestOnWindows struct{}

func (skipTestOnWindows) step(c *gc.C, ctx *context) {
	if runtime.GOOS == "windows" {
		ctx.skipTest = true
	}
}

type setSLA struct {
	level string
}

func (s setSLA) step(c *gc.C, ctx *context) {
	err := ctx.st.SetSLA(s.level, "test-user", []byte(""))
	c.Assert(err, jc.ErrorIsNil)
}

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
	inst, hc := testing.AssertStartInstanceWithConstraints(c, ctx.env, environscontext.NewCloudCallContext(), cfg.ControllerUUID(), m.Id(), cons)
	err = m.SetProvisioned(inst.Id(), "", "fake_nonce", hc)
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
	_, hc := testing.AssertStartInstanceWithConstraints(c, ctx.env, environscontext.NewCloudCallContext(), cfg.ControllerUUID(), m.Id(), cons)
	err = m.SetProvisioned("i-missing", "", "fake_nonce", hc)
	c.Assert(err, jc.ErrorIsNil)
	// lp:1558657
	now := time.Now()
	s := status.StatusInfo{
		Status:  status.Unknown,
		Message: "missing",
		Since:   &now,
	}
	err = m.SetInstanceStatus(s)
	c.Assert(err, jc.ErrorIsNil)
}

type startAliveMachine struct {
	machineId   string
	displayName string
}

func (sam startAliveMachine) step(c *gc.C, ctx *context) {
	m, err := ctx.st.Machine(sam.machineId)
	c.Assert(err, jc.ErrorIsNil)
	pinger := ctx.setAgentPresence(c, m)
	cons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := ctx.st.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	inst, hc := testing.AssertStartInstanceWithConstraints(c, ctx.env, environscontext.NewCloudCallContext(), cfg.ControllerUUID(), m.Id(), cons)
	err = m.SetProvisioned(inst.Id(), sam.displayName, "fake_nonce", hc)
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
	inst, _ := testing.AssertStartInstanceWithConstraints(c, ctx.env, environscontext.NewCloudCallContext(), cfg.ControllerUUID(), m.Id(), cons)
	err = m.SetProvisioned(inst.Id(), "", "fake_nonce", &sm.hc)
	c.Assert(err, jc.ErrorIsNil)
	ctx.pingers[m.Id()] = pinger
}

type startAliveMachineWithDisplayName struct {
	machineId   string
	displayName string
}

func (sm startAliveMachineWithDisplayName) step(c *gc.C, ctx *context) {
	m, err := ctx.st.Machine(sm.machineId)
	c.Assert(err, jc.ErrorIsNil)
	pinger := ctx.setAgentPresence(c, m)
	cons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := ctx.st.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	inst, hc := testing.AssertStartInstanceWithConstraints(c, ctx.env, environscontext.NewCloudCallContext(), cfg.ControllerUUID(), m.Id(), cons)
	err = m.SetProvisioned(inst.Id(), sm.displayName, "fake_nonce", hc)
	c.Assert(err, jc.ErrorIsNil)
	ctx.pingers[m.Id()] = pinger
	_, displayName, err := m.InstanceNames()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(displayName, gc.Equals, sm.displayName)
}

type setMachineInstanceStatus struct {
	machineId string
	Status    status.Status
	Message   string
}

func (sm setMachineInstanceStatus) step(c *gc.C, ctx *context) {
	m, err := ctx.st.Machine(sm.machineId)
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now()
	s := status.StatusInfo{
		Status:  sm.Status,
		Message: sm.Message,
		Since:   &now,
	}
	err = m.SetInstanceStatus(s)
	c.Assert(err, jc.ErrorIsNil)
}

type setMachineModificationStatus struct {
	machineId string
	Status    status.Status
	Message   string
}

func (sm setMachineModificationStatus) step(c *gc.C, ctx *context) {
	m, err := ctx.st.Machine(sm.machineId)
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now()
	s := status.StatusInfo{
		Status:  sm.Status,
		Message: sm.Message,
		Since:   &now,
	}
	err = m.SetModificationStatus(s)
	c.Assert(err, jc.ErrorIsNil)
}

type addSpace struct {
	spaceName string
}

func (sp addSpace) step(c *gc.C, ctx *context) {
	f := factory.NewFactory(ctx.st, ctx.pool)
	f.MakeSpace(c, &factory.SpaceParams{
		Name: sp.spaceName, ProviderID: corenetwork.Id("provider"), IsPublic: true})
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
	addrs := make([]state.LinkLayerDeviceAddress, len(sa.addresses))
	lldevs := make([]state.LinkLayerDeviceArgs, len(sa.addresses))
	for i, address := range sa.addresses {
		devName := fmt.Sprintf("eth%d", i)
		macAddr := "aa:bb:cc:dd:ee:ff"
		configMethod := state.StaticAddress
		devType := state.EthernetDevice
		if address.Scope == network.ScopeMachineLocal ||
			address.Value == "localhost" {
			devName = "lo"
			macAddr = "00:00:00:00:00:00"
			configMethod = state.LoopbackAddress
			devType = state.LoopbackDevice
		}
		lldevs[i] = state.LinkLayerDeviceArgs{
			Name:       devName,
			MACAddress: macAddr, // TODO(macgreagoir) Enough for first pass
			IsUp:       true,
			Type:       devType,
		}
		addrs[i] = state.LinkLayerDeviceAddress{
			DeviceName:   devName,
			ConfigMethod: configMethod,
			// TODO(macgreagoir) Enough for first pass, but
			// incorrect for IPv4 loopback, and breaks IPv6
			// loopback.
			CIDRAddress: fmt.Sprintf("%s/24", address.Value)}
	}
	// TODO(macgreagoir) Let these go for now, before this turns into a test for setting lldevs and addrs.
	// err = m.SetLinkLayerDevices(lldevs...)
	// c.Assert(err, jc.ErrorIsNil)
	_ = m.SetLinkLayerDevices(lldevs...)
	// err = m.SetDevicesAddresses(addrs...)
	// c.Assert(err, jc.ErrorIsNil)
	_ = m.SetDevicesAddresses(addrs...)
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

type addApplication struct {
	name    string
	charm   string
	binding map[string]string
	cons    constraints.Value
}

func (as addApplication) step(c *gc.C, ctx *context) {
	ch, ok := ctx.charms[as.charm]
	c.Assert(ok, jc.IsTrue)
	app, err := ctx.st.AddApplication(state.AddApplicationArgs{Name: as.name, Charm: ch, EndpointBindings: as.binding})
	c.Assert(err, jc.ErrorIsNil)
	if app.IsPrincipal() {
		err = app.SetConstraints(as.cons)
		c.Assert(err, jc.ErrorIsNil)
	}
}

type addRemoteApplication struct {
	name            string
	url             string
	charm           string
	endpoints       []string
	isConsumerProxy bool
}

func (as addRemoteApplication) step(c *gc.C, ctx *context) {
	ch, ok := ctx.charms[as.charm]
	c.Assert(ok, jc.IsTrue)
	var endpoints []charm.Relation
	for _, ep := range as.endpoints {
		r, ok := ch.Meta().Requires[ep]
		if !ok {
			r, ok = ch.Meta().Provides[ep]
		}
		c.Assert(ok, jc.IsTrue)
		endpoints = append(endpoints, r)
	}
	_, err := ctx.st.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            as.name,
		URL:             as.url,
		SourceModel:     coretesting.ModelTag,
		Endpoints:       endpoints,
		IsConsumerProxy: as.isConsumerProxy,
	})
	c.Assert(err, jc.ErrorIsNil)
}

type addApplicationOffer struct {
	name            string
	owner           string
	applicationName string
	endpoints       []string
}

func (ao addApplicationOffer) step(c *gc.C, ctx *context) {
	endpoints := make(map[string]string)
	for _, ep := range ao.endpoints {
		endpoints[ep] = ep
	}
	offers := state.NewApplicationOffers(ctx.st)
	_, err := offers.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       ao.name,
		Owner:           ao.owner,
		ApplicationName: ao.applicationName,
		Endpoints:       endpoints,
	})
	c.Assert(err, jc.ErrorIsNil)
}

type addOfferConnection struct {
	sourceModelUUID string
	name            string
	username        string
	relationKey     string
}

func (oc addOfferConnection) step(c *gc.C, ctx *context) {
	rel, err := ctx.st.KeyRelation(oc.relationKey)
	c.Assert(err, jc.ErrorIsNil)
	offer, err := state.NewApplicationOffers(ctx.st).ApplicationOffer(oc.name)
	c.Assert(err, jc.ErrorIsNil)
	_, err = ctx.st.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: oc.sourceModelUUID,
		OfferUUID:       offer.OfferUUID,
		Username:        oc.username,
		RelationId:      rel.Id(),
		RelationKey:     rel.Tag().Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
}

type setApplicationExposed struct {
	name    string
	exposed bool
}

func (sse setApplicationExposed) step(c *gc.C, ctx *context) {
	s, err := ctx.st.Application(sse.name)
	c.Assert(err, jc.ErrorIsNil)
	err = s.ClearExposed()
	c.Assert(err, jc.ErrorIsNil)
	if sse.exposed {
		err = s.SetExposed()
		c.Assert(err, jc.ErrorIsNil)
	}
}

type setApplicationCharm struct {
	name  string
	charm string
}

func (ssc setApplicationCharm) step(c *gc.C, ctx *context) {
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
	applicationName string
	machineId       string
}

func (au addUnit) step(c *gc.C, ctx *context) {
	s, err := ctx.st.Application(au.applicationName)
	c.Assert(err, jc.ErrorIsNil)
	u, err := s.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	m, err := ctx.st.Machine(au.machineId)
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	ctx.statusSetter.SetAgentStatus(u.Tag().String(), corepresence.Missing)
}

type addAliveUnit struct {
	applicationName string
	machineId       string
}

func (aau addAliveUnit) step(c *gc.C, ctx *context) {
	s, err := ctx.st.Application(aau.applicationName)
	c.Assert(err, jc.ErrorIsNil)
	u, err := s.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	pinger := ctx.setAgentPresence(c, u)
	m, err := ctx.st.Machine(aau.machineId)
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	ctx.pingers[u.Name()] = pinger
}

type setUnitsAlive struct {
	applicationName string
}

func (sua setUnitsAlive) step(c *gc.C, ctx *context) {
	s, err := ctx.st.Application(sua.applicationName)
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

type setModelMeterStatus struct {
	color   string
	message string
}

func (s setModelMeterStatus) step(c *gc.C, ctx *context) {
	m, err := ctx.st.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetMeterStatus(s.color, s.message)
	c.Assert(err, jc.ErrorIsNil)
}

type setUnitAsLeader struct {
	unitName string
}

func (s setUnitAsLeader) step(c *gc.C, ctx *context) {
	u, err := ctx.st.Unit(s.unitName)
	c.Assert(err, jc.ErrorIsNil)

	// We must use the lease manager from the API server.
	// Requesting it from state will claim against a *different* legacy lease
	// manager running in the state workers collection.
	stater := ctx.env.(testing.GetStater)
	claimer, err := stater.GetLeaseManagerInAPIServer().Claimer("application-leadership", ctx.st.ModelUUID())

	err = claimer.Claim(u.ApplicationName(), u.Name(), time.Minute)
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
		Status:  status.Active,
		Message: "",
		Since:   &now,
	}
	err = u.SetStatus(s)
	c.Assert(err, jc.ErrorIsNil)
	sInfo := status.StatusInfo{
		Status:  status.Idle,
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

type ensureDyingApplication struct {
	applicationName string
}

func (e ensureDyingApplication) step(c *gc.C, ctx *context) {
	svc, err := ctx.st.Application(e.applicationName)
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

type relateApplications struct {
	ep1, ep2 string
	status   string
}

func (rs relateApplications) step(c *gc.C, ctx *context) {
	eps, err := ctx.st.InferEndpoints(rs.ep1, rs.ep2)
	c.Assert(err, jc.ErrorIsNil)
	rel, err := ctx.st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	s := rs.status
	if s == "" {
		s = "joined"
	}
	err = rel.SetStatus(status.StatusInfo{Status: status.Status(s)})
	c.Assert(err, jc.ErrorIsNil)
}

type addSubordinate struct {
	prinUnit       string
	subApplication string
}

func (as addSubordinate) step(c *gc.C, ctx *context) {
	u, err := ctx.st.Unit(as.prinUnit)
	c.Assert(err, jc.ErrorIsNil)
	eps, err := ctx.st.InferEndpoints(u.ApplicationName(), as.subApplication)
	c.Assert(err, jc.ErrorIsNil)
	rel, err := ctx.st.EndpointsRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(u)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
}

type setCharmProfiles struct {
	machineId string
	profiles  []string
}

func (s setCharmProfiles) step(c *gc.C, ctx *context) {
	m, err := ctx.st.Machine(s.machineId)
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetCharmProfiles(s.profiles)
	c.Assert(err, jc.ErrorIsNil)
}

type scopedExpect struct {
	what   string
	scope  []string
	output M
	stderr string
}

type expect struct {
	what   string
	output M
	stderr string
}

// substituteFakeTime replaces all key values
// in actual status output with a known fake value.
func substituteFakeTime(c *gc.C, key string, in []byte, expectIsoTime bool) []byte {
	// This regexp will work for yaml and json.
	exp := regexp.MustCompile(`(?P<key>"?` + key + `"?:\ ?)(?P<quote>"?)(?P<timestamp>[^("|\n)]*)*"?`)
	// Before the substitution is done, check that the timestamp produced
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

	out := exp.ReplaceAllString(string(in), `$key$quote<timestamp>$quote`)
	// Substitute a made up time used in our expected output.
	out = strings.Replace(out, "<timestamp>", "01 Apr 15 01:23+10:00", -1)
	return []byte(out)
}

// substituteFakeTimestamp replaces all key values for a given timestamp
// in actual status output with a known fake value.
func substituteFakeTimestamp(c *gc.C, in []byte, expectIsoTime bool) []byte {
	timeFormat := "15:04:05Z07:00"
	output := strings.Replace(timeFormat, "Z", "+", -1)
	if expectIsoTime {
		timeFormat = "15:04:05Z"
		output = "15:04:05"
	}
	// This regexp will work for any input type
	exp := regexp.MustCompile(`(?P<timestamp>[0-9]{2}:[0-9]{2}:[0-9]{2}((Z|\+|\-)([0-9]{2}:[0-9]{2})?)?)`)
	if matches := exp.FindStringSubmatch(string(in)); matches != nil {
		for i, name := range exp.SubexpNames() {
			if name != "timestamp" {
				continue
			}
			value := matches[i]
			if num := len(value); num == 8 {
				value = fmt.Sprintf("%sZ", value)
			} else if num == 14 && (expectIsoTime || value[8] == 'Z') {
				value = fmt.Sprintf("%sZ", value[:8])
			}
			_, err := time.Parse(timeFormat, value)
			c.Assert(err, jc.ErrorIsNil)
		}
	}

	out := exp.ReplaceAllString(string(in), `<timestamp>`)
	// Substitute a made up time used in our expected output.
	out = strings.Replace(out, "<timestamp>", output, -1)
	return []byte(out)
}

// substituteSpacingBetweenTimestampAndNotes forces the spacing between the
// headers Timestamp and Notes to be consistent regardless of the time. This
// happens because we're dealing with the result of the strings of stdout and
// not with any useable AST
func substituteSpacingBetweenTimestampAndNotes(c *gc.C, in []byte) []byte {
	exp := regexp.MustCompile(`Timestamp(?P<spacing>\s+)Notes`)
	result := exp.ReplaceAllString(string(in), fmt.Sprintf("Timestamp%sNotes", strings.Repeat(" ", 7)))
	return []byte(result)
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
		c.Assert(string(stderr), gc.Equals, e.stderr)

		// Prepare the output in the same format.
		buf, err := format.marshal(e.output)
		c.Assert(err, jc.ErrorIsNil)

		// we have to force the timestamp into the correct format as the model
		// is in string.
		buf = substituteFakeTimestamp(c, buf, ctx.expectIsoTime)

		expected := make(M)
		err = format.unmarshal(buf, &expected)
		c.Assert(err, jc.ErrorIsNil)

		// Check the output is as expected.
		actual := make(M)
		out := substituteFakeTime(c, "since", stdout, ctx.expectIsoTime)
		out = substituteFakeTimestamp(c, out, ctx.expectIsoTime)
		err = format.unmarshal(out, &actual)
		c.Assert(err, jc.ErrorIsNil)
		pretty.Ldiff(c, actual, expected)
		c.Assert(actual, jc.DeepEquals, expected)
	}
}

func (e expect) step(c *gc.C, ctx *context) {
	scopedExpect{e.what, nil, e.output, e.stderr}.step(c, ctx)
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
			"type":       "iaas",
			"controller": "kontroll",
			"cloud":      "dummy",
			"region":     "dummy-region",
			"version":    "1.2.3",
			"model-status": M{
				"current": "busy",
				"since":   "01 Apr 15 01:23+10:00",
				"message": "migrating: foo bar",
			},
			"sla": "unsupported",
		},
		"machines":     M{},
		"applications": M{},
		"storage":      M{},
		"controller": M{
			"timestamp": "15:04:05+07:00",
		},
	}

	for _, format := range statusFormats {
		code, stdout, stderr := runStatus(c, "-m", "hosted", "--format", format.name)
		c.Check(code, gc.Equals, 0)
		c.Assert(string(stderr), gc.Equals, "Model \"hosted\" is empty.\n")

		stdout = substituteFakeTime(c, "since", stdout, false)
		stdout = substituteFakeTimestamp(c, stdout, false)

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
Model   Controller  Cloud/Region        Version  SLA          Timestamp       Notes
hosted  kontroll    dummy/dummy-region  1.2.3    unsupported  15:04:05+07:00  migrating: foo bar

`[1:]

	st := s.setupMigrationTest(c)
	defer st.Close()
	code, stdout, stderr := runStatus(c, "-m", "hosted", "--format", "tabular")
	c.Assert(code, gc.Equals, 0)
	c.Assert(string(stderr), gc.Equals, "Model \"hosted\" is empty.\n")

	output := substituteFakeTimestamp(c, stdout, false)
	output = substituteSpacingBetweenTimestampAndNotes(c, output)
	c.Assert(string(output), gc.Equals, expected)
}

func (s *StatusSuite) TestMigrationInProgressAndUpgradeAvailable(c *gc.C) {
	expected := `
Model   Controller  Cloud/Region        Version  SLA          Timestamp       Notes
hosted  kontroll    dummy/dummy-region  1.2.3    unsupported  15:04:05+07:00  migrating: foo bar

`[1:]

	st := s.setupMigrationTest(c)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.UpdateLatestToolsVersion(nextVersion)
	c.Assert(err, jc.ErrorIsNil)

	code, stdout, stderr := runStatus(c, "-m", "hosted", "--format", "tabular")
	c.Assert(code, gc.Equals, 0)
	c.Assert(string(stderr), gc.Equals, "Model \"hosted\" is empty.\n")

	output := substituteFakeTimestamp(c, stdout, false)
	output = substituteSpacingBetweenTimestampAndNotes(c, output)
	c.Assert(string(output), gc.Equals, expected)
}

func (s *StatusSuite) setupMigrationTest(c *gc.C) *state.State {
	const hostedModelName = "hosted"
	const statusText = "foo bar"

	f := factory.NewFactory(s.BackingState, s.StatePool)
	hostedSt := f.MakeModel(c, &factory.ModelParams{
		Name: hostedModelName,
	})

	mig, err := hostedSt.CreateMigration(state.MigrationSpec{
		InitiatedBy: names.NewUserTag("admin"),
		TargetInfo: migration.TargetInfo{
			ControllerTag: names.NewControllerTag(utils.MustNewUUID().String()),
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

type fakeAPIClient struct {
	statusReturn *params.FullStatus
	patternsUsed []string
	closeCalled  bool
}

func (a *fakeAPIClient) Status(patterns []string) (*params.FullStatus, error) {
	a.patternsUsed = patterns
	return a.statusReturn, nil
}

func (a *fakeAPIClient) Close() error {
	a.closeCalled = true
	return nil
}

func (s *StatusSuite) TestStatusWithFormatSummary(c *gc.C) {
	ctx := s.newContext(c)
	defer s.resetContext(c, ctx)
	steps := []stepper{
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("localhost")},
		startAliveMachine{"0", "snowflake"},
		setMachineStatus{"0", status.Started, ""},
		addCharm{"wordpress"},
		addCharm{"mysql"},
		addCharm{"logging"},
		addCharm{"riak"},
		addRemoteApplication{name: "hosted-riak", url: "me/model.riak", charm: "riak", endpoints: []string{"endpoint"}},
		addApplication{name: "wordpress", charm: "wordpress"},
		setApplicationExposed{"wordpress", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("localhost")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addAliveUnit{"wordpress", "1"},
		setAgentStatus{"wordpress/0", status.Idle, "", nil},
		setUnitStatus{"wordpress/0", status.Active, "", nil},
		addApplication{name: "mysql", charm: "mysql"},
		setApplicationExposed{"mysql", true},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", network.NewAddresses("10.0.2.1")},
		startAliveMachine{"2", ""},
		setMachineStatus{"2", status.Started, ""},
		addAliveUnit{"mysql", "2"},
		setAgentStatus{"mysql/0", status.Idle, "", nil},
		setUnitStatus{"mysql/0", status.Active, "", nil},
		addApplication{name: "logging", charm: "logging"},
		setApplicationExposed{"logging", true},
		relateApplications{"wordpress", "mysql", ""},
		relateApplications{"wordpress", "logging", ""},
		relateApplications{"mysql", "logging", ""},
		addSubordinate{"wordpress/0", "logging"},
		addSubordinate{"mysql/0", "logging"},
		setUnitsAlive{"logging"},
		setAgentStatus{"logging/0", status.Idle, "", nil},
		setUnitStatus{"logging/0", status.Active, "", nil},
		setAgentStatus{"logging/1", status.Error, "somehow lost in all those logs", nil},
	}
	for _, s := range steps {
		s.step(c, ctx)
	}
	code, stdout, stderr := runStatus(c, "--format", "summary")
	c.Check(code, gc.Equals, 0)
	c.Check(string(stderr), gc.Equals, "")
	c.Assert(string(stdout), gc.Equals, `
Running on subnets:  127.0.0.1/8, 10.0.2.1/8  
 Utilizing ports:                             
      # Machines:  (3)
         started:   3 
                 
         # Units:  (4)
          active:   3 
           error:   1 
                 
  # Applications:  (3)
          logging  1/1  exposed
            mysql  1/1  exposed
        wordpress  1/1  exposed
                 
        # Remote:  (1)
      hosted-riak       me/model.riak

`[1:])
}
func (s *StatusSuite) TestStatusWithFormatOneline(c *gc.C) {
	ctx := s.newContext(c)
	defer s.resetContext(c, ctx)
	steps := []stepper{
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("10.0.0.1")},
		startAliveMachine{"0", "snowflake"},
		setMachineStatus{"0", status.Started, ""},
		addCharm{"wordpress"},
		addCharm{"mysql"},
		addCharm{"logging"},

		addApplication{name: "wordpress", charm: "wordpress"},
		setApplicationExposed{"wordpress", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addAliveUnit{"wordpress", "1"},
		setAgentStatus{"wordpress/0", status.Idle, "", nil},
		setUnitStatus{"wordpress/0", status.Active, "", nil},

		addApplication{name: "mysql", charm: "mysql"},
		setApplicationExposed{"mysql", true},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", network.NewAddresses("10.0.2.1")},
		startAliveMachine{"2", ""},
		setMachineStatus{"2", status.Started, ""},
		addAliveUnit{"mysql", "2"},
		setAgentStatus{"mysql/0", status.Idle, "", nil},
		setUnitStatus{"mysql/0", status.Active, "", nil},

		addApplication{name: "logging", charm: "logging"},
		setApplicationExposed{"logging", true},

		relateApplications{"wordpress", "mysql", ""},
		relateApplications{"wordpress", "logging", ""},
		relateApplications{"mysql", "logging", ""},

		addSubordinate{"wordpress/0", "logging"},
		addSubordinate{"mysql/0", "logging"},

		setUnitsAlive{"logging"},
		setAgentStatus{"logging/0", status.Idle, "", nil},
		setUnitStatus{"logging/0", status.Active, "", nil},
		setAgentStatus{"logging/1", status.Error, "somehow lost in all those logs", nil},
	}

	ctx.run(c, steps)

	const expected = `
- mysql/0: 10.0.2.1 (agent:idle, workload:active)
  - logging/1: 10.0.2.1 (agent:idle, workload:error)
- wordpress/0: 10.0.1.1 (agent:idle, workload:active)
  - logging/0: 10.0.1.1 (agent:idle, workload:active)
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
		setAddresses{"0", network.NewAddresses("10.0.0.1")},
		startMachineWithHardware{"0", instance.MustParseHardware("availability-zone=us-east-1a")},
		setMachineStatus{"0", status.Started, ""},
		addCharm{"wordpress"},
		addCharm{"mysql"},
		addCharm{"logging"},
		addCharm{"riak"},
		addRemoteApplication{name: "hosted-riak", url: "me/model.riak", charm: "riak", endpoints: []string{"endpoint"}},
		addApplication{name: "wordpress", charm: "wordpress"},
		setApplicationExposed{"wordpress", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		setAddresses{"1", network.NewAddresses("10.0.1.1")},
		startAliveMachine{"1", "snowflake"},
		setMachineStatus{"1", status.Started, ""},
		addAliveUnit{"wordpress", "1"},
		setAgentStatus{"wordpress/0", status.Idle, "", nil},
		setUnitStatus{"wordpress/0", status.Active, "", nil},
		setUnitTools{"wordpress/0", version.MustParseBinary("1.2.3-trusty-ppc")},
		addApplication{name: "mysql", charm: "mysql"},
		setApplicationExposed{"mysql", true},
		addMachine{machineId: "2", job: state.JobHostUnits},
		setAddresses{"2", network.NewAddresses("10.0.2.1")},
		startAliveMachine{"2", ""},
		setMachineStatus{"2", status.Started, ""},
		addAliveUnit{"mysql", "2"},
		setAgentStatus{"mysql/0", status.Idle, "", nil},
		setUnitStatus{
			"mysql/0",
			status.Maintenance,
			"installing all the things", nil},
		addAliveUnit{"mysql", "1"},
		setAgentStatus{"mysql/1", status.Idle, "", nil},
		setUnitStatus{
			"mysql/1",
			status.Terminated,
			"gooooone", nil},
		setUnitTools{"mysql/0", version.MustParseBinary("1.2.3-trusty-ppc")},
		addApplication{name: "logging", charm: "logging"},
		setApplicationExposed{"logging", true},
		relateApplications{"wordpress", "mysql", "suspended"},
		relateApplications{"wordpress", "logging", ""},
		relateApplications{"mysql", "logging", ""},
		addSubordinate{"wordpress/0", "logging"},
		addSubordinate{"mysql/0", "logging"},
		setUnitsAlive{"logging"},
		setAgentStatus{"logging/0", status.Idle, "", nil},
		setUnitStatus{"logging/0", status.Active, "", nil},
		setAgentStatus{"logging/1", status.Error, "somehow lost in all those logs", nil},
		setUnitWorkloadVersion{"logging/1", "a bit too long, really"},
		setUnitWorkloadVersion{"wordpress/0", "4.5.3"},
		setUnitWorkloadVersion{"mysql/0", "5.7.13"},
		setUnitAsLeader{"mysql/0"},
		setUnitAsLeader{"logging/1"},
		setUnitAsLeader{"wordpress/0"},
		addMachine{machineId: "3", job: state.JobHostUnits},
		setAddresses{"3", network.NewAddresses("10.0.3.1")},
		startAliveMachine{"3", ""},
		setMachineStatus{"3", status.Started, ""},
		setMachineInstanceStatus{"3", status.Started, "I am number three"},

		addApplicationOffer{name: "hosted-mysql", applicationName: "mysql", owner: "admin", endpoints: []string{"server"}},
		addRemoteApplication{name: "remote-wordpress", charm: "wordpress", endpoints: []string{"db"}, isConsumerProxy: true},
		relateApplications{"remote-wordpress", "mysql", ""},
		addOfferConnection{sourceModelUUID: coretesting.ModelTag.Id(), name: "hosted-mysql", username: "fred", relationKey: "remote-wordpress:db mysql:server"},

		// test modification status
		addMachine{machineId: "4", job: state.JobHostUnits},
		setAddresses{"4", network.NewAddresses("10.0.3.1")},
		startAliveMachine{"4", ""},
		setMachineStatus{"4", status.Started, ""},
		setMachineInstanceStatus{"4", status.Started, "I am number four"},
		setMachineModificationStatus{"4", status.Error, "I am an error"},
	}
	for _, s := range steps {
		s.step(c, ctx)
	}
	return ctx
}

var expectedTabularStatus = `
Model       Controller  Cloud/Region        Version  SLA          Timestamp       Notes
controller  kontroll    dummy/dummy-region  1.2.3    unsupported  15:04:05+07:00  upgrade available: 1.2.4

SAAS         Status   Store  URL
hosted-riak  unknown  local  me/model.riak

App        Version          Status       Scale  Charm      Store       Rev  OS      Notes
logging    a bit too lo...  error            2  logging    jujucharms    1  ubuntu  exposed
mysql      5.7.13           maintenance    1/2  mysql      jujucharms    1  ubuntu  exposed
wordpress  4.5.3            active           1  wordpress  jujucharms    3  ubuntu  exposed

Unit          Workload     Agent  Machine  Public address  Ports  Message
mysql/0*      maintenance  idle   2        10.0.2.1               installing all the things
  logging/1*  error        idle            10.0.2.1               somehow lost in all those logs
mysql/1       terminated   idle   1        10.0.1.1               gooooone
wordpress/0*  active       idle   1        10.0.1.1               
  logging/0   active       idle            10.0.1.1               

Machine  State    DNS       Inst id       Series   AZ          Message
0        started  10.0.0.1  controller-0  quantal  us-east-1a  
1        started  10.0.1.1  snowflake     quantal              
2        started  10.0.2.1  controller-2  quantal              
3        started  10.0.3.1  controller-3  quantal              I am number three
4        error    10.0.3.1  controller-4  quantal              I am an error

Offer         Application  Charm  Rev  Connected  Endpoint  Interface  Role
hosted-mysql  mysql        mysql  1    1/1        server    mysql      provider

Relation provider      Requirer                   Interface  Type         Message
mysql:juju-info        logging:info               juju-info  subordinate  
mysql:server           wordpress:db               mysql      regular      suspended  
wordpress:logging-dir  logging:logging-directory  logging    subordinate  

`[1:]

func (s *StatusSuite) TestStatusWithFormatTabular(c *gc.C) {
	ctx := s.prepareTabularData(c)
	defer s.resetContext(c, ctx)
	code, stdout, stderr := runStatus(c, "--format", "tabular", "--relations")
	c.Check(code, gc.Equals, 0)
	c.Check(string(stderr), gc.Equals, "")

	output := substituteFakeTimestamp(c, stdout, false)
	output = substituteSpacingBetweenTimestampAndNotes(c, output)
	c.Assert(string(output), gc.Equals, expectedTabularStatus)
}

func (s *StatusSuite) TestStatusWithFormatTabularValidModelUUID(c *gc.C) {
	ctx := s.prepareTabularData(c)
	defer s.resetContext(c, ctx)

	code, stdout, stderr := runStatus(c, "--format", "tabular", "--relations", "-m", s.Model.UUID())
	c.Check(code, gc.Equals, 0)
	c.Check(string(stderr), gc.Equals, "")

	output := substituteFakeTimestamp(c, stdout, false)
	output = substituteSpacingBetweenTimestampAndNotes(c, output)
	c.Assert(string(output), gc.Equals, expectedTabularStatus)
}

func (s *StatusSuite) TestStatusWithFormatYaml(c *gc.C) {
	ctx := s.prepareTabularData(c)
	defer s.resetContext(c, ctx)
	code, stdout, stderr := runStatus(c, "--format", "yaml")
	c.Check(code, gc.Equals, 0)
	c.Check(string(stderr), gc.Equals, "")
	c.Assert(string(stdout), jc.Contains, "display-name: snowflake")
}

func (s *StatusSuite) TestStatusWithFormatJson(c *gc.C) {
	ctx := s.prepareTabularData(c)
	defer s.resetContext(c, ctx)
	code, stdout, stderr := runStatus(c, "--format", "json")
	c.Check(code, gc.Equals, 0)
	c.Check(string(stderr), gc.Equals, "")
	c.Assert(string(stdout), jc.Contains, `"display-name":"snowflake"`)
}

func (s *StatusSuite) TestFormatTabularHookActionName(c *gc.C) {
	status := formattedStatus{
		Applications: map[string]applicationStatus{
			"foo": {
				Units: map[string]unitStatus{
					"foo/0": {
						JujuStatusInfo: statusInfoContents{
							Current: status.Executing,
							Message: "running config-changed hook",
						},
						WorkloadStatusInfo: statusInfoContents{
							Current: status.Maintenance,
							Message: "doing some work",
						},
					},
					"foo/1": {
						JujuStatusInfo: statusInfoContents{
							Current: status.Executing,
							Message: "running action backup database",
						},
						WorkloadStatusInfo: statusInfoContents{
							Current: status.Maintenance,
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
Model  Controller  Cloud/Region  Version
                                 

App  Version  Status  Scale  Charm  Store  Rev  OS  Notes
foo                       2                  0      

Unit   Workload     Agent      Machine  Public address  Ports  Message
foo/0  maintenance  executing                                  (config-changed) doing some work
foo/1  maintenance  executing                                  (backup database) doing some work
`[1:])
}

func (s *StatusSuite) TestFormatTabularCAASModel(c *gc.C) {
	status := formattedStatus{
		Model: modelStatus{
			Type: "caas",
		},
		Applications: map[string]applicationStatus{
			"foo": {
				Scale:   2,
				Address: "54.32.1.2",
				Units: map[string]unitStatus{
					"foo/0": {
						JujuStatusInfo: statusInfoContents{
							Current: status.Allocating,
						},
						WorkloadStatusInfo: statusInfoContents{
							Current: status.Active,
						},
					},
					"foo/1": {
						Address:     "10.0.0.1",
						OpenedPorts: []string{"80/TCP"},
						JujuStatusInfo: statusInfoContents{
							Current: status.Running,
						},
						WorkloadStatusInfo: statusInfoContents{
							Current: status.Active,
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
Model  Controller  Cloud/Region  Version
                                 

App  Version  Status  Scale  Charm  Store  Rev  OS  Address    Notes
foo                     1/2                  0      54.32.1.2  

Unit   Workload  Agent       Address   Ports   Message
foo/0  active    allocating                    
foo/1  active    running     10.0.0.1  80/TCP  
`[1:])
}

func (s *StatusSuite) TestFormatTabularStatusNotes(c *gc.C) {
	fStatus := formattedStatus{
		Model: modelStatus{
			Type: "caas",
		},
		Applications: map[string]applicationStatus{
			"foo": {
				Scale:   1,
				Address: "54.32.1.2",
				StatusInfo: statusInfoContents{
					Message: "Error: ImagePullBackOff",
				},
				Units: map[string]unitStatus{
					"foo/0": {
						Address:     "10.0.0.1",
						OpenedPorts: []string{"80/TCP"},
						JujuStatusInfo: statusInfoContents{
							Current: status.Allocating,
						},
						WorkloadStatusInfo: statusInfoContents{
							Current: status.Waiting,
						},
					},
				},
			},
		},
	}
	out := &bytes.Buffer{}
	err := FormatTabular(out, false, fStatus)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out.String(), gc.Equals, `
Model  Controller  Cloud/Region  Version
                                 

App  Version  Status  Scale  Charm  Store  Rev  OS  Address    Notes
foo                     0/1                  0      54.32.1.2  Error: ImagePullBackOff

Unit   Workload  Agent       Address   Ports   Message
foo/0  waiting   allocating  10.0.0.1  80/TCP  
`[1:])
}

func (s *StatusSuite) TestFormatTabularStatusNotesIAAS(c *gc.C) {
	status := formattedStatus{
		Applications: map[string]applicationStatus{
			"foo": {
				Address: "54.32.1.2",
				StatusInfo: statusInfoContents{
					Message: "Error: ImagePullBackOff",
				},
				Units: map[string]unitStatus{
					"foo/0": {
						Address:     "10.0.0.1",
						OpenedPorts: []string{"80/TCP"},
						JujuStatusInfo: statusInfoContents{
							Current: status.Idle,
						},
						WorkloadStatusInfo: statusInfoContents{
							Current: status.Waiting,
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
Model  Controller  Cloud/Region  Version
                                 

App  Version  Status  Scale  Charm  Store  Rev  OS  Notes
foo                       1                  0      

Unit   Workload  Agent  Machine  Public address  Ports   Message
foo/0  waiting   idle                            80/TCP  
`[1:])
}

func (s *StatusSuite) TestStatusWithNilStatusAPI(c *gc.C) {
	ctx := s.newContext(c)
	defer s.resetContext(c, ctx)
	steps := []stepper{
		addMachine{machineId: "0", job: state.JobManageModel},
		setAddresses{"0", network.NewAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
	}

	for _, s := range steps {
		s.step(c, ctx)
	}

	client := fakeAPIClient{}
	var status = client.Status
	s.PatchValue(&status, func(_ []string) (*params.FullStatus, error) {
		return nil, nil
	})
	s.PatchValue(&newAPIClientForStatus, func(_ *statusCommand) (statusAPI, error) {
		return &client, nil
	})

	code, _, stderr := runStatus(c, "--format", "tabular")
	c.Check(code, gc.Equals, 1)
	c.Check(string(stderr), gc.Equals, "ERROR unable to obtain the current status\n")
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
Model  Controller  Cloud/Region  Version
                                 

App  Version  Status  Scale  Charm  Store  Rev  OS  Notes
foo                     0/2                  0      

Unit   Workload  Agent  Machine  Public address  Ports  Message
foo/0                                                   
foo/1                                                   

Entity  Meter status  Message
foo/0   strange       warning: stable strangelets  
foo/1   up            things are looking up        
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
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		// And the machine's address is "10.0.0.1"
		setAddresses{"0", network.NewAddresses("10.0.0.1")},
		// And a container is started
		// And the container's ID is "0/lxd/0"
		addContainer{"0", "0/lxd/0", state.JobHostUnits},

		// And the "wordpress" charm is available
		addCharm{"wordpress"},
		addApplication{name: "wordpress", charm: "wordpress"},
		// And the "mysql" charm is available
		addCharm{"mysql"},
		addApplication{name: "mysql", charm: "mysql"},
		// And the "logging" charm is available
		addCharm{"logging"},

		// And a machine is started
		// And the machine's ID is "1"
		// And the machine's job is to host units
		addMachine{machineId: "1", job: state.JobHostUnits},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		// And the machine's address is "10.0.1.1"
		setAddresses{"1", network.NewAddresses("10.0.1.1")},
		// And a unit of "wordpress" is deployed to machine "1"
		addAliveUnit{"wordpress", "1"},
		// And the unit is started
		setAgentStatus{"wordpress/0", status.Idle, "", nil},
		setUnitStatus{"wordpress/0", status.Active, "", nil},
		// And a machine is started

		// And the machine's ID is "2"
		// And the machine's job is to host units
		addMachine{machineId: "2", job: state.JobHostUnits},
		startAliveMachine{"2", ""},
		setMachineStatus{"2", status.Started, ""},
		// And the machine's address is "10.0.2.1"
		setAddresses{"2", network.NewAddresses("10.0.2.1")},
		// And a unit of "mysql" is deployed to machine "2"
		addAliveUnit{"mysql", "2"},
		// And the unit is started
		setAgentStatus{"mysql/0", status.Idle, "", nil},
		setUnitStatus{"mysql/0", status.Active, "", nil},
		// And the "logging" application is added
		addApplication{name: "logging", charm: "logging"},
		// And the application is exposed
		setApplicationExposed{"logging", true},
		// And the "wordpress" application is related to the "mysql" application
		relateApplications{"wordpress", "mysql", ""},
		// And the "wordpress" application is related to the "logging" application
		relateApplications{"wordpress", "logging", ""},
		// And the "mysql" application is related to the "logging" application
		relateApplications{"mysql", "logging", ""},
		// And the "logging" application is a subordinate to unit 0 of the "wordpress" application
		addSubordinate{"wordpress/0", "logging"},
		setAgentStatus{"logging/0", status.Idle, "", nil},
		setUnitStatus{"logging/0", status.Active, "", nil},
		// And the "logging" application is a subordinate to unit 0 of the "mysql" application
		addSubordinate{"mysql/0", "logging"},
		setAgentStatus{"logging/1", status.Idle, "", nil},
		setUnitStatus{"logging/1", status.Active, "", nil},
		setUnitsAlive{"logging"},
	}

	ctx.run(c, steps)
	return ctx
}

// Scenario: One unit is in an errored state and user filters to active
func (s *StatusSuite) TestFilterToActive(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	// Given unit 1 of the "logging" application has an error
	setAgentStatus{"logging/1", status.Error, "mock error", nil}.step(c, ctx)
	// And unit 0 of the "mysql" application has an error
	setAgentStatus{"mysql/0", status.Error, "mock error", nil}.step(c, ctx)
	// When I run juju status --format oneline started
	_, stdout, stderr := runStatus(c, "--format", "oneline", "active")
	c.Assert(string(stderr), gc.Equals, "")
	// Then I should receive output prefixed with:
	const expected = `

- wordpress/0: 10.0.1.1 (agent:idle, workload:active)
  - logging/0: 10.0.1.1 (agent:idle, workload:active)
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

- wordpress/0: 10.0.1.1 (agent:idle, workload:active)
  - logging/0: 10.0.1.1 (agent:idle, workload:active)
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

	const expected = "" +
		"model:\n" +
		"  name: controller\n" +
		"  type: iaas\n" +
		"  controller: kontroll\n" +
		"  cloud: dummy\n" +
		"  region: dummy-region\n" +
		"  version: 1.2.3\n" +
		"  model-status:\n" +
		"    current: available\n" +
		"    since: 01 Apr 15 01:23+10:00\n" +
		"  sla: unsupported\n" +
		"machines:\n" +
		"  \"0\":\n" +
		"    juju-status:\n" +
		"      current: started\n" +
		"      since: 01 Apr 15 01:23+10:00\n" +
		"    dns-name: 10.0.0.1\n" +
		"    ip-addresses:\n" +
		"    - 10.0.0.1\n" +
		"    instance-id: controller-0\n" +
		"    machine-status:\n" +
		"      current: pending\n" +
		"      since: 01 Apr 15 01:23+10:00\n" +
		"    modification-status:\n" +
		"      current: idle\n" +
		"      since: 01 Apr 15 01:23+10:00\n" +
		"    series: quantal\n" +
		"    network-interfaces:\n" +
		"      eth0:\n" +
		"        ip-addresses:\n" +
		"        - 10.0.0.1\n" +
		"        mac-address: aa:bb:cc:dd:ee:ff\n" +
		"        is-up: true\n" +
		"    containers:\n" +
		"      0/lxd/0:\n" +
		"        juju-status:\n" +
		"          current: pending\n" +
		"          since: 01 Apr 15 01:23+10:00\n" +
		"        instance-id: pending\n" +
		"        machine-status:\n" +
		"          current: pending\n" +
		"          since: 01 Apr 15 01:23+10:00\n" +
		"        modification-status:\n" +
		"          current: idle\n" +
		"          since: 01 Apr 15 01:23+10:00\n" +
		"        series: quantal\n" +
		"    hardware: arch=amd64 cores=1 mem=1024M root-disk=8192M\n" +
		"    controller-member-status: adding-vote\n" +
		"applications: {}\n" +
		"storage: {}\n" +
		"controller:\n" +
		"  timestamp: 15:04:05+07:00\n"

	out := substituteFakeTime(c, "since", stdout, ctx.expectIsoTime)
	out = substituteFakeTimestamp(c, out, ctx.expectIsoTime)
	c.Assert(string(out), gc.Equals, expected)
}

// Scenario: One unit is in an errored state and user filters to errored
func (s *StatusSuite) TestFilterToErrored(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	// Given unit 1 of the "logging" application has an error
	setAgentStatus{"logging/1", status.Error, "mock error", nil}.step(c, ctx)
	// When I run juju status --format oneline error
	_, stdout, stderr := runStatus(c, "--format", "oneline", "error")
	c.Assert(stderr, gc.IsNil)
	// Then I should receive output prefixed with:
	const expected = `

- mysql/0: 10.0.2.1 (agent:idle, workload:active)
  - logging/1: 10.0.2.1 (agent:idle, workload:error)
`
	c.Assert(string(stdout), gc.Equals, expected[1:])
}

// Scenario: User filters to mysql application
func (s *StatusSuite) TestFilterToApplication(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	// When I run juju status --format oneline error
	_, stdout, stderr := runStatus(c, "--format", "oneline", "mysql")
	c.Assert(stderr, gc.IsNil)
	// Then I should receive output prefixed with:
	const expected = `

- mysql/0: 10.0.2.1 (agent:idle, workload:active)
  - logging/1: 10.0.2.1 (agent:idle, workload:active)
`

	c.Assert(string(stdout), gc.Equals, expected[1:])
}

// Scenario: User filters to exposed applications
func (s *StatusSuite) TestFilterToExposedApplication(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	// Given unit 1 of the "mysql" application is exposed
	setApplicationExposed{"mysql", true}.step(c, ctx)
	// And the logging application is not exposed
	setApplicationExposed{"logging", false}.step(c, ctx)
	// And the wordpress application is not exposed
	setApplicationExposed{"wordpress", false}.step(c, ctx)
	// When I run juju status --format oneline exposed
	_, stdout, stderr := runStatus(c, "--format", "oneline", "exposed")
	c.Assert(stderr, gc.IsNil)
	// Then I should receive output prefixed with:
	const expected = `

- mysql/0: 10.0.2.1 (agent:idle, workload:active)
  - logging/1: 10.0.2.1 (agent:idle, workload:active)
`
	c.Assert(string(stdout), gc.Equals, expected[1:])
}

// Scenario: User filters to non-exposed applications
func (s *StatusSuite) TestFilterToNotExposedApplication(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	setApplicationExposed{"mysql", true}.step(c, ctx)
	// When I run juju status --format oneline not exposed
	_, stdout, stderr := runStatus(c, "--format", "oneline", "not", "exposed")
	c.Assert(stderr, gc.IsNil)
	// Then I should receive output prefixed with:
	const expected = `

- wordpress/0: 10.0.1.1 (agent:idle, workload:active)
  - logging/0: 10.0.1.1 (agent:idle, workload:active)
`
	c.Assert(string(stdout), gc.Equals, expected[1:])
}

// Scenario: Filtering on Subnets
func (s *StatusSuite) TestFilterOnSubnet(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	// Given the address for machine "1" is "localhost"
	setAddresses{"1", network.NewAddresses("localhost", "127.0.0.1")}.step(c, ctx)
	// And the address for machine "2" is "10.0.2.1"
	setAddresses{"2", network.NewAddresses("10.0.2.1")}.step(c, ctx)
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
	// And the address for machine "2" is "10.0.2.1"
	setAddresses{"2", network.NewAddresses("10.0.2.1")}.step(c, ctx)
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

- mysql/0: 10.0.2.1 (agent:idle, workload:active)
  - logging/1: 10.0.2.1 (agent:idle, workload:active)
- wordpress/0: 10.0.1.1 (agent:idle, workload:active)
  - logging/0: 10.0.1.1 (agent:idle, workload:active)
`
	c.Assert(string(stdout), gc.Equals, expected[1:])
}

// Scenario: User filters out a subordinate, but not its parent
func (s *StatusSuite) TestFilterSubordinateButNotParent(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	// Given the wordpress application is exposed
	setApplicationExposed{"wordpress", true}.step(c, ctx)
	// When I run juju status --format oneline not exposed
	_, stdout, stderr := runStatus(c, "--format", "oneline", "not", "exposed")
	c.Assert(stderr, gc.IsNil)
	// Then I should receive output prefixed with:
	const expected = `

- mysql/0: 10.0.2.1 (agent:idle, workload:active)
  - logging/1: 10.0.2.1 (agent:idle, workload:active)
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

- mysql/0: 10.0.2.1 (agent:idle, workload:active)
  - logging/1: 10.0.2.1 (agent:idle, workload:active)
- wordpress/0: 10.0.1.1 (agent:idle, workload:active)
  - logging/0: 10.0.1.1 (agent:idle, workload:active)
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

- mysql/0: 10.0.2.1 (agent:idle, workload:active)
  - logging/1: 10.0.2.1 (agent:idle, workload:active)
- wordpress/0: 10.0.1.1 (agent:idle, workload:active)
  - logging/0: 10.0.1.1 (agent:idle, workload:active)
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
	return com, cmdtesting.InitCommand(modelcmd.Wrap(com), args)
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
	setAddresses{"0", network.NewAddresses("10.0.0.1")},
	startAliveMachine{"0", ""},
	setMachineStatus{"0", status.Started, ""},
	addCharm{"dummy"},
	addApplication{name: "dummy-application", charm: "dummy"},

	addMachine{machineId: "1", job: state.JobHostUnits},
	startAliveMachine{"1", ""},
	setAddresses{"1", network.NewAddresses("10.0.1.1")},
	setMachineStatus{"1", status.Started, ""},

	addAliveUnit{"dummy-application", "1"},
	expect{
		what: "add two units, one alive (in error state), one started",
		output: M{
			"model": M{
				"name":       "controller",
				"type":       "iaas",
				"controller": "kontroll",
				"cloud":      "dummy",
				"region":     "dummy-region",
				"version":    "1.2.3",
				"model-status": M{
					"current": "available",
					"since":   "01 Apr 15 01:23+10:00",
				},
				"sla": "unsupported",
			},
			"machines": M{
				"0": machine0,
				"1": machine1,
			},
			"applications": M{
				"dummy-application": dummyCharm(M{
					"application-status": M{
						"current": "waiting",
						"message": "waiting for machine",
						"since":   "01 Apr 15 01:23+10:00",
					},
					"units": M{
						"dummy-application/0": M{
							"machine": "1",
							"workload-status": M{
								"current": "waiting",
								"message": "waiting for machine",
								"since":   "01 Apr 15 01:23+10:00",
							},
							"juju-status": M{
								"current": "allocating",
								"since":   "01 Apr 15 01:23+10:00",
							},
							"public-address": "10.0.1.1",
						},
					},
				}),
			},
			"storage": M{},
			"controller": M{
				"timestamp": "15:04:05",
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
	now := time.Now()
	status := &params.FullStatus{
		Model: params.ModelStatusInfo{
			CloudTag: "cloud-dummy",
		},
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
		ControllerTimestamp: &now,
	}
	isoTime := true
	formatter := NewStatusFormatter(status, isoTime)
	formatted, err := formatter.format()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(formatted, jc.DeepEquals, formattedStatus{
		Model: modelStatus{
			Cloud: "dummy",
		},
		Machines: map[string]machineStatus{
			"1": {
				JujuStatus:        statusInfoContents{Current: "error", Message: "<error while provisioning>"},
				InstanceId:        "pending",
				Series:            "trusty",
				Id:                "1",
				Containers:        map[string]machineStatus{},
				NetworkInterfaces: map[string]networkInterface{},
				LXDProfiles:       map[string]lxdProfileContents{},
			},
		},
		Applications:       map[string]applicationStatus{},
		RemoteApplications: map[string]remoteApplicationStatus{},
		Offers:             map[string]offerStatus{},
		Controller: &controllerStatus{
			Timestamp: common.FormatTimeAsTimestamp(&now, isoTime),
		},
	})
}

func (s *StatusSuite) TestMissingControllerTimestampInFullStatus(c *gc.C) {
	status := &params.FullStatus{
		Model: params.ModelStatusInfo{
			CloudTag: "cloud-dummy",
		},
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
	isoTime := true
	formatter := NewStatusFormatter(status, isoTime)
	formatted, err := formatter.format()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(formatted, jc.DeepEquals, formattedStatus{
		Model: modelStatus{
			Cloud: "dummy",
		},
		Machines: map[string]machineStatus{
			"1": {
				JujuStatus:        statusInfoContents{Current: "error", Message: "<error while provisioning>"},
				InstanceId:        "pending",
				Series:            "trusty",
				Id:                "1",
				Containers:        map[string]machineStatus{},
				NetworkInterfaces: map[string]networkInterface{},
				LXDProfiles:       map[string]lxdProfileContents{},
			},
		},
		Applications:       map[string]applicationStatus{},
		RemoteApplications: map[string]remoteApplicationStatus{},
		Offers:             map[string]offerStatus{},
	})
}

func (s *StatusSuite) TestControllerTimestampInFullStatus(c *gc.C) {
	now := time.Now()
	status := &params.FullStatus{
		Model: params.ModelStatusInfo{
			CloudTag: "cloud-dummy",
		},
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
		ControllerTimestamp: &now,
	}
	isoTime := true
	formatter := NewStatusFormatter(status, isoTime)
	formatted, err := formatter.format()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(formatted, jc.DeepEquals, formattedStatus{
		Model: modelStatus{
			Cloud: "dummy",
		},
		Machines: map[string]machineStatus{
			"1": {
				JujuStatus:        statusInfoContents{Current: "error", Message: "<error while provisioning>"},
				InstanceId:        "pending",
				Series:            "trusty",
				Id:                "1",
				Containers:        map[string]machineStatus{},
				NetworkInterfaces: map[string]networkInterface{},
				LXDProfiles:       map[string]lxdProfileContents{},
			},
		},
		Applications:       map[string]applicationStatus{},
		RemoteApplications: map[string]remoteApplicationStatus{},
		Offers:             map[string]offerStatus{},
		Controller: &controllerStatus{
			Timestamp: common.FormatTimeAsTimestamp(&now, isoTime),
		},
	})
}

func (s *StatusSuite) TestTabularNoRelations(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	_, stdout, stderr := runStatus(c)
	c.Assert(stderr, gc.IsNil)
	c.Assert(strings.Contains(string(stdout), "Relation provider"), jc.IsFalse)
}

func (s *StatusSuite) TestTabularDisplayRelations(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	_, stdout, stderr := runStatus(c, "--relations")
	c.Assert(stderr, gc.IsNil)
	c.Assert(strings.Contains(string(stdout), "Relation provider"), jc.IsTrue)
}

func (s *StatusSuite) TestNonTabularDisplayRelations(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	_, stdout, stderr := runStatus(c, "--format=yaml", "--relations")
	c.Assert(string(stderr), gc.Equals, "provided relations option is always enabled in non tabular formats\n")
	logger.Debugf("stdout -> \n%q", stdout)
	c.Assert(strings.Contains(string(stdout), "    relations:"), jc.IsTrue)
	c.Assert(strings.Contains(string(stdout), "storage:"), jc.IsTrue)
}

func (s *StatusSuite) TestNonTabularDisplayStorage(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	_, stdout, stderr := runStatus(c, "--format=yaml", "--storage")
	c.Assert(string(stderr), gc.Equals, "provided storage option is always enabled in non tabular formats\n")
	c.Assert(strings.Contains(string(stdout), "    relations:"), jc.IsTrue)
	c.Assert(strings.Contains(string(stdout), "storage:"), jc.IsTrue)
}

func (s *StatusSuite) TestNonTabularDisplayRelationsAndStorage(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	_, stdout, stderr := runStatus(c, "--format=yaml", "--relations", "--storage")
	c.Assert(string(stderr), gc.Equals, "provided relations, storage options are always enabled in non tabular formats\n")
	c.Assert(strings.Contains(string(stdout), "    relations:"), jc.IsTrue)
	c.Assert(strings.Contains(string(stdout), "storage:"), jc.IsTrue)
}

func (s *StatusSuite) TestNonTabularRelations(c *gc.C) {
	ctx := s.FilteringTestSetup(c)
	defer s.resetContext(c, ctx)

	_, stdout, stderr := runStatus(c, "--format=yaml")
	c.Assert(stderr, gc.IsNil)
	c.Assert(strings.Contains(string(stdout), "    relations:"), jc.IsTrue)
	c.Assert(strings.Contains(string(stdout), "storage:"), jc.IsTrue)
}

func (s *StatusSuite) TestStatusFormatTabularEmptyModel(c *gc.C) {
	code, stdout, stderr := runStatus(c)
	c.Check(code, gc.Equals, 0)
	c.Check(string(stderr), gc.Equals, "Model \"controller\" is empty.\n")
	expected := `
Model       Controller  Cloud/Region        Version  SLA          Timestamp
controller  kontroll    dummy/dummy-region  1.2.3    unsupported  15:04:05+07:00

`[1:]
	output := substituteFakeTimestamp(c, stdout, false)
	c.Assert(string(output), gc.Equals, expected)
}

func (s *StatusSuite) TestStatusFormatTabularForUnmatchedFilter(c *gc.C) {
	code, stdout, stderr := runStatus(c, "unmatched")
	c.Check(code, gc.Equals, 0)
	c.Check(string(stderr), gc.Equals, "Nothing matched specified filter.\n")
	expected := `
Model       Controller  Cloud/Region        Version  SLA          Timestamp
controller  kontroll    dummy/dummy-region  1.2.3    unsupported  15:04:05+07:00

`[1:]
	output := substituteFakeTimestamp(c, stdout, false)
	c.Assert(string(output), gc.Equals, expected)

	_, _, stderr = runStatus(c, "cannot", "match", "me")
	c.Check(string(stderr), gc.Equals, "Nothing matched specified filters.\n")
}
