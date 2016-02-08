// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/provisioner"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	storagedummy "github.com/juju/juju/storage/provider/dummy"
	"github.com/juju/juju/storage/provider/registry"
	coretesting "github.com/juju/juju/testing"
)

func (s *withoutControllerSuite) TestProvisioningInfoWithStorage(c *gc.C) {
	s.registerStorageProviders(c, "static")

	pm := poolmanager.New(state.NewStateSettings(s.State))
	_, err := pm.Create("static-pool", "static", map[string]interface{}{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("cpu-cores=123 mem=8G")
	template := state.MachineTemplate{
		Series:      "quantal",
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: cons,
		Placement:   "valid",
		Volumes: []state.MachineVolumeParams{
			{Volume: state.VolumeParams{Size: 1000, Pool: "static-pool"}},
			{Volume: state.VolumeParams{Size: 2000, Pool: "static-pool"}},
		},
	}
	placementMachine, err := s.State.AddOneMachine(template)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: placementMachine.Tag().String()},
	}}
	result, err := s.provisioner.ProvisioningInfo(args)
	c.Assert(err, jc.ErrorIsNil)

	expected := params.ProvisioningInfoResults{
		Results: []params.ProvisioningInfoResult{
			{Result: &params.ProvisioningInfo{
				Series:   "quantal",
				Networks: []string{},
				Jobs:     []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
				Tags: map[string]string{
					tags.JujuModel: coretesting.ModelTag.Id(),
				},
			}},
			{Result: &params.ProvisioningInfo{
				Series:      "quantal",
				Constraints: template.Constraints,
				Placement:   template.Placement,
				Networks:    template.RequestedNetworks,
				Jobs:        []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
				Tags: map[string]string{
					tags.JujuModel: coretesting.ModelTag.Id(),
				},
				Volumes: []params.VolumeParams{{
					VolumeTag:  "volume-0",
					Size:       1000,
					Provider:   "static",
					Attributes: map[string]interface{}{"foo": "bar"},
					Tags: map[string]string{
						tags.JujuModel: coretesting.ModelTag.Id(),
					},
					Attachment: &params.VolumeAttachmentParams{
						MachineTag: placementMachine.Tag().String(),
						VolumeTag:  "volume-0",
						Provider:   "static",
					},
				}, {
					VolumeTag:  "volume-1",
					Size:       2000,
					Provider:   "static",
					Attributes: map[string]interface{}{"foo": "bar"},
					Tags: map[string]string{
						tags.JujuModel: coretesting.ModelTag.Id(),
					},
					Attachment: &params.VolumeAttachmentParams{
						MachineTag: placementMachine.Tag().String(),
						VolumeTag:  "volume-1",
						Provider:   "static",
					},
				}},
			}},
		},
	}
	// The order of volumes is not predictable, so we make sure we
	// compare the right ones. This only applies to Results[1] since
	// it is the only result to contain volumes.
	if expected.Results[1].Result.Volumes[0].VolumeTag != result.Results[1].Result.Volumes[0].VolumeTag {
		vols := expected.Results[1].Result.Volumes
		vols[0], vols[1] = vols[1], vols[0]
	}
	c.Assert(result, jc.DeepEquals, expected)
}

func (s *withoutControllerSuite) registerStorageProviders(c *gc.C, names ...string) {
	types := make([]storage.ProviderType, len(names))
	for i, name := range names {
		types[i] = storage.ProviderType(name)
		if name == "dynamic" {
			s.registerDynamicStorageProvider(c)
		} else if name == "static" {
			s.registerStaticStorageProvider(c)
		} else {
			c.Fatalf("unknown storage provider type: %q, expected static or dynamic", name)
		}
	}
	registry.RegisterEnvironStorageProviders("dummy", types...)
}

func (s *withoutControllerSuite) registerDynamicStorageProvider(c *gc.C) {
	registry.RegisterProvider("dynamic", &storagedummy.StorageProvider{IsDynamic: true})
	s.AddCleanup(func(*gc.C) {
		registry.RegisterProvider("dynamic", nil)
	})
}

func (s *withoutControllerSuite) registerStaticStorageProvider(c *gc.C) {
	registry.RegisterProvider("static", &storagedummy.StorageProvider{IsDynamic: false})
	s.AddCleanup(func(*gc.C) {
		registry.RegisterProvider("static", nil)
	})
}

