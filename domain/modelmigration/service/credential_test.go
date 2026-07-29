// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	credentialservice "github.com/juju/juju/domain/credential/service"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

// expectCredentialValidationInfo sets up the single model-state read that
// [Service.CheckMachines] makes before validating the credential.
func (s *serviceSuite) expectCredentialValidationInfo(cloudType string) {
	s.modelState.EXPECT().GetCredentialValidationInfo(gomock.Any()).
		Return(modelmigration.CredentialValidationInfo{
			ControllerUUID: s.controllerUUID,
			ModelType:      "iaas",
			CloudName:      "aws",
			CloudType:      cloudType,
			CloudRegion:    "myregion",
			Config: map[string]string{
				"uuid": s.modelUUID,
				"name": "my-model",
				"type": "iaas",
			},
		}, nil)
}

// TestCheckMachinesCredentialError checks that [Service.CheckMachines] fails
// before any provider instance reconciliation when the model credential
// cannot be read.
func (s *serviceSuite) TestCheckMachinesCredentialError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectCredentialValidationInfo("ec2")
	s.controllerState.EXPECT().GetModelCloudCredential(gomock.Any(), s.modelUUID).
		Return(nil, errors.Errorf("boom"))

	_, err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
		loggertesting.WrapCheckLog(c),
	).CheckMachines(c.Context())
	c.Assert(err, tc.ErrorMatches, ".*validating model credential: getting model cloud credential: boom")
}

// TestCheckMachinesRevokedCredential checks that [Service.CheckMachines]
// rejects a model whose credential is revoked, without invoking the validator.
func (s *serviceSuite) TestCheckMachinesRevokedCredential(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectCredentialValidationInfo("ec2")
	s.controllerState.EXPECT().GetModelCloudCredential(gomock.Any(), s.modelUUID).
		Return(&modelmigration.ModelCloudCredential{
			Cloud:   "aws",
			Owner:   "fred",
			Name:    "default",
			Revoked: true,
		}, nil)

	_, err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
		loggertesting.WrapCheckLog(c),
	).CheckMachines(c.Context())
	c.Assert(err, tc.ErrorMatches, ".*credential.*revoked.*")
}

// TestCheckMachinesCredentialValidationError checks that [Service.CheckMachines]
// fails when the credential validator rejects the imported credential.
func (s *serviceSuite) TestCheckMachinesCredentialValidationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	credential := modelmigration.ModelCloudCredential{
		Cloud:      "aws",
		Owner:      "fred",
		Name:       "default",
		AuthType:   "access-key",
		Attributes: map[string]string{"access-key": "foo"},
	}
	s.expectCredentialValidationInfo("ec2")
	s.controllerState.EXPECT().GetModelCloudCredential(gomock.Any(), s.modelUUID).
		Return(&credential, nil)
	s.credentialValidator.EXPECT().Validate(gomock.Any(), gomock.Any(), credential).
		Return(errors.Errorf("invalid credential"))

	_, err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
		loggertesting.WrapCheckLog(c),
	).CheckMachines(c.Context())
	c.Assert(err, tc.ErrorMatches, ".*invalid credential.*")
}

// TestCheckMachinesInvalidCredential checks that a credential Juju has already
// marked invalid is refused outright, without asking the provider.
func (s *serviceSuite) TestCheckMachinesInvalidCredential(c *tc.C) {
	defer s.setupMocks(c).Finish()

	credential := modelmigration.ModelCloudCredential{
		Cloud:         "aws",
		Owner:         "fred",
		Name:          "default",
		AuthType:      "access-key",
		Invalid:       true,
		InvalidReason: "cloud rejected the key",
	}
	s.expectCredentialValidationInfo("ec2")
	s.controllerState.EXPECT().GetModelCloudCredential(gomock.Any(), s.modelUUID).
		Return(&credential, nil)

	_, err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
		loggertesting.WrapCheckLog(c),
	).CheckMachines(c.Context())
	c.Assert(err, tc.ErrorMatches, `.*credential "default" on cloud "aws" for owner "fred" is not valid: cloud rejected the key.*`)
}

