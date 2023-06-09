// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/caas"
	corecontainer "github.com/juju/juju/core/container"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/names/v4"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/apiserver/facades/agent/provisioner"
	"github.com/juju/juju/apiserver/facades/agent/provisioner/mocks"
	"github.com/juju/juju/environs/imagemetadata"
	imagetesting "github.com/juju/juju/environs/imagemetadata/testing"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/cloudimagemetadata"
)

// useTestImageData causes the given content to be served when published metadata is requested.
func useTestImageData(c *gc.C, files map[string]string) {
	if files != nil {
		sstesting.SetRoundTripperFiles(sstesting.AddSignedFiles(c, files), nil)
	} else {
		sstesting.SetRoundTripperFiles(nil, nil)
	}
}

type ImageMetadataSuite struct {
	provisionerSuite

	ctrlConfigService *mocks.MockControllerConfigGetter
}

var _ = gc.Suite(&ImageMetadataSuite{})

func (s *ImageMetadataSuite) SetUpSuite(c *gc.C) {
	s.provisionerSuite.SetUpSuite(c)
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ctrlConfigService = mocks.NewMockControllerConfigGetter(ctrl)

	// Make sure that there is nothing in data sources.
	// Each individual tests will decide if it needs metadata there.
	imagetesting.PatchOfficialDataSources(&s.CleanupSuite, "test:/daily")
	s.PatchValue(&imagemetadata.SimplestreamsImagesPublicKey, sstesting.SignedMetadataPublicKey)
	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
	useTestImageData(c, nil)
}

func (s *ImageMetadataSuite) TearDownSuite(c *gc.C) {
	useTestImageData(c, nil)
	s.provisionerSuite.TearDownSuite(c)
}

func (s *ImageMetadataSuite) SetUpTest(c *gc.C) {
	s.provisionerSuite.SetUpTest(c)
}

func (s *ImageMetadataSuite) TestMetadataNone(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// Create a provisioner API for the machine.
	facadeContext := s.facadeContext()
	getAuthFunc := func() (common.AuthFunc, error) {
		isModelManager := s.authorizer.AuthController()
		isMachineAgent := s.authorizer.AuthMachineAgent()
		authEntityTag := s.authorizer.GetAuthTag()

		return func(tag names.Tag) bool {
			if isMachineAgent && tag == authEntityTag {
				// A machine agent can always access its own machine.
				return true
			}
			switch tag := tag.(type) {
			case names.MachineTag:
				parentId := corecontainer.ParentId(tag.Id())
				if parentId == "" {
					// All top-level machines are accessible by the controller.
					return isModelManager
				}
				// All containers with the authenticated machine as a
				// parent are accessible by it.
				// TODO(dfc) sometimes authEntity tag is nil, which is fine because nil is
				// only equal to nil, but it suggests someone is passing an authorizer
				// with a nil tag.
				return isMachineAgent && names.NewMachineTag(parentId) == authEntityTag
			default:
				return false
			}
		}, nil
	}
	getCanModify := func() (common.AuthFunc, error) {
		return s.authorizer.AuthOwner, nil
	}
	getAuthOwner := func() (common.AuthFunc, error) {
		return s.authorizer.AuthOwner, nil
	}
	st := facadeContext.State()
	model, err := st.Model()

	configGetter := stateenvirons.EnvironConfigGetter{Model: model}
	isCaasModel := model.Type() == state.ModelTypeCAAS

	var env storage.ProviderRegistry
	if isCaasModel {
		env, err = stateenvirons.GetNewCAASBrokerFunc(caas.New)(model)
	} else {
		env, err = environs.GetEnviron(configGetter, environs.New)
	}
	storageProviderRegistry := stateenvirons.NewStorageProviderRegistry(env)
	netConfigAPI, err := networkingcommon.NewNetworkConfigAPI(st, getCanModify)
	systemState, err := facadeContext.StatePool().SystemState()
	urlGetter := common.NewToolsURLGetter(model.UUID(), systemState)
	callCtx := context.CallContext(st)
	resources := facadeContext.Resources()
	api, err := provisioner.NewProvisionerAPI(
		common.NewRemover(st, nil, false, getAuthFunc),
		common.NewStatusSetter(st, getAuthFunc),
		common.NewStatusGetter(st, getAuthFunc),
		common.NewDeadEnsurer(st, nil, getAuthFunc),
		common.NewPasswordChanger(st, getAuthFunc),
		common.NewLifeGetter(st, getAuthFunc),
		common.NewAPIAddresser(systemState, resources, s.ctrlConfigService),
		common.NewModelWatcher(model, resources, s.authorizer),
		common.NewModelMachinesWatcher(st, resources, s.authorizer),
		common.NewStateControllerConfig(st, s.ctrlConfigService),
		netConfigAPI,
		st,
		model,
		resources,
		s.authorizer,
		configGetter,
		storageProviderRegistry,
		poolmanager.New(state.NewStateSettings(st), storageProviderRegistry),
		getAuthFunc,
		getCanModify,
		callCtx,
		facadeContext.Logger().Child("provisioner"),
		s.ctrlConfigService,
	)
	if !isCaasModel {
		newEnviron := func() (environs.BootstrapEnviron, error) {
			return environs.GetEnviron(configGetter, environs.New)
		}
		api.InstanceIdGetter = common.NewInstanceIdGetter(st, getAuthFunc)
		api.ToolsFinder = common.NewToolsFinder(configGetter, st, urlGetter, newEnviron)
		api.ToolsGetter = common.NewToolsGetter(st, configGetter, st, urlGetter, api.ToolsFinder, getAuthOwner, s.ctrlConfigService)
	}
	c.Assert(err, jc.ErrorIsNil)

	result, err := api.ProvisioningInfo(s.getTestMachinesTags(c))
	c.Assert(err, jc.ErrorIsNil)

	expected := make([][]params.CloudImageMetadata, len(s.machines))
	for i := range result.Results {
		expected[i] = nil
	}
	s.assertImageMetadataResults(c, result, expected...)
}

