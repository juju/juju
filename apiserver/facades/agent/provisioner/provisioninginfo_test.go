// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"

	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/agent/provisioner"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	coretesting "github.com/juju/juju/testing"
)

func (s *withoutControllerSuite) TestProvisioningInfoWithStorage(c *gc.C) {
	pm := poolmanager.New(state.NewStateSettings(s.State), storage.ChainedProviderRegistry{
		dummy.StorageProviders(),
		provider.CommonStorageProviders(),
	})
	_, err := pm.Create("static-pool", "static", map[string]interface{}{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("cores=123 mem=8G")
	template := state.MachineTemplate{
		Series:      "quantal",
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: cons,
		Placement:   "valid",
		Volumes: []state.HostVolumeParams{
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

	controllerCfg := s.ControllerConfig
	expected := params.ProvisioningInfoResultsV10{
		Results: []params.ProvisioningInfoResultV10{
			{Result: &params.ProvisioningInfoV10{
				ProvisioningInfoBase: params.ProvisioningInfoBase{
					ControllerConfig: controllerCfg,
					Series:           "quantal",
					Jobs:             []model.MachineJob{model.JobHostUnits},
					Tags: map[string]string{
						tags.JujuController: coretesting.ControllerTag.Id(),
						tags.JujuModel:      coretesting.ModelTag.Id(),
						tags.JujuMachine:    "controller-machine-0",
					},
					EndpointBindings: make(map[string]string),
				},
			}},
			{Result: &params.ProvisioningInfoV10{
				ProvisioningInfoBase: params.ProvisioningInfoBase{
					ControllerConfig: controllerCfg,
					Series:           "quantal",
					Constraints:      template.Constraints,
					Placement:        template.Placement,
					Jobs:             []model.MachineJob{model.JobHostUnits},
					Tags: map[string]string{
						tags.JujuController: coretesting.ControllerTag.Id(),
						tags.JujuModel:      coretesting.ModelTag.Id(),
						tags.JujuMachine:    "controller-machine-5",
					},
					EndpointBindings: make(map[string]string),
					Volumes: []params.VolumeParams{{
						VolumeTag:  "volume-0",
						Size:       1000,
						Provider:   "static",
						Attributes: map[string]interface{}{"foo": "bar"},
						Tags: map[string]string{
							tags.JujuController: coretesting.ControllerTag.Id(),
							tags.JujuModel:      coretesting.ModelTag.Id(),
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
							tags.JujuController: coretesting.ControllerTag.Id(),
							tags.JujuModel:      coretesting.ModelTag.Id(),
						},
						Attachment: &params.VolumeAttachmentParams{
							MachineTag: placementMachine.Tag().String(),
							VolumeTag:  "volume-1",
							Provider:   "static",
						},
					}},
				},
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

func (s *withoutControllerSuite) TestProvisioningInfoWithSingleNegativeAndPositiveSpaceInConstraintsV9(c *gc.C) {
	s.addSpacesAndSubnets(c)

	cons := constraints.MustParse("cores=123 mem=8G spaces=^space1,space2")
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

	api, err := provisioner.NewProvisionerAPIV9(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(api, gc.NotNil)

	result, err := api.ProvisioningInfo(args)
	c.Assert(err, jc.ErrorIsNil)

	controllerCfg := s.ControllerConfig
	expected := params.ProvisioningInfoResults{
		Results: []params.ProvisioningInfoResult{{
			Result: &params.ProvisioningInfo{
				ProvisioningInfoBase: params.ProvisioningInfoBase{
					ControllerConfig: controllerCfg,
					Series:           "quantal",
					Constraints:      template.Constraints,
					Placement:        template.Placement,
					Jobs:             []model.MachineJob{model.JobHostUnits},
					Tags: map[string]string{
						tags.JujuController: coretesting.ControllerTag.Id(),
						tags.JujuModel:      coretesting.ModelTag.Id(),
						tags.JujuMachine:    "controller-machine-5",
					},
					EndpointBindings: make(map[string]string),
				},
				SubnetsToZones: map[string][]string{
					"subnet-1": {"zone1"},
					"subnet-2": {"zone2"},
				},
			},
		}}}
	c.Assert(result, jc.DeepEquals, expected)
}

func (s *withoutControllerSuite) TestProvisioningInfoWithMultiplePositiveSpaceConstraints(c *gc.C) {
	s.addSpacesAndSubnets(c)

	cons := constraints.MustParse("cores=123 mem=8G spaces=space1,space2")
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
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	expected := params.ProvisioningInfoV10{
		ProvisioningInfoBase: params.ProvisioningInfoBase{
			ControllerConfig: s.ControllerConfig,
			Series:           "quantal",
			Constraints:      template.Constraints,
			Placement:        template.Placement,
			Jobs:             []model.MachineJob{model.JobHostUnits},
			Tags: map[string]string{
				tags.JujuController: coretesting.ControllerTag.Id(),
				tags.JujuModel:      coretesting.ModelTag.Id(),
				tags.JujuMachine:    "controller-machine-5",
			},
			EndpointBindings: make(map[string]string),
		},
		ProvisioningNetworkTopology: params.ProvisioningNetworkTopology{
			SubnetAZs: map[string][]string{
				"subnet-0": {"zone0"},
				"subnet-1": {"zone1"},
				"subnet-2": {"zone2"},
			},
			SpaceSubnets: map[string][]string{
				"space1": {"subnet-0"},
				"space2": {"subnet-1", "subnet-2"},
			},
		},
	}

	res := result.Results[0].Result
	c.Assert(res.ProvisioningInfoBase, jc.DeepEquals, expected.ProvisioningInfoBase)
	c.Assert(res.SubnetAZs, jc.DeepEquals, expected.SubnetAZs)
	c.Assert(res.SpaceSubnets, gc.HasLen, 2)
	c.Assert(res.SpaceSubnets["space1"], jc.SameContents, expected.SpaceSubnets["space1"])
	c.Assert(res.SpaceSubnets["space2"], jc.SameContents, expected.SpaceSubnets["space2"])

}

func (s *withoutControllerSuite) addSpacesAndSubnets(c *gc.C) {
	// Add a couple of spaces.
	space1, err := s.State.AddSpace("space1", "first space id", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	space2, err := s.State.AddSpace("space2", "", nil, false) // no provider ID
	c.Assert(err, jc.ErrorIsNil)
	// Add 1 subnet into space1, and 2 into space2.
	// Each subnet is in a matching zone (e.g "subnet-#" in "zone#").
	testing.AddSubnetsWithTemplate(c, s.State, 3, network.SubnetInfo{
		CIDR:              "10.10.{{.}}.0/24",
		ProviderId:        "subnet-{{.}}",
		AvailabilityZones: []string{"zone{{.}}"},
		SpaceID:           fmt.Sprintf("{{if (eq . 0)}}%s{{else}}%s{{end}}", space1.Id(), space2.Id()),
		VLANTag:           42,
	})
}

func (s *withoutControllerSuite) TestProvisioningInfoWithEndpointBindings(c *gc.C) {
	s.addSpacesAndSubnets(c)

	alphaSpace, err := s.State.SpaceByName("alpha")
	c.Assert(err, jc.ErrorIsNil)

	testing.AddSubnetsWithTemplate(c, s.State, 1, network.SubnetInfo{
		CIDR:              "10.10.{{add 4 .}}.0/24",
		ProviderId:        "subnet-alpha",
		AvailabilityZones: []string{"zone-alpha"},
		SpaceID:           alphaSpace.Id(),
		VLANTag:           43,
	})

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
	wordpressService := s.AddTestingApplicationWithBindings(c, "wordpress", wordpressCharm, bindings)
	wordpressUnit, err := wordpressService.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = wordpressUnit.AssignToMachine(wordpressMachine)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: wordpressMachine.Tag().String()},
	}}
	result, err := s.provisioner.ProvisioningInfo(args)
	c.Assert(err, jc.ErrorIsNil)

	controllerCfg := s.ControllerConfig
	expected := params.ProvisioningInfoResultsV10{
		Results: []params.ProvisioningInfoResultV10{{
			Result: &params.ProvisioningInfoV10{
				ProvisioningInfoBase: params.ProvisioningInfoBase{
					ControllerConfig: controllerCfg,
					Series:           "quantal",
					Jobs:             []model.MachineJob{model.JobHostUnits},
					Tags: map[string]string{
						tags.JujuController:    coretesting.ControllerTag.Id(),
						tags.JujuModel:         coretesting.ModelTag.Id(),
						tags.JujuMachine:       "controller-machine-5",
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
				ProvisioningNetworkTopology: params.ProvisioningNetworkTopology{
					SubnetAZs: map[string][]string{
						"subnet-0":     {"zone0"},
						"subnet-1":     {"zone1"},
						"subnet-2":     {"zone2"},
						"subnet-alpha": {"zone-alpha"},
					},
					SpaceSubnets: map[string][]string{
						"space1": {"subnet-0"},
						"space2": {"subnet-1", "subnet-2"},
						"alpha":  {"subnet-alpha"},
					},
				},
			},
		}},
	}
	c.Assert(result, jc.DeepEquals, expected)
}

func (s *withoutControllerSuite) TestProvisioningInfoWithEndpointBindingsAndNoAlphaSpace(c *gc.C) {
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
	wordpressService := s.AddTestingApplicationWithBindings(c, "wordpress", wordpressCharm, bindings)
	wordpressUnit, err := wordpressService.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = wordpressUnit.AssignToMachine(wordpressMachine)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: wordpressMachine.Tag().String()},
	}}
	result, err := s.provisioner.ProvisioningInfo(args)
	c.Assert(err, jc.ErrorIsNil)

	expected := params.ProvisioningInfoResultsV10{
		Results: []params.ProvisioningInfoResultV10{{
			Error: apiservertesting.ServerError("matching subnets to zones: cannot use space \"alpha\" as deployment target: no subnets"),
		}},
	}
	c.Assert(result, jc.DeepEquals, expected)
}

func (s *withoutControllerSuite) TestProvisioningInfoWithAlphaEndpointBindings(c *gc.C) {
	s.addSpacesAndSubnets(c)

	wordpressMachine, err := s.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Use juju names for spaces in bindings, simulating ''juju deploy
	// --bind...' was called.
	bindings := map[string]string{
		"url": "alpha",
	}
	wordpressCharm := s.AddTestingCharm(c, "wordpress")
	wordpressService := s.AddTestingApplicationWithBindings(c, "wordpress", wordpressCharm, bindings)
	wordpressUnit, err := wordpressService.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = wordpressUnit.AssignToMachine(wordpressMachine)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: wordpressMachine.Tag().String()},
	}}
	result, err := s.provisioner.ProvisioningInfo(args)
	c.Assert(err, jc.ErrorIsNil)

	controllerCfg := s.ControllerConfig
	expected := params.ProvisioningInfoResultsV10{
		Results: []params.ProvisioningInfoResultV10{{
			Result: &params.ProvisioningInfoV10{
				ProvisioningInfoBase: params.ProvisioningInfoBase{
					ControllerConfig: controllerCfg,
					Series:           "quantal",
					Jobs:             []model.MachineJob{model.JobHostUnits},
					Tags: map[string]string{
						tags.JujuController:    coretesting.ControllerTag.Id(),
						tags.JujuModel:         coretesting.ModelTag.Id(),
						tags.JujuMachine:       "controller-machine-5",
						tags.JujuUnitsDeployed: wordpressUnit.Name(),
					},
					// Ensure space names are translated to provider IDs, where
					// possible.
					EndpointBindings: map[string]string{},
				},
			},
		}},
	}
	c.Assert(result, jc.DeepEquals, expected)
}

func (s *withoutControllerSuite) TestProvisioningInfoWithUnsuitableSpacesConstraints(c *gc.C) {
	// Add an empty space.
	_, err := s.State.AddSpace("empty", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)

	consEmptySpace := constraints.MustParse("cores=123 mem=8G spaces=empty")
	consMissingSpace := constraints.MustParse("cores=123 mem=8G spaces=missing")
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

	expectedErrorEmptySpace := `matching subnets to zones: ` +
		`cannot use space "empty" as deployment target: no subnets`
	expectedErrorMissingSpace := `matching subnets to zones: ` +
		`space "missing"` // " not found" will be appended by NotFoundError helper below.
	expected := params.ProvisioningInfoResultsV10{Results: []params.ProvisioningInfoResultV10{
		{Error: apiservertesting.ServerError(expectedErrorEmptySpace)},
		{Error: apiservertesting.NotFoundError(expectedErrorMissingSpace)},
	}}
	c.Assert(result, jc.DeepEquals, expected)
}

func (s *withoutControllerSuite) TestProvisioningInfoWithLXDProfile(c *gc.C) {
	profileMachine, err := s.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	})
	c.Assert(err, jc.ErrorIsNil)

	profileCharm := s.AddTestingCharm(c, "lxd-profile")
	profileService := s.AddTestingApplication(c, "lxd-profile", profileCharm)
	profileUnit, err := profileService.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = profileUnit.AssignToMachine(profileMachine)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: profileMachine.Tag().String()},
	}}
	result, err := s.provisioner.ProvisioningInfo(args)
	c.Assert(err, jc.ErrorIsNil)

	controllerCfg := s.ControllerConfig

	pName := fmt.Sprintf("juju-%s-lxd-profile-0", profileMachine.ModelName())
	expected := params.ProvisioningInfoResultsV10{
		Results: []params.ProvisioningInfoResultV10{{
			Result: &params.ProvisioningInfoV10{
				ProvisioningInfoBase: params.ProvisioningInfoBase{
					ControllerConfig: controllerCfg,
					Series:           "quantal",
					Jobs:             []model.MachineJob{model.JobHostUnits},
					Tags: map[string]string{
						tags.JujuController:    coretesting.ControllerTag.Id(),
						tags.JujuModel:         coretesting.ModelTag.Id(),
						tags.JujuMachine:       "controller-machine-5",
						tags.JujuUnitsDeployed: profileUnit.Name(),
					},
					EndpointBindings: make(map[string]string),
					CharmLXDProfiles: []string{pName},
				},
			},
		}}}
	c.Assert(result, jc.DeepEquals, expected)
}

