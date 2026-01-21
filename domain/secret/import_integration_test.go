// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/juju/description/v11"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/secrets"
	migrationtesting "github.com/juju/juju/domain/modelmigration/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/secret"
	secretmodelmigration "github.com/juju/juju/domain/secret/modelmigration"
	"github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/domain/secret/state"
	secretbackendstate "github.com/juju/juju/domain/secretbackend/state"
	domaintesting "github.com/juju/juju/domain/testing"
	"github.com/juju/juju/environs/config"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type importSuite struct {
	schematesting.ControllerSuite
	schematesting.ModelSuite
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) SetUpSuite(c *tc.C) {
	s.ControllerSuite.SetUpSuite(c)
	s.ModelSuite.SetUpSuite(c)
}

func (s *importSuite) TearDownSuite(c *tc.C) {
	s.ModelSuite.TearDownSuite(c)
	s.ControllerSuite.TearDownSuite(c)
}

func (s *importSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.ModelSuite.SetUpTest(c)
}

func (s *importSuite) setupService(c *tc.C) *service.SecretService {
	secretState := state.NewState(s.ModelSuite.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	secretBackendState := secretbackendstate.NewState(s.ControllerSuite.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	return service.NewSecretService(
		secretState,
		secretBackendState,
		domaintesting.NoopLeaderEnsurer(),
		loggertesting.WrapCheckLog(c),
	)
}

func (s *importSuite) addSecretBackend(c *tc.C, name, sbType string) {
	tc.Must2_0(c, s.ControllerTxnRunner().StdTxn, c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO secret_backend (uuid, name, backend_type_id)
			VALUES (?, ?,
			        (SELECT id FROM secret_backend_type WHERE type = ?))
		`, tc.Must(c, uuid.NewUUID).String(), name, sbType)
		return err
	})
}

func (s *importSuite) prepareModel(c *tc.C) description.Model {

	tc.Must2_0(c, s.ControllerTxnRunner().StdTxn, c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		cloudUUID := tc.Must(c, uuid.NewUUID).String()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO cloud (uuid, name, cloud_type_id, endpoint, skip_tls_verify)
			VALUES (?,'lxd', 1, 'placeholder', true)`, cloudUUID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, activated, cloud_uuid, model_type_id, life_id, name, qualifier)
			VALUES (?, true, ?, 0, 1, 'test-model', 'integration')
		`, s.ModelUUID(), cloudUUID); err != nil {
			return err
		}
		return nil
	})

	tc.Must2_0(c, s.ModelTxnRunner().StdTxn, c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
			VALUES (?, ?, 'test-model', 'integration', 'iaas', 'lxd', 'fake')
		`, s.ModelUUID(), jujutesting.ControllerTag.Id())
		return err
	})

	// Add a secret backend to handle secrets.
	s.addSecretBackend(c, "internal", "controller")

	return description.NewModel(description.ModelArgs{
		Type:  string(model.IAAS),
		Owner: "admin",
		Cloud: "lxd",
		Config: map[string]any{
			config.NameKey: "test-model",
			config.UUIDKey: s.ModelUUID(),
			config.TypeKey: string(model.IAAS),
		},
	})
}