func (s *withoutControllerSuite) TestProvisioningInfoWithSingleNegativeAndPositiveSpaceInConstraints(c *gc.C) {
	s.addSpacesAndSubnets(c)

	cons := constraints.MustParse("cpu-cores=123 mem=8G spaces=^space1,space2")
	template := state.MachineTemplate{
		Series:      "quantal",
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: cons,
		Placement:   "valid",
	}
	placementMachine, err := s.State.AddOneMachine(template)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: placementMachine.Tag().String()},
	}}
	result, err := s.provisioner.ProvisioningInfo(args)
	c.Assert(err, jc.ErrorIsNil)

	expected := params.ProvisioningInfoResults{
		Results: []params.ProvisioningInfoResult{{
			Result: &params.ProvisioningInfo{
				Series:      "quantal",
				Constraints: template.Constraints,
				Placement:   template.Placement,
				Networks:    template.RequestedNetworks,
				Jobs:        []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
				Tags: map[string]string{
					tags.JujuModel: coretesting.ModelTag.Id(),
				},
				SubnetsToZones: map[string][]string{
					"subnet-1": []string{"zone1"},
					"subnet-2": []string{"zone2"},
				},
			},
		}}}
	c.Assert(result, jc.DeepEquals, expected)
}

func (s *withoutControllerSuite) addSpacesAndSubnets(c *gc.C) {
	// Add a couple of spaces.
	_, err := s.State.AddSpace("space1", "first space id", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("space2", "", nil, false) // no provider ID
	c.Assert(err, jc.ErrorIsNil)
	// Add 1 subnet into space1, and 2 into space2.
	// Only the first subnet of space2 has AllocatableIPLow|High set.
	// Each subnet is in a matching zone (e.g "subnet-#" in "zone#").
	testing.AddSubnetsWithTemplate(c, s.State, 3, state.SubnetInfo{
		CIDR:              "10.10.{{.}}.0/24",
		ProviderId:        "subnet-{{.}}",
		AllocatableIPLow:  "{{if (eq . 1)}}10.10.{{.}}.5{{end}}",
		AllocatableIPHigh: "{{if (eq . 1)}}10.10.{{.}}.254{{end}}",
		AvailabilityZone:  "zone{{.}}",
		SpaceName:         "{{if (eq . 0)}}space1{{else}}space2{{end}}",
		VLANTag:           42,
	})
}

func (s *withoutControllerSuite) TestProvisioningInfoWithEndpointBindings(c *gc.C) {
	s.addSpacesAndSubnets(c)

	wordpressMachine, err := s.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Use juju names for spaces in bindings, simulating ''juju deploy
	// --bind...' was called.
	bindings := map[string]string{
		"url": "space1", // has both name and provider ID
		"db":  "space2", // has only name, no provider ID
	}
	wordpressCharm := s.AddTestingCharm(c, "wordpress")
	wordpressService := s.AddTestingServiceWithBindings(c, "wordpress", wordpressCharm, bindings)
	wordpressUnit, err := wordpressService.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = wordpressUnit.AssignToMachine(wordpressMachine)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: wordpressMachine.Tag().String()},
	}}
	result, err := s.provisioner.ProvisioningInfo(args)
	c.Assert(err, jc.ErrorIsNil)

	expected := params.ProvisioningInfoResults{
		Results: []params.ProvisioningInfoResult{{
			Result: &params.ProvisioningInfo{
				Series: "quantal",
				Jobs:   []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
				Tags: map[string]string{
					tags.JujuModel:         coretesting.ModelTag.Id(),
					tags.JujuUnitsDeployed: wordpressUnit.Name(),
				},
				// Ensure space names are translated to provider IDs, where
				// possible.
				EndpointBindings: map[string]string{
					"db":  "space2",         // just name, no provider ID
					"url": "first space id", // has provider ID
					// We expect none of the unspecified bindings in the result.
				},
			},
		}}}
	c.Assert(result, jc.DeepEquals, expected)
}

