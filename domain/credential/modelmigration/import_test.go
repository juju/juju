// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"maps"
	"regexp"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/description/v12"
	"github.com/juju/tc"

	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	usertesting "github.com/juju/juju/core/user/testing"
	credentialerrors "github.com/juju/juju/domain/credential/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	coordinator *MockCoordinator
	service     *MockImportService
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockImportService(ctrl)

	return ctrl
}

func (s *importSuite) newImportOperation() *importOperation {
	return &importOperation{
		service: s.service,
	}
}

func (s *importSuite) TestRegisterImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImport(s.coordinator, loggertesting.WrapCheckLog(c))
}

func (s *importSuite) TestEmptyCredential(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Empty model.
	model := description.NewModel(description.ModelArgs{})

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
	// No import executed.
	s.service.EXPECT().InsertCloudCredential(gomock.All(), gomock.Any(), gomock.Any()).Times(0)
}

func (s *importSuite) TestImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Model with 2 external controllers.
	model := description.NewModel(description.ModelArgs{})
	model.SetCloudCredential(
		description.CloudCredentialArgs{
			Owner:      "fred",
			Cloud:      "cirrus",
			Name:       "foo",
			AuthType:   string(cloud.UserPassAuthType),
			Attributes: map[string]string{"hello": "world"},
		},
	)
	cred := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{"hello": "world"})
	key := credential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred"), Name: "foo"}
	s.service.EXPECT().CloudCredential(gomock.All(), key).Times(1).Return(cloud.Credential{}, credentialerrors.NotFound)
	s.service.EXPECT().InsertCloudCredential(gomock.Any(), key, cred).Times(1)

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportExistingMatches(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Model with 2 external controllers.
	model := description.NewModel(description.ModelArgs{})
	model.SetCloudCredential(
		description.CloudCredentialArgs{
			Owner:      "fred",
			Cloud:      "cirrus",
			Name:       "foo",
			AuthType:   string(cloud.UserPassAuthType),
			Attributes: map[string]string{"hello": "world"},
		},
	)
	cred := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{"hello": "world"})
	key := credential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred"), Name: "foo"}
	s.service.EXPECT().CloudCredential(gomock.All(), key).Times(1).Return(cred, nil)

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportExistingAuthTypeMisMatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Model with 2 external controllers.
	model := description.NewModel(description.ModelArgs{})
	model.SetCloudCredential(
		description.CloudCredentialArgs{
			Owner:      "fred",
			Cloud:      "cirrus",
			Name:       "foo",
			AuthType:   string(cloud.UserPassAuthType),
			Attributes: map[string]string{"hello": "world"},
		},
	)
	cred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{"hello": "world"})
	key := credential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred"), Name: "foo"}
	s.service.EXPECT().CloudCredential(gomock.All(), key).Times(1).Return(cred, nil)

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorMatches, `credential auth type mismatch: "access-key" != "userpass"`)
}

func (s *importSuite) TestImportExistingAttributesMisMatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Model with 2 external controllers.
	model := description.NewModel(description.ModelArgs{})
	model.SetCloudCredential(
		description.CloudCredentialArgs{
			Owner:      "fred",
			Cloud:      "cirrus",
			Name:       "foo",
			AuthType:   string(cloud.UserPassAuthType),
			Attributes: map[string]string{"hello": "world"},
		},
	)
	cred := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{"goodbye": "world"})
	key := credential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred"), Name: "foo"}
	s.service.EXPECT().CloudCredential(gomock.All(), key).Times(1).Return(cred, nil)

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Check(err, tc.ErrorMatches, regexp.QuoteMeta(`credential attribute mismatch for "cirrus/fred/foo"`))
}

func (s *importSuite) TestImportExistingKubernetesCredentialWithTargetRBACID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// A token-only export must match a target with RBAC bookkeeping metadata.
	model := description.NewModel(description.ModelArgs{Type: description.CAAS})
	model.SetCloudCredential(
		description.CloudCredentialArgs{
			Owner:    "fred",
			Cloud:    "kubernetes",
			Name:     "foo",
			AuthType: string(cloud.OAuth2AuthType),
			Attributes: map[string]string{
				"Token": "token",
			},
		},
	)
	cred := cloud.NewCredential(cloud.OAuth2AuthType, map[string]string{
		"Token":                   "token",
		k8scloud.RBACLabelKeyName: "target-controller-id",
	})
	key := credential.Key{Cloud: "kubernetes", Owner: usertesting.GenNewName(c, "fred"), Name: "foo"}
	s.service.EXPECT().CloudCredential(gomock.All(), key).Times(1).Return(cred, nil)

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportExistingKubernetesCredentialWithDifferentAuthenticationAttributes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ignoring RBAC metadata must not hide different authentication material.
	model := description.NewModel(description.ModelArgs{Type: description.CAAS})
	model.SetCloudCredential(
		description.CloudCredentialArgs{
			Owner:    "fred",
			Cloud:    "kubernetes",
			Name:     "foo",
			AuthType: string(cloud.OAuth2AuthType),
			Attributes: map[string]string{
				"Token": "source-token",
			},
		},
	)
	cred := cloud.NewCredential(cloud.OAuth2AuthType, map[string]string{
		"Token":                   "target-token",
		k8scloud.RBACLabelKeyName: "target-controller-id",
	})
	key := credential.Key{Cloud: "kubernetes", Owner: usertesting.GenNewName(c, "fred"), Name: "foo"}
	s.service.EXPECT().CloudCredential(gomock.All(), key).Times(1).Return(cred, nil)

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Check(err, tc.ErrorMatches, regexp.QuoteMeta(`credential attribute mismatch for "kubernetes/fred/foo"`))
}

