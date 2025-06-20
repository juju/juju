// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/kr/pretty"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testcharms"
)

var (
	currentVersion = semversion.Number{Major: 1, Minor: 2, Patch: 3}
	nextVersion    = semversion.Number{Major: 1, Minor: 2, Patch: 4}
)

func runStatus(c *tc.C, testCtx *ctx, args ...string) (code int, stdout, stderr string) {
	ctx := cmdtesting.Context(c)
	code = cmd.Main(NewStatusCommandForTest(testCtx.store, testCtx.api, clock.WallClock), ctx, args)
	stdout = ctx.Stdout.(*bytes.Buffer).String()
	stderr = ctx.Stderr.(*bytes.Buffer).String()
	return
}

type StatusSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	store *jujuclient.MemStore
}

func TestStatusSuite(t *testing.T) {
	tc.Run(t, &StatusSuite{})
}

func (s *StatusSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "kontroll"
	store.Controllers["kontroll"] = jujuclient.ControllerDetails{}
	store.Models["kontroll"] = &jujuclient.ControllerModels{
		CurrentModel: "controller",
		Models: map[string]jujuclient.ModelDetails{
			"admin/controller": {
				ModelType: coremodel.IAAS,
			},
		},
	}
	store.Accounts["kontroll"] = jujuclient.AccountDetails{
		User: "admin",
	}
	s.store = store
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
	step(c *tc.C, ctx *ctx)
}

//
// ctx
//

type charmInfo struct {
	charm charm.Charm
	url   string
}

type ctx struct {
	expectIsoTime    bool
	spaceName        string
	charms           map[string]charmInfo
	remoteProxies    map[string]params.RemoteApplicationStatus
	subordinateApps  map[string]*params.ApplicationStatus
	subordinateUnits map[string]int
	nextinstanceId   int

	store *jujuclient.MemStore
	api   *fakeStatusAPI
}

func (ctx *ctx) run(c *tc.C, steps []stepper) {
	for i, s := range steps {
		c.Logf("step %d", i)
		c.Logf("%#v", s)
		s.step(c, ctx)
	}
}

