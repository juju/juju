// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/juju/cmd/juju/status/mocks"
	"github.com/juju/juju/cmd/juju/storage"
	jujumodel "github.com/juju/juju/core/model"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"github.com/kr/pretty"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

const (
	ControllerName = "kontroll"
)

var (
	currentVersion = version.Number{Major: 1, Minor: 2, Patch: 3}
	nextVersion    = version.Number{Major: 1, Minor: 2, Patch: 4}
)

type StatusSuite struct {
	coretesting.BaseSuite
	statusAPI  *mocks.MockstatusAPI
	storageAPI *mocks.MockStorageListAPI
	clock      *mocks.MockClock
	since      time.Time
}

var _ = gc.Suite(&StatusSuite{})

func (s *StatusSuite) setup(c *gc.C, modelName string) *gomock.Controller {
	s.BaseSuite.SetUpTest(c)
	ctrl := gomock.NewController(c)

	s.statusAPI = mocks.NewMockstatusAPI(ctrl)
	s.storageAPI = mocks.NewMockStorageListAPI(ctrl)
	s.clock = mocks.NewMockClock(ctrl)
	s.since = time.Date(2022, 9, 26, 8, 4, 5, 11, time.UTC)
	s.SetModelAndController(c, ControllerName, modelName)
	return ctrl
}

func (s *StatusSuite) newCommand() cmd.Command {
	return NewTestStatusCommand(s.statusAPI, s.storageAPI, s.clock)
}

func (s *StatusSuite) runStatus(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, s.newCommand(), args...)
}

type M map[string]interface{}

type L []interface{}

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
		"charm":        "cs:quantal/logging-1",
		"charm-origin": "charmstore",
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

func dummyCharm(extras M) M {
	charm := M{
		"charm":        "cs:quantal/dummy-1",
		"charm-origin": "charmstore",
		"charm-name":   "dummy",
		"charm-rev":    1,
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

func (s *StatusSuite) TestMigrationInProgress(c *gc.C) {
	defer s.setup(c, "admin/hosted").Finish()

	expected := M{
		"model": M{
			"name":       "hosted",
			"type":       "iaas",
			"controller": "kontroll",
			"cloud":      "dummy",
			"region":     "dummy-region",
			"version":    "2.0.0",
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

	now := time.Now()
	results := params.FullStatus{
		Model: params.ModelStatusInfo{
			Name:        "hosted",
			Type:        "iaas",
			CloudTag:    names.NewCloudTag("dummy").String(),
			Version:     "2.0.0",
			CloudRegion: "dummy-region",
			ModelStatus: params.DetailedStatus{
				Info:   "migrating: foo bar",
				Since:  &now,
				Status: "busy",
			},
			SLA: "unsupported",
		},
		ControllerTimestamp: &now,
	}

	s.statusAPI.EXPECT().Status(gomock.Any()).Times(len(statusFormats)).Return(&results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)

	for _, format := range statusFormats {
		ctx, err := s.runStatus(c, "-m", "admin/hosted", "--format", format.name)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "Model \"admin/hosted\" is empty.\n")

		output := ctx.Stdout.(*bytes.Buffer).Bytes()
		output = substituteFakeTime(c, "since", output, false)
		output = substituteFakeTimestamp(c, output, false)

		// Roundtrip expected through format so that types will match.
		buf, err := format.marshal(expected)
		c.Assert(err, jc.ErrorIsNil)
		var expectedForFormat M
		err = format.unmarshal(buf, &expectedForFormat)
		c.Assert(err, jc.ErrorIsNil)

		var actual M
		c.Assert(format.unmarshal(output, &actual), jc.ErrorIsNil)
		c.Check(actual, jc.DeepEquals, expectedForFormat)
	}
}

func (s *StatusSuite) TestMigrationInProgressTabular(c *gc.C) {
	defer s.setup(c, "admin/hosted").Finish()

	expected := `
Model   Controller  Cloud/Region        Version  SLA          Timestamp       Notes
hosted  kontroll    dummy/dummy-region  2.0.0    unsupported  15:04:05+07:00  migrating: foo bar

`[1:]
	now := time.Now()
	results := params.FullStatus{
		Model: params.ModelStatusInfo{
			Name:        "hosted",
			CloudTag:    names.NewCloudTag("dummy").String(),
			Version:     "2.0.0",
			CloudRegion: "dummy-region",
			ModelStatus: params.DetailedStatus{
				Version: "2.0.0",
				Info:    "migrating: foo bar",
				Since:   &s.since,
				Status:  "busy",
			},
			AvailableVersion: "2.0.0",
			SLA:              "unsupported",
		},
		ControllerTimestamp: &now,
	}

	s.statusAPI.EXPECT().Status(gomock.Any()).Return(&results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)

	ctx, err := s.runStatus(c, "-m", "admin/hosted", "--format", "tabular")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "Model \"admin/hosted\" is empty.\n")

	output := substituteFakeTimestamp(c, ctx.Stdout.(*bytes.Buffer).Bytes(), false)
	c.Assert(string(output), gc.Equals, expected)
}

func (s *StatusSuite) TestMigrationInProgressAndUpgradeAvailable(c *gc.C) {
	defer s.setup(c, "admin/hosted").Finish()

	expected := `
Model   Controller  Cloud/Region        Version  SLA          Timestamp       Notes
hosted  kontroll    dummy/dummy-region  2.0.0    unsupported  15:04:05+07:00  migrating: foo bar

`[1:]

	now := time.Now()
	results := params.FullStatus{
		Model: params.ModelStatusInfo{
			Name:        "hosted",
			CloudTag:    names.NewCloudTag("dummy").String(),
			Version:     "2.0.0",
			CloudRegion: "dummy-region",
			ModelStatus: params.DetailedStatus{
				Version: "2.0.0",
				Info:    "migrating: foo bar",
				Since:   &s.since,
				Status:  "busy",
			},
			AvailableVersion: "2.0.1",
			SLA:              "unsupported",
		},
		ControllerTimestamp: &now,
	}

	s.statusAPI.EXPECT().Status(gomock.Any()).Times(1).Return(&results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)

	ctx, err := s.runStatus(c, "-m", "admin/hosted", "--format", "tabular")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "Model \"admin/hosted\" is empty.\n")

	output := substituteFakeTimestamp(c, ctx.Stdout.(*bytes.Buffer).Bytes(), false)
	c.Assert(string(output), gc.Equals, expected)
}

func (s *StatusSuite) TestStatusWithFormatSummary(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	results := s.prepSummaryData()

	s.statusAPI.EXPECT().Status(gomock.Any()).Times(2).Return(results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)
	status, err := s.statusAPI.Status(nil)
	c.Assert(err, jc.ErrorIsNil)

	formatterParams := newStatusFormatterParams{
		status:         status,
		controllerName: ControllerName,
		showRelations:  true,
	}
	formatter := newStatusFormatter(formatterParams)
	formatted, err := formatter.format()
	var got strings.Builder
	err = FormatOneline(&got, formatted)
	got.WriteString("\n")

	ctx, err := s.runStatus(c, "--format", "summary")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
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
	defer s.setup(c, "admin/controller").Finish()

	results := s.prepOnelineData()

	s.statusAPI.EXPECT().Status(gomock.Any()).Times(4).Return(results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)
	status, err := s.statusAPI.Status(nil)
	c.Assert(err, jc.ErrorIsNil)

	formatterParams := newStatusFormatterParams{
		status:         status,
		controllerName: ControllerName,
		showRelations:  true,
	}
	formatter := newStatusFormatter(formatterParams)
	formatted, err := formatter.format()
	var got strings.Builder
	err = FormatOneline(&got, formatted)
	got.WriteString("\n")

	const expected = `
- mysql/0: 10.0.2.1 (agent:idle, workload:active)
  - logging/1: 10.0.2.1 (agent:idle, workload:error)
- wordpress/0: 10.0.1.1 (agent:idle, workload:active)
  - logging/0: 10.0.1.1 (agent:idle, workload:active)
`
	s.assertOneLineStatus(c, expected)
}

func (s *StatusSuite) assertOneLineStatus(c *gc.C, expected string) {

	ctx, err := s.runStatus(c, "--format", "oneline")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expected)

	c.Log(`Check that "short" is an alias for oneline.`)
	ctx, err = s.runStatus(c, "--format", "short")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expected)

	c.Log(`Check that "line" is an alias for oneline.`)
	ctx, err = s.runStatus(c, "--format", "line")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expected)
}

var expectedTabularStatus = `
Model       Controller  Cloud/Region        Version  SLA          Timestamp       Notes
controller  kontroll    dummy/dummy-region  1.2.3    unsupported  11:04:05+03:00  upgrade available: 1.2.4

SAAS         Status   Store  URL
hosted-riak  unknown  local  me/model.riak

App        Version          Status       Scale  Charm      Channel  Rev  Exposed  Message
logging    a bit too lo...  error            2  logging               1  yes      somehow lost in all those logs
mysql      5.7.13           maintenance    1/2  mysql                 1  yes      installing all the things
wordpress  4.5.3            active           1  wordpress             3  yes      

Unit          Workload     Agent  Machine  Public address  Ports  Message
mysql/0*      maintenance  idle   2        10.0.2.1               installing all the things
  logging/1*  error        idle            10.0.2.1               somehow lost in all those logs
mysql/1       terminated   idle   1        10.0.1.1               gooooone
wordpress/0*  active       idle   1        10.0.1.1               
  logging/0   active       idle            10.0.1.1               

Machine  State    Address   Inst id       Series   AZ          Message
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
	defer s.setup(c, "admin/controller").Finish()

	results := s.prepTabularData()

	s.statusAPI.EXPECT().Status(gomock.Any()).Times(2).Return(results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)
	status, err := s.statusAPI.Status(nil)
	c.Assert(err, jc.ErrorIsNil)

	formatterParams := newStatusFormatterParams{
		status:         status,
		controllerName: ControllerName,
		showRelations:  true,
	}
	formatter := newStatusFormatter(formatterParams)
	formatted, err := formatter.format()
	var got strings.Builder
	err = FormatTabular(&got, false, formatted)
	// we add a newline because output does not go through cmd.WriteFormatter, which is responsible for
	// appending a newline delimiter when you use the default formatter (tabular).
	got.WriteString("\n")
	c.Assert(err, jc.ErrorIsNil)
	ctx, err := s.runStatus(c, "--format", "tabular", "--relations")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Check(cmdtesting.Stdout(ctx), gc.Equals, got.String())
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expectedTabularStatus)
}

func (s *StatusSuite) TestStatusWithFormatTabularValidModel(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	results := s.prepTabularData()
	s.statusAPI.EXPECT().Status(gomock.Any()).Times(2).Return(results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)

	status, err := s.statusAPI.Status(nil)
	c.Assert(err, jc.ErrorIsNil)

	formatterParams := newStatusFormatterParams{
		status:         status,
		controllerName: ControllerName,
		showRelations:  true,
	}
	formatter := newStatusFormatter(formatterParams)
	formatted, err := formatter.format()
	var got strings.Builder
	err = FormatTabular(&got, false, formatted)
	got.WriteString("\n")
	ctx, err := s.runStatus(c, "--format", "tabular", "--relations", "-m", "admin/controller")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Check(cmdtesting.Stdout(ctx), gc.Equals, got.String())
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expectedTabularStatus)
}

func (s *StatusSuite) TestStatusWithFormatYaml(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	results := s.prepTabularData()

	s.statusAPI.EXPECT().Status(gomock.Any()).Times(2).Return(results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)
	status, err := s.statusAPI.Status(nil)
	c.Assert(err, jc.ErrorIsNil)

	formatterParams := newStatusFormatterParams{
		status:         status,
		controllerName: ControllerName,
		storage:        &storage.CombinedStorage{},
	}
	formatter := newStatusFormatter(formatterParams)
	formatted, err := formatter.format()

	var got bytes.Buffer
	err = cmd.FormatYaml(&got, formatted)
	c.Assert(err, jc.ErrorIsNil)
	ctx, err := s.runStatus(c, "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	out := substituteFakeTime(c, "since", got.Bytes(), false)
	expectedOut := substituteFakeTime(c, "since", ctx.Stdout.(*bytes.Buffer).Bytes(), false)
	c.Check(string(expectedOut), gc.Equals, string(out))
	c.Check(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), jc.Contains, "display-name: snowflake")
}

func (s *StatusSuite) TestStatusWithFormatJson(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	results := s.prepTabularData()
	s.statusAPI.EXPECT().Status(gomock.Any()).Times(2).Return(results, nil)

	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)
	status, err := s.statusAPI.Status(nil)
	c.Assert(err, jc.ErrorIsNil)

	formatterParams := newStatusFormatterParams{
		status:         status,
		controllerName: ControllerName,
		storage:        &storage.CombinedStorage{},
	}
	formatter := newStatusFormatter(formatterParams)
	formatted, err := formatter.format()

	var got bytes.Buffer
	err = cmd.FormatJson(&got, formatted)
	c.Assert(err, jc.ErrorIsNil)
	ctx, err := s.runStatus(c, "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	out := substituteFakeTime(c, "since", got.Bytes(), false)
	expectedOut := substituteFakeTime(c, "since", ctx.Stdout.(*bytes.Buffer).Bytes(), false)
	c.Check(string(expectedOut), gc.Equals, string(out))
	c.Check(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), jc.Contains, `"display-name":"snowflake"`)
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
                                 

App  Version  Status  Scale  Charm  Channel  Rev  Exposed  Message
foo                       2                    0  no       

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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out.String(), gc.Equals, `
Model  Controller  Cloud/Region  Version
                                 

