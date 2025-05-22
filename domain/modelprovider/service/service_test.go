// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cloud"
	coreerrors "github.com/juju/juju/core/errors"
	modeltesting "github.com/juju/juju/core/model/testing"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modelprovider"
	"github.com/juju/juju/environs/cloudspec"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type serviceSuite struct {
	testhelpers.IsolationSuite
}

func TestServiceSuite(t *stdtesting.T) {
	tc.Run(t, &serviceSuite{})
}

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

func (s *serviceSuite) TestGetCloudSpec(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	st := NewMockState(ctrl)
	st.EXPECT().GetModelCloudAndCredential(gomock.Any(), modelUUID).Return(&testCloud, "test-region", &testCredential, nil)

	svc := NewService(modelUUID, st, loggertesting.WrapCheckLog(c), nil)
	spec, err := svc.GetCloudSpec(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	cred := cloud.NewCredential(testCredential.AuthType, testCredential.Attributes)
	c.Assert(spec, tc.DeepEquals, cloudspec.CloudSpec{
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

func (s *serviceSuite) TestGetCloudSpecNoCredential(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	st := NewMockState(ctrl)
	st.EXPECT().GetModelCloudAndCredential(gomock.Any(), modelUUID).Return(&testCloud, "test-region", nil, nil)

	svc := NewService(modelUUID, st, loggertesting.WrapCheckLog(c), nil)
	spec, err := svc.GetCloudSpec(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(spec, tc.DeepEquals, cloudspec.CloudSpec{
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

func (s *serviceSuite) TestGetCloudSpecModelNotFound(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	st := NewMockState(ctrl)
	st.EXPECT().GetModelCloudAndCredential(gomock.Any(), modelUUID).Return(nil, "", nil, modelerrors.NotFound)

	svc := NewService(modelUUID, st, loggertesting.WrapCheckLog(c), nil)
	_, err := svc.GetCloudSpec(c.Context())
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}

func (s *serviceSuite) TestGetCloudSpecForSSH(c *tc.C) {
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
	spec, err := svc.GetCloudSpecForSSH(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	testCredential.Attributes["Token"] = "secret"
	testCredential.Attributes["username"] = ""
	testCredential.Attributes["password"] = ""
	cred := cloud.NewCredential(testCredential.AuthType, testCredential.Attributes)
	c.Assert(spec, tc.DeepEquals, cloudspec.CloudSpec{
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

func (s *serviceSuite) TestGetCloudSpecForSSHNotSupported(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	st := NewMockState(ctrl)

	svc := NewService(modelUUID, st, loggertesting.WrapCheckLog(c), func(ctx context.Context) (ProviderWithSecretToken, error) {
		return nil, coreerrors.NotSupported
	})
	_, err := svc.GetCloudSpecForSSH(c.Context())
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
}
