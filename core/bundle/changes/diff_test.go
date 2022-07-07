// Copyright 2018 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bundlechanges_test

import (
	"strings"

	"github.com/juju/charm/v9"
	"github.com/juju/loggo"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	bundlechanges "github.com/juju/juju/core/bundle/changes"
)

type diffSuite struct {
	jujutesting.IsolationSuite
	logger loggo.Logger
}

var _ = gc.Suite(&diffSuite{})

func (s *diffSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.logger = loggo.GetLogger("diff_test")
	s.logger.SetLogLevel(loggo.TRACE)
}

func (s *diffSuite) TestNewDiffEmpty(c *gc.C) {
	diff := &bundlechanges.BundleDiff{}
	c.Assert(diff.Empty(), jc.IsTrue)
}

func (s *diffSuite) TestApplicationsNotEmpty(c *gc.C) {
	diff := &bundlechanges.BundleDiff{
		Applications: make(map[string]*bundlechanges.ApplicationDiff),
	}
	diff.Applications["mantell"] = &bundlechanges.ApplicationDiff{
		Missing: bundlechanges.ModelSide,
	}
	c.Assert(diff.Empty(), jc.IsFalse)
}

func (s *diffSuite) TestApplicationExposedEndpointsDiff(c *gc.C) {
	bundleContent := `
applications:
  prometheus:
    charm: cs:xenial/prometheus-7
    exposed-endpoints:
      foo:
        expose-to-spaces:
        - outer
        expose-to-cidrs:
        - 10.0.0.0/24
      bar:
        expose-to-spaces:
        - outer
        expose-to-cidrs:
        - 42.0.0.0/8
      baz:
        expose-to-cidrs:
        - 42.0.0.0/8
      bundle-only:
        expose-to-spaces:
        - dmz
    `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:  "prometheus",
				Charm: "cs:xenial/prometheus-7",
				ExposedEndpoints: map[string]bundlechanges.ExposedEndpoint{
					"foo": { // Same space and CIDRs
						ExposeToSpaces: []string{"outer"},
						ExposeToCIDRs:  []string{"10.0.0.0/24"},
					},
					"bar": { // Different space; same CIDRs
						ExposeToSpaces: []string{"inner"},
						ExposeToCIDRs:  []string{"10.0.0.0/24"},
					},
					"baz": { // Different CIDRs
						ExposeToCIDRs: []string{"192.168.0.0/24"},
					},
					"model-only": { // Only exists in model
						ExposeToSpaces: []string{"twisted"},
						ExposeToCIDRs:  []string{"10.0.0.0/24"},
					},
				},
			},
		},
	}
	expectedDiff := &bundlechanges.BundleDiff{
		Applications: map[string]*bundlechanges.ApplicationDiff{
			"prometheus": {
				ExposedEndpoints: map[string]bundlechanges.ExposedEndpointDiff{
					"bar": {
						Bundle: &bundlechanges.ExposedEndpointDiffEntry{
							ExposeToSpaces: []string{"outer"},
							ExposeToCIDRs:  []string{"42.0.0.0/8"},
						},
						Model: &bundlechanges.ExposedEndpointDiffEntry{
							ExposeToSpaces: []string{"inner"},
							ExposeToCIDRs:  []string{"10.0.0.0/24"},
						},
					},
					"baz": {
						Bundle: &bundlechanges.ExposedEndpointDiffEntry{
							ExposeToCIDRs: []string{"42.0.0.0/8"},
						},
						Model: &bundlechanges.ExposedEndpointDiffEntry{
							ExposeToCIDRs: []string{"192.168.0.0/24"},
						},
					},
					"bundle-only": {
						Bundle: &bundlechanges.ExposedEndpointDiffEntry{
							ExposeToSpaces: []string{"dmz"},
						},
						Model: nil,
					},
					"model-only": {
						Bundle: nil,
						Model: &bundlechanges.ExposedEndpointDiffEntry{
							ExposeToSpaces: []string{"twisted"},
							ExposeToCIDRs:  []string{"10.0.0.0/24"},
						},
					},
				},
			},
		},
	}
	s.checkDiff(c, bundleContent, model, expectedDiff)
}

