// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"

	corecredential "github.com/juju/juju/core/credential"
	usertesting "github.com/juju/juju/core/user/testing"
	accessstate "github.com/juju/juju/domain/access/state"
	"github.com/juju/juju/domain/cloudimagemetadata"
	"github.com/juju/juju/domain/credential"
	credentialstate "github.com/juju/juju/domain/credential/state"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationinternal "github.com/juju/juju/domain/modelmigration/internal"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

// TestGetControllerModelInfoIdentity verifies that the model bootstrap
// identity, credential, model namespace, the seeded admin model permission and
// the model secret backend are read back in target-portable form for a model
// created by the test fixture.
func (s *stateSuite) TestGetControllerModelInfoIdentity(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	info, err := st.GetControllerModelInfo(c.Context(), s.modelUUID.String(), nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(info.ModelInfo.UUID, tc.Equals, s.modelUUID.String())
	c.Check(info.ModelInfo.Name, tc.Equals, "my-test-model")
	c.Check(info.ModelInfo.Qualifier, tc.Equals, "prod")
	c.Check(info.ModelInfo.Type, tc.Equals, "iaas")
	c.Check(info.ModelInfo.Cloud, tc.Equals, "my-cloud")
	c.Check(info.ModelInfo.CloudRegion, tc.Equals, "my-region")
	c.Check(info.ModelInfo.CredentialName, tc.Equals, "foobar")
	c.Check(info.ModelInfo.CredentialOwner, tc.Equals, "test-user")
	c.Check(info.ModelInfo.Life, tc.Equals, "alive")

	c.Check(info.ModelNamespace, tc.Not(tc.Equals), "")

	// The model was created with an admin user; that permission must travel.
	var foundAdmin bool
	for _, p := range info.Permissions {
		if p.ObjectType == "model" && p.GrantOn == s.modelUUID.String() &&
			p.SubjectName == "test-user" && p.Access == "admin" {
			foundAdmin = true
		}
	}
	c.Check(foundAdmin, tc.IsTrue, tc.Commentf("expected model admin permission, got %#v", info.Permissions))

	// The permission principal is recreatable as a model user.
	var foundUser bool
	for _, u := range info.Users {
		if u.Name == "test-user" {
			foundUser = true
		}
	}
	c.Check(foundUser, tc.IsTrue, tc.Commentf("expected test-user in users, got %#v", info.Users))

	c.Assert(info.ModelCredential, tc.NotNil)
	c.Check(info.ModelCredential.Cloud, tc.Equals, "my-cloud")
	c.Check(info.ModelCredential.Owner, tc.Equals, "test-user")
	c.Check(info.ModelCredential.Name, tc.Equals, "foobar")
	c.Check(info.ModelCredential.AuthType, tc.Equals, "access-key")
	c.Check(info.ModelCredential.Attributes, tc.DeepEquals, map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	})

	c.Assert(info.ModelCredential, tc.NotNil)
	// The fixture creates the model with the juju (internal) secret backend.
	c.Assert(info.SecretBackend, tc.NotNil)
	c.Check(info.SecretBackend.Name, tc.Not(tc.Equals), "")
}

