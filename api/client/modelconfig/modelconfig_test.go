// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig_test

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/modelconfig"
	"github.com/juju/juju/core/constraints"
	modeltesting "github.com/juju/juju/core/model/testing"
	modelerrors "github.com/juju/juju/domain/model/errors"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
)

type modelconfigSuite struct{}

var _ = tc.Suite(&modelconfigSuite{})

func (s *modelconfigSuite) TestModelGet(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var args interface{}
	res := new(params.ModelConfigResults)
	results := params.ModelConfigResults{
		Config: map[string]params.ConfigValue{
			"foo": {"bar", "model"},
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ModelGet", args, res).SetArg(3, results).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	result, err := client.ModelGet(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, map[string]interface{}{
		"foo": "bar",
	})
}

func (s *modelconfigSuite) TestModelGetWithMetadata(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var args interface{}
	res := new(params.ModelConfigResults)
	results := params.ModelConfigResults{
		Config: map[string]params.ConfigValue{
			"foo": {"bar", "model"},
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ModelGet", args, res).SetArg(3, results).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	result, err := client.ModelGetWithMetadata(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, config.ConfigValues{
		"foo": {"bar", "model"},
	})
}

func (s *modelconfigSuite) TestModelSet(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var res interface{}
	args := params.ModelSet{
		Config: map[string]interface{}{
			"some-name":  "value",
			"other-name": true,
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ModelSet", args, res).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	err := client.ModelSet(context.Background(), map[string]interface{}{
		"some-name":  "value",
		"other-name": true,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelconfigSuite) TestModelUnset(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var res interface{}
	args := params.ModelUnset{
		Keys: []string{"foo", "bar"},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ModelUnset", args, res).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	err := client.ModelUnset(context.Background(), "foo", "bar")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelconfigSuite) TestSequences(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var args interface{}
	res := new(params.ModelSequencesResult)
	results := params.ModelSequencesResult{
		Sequences: map[string]int{"foo": 5, "bar": 2},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Sequences", args, res).SetArg(3, results).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	sequences, err := client.Sequences(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sequences, jc.DeepEquals, map[string]int{"foo": 5, "bar": 2})
}

func (s *modelconfigSuite) TestGetModelConstraints(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var args interface{}
	res := new(params.GetConstraintsResults)
	results := params.GetConstraintsResults{
		Constraints: constraints.MustParse("arch=amd64"),
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "GetModelConstraints", args, res).SetArg(3, results).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	result, err := client.GetModelConstraints(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, constraints.MustParse("arch=amd64"))
}

func (s *modelconfigSuite) TestSetModelConstraints(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var res interface{}
	args := params.SetConstraints{
		Constraints: constraints.MustParse("arch=amd64"),
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "SetModelConstraints", args, res).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	err := client.SetModelConstraints(context.Background(), constraints.MustParse("arch=amd64"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelconfigSuite) TestGetModelSecretBackendNotSupported(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().BestAPIVersion().Return(3)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	_, err := client.GetModelSecretBackend(context.Background())
	c.Assert(err, tc.ErrorMatches, "getting model secret backend not supported")
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}

func (s *modelconfigSuite) TestGetModelSecretBackendModelNotFound(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelID := modeltesting.GenModelUUID(c)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().BestAPIVersion().Return(4)
	results := params.StringResult{
		Error: &params.Error{
			Code:    params.CodeModelNotFound,
			Message: fmt.Sprintf("model %q not found", modelID),
		},
	}
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "GetModelSecretBackend", nil, gomock.Any()).SetArg(3, results).Return(nil)

	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	_, err := client.GetModelSecretBackend(context.Background())
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf("model %q not found", modelID))
}

func (s *modelconfigSuite) TestGetModelSecretBackend(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().BestAPIVersion().Return(4)
	results := params.StringResult{
		Result: "backend-id",
	}
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "GetModelSecretBackend", nil, gomock.Any()).SetArg(3, results).Return(nil)

	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	result, err := client.GetModelSecretBackend(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, tc.Equals, "backend-id")
}

func (s *modelconfigSuite) TestSetModelSecretBackendNotSupported(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().BestAPIVersion().Return(3)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	err := client.SetModelSecretBackend(context.Background(), "backend-id")
	c.Assert(err, tc.ErrorMatches, "setting model secret backend not supported")
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}

func (s *modelconfigSuite) TestSetModelSecretBackend(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().BestAPIVersion().Return(4)
	results := params.ErrorResult{}
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "SetModelSecretBackend", params.SetModelSecretBackendArg{
		SecretBackendName: "backend-id",
	}, gomock.Any()).SetArg(3, results).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	err := client.SetModelSecretBackend(context.Background(), "backend-id")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelconfigSuite) TestSetModelSecretBackendFaildBackendNotFound(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().BestAPIVersion().Return(4)
	results := params.ErrorResult{
		Error: &params.Error{
			Code: params.CodeSecretBackendNotFound,
		},
	}
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "SetModelSecretBackend", params.SetModelSecretBackendArg{
		SecretBackendName: "backend-id",
	}, gomock.Any()).SetArg(3, results).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	err := client.SetModelSecretBackend(context.Background(), "backend-id")
	c.Assert(err, jc.ErrorIs, secretbackenderrors.NotFound)
}

func (s *modelconfigSuite) TestSetModelSecretBackendFaildBackendNotValid(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().BestAPIVersion().Return(4)
	results := params.ErrorResult{
		Error: &params.Error{
			Code: params.CodeSecretBackendNotValid,
		},
	}
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "SetModelSecretBackend", params.SetModelSecretBackendArg{
		SecretBackendName: "backend-id",
	}, gomock.Any()).SetArg(3, results).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	err := client.SetModelSecretBackend(context.Background(), "backend-id")
	c.Assert(err, jc.ErrorIs, secretbackenderrors.NotValid)
}

func (s *modelconfigSuite) TestSetModelSecretBackendFaildModelNotFound(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelID := modeltesting.GenModelUUID(c)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().BestAPIVersion().Return(4)
	results := params.ErrorResult{
		Error: &params.Error{
			Code:    params.CodeModelNotFound,
			Message: fmt.Sprintf("model %q not found", modelID),
		},
	}
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "SetModelSecretBackend", params.SetModelSecretBackendArg{
		SecretBackendName: "backend-id",
	}, gomock.Any()).SetArg(3, results).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	err := client.SetModelSecretBackend(context.Background(), "backend-id")
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf("model %q not found", modelID))
}