func (s *diffSuite) TestMachinesNotEmpty(c *gc.C) {
	diff := &bundlechanges.BundleDiff{
		Machines: make(map[string]*bundlechanges.MachineDiff),
	}
	diff.Machines["1"] = &bundlechanges.MachineDiff{
		Missing: bundlechanges.BundleSide,
	}
	c.Assert(diff.Empty(), jc.IsFalse)
}

func (s *diffSuite) TestRelationsNotEmpty(c *gc.C) {
	diff := &bundlechanges.BundleDiff{}
	diff.Relations = &bundlechanges.RelationsDiff{
		BundleAdditions: [][]string{
			{"sinkane:telephone", "bad-sav:hensteeth"},
		},
	}
	c.Assert(diff.Empty(), jc.IsFalse)
}

func (s *diffSuite) TestModelMissingApplication(c *gc.C) {
	bundleContent := `
        applications:
            prometheus:
                charm: cs:xenial/prometheus-7
                num_units: 2
                to: [0, 1]
        machines:
            0:
            1:
            `
	model := &bundlechanges.Model{
		Machines: map[string]*bundlechanges.Machine{
			"0": {ID: "0"},
			"1": {ID: "1"},
		},
	}
	expectedDiff := &bundlechanges.BundleDiff{
		Applications: map[string]*bundlechanges.ApplicationDiff{
			"prometheus": {Missing: "model"},
		},
	}
	s.checkDiff(c, bundleContent, model, expectedDiff)
}

func (s *diffSuite) TestBundleMissingApplication(c *gc.C) {
	bundleContent := `
        applications:
            memcached:
                charm: cs:xenial/memcached-7
                num_units: 2
                to: [0, 1]
        machines:
            0:
            1:
            `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:  "prometheus",
				Charm: "cs:xenial/prometheus-7",
				Units: []bundlechanges.Unit{
					{Name: "prometheus/0", Machine: "0"},
				},
			},
			"memcached": {
				Name:  "memcached",
				Charm: "cs:xenial/memcached-7",
				Units: []bundlechanges.Unit{
					{Name: "memcached/0", Machine: "0"},
					{Name: "memcached/1", Machine: "1"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {ID: "0"},
			"1": {ID: "1"},
		},
	}
	expectedDiff := &bundlechanges.BundleDiff{
		Applications: map[string]*bundlechanges.ApplicationDiff{
			"prometheus": {Missing: "bundle"},
		},
	}
	s.checkDiff(c, bundleContent, model, expectedDiff)
}

func (s *diffSuite) TestMissingApplicationBoth(c *gc.C) {
	bundleContent := `
        applications:
            memcached:
                charm: cs:xenial/memcached-7
                num_units: 2
                to: [0, 1]
        machines:
            0:
            1:
            `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:  "prometheus",
				Charm: "cs:xenial/prometheus-7",
				Units: []bundlechanges.Unit{
					{Name: "prometheus/0", Machine: "0"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {ID: "0"},
			"1": {ID: "1"},
		},
	}
	expectedDiff := &bundlechanges.BundleDiff{
		Applications: map[string]*bundlechanges.ApplicationDiff{
			"prometheus": {Missing: "bundle"},
			"memcached":  {Missing: "model"},
		},
	}
	s.checkDiff(c, bundleContent, model, expectedDiff)
}

func (s *diffSuite) TestApplicationCharm(c *gc.C) {
	bundleContent := `
        applications:
            prometheus:
                charm: cs:xenial/prometheus-7
                num_units: 2
                to: [0, 1]
        machines:
            0:
            1:
            `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:  "prometheus",
				Charm: "cs:xenial/prometheus-8",
				Units: []bundlechanges.Unit{
					{Name: "prometheus/0", Machine: "0"},
					{Name: "prometheus/1", Machine: "1"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {ID: "0"},
			"1": {ID: "1"},
		},
	}
	expectedDiff := &bundlechanges.BundleDiff{
		Applications: map[string]*bundlechanges.ApplicationDiff{
			"prometheus": {
				Charm: &bundlechanges.StringDiff{
					Bundle: "cs:xenial/prometheus-7",
					Model:  "cs:xenial/prometheus-8",
				},
			},
		},
	}
	s.checkDiff(c, bundleContent, model, expectedDiff)
}