func (s *withoutControllerSuite) TestStorageProviderFallbackToType(c *gc.C) {
	template := state.MachineTemplate{
		Series:    "quantal",
		Jobs:      []state.MachineJob{state.JobHostUnits},
		Placement: "valid",
		Volumes: []state.HostVolumeParams{
			{Volume: state.VolumeParams{Size: 1000, Pool: "loop"}},
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

	controllerCfg := s.ControllerConfig
	c.Assert(result, jc.DeepEquals, params.ProvisioningInfoResultsV10{
		Results: []params.ProvisioningInfoResultV10{
			{Result: &params.ProvisioningInfoV10{
				ProvisioningInfoBase: params.ProvisioningInfoBase{
					ControllerConfig: controllerCfg,
					Series:           "quantal",
					Constraints:      template.Constraints,
					Placement:        template.Placement,
					Jobs:             []model.MachineJob{model.JobHostUnits},
					Tags: map[string]string{
						tags.JujuController: coretesting.ControllerTag.Id(),
						tags.JujuModel:      coretesting.ModelTag.Id(),
						tags.JujuMachine:    "controller-machine-5",
					},
					// volume-0 should not be included as it is not managed by
					// the environ provider.
					Volumes: []params.VolumeParams{{
						VolumeTag:  "volume-1",
						Size:       1000,
						Provider:   "static",
						Attributes: nil,
						Tags: map[string]string{
							tags.JujuController: coretesting.ControllerTag.Id(),
							tags.JujuModel:      coretesting.ModelTag.Id(),
						},
						Attachment: &params.VolumeAttachmentParams{
							MachineTag: placementMachine.Tag().String(),
							VolumeTag:  "volume-1",
							Provider:   "static",
						},
					}},
					EndpointBindings: make(map[string]string),
				},
			}},
		},
	})
}

func (s *withoutControllerSuite) TestStorageProviderVolumes(c *gc.C) {
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
		Volumes: []state.HostVolumeParams{
			{Volume: state.VolumeParams{Size: 1000, Pool: "modelscoped"}},
			{Volume: state.VolumeParams{Size: 1000, Pool: "modelscoped"}},
		},
	}
	machine, err := s.State.AddOneMachine(template)
	c.Assert(err, jc.ErrorIsNil)

	// Provision just one of the volumes, but neither of the attachments.
	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	err = sb.SetVolumeInfo(names.NewVolumeTag("1"), state.VolumeInfo{
		Pool:       "modelscoped",
		Size:       1000,
		VolumeId:   "vol-ume",
		Persistent: true,
	})
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: machine.Tag().String()},
	}}
	result, err := s.provisioner.ProvisioningInfo(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].Result, gc.NotNil)

	// volume-0 should be created, as it hasn't yet been provisioned.
	c.Assert(result.Results[0].Result.Volumes, jc.DeepEquals, []params.VolumeParams{{
		VolumeTag: "volume-0",
		Size:      1000,
		Provider:  "modelscoped",
		Tags: map[string]string{
			tags.JujuController: coretesting.ControllerTag.Id(),
			tags.JujuModel:      coretesting.ModelTag.Id(),
		},
		Attachment: &params.VolumeAttachmentParams{
			MachineTag: machine.Tag().String(),
			VolumeTag:  "volume-0",
			Provider:   "modelscoped",
		},
	}})

	// volume-1 has already been provisioned, it just needs to be attached.
	c.Assert(result.Results[0].Result.VolumeAttachments, jc.DeepEquals, []params.VolumeAttachmentParams{{
		MachineTag: machine.Tag().String(),
		VolumeTag:  "volume-1",
		VolumeId:   "vol-ume",
		Provider:   "modelscoped",
	}})
}