App  Version         Status  Scale  Charm  Channel  Rev  Address    Exposed  Message
foo  user/image:tag            1/2                    0  54.32.1.2  no       

Unit   Workload  Agent       Address   Ports   Message
foo/0  active    allocating                    
foo/1  active    running     10.0.0.1  80/TCP  
`[1:])
}

func (s *StatusSuite) TestFormatTabularCAASModelTruncatedVersion(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out.String(), gc.Equals, `
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

func (s *StatusSuite) TestFormatTabularStatusMessage(c *gc.C) {
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
                                 

App  Version  Status  Scale  Charm  Channel  Rev  Address    Exposed  Message
foo                     0/1                    0  54.32.1.2  no       Error: ImagePullBackOff

Unit   Workload  Agent       Address   Ports   Message
foo/0  waiting   allocating  10.0.0.1  80/TCP  
`[1:])
}

func (s *StatusSuite) TestStatusWithNilStatusAPI(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	s.statusAPI.EXPECT().Status(gomock.Any()).Return(nil, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()

	_, err := s.runStatus(c, "--format", "tabular")
	c.Check(err.Error(), gc.Equals, "unable to obtain the current status")
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
                                 

App  Version  Status  Scale  Charm  Channel  Rev  Exposed  Message
foo                     0/2                    0  no       

Unit   Workload  Agent  Machine  Public address  Ports  Message
foo/0                                                   
foo/1                                                   

Entity  Meter status  Message
foo/0   strange       warning: stable strangelets  
foo/1   up            things are looking up        
`[1:])
}

// Filtering Feature
//
// Scenario: One unit is in an errored state and user filters to active
func (s *StatusSuite) TestFilterToActive(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	results := s.prepFilteringData()

	s.statusAPI.EXPECT().Status(gomock.Any()).Return(results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)

	// When I run juju status --format oneline started
	ctx, err := s.runStatus(c, "--format", "oneline", "active")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	// Then I should receive output prefixed with:
	const expected = `

- wordpress/0: 10.0.1.1 (agent:idle, workload:active)
  - logging/0: 10.0.1.1 (agent:idle, workload:active)
`
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expected[1:])
}

// Scenario: user filters to a single machine
func (s *StatusSuite) TestFilterToMachine(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	results := s.prepFilteringData()

	s.statusAPI.EXPECT().Status(gomock.Any()).Return(results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)

	// When I run juju status --format oneline 1
	ctx, err := s.runStatus(c, "--format", "oneline", "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	// Then I should receive output prefixed with:
	const expected = `

- wordpress/0: 10.0.1.1 (agent:idle, workload:active)
  - logging/0: 10.0.1.1 (agent:idle, workload:active)
`
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expected[1:])
}

// Scenario: user filters to a machine, shows containers
func (s *StatusSuite) TestFilterToMachineShowsContainer(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	results := s.prepFilteringData()
	s.statusAPI.EXPECT().Status(gomock.Any()).Return(results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)

	// When I run juju status --format yaml 0
	ctx, err := s.runStatus(c, "--format", "yaml", "0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	// Then I should receive output matching:
	const expected = "(.|\n)*machines:(.|\n)*\"0\"(.|\n)*0/lxd/0(.|\n)*"
	c.Assert(cmdtesting.Stdout(ctx), gc.Matches, expected)
}

// Scenario: user filters to a container
func (s *StatusSuite) TestFilterToContainer(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()
	results := s.prepFilteringData()
	results.Applications = map[string]params.ApplicationStatus{}

	s.statusAPI.EXPECT().Status(gomock.Any()).Return(results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)

	// When I run juju status --format yaml 0/lxd/0
	ctx, err := s.runStatus(c, "--format", "yaml", "0/lxd/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")

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
	isoTime := false
	stdout := ctx.Stdout.(*bytes.Buffer).Bytes()
	out := substituteFakeTime(c, "since", stdout, isoTime)
	out = substituteFakeTimestamp(c, out, isoTime)
	c.Assert(string(out), gc.Equals, expected)
}

// Scenario: One unit is in an errored state and user filters to errored
func (s *StatusSuite) TestFilterToErrored(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	results := params.FullStatus{
		Model: params.ModelStatusInfo{
			Name:        "controller",
			Type:        "iaas",
			Version:     currentVersion.String(),
			CloudTag:    names.NewCloudTag("dummy").String(),
			CloudRegion: "dummy-region",
			ModelStatus: params.DetailedStatus{
				//Version: currentVersion.String(),
				Since:  &s.since,
				Status: "available",
			},
			SLA: "unsupported",
		},
		Machines: map[string]params.MachineStatus{
			"0": {
				AgentStatus: params.DetailedStatus{
					Status: "started",
					Since:  &s.since,
				},
				Containers: map[string]params.MachineStatus{
					"0/lxd/0": {
						Id:   "0/lxd/0",
						Jobs: []jujumodel.MachineJob{jujumodel.JobHostUnits},
						AgentStatus: params.DetailedStatus{
							Status: "pending",
							Since:  &s.since,
						},
						InstanceStatus: params.DetailedStatus{
							Status: "pending",
							Since:  &s.since,
						},
						ModificationStatus: params.DetailedStatus{
							Status: "idle",
							Since:  &s.since,
						},
						Series:     "quantal",
						InstanceId: "pending",
					},
				},
				InstanceStatus: params.DetailedStatus{
					Status: "pending",
					Since:  &s.since,
				},
				ModificationStatus: params.DetailedStatus{
					Status: "idle",
					Since:  &s.since,
				},
				DNSName:     "10.0.0.1",
				IPAddresses: []string{"10.0.0.1"},
				InstanceId:  "controller-0",
				Series:      "quantal",
				Id:          "0",
				NetworkInterfaces: map[string]params.NetworkInterface{
					"eth0": {
						IPAddresses: []string{"10.0.0.1"},
						MACAddress:  "aa:bb:cc:dd:ee:ff",
						IsUp:        true,
					},
				},
				Hardware:  "arch=amd64 cores=1 mem=1024M root-disk=8192M",
				Jobs:      []jujumodel.MachineJob{jujumodel.JobManageModel},
				HasVote:   false,
				WantsVote: true,
			},
		},
		Applications: map[string]params.ApplicationStatus{
			"wordpress": {
				Charm:           "local:focal/wordpress-3",
				WorkloadVersion: "4.5.3",
				Series:          "quantal",
				Exposed:         true,
				Relations:       map[string][]string{"db": {"mysql"}, "logging-dir": {"logging"}},
				Status: params.DetailedStatus{
					Status: "active",
					Since:  &s.since,
				},
			},
			"logging": {
				Charm:         "local:focal/logging-1",
				Series:        "quantal",
				Exposed:       true,
				SubordinateTo: []string{"wordpress"},
				Status: params.DetailedStatus{
					Info:   "somehow lost in all those logs",
					Status: "error",
					Since:  &s.since,
				},
				WorkloadVersion: "a bit too lo...",
			},
			"mysql": {
				Charm:           "local:focal/mysql-1",
				WorkloadVersion: "5.7.13",
				Series:          "quantal",
				Exposed:         true,
				Units: map[string]params.UnitStatus{
					"mysql/0": {
						AgentStatus: params.DetailedStatus{
							Status: "idle",
							Since:  &s.since,
							Info:   "mock error",
						},
						WorkloadStatus: params.DetailedStatus{
							Status: "active",
							Info:   "installing all the things",
							Since:  &s.since,
						},
						Machine:       "2",
						PublicAddress: "10.0.2.1",
						Subordinates: map[string]params.UnitStatus{
							"logging/1": {
								AgentStatus: params.DetailedStatus{
									Status: "idle",
									Since:  &s.since,
								},
								WorkloadStatus: params.DetailedStatus{
									Info:   "mock error",
									Status: "error",
									Since:  &s.since,
								},
								PublicAddress: "10.0.2.1",
								Leader:        true,
							},
						},
					},
				},
				Status: params.DetailedStatus{
					Status: "maintenance",
					Info:   "installing all the things",
					Since:  &s.since,
				},
				EndpointBindings: map[string]string{
					"":               "alpha",
					"metrics-client": "alpha",
					"server":         "alpha",
					"server-admin":   "alpha",
				},
				Scale: 0,
			},
		},
		ControllerTimestamp: &s.since,
	}

	s.statusAPI.EXPECT().Status(gomock.Any()).Return(&results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)

	// When I run juju status --format oneline error
	ctx, err := s.runStatus(c, "--format", "oneline", "error")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	// Then I should receive output prefixed with:
	const expected = `

- mysql/0: 10.0.2.1 (agent:idle, workload:active)
  - logging/1: 10.0.2.1 (agent:idle, workload:error)
`
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expected[1:])
}

// Scenario: User filters to mysql application
func (s *StatusSuite) TestFilterToApplication(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()
	results := params.FullStatus{
		Model: params.ModelStatusInfo{
			Name:        "controller",
			Type:        "iaas",
			Version:     currentVersion.String(),
			CloudTag:    names.NewCloudTag("dummy").String(),
			CloudRegion: "dummy-region",
			ModelStatus: params.DetailedStatus{
				Since:  &s.since,
				Status: "available",
			},
			SLA: "unsupported",
		},
		Machines: map[string]params.MachineStatus{
			"0": {
				AgentStatus: params.DetailedStatus{
					Status: "started",
					Since:  &s.since,
				},
				InstanceStatus: params.DetailedStatus{
					Status: "pending",
					Since:  &s.since,
				},
				ModificationStatus: params.DetailedStatus{
					Status: "idle",
					Since:  &s.since,
				},
				DNSName:     "10.0.0.1",
				IPAddresses: []string{"10.0.0.1"},
				InstanceId:  "controller-0",
				Series:      "quantal",
				Id:          "0",
				NetworkInterfaces: map[string]params.NetworkInterface{
					"eth0": {
						IPAddresses: []string{"10.0.0.1"},
						MACAddress:  "aa:bb:cc:dd:ee:ff",
						IsUp:        true,
					},
				},
				Hardware:  "arch=amd64 cores=1 mem=1024M root-disk=8192M",
				Jobs:      []jujumodel.MachineJob{jujumodel.JobManageModel},
				HasVote:   false,
				WantsVote: true,
			},
		},
		Applications: map[string]params.ApplicationStatus{
			"wordpress": {
				Charm:           "local:focal/wordpress-3",
				WorkloadVersion: "4.5.3",
				Series:          "quantal",
				Exposed:         true,
				Relations:       map[string][]string{"db": {"mysql"}, "logging-dir": {"logging"}},
				Status: params.DetailedStatus{
					Status: "active",
					Since:  &s.since,
				},
			},
			"logging": {
				Charm:         "local:focal/logging-1",
				Series:        "quantal",
				Exposed:       true,
				SubordinateTo: []string{"wordpress"},
				Status: params.DetailedStatus{
					Info:   "somehow lost in all those logs",
					Status: "error",
					Since:  &s.since,
				},
				WorkloadVersion: "a bit too lo...",
			},
			"mysql": {
				Charm:           "local:focal/mysql-1",
				WorkloadVersion: "5.7.13",
				Series:          "quantal",
				Exposed:         true,
				Units: map[string]params.UnitStatus{
					"mysql/0": {
						AgentStatus: params.DetailedStatus{
							Status: "idle",
							Since:  &s.since,
							Info:   "mock error",
						},
						WorkloadStatus: params.DetailedStatus{
							Status: "active",
							Info:   "installing all the things",
							Since:  &s.since,
						},
						Machine:       "2",
						PublicAddress: "10.0.2.1",
						Subordinates: map[string]params.UnitStatus{
							"logging/1": {
								AgentStatus: params.DetailedStatus{
									Status: "idle",
									Since:  &s.since,
								},
								WorkloadStatus: params.DetailedStatus{
									Status: "active",
									Since:  &s.since,
								},
								PublicAddress: "10.0.2.1",
								Leader:        true,
							},
						},
					},
				},
				Status: params.DetailedStatus{
					Status: "maintenance",
					Info:   "installing all the things",
					Since:  &s.since,
				},
			},
		},
		ControllerTimestamp: &s.since,
	}

	s.statusAPI.EXPECT().Status(gomock.Any()).Return(&results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)

	// When I run juju status --format oneline error
	ctx, err := s.runStatus(c, "--format", "oneline", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	// Then I should receive output prefixed with:
	const expected = `

- mysql/0: 10.0.2.1 (agent:idle, workload:active)
  - logging/1: 10.0.2.1 (agent:idle, workload:active)
`

	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expected[1:])
}