func (s *diffSuite) TestApplicationSeries(c *gc.C) {
	bundleContent := `
        applications:
            prometheus:
                charm: cs:prometheus-7
                series: bionic
                num_units: 2
                to: [0, 1]
        machines:
            0:
            1:
            `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:   "prometheus",
				Charm:  "cs:prometheus-7",
				Series: "xenial",
				Units: []bundlechanges.Unit{
					{Name: "prometheus/0", Machine: "0"},
					{Name: "prometheus/1", Machine: "1"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {ID: "0"},
			"1": {ID: "1"},
		},
	}
	expectedDiff := &bundlechanges.BundleDiff{
		Applications: map[string]*bundlechanges.ApplicationDiff{
			"prometheus": {
				Series: &bundlechanges.StringDiff{
					Bundle: "bionic",
					Model:  "xenial",
				},
			},
		},
	}
	s.checkDiff(c, bundleContent, model, expectedDiff)
}

func (s *diffSuite) TestApplicationChannel(c *gc.C) {
	bundleContent := `
        applications:
            prometheus:
                charm: cs:prometheus-7
                series: bionic
                channel: 1.0/stable
                num_units: 2
                to: [0, 1]
        machines:
            0:
            1:
            `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:    "prometheus",
				Charm:   "cs:prometheus-7",
				Channel: "2.0/edge",
				Series:  "bionic",
				Units: []bundlechanges.Unit{
					{Name: "prometheus/0", Machine: "0"},
					{Name: "prometheus/1", Machine: "1"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {ID: "0"},
			"1": {ID: "1"},
		},
	}
	expectedDiff := &bundlechanges.BundleDiff{
		Applications: map[string]*bundlechanges.ApplicationDiff{
			"prometheus": {
				Channel: &bundlechanges.StringDiff{
					Bundle: "1.0/stable",
					Model:  "2.0/edge",
				},
			},
		},
	}
	s.checkDiff(c, bundleContent, model, expectedDiff)
}

func (s *diffSuite) TestApplicationNumUnits(c *gc.C) {
	bundleContent := `
        applications:
            prometheus:
                charm: cs:xenial/prometheus-7
                num_units: 2
                to: [0, 1]
        machines:
            0:
            1:
            `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:  "prometheus",
				Charm: "cs:xenial/prometheus-7",
				Units: []bundlechanges.Unit{
					{Name: "prometheus/0", Machine: "0"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {ID: "0"},
			"1": {ID: "1"},
		},
	}
	expectedDiff := &bundlechanges.BundleDiff{
		Applications: map[string]*bundlechanges.ApplicationDiff{
			"prometheus": {
				NumUnits: &bundlechanges.IntDiff{
					Bundle: 2,
					Model:  1,
				},
			},
		},
	}
	s.checkDiff(c, bundleContent, model, expectedDiff)
}

func (s *diffSuite) TestApplicationScale(c *gc.C) {
	bundleContent := `
        bundle: kubernetes
        applications:
            prometheus:
                charm: cs:prometheus-7
                scale: 2
                placement: foo=bar
            `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:      "prometheus",
				Charm:     "cs:prometheus-7",
				Scale:     1,
				Placement: "foo=bar",
			},
		},
	}
	expectedDiff := &bundlechanges.BundleDiff{
		Applications: map[string]*bundlechanges.ApplicationDiff{
			"prometheus": {
				Scale: &bundlechanges.IntDiff{
					Bundle: 2,
					Model:  1,
				},
			},
		},
	}
	s.checkDiff(c, bundleContent, model, expectedDiff)
}