// TestCheckMachinesUnmanagedCloudIgnoresUntrackedInstances checks 3.6 parity
// for an unmanaged ("manual" in 3.6) cloud: its machines are not provisioned by
// Juju, so an instance the model does not track is not a discrepancy. A model
// machine the cloud has lost is still reported.
func (s *serviceSuite) TestCheckMachinesUnmanagedCloudIgnoresUntrackedInstances(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectCredentialValidationInfo("unmanaged")
	s.controllerState.EXPECT().GetModelCloudCredential(gomock.Any(), s.modelUUID).Return(nil, nil)
	s.instanceProvider.EXPECT().AllInstances(gomock.Any()).
		Return([]instances.Instance{&instanceStub{id: "instance0"}}, nil)
	s.modelState.EXPECT().GetMachineInstanceIDs(gomock.Any()).
		Return(map[string]string{"instance1": "1"}, nil)

	discrepancies, err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
		loggertesting.WrapCheckLog(c),
	).CheckMachines(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(discrepancies, tc.DeepEquals, []modelmigration.MigrationMachineDiscrepancy{{
		MachineName:     "1",
		CloudInstanceId: instance.Id("instance1"),
	}})
}

// TestCredentialValidationContext verifies the credential validation context is
// assembled from the model information read for the phase plus the target
// controller's definition of the cloud.
func (s *serviceSuite) TestCredentialValidationContext(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cld := cloud.Cloud{
		Name:      "aws",
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType},
	}
	s.controllerState.EXPECT().GetCloud(gomock.Any(), "aws").Return(cld, nil)

	v := credentialValidator{
		controllerState: s.controllerState,
		modelState:      s.modelState,
	}
	got, err := v.validationContext(c.Context(), modelmigration.CredentialValidationInfo{
		ControllerUUID: s.controllerUUID,
		ModelType:      "iaas",
		CloudName:      "aws",
		CloudType:      "ec2",
		CloudRegion:    "myregion",
		Config: map[string]string{
			"uuid":      s.modelUUID,
			"name":      "my-model",
			"type":      "iaas",
			"ftp-proxy": "http://proxy",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.ControllerUUID, tc.Equals, s.controllerUUID)
	c.Check(got.ModelType, tc.Equals, coremodel.IAAS)
	c.Check(got.Cloud, tc.DeepEquals, cld)
	c.Check(got.Region, tc.Equals, "myregion")
	c.Assert(got.Config, tc.NotNil)
	c.Check(got.Config.AllAttrs()["ftp-proxy"], tc.Equals, "http://proxy")
	c.Assert(got.MachineService, tc.NotNil)
}

// TestCredentialValidationContextCloudError verifies a failure reading the
// target controller's cloud aborts the context assembly.
func (s *serviceSuite) TestCredentialValidationContextCloudError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerState.EXPECT().GetCloud(gomock.Any(), "aws").
		Return(cloud.Cloud{}, errors.Errorf("boom"))

	v := credentialValidator{
		controllerState: s.controllerState,
		modelState:      s.modelState,
	}
	_, err := v.validationContext(c.Context(), modelmigration.CredentialValidationInfo{CloudName: "aws"})
	c.Assert(err, tc.ErrorMatches, ".*getting model cloud: boom")
}

// TestCredentialKeyRejectsIncompleteCredential verifies an imported credential
// missing part of its natural key, or its auth type, is refused before any
// provider is opened with it.
func (s *serviceSuite) TestCredentialKeyRejectsIncompleteCredential(c *tc.C) {
	complete := modelmigration.ModelCloudCredential{
		Cloud:    "aws",
		Owner:    "fred",
		Name:     "default",
		AuthType: "access-key",
	}

	key, err := credentialKey(complete)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key.Cloud, tc.Equals, "aws")
	c.Check(key.Name, tc.Equals, "default")
	c.Check(key.Owner.Name(), tc.Equals, "fred")

	noCloud := complete
	noCloud.Cloud = ""
	_, err = credentialKey(noCloud)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)

	noName := complete
	noName.Name = ""
	_, err = credentialKey(noName)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)

	badOwner := complete
	badOwner.Owner = "!not a user!"
	_, err = credentialKey(badOwner)
	c.Check(err, tc.ErrorMatches, ".*parsing credential owner.*")

	noAuthType := complete
	noAuthType.AuthType = ""
	_, err = credentialKey(noAuthType)
	c.Check(err, tc.ErrorMatches, ".*has no auth type")
}