func (s *importSuite) TestImportCredentialMetadata(c *tc.C) {
	// Exercise compatibility in both directions and ensure it cannot bypass
	// authentication or revocation checks or mutate either credential.
	for _, test := range []struct {
		name      string
		modelType string
		authType  cloud.AuthType
		existing  map[string]string
		imported  map[string]string
		revoked   bool
		wantError string
	}{
		{
			name: "different RBAC IDs", modelType: description.CAAS,
			existing: map[string]string{"Token": "token", "rbac-id": "target"},
			imported: map[string]string{"Token": "token", "rbac-id": "source"},
		}, {
			name: "source-only RBAC ID", modelType: description.CAAS,
			existing: map[string]string{"Token": "token"},
			imported: map[string]string{"Token": "token", "rbac-id": "source"},
		}, {
			name: "IAAS attributes stay strict", modelType: description.IAAS,
			existing:  map[string]string{"Token": "token", "rbac-id": "target"},
			imported:  map[string]string{"Token": "token"},
			wantError: `credential attribute mismatch for "qa-k8s/fred/foo"`,
		}, {
			name: "other metadata stays strict", modelType: description.CAAS,
			existing:  map[string]string{"Token": "token", "other": "value"},
			imported:  map[string]string{"Token": "token"},
			wantError: `credential attribute mismatch for "qa-k8s/fred/foo"`,
		}, {
			name: "revoked target", modelType: description.CAAS, revoked: true,
			existing:  map[string]string{"Token": "token", "rbac-id": "target"},
			imported:  map[string]string{"Token": "token"},
			wantError: `credential "qa-k8s/fred/foo" is revoked`,
		}, {
			name: "certificate authentication", modelType: description.CAAS,
			authType: cloud.ClientCertificateAuthType,
			existing: map[string]string{"ClientCertificateData": "cert", "ClientKeyData": "key", "rbac-id": "target"},
			imported: map[string]string{"ClientCertificateData": "cert", "ClientKeyData": "key"},
		}, {
			name: "different certificate key", modelType: description.CAAS,
			authType:  cloud.ClientCertificateAuthType,
			existing:  map[string]string{"ClientCertificateData": "cert", "ClientKeyData": "target", "rbac-id": "target"},
			imported:  map[string]string{"ClientCertificateData": "cert", "ClientKeyData": "source"},
			wantError: `credential attribute mismatch for "qa-k8s/fred/foo"`,
		},
	} {
		c.Logf("case: %s", test.name)
		ctrl := s.setupMocks(c)
		if test.authType == "" {
			test.authType = cloud.OAuth2AuthType
		}
		model := description.NewModel(description.ModelArgs{Type: test.modelType})
		model.SetCloudCredential(description.CloudCredentialArgs{
			Owner: "fred", Cloud: "qa-k8s", Name: "foo",
			AuthType: string(test.authType), Attributes: maps.Clone(test.imported),
		})
		cred := cloud.NewNamedCredential("foo", test.authType, test.existing, test.revoked)
		key := credential.Key{Cloud: "qa-k8s", Owner: usertesting.GenNewName(c, "fred"), Name: "foo"}
		s.service.EXPECT().CloudCredential(gomock.Any(), key).Return(cred, nil)
		err := s.newImportOperation().Execute(c.Context(), model)
		if test.wantError == "" {
			c.Assert(err, tc.ErrorIsNil)
		} else {
			c.Check(err, tc.ErrorMatches, regexp.QuoteMeta(test.wantError))
		}
		c.Check(cred.Attributes(), tc.DeepEquals, test.existing)
		c.Check(model.CloudCredential().Attributes(), tc.DeepEquals, test.imported)
		ctrl.Finish()
	}
}