func (s *diffSuite) TestApplicationSubordinateNumUnits(c *gc.C) {
	bundleContent := `
        applications:
            prometheus:
                charm: cs:xenial/prometheus-7
                num_units: 2
                to: [0, 1]
            nrpe:
                charm: cs:xenial/nrpe-12
        machines:
            0:
            1:
        relations:
            - - nrpe:collector
              - prometheus:nrpe
            `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:  "prometheus",
				Charm: "cs:xenial/prometheus-7",
				Units: []bundlechanges.Unit{
					{Name: "prometheus/0", Machine: "0"},
					{Name: "prometheus/1", Machine: "1"},
				},
			},
			"nrpe": {
				Name:          "nrpe",
				Charm:         "cs:xenial/nrpe-12",
				SubordinateTo: []string{"prometheus"},
				Units: []bundlechanges.Unit{
					{Name: "nrpe/0", Machine: "0"},
					{Name: "nrpe/1", Machine: "1"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {ID: "0"},
			"1": {ID: "1"},
		},
		Relations: []bundlechanges.Relation{{
			App1:      "prometheus",
			Endpoint1: "nrpe",
			App2:      "nrpe",
			Endpoint2: "collector",
		}},
	}
	// We don't complain about num_units differing for subordinate
	// applications.
	expectedDiff := &bundlechanges.BundleDiff{}
	s.checkDiff(c, bundleContent, model, expectedDiff)
}

func (s *diffSuite) TestApplicationConstraints(c *gc.C) {
	bundleContent := `
        applications:
            prometheus:
                charm: cs:xenial/prometheus-7
                num_units: 1
                constraints: something
                to: [0]
        machines:
            0:
            `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:        "prometheus",
				Charm:       "cs:xenial/prometheus-7",
				Constraints: "else",
				Units: []bundlechanges.Unit{
					{Name: "prometheus/0", Machine: "0"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {ID: "0"},
		},
	}
	expectedDiff := &bundlechanges.BundleDiff{
		Applications: map[string]*bundlechanges.ApplicationDiff{
			"prometheus": {
				Constraints: &bundlechanges.StringDiff{
					Bundle: "something",
					Model:  "else",
				},
			},
		},
	}
	s.checkDiff(c, bundleContent, model, expectedDiff)
}

func (s *diffSuite) TestBundleSeries(c *gc.C) {
	bundleContent := `
        series: focal
        applications:
            prometheus:
                charm: cs:focal/prometheus-7
                num_units: 1
                constraints: something
                to: [0]
        machines:
            0:
            `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:        "prometheus",
				Charm:       "cs:focal/prometheus-7",
				Series:      "focal",
				Constraints: "something",
				Units: []bundlechanges.Unit{
					{Name: "prometheus/0", Machine: "0"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {
				ID:     "0",
				Series: "focal",
			},
		},
	}
	expectedDiff := &bundlechanges.BundleDiff{}
	s.checkDiff(c, bundleContent, model, expectedDiff)
}

func (s *diffSuite) TestNoBundleSeries(c *gc.C) {
	bundleContent := `
        applications:
            prometheus:
                charm: cs:focal/prometheus-7
                num_units: 1
                constraints: something
                to: [0]
        machines:
            0:
            `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:        "prometheus",
				Charm:       "cs:focal/prometheus-7",
				Series:      "focal",
				Constraints: "something",
				Units: []bundlechanges.Unit{
					{Name: "prometheus/0", Machine: "0"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {
				ID:     "0",
				Series: "focal",
			},
		},
	}
	expectedDiff := &bundlechanges.BundleDiff{
		Applications: map[string]*bundlechanges.ApplicationDiff{
			"prometheus": {
				Series: &bundlechanges.StringDiff{
					Bundle: "",
					Model:  "focal",
				},
			},
		},
		Machines: map[string]*bundlechanges.MachineDiff{
			"0": {
				Series: &bundlechanges.StringDiff{
					Bundle: "",
					Model:  "focal",
				},
			},
		},
	}
	s.checkDiff(c, bundleContent, model, expectedDiff)
}

func (s *diffSuite) TestApplicationOptions(c *gc.C) {
	bundleContent := `
        applications:
            prometheus:
                charm: cs:xenial/prometheus-7
                num_units: 1
                options:
                    griffin: [shoes, undies]
                    travis: glasses
                    clint: hat
                to: [0]
        machines:
            0:
            `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:  "prometheus",
				Charm: "cs:xenial/prometheus-7",
				Options: map[string]interface{}{
					"griffin": []interface{}{"shoes", "undies"},
					"justin":  "tshirt",
					"clint":   "scarf",
				},
				Units: []bundlechanges.Unit{
					{Name: "prometheus/0", Machine: "0"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {ID: "0"},
		},
	}
	expectedDiff := &bundlechanges.BundleDiff{
		Applications: map[string]*bundlechanges.ApplicationDiff{
			"prometheus": {
				Options: map[string]bundlechanges.OptionDiff{
					"travis": {"glasses", nil},
					"justin": {nil, "tshirt"},
					"clint":  {"hat", "scarf"},
				},
			},
		},
	}
	s.checkDiff(c, bundleContent, model, expectedDiff)
}