// Scenario: User filters to exposed applications
func (s *StatusSuite) TestFilterToExposedApplication(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	//// Given unit 1 of the "mysql" application is exposed
	results := params.FullStatus{
		Model: params.ModelStatusInfo{
			Name:        "controller",
			Type:        "iaas",
			Version:     currentVersion.String(),
			CloudTag:    names.NewCloudTag("dummy").String(),
			CloudRegion: "dummy-region",
			ModelStatus: params.DetailedStatus{
				Since:  &s.since,
				Status: "available",
			},
			SLA: "unsupported",
		},
		Machines: map[string]params.MachineStatus{
			"0": {
				AgentStatus: params.DetailedStatus{
					Status: "started",
					Since:  &s.since,
				},
				InstanceStatus: params.DetailedStatus{
					Status: "pending",
					Since:  &s.since,
				},
				ModificationStatus: params.DetailedStatus{
					Status: "idle",
					Since:  &s.since,
				},
				DNSName:     "10.0.0.1",
				IPAddresses: []string{"10.0.0.1"},
				InstanceId:  "controller-0",
				Series:      "quantal",
				Id:          "0",
				NetworkInterfaces: map[string]params.NetworkInterface{
					"eth0": {
						IPAddresses: []string{"10.0.0.1"},
						MACAddress:  "aa:bb:cc:dd:ee:ff",
						IsUp:        true,
					},
				},
				Hardware:  "arch=amd64 cores=1 mem=1024M root-disk=8192M",
				Jobs:      []jujumodel.MachineJob{jujumodel.JobManageModel},
				HasVote:   false,
				WantsVote: true,
			},
		},
		Applications: map[string]params.ApplicationStatus{
			"wordpress": {
				Charm:           "local:focal/wordpress-3",
				WorkloadVersion: "4.5.3",
				Series:          "quantal",
				Exposed:         true,
				Relations:       map[string][]string{"db": {"mysql"}, "logging-dir": {"logging"}},
				Status: params.DetailedStatus{
					Status: "active",
					Since:  &s.since,
				},
			},
			"logging": {
				Charm:         "local:focal/logging-1",
				Series:        "quantal",
				Exposed:       true,
				SubordinateTo: []string{"wordpress"},
				Status: params.DetailedStatus{
					Info:   "somehow lost in all those logs",
					Status: "error",
					Since:  &s.since,
				},
				WorkloadVersion: "a bit too lo...",
			},
			"mysql": {
				Charm:           "local:focal/mysql-1",
				WorkloadVersion: "5.7.13",
				Series:          "quantal",
				Exposed:         true,
				Units: map[string]params.UnitStatus{
					"mysql/0": {
						AgentStatus: params.DetailedStatus{
							Status: "idle",
							Since:  &s.since,
							Info:   "mock error",
						},
						WorkloadStatus: params.DetailedStatus{
							Status: "active",
							Info:   "installing all the things",
							Since:  &s.since,
						},
						Machine:       "2",
						PublicAddress: "10.0.2.1",
						Subordinates: map[string]params.UnitStatus{
							"logging/1": {
								AgentStatus: params.DetailedStatus{
									Status: "idle",
									Since:  &s.since,
								},
								WorkloadStatus: params.DetailedStatus{
									Status: "active",
									Since:  &s.since,
								},
								PublicAddress: "10.0.2.1",
								Leader:        true,
							},
						},
					},
				},
				Status: params.DetailedStatus{
					Status: "maintenance",
					Info:   "installing all the things",
					Since:  &s.since,
				},
			},
		},
		ControllerTimestamp: &s.since,
	}

	s.statusAPI.EXPECT().Status(gomock.Any()).Return(&results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)

	// When I run juju status --format oneline exposed
	ctx, err := s.runStatus(c, "--format", "oneline", "exposed")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	// Then I should receive output prefixed with:
	const expected = `

- mysql/0: 10.0.2.1 (agent:idle, workload:active)
  - logging/1: 10.0.2.1 (agent:idle, workload:active)
`
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expected[1:])
}

// Scenario: User filters to non-exposed applications
func (s *StatusSuite) TestFilterToNotExposedApplication(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	results := params.FullStatus{
		Model: params.ModelStatusInfo{
			Name:        "controller",
			Type:        "iaas",
			Version:     currentVersion.String(),
			CloudTag:    names.NewCloudTag("dummy").String(),
			CloudRegion: "dummy-region",
			ModelStatus: params.DetailedStatus{
				Since:  &s.since,
				Status: "available",
			},
			SLA: "unsupported",
		},
		Machines: map[string]params.MachineStatus{
			"0": {
				AgentStatus: params.DetailedStatus{
					Status: "started",
					Since:  &s.since,
				},
				InstanceStatus: params.DetailedStatus{
					Status: "pending",
					Since:  &s.since,
				},
				ModificationStatus: params.DetailedStatus{
					Status: "idle",
					Since:  &s.since,
				},
				DNSName:     "10.0.0.1",
				IPAddresses: []string{"10.0.0.1"},
				InstanceId:  "controller-0",
				Series:      "quantal",
				Id:          "0",
				NetworkInterfaces: map[string]params.NetworkInterface{
					"eth0": {
						IPAddresses: []string{"10.0.0.1"},
						MACAddress:  "aa:bb:cc:dd:ee:ff",
						IsUp:        true,
					},
				},
				Hardware:  "arch=amd64 cores=1 mem=1024M root-disk=8192M",
				Jobs:      []jujumodel.MachineJob{jujumodel.JobManageModel},
				HasVote:   false,
				WantsVote: true,
			},
		},
		Applications: map[string]params.ApplicationStatus{
			"wordpress": {
				Charm:           "local:focal/wordpress-3",
				WorkloadVersion: "4.5.3",
				Series:          "quantal",
				Exposed:         false,
				Units: map[string]params.UnitStatus{
					"wordpress/0": {
						AgentStatus: params.DetailedStatus{
							Status: "idle",
							Since:  &s.since,
							Info:   "mock error",
						},
						WorkloadStatus: params.DetailedStatus{
							Status: "active",
							Info:   "installing all the things",
							Since:  &s.since,
						},
						Machine:       "0",
						PublicAddress: "10.0.1.1",
						Subordinates: map[string]params.UnitStatus{
							"logging/0": {
								AgentStatus: params.DetailedStatus{
									Status: "idle",
									Since:  &s.since,
								},
								WorkloadStatus: params.DetailedStatus{
									Status: "active",
									Since:  &s.since,
								},
								PublicAddress: "10.0.1.1",
								Leader:        true,
							},
						},
					},
				},
				Status: params.DetailedStatus{
					Status: "active",
					Since:  &s.since,
				},
			},
		},
		ControllerTimestamp: &s.since,
	}

	s.statusAPI.EXPECT().Status(gomock.Any()).Return(&results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)
	// When I run juju status --format oneline not exposed
	ctx, err := s.runStatus(c, "--format", "oneline", "not", "exposed")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	// Then I should receive output prefixed with:
	const expected = `

- wordpress/0: 10.0.1.1 (agent:idle, workload:active)
  - logging/0: 10.0.1.1 (agent:idle, workload:active)
`
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expected[1:])
}

// Scenario: Filtering on Subnets
func (s *StatusSuite) TestFilterOnSubnet(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	results := params.FullStatus{
		Model: params.ModelStatusInfo{
			Name:        "controller",
			Type:        "iaas",
			Version:     currentVersion.String(),
			CloudTag:    names.NewCloudTag("dummy").String(),
			CloudRegion: "dummy-region",
			ModelStatus: params.DetailedStatus{
				Since:  &s.since,
				Status: "available",
			},
			SLA: "unsupported",
		},
		Applications: map[string]params.ApplicationStatus{
			"wordpress": {
				Charm:           "local:focal/wordpress-3",
				WorkloadVersion: "4.5.3",
				Series:          "quantal",
				Exposed:         true,
				Units: map[string]params.UnitStatus{
					"wordpress/0": {
						AgentStatus: params.DetailedStatus{
							Status: "idle",
							Since:  &s.since,
							Info:   "mock error",
						},
						WorkloadStatus: params.DetailedStatus{
							Status: "active",
							Info:   "installing all the things",
							Since:  &s.since,
						},
						Machine:       "1",
						PublicAddress: "localhost",
						Subordinates: map[string]params.UnitStatus{
							"logging/0": {
								AgentStatus: params.DetailedStatus{
									Status: "idle",
									Since:  &s.since,
								},
								WorkloadStatus: params.DetailedStatus{
									Status: "active",
									Since:  &s.since,
								},
								PublicAddress: "localhost",
								Leader:        true,
							},
						},
					},
				},
				Status: params.DetailedStatus{
					Status: "active",
					Since:  &s.since,
				},
			},
		},
		ControllerTimestamp: &s.since,
	}

	s.statusAPI.EXPECT().Status(gomock.Any()).Return(&results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)
	// When I run juju status --format oneline 127.0.0.1
	ctx, err := s.runStatus(c, "--format", "oneline", "127.0.0.1")
	c.Assert(err, gc.IsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	// Then I should receive output prefixed with:
	const expected = `

- wordpress/0: localhost (agent:idle, workload:active)
  - logging/0: localhost (agent:idle, workload:active)
`
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expected[1:])
}

// Scenario: Filtering on Ports
func (s *StatusSuite) TestFilterOnPorts(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	results := params.FullStatus{
		Model: params.ModelStatusInfo{
			Name:        "controller",
			Type:        "iaas",
			Version:     currentVersion.String(),
			CloudTag:    names.NewCloudTag("dummy").String(),
			CloudRegion: "dummy-region",
			ModelStatus: params.DetailedStatus{
				Since:  &s.since,
				Status: "available",
			},
			SLA: "unsupported",
		},
		Applications: map[string]params.ApplicationStatus{
			"wordpress": {
				Charm:           "local:focal/wordpress-3",
				WorkloadVersion: "4.5.3",
				Series:          "quantal",
				Exposed:         true,
				Units: map[string]params.UnitStatus{
					"wordpress/0": {
						AgentStatus: params.DetailedStatus{
							Status: "idle",
							Since:  &s.since,
							Info:   "mock error",
						},
						OpenedPorts: []string{"80/tcp"},
						WorkloadStatus: params.DetailedStatus{
							Status: "active",
							Info:   "installing all the things",
							Since:  &s.since,
						},
						Machine:       "1",
						PublicAddress: "localhost",
						Subordinates: map[string]params.UnitStatus{
							"logging/0": {
								AgentStatus: params.DetailedStatus{
									Status: "idle",
									Since:  &s.since,
								},
								WorkloadStatus: params.DetailedStatus{
									Status: "active",
									Since:  &s.since,
								},
								PublicAddress: "localhost",
								Leader:        true,
							},
						},
					},
				},
				Status: params.DetailedStatus{
					Status: "active",
					Since:  &s.since,
				},
			},
		},
		ControllerTimestamp: &s.since,
	}

	s.statusAPI.EXPECT().Status(gomock.Any()).Return(&results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)
	// When I run juju status --format oneline 80/tcp
	ctx, err := s.runStatus(c, "--format", "oneline", "80/tcp")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	// Then I should receive output prefixed with:
	const expected = `

- wordpress/0: localhost (agent:idle, workload:active) 80/tcp
  - logging/0: localhost (agent:idle, workload:active)
`
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expected[1:])
}