// TestGetControllerModelInfoIncludesCredentialOwner verifies the user profile
// set includes the model credential owner even when that user has no model or
// offer permission grant.
func (s *stateSuite) TestGetControllerModelInfoIncludesCredentialOwner(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	db := s.DB()

	ownerName := usertesting.GenNewName(c, "credential-owner")
	ownerUUID := usertesting.GenUserUUID(c)
	accessState := accessstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	err := accessState.AddUser(
		c.Context(),
		ownerUUID,
		ownerName,
		ownerName.Name(),
		false,
		s.userUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	credSt := credentialstate.NewState(s.TxnRunnerFactory())
	err = credSt.UpsertCloudCredential(
		c.Context(), corecredential.Key{
			Cloud: "my-cloud",
			Owner: ownerName,
			Name:  "owner-only",
		},
		credential.CloudCredentialInfo{
			Label:    "owner-only",
			AuthType: "access-key",
			Attributes: map[string]string{
				"foo": "bar",
			},
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	var credUUID string
	err = db.QueryRowContext(c.Context(),
		`SELECT uuid FROM cloud_credential WHERE owner_uuid = ? AND name = ? AND cloud_uuid = ?`,
		ownerUUID, "owner-only", s.cloudUUID).Scan(&credUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(),
		`UPDATE model SET cloud_credential_uuid = ? WHERE uuid = ?`,
		credUUID, s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	info, err := st.GetControllerModelInfo(c.Context(), s.modelUUID.String(), nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	var foundOwner bool
	for _, u := range info.Users {
		if u.Name == ownerName.String() {
			foundOwner = true
		}
	}
	c.Check(foundOwner, tc.IsTrue, tc.Commentf("expected credential owner in users, got %#v", info.Users))
	c.Assert(info.ModelCredential, tc.NotNil)
	c.Check(info.ModelCredential.Owner, tc.Equals, ownerName.String())
}

// TestGetControllerModelInfoIncludesModelQualifierUser verifies the user
// profile set includes the model qualifier when it exists as an active user.
func (s *stateSuite) TestGetControllerModelInfoIncludesModelQualifierUser(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	db := s.DB()

	ownerName := usertesting.GenNewName(c, "prod")
	ownerUUID := usertesting.GenUserUUID(c)
	accessState := accessstate.NewState(
		s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c),
	)
	err := accessState.AddUser(
		c.Context(),
		ownerUUID,
		ownerName,
		ownerName.Name(),
		false,
		s.userUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	info, err := st.GetControllerModelInfo(c.Context(), s.modelUUID.String(), nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	var foundQualifier bool
	for _, u := range info.Users {
		if u.Name == ownerName.String() {
			foundQualifier = true
		}
	}
	c.Check(foundQualifier, tc.IsTrue, tc.Commentf(
		"expected qualifier user in users, got %#v", info.Users,
	))

	_, err = db.ExecContext(c.Context(),
		`UPDATE user SET removed = TRUE WHERE uuid = ?`,
		ownerUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	info, err = st.GetControllerModelInfo(c.Context(), s.modelUUID.String(), nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	for _, u := range info.Users {
		c.Check(u.Name, tc.Not(tc.Equals), ownerName.String())
	}
}

// TestGetControllerModelInfoFacts inserts a representative row for each
// remaining controller-scoped fact and verifies they are all read back,
// including offer-scoped permissions and third-party external controllers
// selected from the caller-supplied inputs.
func (s *stateSuite) TestGetControllerModelInfoFacts(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	db := s.DB()
	modelUUID := s.modelUUID.String()
	userUUID := s.userUUID.String()

	exec := func(query string, args ...any) {
		_, err := db.ExecContext(c.Context(), query, args...)
		c.Assert(err, tc.ErrorIsNil)
	}

	// Offer permission: consume on an offer hosted by this model.
	offerUUID := uuid.MustNewUUID().String()
	exec(`INSERT INTO permission (uuid, access_type_id, object_type_id, grant_on, grant_to)
	      VALUES (?, 2, 3, ?, ?)`, uuid.MustNewUUID().String(), offerUUID, userUUID)

	exec(`UPDATE cloud_credential SET invalid = TRUE, invalid_reason = 'expired'
	      WHERE uuid = ?`, s.credentialUUID.String())

	// Authorized key: requires user_authentication (for the view) + a public key.
	exec(`INSERT OR IGNORE INTO user_authentication (user_uuid, disabled) VALUES (?, FALSE)`, userUUID)
	var keyID int64
	err := db.QueryRowContext(c.Context(),
		`INSERT INTO user_public_ssh_key (comment, fingerprint_hash_algorithm_id, fingerprint, public_key, user_uuid)
		 VALUES ('comment', 1, 'fp', 'ssh-ed25519 AAAAkey', ?) RETURNING id`, userUUID).Scan(&keyID)
	c.Assert(err, tc.ErrorIsNil)
	exec(`INSERT INTO model_authorized_keys (model_uuid, user_public_ssh_key_id) VALUES (?, ?)`, modelUUID, keyID)

	// Lease + lease pin.
	leaseUUID := uuid.MustNewUUID().String()
	start := time.Now().UTC().Truncate(time.Second)
	expiry := start.Add(time.Hour)
	exec(`INSERT INTO lease (uuid, lease_type_id, model_uuid, name, holder, start, expiry)
	      VALUES (?, 1, ?, 'app', 'app/0', ?, ?)`, leaseUUID, modelUUID, start, expiry)
	exec(`INSERT INTO lease_pin (uuid, lease_uuid, entity_id) VALUES (?, ?, 'machine-0')`,
		uuid.MustNewUUID().String(), leaseUUID)

	// Secret backend reference for the model, against the internal backend.
	var backendUUID string
	err = db.QueryRowContext(c.Context(),
		`SELECT secret_backend_uuid FROM model_secret_backend WHERE model_uuid = ?`, modelUUID).Scan(&backendUUID)
	c.Assert(err, tc.ErrorIsNil)
	var backendName string
	err = db.QueryRowContext(c.Context(),
		`SELECT name FROM secret_backend WHERE uuid = ?`, backendUUID).Scan(&backendName)
	c.Assert(err, tc.ErrorIsNil)
	revUUID := uuid.MustNewUUID().String()
	secretID := "secret:" + uuid.MustNewUUID().String()
	exec(`INSERT INTO secret_backend_reference (secret_backend_uuid, model_uuid, secret_revision_uuid, secret_id)
	      VALUES (?, ?, ?, ?)`, backendUUID, modelUUID, revUUID, secretID)

	// Last login.
	loginTime := start.Add(-time.Hour)
	exec(`INSERT INTO model_last_login (model_uuid, user_uuid, time) VALUES (?, ?, ?)`,
		modelUUID, userUUID, loginTime)

	// Custom cloud image metadata is controller-global, but must be carried so
	// the target controller can recreate user-defined image selection data.
	rootStorageSize := uint64(8192)
	createdAt := start.Add(-2 * time.Hour)
	exec(`INSERT INTO cloud_image_metadata
	      (uuid, created_at, source, stream, region, version, architecture_id,
	       virt_type, root_storage_type, root_storage_size, priority, image_id)
	      VALUES (?, ?, ?, 'released', 'us-east-1', '22.04', 0, 'hvm', 'ebs',
	              ?, 10, 'ami-custom')`,
		uuid.MustNewUUID().String(), createdAt,
		cloudimagemetadata.CustomSource, rootStorageSize)
	exec(`INSERT INTO cloud_image_metadata
	      (uuid, created_at, source, stream, region, version, architecture_id,
	       virt_type, root_storage_type, priority, image_id)
	      VALUES (?, ?, 'simplestreams', 'released', 'us-east-1', '22.04', 0,
	              'hvm', 'ebs', 20, 'ami-cached')`,
		uuid.MustNewUUID().String(), createdAt)

	// Third-party external controller with an address and a consumed model.
	extCtrlUUID := uuid.MustNewUUID().String()
	consumedModelUUID := uuid.MustNewUUID().String()
	exec(`INSERT INTO external_controller (uuid, alias, ca_cert) VALUES (?, 'other-ctrl', 'CACERT')`, extCtrlUUID)
	exec(`INSERT INTO external_controller_address (uuid, controller_uuid, address) VALUES (?, ?, '1.2.3.4:17070')`,
		uuid.MustNewUUID().String(), extCtrlUUID)
	exec(`INSERT INTO external_model (uuid, controller_uuid) VALUES (?, ?)`,
		consumedModelUUID, extCtrlUUID)

	offererModels := []modelmigrationinternal.OffererModel{
		{ControllerUUID: extCtrlUUID, ModelUUID: consumedModelUUID},
	}

	info, err := st.GetControllerModelInfo(c.Context(), modelUUID, []string{offerUUID}, offererModels)
	c.Assert(err, tc.ErrorIsNil)

	// Offer permission present alongside the model admin permission.
	var foundOffer bool
	for _, p := range info.Permissions {
		if p.ObjectType == "offer" && p.GrantOn == offerUUID &&
			p.SubjectName == "test-user" && p.Access == "consume" {
			foundOffer = true
		}
	}
	c.Check(foundOffer, tc.IsTrue, tc.Commentf("expected offer permission, got %#v", info.Permissions))

	c.Check(info.AuthorizedKeys, tc.DeepEquals, []modelmigration.ModelAuthorizedKey{
		{Username: "test-user", PublicKey: "ssh-ed25519 AAAAkey"},
	})

	c.Assert(info.Leases, tc.HasLen, 1)
	c.Check(info.Leases[0].Type, tc.Equals, "application-leadership")
	c.Check(info.Leases[0].Name, tc.Equals, "app")
	c.Check(info.Leases[0].Holder, tc.Equals, "app/0")

	c.Check(info.LeasePins, tc.DeepEquals, []modelmigration.LeasePin{
		{LeaseType: "application-leadership", LeaseName: "app", EntityID: "machine-0"},
	})

	c.Check(info.SecretBackendRefs, tc.DeepEquals, []modelmigration.SecretBackendReference{
		{BackendName: backendName, SecretRevisionUUID: revUUID, SecretID: secretID},
	})

	c.Assert(info.ModelCredential, tc.NotNil)
	c.Check(info.ModelCredential.Invalid, tc.IsTrue)
	c.Check(info.ModelCredential.InvalidReason, tc.Equals, "expired")

	c.Assert(info.LastLogins, tc.HasLen, 1)
	c.Check(info.LastLogins[0].Username, tc.Equals, "test-user")

	c.Assert(info.CloudImageMetadata, tc.HasLen, 1)
	c.Check(info.CloudImageMetadata[0], tc.DeepEquals, modelmigration.CloudImageMetadata{
		Stream:          "released",
		Region:          "us-east-1",
		Version:         "22.04",
		Arch:            "amd64",
		VirtType:        "hvm",
		RootStorageType: "ebs",
		RootStorageSize: &rootStorageSize,
		Source:          cloudimagemetadata.CustomSource,
		Priority:        10,
		ImageID:         "ami-custom",
		CreatedAt:       createdAt,
	})

	c.Assert(info.ExternalControllers, tc.HasLen, 1)
	ec := info.ExternalControllers[0]
	c.Check(ec.UUID, tc.Equals, extCtrlUUID)
	c.Check(ec.Alias, tc.Equals, "other-ctrl")
	c.Check(ec.CACert, tc.Equals, "CACERT")
	c.Check(ec.Addresses, tc.DeepEquals, []string{"1.2.3.4:17070"})
	c.Check(ec.ConsumedModels, tc.DeepEquals, []string{consumedModelUUID})
}

// TestGetControllerModelInfoExternalModelMissing verifies model DB offerer
// selectors must be backed by source controller DB external_model rows.
func (s *stateSuite) TestGetControllerModelInfoExternalModelMissing(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	db := s.DB()

	extCtrlUUID := uuid.MustNewUUID().String()
	consumedModelUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		`INSERT INTO external_controller (uuid, alias, ca_cert) VALUES (?, 'other-ctrl', 'CACERT')`,
		extCtrlUUID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.GetControllerModelInfo(
		c.Context(),
		s.modelUUID.String(),
		nil,
		[]modelmigrationinternal.OffererModel{{
			ControllerUUID: extCtrlUUID,
			ModelUUID:      consumedModelUUID,
		}},
	)
	c.Assert(err, tc.ErrorMatches,
		`external model "`+consumedModelUUID+`" for controller "`+extCtrlUUID+`" not found`)
}

// TestGetControllerModelInfoExternalControllerMissing verifies model DB offerer
// selectors must be backed by source controller DB external_controller rows.
func (s *stateSuite) TestGetControllerModelInfoExternalControllerMissing(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	extCtrlUUID := uuid.MustNewUUID().String()
	consumedModelUUID := uuid.MustNewUUID().String()
	_, err := st.GetControllerModelInfo(
		c.Context(),
		s.modelUUID.String(),
		nil,
		[]modelmigrationinternal.OffererModel{{
			ControllerUUID: extCtrlUUID,
			ModelUUID:      consumedModelUUID,
		}},
	)
	c.Assert(err, tc.ErrorMatches,
		`external controller "`+extCtrlUUID+`" for offerer model "`+consumedModelUUID+`" not found`)
}

// TestGetControllerModelInfoModelNotFound verifies a clear error for an unknown
// model UUID.
func (s *stateSuite) TestGetControllerModelInfoModelNotFound(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	_, err := st.GetControllerModelInfo(c.Context(), uuid.MustNewUUID().String(), nil, nil)
	c.Assert(err, tc.ErrorMatches, `.*not found.*`)
}