func (s *diffSuite) TestApplicationAnnotations(c *gc.C) {
	bundleContent := `
        applications:
            prometheus:
                charm: cs:xenial/prometheus-7
                num_units: 1
                annotations:
                    griffin: shoes
                    travis: glasses
                to: [0]
        machines:
            0:
            `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:  "prometheus",
				Charm: "cs:xenial/prometheus-7",
				Annotations: map[string]string{
					"griffin": "shorts",
					"justin":  "tshirt",
				},
				Units: []bundlechanges.Unit{
					{Name: "prometheus/0", Machine: "0"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {ID: "0"},
		},
	}
	expectedDiff := &bundlechanges.BundleDiff{
		Applications: map[string]*bundlechanges.ApplicationDiff{
			"prometheus": {
				Annotations: map[string]bundlechanges.StringDiff{
					"griffin": {"shoes", "shorts"},
					"travis":  {"glasses", ""},
					"justin":  {"", "tshirt"},
				},
			},
		},
	}
	s.checkDiff(c, bundleContent, model, expectedDiff)
}

func (s *diffSuite) TestApplicationAnnotationsWithOptionOff(c *gc.C) {
	bundleContent := `
        applications:
            prometheus:
                charm: cs:xenial/prometheus-7
                num_units: 1
                annotations:
                    clint: hat
                    griffin: shoes
                    travis: glasses
                to: [0]
        machines:
            0:
            `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:  "prometheus",
				Charm: "cs:xenial/prometheus-7",
				Annotations: map[string]string{
					"clint":   "hat",
					"griffin": "shorts",
					"justin":  "tshirt",
				},
				Units: []bundlechanges.Unit{
					{Name: "prometheus/0", Machine: "0"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {ID: "0"},
		},
	}
	expectedDiff := &bundlechanges.BundleDiff{}
	config := bundlechanges.DiffConfig{
		Bundle:             s.readBundle(c, bundleContent),
		Model:              model,
		IncludeAnnotations: false,
		Logger:             s.logger,
	}
	s.checkDiffImpl(c, config, expectedDiff, "")
}

func (s *diffSuite) TestApplicationExpose(c *gc.C) {
	bundleContent := `
        applications:
            prometheus:
                charm: cs:xenial/prometheus-7
                num_units: 1
                to: [0]
        machines:
            0:
            `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:    "prometheus",
				Charm:   "cs:xenial/prometheus-7",
				Exposed: true,
				Units: []bundlechanges.Unit{
					{Name: "prometheus/0", Machine: "0"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {ID: "0"},
		},
	}
	expectedDiff := &bundlechanges.BundleDiff{
		Applications: map[string]*bundlechanges.ApplicationDiff{
			"prometheus": {
				Expose: &bundlechanges.BoolDiff{
					Bundle: false,
					Model:  true,
				},
				// Since the model specifies "expose:true", all
				// endpoints are exposed to 0.0.0.0/0 and ::/0
				// which will show up in the diff given that
				// the bundle is *not* exposed.
				ExposedEndpoints: map[string]bundlechanges.ExposedEndpointDiff{
					"": {
						Model: &bundlechanges.ExposedEndpointDiffEntry{
							ExposeToCIDRs: []string{"0.0.0.0/0", "::/0"},
						},
					},
				},
			},
		},
	}
	s.checkDiff(c, bundleContent, model, expectedDiff)
}