func (s *withoutControllerSuite) TestProvisioningInfoWithUnsuitableSpacesConstraints(c *gc.C) {
	// Add an empty space.
	_, err := s.State.AddSpace("empty", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)

	consEmptySpace := constraints.MustParse("cpu-cores=123 mem=8G spaces=empty")
	consMissingSpace := constraints.MustParse("cpu-cores=123 mem=8G spaces=missing")
	templates := []state.MachineTemplate{{
		Series:      "quantal",
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: consEmptySpace,
		Placement:   "valid",
	}, {
		Series:      "quantal",
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: consMissingSpace,
		Placement:   "valid",
	}}
	placementMachines, err := s.State.AddMachines(templates...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(placementMachines, gc.HasLen, 2)

	args := params.Entities{Entities: []params.Entity{
		{Tag: placementMachines[0].Tag().String()},
		{Tag: placementMachines[1].Tag().String()},
	}}
	result, err := s.provisioner.ProvisioningInfo(args)
	c.Assert(err, jc.ErrorIsNil)

	expectedErrorEmptySpace := `cannot match subnets to zones: ` +
		`cannot use space "empty" as deployment target: no subnets`
	expectedErrorMissingSpace := `cannot match subnets to zones: ` +
		`space "missing"` // " not found" will be appended by NotFoundError helper below.
	expected := params.ProvisioningInfoResults{Results: []params.ProvisioningInfoResult{
		{Error: apiservertesting.ServerError(expectedErrorEmptySpace)},
		{Error: apiservertesting.NotFoundError(expectedErrorMissingSpace)},
	}}
	c.Assert(result, jc.DeepEquals, expected)
}

func (s *withoutControllerSuite) TestStorageProviderFallbackToType(c *gc.C) {
	s.registerStorageProviders(c, "dynamic", "static")

	template := state.MachineTemplate{
		Series:            "quantal",
		Jobs:              []state.MachineJob{state.JobHostUnits},
		Placement:         "valid",
		RequestedNetworks: []string{"net1", "net2"},
		Volumes: []state.MachineVolumeParams{
			{Volume: state.VolumeParams{Size: 1000, Pool: "dynamic"}},
			{Volume: state.VolumeParams{Size: 1000, Pool: "static"}},
		},
	}
	placementMachine, err := s.State.AddOneMachine(template)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: placementMachine.Tag().String()},
	}}
	result, err := s.provisioner.ProvisioningInfo(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result, jc.DeepEquals, params.ProvisioningInfoResults{
		Results: []params.ProvisioningInfoResult{
			{Result: &params.ProvisioningInfo{
				Series:      "quantal",
				Constraints: template.Constraints,
				Placement:   template.Placement,
				Networks:    template.RequestedNetworks,
				Jobs:        []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
				Tags: map[string]string{
					tags.JujuModel: coretesting.ModelTag.Id(),
				},
				Volumes: []params.VolumeParams{{
					VolumeTag:  "volume-1",
					Size:       1000,
					Provider:   "static",
					Attributes: nil,
					Tags: map[string]string{
						tags.JujuModel: coretesting.ModelTag.Id(),
					},
					Attachment: &params.VolumeAttachmentParams{
						MachineTag: placementMachine.Tag().String(),
						VolumeTag:  "volume-1",
						Provider:   "static",
					},
				}},
			}},
		},
	})
}

func (s *withoutControllerSuite) TestProvisioningInfoPermissions(c *gc.C) {
	// Login as a machine agent for machine 0.
	anAuthorizer := s.authorizer
	anAuthorizer.EnvironManager = false
	anAuthorizer.Tag = s.machines[0].Tag()
	aProvisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(aProvisioner, gc.NotNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[0].Tag().String() + "-lxc-0"},
		{Tag: "machine-42"},
		{Tag: s.machines[1].Tag().String()},
		{Tag: "service-bar"},
	}}

	// Only machine 0 and containers therein can be accessed.
	results, err := aProvisioner.ProvisioningInfo(args)
	c.Assert(results, jc.DeepEquals, params.ProvisioningInfoResults{
		Results: []params.ProvisioningInfoResult{
			{Result: &params.ProvisioningInfo{
				Series:   "quantal",
				Networks: []string{},
				Jobs:     []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
				Tags: map[string]string{
					tags.JujuModel: coretesting.ModelTag.Id(),
				},
			}},
			{Error: apiservertesting.NotFoundError("machine 0/lxc/0")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}