// Scenario: User filters out a parent, but not its subordinate
func (s *StatusSuite) TestFilterParentButNotSubordinate(c *gc.C) {
	s.setup(c, "admin/controller").Finish()

	results := params.FullStatus{
		Model: params.ModelStatusInfo{
			Name:        "controller",
			Type:        "iaas",
			Version:     currentVersion.String(),
			CloudTag:    names.NewCloudTag("dummy").String(),
			CloudRegion: "dummy-region",
			ModelStatus: params.DetailedStatus{
				Since:  &s.since,
				Status: "available",
			},
			SLA: "unsupported",
		},
		Applications: map[string]params.ApplicationStatus{
			"wordpress": {
				Charm:           "local:focal/wordpress-3",
				WorkloadVersion: "4.5.3",
				Series:          "quantal",
				Exposed:         true,
				Units: map[string]params.UnitStatus{
					"wordpress/0": {
						AgentStatus: params.DetailedStatus{
							Status:  "idle",
							Since:   &s.since,
							Version: currentVersion.String(),
						},
						WorkloadStatus: params.DetailedStatus{
							Status: "active",
							Since:  &s.since,
						},
						Machine:       "1",
						PublicAddress: "10.0.1.1",
						Subordinates: map[string]params.UnitStatus{
							"logging/0": {
								AgentStatus: params.DetailedStatus{
									Status:  "idle",
									Since:   &s.since,
									Version: currentVersion.String(),
								},
								WorkloadStatus: params.DetailedStatus{
									Status: "active",
									Since:  &s.since,
								},
								PublicAddress: "10.0.1.1",
								Leader:        false,
							},
						},
						Leader: true,
					},
				},
				Status: params.DetailedStatus{
					Status: "active",
					Since:  &s.since,
				},
			},
			"mysql": {
				Charm:           "local:focal/mysql-1",
				WorkloadVersion: "5.7.13",
				Series:          "quantal",
				Exposed:         true,
				Relations: map[string][]string{
					"server": {"wordpress"},
				},
				Units: map[string]params.UnitStatus{
					"mysql/0": {
						AgentStatus: params.DetailedStatus{
							Status:  "idle",
							Since:   &s.since,
							Version: currentVersion.String(),
						},
						WorkloadStatus: params.DetailedStatus{
							Status: "active",
							Info:   "installing all the things",
							Since:  &s.since,
						},
						Machine:       "2",
						PublicAddress: "10.0.2.1",
						Subordinates: map[string]params.UnitStatus{
							"logging/1": {
								AgentStatus: params.DetailedStatus{
									Status:  "idle",
									Since:   &s.since,
									Version: currentVersion.String(),
								},
								WorkloadStatus: params.DetailedStatus{
									Info:   "somehow lost in all those logs",
									Status: "active",
									Since:  &s.since,
								},
								PublicAddress: "10.0.2.1",
								Leader:        true,
							},
						},
						Leader: true,
					},
				},
			},
		},
		ControllerTimestamp: &s.since,
	}

	s.statusAPI.EXPECT().Status(gomock.Any()).Return(&results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)
	// When I run juju status --format oneline logging
	ctx, err := s.runStatus(c, "--format", "oneline", "logging")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	// Then I should receive output prefixed with:
	const expected = `

- mysql/0: 10.0.2.1 (agent:idle, workload:active)
  - logging/1: 10.0.2.1 (agent:idle, workload:active)
- wordpress/0: 10.0.1.1 (agent:idle, workload:active)
  - logging/0: 10.0.1.1 (agent:idle, workload:active)
`
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expected[1:])
}

// Scenario: User filters out a subordinate, but not its parent
func (s *StatusSuite) TestFilterSubordinateButNotParent(c *gc.C) {
	s.setup(c, "admin/controller").Finish()

	results := params.FullStatus{
		Model: params.ModelStatusInfo{
			Name:        "controller",
			Type:        "iaas",
			Version:     currentVersion.String(),
			CloudTag:    names.NewCloudTag("dummy").String(),
			CloudRegion: "dummy-region",
			ModelStatus: params.DetailedStatus{
				Since:  &s.since,
				Status: "available",
			},
			SLA: "unsupported",
		},
		Applications: map[string]params.ApplicationStatus{
			"mysql": {
				Charm:           "local:focal/mysql-1",
				WorkloadVersion: "5.7.13",
				Series:          "quantal",
				Exposed:         false,
				Relations: map[string][]string{
					"server": {"wordpress"},
				},
				Units: map[string]params.UnitStatus{
					"mysql/0": {
						AgentStatus: params.DetailedStatus{
							Status:  "idle",
							Since:   &s.since,
							Version: currentVersion.String(),
						},
						WorkloadStatus: params.DetailedStatus{
							Status: "active",
							Info:   "installing all the things",
							Since:  &s.since,
						},
						Machine:       "2",
						PublicAddress: "10.0.2.1",
						Subordinates: map[string]params.UnitStatus{
							"logging/1": {
								AgentStatus: params.DetailedStatus{
									Status:  "idle",
									Since:   &s.since,
									Version: currentVersion.String(),
								},
								WorkloadStatus: params.DetailedStatus{
									Info:   "somehow lost in all those logs",
									Status: "active",
									Since:  &s.since,
								},
								PublicAddress: "10.0.2.1",
								Leader:        true,
							},
						},
						Leader: true,
					},
				},
			},
		},
		ControllerTimestamp: &s.since,
	}

	s.statusAPI.EXPECT().Status(gomock.Any()).Return(&results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)
	// Given the wordpress application is exposed
	//setApplicationExposed{"wordpress", true}.step(c, ctx)
	// When I run juju status --format oneline not exposed
	ctx, err := s.runStatus(c, "--format", "oneline", "not", "exposed")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	const expected = `

- mysql/0: 10.0.2.1 (agent:idle, workload:active)
  - logging/1: 10.0.2.1 (agent:idle, workload:active)
`
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expected[1:])
}

func (s *StatusSuite) TestFilterMultipleHomogenousPatterns(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	results := params.FullStatus{
		Model: params.ModelStatusInfo{
			Name:        "controller",
			Type:        "iaas",
			Version:     currentVersion.String(),
			CloudTag:    names.NewCloudTag("dummy").String(),
			CloudRegion: "dummy-region",
			ModelStatus: params.DetailedStatus{
				Since:  &s.since,
				Status: "available",
			},
			SLA: "unsupported",
		},
		Applications: map[string]params.ApplicationStatus{
			"wordpress": {
				Charm:           "local:focal/wordpress-3",
				WorkloadVersion: "4.5.3",
				Series:          "quantal",
				Exposed:         true,
				Units: map[string]params.UnitStatus{
					"wordpress/0": {
						AgentStatus: params.DetailedStatus{
							Status:  "idle",
							Since:   &s.since,
							Version: currentVersion.String(),
						},
						WorkloadStatus: params.DetailedStatus{
							Status: "active",
							Since:  &s.since,
						},
						Machine:       "1",
						PublicAddress: "10.0.1.1",
						Subordinates: map[string]params.UnitStatus{
							"logging/0": {
								AgentStatus: params.DetailedStatus{
									Status:  "idle",
									Since:   &s.since,
									Version: currentVersion.String(),
								},
								WorkloadStatus: params.DetailedStatus{
									Status: "active",
									Since:  &s.since,
								},
								PublicAddress: "10.0.1.1",
								Leader:        false,
							},
						},
						Leader: true,
					},
				},
				Status: params.DetailedStatus{
					Status: "active",
					Since:  &s.since,
				},
			},
			"mysql": {
				Charm:           "local:focal/mysql-1",
				WorkloadVersion: "5.7.13",
				Series:          "quantal",
				Exposed:         true,
				Relations: map[string][]string{
					"server": {"wordpress"},
				},
				Units: map[string]params.UnitStatus{
					"mysql/0": {
						AgentStatus: params.DetailedStatus{
							Status:  "idle",
							Since:   &s.since,
							Version: currentVersion.String(),
						},
						WorkloadStatus: params.DetailedStatus{
							Status: "active",
							Info:   "installing all the things",
							Since:  &s.since,
						},
						Machine:       "2",
						PublicAddress: "10.0.2.1",
						Subordinates: map[string]params.UnitStatus{
							"logging/1": {
								AgentStatus: params.DetailedStatus{
									Status:  "idle",
									Since:   &s.since,
									Version: currentVersion.String(),
								},
								WorkloadStatus: params.DetailedStatus{
									Info:   "somehow lost in all those logs",
									Status: "active",
									Since:  &s.since,
								},
								PublicAddress: "10.0.2.1",
								Leader:        true,
							},
						},
						Leader: true,
					},
				},
			},
		},
		ControllerTimestamp: &s.since,
	}

	s.statusAPI.EXPECT().Status(gomock.Any()).Return(&results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)

	ctx, err := s.runStatus(c, "--format", "oneline", "wordpress/0", "mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	// Then I should receive output prefixed with:
	const expected = `

- mysql/0: 10.0.2.1 (agent:idle, workload:active)
  - logging/1: 10.0.2.1 (agent:idle, workload:active)
- wordpress/0: 10.0.1.1 (agent:idle, workload:active)
  - logging/0: 10.0.1.1 (agent:idle, workload:active)
`
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expected[1:])
}

func (s *StatusSuite) TestFilterMultipleHeterogenousPatterns(c *gc.C) {
	//ctx := s.FilteringTestSetup(c)
	//defer s.resetContext(c, ctx)
	defer s.setup(c, "admin/controller").Finish()

	results := params.FullStatus{
		Model: params.ModelStatusInfo{
			Name:        "controller",
			Type:        "iaas",
			Version:     currentVersion.String(),
			CloudTag:    names.NewCloudTag("dummy").String(),
			CloudRegion: "dummy-region",
			ModelStatus: params.DetailedStatus{
				Since:  &s.since,
				Status: "available",
			},
			SLA: "unsupported",
		},
		Applications: map[string]params.ApplicationStatus{
			"wordpress": {
				Charm:           "local:focal/wordpress-3",
				WorkloadVersion: "4.5.3",
				Series:          "quantal",
				Exposed:         true,
				Units: map[string]params.UnitStatus{
					"wordpress/0": {
						AgentStatus: params.DetailedStatus{
							Status:  "idle",
							Since:   &s.since,
							Version: currentVersion.String(),
						},
						WorkloadStatus: params.DetailedStatus{
							Status: "active",
							Since:  &s.since,
						},
						Machine:       "1",
						PublicAddress: "10.0.1.1",
						Subordinates: map[string]params.UnitStatus{
							"logging/0": {
								AgentStatus: params.DetailedStatus{
									Status:  "idle",
									Since:   &s.since,
									Version: currentVersion.String(),
								},
								WorkloadStatus: params.DetailedStatus{
									Status: "active",
									Since:  &s.since,
								},
								PublicAddress: "10.0.1.1",
								Leader:        false,
							},
						},
						Leader: true,
					},
				},
				Status: params.DetailedStatus{
					Status: "active",
					Since:  &s.since,
				},
			},
			"mysql": {
				Charm:           "local:focal/mysql-1",
				WorkloadVersion: "5.7.13",
				Series:          "quantal",
				Exposed:         true,
				Relations: map[string][]string{
					"server": {"wordpress"},
				},
				Units: map[string]params.UnitStatus{
					"mysql/0": {
						AgentStatus: params.DetailedStatus{
							Status:  "idle",
							Since:   &s.since,
							Version: currentVersion.String(),
						},
						WorkloadStatus: params.DetailedStatus{
							Status: "active",
							Info:   "installing all the things",
							Since:  &s.since,
						},
						Machine:       "2",
						PublicAddress: "10.0.2.1",
						Subordinates: map[string]params.UnitStatus{
							"logging/1": {
								AgentStatus: params.DetailedStatus{
									Status:  "idle",
									Since:   &s.since,
									Version: currentVersion.String(),
								},
								WorkloadStatus: params.DetailedStatus{
									Info:   "somehow lost in all those logs",
									Status: "active",
									Since:  &s.since,
								},
								PublicAddress: "10.0.2.1",
								Leader:        true,
							},
						},
						Leader: true,
					},
				},
			},
		},
		ControllerTimestamp: &s.since,
	}

	s.statusAPI.EXPECT().Status(gomock.Any()).Return(&results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)

	ctx, err := s.runStatus(c, "--format", "oneline", "wordpress/0", "active")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	// Then I should receive output prefixed with:
	const expected = `

- mysql/0: 10.0.2.1 (agent:idle, workload:active)
  - logging/1: 10.0.2.1 (agent:idle, workload:active)
- wordpress/0: 10.0.1.1 (agent:idle, workload:active)
  - logging/0: 10.0.1.1 (agent:idle, workload:active)
`
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expected[1:])
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