func (s *diffSuite) TestApplicationExposeImplicitCIDRs(c *gc.C) {
	bundleContent := `
        applications:
            prometheus:
                charm: cs:xenial/prometheus-7
                num_units: 1
                expose: true
                to: [0]
        machines:
            0:
            `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:    "prometheus",
				Charm:   "cs:xenial/prometheus-7",
				Exposed: true,
				ExposedEndpoints: map[string]bundlechanges.ExposedEndpoint{
					"": {
						ExposeToCIDRs: []string{"10.0.0.0/24"},
					},
				},
				Units: []bundlechanges.Unit{
					{Name: "prometheus/0", Machine: "0"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {ID: "0"},
		},
	}
	expectedDiff := &bundlechanges.BundleDiff{
		Applications: map[string]*bundlechanges.ApplicationDiff{
			"prometheus": {
				// Since the model specifies "expose:true", all
				// endpoints are implicitly exposed to 0.0.0.0/0.
				ExposedEndpoints: map[string]bundlechanges.ExposedEndpointDiff{
					"": {
						Bundle: &bundlechanges.ExposedEndpointDiffEntry{
							// Implicit CIDRs as the bundle specifies "expose: true"
							ExposeToCIDRs: []string{"0.0.0.0/0", "::/0"},
						},
						Model: &bundlechanges.ExposedEndpointDiffEntry{
							ExposeToCIDRs: []string{"10.0.0.0/24"},
						},
					},
				},
			},
		},
	}
	s.checkDiff(c, bundleContent, model, expectedDiff)
}

func (s *diffSuite) TestApplicationPlacement(c *gc.C) {
	bundleContent := `
        bundle: kubernetes
        applications:
            prometheus:
                charm: cs:prometheus-7
                scale: 2
                placement: foo=bar
            `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:      "prometheus",
				Charm:     "cs:prometheus-7",
				Scale:     2,
				Placement: "foo=baz",
			},
		},
	}
	expectedDiff := &bundlechanges.BundleDiff{
		Applications: map[string]*bundlechanges.ApplicationDiff{
			"prometheus": {
				Placement: &bundlechanges.StringDiff{
					Bundle: "foo=bar",
					Model:  "foo=baz",
				},
			},
		},
	}
	s.checkDiff(c, bundleContent, model, expectedDiff)
}

func (s *diffSuite) TestModelMissingMachine(c *gc.C) {
	bundleContent := `
        applications:
            prometheus:
                charm: cs:xenial/prometheus-7
                num_units: 2
                to: [0, 1]
        machines:
            0:
            1:
            `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:  "prometheus",
				Charm: "cs:xenial/prometheus-7",
				Units: []bundlechanges.Unit{
					{Name: "prometheus/0", Machine: "2"},
					{Name: "prometheus/1", Machine: "2"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"2": {ID: "2"},
		},
		MachineMap: map[string]string{
			"0": "2",
		},
	}
	expectedDiff := &bundlechanges.BundleDiff{
		Machines: map[string]*bundlechanges.MachineDiff{
			"1": {Missing: "model"},
		},
	}
	s.checkDiff(c, bundleContent, model, expectedDiff)
}

func (s *diffSuite) TestBundleMissingMachine(c *gc.C) {
	bundleContent := `
        applications:
            prometheus:
                charm: cs:xenial/prometheus-7
                num_units: 2
                to: [0]
        machines:
            0:
            `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:  "prometheus",
				Charm: "cs:xenial/prometheus-7",
				Units: []bundlechanges.Unit{
					{Name: "prometheus/0", Machine: "0"},
					{Name: "prometheus/1", Machine: "1"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {ID: "0"},
			"1": {ID: "1"},
		},
		MachineMap: map[string]string{
			"0": "1",
		},
	}
	expectedDiff := &bundlechanges.BundleDiff{
		Machines: map[string]*bundlechanges.MachineDiff{
			"0": {Missing: "bundle"},
		},
	}
	s.checkDiff(c, bundleContent, model, expectedDiff)
}

func (s *diffSuite) TestMachineSeries(c *gc.C) {
	bundleContent := `
        applications:
            prometheus:
                charm: cs:xenial/prometheus-7
                num_units: 1
                to: [0]
        machines:
            0:
                series: bionic
            `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:  "prometheus",
				Charm: "cs:xenial/prometheus-7",
				Units: []bundlechanges.Unit{
					{Name: "prometheus/0", Machine: "0"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {
				ID:     "0",
				Series: "xenial",
			},
		},
	}
	expectedDiff := &bundlechanges.BundleDiff{
		Machines: map[string]*bundlechanges.MachineDiff{
			"0": {
				Series: &bundlechanges.StringDiff{
					Bundle: "bionic",
					Model:  "xenial",
				},
			},
		},
	}
	s.checkDiff(c, bundleContent, model, expectedDiff)
}