// stubCredentialValidator records the arguments the migration validator passes
// down to the credential domain, and reports the supplied machine errors.
type stubCredentialValidator struct {
	checkCloudInstances bool
	validationContext   credentialservice.CredentialValidationContext
	machineErrors       []error
	err                 error
}

func (v *stubCredentialValidator) Validate(
	_ context.Context,
	validationContext credentialservice.CredentialValidationContext,
	_ corecredential.Key,
	_ *cloud.Credential,
	checkCloudInstances bool,
) ([]error, error) {
	v.validationContext = validationContext
	v.checkCloudInstances = checkCloudInstances
	return v.machineErrors, v.err
}

// TestCredentialValidatorSkipsCloudInstancesForUnmanaged checks 3.6 parity: the
// 1:1 machine/instance mapping is only required of clouds Juju provisions, so
// an unmanaged cloud ("manual" in 3.6) is validated without it.
func (s *serviceSuite) TestCredentialValidatorSkipsCloudInstancesForUnmanaged(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerState.EXPECT().GetCloud(gomock.Any(), "aws").
		Return(cloud.Cloud{Name: "aws", Type: "ec2"}, nil).Times(2)

	credential := modelmigration.ModelCloudCredential{
		Cloud:    "aws",
		Owner:    "fred",
		Name:     "default",
		AuthType: "access-key",
	}
	info := modelmigration.CredentialValidationInfo{
		ControllerUUID: s.controllerUUID,
		ModelType:      "iaas",
		CloudName:      "aws",
		CloudType:      "ec2",
		Config:         map[string]string{"uuid": s.modelUUID, "name": "my-model", "type": "iaas"},
	}

	stub := &stubCredentialValidator{}
	v := NewCredentialValidator(s.controllerState, s.modelState, stub)
	c.Assert(v.Validate(c.Context(), info, credential), tc.ErrorIsNil)
	c.Check(stub.checkCloudInstances, tc.IsTrue)

	info.CloudType = "unmanaged"
	c.Assert(v.Validate(c.Context(), info, credential), tc.ErrorIsNil)
	c.Check(stub.checkCloudInstances, tc.IsFalse)
}

// TestCredentialValidatorReportsMachineErrors checks the discrepancies the
// credential domain reports are surfaced as a validation failure.
func (s *serviceSuite) TestCredentialValidatorReportsMachineErrors(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerState.EXPECT().GetCloud(gomock.Any(), "aws").
		Return(cloud.Cloud{Name: "aws", Type: "ec2"}, nil)

	stub := &stubCredentialValidator{
		machineErrors: []error{errors.Errorf("couldn't find instance %q for machine %q", "i-1", "0")},
	}
	v := NewCredentialValidator(s.controllerState, s.modelState, stub)
	err := v.Validate(c.Context(),
		modelmigration.CredentialValidationInfo{
			ControllerUUID: s.controllerUUID,
			ModelType:      "iaas",
			CloudName:      "aws",
			CloudType:      "ec2",
			Config:         map[string]string{"uuid": s.modelUUID, "name": "my-model", "type": "iaas"},
		},
		modelmigration.ModelCloudCredential{
			Cloud:    "aws",
			Owner:    "fred",
			Name:     "default",
			AuthType: "access-key",
		},
	)
	c.Assert(err, tc.ErrorMatches, `.*model credential validation failed:.*couldn't find instance "i-1" for machine "0".*`)
}

// TestCredentialMachineService verifies the migration state is adapted to the
// credential domain's machine interface.
func (s *serviceSuite) TestCredentialMachineService(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetMachineInstanceIDs(gomock.Any()).Return(map[string]string{
		"instance-0": "0",
		"instance-1": "1",
	}, nil)
	s.modelState.EXPECT().GetMachineInstanceID(gomock.Any(), "machine-uuid").Return("instance-0", nil)

	ms := machineService{modelState: s.modelState}
	all, err := ms.GetAllProvisionedMachineInstanceID(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(all, tc.DeepEquals, map[machine.Name]instance.Id{
		"0": instance.Id("instance-0"),
		"1": instance.Id("instance-1"),
	})

	id, err := ms.InstanceID(c.Context(), machine.UUID("machine-uuid"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(id, tc.Equals, "instance-0")
}
