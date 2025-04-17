// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	coreerrors "github.com/juju/juju/core/errors"
	modeltesting "github.com/juju/juju/core/model/testing"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modelprovider"
	"github.com/juju/juju/environs/cloudspec"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type serviceSuite struct {
	jtesting.IsolationSuite
}

var _ = gc.Suite(&serviceSuite{})

var (
	testCloud = cloud.Cloud{
		Name:      "test",
		Type:      "ec2",
		AuthTypes: []cloud.AuthType{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
		Endpoint:  "https://endpoint",
		Regions: []cloud.Region{{
			Name:             "test-region",
			Endpoint:         "http://region-endpoint1",
			IdentityEndpoint: "http://region-identity-endpoint1",
			StorageEndpoint:  "http://region-storage-endpoint1",
		}},
		CACertificates:    []string{"cert1"},
		SkipTLSVerify:     true,
		IsControllerCloud: false,
	}
	testCredential = modelprovider.CloudCredentialInfo{
		AuthType: "userpass",
		Attributes: map[string]string{
			"foo": "bar",
		},
	}
)

func (s *serviceSuite) TestGetCloudSpec(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	st := NewMockState(ctrl)
	st.EXPECT().GetModelCloudAndCredential(gomock.Any(), modelUUID).Return(&testCloud, "test-region", &testCredential, nil)

	svc := NewService(modelUUID, st, loggertesting.WrapCheckLog(c), nil)
	spec, err := svc.GetCloudSpec(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	cred := cloud.NewCredential(testCredential.AuthType, testCredential.Attributes)
	c.Assert(spec, jc.DeepEquals, cloudspec.CloudSpec{
		Type:              "ec2",
		Name:              "test",
		Region:            "test-region",
		Endpoint:          "http://region-endpoint1",
		IdentityEndpoint:  "http://region-identity-endpoint1",
		StorageEndpoint:   "http://region-storage-endpoint1",
		Credential:        &cred,
		CACertificates:    []string{"cert1"},
		SkipTLSVerify:     true,
		IsControllerCloud: false,
	})
}

func (s *serviceSuite) TestGetCloudSpecNoCredential(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	st := NewMockState(ctrl)
	st.EXPECT().GetModelCloudAndCredential(gomock.Any(), modelUUID).Return(&testCloud, "test-region", nil, nil)

	svc := NewService(modelUUID, st, loggertesting.WrapCheckLog(c), nil)
	spec, err := svc.GetCloudSpec(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spec, jc.DeepEquals, cloudspec.CloudSpec{
		Type:              "ec2",
		Name:              "test",
		Region:            "test-region",
		Endpoint:          "http://region-endpoint1",
		IdentityEndpoint:  "http://region-identity-endpoint1",
		StorageEndpoint:   "http://region-storage-endpoint1",
		CACertificates:    []string{"cert1"},
		SkipTLSVerify:     true,
		IsControllerCloud: false,
	})
}

func (s *serviceSuite) TestGetCloudSpecModelNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	st := NewMockState(ctrl)
	st.EXPECT().GetModelCloudAndCredential(gomock.Any(), modelUUID).Return(nil, "", nil, modelerrors.NotFound)

	svc := NewService(modelUUID, st, loggertesting.WrapCheckLog(c), nil)
	_, err := svc.GetCloudSpec(context.Background())
	c.Assert(err, jc.ErrorIs, coreerrors.NotFound)
}

func (s *serviceSuite) TestGetCloudSpecForSSH(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	st := NewMockState(ctrl)
	st.EXPECT().GetModelCloudAndCredential(gomock.Any(), modelUUID).Return(&testCloud, "test-region", &testCredential, nil)

	provider := NewMockProviderWithSecretToken(ctrl)
	provider.EXPECT().GetSecretToken(gomock.Any(), "model-exec").Return("secret", nil)

	svc := NewService(modelUUID, st, loggertesting.WrapCheckLog(c), func(ctx context.Context) (ProviderWithSecretToken, error) {
		return provider, nil
	})
	spec, err := svc.GetCloudSpecForSSH(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	testCredential.Attributes["Token"] = "secret"
	testCredential.Attributes["username"] = ""
	testCredential.Attributes["password"] = ""
	cred := cloud.NewCredential(testCredential.AuthType, testCredential.Attributes)
	c.Assert(spec, jc.DeepEquals, cloudspec.CloudSpec{
		Type:              "ec2",
		Name:              "test",
		Region:            "test-region",
		Endpoint:          "http://region-endpoint1",
		IdentityEndpoint:  "http://region-identity-endpoint1",
		StorageEndpoint:   "http://region-storage-endpoint1",
		Credential:        &cred,
		CACertificates:    []string{"cert1"},
		SkipTLSVerify:     true,
		IsControllerCloud: false,
	})
}

func (s *serviceSuite) TestGetCloudSpecForSSHNotSupported(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	st := NewMockState(ctrl)

	svc := NewService(modelUUID, st, loggertesting.WrapCheckLog(c), func(ctx context.Context) (ProviderWithSecretToken, error) {
		return nil, coreerrors.NotSupported
	})
	_, err := svc.GetCloudSpecForSSH(context.Background())
	c.Assert(err, jc.ErrorIs, coreerrors.NotSupported)
}