func (s *diffSuite) TestMachineAnnotations(c *gc.C) {
	bundleContent := `
        applications:
            prometheus:
                charm: cs:xenial/prometheus-7
                num_units: 1
                to: [0]
        machines:
            0:
                annotations:
                    scott: pilgrim
                    dark: knight
            `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:  "prometheus",
				Charm: "cs:xenial/prometheus-7",
				Units: []bundlechanges.Unit{
					{Name: "prometheus/0", Machine: "0"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {
				ID: "0",
				Annotations: map[string]string{
					"scott":  "pilgrim",
					"galaxy": "quest",
				},
			},
		},
	}
	expectedDiff := &bundlechanges.BundleDiff{
		Machines: map[string]*bundlechanges.MachineDiff{
			"0": {
				Annotations: map[string]bundlechanges.StringDiff{
					"dark":   {"knight", ""},
					"galaxy": {"", "quest"},
				},
			},
		},
	}
	s.checkDiff(c, bundleContent, model, expectedDiff)
}

func (s *diffSuite) TestMachineAnnotationsWithOptionOff(c *gc.C) {
	bundleContent := `
        applications:
            prometheus:
                charm: cs:xenial/prometheus-7
                num_units: 1
                to: [0]
        machines:
            0:
                annotations:
                    scott: pilgrim
                    dark: knight
            `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:  "prometheus",
				Charm: "cs:xenial/prometheus-7",
				Units: []bundlechanges.Unit{
					{Name: "prometheus/0", Machine: "0"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {
				ID: "0",
				Annotations: map[string]string{
					"scott":  "pilgrim",
					"galaxy": "quest",
				},
			},
		},
	}
	expectedDiff := &bundlechanges.BundleDiff{}
	config := bundlechanges.DiffConfig{
		Bundle:             s.readBundle(c, bundleContent),
		Model:              model,
		IncludeAnnotations: false,
		Logger:             s.logger,
	}
	s.checkDiffImpl(c, config, expectedDiff, "")
}

func (s *diffSuite) TestRelations(c *gc.C) {
	bundleContent := `
        applications:
            memcached:
                charm: cs:xenial/memcached-7
                num_units: 1
                to: [0]
            prometheus:
                charm: cs:xenial/prometheus-7
                num_units: 1
                to: [1]
        machines:
            0:
            1:
        relations:
            - ["memcached:juju-info", "prometheus:target"]
            - ["memcached:admin", "prometheus:tickling"]
            `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:  "prometheus",
				Charm: "cs:xenial/prometheus-7",
				Units: []bundlechanges.Unit{
					{Name: "prometheus/0", Machine: "0"},
				},
			},
			"memcached": {
				Name:  "memcached",
				Charm: "cs:xenial/memcached-7",
				Units: []bundlechanges.Unit{
					{Name: "memcached/1", Machine: "1"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {ID: "0"},
			"1": {ID: "1"},
		},
		Relations: []bundlechanges.Relation{{
			App1:      "prometheus",
			Endpoint1: "target",
			App2:      "memcached",
			Endpoint2: "juju-info",
		}, {
			App1:      "prometheus",
			Endpoint1: "juju-info",
			App2:      "memcached",
			Endpoint2: "fish",
		}, {
			App1:      "memcached",
			Endpoint1: "zebra",
			App2:      "memcached",
			Endpoint2: "alligator",
		}},
	}
	expectedDiff := &bundlechanges.BundleDiff{
		Relations: &bundlechanges.RelationsDiff{
			BundleAdditions: [][]string{
				{"memcached:admin", "prometheus:tickling"},
			},
			ModelAdditions: [][]string{
				{"memcached:alligator", "memcached:zebra"},
				{"memcached:fish", "prometheus:juju-info"},
			},
		},
	}
	s.checkDiff(c, bundleContent, model, expectedDiff)
}