func (s *StatusSuite) TestStatusCommandInit(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

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

func (s *StatusSuite) TestIsoTimeFormat(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	now := time.Now()
	results := params.FullStatus{
		Model: params.ModelStatusInfo{
			Name:        "controller",
			Type:        "iaas",
			Version:     currentVersion.String(),
			CloudTag:    names.NewCloudTag("dummy").String(),
			CloudRegion: "dummy-region",
			ModelStatus: params.DetailedStatus{
				Since:  &s.since,
				Status: "available",
			},
			SLA: "unsupported",
		},
		Machines: map[string]params.MachineStatus{
			"0": {AgentStatus: params.DetailedStatus{
				Status: "started",
				Since:  &s.since,
			},
				InstanceStatus: params.DetailedStatus{
					Status: "pending",
					Since:  &s.since,
				},
				ModificationStatus: params.DetailedStatus{
					Status: "idle",
					Since:  &s.since,
				},
				DNSName:     "10.0.0.1",
				IPAddresses: []string{"10.0.0.1"},
				InstanceId:  "controller-0",
				Series:      "quantal",
				Id:          "0",
				NetworkInterfaces: map[string]params.NetworkInterface{
					"eth0": {
						IPAddresses: []string{"10.0.0.1"},
						MACAddress:  "aa:bb:cc:dd:ee:ff",
						IsUp:        true,
					},
				},
				Hardware:  "arch=amd64 cores=1 mem=1024M root-disk=8192M",
				Jobs:      []jujumodel.MachineJob{jujumodel.JobManageModel},
				HasVote:   false,
				WantsVote: true,
			},
			"1": {
				AgentStatus: params.DetailedStatus{
					Status: "started",
					Since:  &s.since,
				},
				InstanceStatus: params.DetailedStatus{
					Status: "pending",
					Since:  &s.since,
				},
				ModificationStatus: params.DetailedStatus{
					Status: "idle",
					Since:  &s.since,
				},
				Hostname:    "eldritch-octopii",
				DNSName:     "10.0.1.1",
				IPAddresses: []string{"10.0.1.1"},
				InstanceId:  "controller-1",
				Series:      "quantal",
				Id:          "1",
				NetworkInterfaces: map[string]params.NetworkInterface{"eth0": {
					IPAddresses: []string{"10.0.1.1"},
					MACAddress:  "aa:bb:cc:dd:ee:ff",
					IsUp:        true,
				},
				},
				Hardware:  "arch=amd64 cores=1 mem=1024M root-disk=8192M",
				Jobs:      []jujumodel.MachineJob{jujumodel.JobHostUnits},
				HasVote:   false,
				WantsVote: true,
			},
		},
		Applications: map[string]params.ApplicationStatus{
			"dummy-application": {
				Charm:   "cs:quantal/dummy-1",
				Series:  "quantal",
				Exposed: false,
				Units: map[string]params.UnitStatus{
					"dummy-application/0": {
						AgentStatus: params.DetailedStatus{
							Status: "allocating",
							Since:  &s.since,
						},
						WorkloadStatus: params.DetailedStatus{
							Status: "waiting",
							Info:   "waiting for machine",
							Since:  &s.since,
						},
						Machine:       "1",
						PublicAddress: "10.0.1.1",
					},
				},
				Status: params.DetailedStatus{
					Status: "waiting",
					Info:   "waiting for machine",
					Since:  &s.since,
				},
				Scale: 0,
			},
		},
		ControllerTimestamp: &now,
	}

	output := M{
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
	}

	s.statusAPI.EXPECT().Status(gomock.Any()).AnyTimes().Return(&results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)

	for _, format := range statusFormats {
		ctx, err := s.runStatus(c, "--format", format.name, "--utc")
		c.Assert(err, jc.ErrorIsNil)
		stdout := ctx.Stdout.(*bytes.Buffer).Bytes()

		// Prepare the output in the same format.
		buf, err := format.marshal(output)
		c.Assert(err, jc.ErrorIsNil)

		// we have to force the timestamp into the correct format as the model
		// is in string.
		buf = substituteFakeTimestamp(c, buf, true)

		expected := make(M)
		err = format.unmarshal(buf, &expected)
		c.Assert(err, jc.ErrorIsNil)

		// Check the output is as expected.
		actual := make(M)
		out := substituteFakeTime(c, "since", stdout, true)
		out = substituteFakeTimestamp(c, out, true)
		err = format.unmarshal(out, &actual)
		c.Assert(err, jc.ErrorIsNil)
		pretty.Ldiff(c, actual, expected)
		c.Assert(actual, jc.DeepEquals, expected)
	}
}

func (s *StatusSuite) TestFormatProvisioningError(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	now := time.Now()
	results := params.FullStatus{
		Model: params.ModelStatusInfo{
			CloudTag: names.NewCloudTag("dummy").String(),
		},
		Machines: map[string]params.MachineStatus{
			"1": {
				AgentStatus: params.DetailedStatus{
					Status: "error",
					Info:   "<error while provisioning>",
				},
				InstanceId: "pending",
				Series:     "trusty",
				Id:         "1",
			},
		},
		ControllerTimestamp: &now,
	}

	s.statusAPI.EXPECT().Status(gomock.Any()).Times(1).Return(&results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)

	status, err := s.statusAPI.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
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
		Branches: map[string]branchStatus{},
	})
}

func (s *StatusSuite) TestMissingControllerTimestampInFullStatus(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	results := params.FullStatus{
		Model: params.ModelStatusInfo{
			CloudTag: names.NewCloudTag("dummy").String(),
		},
		Machines: map[string]params.MachineStatus{
			"1": {
				AgentStatus: params.DetailedStatus{
					Status: "error",
					Info:   "<error while provisioning>",
				},
				InstanceId: "pending",
				Series:     "trusty",
				Id:         "1",
			},
		},
	}

	s.statusAPI.EXPECT().Status(gomock.Any()).Times(1).Return(&results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)

	status, err := s.statusAPI.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
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
		Branches:           map[string]branchStatus{},
	})
}

func (s *StatusSuite) TestControllerTimestampInFullStatus(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	now := time.Now()
	results := params.FullStatus{
		Model: params.ModelStatusInfo{
			CloudTag: names.NewCloudTag("dummy").String(),
		},
		Machines: map[string]params.MachineStatus{
			"1": {
				AgentStatus: params.DetailedStatus{
					Status: "error",
					Info:   "<error while provisioning>",
				},
				InstanceId: "pending",
				Series:     "trusty",
				Id:         "1",
			},
		},
		ControllerTimestamp: &now,
	}

	s.statusAPI.EXPECT().Status(gomock.Any()).Times(1).Return(&results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)

	status, err := s.statusAPI.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
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
		Branches: map[string]branchStatus{},
	})
}

func (s *StatusSuite) TestTabularNoRelations(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	results := s.prepTabularData()

	s.statusAPI.EXPECT().Status(gomock.Any()).Times(2).Return(results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)

	_, err := s.statusAPI.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	ctx, err := s.runStatus(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(strings.Contains(cmdtesting.Stdout(ctx), "Relation provider"), jc.IsFalse)
}

func (s *StatusSuite) TestTabularDisplayRelations(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	results := s.prepTabularData()

	s.statusAPI.EXPECT().Status(gomock.Any()).Times(2).Return(results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)

	_, err := s.statusAPI.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	ctx, err := s.runStatus(c, "--relations")
	c.Assert(err, gc.IsNil)
	c.Assert(strings.Contains(cmdtesting.Stdout(ctx), "Relation provider"), jc.IsTrue)
}

func (s *StatusSuite) TestNonTabularDisplayRelations(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	results := s.prepTabularData()

	s.statusAPI.EXPECT().Status(gomock.Any()).Times(2).Return(results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)

	_, err := s.statusAPI.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	ctx, err := s.runStatus(c, "--format=yaml", "--relations")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "provided relations option is always enabled in non tabular formats\n")
	c.Assert(strings.Contains(cmdtesting.Stdout(ctx), "    relations:"), jc.IsTrue)
	c.Assert(strings.Contains(cmdtesting.Stdout(ctx), "storage:"), jc.IsTrue)
}

func (s *StatusSuite) TestNonTabularDisplayStorage(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	results := s.prepTabularData()

	s.statusAPI.EXPECT().Status(gomock.Any()).Times(2).Return(results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)

	_, err := s.statusAPI.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	ctx, err := s.runStatus(c, "--format=yaml", "--storage")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "provided storage option is always enabled in non tabular formats\n")
	c.Assert(strings.Contains(cmdtesting.Stdout(ctx), "    relations:"), jc.IsTrue)
	c.Assert(strings.Contains(cmdtesting.Stdout(ctx), "storage:"), jc.IsTrue)
}

func (s *StatusSuite) TestNonTabularDisplayRelationsAndStorage(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	results := s.prepTabularData()

	s.statusAPI.EXPECT().Status(gomock.Any()).Times(2).Return(results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)
	_, err := s.statusAPI.Status(nil)
	c.Assert(err, jc.ErrorIsNil)

	ctx, err := s.runStatus(c, "--format=yaml", "--relations", "--storage")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "provided relations, storage options are always enabled in non tabular formats\n")
	c.Assert(strings.Contains(cmdtesting.Stdout(ctx), "    relations:"), jc.IsTrue)
	c.Assert(strings.Contains(cmdtesting.Stdout(ctx), "storage:"), jc.IsTrue)
}

func (s *StatusSuite) TestNonTabularRelations(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	results := s.prepTabularData()

	s.statusAPI.EXPECT().Status(gomock.Any()).Times(2).Return(results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)
	_, err := s.statusAPI.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	ctx, err := s.runStatus(c, "--format=yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(strings.Contains(cmdtesting.Stdout(ctx), "    relations:"), jc.IsTrue)
	c.Assert(strings.Contains(cmdtesting.Stdout(ctx), "storage:"), jc.IsTrue)
}

func (s *StatusSuite) TestStatusFormatTabularEmptyModel(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	results := params.FullStatus{
		Model: params.ModelStatusInfo{
			Name:        "controller",
			Type:        "iaas",
			Version:     currentVersion.String(),
			CloudTag:    names.NewCloudTag("dummy").String(),
			CloudRegion: "dummy-region",
			ModelStatus: params.DetailedStatus{
				Version: currentVersion.String(),
				Since:   &s.since,
				Status:  "available",
			},
			SLA: "unsupported",
		},
		ControllerTimestamp: &s.since,
	}

	s.statusAPI.EXPECT().Status(gomock.Any()).Times(2).Return(&results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)
	_, err := s.statusAPI.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	ctx, err := s.runStatus(c)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(cmdtesting.Stderr(ctx), gc.Equals, "Model \"admin/controller\" is empty.\n")
	expected := `
Model       Controller  Cloud/Region        Version  SLA          Timestamp
controller  kontroll    dummy/dummy-region  1.2.3    unsupported  15:04:05+07:00

`[1:]
	output := substituteFakeTimestamp(c, ctx.Stdout.(*bytes.Buffer).Bytes(), false)
	c.Assert(string(output), gc.Equals, expected)
}

func (s *StatusSuite) TestStatusFormatTabularForUnmatchedFilter(c *gc.C) {
	defer s.setup(c, "admin/controller").Finish()

	results := params.FullStatus{
		Model: params.ModelStatusInfo{
			Name:        "controller",
			Type:        "iaas",
			Version:     currentVersion.String(),
			CloudTag:    names.NewCloudTag("dummy").String(),
			CloudRegion: "dummy-region",
			ModelStatus: params.DetailedStatus{
				Version: currentVersion.String(),
				Since:   &s.since,
				Status:  "available",
			},
			SLA: "unsupported",
		},
		ControllerTimestamp: &s.since,
	}

	s.statusAPI.EXPECT().Status(gomock.Any()).Times(3).Return(&results, nil)
	s.statusAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().Close().AnyTimes()
	s.storageAPI.EXPECT().ListFilesystems([]string{}).AnyTimes().Return([]params.FilesystemDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListVolumes([]string{}).AnyTimes().Return([]params.VolumeDetailsListResult{}, nil)
	s.storageAPI.EXPECT().ListStorageDetails().AnyTimes().Return([]params.StorageDetails{}, nil)
	_, err := s.statusAPI.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	ctx, err := s.runStatus(c, "unmatch")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), gc.Equals, "Nothing matched specified filter.\n")

	ctx, err = s.runStatus(c, "cannot", "match", "me")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), gc.Equals, "Nothing matched specified filters.\n")
	expected := `
Model       Controller  Cloud/Region        Version  SLA          Timestamp
controller  kontroll    dummy/dummy-region  1.2.3    unsupported  15:04:05+07:00

`[1:]
	output := substituteFakeTimestamp(c, ctx.Stdout.(*bytes.Buffer).Bytes(), false)
	c.Assert(string(output), gc.Equals, expected)
}

