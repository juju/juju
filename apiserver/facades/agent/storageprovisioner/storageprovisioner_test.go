// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"context"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/storageprovisioner"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type provisionerSuite struct {
	jujutesting.ApiServerSuite

	storageSetUp

	st             *state.State
	resources      *common.Resources
	authorizer     *apiservertesting.FakeAuthorizer
	api            *storageprovisioner.StorageProvisionerAPIv4
	storageBackend storageprovisioner.StorageBackend
}

// modelConfigService is a convenience function to get the controller model's
// model config service inside a test.
func (s *provisionerSuite) modelConfigService(c *gc.C) storageprovisioner.ModelConfigService {
	return s.ControllerDomainServices(c).Config()
}

func (s *provisionerSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)
}

func (s *provisionerSuite) TestNewStorageProvisionerAPINonMachine(c *gc.C) {
	tag := names.NewUnitTag("mysql/0")
	authorizer := &apiservertesting.FakeAuthorizer{Tag: tag}
	backend, storageBackend, err := storageprovisioner.NewStateBackends(s.st)
	c.Assert(err, jc.ErrorIsNil)

	modelInfo, err := s.ControllerDomainServices(c).ModelInfo().GetModelInfo(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	_, err = storageprovisioner.NewStorageProvisionerAPIv4(
		context.Background(),
		nil,
		backend,
		storageBackend,
		s.DefaultModelDomainServices(c).BlockDevice(),
		s.ControllerDomainServices(c).Config(),
		s.DefaultModelDomainServices(c).Machine(),
		common.NewResources(),
		authorizer,
		nil, nil,
		loggertesting.WrapCheckLog(c),
		modelInfo.UUID,
		testing.ControllerTag.Id(),
	)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *provisionerSuite) TestVolumesEmptyArgs(c *gc.C) {
	results, err := s.api.Volumes(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}

func (s *provisionerSuite) TestVolumeParamsEmptyArgs(c *gc.C) {
	results, err := s.api.VolumeParams(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}