func (s *diffSuite) TestRelationsWithMissingEndpoints(c *gc.C) {
	bundleContent := `
        applications:
            memcached:
                charm: cs:xenial/memcached-7
                num_units: 1
                to: [0]
            prometheus:
                charm: cs:xenial/prometheus-7
                num_units: 1
                to: [1]
        machines:
            0:
            1:
        relations:
            - ["memcached", "prometheus:target"]
            `
	model := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"prometheus": {
				Name:  "prometheus",
				Charm: "cs:xenial/prometheus-7",
				Units: []bundlechanges.Unit{
					{Name: "prometheus/0", Machine: "0"},
				},
			},
			"memcached": {
				Name:  "memcached",
				Charm: "cs:xenial/memcached-7",
				Units: []bundlechanges.Unit{
					{Name: "memcached/1", Machine: "1"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {ID: "0"},
			"1": {ID: "1"},
		},
		Relations: []bundlechanges.Relation{{
			App1:      "prometheus",
			Endpoint1: "target",
			App2:      "memcached",
			Endpoint2: "juju-info",
		}},
	}
	expectedDiff := &bundlechanges.BundleDiff{
		Relations: &bundlechanges.RelationsDiff{
			BundleAdditions: [][]string{
				{"memcached:", "prometheus:target"},
			},
			ModelAdditions: [][]string{
				{"memcached:juju-info", "prometheus:target"},
			},
		},
	}
	s.checkDiff(c, bundleContent, model, expectedDiff)
}

func (s *diffSuite) TestValidationMissingBundle(c *gc.C) {
	config := bundlechanges.DiffConfig{
		Bundle: nil,
		Model:  &bundlechanges.Model{},
		Logger: s.logger,
	}
	s.checkDiffImpl(c, config, nil, "nil bundle not valid")
}

func (s *diffSuite) TestValidationMissingModel(c *gc.C) {
	bundleContent := `
        applications:
            prometheus:
                charm: cs:xenial/prometheus-7
                num_units: 1
                to: [0]
        machines:
            0:
            `
	config := bundlechanges.DiffConfig{
		Bundle: s.readBundle(c, bundleContent),
		Model:  nil,
		Logger: s.logger,
	}
	s.checkDiffImpl(c, config, nil, "nil model not valid")
}

func (s *diffSuite) TestValidationMissingLogger(c *gc.C) {
	bundleContent := `
        applications:
            prometheus:
                charm: cs:xenial/prometheus-7
                num_units: 1
                to: [0]
        machines:
            0:
            `
	config := bundlechanges.DiffConfig{
		Bundle: s.readBundle(c, bundleContent),
		Model:  &bundlechanges.Model{},
		Logger: nil,
	}
	s.checkDiffImpl(c, config, nil, "nil logger not valid")
}

func (s *diffSuite) TestValidationInvalidBundle(c *gc.C) {
	config := bundlechanges.DiffConfig{
		Bundle: &charm.BundleData{},
		Model:  &bundlechanges.Model{},
		Logger: s.logger,
	}
	s.checkDiffImpl(c, config, nil, "at least one application must be specified")
}

func (s *diffSuite) checkDiff(c *gc.C, bundleContent string, model *bundlechanges.Model, expected *bundlechanges.BundleDiff) {
	config := bundlechanges.DiffConfig{
		Bundle:             s.readBundle(c, bundleContent),
		Model:              model,
		IncludeAnnotations: true,
		Logger:             s.logger,
	}
	s.checkDiffImpl(c, config, expected, "")
}

func (s *diffSuite) checkDiffImpl(c *gc.C, config bundlechanges.DiffConfig, expected *bundlechanges.BundleDiff, errMatch string) {

	diff, err := bundlechanges.BuildDiff(config)
	if errMatch != "" {
		c.Assert(err, gc.ErrorMatches, errMatch)
		c.Assert(diff, gc.IsNil)
	} else {
		c.Assert(err, jc.ErrorIsNil)
		diffOut, err := yaml.Marshal(diff)
		c.Assert(err, jc.ErrorIsNil)
		c.Logf("actual: %s", diffOut)
		expectedOut, err := yaml.Marshal(expected)
		c.Assert(err, jc.ErrorIsNil)
		c.Logf("expected: %s", expectedOut)
		c.Assert(diff, gc.DeepEquals, expected)
	}
}

func (s *diffSuite) readBundle(c *gc.C, bundleContent string) *charm.BundleData {
	data, err := charm.ReadBundleData(strings.NewReader(bundleContent))
	c.Assert(err, jc.ErrorIsNil)
	err = data.Verify(nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	return data
}