func (s *StatusSuite) prepOnelineData() *params.FullStatus {
	results := params.FullStatus{
		Model: params.ModelStatusInfo{
			Name:        "controller",
			Type:        "iaas",
			Version:     currentVersion.String(),
			CloudTag:    names.NewCloudTag("dummy").String(),
			CloudRegion: "dummy-region",
			ModelStatus: params.DetailedStatus{
				Version: currentVersion.String(),
				Since:   &s.since,
				Status:  "available",
			},
			AvailableVersion: nextVersion.String(),
			SLA:              "unsupported",
		},
		Machines: map[string]params.MachineStatus{
			"0": {AgentStatus: params.DetailedStatus{
				Status:  "started",
				Since:   &s.since,
				Version: currentVersion.String(),
			},
				InstanceStatus: params.DetailedStatus{
					Status: "started",
					Since:  &s.since,
				},
				ModificationStatus: params.DetailedStatus{
					Status: "applied",
					Since:  &s.since,
				},
				Hostname:    "controller-0",
				DNSName:     "10.0.1.1",
				IPAddresses: []string{"10.0.1.1"},
				InstanceId:  "snowflake",
				DisplayName: "snowflake",
				Series:      "quantal",
				Id:          "0",
				NetworkInterfaces: map[string]params.NetworkInterface{
					"eth0": {
						IPAddresses: []string{"10.0.1.1"},
						MACAddress:  "00:16:3e:23:1f:1f ",
						Gateway:     "10.0.0.1",
						Space:       "alpha",
						IsUp:        true,
					},
				},
				Hardware:    "availability-zone=us-east-1a",
				Constraints: "arch=amd64 Hardware:arch=amd64 cores=0 mem=0M",
				Jobs:        []jujumodel.MachineJob{jujumodel.JobManageModel},
				HasVote:     false,
				WantsVote:   false,
			},
			"1": {
				AgentStatus: params.DetailedStatus{
					Status:  "started",
					Since:   &s.since,
					Version: currentVersion.String(),
				},
				InstanceStatus: params.DetailedStatus{
					Status: "starting",
					Since:  &s.since,
				},
				ModificationStatus: params.DetailedStatus{
					Status: "applied",
					Since:  &s.since,
				},
				Hostname:    "snowflake",
				DNSName:     "10.0.1.1",
				IPAddresses: []string{"10.0.1.1"},
				Series:      "quantal",
				Id:          "1",
				NetworkInterfaces: map[string]params.NetworkInterface{"eth0": {
					IPAddresses: []string{"10.0.1.1"},
					MACAddress:  "00:16:3e:23:f2:25",
					Gateway:     "10.0.0.0",
					Space:       "alpha",
					IsUp:        true,
				},
				},
				Constraints: "arch=amd64 Hardware:arch=amd64 cores=0 mem=0M",
				Jobs:        []jujumodel.MachineJob{jujumodel.JobHostUnits},
				HasVote:     false,
				WantsVote:   false,
			},
			"2": {
				AgentStatus: params.DetailedStatus{
					Status:  "started",
					Since:   &s.since,
					Version: currentVersion.String(),
				},
				InstanceStatus: params.DetailedStatus{
					Status: "started",
					Since:  &s.since,
				}, ModificationStatus: params.DetailedStatus{Status: "applied",
					Since: &s.since,
				},
				Hostname:    "controller-2",
				DNSName:     "10.0.2.1",
				IPAddresses: []string{"10.0.2.1"},
				InstanceId:  "controller-2",
				Series:      "quantal",
				Id:          "2",
				NetworkInterfaces: map[string]params.NetworkInterface{"eth0": {
					IPAddresses: []string{"10.0.2.1"},
					MACAddress:  "00:16:3e:0f:76:de",
					Gateway:     "10.0.0.0",
					Space:       "alpha",
					IsUp:        true,
				},
				},
				Constraints: "arch=amd64 Hardware:arch=amd64 cores=0 mem=0M",
				Jobs:        []jujumodel.MachineJob{jujumodel.JobHostUnits},
				HasVote:     false,
				WantsVote:   false,
			},
		},
		Applications: map[string]params.ApplicationStatus{
			"logging": {
				Charm:   "local:focal/logging-1",
				Series:  "quantal",
				Exposed: true,
				Relations: map[string][]string{
					"logging-directory": {"wordpress"},
				},
				SubordinateTo: []string{"wordpress"},
				Status: params.DetailedStatus{
					Info:   "somehow lost in all those logs",
					Status: "error",
					Since:  &s.since,
				},
				EndpointBindings: map[string]string{
					"":                  "alpha",
					"info":              "alpha",
					"logging-client":    "alpha",
					"logging-directory": "alpha",
				},
				Scale:           0,
				WorkloadVersion: "a bit too lo...",
			},
			"mysql": {
				Charm:           "local:focal/mysql-1",
				WorkloadVersion: "5.7.13",
				Series:          "quantal",
				Exposed:         true,
				Relations: map[string][]string{
					"server": {"wordpress"},
				},
				Units: map[string]params.UnitStatus{
					"mysql/0": {
						AgentStatus: params.DetailedStatus{
							Status:  "idle",
							Since:   &s.since,
							Version: currentVersion.String(),
						},
						WorkloadStatus: params.DetailedStatus{
							Status: "active",
							Info:   "installing all the things",
							Since:  &s.since,
						},
						Machine:       "2",
						PublicAddress: "10.0.2.1",
						Subordinates: map[string]params.UnitStatus{
							"logging/1": {
								AgentStatus: params.DetailedStatus{
									Status:  "idle",
									Since:   &s.since,
									Version: currentVersion.String(),
								},
								WorkloadStatus: params.DetailedStatus{
									Info:   "somehow lost in all those logs",
									Status: "error",
									Since:  &s.since,
								},
								PublicAddress: "10.0.2.1",
								Leader:        true,
							},
						},
						Leader: true,
					},
				},
				Status: params.DetailedStatus{
					Status: "maintenance",
					Info:   "installing all the things",
					Since:  &s.since,
				},
				EndpointBindings: map[string]string{
					"":               "alpha",
					"metrics-client": "alpha",
					"server":         "alpha",
					"server-admin":   "alpha",
				},
				Scale: 0,
			},
			"wordpress": {
				Charm:           "local:focal/wordpress-3",
				WorkloadVersion: "4.5.3",
				Series:          "quantal",
				Exposed:         true,
				Relations:       map[string][]string{"db": {"mysql"}, "logging-dir": {"logging"}},
				Units: map[string]params.UnitStatus{
					"wordpress/0": {
						AgentStatus: params.DetailedStatus{
							Status:  "idle",
							Since:   &s.since,
							Version: currentVersion.String(),
						},
						WorkloadStatus: params.DetailedStatus{
							Status: "active",
							Since:  &s.since,
						},
						Machine:       "1",
						PublicAddress: "10.0.1.1",
						Subordinates: map[string]params.UnitStatus{
							"logging/0": {
								AgentStatus: params.DetailedStatus{
									Status:  "idle",
									Since:   &s.since,
									Version: currentVersion.String(),
								},
								WorkloadStatus: params.DetailedStatus{
									Status: "active",
									Since:  &s.since,
								},
								PublicAddress: "10.0.1.1",
								Leader:        false,
							},
						},
						Leader: true,
					},
				},
				Status: params.DetailedStatus{
					Status: "active",
					Since:  &s.since,
				},
				EndpointBindings: map[string]string{"": "alpha",
					"admin-api":       "alpha",
					"cache":           "alpha",
					"db":              "alpha",
					"db-client":       "alpha",
					"foo-bar":         "alpha",
					"logging-dir":     "alpha",
					"monitoring-port": "alpha",
					"url":             "alpha"},
				Scale: 0,
			},
		},
		Relations: []params.RelationStatus{
			{
				Id:        0,
				Key:       "logging:info mysql:juju-info",
				Interface: "juju-info",
				Scope:     "container",
				Endpoints: []params.EndpointStatus{
					{
						ApplicationName: "mysql",
						Name:            "juju-info",
						Role:            "provider",
						Subordinate:     false,
					},
					{
						ApplicationName: "logging",
						Name:            "info",
						Role:            "requirer",
						Subordinate:     true,
					},
				},
				Status: params.DetailedStatus{
					Status: "joined",
					Since:  &s.since,
				},
			},
			{
				Id:        1,
				Key:       "wordpress:db mysql:server",
				Interface: "mysql",
				Scope:     "global",
				Endpoints: []params.EndpointStatus{
					{
						ApplicationName: "wordpress",
						Name:            "db",
						Role:            "requirer",
						Subordinate:     false,
					},
					{
						ApplicationName: "mysql",
						Name:            "server",
						Role:            "provider",
						Subordinate:     false,
					},
				},
				Status: params.DetailedStatus{
					Status: "suspended",
					Since:  &s.since,
				},
			},
		},
		ControllerTimestamp: &s.since,
	}

	return &results
}