func (s *importSuite) prepareApplication(c *tc.C, desc description.Model) {
	app := desc.AddApplication(description.ApplicationArgs{
		Name:     "qa",
		CharmURL: "ch:amd64/jammy/qa-0",
	})
	app.SetCharmOrigin(description.CharmOriginArgs{
		Source:   "charm-hub",
		ID:       "qa-id",
		Hash:     "qa-hash",
		Revision: 0,
		Channel:  "latest/stable",
		Platform: "amd64/ubuntu/22.04",
	})
	app.SetCharmMetadata(description.CharmMetadataArgs{
		Name: "qa",
	})
	app.SetCharmManifest(description.CharmManifestArgs{
		Bases: []description.CharmManifestBase{
			migrationtesting.ManifestBase{
				Name_:    "ubuntu",
				Channel_: "22.04/stable",
			},
		},
	})

	tc.Must2_0(c, s.ModelTxnRunner().StdTxn, c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		netNodeUUID := tc.Must(c, uuid.NewUUID).String()
		appUUID := tc.Must(c, uuid.NewUUID).String()
		charmUUID := tc.Must(c, uuid.NewUUID).String()
		unitUUID := tc.Must(c, uuid.NewUUID).String()
		if _, err := tx.ExecContext(ctx, `
		INSERT INTO net_node (uuid)
		VALUES (?)`, netNodeUUID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
		INSERT INTO charm (uuid, reference_name)
		VALUES (?, ?)`, charmUUID, "ch:amd64/jammy/qa-0"); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
		INSERT INTO application (uuid, name, charm_uuid, life_id, space_uuid)
		VALUES (?, ?, ?, ?, ?)`, appUUID, "qa", charmUUID, 0, network.AlphaSpaceId); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
		INSERT INTO unit (uuid, application_uuid, charm_uuid, net_node_uuid, name, life_id)
		VALUES (?, ?, ?, ?, ?, 0)`, unitUUID, appUUID, charmUUID, netNodeUUID, "qa/0"); err != nil {
			return err
		}
		return nil
	})

}

func (s *importSuite) checkSecret(c *tc.C, svc *service.SecretService, args description.SecretArgs, accessor secret.SecretAccessor) {
	uri := &secrets.URI{ID: args.ID}
	md, revisions, err := svc.ListSecrets(c.Context(), uri, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md, tc.HasLen, 1)
	c.Assert(revisions, tc.HasLen, 1)
	c.Assert(revisions[0], tc.HasLen, len(args.Revisions))

	if args.Label != "" {
		c.Check(md[0].Label, tc.Equals, args.Label)
	}
	if args.Description != "" {
		c.Check(md[0].Description, tc.Equals, args.Description)
	}
	c.Check(md[0].Owner.String(), tc.Equals, args.Owner.String())
	c.Check(md[0].LatestRevisionChecksum, tc.Equals, args.LatestRevisionChecksum)
	c.Check(md[0].CreateTime, tc.Equals, args.Created.UTC())
	c.Check(md[0].UpdateTime, tc.Equals, args.Updated.UTC())

	for i, obtained := range revisions[0] {
		revOK := c.Check(obtained.Revision, tc.Equals, args.Revisions[i].Number, tc.Commentf("revision %d", i))
		if revOK {
			value, ref, err := svc.GetSecretValue(c.Context(), uri, obtained.Revision, accessor)
			if ok := c.Check(err, tc.ErrorIsNil, tc.Commentf("revision %d", i)); !ok {
				continue
			}
			c.Check(value.EncodedValues(), tc.DeepEquals, args.Revisions[i].Content, tc.Commentf("revision %d", i))
			c.Check(ref, tc.DeepEquals, obtained.ValueRef, tc.Commentf("revision %d", i))
			c.Check(obtained.CreateTime, tc.DeepEquals, args.Revisions[i].Created, tc.Commentf("revision %d", i))
			c.Check(obtained.UpdateTime, tc.DeepEquals, args.Revisions[i].Updated, tc.Commentf("revision %d", i))
			c.Check(obtained.ExpireTime, tc.DeepEquals, args.Revisions[i].ExpireTime, tc.Commentf("revision %d", i))
		}
	}
}

func (s *importSuite) TestImportModelSecret(c *tc.C) {
	// Arrange
	desc := s.prepareModel(c)

	// Use now at 0h to make it easier to reason about failures.
	now := time.Now().UTC().Truncate(24 * time.Hour)

	secretID := "d5l5an0muugu4fd7q7cg"
	secretArg1 := description.SecretArgs{
		ID:                     secretID,
		Label:                  "model-secret-1",
		Description:            "revision 2 updated info",
		Owner:                  names.NewModelTag(s.ModelUUID()),
		Created:                now.Add(1 * time.Hour),
		Updated:                now.Add(12 * time.Hour), // updated after latest revision
		LatestRevisionChecksum: "checksum-1",
		Revisions: []description.SecretRevisionArgs{
			// Let's say revision 1 has been pruned
			{
				Number:  2,
				Created: now.Add(2 * time.Hour),
				Updated: now.Add(2 * time.Hour),
				Content: map[string]string{"key1": "val1-rev2"},
			},
			{
				Number:  3,
				Created: now.Add(3 * time.Hour),
				Updated: now.Add(3 * time.Hour),
				Content: map[string]string{"key1": "val1-rev3"},
			},
		},
	}
	desc.AddSecret(secretArg1)

	// Act
	s.doImport(c, desc)

	// Assert
	svc := s.setupService(c)
	s.checkSecret(c, svc, secretArg1, secret.SecretAccessor{
		Kind: secret.ModelAccessor,
		ID:   s.ModelUUID(),
	})
}

func (s *importSuite) TestImportApplicationAndUnitSecrets(c *tc.C) {
	// Arrange
	desc := s.prepareModel(c)
	s.prepareApplication(c, desc)

	// Use now at 0h to make it easier to reason about failures.
	now := time.Now().UTC().Truncate(24 * time.Hour)

	// App-owned secret
	appSecretID := "d5l5an8muugu4fd7q7dg"
	appSecretArg := description.SecretArgs{
		ID:                     appSecretID,
		Owner:                  names.NewApplicationTag("qa"),
		Created:                now.Add(1 * time.Hour),
		Updated:                now.Add(6 * time.Hour),
		LatestRevisionChecksum: "checksum-app-2",
		Revisions: []description.SecretRevisionArgs{
			{
				Number:     2,
				Created:    now.Add(2 * time.Hour),
				Updated:    now.Add(3 * time.Hour),
				Content:    map[string]string{"key2": "val2"},
				ExpireTime: ptr(now.Add(24 * time.Hour)),
			},
			{
				Number:     3,
				Created:    now.Add(4 * time.Hour),
				Updated:    now.Add(5 * time.Hour),
				Content:    map[string]string{"key3": "val3"},
				ExpireTime: ptr(now.Add(30 * time.Hour)),
			},
		},
	}
	desc.AddSecret(appSecretArg)

	// Unit-owned secret
	unitSecretID := "d5l5an8muugu4fd7q7e0"
	unitSecretArg := description.SecretArgs{
		ID:                     unitSecretID,
		Owner:                  names.NewUnitTag("qa/0"),
		Created:                now.Add(10 * time.Hour),
		Updated:                now.Add(16 * time.Hour),
		LatestRevisionChecksum: "checksum-unit-5",
		Revisions: []description.SecretRevisionArgs{
			{
				Number:     5,
				Created:    now.Add(12 * time.Hour),
				Updated:    now.Add(13 * time.Hour),
				Content:    map[string]string{"key3": "val3"},
				ExpireTime: ptr(now.Add(36 * time.Hour)),
			},
			{
				Number:     6,
				Created:    now.Add(14 * time.Hour),
				Updated:    now.Add(15 * time.Hour),
				Content:    map[string]string{"key3": "val3"},
				ExpireTime: ptr(now.Add(48 * time.Hour)),
			},
		},
	}
	desc.AddSecret(unitSecretArg)

	// Act
	s.doImport(c, desc)

	// Assert
	svc := s.setupService(c)

	// Check app secret
	s.checkSecret(c, svc, appSecretArg, secret.SecretAccessor{
		Kind: secret.ApplicationAccessor,
		ID:   "qa",
	})

	// Check unit secret
	s.checkSecret(c, svc, unitSecretArg, secret.SecretAccessor{
		Kind: secret.UnitAccessor,
		ID:   "qa/0",
	})
}

func (s *importSuite) TestImportSecretWithGrants(c *tc.C) {
	// Arrange
	desc := s.prepareModel(c)
	s.prepareApplication(c, desc)

	// Use now at 0h to make it easier to reason about failures.
	now := time.Now().UTC().Truncate(24 * time.Hour)

	modelTag := names.NewModelTag(s.ModelUUID())
	secretID := "d5l5an0muugu4fd7q7cg"
	secretArg := description.SecretArgs{
		ID:                     secretID,
		Label:                  "model-secret-1",
		Owner:                  modelTag,
		Created:                now,
		Updated:                now,
		LatestRevisionChecksum: "checksum-1",
		ACL: map[string]description.SecretAccessArgs{
			"application-qa": {
				Scope: modelTag.String(),
				Role:  string(secrets.RoleView),
			},
		},
		Revisions: []description.SecretRevisionArgs{
			{
				Number:  1,
				Created: now,
				Updated: now,
				Content: map[string]string{"key1": "val1"},
			},
		},
	}
	desc.AddSecret(secretArg)

	// Act
	s.doImport(c, desc)

	// Assert
	svc := s.setupService(c)

	// Check secret and revisions
	s.checkSecret(c, svc, secretArg, secret.SecretAccessor{
		Kind: secret.ModelAccessor,
		ID:   s.ModelUUID(),
	})

	// Check grants
	uri := &secrets.URI{ID: secretID}
	grants, err := svc.GetSecretGrants(c.Context(), uri, secrets.RoleView)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(grants, tc.HasLen, 1)

	// Exhaustive check of grant
	c.Check(grants[0].Role, tc.Equals, secrets.RoleView)
	c.Check(grants[0].Subject.Kind, tc.Equals, secret.ApplicationAccessor)
	c.Check(grants[0].Subject.ID, tc.Equals, "qa")
	c.Check(grants[0].Scope.Kind, tc.Equals, secret.ModelAccessScope)
	c.Check(grants[0].Scope.ID, tc.Equals, s.ModelUUID())
}

func (s *importSuite) doImport(c *tc.C, desc description.Model) {
	coordinator := modelmigration.NewCoordinator(loggertesting.WrapCheckLog(c))
	secretmodelmigration.RegisterImport(coordinator, loggertesting.WrapCheckLog(c))

	err := coordinator.Perform(c.Context(), modelmigration.NewScope(s.ControllerSuite.TxnRunnerFactory(),
		s.ModelSuite.TxnRunnerFactory(), nil,
		model.UUID(s.ModelUUID())), desc)
	c.Assert(err, tc.ErrorIsNil)
}