func (s *withoutControllerSuite) TestProviderInfoCloudInitUserData(c *gc.C) {
	attrs := map[string]interface{}{"cloudinit-userdata": validCloudInitUserData}
	err := s.Model.UpdateModelConfig(attrs, nil)
	c.Assert(err, jc.ErrorIsNil)
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	m, err := s.State.AddOneMachine(template)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: m.Tag().String()},
	}}
	result, err := s.provisioner.ProvisioningInfo(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Result.CloudInitUserData, gc.DeepEquals, map[string]interface{}{
		"packages":        []interface{}{"python-keystoneclient", "python-glanceclient"},
		"preruncmd":       []interface{}{"mkdir /tmp/preruncmd", "mkdir /tmp/preruncmd2"},
		"postruncmd":      []interface{}{"mkdir /tmp/postruncmd", "mkdir /tmp/postruncmd2"},
		"package_upgrade": false})
}

var validCloudInitUserData = `
packages:
  - 'python-keystoneclient'
  - 'python-glanceclient'
preruncmd:
  - mkdir /tmp/preruncmd
  - mkdir /tmp/preruncmd2
postruncmd:
  - mkdir /tmp/postruncmd
  - mkdir /tmp/postruncmd2
package_upgrade: false
`[1:]

func (s *withoutControllerSuite) TestProvisioningInfoPermissions(c *gc.C) {
	// Login as a machine agent for machine 0.
	anAuthorizer := s.authorizer
	anAuthorizer.Controller = false
	anAuthorizer.Tag = s.machines[0].Tag()
	aProvisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(aProvisioner, gc.NotNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[0].Tag().String() + "-lxd-0"},
		{Tag: "machine-42"},
		{Tag: s.machines[1].Tag().String()},
		{Tag: "application-bar"},
	}}

	// Only machine 0 and containers therein can be accessed.
	results, err := aProvisioner.ProvisioningInfo(args)
	c.Assert(err, jc.ErrorIsNil)
	controllerCfg := s.ControllerConfig
	c.Assert(results, jc.DeepEquals, params.ProvisioningInfoResultsV10{
		Results: []params.ProvisioningInfoResultV10{
			{Result: &params.ProvisioningInfoV10{
				ProvisioningInfoBase: params.ProvisioningInfoBase{
					ControllerConfig: controllerCfg,
					Series:           "quantal",
					Jobs:             []model.MachineJob{model.JobHostUnits},
					Tags: map[string]string{
						tags.JujuController: coretesting.ControllerTag.Id(),
						tags.JujuModel:      coretesting.ModelTag.Id(),
						tags.JujuMachine:    "controller-machine-0",
					},
					EndpointBindings: make(map[string]string),
				},
			}},
			{Error: apiservertesting.NotFoundError("machine 0/lxd/0")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}