func (s *StatusSuite) prepTabularData() *params.FullStatus {

	results := params.FullStatus{
		Model: params.ModelStatusInfo{
			Name:        "controller",
			Type:        "iaas",
			Version:     currentVersion.String(),
			CloudTag:    names.NewCloudTag("dummy").String(),
			CloudRegion: "dummy-region",
			ModelStatus: params.DetailedStatus{
				Version: currentVersion.String(),
				Since:   &s.since,
				Status:  "available",
			},
			AvailableVersion: nextVersion.String(),
			SLA:              "unsupported",
		},
		Machines: map[string]params.MachineStatus{
			"0": {AgentStatus: params.DetailedStatus{
				Status:  "started",
				Since:   &s.since,
				Version: currentVersion.String(),
			},
				InstanceStatus: params.DetailedStatus{
					Status: "started",
					Since:  &s.since,
				},
				ModificationStatus: params.DetailedStatus{
					Status: "applied",
					Since:  &s.since,
				},
				Hostname:    "controller-0",
				DNSName:     "10.0.0.1",
				IPAddresses: []string{"10.0.0.1"},
				InstanceId:  "controller-0",
				DisplayName: "controller-0",
				Series:      "quantal",
				Id:          "0",
				NetworkInterfaces: map[string]params.NetworkInterface{
					"eth0": {
						IPAddresses: []string{"10.0.0.1"},
						MACAddress:  "00:16:3e:23:1f:1f ",
						Gateway:     "10.0.0.1",
						Space:       "alpha",
						IsUp:        true,
					},
				},
				Hardware:    "availability-zone=us-east-1a",
				Constraints: "arch=amd64 Hardware:arch=amd64 cores=0 mem=0M",
				Jobs:        []jujumodel.MachineJob{jujumodel.JobHostUnits},
				HasVote:     false,
				WantsVote:   false,
			},
			"1": {
				AgentStatus: params.DetailedStatus{
					Status:  "started",
					Since:   &s.since,
					Version: currentVersion.String(),
				},
				InstanceStatus: params.DetailedStatus{
					Status: "starting",
					Since:  &s.since,
				},
				ModificationStatus: params.DetailedStatus{
					Status: "applied",
					Since:  &s.since,
				},
				Hostname:    "snowflake",
				DNSName:     "10.0.1.1",
				IPAddresses: []string{"10.0.1.1"},
				InstanceId:  "snowflake",
				DisplayName: "snowflake",
				Series:      "quantal",
				Id:          "1",
				NetworkInterfaces: map[string]params.NetworkInterface{"eth0": {
					IPAddresses: []string{"10.0.1.1"},
					MACAddress:  "00:16:3e:23:f2:25",
					Gateway:     "10.0.0.0",
					Space:       "alpha",
					IsUp:        true,
				},
				},
				Constraints: "arch=amd64 Hardware:arch=amd64 cores=0 mem=0M",
				Jobs:        []jujumodel.MachineJob{jujumodel.JobHostUnits},
				HasVote:     false,
				WantsVote:   false,
			},
			"2": {
				AgentStatus: params.DetailedStatus{
					Status:  "started",
					Since:   &s.since,
					Version: currentVersion.String(),
				},
				InstanceStatus: params.DetailedStatus{
					Status: "started",
					Since:  &s.since,
				}, ModificationStatus: params.DetailedStatus{Status: "applied",
					Since: &s.since,
				},
				Hostname:    "controller-2",
				DNSName:     "10.0.2.1",
				IPAddresses: []string{"10.0.2.1"},
				InstanceId:  "controller-2",
				Series:      "quantal",
				Id:          "2",
				NetworkInterfaces: map[string]params.NetworkInterface{"eth0": {
					IPAddresses: []string{"10.0.2.1"},
					MACAddress:  "00:16:3e:0f:76:de",
					Gateway:     "10.0.0.0",
					Space:       "alpha",
					IsUp:        true,
				},
				},
				Constraints: "arch=amd64 Hardware:arch=amd64 cores=0 mem=0M",
				Jobs:        []jujumodel.MachineJob{jujumodel.JobHostUnits},
				HasVote:     false,
				WantsVote:   false,
			},
			"3": {
				AgentStatus: params.DetailedStatus{
					Status:  "started",
					Since:   &s.since,
					Version: currentVersion.String(),
				},
				InstanceStatus: params.DetailedStatus{
					Status: "started",
					Info:   "I am number three",
					Since:  &s.since,
				},
				ModificationStatus: params.DetailedStatus{
					Status: "idle",
					Since:  &s.since,
				},
				Hostname:    "controller-3",
				DNSName:     "10.0.3.1",
				IPAddresses: []string{"10.0.3.1"},
				InstanceId:  "controller-3",
				DisplayName: "controller-3",
				Series:      "quantal",
				Id:          "3",
				NetworkInterfaces: map[string]params.NetworkInterface{"eth0": {
					IPAddresses: []string{"10.0.3.1"},
					MACAddress:  "00:16:3e:98:f9:19",
					Gateway:     "10.0.0.0",
					Space:       "alpha",
					IsUp:        true,
				},
				},
				Hardware:  "arch=amd64 cores=0 mem=0M",
				Jobs:      []jujumodel.MachineJob{jujumodel.JobHostUnits},
				HasVote:   false,
				WantsVote: false,
			},
			"4": {
				AgentStatus: params.DetailedStatus{
					Status: "error",
					Since:  &s.since,
				},
				InstanceStatus: params.DetailedStatus{
					Status: "error",
					Info:   "I am an error",
					Since:  &s.since,
				},
				ModificationStatus: params.DetailedStatus{
					Status: "applied",
					Since:  &s.since,
				},
				Hostname:    "controller-4",
				DNSName:     "10.0.3.1",
				IPAddresses: []string{"10.0.3.1"},
				NetworkInterfaces: map[string]params.NetworkInterface{"eth0": {
					IPAddresses: []string{"10.0.3.1"},
					MACAddress:  "00:16:3e:98:f9:19",
					Gateway:     "10.0.0.0",
					Space:       "alpha",
					IsUp:        true,
				},
				},
				InstanceId:  "controller-4",
				Series:      "quantal",
				Id:          "4",
				Constraints: "arch=amd64 Hardware:arch=amd64 cores=0 mem=0M",
				Jobs:        []jujumodel.MachineJob{jujumodel.JobHostUnits},
				HasVote:     false,
				WantsVote:   false,
			},
		},
		Applications: map[string]params.ApplicationStatus{
			"logging": {
				Charm:   "local:focal/logging-1",
				Series:  "quantal",
				Exposed: true,
				Relations: map[string][]string{
					"logging-directory": {"wordpress"},
				},
				SubordinateTo: []string{"wordpress"},
				Status: params.DetailedStatus{
					Info:   "somehow lost in all those logs",
					Status: "error",
					Since:  &s.since,
				},
				EndpointBindings: map[string]string{
					"":                  "alpha",
					"info":              "alpha",
					"logging-client":    "alpha",
					"logging-directory": "alpha",
				},
				Scale:           0,
				WorkloadVersion: "a bit too lo...",
			},
			"mysql": {
				Charm:           "local:focal/mysql-1",
				WorkloadVersion: "5.7.13",
				Series:          "quantal",
				Exposed:         true,
				Relations: map[string][]string{
					"server": {"wordpress"},
				},
				Units: map[string]params.UnitStatus{
					"mysql/0": {
						AgentStatus: params.DetailedStatus{
							Status:  "idle",
							Since:   &s.since,
							Version: currentVersion.String(),
						},
						WorkloadStatus: params.DetailedStatus{
							Status: "maintenance",
							Info:   "installing all the things",
							Since:  &s.since,
						},
						Machine:       "2",
						PublicAddress: "10.0.2.1",
						Subordinates: map[string]params.UnitStatus{
							"logging/1": {
								AgentStatus: params.DetailedStatus{
									Status:  "idle",
									Since:   &s.since,
									Version: currentVersion.String(),
								},
								WorkloadStatus: params.DetailedStatus{
									Info:   "somehow lost in all those logs",
									Status: "error",
									Since:  &s.since,
								},
								PublicAddress: "10.0.2.1",
								Leader:        true,
							},
						},
						Leader: true,
					},
					"mysql/1": {
						AgentStatus: params.DetailedStatus{
							Status:  "idle",
							Since:   &s.since,
							Version: currentVersion.String(),
						},
						WorkloadStatus: params.DetailedStatus{
							Status: "terminated",
							Info:   "gooooone",
							Since:  &s.since,
						},
						Machine:       "1",
						PublicAddress: "10.0.1.1",
						Leader:        false,
					},
				},
				Status: params.DetailedStatus{
					Status: "maintenance",
					Info:   "installing all the things",
					Since:  &s.since,
				},
				EndpointBindings: map[string]string{
					"":               "alpha",
					"metrics-client": "alpha",
					"server":         "alpha",
					"server-admin":   "alpha",
				},
				Scale: 0,
			},
			"wordpress": {
				Charm:           "local:focal/wordpress-3",
				WorkloadVersion: "4.5.3",
				Series:          "quantal",
				Exposed:         true,
				Relations:       map[string][]string{"db": {"mysql"}, "logging-dir": {"logging"}},
				Units: map[string]params.UnitStatus{
					"wordpress/0": {
						AgentStatus: params.DetailedStatus{
							Status:  "idle",
							Since:   &s.since,
							Version: currentVersion.String(),
						},
						WorkloadStatus: params.DetailedStatus{
							Status: "active",
							Since:  &s.since,
						},
						Machine:       "1",
						PublicAddress: "10.0.1.1",
						Subordinates: map[string]params.UnitStatus{
							"logging/0": {
								AgentStatus: params.DetailedStatus{
									Status:  "idle",
									Since:   &s.since,
									Version: currentVersion.String(),
								},
								WorkloadStatus: params.DetailedStatus{
									Status: "active",
									Since:  &s.since,
								},
								PublicAddress: "10.0.1.1",
								Leader:        false,
							},
						},
						Leader: true,
					},
				},
				Status: params.DetailedStatus{
					Status: "active",
					Since:  &s.since,
				},
				EndpointBindings: map[string]string{"": "alpha",
					"admin-api":       "alpha",
					"cache":           "alpha",
					"db":              "alpha",
					"db-client":       "alpha",
					"foo-bar":         "alpha",
					"logging-dir":     "alpha",
					"monitoring-port": "alpha",
					"url":             "alpha"},
				Scale: 0,
			},
		},
		RemoteApplications: map[string]params.RemoteApplicationStatus{
			"hosted-riak": {
				OfferURL:  "me/model.riak",
				OfferName: "hosted-riak",
				Endpoints: []params.RemoteEndpoint{
					{
						Name:      "server",
						Role:      charm.RelationRole("provider"),
						Interface: "mysql",
						Limit:     0,
					},
				},
				Relations: map[string][]string{
					"server": {
						"remote-wordpress",
					},
				},
				Status: params.DetailedStatus{
					Status: "unknown",
					Since:  &s.since,
				},
			},
		},
		Offers: map[string]params.ApplicationOfferStatus{
			"hosted-mysql": {
				OfferName:       "hosted-mysql",
				ApplicationName: "mysql",
				CharmURL:        "local:focal/mysql-1",
				Endpoints: map[string]params.RemoteEndpoint{
					"server": {
						Name:      "server",
						Role:      charm.RelationRole("provider"),
						Interface: "mysql",
						Limit:     0,
					},
				},
				ActiveConnectedCount: 1,
				TotalConnectedCount:  1,
			},
		},
		Relations: []params.RelationStatus{
			{
				Id:        0,
				Key:       "logging:info mysql:juju-info",
				Interface: "juju-info",
				Scope:     "container",
				Endpoints: []params.EndpointStatus{
					{
						ApplicationName: "mysql",
						Name:            "juju-info",
						Role:            "provider",
						Subordinate:     false,
					},
					{
						ApplicationName: "logging",
						Name:            "info",
						Role:            "requirer",
						Subordinate:     true,
					},
				},
				Status: params.DetailedStatus{
					Status: "joined",
					Since:  &s.since,
				},
			},
			{
				Id:        1,
				Key:       "wordpress:db mysql:server",
				Interface: "mysql",
				Scope:     "global",
				Endpoints: []params.EndpointStatus{
					{
						ApplicationName: "wordpress",
						Name:            "db",
						Role:            "requirer",
						Subordinate:     false,
					},
					{
						ApplicationName: "mysql",
						Name:            "server",
						Role:            "provider",
						Subordinate:     false,
					},
				},
				Status: params.DetailedStatus{
					Status: "suspended",
					Since:  &s.since,
				},
			},
			{
				Id:        2,
				Key:       "logging:logging-directory wordpress:logging-dir",
				Interface: "logging",
				Scope:     "container",
				Endpoints: []params.EndpointStatus{
					{
						ApplicationName: "wordpress",
						Name:            "logging-dir",
						Role:            "provider",
						Subordinate:     false,
					},
					{
						ApplicationName: "logging",
						Name:            "logging-directory",
						Role:            "requirer",
						Subordinate:     true,
					},
				},
				Status: params.DetailedStatus{
					Status: "joined",
					Since:  &s.since,
				},
			},
		},
		ControllerTimestamp: &s.since,
	}

	return &results
}

func (s *StatusSuite) prepFilteringData() *params.FullStatus {
	results := params.FullStatus{
		Model: params.ModelStatusInfo{
			Name:        "controller",
			Type:        "iaas",
			Version:     currentVersion.String(),
			CloudTag:    names.NewCloudTag("dummy").String(),
			CloudRegion: "dummy-region",
			ModelStatus: params.DetailedStatus{
				Since:  &s.since,
				Status: "available",
			},
			SLA: "unsupported",
		},
		Machines: map[string]params.MachineStatus{
			"0": {
				AgentStatus: params.DetailedStatus{
					Status: "started",
					Since:  &s.since,
				},
				Containers: map[string]params.MachineStatus{
					"0/lxd/0": {
						Id:   "0/lxd/0",
						Jobs: []jujumodel.MachineJob{jujumodel.JobHostUnits},
						AgentStatus: params.DetailedStatus{
							Status: "pending",
							Since:  &s.since,
						},
						InstanceStatus: params.DetailedStatus{
							Status: "pending",
							Since:  &s.since,
						},
						ModificationStatus: params.DetailedStatus{
							Status: "idle",
							Since:  &s.since,
						},
						Series:     "quantal",
						InstanceId: "pending",
					},
				},
				InstanceStatus: params.DetailedStatus{
					Status: "pending",
					Since:  &s.since,
				},
				ModificationStatus: params.DetailedStatus{
					Status: "idle",
					Since:  &s.since,
				},
				DNSName:     "10.0.0.1",
				IPAddresses: []string{"10.0.0.1"},
				InstanceId:  "controller-0",
				Series:      "quantal",
				Id:          "0",
				NetworkInterfaces: map[string]params.NetworkInterface{
					"eth0": {
						IPAddresses: []string{"10.0.0.1"},
						MACAddress:  "aa:bb:cc:dd:ee:ff",
						IsUp:        true,
					},
				},
				Hardware:  "arch=amd64 cores=1 mem=1024M root-disk=8192M",
				Jobs:      []jujumodel.MachineJob{jujumodel.JobManageModel},
				HasVote:   false,
				WantsVote: true,
			},
		},
		Applications: map[string]params.ApplicationStatus{
			"wordpress": {
				Charm:           "local:focal/wordpress-3",
				WorkloadVersion: "4.5.3",
				Series:          "quantal",
				Exposed:         true,
				Relations:       map[string][]string{"db": {"mysql"}, "logging-dir": {"logging"}},
				Units: map[string]params.UnitStatus{
					"wordpress/0": {
						AgentStatus: params.DetailedStatus{
							Status:  "idle",
							Since:   &s.since,
							Version: currentVersion.String(),
						},
						WorkloadStatus: params.DetailedStatus{
							Status: "active",
							Since:  &s.since,
						},
						Machine:       "1",
						PublicAddress: "10.0.1.1",
						Subordinates: map[string]params.UnitStatus{
							"logging/0": {
								AgentStatus: params.DetailedStatus{
									Status:  "idle",
									Since:   &s.since,
									Version: currentVersion.String(),
								},
								WorkloadStatus: params.DetailedStatus{
									Status: "active",
									Since:  &s.since,
								},
								PublicAddress: "10.0.1.1",
								Leader:        false,
							},
						},
						Leader: true,
					},
				},
				Status: params.DetailedStatus{
					Status: "active",
					Since:  &s.since,
				},
			},
			"logging": {
				Charm:   "local:focal/logging-1",
				Series:  "quantal",
				Exposed: true,
				Relations: map[string][]string{
					"logging-directory": {"wordpress"},
				},
				SubordinateTo: []string{"wordpress"},
				Status: params.DetailedStatus{
					Info:   "somehow lost in all those logs",
					Status: "error",
					Since:  &s.since,
				},
				EndpointBindings: map[string]string{
					"":                  "alpha",
					"info":              "alpha",
					"logging-client":    "alpha",
					"logging-directory": "alpha",
				},
				Scale:           0,
				WorkloadVersion: "a bit too lo...",
			},
			"mysql": {
				Charm:           "local:focal/mysql-1",
				WorkloadVersion: "5.7.13",
				Series:          "quantal",
				Exposed:         true,
				Relations: map[string][]string{
					"server": {"wordpress"},
				},
				Status: params.DetailedStatus{
					Status: "maintenance",
					Info:   "installing all the things",
					Since:  &s.since,
				},
				EndpointBindings: map[string]string{
					"":               "alpha",
					"metrics-client": "alpha",
					"server":         "alpha",
					"server-admin":   "alpha",
				},
				Scale: 0,
			},
		},
		Relations: []params.RelationStatus{
			{
				Id:        0,
				Key:       "wordpress:db mysql:server",
				Interface: "mysql",
				Scope:     "global",
				Endpoints: []params.EndpointStatus{
					{
						ApplicationName: "wordpress",
						Name:            "db",
						Role:            "requirer",
						Subordinate:     false,
					},
					{
						ApplicationName: "mysql",
						Name:            "server",
						Role:            "provider",
						Subordinate:     false,
					},
				},
				Status: params.DetailedStatus{
					Status: "suspended",
					Since:  &s.since,
				},
			},
			{
				Id:        1,
				Key:       "logging:logging-directory wordpress:logging-dir",
				Interface: "logging",
				Scope:     "container",
				Endpoints: []params.EndpointStatus{
					{
						ApplicationName: "wordpress",
						Name:            "logging-dir",
						Role:            "provider",
						Subordinate:     false,
					},
					{
						ApplicationName: "logging",
						Name:            "logging-directory",
						Role:            "requirer",
						Subordinate:     true,
					},
				},
				Status: params.DetailedStatus{
					Status: "joined",
					Since:  &s.since,
				},
			},
			{
				Id:        2,
				Key:       "logging:info mysql:juju-info",
				Interface: "juju-info",
				Scope:     "container",
				Endpoints: []params.EndpointStatus{
					{
						ApplicationName: "mysql",
						Name:            "juju-info",
						Role:            "provider",
						Subordinate:     false,
					},
					{
						ApplicationName: "logging",
						Name:            "info",
						Role:            "requirer",
						Subordinate:     true,
					},
				},
				Status: params.DetailedStatus{
					Status: "joined",
					Since:  &s.since,
				},
			},
		},
		ControllerTimestamp: &s.since,
	}

	return &results
}