func (s *StatusSuite) newContext() *ctx {
	now := time.Now()
	return &ctx{
		charms:           make(map[string]charmInfo),
		subordinateApps:  make(map[string]*params.ApplicationStatus),
		subordinateUnits: make(map[string]int),
		remoteProxies:    make(map[string]params.RemoteApplicationStatus),
		store:            s.store,
		api: &fakeStatusAPI{
			result: &params.FullStatus{
				ControllerTimestamp: &now,
				Model: params.ModelStatusInfo{
					Name:        "controller",
					Type:        "iaas",
					CloudTag:    "cloud-dummy",
					CloudRegion: "dummy-region",
					Version:     currentVersion.String(),
					ModelStatus: params.DetailedStatus{
						Status: status.Available.String(),
						Since:  &now,
					},
				},
				Machines:           make(map[string]params.MachineStatus),
				Applications:       make(map[string]params.ApplicationStatus),
				Offers:             make(map[string]params.ApplicationOfferStatus),
				RemoteApplications: make(map[string]params.RemoteApplicationStatus),
			},
		},
	}
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
	}

	machine0 = M{
		"juju-status": M{
			"current": "started",
			"since":   "01 Apr 15 01:23+10:00",
		},
		// In this scenario machine0 runs an older agent version that
		// does not support reporting of host names. Since the hostname
		// is blank it will be skipped.
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
		"base": M{"name": "ubuntu", "channel": "12.10"},
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
		"hostname":     "eldritch-octopii",
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
		"base": M{"name": "ubuntu", "channel": "12.10"},
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
		"hostname":     "eldritch-octopii",
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
		"base": M{"name": "ubuntu", "channel": "12.10"},
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
		"hostname":     "titanium-shoelace",
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
		"base": M{"name": "ubuntu", "channel": "12.10"},
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
		"hostname":     "loud-silence",
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
		"base": M{"name": "ubuntu", "channel": "12.10"},
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
		"hostname":     "antediluvian-furniture",
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
		"base": M{"name": "ubuntu", "channel": "12.10"},
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
						"instance-id":  "controller-4",
						"machine-status": M{
							"current": "pending",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"modification-status": M{
							"current": "idle",
							"since":   "01 Apr 15 01:23+10:00",
						},
						"base": M{"name": "ubuntu", "channel": "12.10"},
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
				"instance-id":  "controller-3",
				"machine-status": M{
					"current": "pending",
					"since":   "01 Apr 15 01:23+10:00",
				},
				"modification-status": M{
					"current": "idle",
					"since":   "01 Apr 15 01:23+10:00",
				},
				"base": M{"name": "ubuntu", "channel": "12.10"},
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
				"base": M{"name": "ubuntu", "channel": "12.10"},
			},
		},
		"hostname":     "eldritch-octopii",
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
		"base": M{"name": "ubuntu", "channel": "12.10"},
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
			"current": "unknown",
			"since":   "01 Apr 15 01:23+10:00",
		},
	})
	exposedApplication = dummyCharm(M{
		"application-status": M{
			"current": "unknown",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"exposed": true,
	})
	loggingCharm = M{
		"charm":         "logging",
		"charm-origin":  "charmhub",
		"charm-name":    "logging",
		"charm-rev":     1,
		"charm-channel": "stable",
		"base":          M{"name": "ubuntu", "channel": "12.10"},
		"exposed":       true,
		"scale":         2,
		"application-status": M{
			"current": "error",
			"message": "somehow lost in all those logs",
			"since":   "01 Apr 15 01:23+10:00",
		},
		"relations": M{
			"logging-directory": L{
				M{
					"interface":           "logging",
					"related-application": "wordpress",
					"scope":               "container",
				},
			},
			"info": L{
				M{
					"interface":           relation.JujuInfo,
					"related-application": "mysql",
					"scope":               "container",
				},
			},
		},
		"endpoint-bindings": M{
			"":                  network.AlphaSpaceName,
			"info":              network.AlphaSpaceName,
			"logging-client":    network.AlphaSpaceName,
			"logging-directory": network.AlphaSpaceName,
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
	//Status tests
	test( // 0
		"bootstrap and starting a single instance",

		addMachine{machineId: "0", job: coremodel.JobManageModel},
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
						"base":                     M{"name": "ubuntu", "channel": "12.10"},
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
		setAddresses{"0", []network.SpaceAddress{
			network.NewSpaceAddress("10.0.0.1", network.WithScope(network.ScopePublic)),
			network.NewSpaceAddress("10.0.0.2"),
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
						"base": M{"name": "ubuntu", "channel": "12.10"},
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
						"base": M{"name": "ubuntu", "channel": "12.10"},
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

		setTools{"0", semversion.MustParseBinary("1.2.3-ubuntu-ppc")},
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
						"base": M{"name": "ubuntu", "channel": "12.10"},
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
		addMachine{machineId: "0", cons: machineCons, job: coremodel.JobManageModel},
		setAddresses{"0", []network.SpaceAddress{
			network.NewSpaceAddress("10.0.0.1", network.WithScope(network.ScopePublic)),
			network.NewSpaceAddress("10.0.0.2"),
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
						"base": M{"name": "ubuntu", "channel": "12.10"},
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
		addMachine{machineId: "0", cons: machineCons, job: coremodel.JobManageModel},
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
						"base":                     M{"name": "ubuntu", "channel": "12.10"},
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
		addMachine{machineId: "0", job: coremodel.JobManageModel},
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
						"base":                     M{"name": "ubuntu", "channel": "12.10"},
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
						"base":                     M{"name": "ubuntu", "channel": "12.10"},
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
		addMachine{machineId: "0", job: coremodel.JobManageModel},
		setAddresses{"0", network.NewSpaceAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addCharmHubCharm{"dummy"},
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
		addMachine{machineId: "1", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "1", hostname: "eldritch-octopii"},
		setAddresses{"1", network.NewSpaceAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addMachine{machineId: "2", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "2", hostname: "titanium-shoelace"},
		setAddresses{"2", network.NewSpaceAddresses("10.0.2.1")},
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
		addMachine{machineId: "3", job: coremodel.JobHostUnits},
		startMachine{"3"},
		recordAgentStartInformation{machineId: "3", hostname: "loud-silence"},
		// Simulate some status with info, while the agent is down.
		setAddresses{"3", network.NewSpaceAddresses("10.0.3.1")},
		setMachineStatus{"3", status.Stopped, "Really?"},
		addMachine{machineId: "4", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "4", hostname: "antediluvian-furniture"},
		setAddresses{"4", network.NewSpaceAddresses("10.0.4.1")},
		startAliveMachine{"4", ""},
		setMachineStatus{"4", status.Error, "Beware the red toys"},
		addMachine{machineId: "5", job: coremodel.JobHostUnits},
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
						"hostname":     "loud-silence",
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
						"base": M{"name": "ubuntu", "channel": "12.10"},
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
						"hostname":     "antediluvian-furniture",
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
						"base": M{"name": "ubuntu", "channel": "12.10"},
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
						"base": M{"name": "ubuntu", "channel": "12.10"},
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
	),
	test( // 5
		"a unit with a hook relation error",
		addMachine{machineId: "0", job: coremodel.JobManageModel},
		setAddresses{"0", network.NewSpaceAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},

		addMachine{machineId: "1", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "1", hostname: "eldritch-octopii"},
		setAddresses{"1", network.NewSpaceAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},

		addCharmHubCharm{"wordpress"},
		addApplication{name: "wordpress", charm: "wordpress"},
		addAliveUnit{"wordpress", "1"},

		addCharmHubCharm{"mysql"},
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
							"db": L{
								M{
									"interface":           "mysql",
									"related-application": "mysql",
									"scope":               "global",
								},
							},
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
							"":                network.AlphaSpaceName,
							"logging-dir":     network.AlphaSpaceName,
							"monitoring-port": network.AlphaSpaceName,
							"url":             network.AlphaSpaceName,
							"admin-api":       network.AlphaSpaceName,
							"cache":           network.AlphaSpaceName,
							"db":              network.AlphaSpaceName,
							"db-client":       network.AlphaSpaceName,
							"foo-bar":         network.AlphaSpaceName,
						},
					}),
					"mysql": mysqlCharm(M{
						"relations": M{
							"server": L{
								M{
									"interface":           "mysql",
									"related-application": "wordpress",
									"scope":               "global",
								},
							},
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
							"":               network.AlphaSpaceName,
							"server":         network.AlphaSpaceName,
							"server-admin":   network.AlphaSpaceName,
							"metrics-client": network.AlphaSpaceName,
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
		addMachine{machineId: "0", job: coremodel.JobManageModel},
		setAddresses{"0", network.NewSpaceAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},

		addMachine{machineId: "1", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "1", hostname: "eldritch-octopii"},
		setAddresses{"1", network.NewSpaceAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},

		addCharmHubCharm{"wordpress"},
		addApplication{name: "wordpress", charm: "wordpress"},
		addAliveUnit{"wordpress", "1"},

		addCharmHubCharm{"mysql"},
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
							"db": L{
								M{
									"interface":           "mysql",
									"related-application": "mysql",
									"scope":               "global",
								},
							},
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
							"":                network.AlphaSpaceName,
							"admin-api":       network.AlphaSpaceName,
							"cache":           network.AlphaSpaceName,
							"db":              network.AlphaSpaceName,
							"db-client":       network.AlphaSpaceName,
							"foo-bar":         network.AlphaSpaceName,
							"logging-dir":     network.AlphaSpaceName,
							"monitoring-port": network.AlphaSpaceName,
							"url":             network.AlphaSpaceName,
						},
					}),
					"mysql": mysqlCharm(M{
						"relations": M{
							"server": L{
								M{
									"interface":           "mysql",
									"related-application": "wordpress",
									"scope":               "global",
								},
							},
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
							"":               network.AlphaSpaceName,
							"server":         network.AlphaSpaceName,
							"server-admin":   network.AlphaSpaceName,
							"metrics-client": network.AlphaSpaceName,
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
		addCharmHubCharm{"dummy"},
		addApplication{name: "dummy-application", charm: "dummy"},
		addMachine{machineId: "0", job: coremodel.JobHostUnits},
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
						"base": M{"name": "ubuntu", "channel": "12.10"},
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
		addCharmHubCharm{"dummy"},
		addApplication{name: "dummy-application", charm: "dummy"},
		addMachine{machineId: "0", job: coremodel.JobHostUnits},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addUnit{"dummy-application", "0"},
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
						"base":     M{"name": "ubuntu", "channel": "12.10"},
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
		addMachine{machineId: "0", job: coremodel.JobManageModel},
		setAddresses{"0", network.NewSpaceAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addCharmHubCharm{"wordpress"},
		addCharmHubCharm{"mysql"},
		addCharmHubCharm{"varnish"},

		addApplication{name: "project", charm: "wordpress"},
		setApplicationExposed{"project", true},
		addMachine{machineId: "1", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "1", hostname: "eldritch-octopii"},
		setAddresses{"1", network.NewSpaceAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addAliveUnit{"project", "1"},
		setAgentStatus{"project/0", status.Idle, "", nil},
		setUnitStatus{"project/0", status.Active, "", nil},

		addApplication{name: "mysql", charm: "mysql"},
		setApplicationExposed{"mysql", true},
		addMachine{machineId: "2", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "2", hostname: "titanium-shoelace"},
		setAddresses{"2", network.NewSpaceAddresses("10.0.2.1")},
		startAliveMachine{"2", ""},
		setMachineStatus{"2", status.Started, ""},
		addAliveUnit{"mysql", "2"},
		setAgentStatus{"mysql/0", status.Idle, "", nil},
		setUnitStatus{"mysql/0", status.Active, "", nil},

		addApplication{name: "varnish", charm: "varnish"},
		setApplicationExposed{"varnish", true},
		addMachine{machineId: "3", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "3", hostname: "loud-silence"},
		setAddresses{"3", network.NewSpaceAddresses("10.0.3.1")},
		startAliveMachine{"3", ""},
		setMachineStatus{"3", status.Started, ""},
		setMachineInstanceStatus{"3", status.Started, "I am number three"},
		addAliveUnit{"varnish", "3"},

		addApplication{name: "private", charm: "wordpress"},
		setApplicationExposed{"private", true},
		addMachine{machineId: "4", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "4", hostname: "antediluvian-furniture"},
		setAddresses{"4", network.NewSpaceAddresses("10.0.4.1")},
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
							"":                network.AlphaSpaceName,
							"db":              network.AlphaSpaceName,
							"db-client":       network.AlphaSpaceName,
							"foo-bar":         network.AlphaSpaceName,
							"logging-dir":     network.AlphaSpaceName,
							"monitoring-port": network.AlphaSpaceName,
							"url":             network.AlphaSpaceName,
							"admin-api":       network.AlphaSpaceName,
							"cache":           network.AlphaSpaceName,
						},
						"relations": M{
							"db": L{
								M{
									"interface":           "mysql",
									"related-application": "mysql",
									"scope":               "global",
								},
							},
							"cache": L{
								M{
									"interface":           "varnish",
									"related-application": "varnish",
									"scope":               "global",
								},
							},
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
							"":               network.AlphaSpaceName,
							"server":         network.AlphaSpaceName,
							"server-admin":   network.AlphaSpaceName,
							"metrics-client": network.AlphaSpaceName,
						},
						"relations": M{
							"server": L{
								M{
									"interface":           "mysql",
									"related-application": "private",
									"scope":               "global",
								},
								M{
									"interface":           "mysql",
									"related-application": "project",
									"scope":               "global",
								},
							},
						},
					}),
					"varnish": M{
						"charm":         "varnish",
						"charm-origin":  "charmhub",
						"charm-name":    "varnish",
						"charm-channel": "stable",
						"charm-rev":     1,
						"base":          M{"name": "ubuntu", "channel": "12.10"},
						"exposed":       true,
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
							"":         network.AlphaSpaceName,
							"webcache": network.AlphaSpaceName,
						},
						"relations": M{
							"webcache": L{
								M{
									"interface":           "varnish",
									"related-application": "project",
									"scope":               "global",
								},
							},
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
							"":                network.AlphaSpaceName,
							"logging-dir":     network.AlphaSpaceName,
							"monitoring-port": network.AlphaSpaceName,
							"url":             network.AlphaSpaceName,
							"admin-api":       network.AlphaSpaceName,
							"cache":           network.AlphaSpaceName,
							"db":              network.AlphaSpaceName,
							"db-client":       network.AlphaSpaceName,
							"foo-bar":         network.AlphaSpaceName,
						},
						"relations": M{
							"db": L{
								M{
									"interface":           "mysql",
									"related-application": "mysql",
									"scope":               "global",
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
	test( // 10
		"simple peer scenario with leader",
		addMachine{machineId: "0", job: coremodel.JobManageModel},
		setAddresses{"0", network.NewSpaceAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addCharmHubCharm{"riak"},
		addCharmHubCharm{"wordpress"},

		addApplication{name: "riak", charm: "riak"},
		setApplicationExposed{"riak", true},
		addMachine{machineId: "1", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "1", hostname: "eldritch-octopii"},
		setAddresses{"1", network.NewSpaceAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addAliveUnit{"riak", "1"},
		setAgentStatus{"riak/0", status.Idle, "", nil},
		setUnitStatus{"riak/0", status.Active, "", nil},
		addMachine{machineId: "2", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "2", hostname: "titanium-shoelace"},
		setAddresses{"2", network.NewSpaceAddresses("10.0.2.1")},
		startAliveMachine{"2", ""},
		setMachineStatus{"2", status.Started, ""},
		addAliveUnit{"riak", "2"},
		setAgentStatus{"riak/1", status.Idle, "", nil},
		setUnitStatus{"riak/1", status.Active, "", nil},
		addMachine{machineId: "3", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "3", hostname: "loud-silence"},
		setAddresses{"3", network.NewSpaceAddresses("10.0.3.1")},
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
						"charm":         "riak",
						"charm-origin":  "charmhub",
						"charm-name":    "riak",
						"charm-rev":     7,
						"charm-channel": "stable",
						"base":          M{"name": "ubuntu", "channel": "12.10"},
						"exposed":       true,
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
							"":         network.AlphaSpaceName,
							"admin":    network.AlphaSpaceName,
							"endpoint": network.AlphaSpaceName,
							"ring":     network.AlphaSpaceName,
						},
						"relations": M{
							"ring": L{
								M{
									"interface":           "riak",
									"related-application": "riak",
									"scope":               "global",
								},
							},
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
		addMachine{machineId: "0", job: coremodel.JobManageModel},
		setAddresses{"0", network.NewSpaceAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addCharmHubCharm{"wordpress"},
		addCharmHubCharm{"mysql"},
		addCharmHubCharm{"logging"},

		addApplication{name: "wordpress", charm: "wordpress"},
		setApplicationExposed{"wordpress", true},
		addMachine{machineId: "1", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "1", hostname: "eldritch-octopii"},
		setAddresses{"1", network.NewSpaceAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addAliveUnit{"wordpress", "1"},
		setAgentStatus{"wordpress/0", status.Idle, "", nil},
		setUnitStatus{"wordpress/0", status.Active, "", nil},

		addApplication{name: "mysql", charm: "mysql"},
		setApplicationExposed{"mysql", true},
		addMachine{machineId: "2", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "2", hostname: "titanium-shoelace"},
		setAddresses{"2", network.NewSpaceAddresses("10.0.2.1")},
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
							"":                network.AlphaSpaceName,
							"monitoring-port": network.AlphaSpaceName,
							"url":             network.AlphaSpaceName,
							"admin-api":       network.AlphaSpaceName,
							"cache":           network.AlphaSpaceName,
							"db":              network.AlphaSpaceName,
							"db-client":       network.AlphaSpaceName,
							"foo-bar":         network.AlphaSpaceName,
							"logging-dir":     network.AlphaSpaceName,
						},
						"relations": M{
							"db": L{
								M{
									"interface":           "mysql",
									"related-application": "mysql",
									"scope":               "global",
								},
							},
							"logging-dir": L{
								M{
									"interface":           "logging",
									"related-application": "logging",
									"scope":               "container",
								},
							},
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
							"":               network.AlphaSpaceName,
							"server":         network.AlphaSpaceName,
							"server-admin":   network.AlphaSpaceName,
							"metrics-client": network.AlphaSpaceName,
						},
						"relations": M{
							"server": L{
								M{
									"interface":           "mysql",
									"related-application": "wordpress",
									"scope":               "global",
								},
							},
							relation.JujuInfo: L{
								M{
									"interface":           relation.JujuInfo,
									"related-application": "logging",
									"scope":               "container",
								},
							},
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
		addMachine{machineId: "0", job: coremodel.JobManageModel},
		setAddresses{"0", network.NewSpaceAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addCharmHubCharm{"mysql"},
		addApplication{name: "mysql", charm: "mysql"},
		setApplicationExposed{"mysql", true},

		// step 7
		addMachine{machineId: "1", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "1", hostname: "eldritch-octopii"},
		setAddresses{"1", network.NewSpaceAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addAliveUnit{"mysql", "1"},
		setAgentStatus{"mysql/0", status.Idle, "", nil},
		setUnitStatus{"mysql/0", status.Active, "", nil},

		// step 14: A container on machine 1.
		addContainer{"1", "1/lxd/0", coremodel.JobHostUnits},
		setAddresses{"1/lxd/0", network.NewSpaceAddresses("10.0.2.1")},
		startAliveMachine{"1/lxd/0", ""},
		setMachineStatus{"1/lxd/0", status.Started, ""},
		addAliveUnit{"mysql", "1/lxd/0"},
		setAgentStatus{"mysql/1", status.Idle, "", nil},
		setUnitStatus{"mysql/1", status.Active, "", nil},
		addContainer{"1", "1/lxd/1", coremodel.JobHostUnits},

		// step 22: A nested container.
		addContainer{"1/lxd/0", "1/lxd/0/lxd/0", coremodel.JobHostUnits},
		setAddresses{"1/lxd/0/lxd/0", network.NewSpaceAddresses("10.0.3.1")},
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
							"":               network.AlphaSpaceName,
							"server":         network.AlphaSpaceName,
							"server-admin":   network.AlphaSpaceName,
							"metrics-client": network.AlphaSpaceName,
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
		addMachine{machineId: "0", job: coremodel.JobManageModel},
		setAddresses{"0", network.NewSpaceAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addMachine{machineId: "1", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "1", hostname: "eldritch-octopii"},
		setAddresses{"1", network.NewSpaceAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addCharmHubCharm{"mysql"},
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
						"can-upgrade-to": "ch:mysql-23",
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
							"":               network.AlphaSpaceName,
							"server":         network.AlphaSpaceName,
							"server-admin":   network.AlphaSpaceName,
							"metrics-client": network.AlphaSpaceName,
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
		"unit with out of date charm, switching from repo to local",
		addMachine{machineId: "0", job: coremodel.JobManageModel},
		setAddresses{"0", network.NewSpaceAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addMachine{machineId: "1", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "1", hostname: "eldritch-octopii"},
		setAddresses{"1", network.NewSpaceAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addCharmHubCharm{"mysql"},
		addApplication{name: "mysql", charm: "mysql"},
		setApplicationExposed{"mysql", true},
		addAliveUnit{"mysql", "1"},
		setUnitCharmURL{"mysql/0", "ch:mysql-1"},
		addLocalCharmWithRevision{addLocalCharm{"mysql"}, "local", 1},
		setApplicationCharm{"mysql", "local:quantal/mysql-1"},
		setUnitStatus{"mysql/0", status.Active, "", nil},

		expect{
			what: "applications and units with correct charm status",
			output: M{
				"model": model,
				"machines": M{
					"0": machine0,
					"1": machine1,
				},
				"applications": M{
					"mysql": localMysqlCharm(M{
						"charm": "local:quantal/mysql-1",
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
								"upgrading-from": "ch:mysql-1",
								"public-address": "10.0.1.1",
							},
						},
						"endpoint-bindings": M{
							"":               network.AlphaSpaceName,
							"server":         network.AlphaSpaceName,
							"server-admin":   network.AlphaSpaceName,
							"metrics-client": network.AlphaSpaceName,
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
		addMachine{machineId: "0", job: coremodel.JobManageModel},
		setAddresses{"0", network.NewSpaceAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addMachine{machineId: "1", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "1", hostname: "eldritch-octopii"},
		setAddresses{"1", network.NewSpaceAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addCharmHubCharm{"mysql"},
		addApplication{name: "mysql", charm: "mysql"},
		setApplicationExposed{"mysql", true},
		addAliveUnit{"mysql", "1"},
		setUnitCharmURL{"mysql/0", "ch:mysql-1"},
		addCharmHubCharmWithRevision{addCharmHubCharm{"mysql"}, "ch", 2},
		setApplicationCharm{"mysql", "ch:mysql-2"},
		addCharmPlaceholder{"mysql", 23},
		setUnitStatus{"mysql/0", status.Active, "", nil},

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
						"charm-rev":      2,
						"can-upgrade-to": "ch:mysql-23",
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
								"upgrading-from": "ch:mysql-1",
								"public-address": "10.0.1.1",
							},
						},
						"endpoint-bindings": M{
							"":               network.AlphaSpaceName,
							"server":         network.AlphaSpaceName,
							"server-admin":   network.AlphaSpaceName,
							"metrics-client": network.AlphaSpaceName,
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
		addMachine{machineId: "0", job: coremodel.JobManageModel},
		setAddresses{"0", network.NewSpaceAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addMachine{machineId: "1", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "1", hostname: "eldritch-octopii"},
		setAddresses{"1", network.NewSpaceAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addCharmHubCharm{"mysql"},
		addApplication{name: "mysql", charm: "mysql"},
		setApplicationExposed{"mysql", true},
		addAliveUnit{"mysql", "1"},
		setUnitCharmURL{"mysql/0", "ch:mysql-1"},
		addLocalCharmWithRevision{addLocalCharm{"mysql"}, "local", 1},
		setApplicationCharm{"mysql", "local:quantal/mysql-1"},
		addCharmPlaceholder{"mysql", 23},
		setUnitStatus{"mysql/0", status.Active, "", nil},

		expect{
			what: "applications and units with correct charm status",
			output: M{
				"model": model,
				"machines": M{
					"0": machine0,
					"1": machine1,
				},
				"applications": M{
					"mysql": localMysqlCharm(M{
						"charm": "local:quantal/mysql-1",
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
								"upgrading-from": "ch:mysql-1",
								"public-address": "10.0.1.1",
							},
						},
						"endpoint-bindings": M{
							"":               network.AlphaSpaceName,
							"server":         network.AlphaSpaceName,
							"server-admin":   network.AlphaSpaceName,
							"metrics-client": network.AlphaSpaceName,
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
				},
				"machines":     M{},
				"applications": M{},
				"storage":      M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
			stderr: "\nModel \"controller\" is empty.\n",
		},
	),
	test( // 18
		"consistent workload version",
		addMachine{machineId: "0", job: coremodel.JobManageModel},
		setAddresses{"0", network.NewSpaceAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},

		addCharmHubCharm{"mysql"},
		addApplication{name: "mysql", charm: "mysql"},

		addMachine{machineId: "1", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "1", hostname: "eldritch-octopii"},
		setAddresses{"1", network.NewSpaceAddresses("10.0.1.1")},
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
							"":               network.AlphaSpaceName,
							"server":         network.AlphaSpaceName,
							"server-admin":   network.AlphaSpaceName,
							"metrics-client": network.AlphaSpaceName,
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
	test( // 19
		"mixed workload version",
		addMachine{machineId: "0", job: coremodel.JobManageModel},
		setAddresses{"0", network.NewSpaceAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},

		addCharmHubCharm{"mysql"},
		addApplication{name: "mysql", charm: "mysql"},

		addMachine{machineId: "1", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "1", hostname: "eldritch-octopii"},
		setAddresses{"1", network.NewSpaceAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addAliveUnit{"mysql", "1"},
		setUnitWorkloadVersion{"mysql/0", "the best!"},

		addMachine{machineId: "2", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "2", hostname: "titanium-shoelace"},
		setAddresses{"2", network.NewSpaceAddresses("10.0.2.1")},
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
							"":               network.AlphaSpaceName,
							"server":         network.AlphaSpaceName,
							"server-admin":   network.AlphaSpaceName,
							"metrics-client": network.AlphaSpaceName,
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
		"instance with localhost addresses",
		addMachine{machineId: "0", job: coremodel.JobManageModel},
		setAddresses{"0", []network.SpaceAddress{
			network.NewSpaceAddress("10.0.0.1", network.WithScope(network.ScopeCloudLocal)),
			network.NewSpaceAddress("127.0.0.1", network.WithScope(network.ScopeMachineLocal)),
			// TODO(macgreagoir) setAddresses step method needs to
			// set netmask correctly before we can test IPv6
			// loopback.
			// network.NewSpaceAddress("::1", network.ScopeMachineLocal),
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
	test( // 21
		"instance with IPv6 addresses",
		addMachine{machineId: "0", cons: machineCons, job: coremodel.JobManageModel},
		setAddresses{"0", []network.SpaceAddress{
			network.NewSpaceAddress("2001:db8::1", network.WithScope(network.ScopeCloudLocal)),
			// TODO(macgreagoir) setAddresses step method needs to
			// set netmask correctly before we can test IPv6
			// loopback.
			// network.NewSpaceAddress("::1", network.ScopeMachineLocal),
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
						"base": M{"name": "ubuntu", "channel": "12.10"},
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
	test( // 22
		"a remote application",
		addMachine{machineId: "0", job: coremodel.JobManageModel},
		setAddresses{"0", network.NewSpaceAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addMachine{machineId: "1", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "1", hostname: "eldritch-octopii"},
		setAddresses{"1", network.NewSpaceAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},

		addCharmHubCharm{"wordpress"},
		addApplication{name: "wordpress", charm: "wordpress"},
		addAliveUnit{"wordpress", "1"},

		addCharmHubCharm{"mysql"},
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
							"db": L{
								M{
									"interface":           "mysql",
									"related-application": "hosted-mysql",
									"scope":               "global",
								},
							},
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
							"":                network.AlphaSpaceName,
							"monitoring-port": network.AlphaSpaceName,
							"url":             network.AlphaSpaceName,
							"admin-api":       network.AlphaSpaceName,
							"cache":           network.AlphaSpaceName,
							"db":              network.AlphaSpaceName,
							"db-client":       network.AlphaSpaceName,
							"foo-bar":         network.AlphaSpaceName,
							"logging-dir":     network.AlphaSpaceName,
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
	test( // 23
		"deploy application with endpoint bound to space",
		addMachine{machineId: "0", job: coremodel.JobManageModel},
		setAddresses{"0", network.NewSpaceAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addMachine{machineId: "1", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "1", hostname: "eldritch-octopii"},
		setAddresses{"1", network.NewSpaceAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},

		// There is a huge assumption of the spaceID which gets
		// assigned to this space in the scopedExpect section
		// in endpoint-bindings for wordpress.
		addSpace{"myspace1"},

		addCharmHubCharm{"wordpress"},
		addApplication{name: "wordpress", charm: "wordpress", binding: map[string]string{"db-client": "", "logging-dir": "", "cache": "", "db": "myspace1", "monitoring-port": "", "url": "", "admin-api": "", "foo-bar": ""}},
		addAliveUnit{"wordpress", "1"},
	),
	test( // 24
		"application with lxd profiles",
		addMachine{machineId: "0", job: coremodel.JobManageModel},
		setAddresses{"0", network.NewSpaceAddresses("10.0.0.1")},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		addMachine{machineId: "1", job: coremodel.JobHostUnits},
		recordAgentStartInformation{machineId: "1", hostname: "eldritch-octopii"},
		setAddresses{"1", network.NewSpaceAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addCharmHubCharm{"lxd-profile"},
		setCharmProfiles{"1", []string{"juju-controller-lxd-profile-1"}},
		addApplication{name: "lxd-profile", charm: "lxd-profile"},
		setApplicationExposed{"lxd-profile", true},
		addAliveUnit{"lxd-profile", "1"},
		setUnitCharmURL{"lxd-profile/0", "ch:lxd-profile-0"},
		addLocalCharmWithRevision{addLocalCharm{"lxd-profile"}, "local", 1},
		setApplicationCharm{"lxd-profile", "local:quantal/lxd-profile-1"},
		addCharmPlaceholder{"lxd-profile", 23},
		setUnitStatus{"lxd-profile/0", status.Active, "", nil},
		expect{
			what: "applications and units with correct lxd profile charm status",
			output: M{
				"model": model,
				"machines": M{
					"0": machine0,
					"1": machine1WithLXDProfile,
				},
				"applications": M{
					"lxd-profile": M{
						"charm":         "local:quantal/lxd-profile-1",
						"charm-origin":  "local",
						"exposed":       true,
						"charm-name":    "lxd-profile",
						"charm-rev":     1,
						"charm-profile": "juju-controller-lxd-profile-1",
						"base":          M{"name": "ubuntu", "channel": "12.10"},
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
								"upgrading-from": "ch:lxd-profile-0",
								"public-address": "10.0.1.1",
							},
						},
						"endpoint-bindings": M{
							"":        network.AlphaSpaceName,
							"another": network.AlphaSpaceName,
							"ubuntu":  network.AlphaSpaceName,
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
	test( // 25
		"suspended model",
		setModelSuspended{"invalid credential", "bad password"},
		expect{
			what: "suspend a model due to bad credential",
			output: M{
				"model": M{
					"name":       "controller",
					"type":       "iaas",
					"controller": "kontroll",
					"cloud":      "dummy",
					"region":     "dummy-region",
					"version":    "1.2.3",
					"model-status": M{
						"current": "suspended",
						"message": "invalid credential",
						"reason":  "bad password",
						"since":   "01 Apr 15 01:23+10:00",
					},
				},
				"machines":     M{},
				"applications": M{},
				"storage":      M{},
				"controller": M{
					"timestamp": "15:04:05+07:00",
				},
			},
			stderr: "\nModel \"controller\" is empty.\n",
		},
	),
}

func mysqlCharm(extras M) M {
	charm := M{
		"charm":         "mysql",
		"charm-origin":  "charmhub",
		"charm-name":    "mysql",
		"charm-rev":     1,
		"charm-channel": "stable",
		"base":          M{"name": "ubuntu", "channel": "12.10"},
		"exposed":       false,
	}
	return composeCharms(charm, extras)
}

func localMysqlCharm(extras M) M {
	charm := M{
		"charm":        "mysql",
		"charm-origin": "local",
		"charm-name":   "mysql",
		"charm-rev":    1,
		"base":         M{"name": "ubuntu", "channel": "12.10"},
		"exposed":      true,
	}
	return composeCharms(charm, extras)
}

func dummyCharm(extras M) M {
	charm := M{
		"charm":         "dummy",
		"charm-origin":  "charmhub",
		"charm-name":    "dummy",
		"charm-rev":     1,
		"charm-channel": "stable",
		"base":          M{"name": "ubuntu", "channel": "12.10"},
		"exposed":       false,
	}
	return composeCharms(charm, extras)
}

func wordpressCharm(extras M) M {
	charm := M{
		"charm":         "wordpress",
		"charm-origin":  "charmhub",
		"charm-name":    "wordpress",
		"charm-rev":     3,
		"charm-channel": "stable",
		"base":          M{"name": "ubuntu", "channel": "12.10"},
		"exposed":       false,
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

type setModelSuspended struct {
	message string
	reason  string
}

func (s setModelSuspended) step(c *tc.C, ctx *ctx) {
	ctx.api.result.Model.ModelStatus.Status = status.Suspended.String()
	ctx.api.result.Model.ModelStatus.Info = s.message
	ctx.api.result.Model.ModelStatus.Data = map[string]interface{}{"reason": s.reason}
}

type addMachine struct {
	machineId string
	cons      constraints.Value
	job       coremodel.MachineJob
}

func (am addMachine) step(c *tc.C, ctx *ctx) {
	now := time.Now()
	ctx.api.result.Machines[am.machineId] = params.MachineStatus{
		Base:              params.Base{Name: "ubuntu", Channel: "12.10"},
		Id:                am.machineId,
		InstanceId:        "pending",
		Constraints:       am.cons.String(),
		WantsVote:         true,
		Containers:        make(map[string]params.MachineStatus),
		Jobs:              []coremodel.MachineJob{am.job},
		LXDProfiles:       make(map[string]params.LXDProfile),
		NetworkInterfaces: make(map[string]params.NetworkInterface),
		AgentStatus: params.DetailedStatus{
			Status: status.Pending.String(),
			Since:  &now,
		},
		InstanceStatus: params.DetailedStatus{
			Status: status.Pending.String(),
			Since:  &now,
		},
		ModificationStatus: params.DetailedStatus{
			Status: status.Idle.String(),
			Since:  &now,
		},
	}
}

type recordAgentStartInformation struct {
	machineId string
	hostname  string
}

func (ri recordAgentStartInformation) step(c *tc.C, ctx *ctx) {
	m, ok := ctx.api.result.Machines[ri.machineId]
	c.Assert(ok, tc.IsTrue)

	m.DisplayName = fmt.Sprintf("controller-%s", ri.machineId)
	ctx.nextinstanceId++
	m.Hostname = ri.hostname
	m.DisplayName = ""
	if m.Hardware == "" {
		m.Hardware = "arch=amd64 cores=1 mem=1024M root-disk=8192M"
	}
	ctx.api.result.Machines[ri.machineId] = m
}

type addContainer struct {
	parentId  string
	machineId string
	job       coremodel.MachineJob
}

func (ac addContainer) step(c *tc.C, ctx *ctx) {
	m, ok := getMachine(ctx, ac.parentId)
	c.Assert(ok, tc.IsTrue)

	now := time.Now()
	m.Containers[ac.machineId] = params.MachineStatus{
		Base:              params.Base{Name: "ubuntu", Channel: "12.10"},
		Id:                ac.machineId,
		InstanceId:        "pending",
		Containers:        make(map[string]params.MachineStatus),
		Jobs:              []coremodel.MachineJob{ac.job},
		NetworkInterfaces: make(map[string]params.NetworkInterface),
		AgentStatus: params.DetailedStatus{
			Status: status.Pending.String(),
			Since:  &now,
		},
		InstanceStatus: params.DetailedStatus{
			Status: status.Pending.String(),
			Since:  &now,
		},
		ModificationStatus: params.DetailedStatus{
			Status: status.Idle.String(),
			Since:  &now,
		},
	}
	saveMachine(ctx, ac.parentId, m)
}

type startMachine struct {
	machineId string
}

func (sm startMachine) step(c *tc.C, ctx *ctx) {
	m, ok := ctx.api.result.Machines[sm.machineId]
	c.Assert(ok, tc.IsTrue)

	if strings.Contains(sm.machineId, "/") {
		m.InstanceId = instance.Id(fmt.Sprintf("controller-%d", ctx.nextinstanceId))
	} else {
		m.InstanceId = instance.Id(fmt.Sprintf("controller-%s", sm.machineId))
	}
	ctx.nextinstanceId++
	m.DisplayName = string(m.InstanceId)
	ctx.api.result.Machines[sm.machineId] = m
}

type startMissingMachine struct {
	machineId string
}

func (sm startMissingMachine) step(c *tc.C, ctx *ctx) {
	m, ok := ctx.api.result.Machines[sm.machineId]
	c.Assert(ok, tc.IsTrue)

	m.InstanceId = "i-missing"
	m.Hardware = "arch=amd64 cores=1 mem=1024M root-disk=8192M"
	now := time.Now()
	m.InstanceStatus = params.DetailedStatus{
		Status: status.Unknown.String(),
		Info:   "missing",
		Since:  &now,
	}
	ctx.api.result.Machines[sm.machineId] = m
}

type startAliveMachine struct {
	machineId   string
	displayName string
}

func (sam startAliveMachine) step(c *tc.C, ctx *ctx) {
	m, ok := getMachine(ctx, sam.machineId)
	c.Assert(ok, tc.IsTrue)

	if strings.Contains(sam.machineId, "/") {
		m.InstanceId = instance.Id(fmt.Sprintf("controller-%d", ctx.nextinstanceId))
	} else {
		m.InstanceId = instance.Id(fmt.Sprintf("controller-%s", sam.machineId))
	}
	ctx.nextinstanceId++
	m.DisplayName = sam.displayName
	if m.Hardware == "" {
		if m.Constraints != "" {
			hw := constraints.MustParse(m.Constraints)
			if !hw.HasArch() {
				a := "amd64"
				hw.Arch = &a
			}
			m.Hardware = hw.String()
		} else if !strings.Contains(sam.machineId, "/") {
			m.Hardware = "arch=amd64 cores=1 mem=1024M root-disk=8192M"
		}
	}
	saveMachine(ctx, sam.machineId, m)
}

type startMachineWithHardware struct {
	machineId string
	hc        instance.HardwareCharacteristics
}

func (sm startMachineWithHardware) step(c *tc.C, ctx *ctx) {
	m, ok := ctx.api.result.Machines[sm.machineId]
	c.Assert(ok, tc.IsTrue)

	m.InstanceId = instance.Id(fmt.Sprintf("controller-%s", m.Id))
	m.Hardware = sm.hc.String()
	ctx.api.result.Machines[sm.machineId] = m
}

type setMachineInstanceStatus struct {
	machineId string
	Status    status.Status
	Message   string
}

func (sm setMachineInstanceStatus) step(c *tc.C, ctx *ctx) {
	m, ok := ctx.api.result.Machines[sm.machineId]
	c.Assert(ok, tc.IsTrue)

	now := time.Now()
	m.InstanceStatus = params.DetailedStatus{
		Status: sm.Status.String(),
		Info:   sm.Message,
		Since:  &now,
	}
	ctx.api.result.Machines[sm.machineId] = m
}

type setMachineModificationStatus struct {
	machineId string
	Status    status.Status
	Message   string
}

func (sm setMachineModificationStatus) step(c *tc.C, ctx *ctx) {
	m, ok := ctx.api.result.Machines[sm.machineId]
	c.Assert(ok, tc.IsTrue)

	now := time.Now()
	m.ModificationStatus = params.DetailedStatus{
		Status: sm.Status.String(),
		Info:   sm.Message,
		Since:  &now,
	}
	ctx.api.result.Machines[sm.machineId] = m
}

type addSpace struct {
	spaceName string
}

func (sp addSpace) step(c *tc.C, ctx *ctx) {
	ctx.spaceName = sp.spaceName
}

func getMachine(ctx *ctx, machineId string) (params.MachineStatus, bool) {
	parentId := strings.Split(machineId, "/")[0]
	m, ok := ctx.api.result.Machines[parentId]
	if ok && parentId == machineId {
		return m, true
	}
	if !ok {
		return params.MachineStatus{}, false
	}
	rest := machineId
again:
	for {
		parts := strings.Split(rest, "/")
		parentCtrId := ""
		if len(parts) >= 3 {
			parentCtrId = strings.Join(parts[:3], "/")
			rest = strings.Join(parts[2:], "/")
		}
		for _, ctr := range m.Containers {
			if ctr.Id == machineId {
				return ctr, true
			}
			if ctr.Id == parentCtrId {
				m = ctr
				continue again
			}
		}
		return params.MachineStatus{}, false
	}
}

func saveMachine(ctx *ctx, machineId string, update params.MachineStatus) {
	machines := ctx.api.result.Machines
	parts := strings.Split(machineId, "/")
	parentId := parts[0]
	m, ok := machines[parentId]
	if ok && parentId == machineId {
		machines[parentId] = update
		return
	}
	rest := machineId
	machines = m.Containers
again:
	for {
		parts := strings.Split(rest, "/")
		parentCtrId := ""
		if len(parts) >= 3 {
			parentCtrId = strings.Join(parts[:3], "/")
			rest = strings.Join(parts[2:], "/")
		}
		for _, ctr := range machines {
			if ctr.Id == machineId {
				machines[machineId] = update
				return
			}
			if ctr.Id == parentCtrId {
				machines = ctr.Containers
				continue again
			}
		}
		return
	}
}

type setAddresses struct {
	machineId string
	addresses []network.SpaceAddress
}

func (sa setAddresses) step(c *tc.C, ctx *ctx) {
	m, ok := getMachine(ctx, sa.machineId)
	c.Assert(ok, tc.IsTrue)

	m.DNSName = sa.addresses[0].Value
	for _, a := range sa.addresses {
		if a.Scope == network.ScopeMachineLocal ||
			a.Value == "localhost" {
			continue
		}
		m.IPAddresses = append(m.IPAddresses, a.Value)
	}
	for i, address := range sa.addresses {
		if address.Scope == network.ScopeMachineLocal ||
			address.Value == "localhost" {
			continue
		}
		devName := fmt.Sprintf("eth%d", i)
		macAddr := "aa:bb:cc:dd:ee:ff"
		m.NetworkInterfaces[devName] = params.NetworkInterface{
			IPAddresses: []string{address.Value},
			MACAddress:  macAddr,
			Space:       ctx.spaceName,
			IsUp:        true,
		}
	}
	saveMachine(ctx, sa.machineId, m)
}

type setTools struct {
	machineId string
	version   semversion.Binary
}

func (st setTools) step(c *tc.C, ctx *ctx) {
	m, ok := ctx.api.result.Machines[st.machineId]
	c.Assert(ok, tc.IsTrue)

	m.AgentStatus.Version = st.version.Number.String()
	ctx.api.result.Machines[st.machineId] = m
}

type setUnitTools struct {
	unitName string
	version  semversion.Binary
}

func (st setUnitTools) step(c *tc.C, ctx *ctx) {
	u, ok := unitByName(ctx, st.unitName)
	c.Assert(ok, tc.IsTrue)

	u.AgentStatus.Version = st.version.String()
	updateUnit(ctx, st.unitName, &u)
}

type addCharmHubCharm struct {
	name string
}

func (ac addCharmHubCharm) addCharmStep(c *tc.C, ctx *ctx, scheme string, rev int) {
	ch := testcharms.Hub.CharmDir(ac.name)
	name := ch.Meta().Name
	curl := fmt.Sprintf("%s:%s-%d", scheme, name, rev)
	ctx.charms[ac.name] = charmInfo{
		charm: ch,
		url:   curl,
	}
}

func (ac addCharmHubCharm) step(c *tc.C, ctx *ctx) {
	ch := testcharms.Repo.CharmDir(ac.name)
	ac.addCharmStep(c, ctx, "ch", ch.Revision())
}

type addCharmHubCharmWithRevision struct {
	addCharmHubCharm
	scheme string
	rev    int
}

func (ac addCharmHubCharmWithRevision) step(c *tc.C, ctx *ctx) {
	ac.addCharmStep(c, ctx, ac.scheme, ac.rev)
}

type addLocalCharm struct {
	name string
}

func (ac addLocalCharm) addCharmStep(c *tc.C, ctx *ctx, scheme string, rev int) {
	ch := testcharms.Repo.CharmDir(ac.name)
	name := ch.Meta().Name
	curl := fmt.Sprintf("%s:quantal/%s-%d", scheme, name, rev)
	ctx.charms[ac.name] = charmInfo{
		charm: ch,
		url:   curl,
	}
}

func (ac addLocalCharm) step(c *tc.C, ctx *ctx) {
	ch := testcharms.Repo.CharmDir(ac.name)
	ac.addCharmStep(c, ctx, "local", ch.Revision())
}

type addLocalCharmWithRevision struct {
	addLocalCharm
	scheme string
	rev    int
}

func (ac addLocalCharmWithRevision) step(c *tc.C, ctx *ctx) {
	ac.addCharmStep(c, ctx, ac.scheme, ac.rev)
}

type addApplication struct {
	name    string
	charm   string
	binding map[string]string
}

func (as addApplication) step(c *tc.C, ctx *ctx) {
	info, ok := ctx.charms[as.charm]
	c.Assert(ok, tc.IsTrue)

	curl := charm.MustParseURL(info.url)
	var channel string
	if charm.CharmHub.Matches(curl.Schema) {
		channel = "stable"
	}

	base := corebase.MustParseBaseFromString("ubuntu@12.10")
	now := time.Now()
	app := params.ApplicationStatus{
		Charm:        info.url,
		CharmChannel: channel,
		Base:         params.Base{Name: base.OS, Channel: base.Channel.String()},
		Status: params.DetailedStatus{
			Status: status.Unknown.String(),
			Since:  &now,
		},
		Units:            make(map[string]params.UnitStatus),
		Relations:        make(map[string][]string),
		EndpointBindings: make(map[string]string),
	}
	if as.charm == "lxd-profile" {
		app.CharmProfile = "juju-controller-lxd-profile-1"
	}
	if info.charm.Meta().Subordinate {
		ctx.subordinateApps[as.name] = &app
	}
	for _, ep := range info.charm.Meta().Provides {
		app.EndpointBindings[ep.Name] = "alpha"
	}
	for _, ep := range info.charm.Meta().Requires {
		app.EndpointBindings[ep.Name] = "alpha"
	}
	for _, ep := range info.charm.Meta().ExtraBindings {
		app.EndpointBindings[ep.Name] = "alpha"
	}
	if len(app.EndpointBindings) > 0 {
		app.EndpointBindings[""] = "alpha"
	}
	for _, ep := range info.charm.Meta().Peers {
		app.EndpointBindings[ep.Name] = "alpha"
		app.Relations[ep.Name] = []string{as.name}
		id := len(ctx.api.result.Relations) + 1
		rel := params.RelationStatus{
			Id:        id,
			Key:       fmt.Sprintf("%s:%s %s:%s", as.name, ep.Name, as.name, ep.Name),
			Interface: ep.Interface,
			Scope:     "global",
			Endpoints: []params.EndpointStatus{{
				Name:            ep.Name,
				ApplicationName: as.name,
				Role:            "peer",
			}},
			Status: params.DetailedStatus{
				Status: "joined",
			},
		}
		ctx.api.result.Relations = append(ctx.api.result.Relations, rel)
	}
	ctx.api.result.Applications[as.name] = app
}

type addRemoteApplication struct {
	name            string
	url             string
	charm           string
	endpoints       []string
	isConsumerProxy bool
}

func (as addRemoteApplication) step(c *tc.C, ctx *ctx) {
	info, ok := ctx.charms[as.charm]
	c.Assert(ok, tc.IsTrue)
	var endpoints []params.RemoteEndpoint
	for _, ep := range as.endpoints {
		r, ok := info.charm.Meta().Requires[ep]
		if !ok {
			r, ok = info.charm.Meta().Provides[ep]
		}
		c.Assert(ok, tc.IsTrue)
		endpoints = append(endpoints, params.RemoteEndpoint{
			Name:      r.Name,
			Role:      r.Role,
			Interface: r.Interface,
			Limit:     r.Limit,
		})
	}
	now := time.Now()
	if as.isConsumerProxy {
		ctx.remoteProxies[as.name] = params.RemoteApplicationStatus{
			OfferName: as.name,
			OfferURL:  as.url,
			Endpoints: endpoints,
			Status:    params.DetailedStatus{},
			Relations: make(map[string][]string),
		}
	} else {
		ctx.api.result.RemoteApplications[as.name] = params.RemoteApplicationStatus{
			OfferName: as.name,
			OfferURL:  as.url,
			Endpoints: endpoints,
			Relations: make(map[string][]string),
			Status: params.DetailedStatus{
				Status: status.Unknown.String(),
				Since:  &now,
			},
		}
	}
}

type addApplicationOffer struct {
	name            string
	qualifier       string
	applicationName string
	endpoints       []string
}

func (ao addApplicationOffer) step(c *tc.C, ctx *ctx) {
	app, ok := ctx.api.result.Applications[ao.applicationName]
	c.Assert(ok, tc.IsTrue)

	curl := charm.MustParseURL(app.Charm)
	info, ok := ctx.charms[curl.Name]
	c.Assert(ok, tc.IsTrue)

	endpoints := make(map[string]params.RemoteEndpoint)
	for _, ep := range ao.endpoints {
		r, ok := info.charm.Meta().Requires[ep]
		if !ok {
			r, ok = info.charm.Meta().Provides[ep]
		}
		c.Assert(ok, tc.IsTrue)
		endpoints[r.Name] = params.RemoteEndpoint{
			Name:      r.Name,
			Role:      r.Role,
			Interface: r.Interface,
			Limit:     r.Limit,
		}
	}

	ctx.api.result.Offers[ao.name] = params.ApplicationOfferStatus{
		Err:             nil,
		OfferName:       ao.name,
		ApplicationName: ao.applicationName,
		CharmURL:        app.Charm,
		Endpoints:       endpoints,
	}
}

type addOfferConnection struct {
	sourceModelUUID string
	name            string
	username        string
	relationKey     string
}

func (oc addOfferConnection) step(c *tc.C, ctx *ctx) {
	offer, ok := ctx.api.result.Offers[oc.name]
	c.Assert(ok, tc.IsTrue)
	offer.TotalConnectedCount++
	if oc.relationKey != "" {
		offer.ActiveConnectedCount++
	}
	ctx.api.result.Offers[oc.name] = offer
}

type setApplicationExposed struct {
	name    string
	exposed bool
}

func (sse setApplicationExposed) step(c *tc.C, ctx *ctx) {
	app, ok := ctx.api.result.Applications[sse.name]
	c.Assert(ok, tc.IsTrue)

	app.Exposed = sse.exposed
	ctx.api.result.Applications[sse.name] = app
}

type setApplicationCharm struct {
	name  string
	charm string
}

func (ssc setApplicationCharm) step(c *tc.C, ctx *ctx) {
	app, ok := ctx.api.result.Applications[ssc.name]
	c.Assert(ok, tc.IsTrue)

	curl := charm.MustParseURL(ssc.charm)
	var channel string
	if charm.CharmHub.Matches(curl.Schema) {
		channel = "stable"
	}

	app.Charm = ssc.charm
	app.CharmChannel = channel
	ctx.api.result.Applications[ssc.name] = app
}

type addCharmPlaceholder struct {
	name string
	rev  int
}

func (ac addCharmPlaceholder) step(c *tc.C, ctx *ctx) {
	ch := testcharms.Repo.CharmDir(ac.name)
	name := ch.Meta().Name
	curl := fmt.Sprintf("ch:quantal/%s-%d", name, ac.rev)
	ctx.charms[ac.name] = charmInfo{
		charm: ch,
		url:   curl,
	}
	for appName, app := range ctx.api.result.Applications {
		appCurl := charm.MustParseURL(app.Charm)
		if appCurl.Name == ac.name && appCurl.Revision < ac.rev && appCurl.Schema != "local" {
			appCurl.Revision = ac.rev
			app.CanUpgradeTo = appCurl.String()
			ctx.api.result.Applications[appName] = app
		}
	}
}

type addUnit struct {
	applicationName string
	machineId       string
}

func (au addUnit) step(c *tc.C, ctx *ctx) {
	app, ok := ctx.api.result.Applications[au.applicationName]
	c.Assert(ok, tc.IsTrue)

	m, ok := ctx.api.result.Machines[au.machineId]
	c.Assert(ok, tc.IsTrue)

	unitId := 0
	for u := range app.Units {
		if strings.HasPrefix(u, au.applicationName+"/") {
			unitId++
		}
	}
	now := time.Now()
	unitName := fmt.Sprintf("%s/%d", au.applicationName, unitId)
	app.Units[unitName] = params.UnitStatus{
		AgentStatus: params.DetailedStatus{
			Status: status.Lost.String(),
			Info:   "agent is not communicating with the server",
			Since:  &now,
		},
		WorkloadStatus: params.DetailedStatus{
			Status: status.Unknown.String(),
			Info:   fmt.Sprintf("agent lost, see 'juju show-status-log %s'", unitName),
			Since:  &now,
		},
		Machine:       au.machineId,
		PublicAddress: m.DNSName,
		Subordinates:  make(map[string]params.UnitStatus),
	}
	app.Status = params.DetailedStatus{
		Status: status.Active.String(),
		Since:  &now,
	}
	ctx.api.result.Applications[au.applicationName] = app
}

type addAliveUnit struct {
	applicationName string
	machineId       string
}

func (aau addAliveUnit) step(c *tc.C, ctx *ctx) {
	app, ok := ctx.api.result.Applications[aau.applicationName]
	c.Assert(ok, tc.IsTrue)

	m, ok := getMachine(ctx, aau.machineId)
	c.Assert(ok, tc.IsTrue)

	unitId := 0
	for u := range app.Units {
		if strings.HasPrefix(u, aau.applicationName+"/") {
			unitId++
		}
	}
	now := time.Now()
	app.Units[fmt.Sprintf("%s/%d", aau.applicationName, unitId)] = params.UnitStatus{
		AgentStatus: params.DetailedStatus{
			Status: status.Allocating.String(),
			Since:  &now,
		},
		WorkloadStatus: params.DetailedStatus{
			Status: status.Waiting.String(),
			Info:   "waiting for machine",
			Since:  &now,
		},
		Machine:       aau.machineId,
		PublicAddress: m.DNSName,
		Subordinates:  make(map[string]params.UnitStatus),
	}
	if app.Status.Status == status.Unknown.String() {
		app.Status = params.DetailedStatus{
			Status: status.Waiting.String(),
			Info:   "waiting for machine",
			Since:  &now,
		}
	}
	ctx.api.result.Applications[aau.applicationName] = app
}

type setUnitAsLeader struct {
	unitName string
}

func (s setUnitAsLeader) step(c *tc.C, ctx *ctx) {
	u, ok := unitByName(ctx, s.unitName)
	c.Assert(ok, tc.IsTrue)

	u.Leader = true
	updateUnit(ctx, s.unitName, &u)
}

func unitByName(ctx *ctx, unitName string) (params.UnitStatus, bool) {
	appName, _ := names.UnitApplication(unitName)
	u, ok := ctx.api.result.Applications[appName].Units[unitName]
	if !ok {
	done:
		for _, prinApp := range ctx.api.result.Applications {
			for _, prinUnit := range prinApp.Units {
				u, ok = prinUnit.Subordinates[unitName]
				if ok {
					break done
				}
			}
		}
	}
	return u, ok
}

func updateUnit(ctx *ctx, unitName string, u *params.UnitStatus) {
	appName, _ := names.UnitApplication(unitName)
	if _, ok := ctx.api.result.Applications[appName].Units[unitName]; ok {
		ctx.api.result.Applications[appName].Units[unitName] = *u
		return
	}
	for prinAppName, prinApp := range ctx.api.result.Applications {
		for prinName, prinUnit := range prinApp.Units {
			if _, ok := prinUnit.Subordinates[unitName]; ok {
				prinUnit.Subordinates[unitName] = *u
				prinApp.Units[prinName] = prinUnit
				ctx.api.result.Applications[prinAppName] = prinApp
				return
			}
		}
	}
}

type setUnitStatus struct {
	unitName   string
	status     status.Status
	statusInfo string
	statusData map[string]interface{}
}

func (sus setUnitStatus) step(c *tc.C, ctx *ctx) {
	u, ok := unitByName(ctx, sus.unitName)
	c.Assert(ok, tc.IsTrue)

	now := time.Now()
	u.WorkloadStatus = params.DetailedStatus{
		Status: sus.status.String(),
		Info:   sus.statusInfo,
		Data:   sus.statusData,
		Since:  &now,
	}
	if sus.status == status.Terminated {
		u.Subordinates = nil
	}
	updateUnit(ctx, sus.unitName, &u)

	appName, _ := names.UnitApplication(sus.unitName)
	app := ctx.api.result.Applications[appName]
	if sus.status == status.Terminated && len(app.Units) > 1 {
		return
	}
	if app.Status.Status == status.Active.String() || app.Status.Status == status.Unknown.String() || app.Status.Info == "waiting for machine" {
		app.Status = params.DetailedStatus{
			Status: sus.status.String(),
			Info:   sus.statusInfo,
			Data:   sus.statusData,
			Since:  &now,
		}
	}
	ctx.api.result.Applications[appName] = app
}

type setAgentStatus struct {
	unitName   string
	status     status.Status
	statusInfo string
	statusData map[string]interface{}
}

func (sus setAgentStatus) step(c *tc.C, ctx *ctx) {
	u, ok := unitByName(ctx, sus.unitName)
	c.Assert(ok, tc.IsTrue)

	now := time.Now()
	if sus.status == status.Error {
		unitInfo := sus.statusInfo
		id, ok := sus.statusData["relation-id"].(int)
		if ok {
			unitInfo = sus.statusInfo + " for mysql:server"
			sus.statusData["relation-id"] = float64(id)
		}
		u.WorkloadStatus = params.DetailedStatus{
			Status: sus.status.String(),
			Info:   unitInfo,
			Data:   sus.statusData,
			Since:  &now,
		}
		u.AgentStatus = params.DetailedStatus{
			Status: status.Idle.String(),
			Since:  &now,
		}
		appName, _ := names.UnitApplication(sus.unitName)
		app := ctx.api.result.Applications[appName]
		app.Status = params.DetailedStatus{
			Status: sus.status.String(),
			Info:   sus.statusInfo,
			Data:   sus.statusData,
			Since:  &now,
		}
		ctx.api.result.Applications[appName] = app
	} else {
		u.AgentStatus = params.DetailedStatus{
			Status: sus.status.String(),
			Info:   sus.statusInfo,
			Data:   sus.statusData,
			Since:  &now,
		}
	}
	updateUnit(ctx, sus.unitName, &u)
}

type setUnitCharmURL struct {
	unitName string
	charm    string
}

func (uc setUnitCharmURL) step(c *tc.C, ctx *ctx) {
	appName, _ := names.UnitApplication(uc.unitName)
	u, ok := ctx.api.result.Applications[appName].Units[uc.unitName]
	c.Assert(ok, tc.IsTrue)

	u.Charm = uc.charm

	now := time.Now()
	u.WorkloadStatus = params.DetailedStatus{
		Status: status.Active.String(),
		Since:  &now,
	}
	u.AgentStatus = params.DetailedStatus{
		Status: status.Idle.String(),
		Since:  &now,
	}
	ctx.api.result.Applications[appName].Units[uc.unitName] = u
}

type setUnitWorkloadVersion struct {
	unitName string
	version  string
}

func (wv setUnitWorkloadVersion) step(c *tc.C, ctx *ctx) {
	appName, _ := names.UnitApplication(wv.unitName)
	app, ok := ctx.api.result.Applications[appName]
	c.Assert(ok, tc.IsTrue)

	app.WorkloadVersion = wv.version
	ctx.api.result.Applications[appName] = app
}

type openUnitPort struct {
	unitName string
	protocol string
	number   int
}

func (oup openUnitPort) step(c *tc.C, ctx *ctx) {
	appName, _ := names.UnitApplication(oup.unitName)
	u, ok := ctx.api.result.Applications[appName].Units[oup.unitName]
	c.Assert(ok, tc.IsTrue)

	u.OpenedPorts = append(u.OpenedPorts, fmt.Sprintf("%d/%s", oup.number, oup.protocol))
	sort.Slice(u.OpenedPorts, func(i, j int) bool {
		p1 := u.OpenedPorts[i]
		p2 := u.OpenedPorts[j]
		p1parts := strings.Split(p1, "/")
		p2parts := strings.Split(p2, "/")
		if p1parts[1] != p2parts[1] {
			return p1parts[1] < p2parts[1]
		}
		p1n, _ := strconv.Atoi(p1parts[0])
		p2n, _ := strconv.Atoi(p2parts[0])
		return p1n < p2n
	})
	ctx.api.result.Applications[appName].Units[oup.unitName] = u
}

type ensureDyingApplication struct {
	applicationName string
}

func (e ensureDyingApplication) step(c *tc.C, ctx *ctx) {
	app, ok := ctx.api.result.Applications[e.applicationName]
	c.Assert(ok, tc.IsTrue)

	app.Life = "dying"
	ctx.api.result.Applications[e.applicationName] = app
}

type ensureDeadMachine struct {
	machineId string
}

func (e ensureDeadMachine) step(c *tc.C, ctx *ctx) {
	m, ok := ctx.api.result.Machines[e.machineId]
	c.Assert(ok, tc.IsTrue)

	m.AgentStatus.Life = "dead"
	ctx.api.result.Machines[e.machineId] = m
}

type setMachineStatus struct {
	machineId  string
	status     status.Status
	statusInfo string
}

func (sms setMachineStatus) step(c *tc.C, ctx *ctx) {
	m, ok := getMachine(ctx, sms.machineId)
	c.Assert(ok, tc.IsTrue)

	now := time.Now()
	m.AgentStatus = params.DetailedStatus{
		Status: sms.status.String(),
		Info:   sms.statusInfo,
		Since:  &now,
	}
	saveMachine(ctx, sms.machineId, m)
	if sms.status != status.Started {
		return
	}
}

func counterpartRole(r charm.RelationRole) charm.RelationRole {
	switch r {
	case charm.RoleProvider:
		return charm.RoleRequirer
	case charm.RoleRequirer:
		return charm.RoleProvider
	case charm.RolePeer:
		return charm.RolePeer
	}
	panic(fmt.Errorf("unknown relation role %q", r))
}

func canRelateTo(ep1, ep2 charm.Relation) bool {
	return ep1.Interface == ep2.Interface &&
		ep1.Role != charm.RolePeer &&
		counterpartRole(ep1.Role) == ep2.Role
}

func appEndpoints(c *tc.C, ctx *ctx, appName string) ([]charm.Relation, bool) {
	remoteApp, ok := ctx.api.result.RemoteApplications[appName]
	if !ok {
		remoteApp, ok = ctx.remoteProxies[appName]
	}
	if ok {
		var result []charm.Relation
		for _, ep := range remoteApp.Endpoints {
			result = append(result, charm.Relation{
				Name:      ep.Name,
				Role:      ep.Role,
				Interface: ep.Interface,
				Limit:     ep.Limit,
				Scope:     "global",
			})
		}
		return result, false
	}
	app, ok := ctx.api.result.Applications[appName]
	c.Assert(ok, tc.IsTrue)
	curl := charm.MustParseURL(app.Charm)
	ch, ok := ctx.charms[curl.Name]
	c.Assert(ok, tc.IsTrue)

	var result []charm.Relation
	for _, ep := range ch.charm.Meta().Requires {
		result = append(result, charm.Relation{
			Name:      ep.Name,
			Role:      charm.RoleRequirer,
			Interface: ep.Interface,
			Scope:     ep.Scope,
		})
	}
	for _, ep := range ch.charm.Meta().Provides {
		result = append(result, charm.Relation{
			Name:      ep.Name,
			Role:      charm.RoleProvider,
			Interface: ep.Interface,
			Scope:     ep.Scope,
		})
	}
	if !ch.charm.Meta().Subordinate {
		result = append(result, charm.Relation{
			Name:      relation.JujuInfo,
			Role:      charm.RoleProvider,
			Interface: relation.JujuInfo,
			Scope:     charm.ScopeGlobal,
		})
	}
	return result, ch.charm.Meta().Subordinate
}

func inferEndpoints(c *tc.C, ctx *ctx, app1Name, app2Name string) ([]params.EndpointStatus, string) {
	ch1ep, ch1Subordinate := appEndpoints(c, ctx, app1Name)
	ch2ep, ch2Subordinate := appEndpoints(c, ctx, app2Name)
	var (
		endpoints     []params.EndpointStatus
		interfaceName string
	)
done:
	for _, ep1 := range ch1ep {
		for _, ep2 := range ch2ep {
			if canRelateTo(ep1, ep2) {
				endpoints = []params.EndpointStatus{{
					ApplicationName: app1Name,
					Name:            ep1.Name,
					Role:            string(ep1.Role),
					Subordinate:     ch1Subordinate,
				}, {
					ApplicationName: app2Name,
					Name:            ep2.Name,
					Role:            string(ep2.Role),
					Subordinate:     ch2Subordinate,
				}}
				interfaceName = ep1.Interface
				break done
			}
		}
	}
	c.Assert(endpoints, tc.HasLen, 2)
	return endpoints, interfaceName
}

type relateApplications struct {
	app1, app2 string
	status     string
}

func (rs relateApplications) step(c *tc.C, ctx *ctx) {
	endpoints, interfaceName := inferEndpoints(c, ctx, rs.app1, rs.app2)
	id := len(ctx.api.result.Relations) + 1
	scope := "global"
	if endpoints[0].Subordinate || endpoints[1].Subordinate {
		scope = "container"
	}
	if rs.status == "" {
		rs.status = "joined"
	}
	rel := params.RelationStatus{
		Id:        id,
		Key:       fmt.Sprintf("%s:%s %s:%s", rs.app1, endpoints[0].Name, rs.app2, endpoints[1].Name),
		Interface: interfaceName,
		Scope:     scope,
		Endpoints: endpoints,
		Status: params.DetailedStatus{
			Status: rs.status,
		},
	}
	if !strings.HasPrefix(rs.app1, "remote-") && !strings.HasPrefix(rs.app2, "remote-") {
		ctx.api.result.Relations = append(ctx.api.result.Relations, rel)
	}
	var rels1, rels2 map[string][]string
	app1, app1ok := ctx.api.result.Applications[rs.app1]
	if app1ok {
		rels1 = app1.Relations
	}
	app2, app2ok := ctx.api.result.Applications[rs.app2]
	if app2ok {
		rels2 = app2.Relations
	}
	rapp1, rapp1ok := ctx.api.result.RemoteApplications[rs.app1]
	if rapp1ok {
		rels1 = rapp1.Relations
	}
	rapp2, rapp2ok := ctx.api.result.RemoteApplications[rs.app2]
	if rapp2ok {
		rels2 = rapp2.Relations
	}
	papp1, papp1ok := ctx.remoteProxies[rs.app1]
	if papp1ok {
		rels1 = papp1.Relations
	}
	papp2, papp2ok := ctx.remoteProxies[rs.app2]
	if papp2ok {
		rels2 = papp2.Relations
	}

	for _, ep := range endpoints {
		if ep.ApplicationName == rs.app1 && rels1 != nil {
			rels1[ep.Name] = append(rels1[ep.Name], rs.app2)
			if scope == "global" {
				continue
			}
			if ep.Subordinate {
				app1.SubordinateTo = append(app1.SubordinateTo, rs.app2)
				app1.Scale++
			}
		}
		if ep.ApplicationName == rs.app2 && rels2 != nil {
			rels2[ep.Name] = append(rels2[ep.Name], rs.app1)
			if scope == "global" {
				continue
			}
			if ep.Subordinate {
				app2.SubordinateTo = append(app2.SubordinateTo, rs.app1)
				app2.Scale++
			}
		}
	}
	for ep := range rels1 {
		sort.Strings(rels1[ep])
	}
	for ep := range rels2 {
		sort.Strings(rels2[ep])
	}
	if app1ok {
		sort.Strings(app1.SubordinateTo)
		ctx.api.result.Applications[rs.app1] = app1
	}
	if app2ok {
		sort.Strings(app2.SubordinateTo)
		ctx.api.result.Applications[rs.app2] = app2
	}
}

type addSubordinate struct {
	prinUnit       string
	subApplication string
}

func (as addSubordinate) step(c *tc.C, ctx *ctx) {
	prinAappName, _ := names.UnitApplication(as.prinUnit)

	endpoints, _ := inferEndpoints(c, ctx, prinAappName, as.subApplication)
	_ = fmt.Sprintf("%s:%s %s:%s", prinAappName, endpoints[0].Name, as.subApplication, endpoints[1].Name)
	for _, ep := range endpoints {
		if !ep.Subordinate {
			continue
		}
		for unitName, u := range ctx.api.result.Applications[prinAappName].Units {
			if u.WorkloadStatus.Status == status.Terminated.String() {
				continue
			}
			m, ok := ctx.api.result.Machines[u.Machine]
			c.Assert(ok, tc.IsTrue)
			unitId := ctx.subordinateUnits[as.subApplication]
			subName := fmt.Sprintf("%s/%d", as.subApplication, unitId)
			u.Subordinates[subName] = params.UnitStatus{
				AgentStatus:    params.DetailedStatus{Status: status.Idle.String()},
				WorkloadStatus: params.DetailedStatus{},
				PublicAddress:  m.DNSName,
			}
			ctx.subordinateUnits[as.subApplication] = unitId + 1
			ctx.api.result.Applications[prinAappName].Units[unitName] = u
		}
	}
}

type setCharmProfiles struct {
	machineId string
	profiles  []string
}

func (s setCharmProfiles) step(c *tc.C, ctx *ctx) {
	m, ok := ctx.api.result.Machines[s.machineId]
	c.Assert(ok, tc.IsTrue)

	for _, p := range s.profiles {
		info := ctx.charms["lxd-profile"]
		profile := info.charm.(charm.LXDProfiler).LXDProfile()
		m.LXDProfiles[p] = params.LXDProfile{
			Config:      profile.Config,
			Description: profile.Description,
			Devices:     profile.Devices,
		}
	}
	ctx.api.result.Machines[s.machineId] = m
}

type expect struct {
	what   string
	output M
	stderr string
}

func (e expect) step(c *tc.C, ctx *ctx) {
	c.Logf("\nexpect: %s\n", e.what)

	// Now execute the command for each format.
	for _, format := range statusFormats {
		c.Logf("format %q", format.name)
		// Run command with the required format.
		args := []string{"--no-color", "--format", format.name}
		if ctx.expectIsoTime {
			args = append(args, "--utc")
		}
		ctx.api.expectIncludeStorage = format.name != "tabular"
		c.Logf("running status %s", strings.Join(args, " "))
		code, stdout, stderr := runStatus(c, ctx, args...)
		c.Assert(code, tc.Equals, 0)
		c.Assert(stderr, tc.Equals, e.stderr)

		// Prepare the output in the same format.
		buf, err := format.marshal(e.output)
		c.Assert(err, tc.ErrorIsNil)

		// we have to force the timestamp into the correct format as the model
		// is in string.
		ts := substituteFakeTimestamp(c, string(buf), ctx.expectIsoTime)

		expected := make(M)
		err = format.unmarshal([]byte(ts), &expected)
		c.Assert(err, tc.ErrorIsNil)

		// Check the output is as expected.
		actual := make(M)
		out := substituteFakeTime(c, "since", stdout, ctx.expectIsoTime)
		out = substituteFakeTimestamp(c, out, ctx.expectIsoTime)
		err = format.unmarshal([]byte(out), &actual)
		c.Assert(err, tc.ErrorIsNil)
		pretty.Ldiff(c, actual, expected)
		c.Assert(actual, tc.DeepEquals, expected)
	}
}

// substituteFakeTime replaces all key values
// in actual status output with a known fake value.
func substituteFakeTime(c *tc.C, key string, in string, expectIsoTime bool) string {
	// This regexp will work for yaml and json.
	exp := regexp.MustCompile(`(?P<key>"?` + key + `"?:\ ?)(?P<quote>"?)(?P<timestamp>[^("|\n)]*)*"?`)
	// Before the substitution is done, check that the timestamp produced
	// by status is in the correct format.
	if matches := exp.FindStringSubmatch(in); matches != nil {
		for i, name := range exp.SubexpNames() {
			if name != "timestamp" {
				continue
			}
			timeFormat := "02 Jan 2006 15:04:05Z07:00"
			if expectIsoTime {
				timeFormat = "2006-01-02 15:04:05Z"
			}
			_, err := time.Parse(timeFormat, matches[i])
			c.Assert(err, tc.ErrorIsNil)
		}
	}

	out := exp.ReplaceAllString(in, `$key$quote<timestamp>$quote`)
	// Substitute a made up time used in our expected output.
	out = strings.Replace(out, "<timestamp>", "01 Apr 15 01:23+10:00", -1)
	return out
}

// substituteFakeTimestamp replaces all key values for a given timestamp
// in actual status output with a known fake value.
func substituteFakeTimestamp(c *tc.C, in string, expectIsoTime bool) string {
	timeFormat := "15:04:05Z07:00"
	output := strings.Replace(timeFormat, "Z", "+", -1)
	if expectIsoTime {
		timeFormat = "15:04:05Z"
		output = "15:04:05"
	}
	// This regexp will work for any input type
	exp := regexp.MustCompile(`(?P<timestamp>[0-9]{2}:[0-9]{2}:[0-9]{2}((Z|\+|\-)([0-9]{2}:[0-9]{2})?)?)`)
	if matches := exp.FindStringSubmatch(in); matches != nil {
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
			c.Assert(err, tc.ErrorIsNil)
		}
	}

	out := exp.ReplaceAllString(in, `<timestamp>`)
	// Substitute a made up time used in our expected output.
	out = strings.Replace(out, "<timestamp>", output, -1)
	return out
}

// substituteSpacingBetweenTimestampAndNotes forces the spacing between the
// headers Timestamp and Notes to be consistent regardless of the time. This
// happens because we're dealing with the result of the strings of stdout and
// not with any useable AST
func substituteSpacingBetweenTimestampAndNotes(c *tc.C, in string) string {
	exp := regexp.MustCompile(`Timestamp(?P<spacing>\s+)Notes`)
	result := exp.ReplaceAllString(in, fmt.Sprintf("Timestamp%sNotes", strings.Repeat(" ", 7)))
	return result
}

type setToolsUpgradeAvailable struct{}

func (ua setToolsUpgradeAvailable) step(c *tc.C, ctx *ctx) {
	ctx.api.result.Model.AvailableVersion = nextVersion.String()
}

func (s *StatusSuite) TestStatusAllFormats(c *tc.C) {
	for i, t := range statusTests {
		c.Logf("test %d: %s", i, t.summary)
		func(t testCase) {
			// Prepare ctx and run all steps to setup.
			ctx := s.newContext()
			ctx.run(c, t.steps)
		}(t)
	}
}

func (s *StatusSuite) TestStatusWithFormatSummary(c *tc.C) {
	ctx := s.newContext()
	steps := []stepper{
		addMachine{machineId: "0", job: coremodel.JobManageModel},
		setAddresses{"0", network.NewSpaceAddresses("localhost")},
		startAliveMachine{"0", "snowflake"},
		setMachineStatus{"0", status.Started, ""},
		addCharmHubCharm{"wordpress"},
		addCharmHubCharm{"mysql"},
		addCharmHubCharm{"logging"},
		addCharmHubCharm{"riak"},
		addRemoteApplication{name: "hosted-riak", url: "me/model.riak", charm: "riak", endpoints: []string{"endpoint"}},
		addApplication{name: "wordpress", charm: "wordpress"},
		setApplicationExposed{"wordpress", true},
		addMachine{machineId: "1", job: coremodel.JobHostUnits},
		setAddresses{"1", network.NewSpaceAddresses("localhost")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addAliveUnit{"wordpress", "1"},
		setAgentStatus{"wordpress/0", status.Idle, "", nil},
		setUnitStatus{"wordpress/0", status.Active, "", nil},
		addApplication{name: "mysql", charm: "mysql"},
		setApplicationExposed{"mysql", true},
		addMachine{machineId: "2", job: coremodel.JobHostUnits},
		setAddresses{"2", network.NewSpaceAddresses("10.0.2.1")},
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
		setAgentStatus{"logging/0", status.Idle, "", nil},
		setUnitStatus{"logging/0", status.Active, "", nil},
		setAgentStatus{"logging/1", status.Error, "somehow lost in all those logs", nil},
	}
	for _, s := range steps {
		s.step(c, ctx)
	}
	ctx.api.expectIncludeStorage = true
	code, stdout, stderr := runStatus(c, ctx, "--no-color", "--format", "summary")
	c.Check(code, tc.Equals, 0)
	c.Check(stderr, tc.Equals, "")
	c.Assert(stdout, tc.Equals, `
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

func (s *StatusSuite) TestStatusWithFormatOneline(c *tc.C) {
	ctx := s.newContext()

	steps := []stepper{
		addMachine{machineId: "0", job: coremodel.JobManageModel},
		setAddresses{"0", network.NewSpaceAddresses("10.0.0.1")},
		startAliveMachine{"0", "snowflake"},
		setMachineStatus{"0", status.Started, ""},
		addCharmHubCharm{"wordpress"},
		addCharmHubCharm{"mysql"},
		addCharmHubCharm{"logging"},

		addApplication{name: "wordpress", charm: "wordpress"},
		setApplicationExposed{"wordpress", true},
		addMachine{machineId: "1", job: coremodel.JobHostUnits},
		setAddresses{"1", network.NewSpaceAddresses("10.0.1.1")},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		addAliveUnit{"wordpress", "1"},
		setAgentStatus{"wordpress/0", status.Idle, "", nil},
		setUnitStatus{"wordpress/0", status.Active, "", nil},

		addApplication{name: "mysql", charm: "mysql"},
		setApplicationExposed{"mysql", true},
		addMachine{machineId: "2", job: coremodel.JobHostUnits},
		setAddresses{"2", network.NewSpaceAddresses("10.0.2.1")},
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

		setAgentStatus{"logging/0", status.Idle, "", nil},
		setUnitStatus{"logging/0", status.Active, "", nil},
		setAgentStatus{"logging/1", status.Error, "somehow lost in all those logs", nil},
	}

	ctx.run(c, steps)

	var expected = `
- mysql/0: 10.0.2.1 (agent:idle, workload:active)
  - logging/1: 10.0.2.1 (agent:idle, workload:error)
- wordpress/0: 10.0.1.1 (agent:idle, workload:active)
  - logging/0: 10.0.1.1 (agent:idle, workload:active)
`[1:]
	assertOneLineStatus(c, ctx, expected)
}

func assertOneLineStatus(c *tc.C, ctx *ctx, expected string) {
	ctx.api.expectIncludeStorage = true

	code, stdout, stderr := runStatus(c, ctx, "--no-color", "--format", "oneline")
	c.Check(code, tc.Equals, 0)
	c.Check(stderr, tc.Equals, "")
	c.Assert(stdout, tc.Equals, expected)

	c.Log(`Check that "short" is an alias for oneline.`)
	code, stdout, stderr = runStatus(c, ctx, "--no-color", "--format", "short")
	c.Check(code, tc.Equals, 0)
	c.Check(stderr, tc.Equals, "")
	c.Assert(stdout, tc.Equals, expected)

	c.Log(`Check that "line" is an alias for oneline.`)
	code, stdout, stderr = runStatus(c, ctx, "--no-color", "--format", "line")
	c.Check(code, tc.Equals, 0)
	c.Check(stderr, tc.Equals, "")
	c.Assert(stdout, tc.Equals, expected)
}

func (s *StatusSuite) prepareTabularData(c *tc.C) *ctx {
	ctx := s.newContext()
	steps := []stepper{
		setToolsUpgradeAvailable{},
		addMachine{machineId: "0", job: coremodel.JobManageModel},
		setAddresses{"0", network.NewSpaceAddresses("10.0.0.1")},
		startMachineWithHardware{"0", instance.MustParseHardware("availability-zone=us-east-1a")},
		setMachineStatus{"0", status.Started, ""},
		addCharmHubCharm{"wordpress"},
		addCharmHubCharm{"mysql"},
		addCharmHubCharm{"logging"},
		addCharmHubCharm{"riak"},
		addRemoteApplication{name: "hosted-riak", url: "me/model.riak", charm: "riak", endpoints: []string{"endpoint"}},
		addApplication{name: "wordpress", charm: "wordpress"},
		setApplicationExposed{"wordpress", true},
		addMachine{machineId: "1", job: coremodel.JobHostUnits},
		setAddresses{"1", network.NewSpaceAddresses("10.0.1.1")},
		startAliveMachine{"1", "snowflake"},
		setMachineStatus{"1", status.Started, ""},
		addAliveUnit{"wordpress", "1"},
		setAgentStatus{"wordpress/0", status.Idle, "", nil},
		setUnitStatus{"wordpress/0", status.Active, "", nil},
		setUnitTools{"wordpress/0", semversion.MustParseBinary("1.2.3-ubuntu-ppc")},
		addApplication{name: "mysql", charm: "mysql"},
		setApplicationExposed{"mysql", true},
		addMachine{machineId: "2", job: coremodel.JobHostUnits},
		setAddresses{"2", network.NewSpaceAddresses("10.0.2.1")},
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
		setUnitTools{"mysql/0", semversion.MustParseBinary("1.2.3-ubuntu-ppc")},
		addApplication{name: "logging", charm: "logging"},
		setApplicationExposed{"logging", true},
		relateApplications{"wordpress", "mysql", "suspended"},
		relateApplications{"wordpress", "logging", ""},
		relateApplications{"mysql", "logging", ""},
		addSubordinate{"wordpress/0", "logging"},
		addSubordinate{"mysql/0", "logging"},
		setAgentStatus{"logging/0", status.Idle, "", nil},
		setUnitStatus{"logging/0", status.Active, "", nil},
		setAgentStatus{"logging/1", status.Error, "somehow lost in all those logs", nil},
		setUnitWorkloadVersion{"logging/1", "a bit too long, really"},
		setUnitWorkloadVersion{"wordpress/0", "4.5.3"},
		setUnitWorkloadVersion{"mysql/0", "5.7.13\nanother"},
		setUnitAsLeader{"mysql/0"},
		setUnitAsLeader{"logging/1"},
		setUnitAsLeader{"wordpress/0"},
		addMachine{machineId: "3", job: coremodel.JobHostUnits},
		setAddresses{"3", network.NewSpaceAddresses("10.0.3.1")},
		startAliveMachine{"3", ""},
		setMachineStatus{"3", status.Started, ""},
		setMachineInstanceStatus{"3", status.Started, "I am number three"},

		addApplicationOffer{name: "hosted-mysql", applicationName: "mysql", qualifier: "admin", endpoints: []string{"server"}},
		addRemoteApplication{name: "remote-wordpress", charm: "wordpress", endpoints: []string{"db"}, isConsumerProxy: true},
		relateApplications{"remote-wordpress", "mysql", ""},
		addOfferConnection{sourceModelUUID: coretesting.ModelTag.Id(), name: "hosted-mysql", username: "fred", relationKey: "remote-wordpress:db mysql:server"},

		// test modification status
		addMachine{machineId: "4", job: coremodel.JobHostUnits},
		setAddresses{"4", network.NewSpaceAddresses("10.0.3.1")},
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
Model       Controller  Cloud/Region        Version  Timestamp       Notes
controller  kontroll    dummy/dummy-region  1.2.3    15:04:05+07:00  upgrade available: 1.2.4

SAAS         Status   Store  URL
hosted-riak  unknown  local  me/model.riak

App        Version          Status       Scale  Charm      Channel  Rev  Exposed  Message
logging    a bit too lo...  error            2  logging    stable     1  yes      somehow lost in all those logs
mysql      5.7.13           maintenance    1/2  mysql      stable     1  yes      installing all the things
wordpress  4.5.3            active           1  wordpress  stable     3  yes      

Unit          Workload     Agent  Machine  Public address  Ports  Message
mysql/0*      maintenance  idle   2        10.0.2.1               installing all the things
  logging/1*  error        idle            10.0.2.1               somehow lost in all those logs
mysql/1       terminated   idle   1        10.0.1.1               gooooone
wordpress/0*  active       idle   1        10.0.1.1               
  logging/0   active       idle            10.0.1.1               

Machine  State    Address   Inst id       Base          AZ          Message
0        started  10.0.0.1  controller-0  ubuntu@12.10  us-east-1a  
1        started  10.0.1.1  snowflake     ubuntu@12.10              
2        started  10.0.2.1  controller-2  ubuntu@12.10              
3        started  10.0.3.1  controller-3  ubuntu@12.10              I am number three
4        error    10.0.3.1  controller-4  ubuntu@12.10              I am an error

Offer         Application  Charm  Rev  Connected  Endpoint  Interface  Role
hosted-mysql  mysql        mysql  1    1/1        server    mysql      provider

Integration provider   Requirer                   Interface  Type         Message
mysql:juju-info        logging:info               juju-info  subordinate  
mysql:server           wordpress:db               mysql      regular      suspended  
wordpress:logging-dir  logging:logging-directory  logging    subordinate  
`[1:]

func (s *StatusSuite) TestStatusWithFormatTabular(c *tc.C) {
	ctx := s.prepareTabularData(c)

	code, stdout, stderr := runStatus(c, ctx, "--no-color", "--format", "tabular", "--relations")
	c.Check(code, tc.Equals, 0)
	c.Check(stderr, tc.Equals, "")

	output := substituteFakeTimestamp(c, stdout, false)
	output = substituteSpacingBetweenTimestampAndNotes(c, output)
	c.Assert(output, tc.Equals, expectedTabularStatus)
}

func (s *StatusSuite) TestStatusWithFormatYaml(c *tc.C) {
	ctx := s.prepareTabularData(c)
	ctx.api.expectIncludeStorage = true

	code, stdout, stderr := runStatus(c, ctx, "--no-color", "--format", "yaml")
	c.Check(code, tc.Equals, 0)
	c.Check(stderr, tc.Equals, "")
	c.Assert(stdout, tc.Contains, "display-name: snowflake")
	c.Assert(stdout, tc.Contains, "stable")
}

func (s *StatusSuite) TestStatusWithFormatJson(c *tc.C) {
	ctx := s.prepareTabularData(c)
	ctx.api.expectIncludeStorage = true

	code, stdout, stderr := runStatus(c, ctx, "--no-color", "--format", "json")
	c.Check(code, tc.Equals, 0)
	c.Check(stderr, tc.Equals, "")
	c.Assert(stdout, tc.Contains, `"display-name":"snowflake"`)
}

func (s *StatusSuite) TestFormatTabularHookActionName(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out.String(), tc.Equals, `
Model  Controller  Cloud/Region  Version
                                 

App  Version  Status  Scale  Charm  Channel  Rev  Exposed  Message
foo                       2                    0  no       

Unit   Workload     Agent      Machine  Public address  Ports  Message
foo/0  maintenance  executing                                  (config-changed) doing some work
foo/1  maintenance  executing                                  (backup database) doing some work
`[1:])
}

func (s *StatusSuite) TestFormatTabularCAASModel(c *tc.C) {
	status := formattedStatus{
		Model: modelStatus{
			Type: "caas",
		},
		Applications: map[string]applicationStatus{
			"foo": {
				Scale:   2,
				Address: "54.32.1.2",
				Version: "user/image:tag",
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out.String(), tc.Equals, `
Model  Controller  Cloud/Region  Version
                                 

App  Version         Status  Scale  Charm  Channel  Rev  Address    Exposed  Message
foo  user/image:tag            1/2                    0  54.32.1.2  no       

Unit   Workload  Agent       Address   Ports   Message
foo/0  active    allocating                    
foo/1  active    running     10.0.0.1  80/TCP  
`[1:])
}

func (s *StatusSuite) TestFormatTabularCAASModelTruncatedVersion(c *tc.C) {
	status := formattedStatus{
		Model: modelStatus{
			Type: "caas",
		},
		Applications: map[string]applicationStatus{
			"foo": {
				Scale:   1,
				Address: "54.32.1.2",
				Version: "registry.jujucharms.com/fred/mysql/mysql_image:tag@sha256:3046a3dc76ee23417f889675bce3a4c08f223b87d1e378eeea3e7490cd27fbc5",
				Units: map[string]unitStatus{
					"foo/0": {
						JujuStatusInfo: statusInfoContents{
							Current: status.Allocating,
						},
						WorkloadStatusInfo: statusInfoContents{
							Current: status.Active,
						},
					},
				},
			},
			"bar": {
				Charm:       "bar",
				CharmOrigin: "charmhub",
				Scale:       1,
				Address:     "54.32.1.3",
				Version:     "registry.jujucharms.com/fredbloggsthethrid/bar/image:0.5",
				Units: map[string]unitStatus{
					"bar/0": {
						JujuStatusInfo: statusInfoContents{
							Current: status.Allocating,
						},
						WorkloadStatusInfo: statusInfoContents{
							Current: status.Active,
						},
					},
				},
			},
			"baz": {
				Scale:   1,
				Address: "54.32.1.4",
				Version: "docker.io/reallyreallyreallyreallyreallylong/image:0.6",
				Units: map[string]unitStatus{
					"baz/0": {
						JujuStatusInfo: statusInfoContents{
							Current: status.Allocating,
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out.String(), tc.Equals, `
Model  Controller  Cloud/Region  Version
                                 

App  Version                         Status  Scale  Charm  Channel  Rev  Address    Exposed  Message
bar  res:image:0.5                             0/1                    0  54.32.1.3  no       
baz  .../image:0.6                             0/1                    0  54.32.1.4  no       
foo  .../mysql/mysql_image:tag@3...            0/1                    0  54.32.1.2  no       

Unit   Workload  Agent       Address  Ports  Message
bar/0  active    allocating                  
baz/0  active    allocating                  
foo/0  active    allocating                  
`[1:])
}

func (s *StatusSuite) TestFormatTabularStatusMessage(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out.String(), tc.Equals, `
Model  Controller  Cloud/Region  Version
                                 

App  Version  Status  Scale  Charm  Channel  Rev  Address    Exposed  Message
foo                     0/1                    0  54.32.1.2  no       Error: ImagePullBackOff

Unit   Workload  Agent       Address   Ports   Message
foo/0  waiting   allocating  10.0.0.1  80/TCP  
`[1:])
}

func (s *StatusSuite) TestFormatTabularManyPorts(c *tc.C) {
	fStatus := formattedStatus{
		Model: modelStatus{
			Type: "caas",
		},
		Applications: map[string]applicationStatus{
			"foo": {
				Scale:   1,
				Address: "54.32.1.2",
				Units: map[string]unitStatus{
					"foo/0": {
						Address:     "10.0.0.1",
						OpenedPorts: []string{"1555/TCP", "123/UDP", "ICMP", "80/TCP"},
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out.String(), tc.Equals, `
Model  Controller  Cloud/Region  Version
                                 

App  Version  Status  Scale  Charm  Channel  Rev  Address    Exposed  Message
foo                     0/1                    0  54.32.1.2  no       

Unit   Workload  Agent       Address   Ports                     Message
foo/0  waiting   allocating  10.0.0.1  80,1555/TCP 123/UDP ICMP  
`[1:])
}

func (s *StatusSuite) TestFormatTabularManyPortsGrouped(c *tc.C) {
	fStatus := formattedStatus{
		Model: modelStatus{
			Type: "caas",
		},
		Applications: map[string]applicationStatus{
			"foo": {
				Scale:   1,
				Address: "54.32.1.2",
				Units: map[string]unitStatus{
					"foo/0": {
						Address:     "10.0.0.1",
						OpenedPorts: []string{"1557/TCP", "1555/TCP", "80/TCP", "ICMP", "1556/TCP"},
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out.String(), tc.Equals, `
Model  Controller  Cloud/Region  Version
                                 

App  Version  Status  Scale  Charm  Channel  Rev  Address    Exposed  Message
foo                     0/1                    0  54.32.1.2  no       

Unit   Workload  Agent       Address   Ports                  Message
foo/0  waiting   allocating  10.0.0.1  80,1555-1557/TCP ICMP  
`[1:])
}

func (s *StatusSuite) TestFormatTabularManyPortsCommonGrouped(c *tc.C) {
	fStatus := formattedStatus{
		Model: modelStatus{
			Type: "caas",
		},
		Applications: map[string]applicationStatus{
			"foo": {
				Scale:   1,
				Address: "54.32.1.2",
				Units: map[string]unitStatus{
					"foo/0": {
						Address:     "10.0.0.1",
						OpenedPorts: []string{"1557/TCP", "1555/TCP", "1558/TCP", "1559/TCP", "80/TCP", "1556/TCP"},
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out.String(), tc.Equals, `
Model  Controller  Cloud/Region  Version
                                 

App  Version  Status  Scale  Charm  Channel  Rev  Address    Exposed  Message
foo                     0/1                    0  54.32.1.2  no       

Unit   Workload  Agent       Address   Ports             Message
foo/0  waiting   allocating  10.0.0.1  80,1555-1559/TCP  
`[1:])
}

func (s *StatusSuite) TestFormatTabularTruncateMessage(c *tc.C) {
	longMessage := "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum."
	longStatusInfo := statusInfoContents{
		Current: status.Active,
		Message: longMessage,
	}

	status := formattedStatus{
		Model: modelStatus{
			Name:       "m",
			Controller: "c",
			Cloud:      "localhost",
			Version:    "3.0.0",
			Status:     longStatusInfo,
		},
		Applications: map[string]applicationStatus{
			"foo": {
				CharmName:    "foo",
				CharmChannel: "latest/stable",
				StatusInfo:   longStatusInfo,
				Units: map[string]unitStatus{
					"foo/0": {
						WorkloadStatusInfo: longStatusInfo,
						JujuStatusInfo:     longStatusInfo,
						Machine:            "0",
						PublicAddress:      "10.53.62.100",
						Subordinates: map[string]unitStatus{
							"foo/1": {
								WorkloadStatusInfo: longStatusInfo,
								JujuStatusInfo:     longStatusInfo,
								Machine:            "0/lxd/0",
								PublicAddress:      "10.53.62.101",
							},
						},
					},
				},
			},
		},
		RemoteApplications: map[string]remoteApplicationStatus{
			"bar": {
				OfferURL:   "model.io/bar",
				StatusInfo: longStatusInfo,
			},
		},
		Machines: map[string]machineStatus{
			"0": {
				Id:      "0",
				DNSName: "10.53.62.100",
				Base: &formattedBase{
					Name:    "ubuntu",
					Channel: "22.04",
				},
				JujuStatus:         longStatusInfo,
				MachineStatus:      longStatusInfo,
				ModificationStatus: longStatusInfo,
				Containers: map[string]machineStatus{
					"0": {
						Id:      "0/lxd/0",
						DNSName: "10.53.62.101",
						Base: &formattedBase{
							Name:    "ubuntu",
							Channel: "22.04",
						},
						JujuStatus:         longStatusInfo,
						MachineStatus:      longStatusInfo,
						ModificationStatus: longStatusInfo,
					},
				},
			},
		},
		Relations: []relationStatus{
			{
				Provider:  "foo:cluster",
				Requirer:  "bar:cluster",
				Interface: "baz",
				Message:   longMessage,
			},
		},
	}

	out := &bytes.Buffer{}
	err := FormatTabular(out, false, status)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out.String(), tc.Equals, `
Model  Controller  Cloud/Region  Version  Notes
m      c           localhost     3.0.0    Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna a...

SAAS  Status  Store    URL
bar   active  unknown  model.io/bar

App  Version  Status  Scale  Charm  Channel        Rev  Exposed  Message
foo           active    0/1  foo    latest/stable    0  no       Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna a...

Unit     Workload  Agent   Machine  Public address  Ports  Message
foo/0    active    active  0        10.53.62.100           Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna a...
  foo/1  active    active  0/lxd/0  10.53.62.101           Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna a...

Machine  State   Address       Inst id  Base          AZ  Message
0        active  10.53.62.100           ubuntu@22.04      Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna a...
0/lxd/0  active  10.53.62.101           ubuntu@22.04      Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna a...

Integration provider  Requirer     Interface  Type  Message
foo:cluster           bar:cluster  baz                Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna a...
`[1:])
}

func (s *StatusSuite) TestStatusWithNilStatusAPI(c *tc.C) {
	ctx := s.newContext()
	ctx.api.result = nil

	code, _, stderr := runStatus(c, ctx, "--no-color", "--format", "tabular")
	c.Check(code, tc.Equals, 1)
	c.Check(stderr, tc.Equals, "ERROR unable to obtain the current status\n")
}

// Filtering Feature
//

func (s *StatusSuite) setupModel(c *tc.C) *ctx {
	ctx := s.newContext()

	steps := []stepper{
		// Given a machine is started
		// And the machine's ID is "0"
		// And the machine's job is to manage the environment
		addMachine{machineId: "0", job: coremodel.JobManageModel},
		startAliveMachine{"0", ""},
		setMachineStatus{"0", status.Started, ""},
		// And the machine's address is "10.0.0.1"
		setAddresses{"0", network.NewSpaceAddresses("10.0.0.1")},
		// And a container is started
		// And the container's ID is "0/lxd/0"
		addContainer{"0", "0/lxd/0", coremodel.JobHostUnits},

		// And the "wordpress" charm is available
		addCharmHubCharm{"wordpress"},
		addApplication{name: "wordpress", charm: "wordpress"},
		// And the "mysql" charm is available
		addCharmHubCharm{"mysql"},
		addApplication{name: "mysql", charm: "mysql"},
		// And the "logging" charm is available
		addCharmHubCharm{"logging"},

		// And a machine is started
		// And the machine's ID is "1"
		// And the machine's job is to host units
		addMachine{machineId: "1", job: coremodel.JobHostUnits},
		startAliveMachine{"1", ""},
		setMachineStatus{"1", status.Started, ""},
		// And the machine's address is "10.0.1.1"
		setAddresses{"1", network.NewSpaceAddresses("10.0.1.1")},
		// And a unit of "wordpress" is deployed to machine "1"
		addAliveUnit{"wordpress", "1"},
		// And the unit is started
		setAgentStatus{"wordpress/0", status.Idle, "", nil},
		setUnitStatus{"wordpress/0", status.Active, "", nil},
		// And a machine is started

		// And the machine's ID is "2"
		// And the machine's job is to host units
		addMachine{machineId: "2", job: coremodel.JobHostUnits},
		startAliveMachine{"2", ""},
		setMachineStatus{"2", status.Started, ""},
		// And the machine's address is "10.0.2.1"
		setAddresses{"2", network.NewSpaceAddresses("10.0.2.1")},
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
	}

	ctx.run(c, steps)
	return ctx
}

func (s *StatusSuite) TestFilterArgs(c *tc.C) {
	ctx := s.newContext()
	ctx.api.expectIncludeStorage = true
	ctx.api.result = nil
	runStatus(c, ctx, "--no-color", "--format", "oneline", "active")
	c.Assert(ctx.api.patterns, tc.DeepEquals, []string{"active"})
}

// TestSummaryStatusWithUnresolvableDns is result of bug# 1410320.
func (s *StatusSuite) TestSummaryStatusWithUnresolvableDns(c *tc.C) {
	formatter := &summaryFormatter{}
	formatter.resolveAndTrackIp("invalidDns")
	// Test should not panic.
}

func initStatusCommand(store jujuclient.ClientStore, args ...string) (*statusCommand, error) {
	com := &statusCommand{}
	com.SetClientStore(store)
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

func (s *StatusSuite) TestStatusCommandInit(c *tc.C) {
	defer os.Setenv(osenv.JujuStatusIsoTimeEnvKey, os.Getenv(osenv.JujuStatusIsoTimeEnvKey))

	for i, t := range statusInitTests {
		c.Logf("test %d", i)
		os.Setenv(osenv.JujuStatusIsoTimeEnvKey, t.envVar)
		com, err := initStatusCommand(s.store, t.args...)
		if t.err != "" {
			c.Check(err, tc.ErrorMatches, t.err)
		} else {
			c.Check(err, tc.ErrorIsNil)
		}
		c.Check(com.isoTime, tc.DeepEquals, t.isoTime)
	}
}

var statusTimeTest = test(
	"status generates timestamps as UTC in ISO format",
	addMachine{machineId: "0", job: coremodel.JobManageModel},
	setAddresses{"0", network.NewSpaceAddresses("10.0.0.1")},
	startAliveMachine{"0", ""},
	setMachineStatus{"0", status.Started, ""},
	addCharmHubCharm{"dummy"},
	addApplication{name: "dummy-application", charm: "dummy"},

	addMachine{machineId: "1", job: coremodel.JobHostUnits},
	recordAgentStartInformation{machineId: "1", hostname: "eldritch-octopii"},
	startAliveMachine{"1", ""},
	setAddresses{"1", network.NewSpaceAddresses("10.0.1.1")},
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

func (s *StatusSuite) TestIsoTimeFormat(c *tc.C) {
	// Prepare ctx and run all steps to setup.
	ctx := s.newContext()
	ctx.expectIsoTime = true
	ctx.run(c, statusTimeTest.steps)
}

func (s *StatusSuite) TestFormatProvisioningError(c *tc.C) {
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
				Base:           params.Base{Name: "ubuntu", Channel: "22.04"},
				Id:             "1",
				Jobs:           []coremodel.MachineJob{"JobHostUnits"},
			},
		},
		ControllerTimestamp: &now,
	}
	isoTime := true
	formatter := NewStatusFormatter(NewStatusFormatterParams{
		Status:  status,
		ISOTime: isoTime,
	})
	formatted, err := formatter.Format()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(formatted, tc.DeepEquals, formattedStatus{
		Model: modelStatus{
			Cloud: "dummy",
		},
		Machines: map[string]machineStatus{
			"1": {
				JujuStatus:        statusInfoContents{Current: "error", Message: "<error while provisioning>"},
				InstanceId:        "pending",
				Base:              &formattedBase{Name: "ubuntu", Channel: "22.04"},
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

func (s *StatusSuite) TestMissingControllerTimestampInFullStatus(c *tc.C) {
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
				Base:           params.Base{Name: "ubuntu", Channel: "22.04"},
				Id:             "1",
				Jobs:           []coremodel.MachineJob{"JobHostUnits"},
			},
		},
	}
	isoTime := true
	formatter := NewStatusFormatter(NewStatusFormatterParams{
		Status:  status,
		ISOTime: isoTime,
	})
	formatted, err := formatter.Format()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(formatted, tc.DeepEquals, formattedStatus{
		Model: modelStatus{
			Cloud: "dummy",
		},
		Machines: map[string]machineStatus{
			"1": {
				JujuStatus:        statusInfoContents{Current: "error", Message: "<error while provisioning>"},
				InstanceId:        "pending",
				Base:              &formattedBase{Name: "ubuntu", Channel: "22.04"},
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

func (s *StatusSuite) TestControllerTimestampInFullStatus(c *tc.C) {
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
				Base:           params.Base{Name: "ubuntu", Channel: "22.04"},
				Id:             "1",
				Jobs:           []coremodel.MachineJob{"JobHostUnits"},
			},
		},
		ControllerTimestamp: &now,
	}
	isoTime := true

	formatter := NewStatusFormatter(NewStatusFormatterParams{
		Status:  status,
		ISOTime: isoTime,
	})
	formatted, err := formatter.Format()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(formatted, tc.DeepEquals, formattedStatus{
		Model: modelStatus{
			Cloud: "dummy",
		},
		Machines: map[string]machineStatus{
			"1": {
				JujuStatus:        statusInfoContents{Current: "error", Message: "<error while provisioning>"},
				InstanceId:        "pending",
				Base:              &formattedBase{Name: "ubuntu", Channel: "22.04"},
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

func (s *StatusSuite) TestTabularNoRelations(c *tc.C) {
	ctx := s.setupModel(c)

	_, stdout, stderr := runStatus(c, ctx, "--no-color")
	c.Assert(stderr, tc.HasLen, 0)
	c.Assert(strings.Contains(stdout, "Integration provider"), tc.IsFalse)
}

func (s *StatusSuite) TestTabularDisplayRelations(c *tc.C) {
	ctx := s.setupModel(c)

	_, stdout, stderr := runStatus(c, ctx, "--no-color", "--relations")
	c.Assert(stderr, tc.HasLen, 0)
	c.Assert(strings.Contains(stdout, "Integration provider"), tc.IsTrue)
}

func (s *StatusSuite) TestNonTabularDisplayRelations(c *tc.C) {
	ctx := s.setupModel(c)
	ctx.api.expectIncludeStorage = true

	_, stdout, stderr := runStatus(c, ctx, "--no-color", "--format=yaml", "--relations")
	c.Assert(stderr, tc.Equals, "provided relations option is always enabled in non tabular formats\n")
	logger.Debugf(context.TODO(), "stdout -> \n%q", stdout)
	c.Assert(strings.Contains(stdout, "    relations:"), tc.IsTrue)
	c.Assert(strings.Contains(stdout, "storage:"), tc.IsTrue)
}

func (s *StatusSuite) TestNonTabularDisplayStorage(c *tc.C) {
	ctx := s.setupModel(c)
	ctx.api.expectIncludeStorage = true

	_, stdout, stderr := runStatus(c, ctx, "--no-color", "--format=yaml", "--storage")
	c.Assert(stderr, tc.Equals, "provided storage option is always enabled in non tabular formats\n")
	c.Assert(strings.Contains(stdout, "    relations:"), tc.IsTrue)
	c.Assert(strings.Contains(stdout, "storage:"), tc.IsTrue)
}

func (s *StatusSuite) TestNonTabularDisplayRelationsAndStorage(c *tc.C) {
	ctx := s.setupModel(c)
	ctx.api.expectIncludeStorage = true

	_, stdout, stderr := runStatus(c, ctx, "--no-color", "--format=yaml", "--relations", "--storage")
	c.Assert(stderr, tc.Equals, "provided relations, storage options are always enabled in non tabular formats\n")
	c.Assert(strings.Contains(stdout, "    relations:"), tc.IsTrue)
	c.Assert(strings.Contains(stdout, "storage:"), tc.IsTrue)
}

func (s *StatusSuite) TestNonTabularRelations(c *tc.C) {
	ctx := s.setupModel(c)
	ctx.api.expectIncludeStorage = true

	_, stdout, stderr := runStatus(c, ctx, "--no-color", "--format=yaml")
	c.Assert(stderr, tc.HasLen, 0)
	c.Assert(strings.Contains(stdout, "    relations:"), tc.IsTrue)
	c.Assert(strings.Contains(stdout, "storage:"), tc.IsTrue)
}

func (s *StatusSuite) TestStatusFormatTabularEmptyModel(c *tc.C) {
	ctx := s.newContext()
	code, stdout, stderr := runStatus(c, ctx, "--no-color")
	c.Check(code, tc.Equals, 0)
	c.Check(stderr, tc.Equals, "\nModel \"controller\" is empty.\n")
	expected := `
Model       Controller  Cloud/Region        Version  Timestamp
controller  kontroll    dummy/dummy-region  1.2.3    15:04:05+07:00
`[1:]
	output := substituteFakeTimestamp(c, stdout, false)
	c.Assert(output, tc.Equals, expected)
}

func (s *StatusSuite) TestStatusFormatTabularForUnmatchedFilter(c *tc.C) {
	ctx := s.newContext()
	code, stdout, stderr := runStatus(c, ctx, "--no-color", "unmatched")
	c.Check(code, tc.Equals, 0)
	c.Check(stderr, tc.Equals, "Nothing matched specified filter.\n")
	expected := `
Model       Controller  Cloud/Region        Version  Timestamp
controller  kontroll    dummy/dummy-region  1.2.3    15:04:05+07:00
`[1:]
	output := substituteFakeTimestamp(c, stdout, false)
	c.Assert(output, tc.Equals, expected)

	_, _, stderr = runStatus(c, ctx, "--no-color", "cannot", "match", "me")
	c.Check(stderr, tc.Equals, "Nothing matched specified filters.\n")
}

func (s *StatusSuite) TestStatusArgs(c *tc.C) {
	cmd, err := initStatusCommand(s.store)
	c.Assert(err, tc.ErrorIsNil)

	statusArgsGNUStyle := []string{"juju", "status", "--relations"}
	expectedArgsGNUStyle := []string{"juju", "status", "--relations", "--color"}
	c.Check(cmd.statusCommandAllArgs(statusArgsGNUStyle), tc.SameContents, expectedArgsGNUStyle)
}