func (s *ImageMetadataSuite) TestMetadataFromState(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// Create a provisioner API for the machine.
	facadeContext := s.facadeContext()
	getAuthFunc := func() (common.AuthFunc, error) {
		isModelManager := s.authorizer.AuthController()
		isMachineAgent := s.authorizer.AuthMachineAgent()
		authEntityTag := s.authorizer.GetAuthTag()

		return func(tag names.Tag) bool {
			if isMachineAgent && tag == authEntityTag {
				// A machine agent can always access its own machine.
				return true
			}
			switch tag := tag.(type) {
			case names.MachineTag:
				parentId := corecontainer.ParentId(tag.Id())
				if parentId == "" {
					// All top-level machines are accessible by the controller.
					return isModelManager
				}
				// All containers with the authenticated machine as a
				// parent are accessible by it.
				// TODO(dfc) sometimes authEntity tag is nil, which is fine because nil is
				// only equal to nil, but it suggests someone is passing an authorizer
				// with a nil tag.
				return isMachineAgent && names.NewMachineTag(parentId) == authEntityTag
			default:
				return false
			}
		}, nil
	}
	getCanModify := func() (common.AuthFunc, error) {
		return s.authorizer.AuthOwner, nil
	}
	getAuthOwner := func() (common.AuthFunc, error) {
		return s.authorizer.AuthOwner, nil
	}
	st := facadeContext.State()
	model, err := st.Model()

	configGetter := stateenvirons.EnvironConfigGetter{Model: model}
	isCaasModel := model.Type() == state.ModelTypeCAAS

	var env storage.ProviderRegistry
	if isCaasModel {
		env, err = stateenvirons.GetNewCAASBrokerFunc(caas.New)(model)
	} else {
		env, err = environs.GetEnviron(configGetter, environs.New)
	}
	storageProviderRegistry := stateenvirons.NewStorageProviderRegistry(env)
	netConfigAPI, err := networkingcommon.NewNetworkConfigAPI(st, getCanModify)
	systemState, err := facadeContext.StatePool().SystemState()
	urlGetter := common.NewToolsURLGetter(model.UUID(), systemState)
	callCtx := context.CallContext(st)
	resources := facadeContext.Resources()
	api, err := provisioner.NewProvisionerAPI(
		common.NewRemover(st, nil, false, getAuthFunc),
		common.NewStatusSetter(st, getAuthFunc),
		common.NewStatusGetter(st, getAuthFunc),
		common.NewDeadEnsurer(st, nil, getAuthFunc),
		common.NewPasswordChanger(st, getAuthFunc),
		common.NewLifeGetter(st, getAuthFunc),
		common.NewAPIAddresser(systemState, resources, s.ctrlConfigService),
		common.NewModelWatcher(model, resources, s.authorizer),
		common.NewModelMachinesWatcher(st, resources, s.authorizer),
		common.NewStateControllerConfig(st, s.ctrlConfigService),
		netConfigAPI,
		st,
		model,
		resources,
		s.authorizer,
		configGetter,
		storageProviderRegistry,
		poolmanager.New(state.NewStateSettings(st), storageProviderRegistry),
		getAuthFunc,
		getCanModify,
		callCtx,
		facadeContext.Logger().Child("provisioner"),
		s.ctrlConfigService,
	)
	if !isCaasModel {
		newEnviron := func() (environs.BootstrapEnviron, error) {
			return environs.GetEnviron(configGetter, environs.New)
		}
		api.InstanceIdGetter = common.NewInstanceIdGetter(st, getAuthFunc)
		api.ToolsFinder = common.NewToolsFinder(configGetter, st, urlGetter, newEnviron)
		api.ToolsGetter = common.NewToolsGetter(st, configGetter, st, urlGetter, api.ToolsFinder, getAuthOwner, s.ctrlConfigService)
	}
	c.Assert(err, jc.ErrorIsNil)

	expected := s.expectedDataSoureImageMetadata()

	// Write metadata to state.
	metadata := s.convertCloudImageMetadata(expected[0])
	for _, m := range metadata {
		err := s.State.CloudImageMetadataStorage.SaveMetadata(
			[]cloudimagemetadata.Metadata{m},
		)
		c.Assert(err, jc.ErrorIsNil)
	}

	result, err := api.ProvisioningInfo(s.getTestMachinesTags(c))
	c.Assert(err, jc.ErrorIsNil)

	s.assertImageMetadataResults(c, result, expected...)
}