func (s *StatusSuite) prepSummaryData() *params.FullStatus {

	results := params.FullStatus{
		Model: params.ModelStatusInfo{
			Name:        "controller",
			Type:        "iaas",
			Version:     currentVersion.String(),
			CloudTag:    names.NewCloudTag("dummy").String(),
			CloudRegion: "dummy-region",
			ModelStatus: params.DetailedStatus{
				Version: currentVersion.String(),
				Since:   &s.since,
				Status:  "available",
			},
			AvailableVersion: nextVersion.String(),
			SLA:              "unsupported",
		},
		Machines: map[string]params.MachineStatus{
			"0": {AgentStatus: params.DetailedStatus{
				Status:  "started",
				Since:   &s.since,
				Version: currentVersion.String(),
			},
				InstanceStatus: params.DetailedStatus{
					Status: "started",
					Since:  &s.since,
				},
				ModificationStatus: params.DetailedStatus{
					Status: "applied",
					Since:  &s.since,
				},
				Hostname:    "controller-0",
				DNSName:     "localhost",
				IPAddresses: []string{"localhost"},
				InstanceId:  "controller-0",
				DisplayName: "controller-0",
				Series:      "quantal",
				Id:          "0",
				NetworkInterfaces: map[string]params.NetworkInterface{
					"eth0": {
						IPAddresses: []string{"localhost"},
						MACAddress:  "00:16:3e:23:1f:1f ",
						Space:       "alpha",
						IsUp:        true,
					},
				},
				Hardware:    "availability-zone=us-east-1a",
				Constraints: "arch=amd64 Hardware:arch=amd64 cores=0 mem=0M",
				Jobs:        []jujumodel.MachineJob{jujumodel.JobHostUnits},
				HasVote:     false,
				WantsVote:   false,
			},
			"1": {
				AgentStatus: params.DetailedStatus{
					Status:  "started",
					Since:   &s.since,
					Version: currentVersion.String(),
				},
				InstanceStatus: params.DetailedStatus{
					Status: "starting",
					Since:  &s.since,
				},
				ModificationStatus: params.DetailedStatus{
					Status: "applied",
					Since:  &s.since,
				},
				Hostname:    "snowflake",
				DNSName:     "10.0.2.1",
				IPAddresses: []string{"10.0.2.1"},
				InstanceId:  "snowflake",
				DisplayName: "snowflake",
				Series:      "quantal",
				Id:          "1",
				NetworkInterfaces: map[string]params.NetworkInterface{"eth0": {
					IPAddresses: []string{"10.0.2.1"},
					MACAddress:  "00:16:3e:23:f2:25",
					Space:       "alpha",
					IsUp:        true,
				},
				},
				Constraints: "arch=amd64 Hardware:arch=amd64 cores=0 mem=0M",
				Jobs:        []jujumodel.MachineJob{jujumodel.JobHostUnits},
				HasVote:     false,
				WantsVote:   false,
			},
			"2": {
				AgentStatus: params.DetailedStatus{
					Status:  "started",
					Since:   &s.since,
					Version: currentVersion.String(),
				},
				InstanceStatus: params.DetailedStatus{
					Status: "started",
					Since:  &s.since,
				}, ModificationStatus: params.DetailedStatus{Status: "applied",
					Since: &s.since,
				},
				Hostname:    "controller-2",
				DNSName:     "10.0.2.1",
				IPAddresses: []string{"10.0.2.1"},
				InstanceId:  "controller-2",
				Series:      "quantal",
				Id:          "2",
				NetworkInterfaces: map[string]params.NetworkInterface{"eth0": {
					IPAddresses: []string{"10.0.2.1"},
					MACAddress:  "00:16:3e:0f:76:de",
					Gateway:     "10.0.0.0",
					Space:       "alpha",
					IsUp:        true,
				},
				},
				Constraints: "arch=amd64 Hardware:arch=amd64 cores=0 mem=0M",
				Jobs:        []jujumodel.MachineJob{jujumodel.JobHostUnits},
				HasVote:     false,
				WantsVote:   false,
			},
		},
		Applications: map[string]params.ApplicationStatus{
			"logging": {
				Charm:   "local:focal/logging-1",
				Series:  "quantal",
				Exposed: true,
				Relations: map[string][]string{
					"logging-directory": {"wordpress"},
				},
				SubordinateTo: []string{"wordpress"},
				Status: params.DetailedStatus{
					Info:   "somehow lost in all those logs",
					Status: "error",
					Since:  &s.since,
				},
				EndpointBindings: map[string]string{
					"":                  "alpha",
					"info":              "alpha",
					"logging-client":    "alpha",
					"logging-directory": "alpha",
				},
				Scale:           0,
				WorkloadVersion: "a bit too lo...",
			},
			"mysql": {
				Charm:           "local:focal/mysql-1",
				WorkloadVersion: "5.7.13",
				Series:          "quantal",
				Exposed:         true,
				Relations: map[string][]string{
					"server": {"wordpress"},
				},
				Units: map[string]params.UnitStatus{
					"mysql/0": {
						AgentStatus: params.DetailedStatus{
							Status:  "idle",
							Since:   &s.since,
							Version: currentVersion.String(),
						},
						WorkloadStatus: params.DetailedStatus{
							Status: "error",
							Info:   "oops I errored",
							Since:  &s.since,
						},
						Machine:       "2",
						PublicAddress: "10.0.2.1",
						Subordinates: map[string]params.UnitStatus{
							"logging/1": {
								AgentStatus: params.DetailedStatus{
									Status:  "idle",
									Since:   &s.since,
									Version: currentVersion.String(),
								},
								WorkloadStatus: params.DetailedStatus{
									Info:   "somehow lost in all those logs",
									Status: "active",
									Since:  &s.since,
								},
								PublicAddress: "10.0.2.1",
								Leader:        true,
							},
						},
						Leader: true,
					},
				},
				Status: params.DetailedStatus{
					Status: "maintenance",
					Info:   "installing all the things",
					Since:  &s.since,
				},
				EndpointBindings: map[string]string{
					"":               "alpha",
					"metrics-client": "alpha",
					"server":         "alpha",
					"server-admin":   "alpha",
				},
				Scale: 0,
			},
			"wordpress": {
				Charm:           "local:focal/wordpress-3",
				WorkloadVersion: "4.5.3",
				Series:          "quantal",
				Exposed:         true,
				Relations:       map[string][]string{"db": {"mysql"}, "logging-dir": {"logging"}},
				Units: map[string]params.UnitStatus{
					"wordpress/0": {
						AgentStatus: params.DetailedStatus{
							Status:  "idle",
							Since:   &s.since,
							Version: currentVersion.String(),
						},
						WorkloadStatus: params.DetailedStatus{
							Status: "active",
							Since:  &s.since,
						},
						Machine:       "1",
						PublicAddress: "10.0.2.1",
						Subordinates: map[string]params.UnitStatus{
							"logging/0": {
								AgentStatus: params.DetailedStatus{
									Status:  "idle",
									Since:   &s.since,
									Version: currentVersion.String(),
								},
								WorkloadStatus: params.DetailedStatus{
									Status: "active",
									Since:  &s.since,
								},
								PublicAddress: "10.0.2.1",
								Leader:        false,
							},
						},
						Leader: true,
					},
				},
				Status: params.DetailedStatus{
					Status: "active",
					Since:  &s.since,
				},
				EndpointBindings: map[string]string{"": "alpha",
					"admin-api":       "alpha",
					"cache":           "alpha",
					"db":              "alpha",
					"db-client":       "alpha",
					"foo-bar":         "alpha",
					"logging-dir":     "alpha",
					"monitoring-port": "alpha",
					"url":             "alpha"},
				Scale: 0,
			},
		},
		RemoteApplications: map[string]params.RemoteApplicationStatus{
			"hosted-riak": {
				OfferURL:  "me/model.riak",
				OfferName: "hosted-riak",
				Endpoints: []params.RemoteEndpoint{
					{
						Name:      "server",
						Role:      charm.RelationRole("provider"),
						Interface: "mysql",
						Limit:     0,
					},
				},
				Relations: map[string][]string{
					"server": {
						"remote-wordpress",
					},
				},
				Status: params.DetailedStatus{
					Status: "unknown",
					Since:  &s.since,
				},
			},
		},
		Offers: map[string]params.ApplicationOfferStatus{
			"hosted-mysql": {
				OfferName:       "hosted-mysql",
				ApplicationName: "mysql",
				CharmURL:        "local:focal/mysql-1",
				Endpoints: map[string]params.RemoteEndpoint{
					"server": {
						Name:      "server",
						Role:      charm.RelationRole("provider"),
						Interface: "mysql",
						Limit:     0,
					},
				},
				ActiveConnectedCount: 1,
				TotalConnectedCount:  1,
			},
		},
		Relations: []params.RelationStatus{
			{
				Id:        0,
				Key:       "logging:info mysql:juju-info",
				Interface: "juju-info",
				Scope:     "container",
				Endpoints: []params.EndpointStatus{
					{
						ApplicationName: "mysql",
						Name:            "juju-info",
						Role:            "provider",
						Subordinate:     false,
					},
					{
						ApplicationName: "logging",
						Name:            "info",
						Role:            "requirer",
						Subordinate:     true,
					},
				},
				Status: params.DetailedStatus{
					Status: "joined",
					Since:  &s.since,
				},
			},
			{
				Id:        1,
				Key:       "wordpress:db mysql:server",
				Interface: "mysql",
				Scope:     "global",
				Endpoints: []params.EndpointStatus{
					{
						ApplicationName: "wordpress",
						Name:            "db",
						Role:            "requirer",
						Subordinate:     false,
					},
					{
						ApplicationName: "mysql",
						Name:            "server",
						Role:            "provider",
						Subordinate:     false,
					},
				},
				Status: params.DetailedStatus{
					Status: "suspended",
					Since:  &s.since,
				},
			},
			{
				Id:        2,
				Key:       "logging:logging-directory wordpress:logging-dir",
				Interface: "logging",
				Scope:     "container",
				Endpoints: []params.EndpointStatus{
					{
						ApplicationName: "wordpress",
						Name:            "logging-dir",
						Role:            "provider",
						Subordinate:     false,
					},
					{
						ApplicationName: "logging",
						Name:            "logging-directory",
						Role:            "requirer",
						Subordinate:     true,
					},
				},
				Status: params.DetailedStatus{
					Status: "joined",
					Since:  &s.since,
				},
			},
		},
		ControllerTimestamp: &s.since,
		Branches: map[string]params.BranchStatus{
			"bla": {
				AssignedUnits: nil,
				Created:       0,
				CreatedBy:     "",
			},
		},
	}

	return &results
}
