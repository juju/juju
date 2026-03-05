// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"errors"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/description/v11"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/crossmodelrelation/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type importSecretSuite struct {
	importService *MockImportSecretService
}

func TestImportSecretSuite(t *testing.T) {
	tc.Run(t, &importSecretSuite{})
}

func (s *importSecretSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.importService = NewMockImportSecretService(ctrl)

	c.Cleanup(func() {
		s.importService = nil
	})

	return ctrl
}

func (s *importSecretSuite) newImportOperation(c *tc.C) *importSecretOperation {
	return &importSecretOperation{
		importService: s.importService,
		clock:         clock.WallClock,
		logger:        loggertesting.WrapCheckLog(c),
	}
}

func (s *importSecretSuite) TestImportWithSecrets(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	// Add remote application to make Execute proceed.
	model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "remote-app",
		SourceModelUUID: "source-uuid",
	})

	// Add a secret with a remote grant and a remote consumer.
	secretID := uuid.MustNewUUID().String()
	model.AddSecret(description.SecretArgs{
		ID:    secretID,
		Owner: names.NewUserTag("admin"),
		ACL: map[string]description.SecretAccessArgs{
			"application-remote-app": {
				Scope: "relation-app:endpoint remote-app:endpoint",
				Role:  "view",
			},
		},
		RemoteConsumers: []description.SecretRemoteConsumerArgs{
			{
				ID:              "consumer-id",
				Consumer:        names.NewUnitTag("remote-app/0"),
				CurrentRevision: 1,
			},
		},
	})

	// Add a remote secret.
	remoteSecretID := "remote-secret-id"
	model.AddRemoteSecret(description.RemoteSecretArgs{
		ID:              remoteSecretID,
		SourceUUID:      "source-app-uuid",
		Label:           "remote-label",
		Consumer:        names.NewUnitTag("local-app/0"),
		CurrentRevision: 2,
		LatestRevision:  3,
	})

	relKey, err := relation.NewKeyFromString("app:endpoint remote-app:endpoint")
	c.Assert(err, tc.ErrorIsNil)
	expectedGrantedSecrets := []service.GrantedSecretImport{
		{
			SecretID: secretID,
			ACLs: []service.GrantedSecretACLImport{
				{
					ApplicationName: "remote-app",
					RelationKey:     relKey,
					Role:            secrets.RoleView,
				},
			},
			Consumers: []service.GrantedSecretConsumerImport{
				{
					Unit:            unit.Name("remote-app/0"),
					CurrentRevision: 1,
				},
			},
		},
	}
	s.importService.EXPECT().ImportGrantedSecrets(gomock.Any(), expectedGrantedSecrets).Return(nil)

	expectedRemoteSecrets := []service.RemoteSecretImport{
		{
			SecretID:        remoteSecretID,
			SourceUUID:      "source-app-uuid",
			Label:           "remote-label",
			ConsumerUnit:    unit.Name("local-app/0"),
			CurrentRevision: 2,
			LatestRevision:  3,
		},
	}
	s.importService.EXPECT().ImportRemoteSecrets(gomock.Any(), expectedRemoteSecrets).Return(nil)

	// Act
	op := s.newImportOperation(c)
	err = op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSecretSuite) TestImportWithMultipleSecretsAndGrants(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	// Add remote applications.
	model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "remote-app-a",
		SourceModelUUID: "source-uuid-1",
	})
	model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "remote-app-b",
		SourceModelUUID: "source-uuid-2",
	})

	// Add secret 1 with multiple grants and consumers.
	secretID1 := uuid.MustNewUUID().String()
	model.AddSecret(description.SecretArgs{
		ID:    secretID1,
		Owner: names.NewUserTag("admin"),
		ACL: map[string]description.SecretAccessArgs{
			"application-remote-app-a": {
				Scope: "relation-app:endpoint-1 remote-app-a:endpoint-1",
				Role:  "view",
			},
			"application-remote-app-b": {
				Scope: "relation-app:endpoint-2 remote-app-b:endpoint-2",
				Role:  "manage",
			},
		},
		RemoteConsumers: []description.SecretRemoteConsumerArgs{
			{
				ID:              "consumer-id-1",
				Consumer:        names.NewUnitTag("remote-app-a/0"),
				CurrentRevision: 1,
			},
			{
				ID:              "consumer-id-2",
				Consumer:        names.NewUnitTag("remote-app-b/0"),
				CurrentRevision: 2,
			},
		},
	})

	// Add secret 2 with one grant and one consumer.
	secretID2 := uuid.MustNewUUID().String()
	model.AddSecret(description.SecretArgs{
		ID:    secretID2,
		Owner: names.NewUserTag("admin"),
		ACL: map[string]description.SecretAccessArgs{
			"application-remote-app-a": {
				Scope: "relation-app:endpoint-3 remote-app-a:endpoint-3",
				Role:  "view",
			},
		},
		RemoteConsumers: []description.SecretRemoteConsumerArgs{
			{
				ID:              "consumer-id-3",
				Consumer:        names.NewUnitTag("remote-app-a/1"),
				CurrentRevision: 3,
			},
		},
	})

	// Add multiple remote secrets.
	model.AddRemoteSecret(description.RemoteSecretArgs{
		ID:              "remote-secret-1",
		SourceUUID:      "source-app-uuid-1",
		Label:           "label-1",
		Consumer:        names.NewUnitTag("local-app/0"),
		CurrentRevision: 10,
		LatestRevision:  12,
	})
	model.AddRemoteSecret(description.RemoteSecretArgs{
		ID:              "remote-secret-2",
		SourceUUID:      "source-app-uuid-2",
		Label:           "label-2",
		Consumer:        names.NewUnitTag("local-app/1"),
		CurrentRevision: 20,
		LatestRevision:  22,
	})

	relKey1, err := relation.NewKeyFromString("app:endpoint-1 remote-app-a:endpoint-1")
	c.Assert(err, tc.ErrorIsNil)
	relKey2, err := relation.NewKeyFromString("app:endpoint-2 remote-app-b:endpoint-2")
	c.Assert(err, tc.ErrorIsNil)
	relKey3, err := relation.NewKeyFromString("app:endpoint-3 remote-app-a:endpoint-3")
	c.Assert(err, tc.ErrorIsNil)

	s.importService.EXPECT().ImportGrantedSecrets(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, imports []service.GrantedSecretImport) error {
		c.Assert(imports, tc.HasLen, 2)

		// Map imports for easier checking
		importMap := make(map[string]service.GrantedSecretImport)
		for _, imp := range imports {
			importMap[imp.SecretID] = imp
		}

		// Check secret 1
		imp1, ok := importMap[secretID1]
		c.Assert(ok, tc.IsTrue)
		c.Check(imp1.ACLs, tc.HasLen, 2)

		for _, acl := range imp1.ACLs {
			switch acl.ApplicationName {
			case "remote-app-a":
				c.Check(acl.Role, tc.Equals, secrets.RoleView)
				c.Check(acl.RelationKey, tc.DeepEquals, relKey1)
			case "remote-app-b":
				c.Check(acl.Role, tc.Equals, secrets.RoleManage)
				c.Check(acl.RelationKey, tc.DeepEquals, relKey2)
			default:
				c.Errorf("unexpected application name %q", acl.ApplicationName)
			}
		}

		c.Check(imp1.Consumers, tc.HasLen, 2)
		c.Check(imp1.Consumers, tc.DeepEquals, []service.GrantedSecretConsumerImport{
			{Unit: "remote-app-a/0", CurrentRevision: 1},
			{Unit: "remote-app-b/0", CurrentRevision: 2},
		})

		// Check secret 2
		imp2, ok := importMap[secretID2]
		c.Assert(ok, tc.IsTrue)
		c.Check(imp2.ACLs, tc.HasLen, 1)
		c.Check(imp2.ACLs[0].ApplicationName, tc.Equals, "remote-app-a")
		c.Check(imp2.ACLs[0].Role, tc.Equals, secrets.RoleView)
		c.Check(imp2.ACLs[0].RelationKey, tc.DeepEquals, relKey3)

		c.Check(imp2.Consumers, tc.HasLen, 1)
		c.Check(imp2.Consumers[0], tc.DeepEquals, service.GrantedSecretConsumerImport{
			Unit:            "remote-app-a/1",
			CurrentRevision: 3,
		})

		return nil
	})

	s.importService.EXPECT().ImportRemoteSecrets(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, imports []service.RemoteSecretImport) error {
		c.Assert(imports, tc.HasLen, 2)
		c.Check(imports[0], tc.DeepEquals, service.RemoteSecretImport{
			SecretID:        "remote-secret-1",
			SourceUUID:      "source-app-uuid-1",
			Label:           "label-1",
			ConsumerUnit:    "local-app/0",
			CurrentRevision: 10,
			LatestRevision:  12,
		})
		c.Check(imports[1], tc.DeepEquals, service.RemoteSecretImport{
			SecretID:        "remote-secret-2",
			SourceUUID:      "source-app-uuid-2",
			Label:           "label-2",
			ConsumerUnit:    "local-app/1",
			CurrentRevision: 20,
			LatestRevision:  22,
		})
		return nil
	})

	// Act
	op := s.newImportOperation(c)
	err = op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSecretSuite) TestImportGrantedSecretsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name: "remote-app",
	})
	model.AddSecret(description.SecretArgs{
		ID:    uuid.MustNewUUID().String(),
		Owner: names.NewUserTag("admin"),
	})

	s.importService.EXPECT().ImportGrantedSecrets(gomock.Any(), gomock.Any()).Return(errors.New("boom"))

	// Act
	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.NotNil)
	c.Assert(err.Error(), tc.Contains, "importing remote granted secrets: boom")
}

func (s *importSecretSuite) TestImportRemoteSecretsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name: "remote-app",
	})
	model.AddRemoteSecret(description.RemoteSecretArgs{
		ID:       "remote-secret-id",
		Consumer: names.NewUnitTag("app/0"),
	})

	s.importService.EXPECT().ImportGrantedSecrets(gomock.Any(), gomock.Any()).Return(nil)
	s.importService.EXPECT().ImportRemoteSecrets(gomock.Any(), gomock.Any()).Return(errors.New("boom"))

	// Act
	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.NotNil)
	c.Assert(err.Error(), tc.Contains, "importing remote secrets: boom")
}