func (s *ImageMetadataSuite) getTestMachinesTags(c *gc.C) params.Entities {

	testMachines := make([]params.Entity, len(s.machines))

	for i, m := range s.machines {
		testMachines[i] = params.Entity{Tag: m.Tag().String()}
	}
	return params.Entities{Entities: testMachines}
}

func (s *ImageMetadataSuite) convertCloudImageMetadata(all []params.CloudImageMetadata) []cloudimagemetadata.Metadata {
	expected := make([]cloudimagemetadata.Metadata, len(all))
	for i, one := range all {
		expected[i] = cloudimagemetadata.Metadata{
			cloudimagemetadata.MetadataAttributes{
				Region:          one.Region,
				Version:         one.Version,
				Arch:            one.Arch,
				VirtType:        one.VirtType,
				RootStorageType: one.RootStorageType,
				Source:          one.Source,
				Stream:          one.Stream,
			},
			one.Priority,
			one.ImageId,
			0,
		}
	}
	return expected
}

func (s *ImageMetadataSuite) expectedDataSoureImageMetadata() [][]params.CloudImageMetadata {
	expected := make([][]params.CloudImageMetadata, len(s.machines))
	for i := range s.machines {
		expected[i] = []params.CloudImageMetadata{
			{ImageId: "ami-26745463",
				Region:          "dummy_region",
				Version:         "12.10",
				Arch:            "amd64",
				VirtType:        "pv",
				RootStorageType: "ebs",
				Stream:          "daily",
				Source:          "default cloud images",
				Priority:        10},
		}
	}
	return expected
}

func (s *ImageMetadataSuite) assertImageMetadataResults(
	c *gc.C, obtained params.ProvisioningInfoResults, expected ...[]params.CloudImageMetadata,
) {
	c.Assert(obtained.Results, gc.HasLen, len(expected))
	for i, one := range obtained.Results {
		// We are only concerned with images here
		c.Assert(one.Result.ImageMetadata, gc.DeepEquals, expected[i])
	}
}
