// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	coresecrets "github.com/juju/juju/core/secrets"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/schema/testing"
	domainsecret "github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	testing.ModelSuite

	modelUUID string
}

func TestStateSuite(t *stdtesting.T) {
	tc.Run(t, &stateSuite{})
}

func newSecretState(c *tc.C, factory coredatabase.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    loggertesting.WrapCheckLog(c),
	}
}

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.modelUUID = s.setupModel(c)
}

func (s *stateSuite) setupModel(c *tc.C) string {
	modelUUID := uuid.MustNewUUID()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO model (uuid, controller_uuid, name, type, cloud, cloud_type)
VALUES (?, ?, "test", "iaas", "fluffy", "ec2")
		`, modelUUID.String(), coretesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return modelUUID.String()
}

func (s *stateSuite) TestGetModelUUID(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	got, err := st.GetModelUUID(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got.String(), tc.Equals, s.modelUUID)
}

func (s *stateSuite) TestGetSecretNotFound(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	_, err := st.GetSecret(c.Context(), coresecrets.NewURI())
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestCheckApplicationSecretLabelExistsAlreadyUsedByApp(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmApplicationSecret(ctx, st, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	appUUID, err := getApplicationUUID(ctx, st, "mysql")
	c.Assert(err, tc.ErrorIsNil)

	exists, err := checkApplicationSecretLabelExists(ctx, st, appUUID, "my label")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exists, tc.IsTrue)
}

func (s *stateSuite) TestCheckApplicationSecretLabelExistsAlreadyUsedByUnit(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmUnitSecret(ctx, st, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	appUUID, err := getApplicationUUID(ctx, st, "mysql")
	c.Assert(err, tc.ErrorIsNil)

	exists, err := checkApplicationSecretLabelExists(ctx, st, appUUID, "my label")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exists, tc.IsTrue)
}

func (s *stateSuite) TestCheckUnitSecretLabelExistsAlreadyUsedByUnit(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmUnitSecret(ctx, st, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	unitUUID0, err := getUnitUUID(ctx, st, "mysql/0")
	c.Assert(err, tc.ErrorIsNil)

	unitUUID1, err := getUnitUUID(ctx, st, "mysql/1")
	c.Assert(err, tc.ErrorIsNil)

	exists, err := checkUnitSecretLabelExists(ctx, st, unitUUID0, "my label")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exists, tc.IsTrue)
	exists, err = checkUnitSecretLabelExists(ctx, st, unitUUID1, "my label")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exists, tc.IsTrue)
}

func (s *stateSuite) TestCheckUnitSecretLabelExistsAlreadyUsedByApp(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		Checksum:    "checksum-1234",
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmApplicationSecret(ctx, st, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	unitUUID0, err := getUnitUUID(ctx, st, "mysql/0")
	c.Assert(err, tc.ErrorIsNil)

	unitUUID1, err := getUnitUUID(ctx, st, "mysql/1")
	c.Assert(err, tc.ErrorIsNil)

	exists, err := checkUnitSecretLabelExists(ctx, st, unitUUID0, "my label")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exists, tc.IsTrue)
	exists, err = checkUnitSecretLabelExists(ctx, st, unitUUID1, "my label")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exists, tc.IsTrue)
}

func (s *stateSuite) TestCheckUserSecretLabelExists(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   ptr(true),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	exists, err := checkUserSecretLabelExists(ctx, st, "my label")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exists, tc.IsTrue)
}

func (s *stateSuite) TestGetLatestRevisionNotFound(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	_, err := st.GetLatestRevision(c.Context(), coresecrets.NewURI())
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGetLatestRevision(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)
	err = updateSecret(ctx, st, uri, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar1"},
	})
	c.Assert(err, tc.ErrorIsNil)
	latest, err := st.GetLatestRevision(ctx, uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(latest, tc.Equals, 2)
}

func (s *stateSuite) TestGetRotatePolicy(c *tc.C) {
	s.setupUnits(c, "mysql")

	st := newSecretState(c, s.TxnRunnerFactory())

	expireTime := time.Now().Add(2 * time.Hour)
	rotateTime := time.Now().Add(time.Hour)
	sp := domainsecret.UpsertSecretParams{
		Description:    ptr("my secretMetadata"),
		Label:          ptr("my label"),
		Data:           coresecrets.SecretData{"foo": "bar"},
		RotatePolicy:   ptr(domainsecret.RotateYearly),
		ExpireTime:     ptr(expireTime),
		NextRotateTime: ptr(rotateTime),
		RevisionID:     ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmApplicationSecret(ctx, st, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	result, err := st.GetRotatePolicy(c.Context(), uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, coresecrets.RotateYearly)
}

func (s *stateSuite) TestGetRotatePolicyNotFound(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	_, err := st.GetRotatePolicy(c.Context(), coresecrets.NewURI())
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGetRotationExpiryInfo(c *tc.C) {
	s.setupUnits(c, "mysql")

	st := newSecretState(c, s.TxnRunnerFactory())

	expireTime := time.Now().Add(2 * time.Hour)
	rotateTime := time.Now().Add(time.Hour)
	sp := domainsecret.UpsertSecretParams{
		Description:    ptr("my secretMetadata"),
		Label:          ptr("my label"),
		Data:           coresecrets.SecretData{"foo": "bar"},
		RotatePolicy:   ptr(domainsecret.RotateYearly),
		ExpireTime:     ptr(expireTime),
		NextRotateTime: ptr(rotateTime),
		RevisionID:     ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmApplicationSecret(ctx, st, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	result, err := st.GetRotationExpiryInfo(c.Context(), uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, &domainsecret.RotationExpiryInfo{
		RotatePolicy:     coresecrets.RotateYearly,
		LatestExpireTime: ptr(expireTime.UTC()),
		NextRotateTime:   ptr(rotateTime.UTC()),
		LatestRevision:   1,
	})

	newExpireTime := expireTime.Add(2 * time.Hour)
	err = updateSecret(ctx, st, uri, domainsecret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo": "bar1"},
		ExpireTime: ptr(newExpireTime),
		RevisionID: ptr(uuid.MustNewUUID().String()),
	})
	c.Assert(err, tc.ErrorIsNil)

	result, err = st.GetRotationExpiryInfo(c.Context(), uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, &domainsecret.RotationExpiryInfo{
		RotatePolicy:     coresecrets.RotateYearly,
		LatestExpireTime: ptr(newExpireTime.UTC()),
		NextRotateTime:   ptr(rotateTime.UTC()),
		LatestRevision:   2,
	})
}

func (s *stateSuite) TestGetRotationExpiryInfoNotFound(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	_, err := st.GetRotationExpiryInfo(c.Context(), coresecrets.NewURI())
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGetSecretRevisionNotFound(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	_, _, err := st.GetSecretValue(c.Context(), coresecrets.NewURI(), 666)
	c.Assert(err, tc.ErrorIs, secreterrors.SecretRevisionNotFound)
}

func (s *stateSuite) TestCreateUserSecretFailedRevisionIDMissing(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   ptr(true),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorMatches, `*.revision ID must be provided`)
}

func fromDbRotatePolicy(p domainsecret.RotatePolicy) coresecrets.RotatePolicy {
	switch p {
	case domainsecret.RotateHourly:
		return coresecrets.RotateHourly
	case domainsecret.RotateDaily:
		return coresecrets.RotateDaily
	case domainsecret.RotateWeekly:
		return coresecrets.RotateWeekly
	case domainsecret.RotateMonthly:
		return coresecrets.RotateMonthly
	case domainsecret.RotateQuarterly:
		return coresecrets.RotateQuarterly
	case domainsecret.RotateYearly:
		return coresecrets.RotateYearly
	}
	return coresecrets.RotateNever
}

func value[T any](v *T) T {
	if v == nil {
		return *new(T)
	}
	return *v
}

func (s *stateSuite) assertSecret(c *tc.C, st *State, uri *coresecrets.URI, sp domainsecret.UpsertSecretParams, revision int, owner coresecrets.Owner) {
	ctx := c.Context()
	md, revs, err := st.ListSecrets(ctx, uri, &revision, domainsecret.NilLabels)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md, tc.HasLen, 1)
	c.Assert(md[0].Version, tc.Equals, 1)
	c.Assert(md[0].Label, tc.Equals, value(sp.Label))
	c.Assert(md[0].Description, tc.Equals, value(sp.Description))
	c.Assert(md[0].LatestRevision, tc.Equals, 1)
	c.Assert(md[0].AutoPrune, tc.Equals, value(sp.AutoPrune))
	c.Assert(md[0].Owner, tc.DeepEquals, owner)
	if sp.RotatePolicy == nil {
		c.Assert(md[0].RotatePolicy, tc.Equals, coresecrets.RotateNever)
	} else {
		c.Assert(md[0].RotatePolicy, tc.Equals, fromDbRotatePolicy(*sp.RotatePolicy))
	}
	if sp.NextRotateTime == nil {
		c.Assert(md[0].NextRotateTime, tc.IsNil)
	} else {
		c.Assert(*md[0].NextRotateTime, tc.Equals, sp.NextRotateTime.UTC())
	}
	now := time.Now()
	c.Assert(md[0].CreateTime, tc.Almost, now)
	c.Assert(md[0].UpdateTime, tc.Almost, now)

	c.Assert(revs, tc.HasLen, 1)
	c.Assert(revs[0], tc.HasLen, 1)
	rev := revs[0][0]
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rev.Revision, tc.Equals, revision)
	c.Assert(rev.CreateTime, tc.Almost, now)
	if rev.ExpireTime == nil {
		c.Assert(md[0].LatestExpireTime, tc.IsNil)
	} else {
		c.Assert(*md[0].LatestExpireTime, tc.Equals, rev.ExpireTime.UTC())
	}
}

func (s *stateSuite) TestCreateUserSecretWithContent(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		Checksum:    "checksum-1234",
		AutoPrune:   ptr(true),
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)
	owner := coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: s.modelUUID}
	s.assertSecret(c, st, uri, sp, 1, owner)
	data, ref, err := st.GetSecretValue(ctx, uri, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ref, tc.IsNil)
	c.Assert(data, tc.DeepEquals, coresecrets.SecretData{"foo": "bar"})

	ap := domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectModel,
		SubjectID:     s.modelUUID,
	}
	access, err := st.GetSecretAccess(ctx, uri, ap)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.Equals, "manage")
}

func (s *stateSuite) TestCreateManyUserSecretsNoLabelClash(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	createAndCheck := func(label string) {
		content := label
		if content == "" {
			content = "empty"
		}
		sp := domainsecret.UpsertSecretParams{
			Description: ptr("my secretMetadata"),
			Label:       ptr(label),
			Data:        coresecrets.SecretData{"foo": content},
			AutoPrune:   ptr(true),
			RevisionID:  ptr(uuid.MustNewUUID().String()),
		}
		uri := coresecrets.NewURI()
		ctx := c.Context()
		err := createUserSecret(ctx, st, 1, uri, sp)
		c.Assert(err, tc.ErrorIsNil)
		owner := coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: s.modelUUID}
		s.assertSecret(c, st, uri, sp, 1, owner)
		data, ref, err := st.GetSecretValue(ctx, uri, 1)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(ref, tc.IsNil)
		c.Assert(data, tc.DeepEquals, coresecrets.SecretData{"foo": content})
	}
	createAndCheck("my label")
	createAndCheck("")
	createAndCheck("")
	createAndCheck("another label")
}

func (s *stateSuite) TestCreateUserSecretWithValueReference(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		ValueRef:    &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		Checksum:    "checksum-1234",
		AutoPrune:   ptr(true),
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)
	owner := coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: s.modelUUID}
	s.assertSecret(c, st, uri, sp, 1, owner)
	data, ref, err := st.GetSecretValue(ctx, uri, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(data, tc.HasLen, 0)
	c.Assert(ref, tc.DeepEquals, &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"})
}

func (s *stateSuite) createOwnedSecrets(c *tc.C) (appSecretURI *coresecrets.URI, unitSecretURI *coresecrets.URI) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")
	s.setupUnits(c, "mariadb")

	ctx := c.Context()
	uri1 := coresecrets.NewURI()
	sp := domainsecret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	err := createCharmApplicationSecret(ctx, st, 1, uri1, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	uri2 := coresecrets.NewURI()
	sp2 := domainsecret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	err = createCharmUnitSecret(ctx, st, 1, uri2, "mysql/1", sp2)
	c.Assert(err, tc.ErrorIsNil)
	return uri1, uri2
}

func (s *stateSuite) TestGetSecretsForAppOwners(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	uri1, _ := s.createOwnedSecrets(c)
	var gotURIs []*coresecrets.URI
	err := st.RunAtomic(c.Context(), func(ctx domain.AtomicContext) error {
		var err error
		gotURIs, err = st.GetSecretsForOwners(ctx, domainsecret.ApplicationOwners{"mysql"}, domainsecret.NilUnitOwners)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotURIs, tc.SameContents, []*coresecrets.URI{uri1})
}

func (s *stateSuite) TestGetSecretsForUnitOwners(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	_, uri2 := s.createOwnedSecrets(c)
	var gotURIs []*coresecrets.URI
	err := st.RunAtomic(c.Context(), func(ctx domain.AtomicContext) error {
		var err error
		gotURIs, err = st.GetSecretsForOwners(ctx, domainsecret.NilApplicationOwners, domainsecret.UnitOwners{"mysql/1"})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotURIs, tc.SameContents, []*coresecrets.URI{uri2})
}

func (s *stateSuite) TestGetSecretsNone(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.createOwnedSecrets(c)
	var gotURIs []*coresecrets.URI
	err := st.RunAtomic(c.Context(), func(ctx domain.AtomicContext) error {
		var err error
		gotURIs, err = st.GetSecretsForOwners(ctx, domainsecret.ApplicationOwners{"mariadb"}, domainsecret.UnitOwners{"mariadb/1"})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotURIs, tc.HasLen, 0)
}

func (s *stateSuite) TestListSecretsNone(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	ctx := c.Context()
	secrets, revisions, err := st.ListSecrets(
		ctx, domainsecret.NilSecretURI, domainsecret.NilRevision, domainsecret.NilLabels)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(secrets), tc.Equals, 0)
	c.Assert(len(revisions), tc.Equals, 0)
}

func (s *stateSuite) TestListSecrets(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := []domainsecret.UpsertSecretParams{{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		Checksum:    "checksum-1234",
		AutoPrune:   ptr(true),
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}, {
		Description: ptr("my secretMetadata2"),
		Label:       ptr("my label2"),
		Data:        coresecrets.SecretData{"foo": "bar2"},
		Checksum:    "checksum-1234",
		AutoPrune:   ptr(true),
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}}
	uri := []*coresecrets.URI{
		coresecrets.NewURI(),
		coresecrets.NewURI(),
	}

	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri[0], sp[0])
	c.Assert(err, tc.ErrorIsNil)
	err = createUserSecret(ctx, st, 1, uri[1], sp[1])
	c.Assert(err, tc.ErrorIsNil)

	secrets, revisions, err := st.ListSecrets(
		ctx, domainsecret.NilSecretURI, domainsecret.NilRevision, domainsecret.NilLabels)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(secrets), tc.Equals, 2)
	c.Assert(len(revisions), tc.Equals, 2)

	for i, md := range secrets {
		c.Assert(md.Version, tc.Equals, 1)
		c.Assert(md.LatestRevisionChecksum, tc.Equals, sp[i].Checksum)
		c.Assert(md.Label, tc.Equals, value(sp[i].Label))
		c.Assert(md.Description, tc.Equals, value(sp[i].Description))
		c.Assert(md.LatestRevision, tc.Equals, 1)
		c.Assert(md.AutoPrune, tc.Equals, value(sp[i].AutoPrune))
		c.Assert(md.Owner, tc.DeepEquals, coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: s.modelUUID})
		now := time.Now()
		c.Assert(md.CreateTime, tc.Almost, now)
		c.Assert(md.UpdateTime, tc.Almost, now)

		revs := revisions[i]
		c.Assert(revs, tc.HasLen, 1)
		c.Assert(revs[0].Revision, tc.Equals, 1)
		c.Assert(revs[0].CreateTime, tc.Almost, now)
	}
}

func (s *stateSuite) TestListSecretsByURI(c *tc.C) {

	st := newSecretState(c, s.TxnRunnerFactory())

	sp := []domainsecret.UpsertSecretParams{{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   ptr(true),
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}, {
		Description: ptr("my secretMetadata2"),
		Label:       ptr("my label2"),
		Data:        coresecrets.SecretData{"foo": "bar2"},
		AutoPrune:   ptr(true),
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}}
	uri := []*coresecrets.URI{
		coresecrets.NewURI(),
		coresecrets.NewURI(),
	}

	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri[0], sp[0])
	c.Assert(err, tc.ErrorIsNil)
	err = createUserSecret(ctx, st, 1, uri[1], sp[1])
	c.Assert(err, tc.ErrorIsNil)

	secrets, revisions, err := st.ListSecrets(
		ctx, uri[0], domainsecret.NilRevision, domainsecret.NilLabels)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(secrets), tc.Equals, 1)
	c.Assert(len(revisions), tc.Equals, 1)

	md := secrets[0]
	c.Assert(md.Version, tc.Equals, 1)
	c.Assert(md.Label, tc.Equals, value(sp[0].Label))
	c.Assert(md.Description, tc.Equals, value(sp[0].Description))
	c.Assert(md.LatestRevision, tc.Equals, 1)
	c.Assert(md.AutoPrune, tc.Equals, value(sp[0].AutoPrune))
	c.Assert(md.Owner, tc.DeepEquals, coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: s.modelUUID})
	now := time.Now()
	c.Assert(md.CreateTime, tc.Almost, now)
	c.Assert(md.UpdateTime, tc.Almost, now)

	revs := revisions[0]
	c.Assert(revs, tc.HasLen, 1)
	c.Assert(revs[0].Revision, tc.Equals, 1)
	c.Assert(revs[0].CreateTime, tc.Almost, now)
}

func (s *stateSuite) setupUnits(c *tc.C, appName string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		charmUUID := uuid.MustNewUUID().String()
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, reference_name, architecture_id)
VALUES (?, ?, 0);
`, charmUUID, appName)
		if err != nil {
			return errors.Capture(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_metadata (charm_uuid, name)
VALUES (?, ?);
		`, charmUUID, appName)
		if err != nil {
			return errors.Capture(err)
		}

		applicationUUID := uuid.MustNewUUID().String()
		_, err = tx.ExecContext(ctx, `
INSERT INTO application (uuid, charm_uuid, name, life_id, space_uuid)
VALUES (?, ?, ?, ?, ?)
`, applicationUUID, charmUUID, appName, life.Alive, network.AlphaSpaceId)
		if err != nil {
			return errors.Capture(err)
		}

		// Do 2 units.
		for i := 0; i < 2; i++ {
			netNodeUUID := uuid.MustNewUUID().String()
			_, err = tx.ExecContext(ctx, "INSERT INTO net_node (uuid) VALUES (?)", netNodeUUID)
			if err != nil {
				return errors.Capture(err)
			}
			unitUUID := uuid.MustNewUUID().String()
			_, err = tx.ExecContext(ctx, `
INSERT INTO unit (uuid, life_id, name, net_node_uuid, application_uuid, charm_uuid)
VALUES (?, ?, ?, ?, (SELECT uuid from application WHERE name = ?), ?)
`, unitUUID, life.Alive, appName+fmt.Sprintf("/%d", i), netNodeUUID, appName, charmUUID)
			if err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestListCharmSecretsToDrainNone(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()

	ctx := c.Context()
	err := createCharmUnitSecret(ctx, st, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	toDrain, err := st.ListCharmSecretsToDrain(ctx, domainsecret.ApplicationOwners{"mariadb"}, domainsecret.NilUnitOwners)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(toDrain, tc.HasLen, 0)
}

func (s *stateSuite) TestListCharmSecretsToDrain(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")
	s.setupUnits(c, "mariadb")

	sp := []domainsecret.UpsertSecretParams{{
		Data:       coresecrets.SecretData{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}, {
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "rev-id",
		},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}}
	uri := []*coresecrets.URI{
		coresecrets.NewURI(),
		coresecrets.NewURI(),
	}

	ctx := c.Context()
	err := createCharmApplicationSecret(ctx, st, 1, uri[0], "mysql", sp[0])
	c.Assert(err, tc.ErrorIsNil)
	err = createCharmUnitSecret(ctx, st, 1, uri[1], "mysql/0", sp[1])
	c.Assert(err, tc.ErrorIsNil)

	uri3 := coresecrets.NewURI()
	sp3 := domainsecret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	err = createUserSecret(ctx, st, 1, uri3, sp3)
	c.Assert(err, tc.ErrorIsNil)

	toDrain, err := st.ListCharmSecretsToDrain(ctx, domainsecret.ApplicationOwners{"mysql"}, domainsecret.UnitOwners{"mysql/0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(toDrain, tc.SameContents, []*coresecrets.SecretMetadataForDrain{{
		URI: uri[0],
		Revisions: []coresecrets.SecretExternalRevision{{
			Revision: 1,
			ValueRef: nil,
		}},
	}, {
		URI: uri[1],
		Revisions: []coresecrets.SecretExternalRevision{{
			Revision: 1,
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-id",
			},
		}},
	}})
}

func (s *stateSuite) TestListUserSecretsToDrainNone(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()

	ctx := c.Context()
	err := createCharmUnitSecret(ctx, st, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	toDrain, err := st.ListUserSecretsToDrain(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(toDrain, tc.HasLen, 0)
}

func (s *stateSuite) TestListUserSecretsToDrain(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := []domainsecret.UpsertSecretParams{{
		Data:       coresecrets.SecretData{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}, {
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "rev-id",
		},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}}
	uri := []*coresecrets.URI{
		coresecrets.NewURI(),
		coresecrets.NewURI(),
	}

	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri[0], sp[0])
	c.Assert(err, tc.ErrorIsNil)
	err = createUserSecret(ctx, st, 1, uri[1], sp[1])
	c.Assert(err, tc.ErrorIsNil)

	uri3 := coresecrets.NewURI()
	sp3 := domainsecret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	err = createCharmUnitSecret(ctx, st, 1, uri3, "mysql/0", sp3)
	c.Assert(err, tc.ErrorIsNil)

	toDrain, err := st.ListUserSecretsToDrain(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(toDrain, tc.SameContents, []*coresecrets.SecretMetadataForDrain{{
		URI: uri[0],
		Revisions: []coresecrets.SecretExternalRevision{{
			Revision: 1,
			ValueRef: nil,
		}},
	}, {
		URI: uri[1],
		Revisions: []coresecrets.SecretExternalRevision{{
			Revision: 1,
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-id",
			},
		}},
	}})
}

func ptr[T any](v T) *T {
	return &v
}

func (s *stateSuite) TestCreateCharmSecretAutoPrune(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
		AutoPrune:   ptr(true),
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmUnitSecret(ctx, st, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIs, secreterrors.AutoPruneNotSupported)
}

func (s *stateSuite) TestCreateCharmApplicationSecretWithContent(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	expireTime := time.Now().Add(2 * time.Hour)
	rotateTime := time.Now().Add(time.Hour)
	sp := domainsecret.UpsertSecretParams{
		Description:    ptr("my secretMetadata"),
		Label:          ptr("my label"),
		Data:           coresecrets.SecretData{"foo": "bar"},
		RotatePolicy:   ptr(domainsecret.RotateYearly),
		ExpireTime:     ptr(expireTime),
		NextRotateTime: ptr(rotateTime),
		RevisionID:     ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmApplicationSecret(ctx, st, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)
	owner := coresecrets.Owner{Kind: coresecrets.ApplicationOwner, ID: "mysql"}
	s.assertSecret(c, st, uri, sp, 1, owner)
	data, ref, err := st.GetSecretValue(ctx, uri, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ref, tc.IsNil)
	c.Assert(data, tc.DeepEquals, coresecrets.SecretData{"foo": "bar"})

	ap := domainsecret.AccessParams{
		SubjectID:     "mysql",
		SubjectTypeID: domainsecret.SubjectApplication,
	}
	access, err := st.GetSecretAccess(ctx, uri, ap)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.Equals, "manage")
}

func (s *stateSuite) TestCreateCharmApplicationSecretNotFound(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmApplicationSecret(ctx, st, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *stateSuite) TestCreateCharmApplicationSecretFailedRevisionIDMissing(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		Checksum:    "checksum-1234",
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmApplicationSecret(ctx, st, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorMatches, `*.revision ID must be provided`)
}

func (s *stateSuite) TestCreateCharmUnitSecretWithContent(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmUnitSecret(ctx, st, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)
	owner := coresecrets.Owner{Kind: coresecrets.UnitOwner, ID: "mysql/0"}
	s.assertSecret(c, st, uri, sp, 1, owner)
	data, ref, err := st.GetSecretValue(ctx, uri, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ref, tc.IsNil)
	c.Assert(data, tc.DeepEquals, coresecrets.SecretData{"foo": "bar"})

	ap := domainsecret.AccessParams{
		SubjectID:     "mysql/0",
		SubjectTypeID: domainsecret.SubjectUnit,
	}
	access, err := st.GetSecretAccess(ctx, uri, ap)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.Equals, "manage")
}

func (s *stateSuite) TestCreateCharmUnitSecretNotFound(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmUnitSecret(ctx, st, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestCreateCharmUnitSecretFailedRevisionIDMissing(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmUnitSecret(ctx, st, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorMatches, `*.revision ID must be provided`)
}

func (s *stateSuite) TestCreateManyApplicationSecretsNoLabelClash(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	createAndCheck := func(label string) {
		content := label
		if content == "" {
			content = "empty"
		}
		sp := domainsecret.UpsertSecretParams{
			Description: ptr("my secretMetadata"),
			Label:       ptr(label),
			Data:        coresecrets.SecretData{"foo": content},
			RevisionID:  ptr(uuid.MustNewUUID().String()),
		}
		uri := coresecrets.NewURI()
		ctx := c.Context()
		err := createCharmApplicationSecret(ctx, st, 1, uri, "mysql", sp)
		c.Assert(err, tc.ErrorIsNil)
		owner := coresecrets.Owner{Kind: coresecrets.ApplicationOwner, ID: "mysql"}
		s.assertSecret(c, st, uri, sp, 1, owner)
		data, ref, err := st.GetSecretValue(ctx, uri, 1)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(ref, tc.IsNil)
		c.Assert(data, tc.DeepEquals, coresecrets.SecretData{"foo": content})
	}
	createAndCheck("my label")
	createAndCheck("")
	createAndCheck("")
	createAndCheck("another label")
}

func (s *stateSuite) TestCreateManyUnitSecretsNoLabelClash(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	createAndCheck := func(label string) {
		content := label
		if content == "" {
			content = "empty"
		}
		sp := domainsecret.UpsertSecretParams{
			Description: ptr("my secretMetadata"),
			Label:       ptr(label),
			Data:        coresecrets.SecretData{"foo": content},
			RevisionID:  ptr(uuid.MustNewUUID().String()),
		}
		uri := coresecrets.NewURI()
		ctx := c.Context()
		err := createCharmUnitSecret(ctx, st, 1, uri, "mysql/0", sp)
		c.Assert(err, tc.ErrorIsNil)
		owner := coresecrets.Owner{Kind: coresecrets.UnitOwner, ID: "mysql/0"}
		s.assertSecret(c, st, uri, sp, 1, owner)
		data, ref, err := st.GetSecretValue(ctx, uri, 1)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(ref, tc.IsNil)
		c.Assert(data, tc.DeepEquals, coresecrets.SecretData{"foo": content})
	}
	createAndCheck("my label")
	createAndCheck("")
	createAndCheck("")
	createAndCheck("another label")
}

func (s *stateSuite) TestListCharmSecretsMissingOwners(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	_, _, err := st.ListCharmSecrets(c.Context(),
		domainsecret.NilApplicationOwners, domainsecret.NilUnitOwners)
	c.Assert(err, tc.ErrorMatches, "querying charm secrets: must supply at least one app owner or unit owner")
}

func (s *stateSuite) TestListCharmSecretsByUnit(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := []domainsecret.UpsertSecretParams{{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		Checksum:    "checksum-1234",
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}, {
		Description: ptr("my secretMetadata2"),
		Label:       ptr("my label2"),
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "revision-id",
		},
		Checksum:   "checksum-5678",
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}}
	uri := []*coresecrets.URI{
		coresecrets.NewURI(),
		coresecrets.NewURI(),
	}

	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri[0], sp[0])
	c.Assert(err, tc.ErrorIsNil)
	err = createCharmUnitSecret(ctx, st, 1, uri[1], "mysql/0", sp[1])
	c.Assert(err, tc.ErrorIsNil)

	secrets, revisions, err := st.ListCharmSecrets(ctx,
		domainsecret.NilApplicationOwners, domainsecret.UnitOwners{"mysql/0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(secrets), tc.Equals, 1)
	c.Assert(len(revisions), tc.Equals, 1)

	now := time.Now()

	md := secrets[0]
	c.Assert(md.Version, tc.Equals, 1)
	c.Assert(md.LatestRevisionChecksum, tc.Equals, sp[1].Checksum)
	c.Assert(md.Label, tc.Equals, value(sp[1].Label))
	c.Assert(md.Description, tc.Equals, value(sp[1].Description))
	c.Assert(md.LatestRevision, tc.Equals, 1)
	c.Assert(md.AutoPrune, tc.IsFalse)
	c.Assert(md.Owner, tc.DeepEquals, coresecrets.Owner{Kind: coresecrets.UnitOwner, ID: "mysql/0"})
	c.Assert(md.CreateTime, tc.Almost, now)
	c.Assert(md.UpdateTime, tc.Almost, now)

	revs := revisions[0]
	c.Assert(revs, tc.HasLen, 1)
	c.Assert(revs[0].Revision, tc.Equals, 1)
	c.Assert(revs[0].ValueRef, tc.DeepEquals, &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "revision-id",
	})
	c.Assert(revs[0].CreateTime, tc.Almost, now)
}

func (s *stateSuite) TestListCharmSecretsByApplication(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := []domainsecret.UpsertSecretParams{{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   ptr(true),
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}, {
		Description: ptr("my secretMetadata2"),
		Label:       ptr("my label2"),
		Data:        coresecrets.SecretData{"foo": "bar2"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}}
	uri := []*coresecrets.URI{
		coresecrets.NewURI(),
		coresecrets.NewURI(),
	}

	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri[0], sp[0])
	c.Assert(err, tc.ErrorIsNil)
	err = createCharmApplicationSecret(ctx, st, 1, uri[1], "mysql", sp[1])
	c.Assert(err, tc.ErrorIsNil)

	secrets, revisions, err := st.ListCharmSecrets(ctx,
		domainsecret.ApplicationOwners{"mysql"}, domainsecret.NilUnitOwners)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(secrets), tc.Equals, 1)
	c.Assert(len(revisions), tc.Equals, 1)

	now := time.Now()

	md := secrets[0]
	c.Assert(md.Version, tc.Equals, 1)
	c.Assert(md.Label, tc.Equals, value(sp[1].Label))
	c.Assert(md.Description, tc.Equals, value(sp[1].Description))
	c.Assert(md.LatestRevision, tc.Equals, 1)
	c.Assert(md.AutoPrune, tc.IsFalse)
	c.Assert(md.Owner, tc.DeepEquals, coresecrets.Owner{Kind: coresecrets.ApplicationOwner, ID: "mysql"})
	c.Assert(md.CreateTime, tc.Almost, now)
	c.Assert(md.UpdateTime, tc.Almost, now)

	revs := revisions[0]
	c.Assert(revs, tc.HasLen, 1)
	c.Assert(revs[0].Revision, tc.Equals, 1)
	c.Assert(revs[0].CreateTime, tc.Almost, now)
}

func (s *stateSuite) TestListCharmSecretsApplicationOrUnit(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")
	s.setupUnits(c, "postgresql")

	expireTime := time.Now().Add(2 * time.Hour)
	rotateTime := time.Now().Add(time.Hour)
	sp := []domainsecret.UpsertSecretParams{{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   ptr(true),
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}, {
		Description:    ptr("my secretMetadata2"),
		Label:          ptr("my label2"),
		Data:           coresecrets.SecretData{"foo": "bar2"},
		RotatePolicy:   ptr(domainsecret.RotateDaily),
		ExpireTime:     ptr(expireTime),
		NextRotateTime: ptr(rotateTime),
		RevisionID:     ptr(uuid.MustNewUUID().String()),
	}, {
		Description: ptr("my secretMetadata3"),
		Label:       ptr("my label3"),
		Data:        coresecrets.SecretData{"foo": "bar3"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}, {
		Description: ptr("my secretMetadata4"),
		Label:       ptr("my label4"),
		Data:        coresecrets.SecretData{"foo": "bar4"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}}
	uri := []*coresecrets.URI{
		coresecrets.NewURI(),
		coresecrets.NewURI(),
		coresecrets.NewURI(),
		coresecrets.NewURI(),
	}

	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri[0], sp[0])
	c.Assert(err, tc.ErrorIsNil)
	err = createCharmApplicationSecret(ctx, st, 1, uri[1], "mysql", sp[1])
	c.Assert(err, tc.ErrorIsNil)
	err = createCharmUnitSecret(ctx, st, 1, uri[2], "mysql/0", sp[2])
	c.Assert(err, tc.ErrorIsNil)
	err = createCharmUnitSecret(ctx, st, 1, uri[3], "postgresql/0", sp[3])
	c.Assert(err, tc.ErrorIsNil)

	secrets, revisions, err := st.ListCharmSecrets(ctx,
		domainsecret.ApplicationOwners{"mysql"}, domainsecret.UnitOwners{"mysql/0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(secrets), tc.Equals, 2)
	c.Assert(len(revisions), tc.Equals, 2)

	now := time.Now()

	first := 0
	second := 1
	if secrets[first].Label != value(sp[1].Label) {
		first = 1
		second = 0
	}

	md := secrets[first]
	c.Assert(md.Version, tc.Equals, 1)
	c.Assert(md.Label, tc.Equals, value(sp[1].Label))
	c.Assert(md.Description, tc.Equals, value(sp[1].Description))
	c.Assert(md.LatestRevision, tc.Equals, 1)
	c.Assert(md.AutoPrune, tc.IsFalse)
	c.Assert(md.RotatePolicy, tc.Equals, coresecrets.RotateDaily)
	c.Assert(*md.NextRotateTime, tc.Equals, rotateTime.UTC())
	c.Assert(*md.LatestExpireTime, tc.Equals, expireTime.UTC())
	c.Assert(md.Owner, tc.DeepEquals, coresecrets.Owner{Kind: coresecrets.ApplicationOwner, ID: "mysql"})
	c.Assert(md.CreateTime, tc.Almost, now)
	c.Assert(md.UpdateTime, tc.Almost, now)

	revs := revisions[first]
	c.Assert(revs, tc.HasLen, 1)
	c.Assert(revs[0].Revision, tc.Equals, 1)
	c.Assert(*revs[0].ExpireTime, tc.Equals, expireTime.UTC())
	c.Assert(revs[0].CreateTime, tc.Almost, now)

	md = secrets[second]
	c.Assert(md.Version, tc.Equals, 1)
	c.Assert(md.Label, tc.Equals, value(sp[2].Label))
	c.Assert(md.Description, tc.Equals, value(sp[2].Description))
	c.Assert(md.LatestRevision, tc.Equals, 1)
	c.Assert(md.AutoPrune, tc.IsFalse)
	c.Assert(md.RotatePolicy, tc.Equals, coresecrets.RotateNever)
	c.Assert(md.Owner, tc.DeepEquals, coresecrets.Owner{Kind: coresecrets.UnitOwner, ID: "mysql/0"})
	c.Assert(md.CreateTime, tc.Almost, now)
	c.Assert(md.UpdateTime, tc.Almost, now)

	revs = revisions[second]
	c.Assert(revs, tc.HasLen, 1)
	c.Assert(revs[0].Revision, tc.Equals, 1)
	c.Assert(revs[0].ExpireTime, tc.IsNil)
	c.Assert(revs[0].CreateTime, tc.Almost, now)
}

func (s *stateSuite) TestAllSecretConsumers(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		ValueRef:   &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		AutoPrune:  ptr(true),
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	sp2 := domainsecret.UpsertSecretParams{
		Data:       map[string]string{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	ctx := c.Context()
	uri := coresecrets.NewURI().WithSource(s.modelUUID)
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)
	uri2 := coresecrets.NewURI().WithSource(s.modelUUID)
	err = createCharmUnitSecret(ctx, st, 1, uri2, "mysql/1", sp2)
	c.Assert(err, tc.ErrorIsNil)

	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	}
	err = st.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, tc.ErrorIsNil)
	consumer = &coresecrets.SecretConsumerMetadata{
		Label:           "my label2",
		CurrentRevision: 668,
	}
	err = st.SaveSecretConsumer(ctx, uri2, "mysql/1", consumer)
	c.Assert(err, tc.ErrorIsNil)
	consumer = &coresecrets.SecretConsumerMetadata{
		Label:           "my label3",
		CurrentRevision: 667,
	}
	err = st.SaveSecretConsumer(ctx, uri, "mysql/1", consumer)
	c.Assert(err, tc.ErrorIsNil)

	got, err := st.AllSecretConsumers(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, map[string][]domainsecret.ConsumerInfo{
		uri.ID: {{
			SubjectTypeID:   domainsecret.SubjectUnit,
			SubjectID:       "mysql/0",
			Label:           "my label",
			CurrentRevision: 666,
		}, {
			SubjectTypeID:   domainsecret.SubjectUnit,
			SubjectID:       "mysql/1",
			Label:           "my label3",
			CurrentRevision: 667,
		}},
		uri2.ID: {{
			SubjectTypeID:   domainsecret.SubjectUnit,
			SubjectID:       "mysql/1",
			Label:           "my label2",
			CurrentRevision: 668,
		}}},
	)
}

func (s *stateSuite) TestSaveSecretConsumer(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		ValueRef:    &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		AutoPrune:   ptr(true),
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI().WithSource(s.modelUUID)
	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	}

	err = st.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, tc.ErrorIsNil)

	got, latest, err := st.GetSecretConsumer(ctx, uri, "mysql/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, consumer)
	c.Assert(latest, tc.Equals, 1)
}

func (s *stateSuite) TestSaveSecretConsumerMarksObsolete(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		ValueRef:    &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		AutoPrune:   ptr(true),
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI().WithSource(s.modelUUID)
	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	consumer := &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 1,
	}
	err = st.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, tc.ErrorIsNil)

	got, latest, err := st.GetSecretConsumer(ctx, uri, "mysql/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, consumer)
	c.Assert(latest, tc.Equals, 1)

	// Latest revision is 3 now, revision 2 shoule be obsolete.
	sp2 := domainsecret.UpsertSecretParams{
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "new-backend",
			RevisionID: "new-revision",
		},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	err = updateSecret(c.Context(), st, uri, sp2)
	c.Assert(err, tc.ErrorIsNil)
	content, valueRef, err := st.GetSecretValue(ctx, uri, 2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(content, tc.IsNil)
	c.Assert(valueRef, tc.DeepEquals, &coresecrets.ValueRef{BackendID: "new-backend", RevisionID: "new-revision"})

	md, err := st.GetSecret(ctx, uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md.Version, tc.Equals, 1)
	c.Assert(md.Label, tc.Equals, value(sp.Label))
	c.Assert(md.Description, tc.Equals, value(sp.Description))
	c.Assert(md.LatestRevision, tc.Equals, 2)

	// Revision 1 now is been consumed by the unit, so it should NOT be obsolete.
	obsolete, pendingDelete := s.getObsolete(c, uri, 1)
	c.Check(obsolete, tc.IsFalse)
	c.Check(pendingDelete, tc.IsFalse)
	// Revision 2 is the latest revision, so it should be NOT obsolete.
	obsolete, pendingDelete = s.getObsolete(c, uri, 2)
	c.Check(obsolete, tc.IsFalse)
	c.Check(pendingDelete, tc.IsFalse)

	// Change to consume the revision 2, so revision 1 should go obsolete.
	consumer = &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 2,
	}
	err = st.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, tc.ErrorIsNil)

	obsolete, pendingDelete = s.getObsolete(c, uri, 1)
	c.Check(obsolete, tc.IsTrue)
	c.Check(pendingDelete, tc.IsTrue)
	obsolete, pendingDelete = s.getObsolete(c, uri, 2)
	c.Check(obsolete, tc.IsFalse)
	c.Check(pendingDelete, tc.IsFalse)
}

func (s *stateSuite) TestSaveSecretConsumerSecretNotExists(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	uri := coresecrets.NewURI().WithSource(s.modelUUID)
	ctx := c.Context()
	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	}

	err := st.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestSaveSecretConsumerUnitNotExists(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		ValueRef:    &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		AutoPrune:   ptr(true),
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI().WithSource(s.modelUUID)
	ctx := c.Context()

	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	}

	err = st.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestSaveSecretConsumerDifferentModel(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	uri := coresecrets.NewURI().WithSource("some-other-model")

	// Save the remote secret and its latest revision.
	err := st.UpdateRemoteSecretRevision(c.Context(), uri, 666)
	c.Assert(err, tc.ErrorIsNil)

	ctx := c.Context()
	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	}

	err = st.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, tc.ErrorIsNil)

	got, _, err := st.GetSecretConsumer(ctx, uri, "mysql/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, consumer)
}

// TestSaveSecretConsumerDifferentModelFirstTime is the same as
// TestSaveSecretConsumerDifferentModel but there's no remote revision
// recorded yet.
func (s *stateSuite) TestSaveSecretConsumerDifferentModelFirstTime(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	uri := coresecrets.NewURI().WithSource("some-other-model")

	ctx := c.Context()
	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	}

	err := st.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, tc.ErrorIsNil)

	got, _, err := st.GetSecretConsumer(ctx, uri, "mysql/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, consumer)

	var latest int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
SELECT latest_revision FROM secret_reference WHERE secret_id = ?
		`, uri.ID)
		if err := row.Scan(&latest); err != nil {
			return err
		}
		return row.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(latest, tc.Equals, 666)
}

func (s *stateSuite) TestAllRemoteSecrets(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	uri := coresecrets.NewURI().WithSource("some-other-model")

	// Save the remote secret and its latest revision.
	err := st.UpdateRemoteSecretRevision(c.Context(), uri, 666)
	c.Assert(err, tc.ErrorIsNil)

	ctx := c.Context()
	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 1,
	}
	err = st.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, tc.ErrorIsNil)

	consumer = &coresecrets.SecretConsumerMetadata{
		Label:           "my label2",
		CurrentRevision: 2,
	}
	err = st.SaveSecretConsumer(ctx, uri, "mysql/1", consumer)
	c.Assert(err, tc.ErrorIsNil)

	got, err := st.AllRemoteSecrets(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, []domainsecret.RemoteSecretInfo{{
		URI:             uri,
		SubjectTypeID:   domainsecret.SubjectUnit,
		SubjectID:       "mysql/0",
		Label:           "my label",
		CurrentRevision: 1,
		LatestRevision:  666,
	}, {
		URI:             uri,
		SubjectTypeID:   domainsecret.SubjectUnit,
		SubjectID:       "mysql/1",
		Label:           "my label2",
		CurrentRevision: 2,
		LatestRevision:  666,
	}})
}

func (s *stateSuite) TestGetSecretConsumerFirstTime(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		ValueRef:    &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		AutoPrune:   ptr(true),
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()

	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	_, latest, err := st.GetSecretConsumer(ctx, uri, "mysql/0")
	c.Assert(err, tc.ErrorIs, secreterrors.SecretConsumerNotFound)
	c.Assert(latest, tc.Equals, 1)
}

func (s *stateSuite) TestGetSecretConsumerRemoteSecretFirstTime(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	uri := coresecrets.NewURI().WithSource("some-other-model")
	ctx := c.Context()

	err := st.UpdateRemoteSecretRevision(ctx, uri, 666)
	c.Assert(err, tc.ErrorIsNil)

	_, latest, err := st.GetSecretConsumer(ctx, uri, "mysql/0")
	c.Assert(err, tc.ErrorIs, secreterrors.SecretConsumerNotFound)
	c.Assert(latest, tc.Equals, 666)
}

func (s *stateSuite) TestGetSecretConsumerSecretNotExists(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	uri := coresecrets.NewURI()

	_, _, err := st.GetSecretConsumer(c.Context(), uri, "mysql/0")
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGetSecretConsumerUnitNotExists(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		ValueRef:    &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		AutoPrune:   ptr(true),
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()

	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	_, _, err = st.GetSecretConsumer(ctx, uri, "mysql/0")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestGetUserSecretURIByLabel(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   ptr(true),
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	got, err := st.GetUserSecretURIByLabel(ctx, "my label")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got.ID, tc.Equals, uri.ID)
}

func (s *stateSuite) TestGetUserSecretURIByLabelSecretNotExists(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	_, err := st.GetUserSecretURIByLabel(c.Context(), "my label")
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGetURIByConsumerLabel(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmUnitSecret(ctx, st, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)
	err = st.SaveSecretConsumer(ctx, uri, "mysql/0", &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	})
	c.Assert(err, tc.ErrorIsNil)

	got, err := st.GetURIByConsumerLabel(ctx, "my label", "mysql/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got.ID, tc.Equals, uri.ID)
	c.Assert(got.SourceUUID, tc.Equals, uri.SourceUUID)

	_, err = st.GetURIByConsumerLabel(ctx, "another label", "mysql/0")
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)

}

func (s *stateSuite) TestGetURIByConsumerLabelUnitNotExists(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	_, err := st.GetURIByConsumerLabel(c.Context(), "my label", "mysql/2")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestGetSecretOwnerNotFound(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	_, err := getSecretOwner(c.Context(), st, coresecrets.NewURI())
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGetSecretOwnerUnitOwned(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmUnitSecret(ctx, st, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	unitUUID, err := getUnitUUID(ctx, st, "mysql/0")
	c.Assert(err, tc.ErrorIsNil)

	owner, err := getSecretOwner(ctx, st, uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(owner, tc.DeepEquals, domainsecret.Owner{Kind: domainsecret.UnitOwner, UUID: unitUUID.String()})
}

func (s *stateSuite) TestGetSecretOwnerApplicationOwned(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmApplicationSecret(ctx, st, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	appUUID, err := getApplicationUUID(ctx, st, "mysql")
	c.Assert(err, tc.ErrorIsNil)

	owner, err := getSecretOwner(ctx, st, uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(owner, tc.DeepEquals, domainsecret.Owner{Kind: domainsecret.ApplicationOwner, UUID: appUUID.String()})
}

func (s *stateSuite) TestGetSecretOwnerUserSecret(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	owner, err := getSecretOwner(ctx, st, uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(owner, tc.DeepEquals, domainsecret.Owner{Kind: domainsecret.ModelOwner})
}

func (s *stateSuite) TestUpdateSecretNotFound(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	uri := coresecrets.NewURI()
	err := updateSecret(c.Context(), st, uri, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Label:      ptr("label"),
	})
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestUpdateSecretNothingToDo(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	uri := coresecrets.NewURI()
	err := updateSecret(c.Context(), st, uri, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String())})
	c.Assert(err, tc.ErrorMatches, "must specify a new value or metadata to update a secret")
}

func (s *stateSuite) TestUpdateUserSecretMetadataOnly(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label2"),
	}
	err = updateSecret(c.Context(), st, uri, sp2)
	c.Assert(err, tc.ErrorIsNil)

	md, err := st.GetSecret(ctx, uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md.Version, tc.Equals, 1)
	c.Assert(md.Label, tc.Equals, value(sp2.Label))
	c.Assert(md.Description, tc.Equals, value(sp2.Description))
	c.Assert(md.LatestRevision, tc.Equals, 1)

	now := time.Now()
	c.Assert(md.UpdateTime, tc.Almost, now)
}

func (s *stateSuite) TestUpdateUserSecretFailedRevisionIDMissing(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   ptr(true),
	}

	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	sp = domainsecret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo": "something-else"},
	}
	err = updateSecret(ctx, st, uri, sp)
	c.Assert(err, tc.ErrorMatches, `*.revision ID must be provided`)
}

func (s *stateSuite) TestUpdateCharmApplicationSecretMetadataOnly(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmApplicationSecret(ctx, st, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label2"),
	}
	err = updateSecret(c.Context(), st, uri, sp2)
	c.Assert(err, tc.ErrorIsNil)

	md, err := st.GetSecret(ctx, uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md.Version, tc.Equals, 1)
	c.Assert(md.Label, tc.Equals, value(sp2.Label))
	c.Assert(md.Description, tc.Equals, value(sp2.Description))
	c.Assert(md.LatestRevision, tc.Equals, 1)

	now := time.Now()
	c.Assert(md.UpdateTime, tc.Almost, now)
}

func (s *stateSuite) TestUpdateCharmUnitSecretMetadataOnly(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmUnitSecret(ctx, st, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label2"),
	}
	err = updateSecret(c.Context(), st, uri, sp2)
	c.Assert(err, tc.ErrorIsNil)

	md, err := st.GetSecret(ctx, uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md.Version, tc.Equals, 1)
	c.Assert(md.Label, tc.Equals, value(sp2.Label))
	c.Assert(md.Description, tc.Equals, value(sp2.Description))
	c.Assert(md.LatestRevision, tc.Equals, 1)

	now := time.Now()
	c.Assert(md.UpdateTime, tc.Almost, now)
}

func fillDataForUpsertSecretParams(c *tc.C, p *domainsecret.UpsertSecretParams, data coresecrets.SecretData) {
	checksum, err := coresecrets.NewSecretValue(data).Checksum()
	c.Assert(err, tc.ErrorIsNil)
	p.Data = data
	p.Checksum = checksum
}

func (s *stateSuite) TestUpdateSecretContentNoOpsIfNoContentChange(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	fillDataForUpsertSecretParams(c, &sp, coresecrets.SecretData{"foo": "bar", "hello": "world"})
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmUnitSecret(ctx, st, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	err = updateSecret(c.Context(), st, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	md, revs, err := st.ListSecrets(ctx, uri, ptr(1), domainsecret.NilLabels)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md, tc.HasLen, 1)
	c.Assert(md[0].LatestRevision, tc.Equals, 1)

	c.Assert(revs, tc.HasLen, 1)
	c.Assert(revs[0], tc.HasLen, 1)
	rev := revs[0][0]
	c.Assert(rev.Revision, tc.Equals, 1)
}

func (s *stateSuite) TestUpdateSecretContent(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	fillDataForUpsertSecretParams(c, &sp, coresecrets.SecretData{"foo": "bar", "hello": "world"})
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmUnitSecret(ctx, st, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	expireTime := time.Now().Add(2 * time.Hour)
	sp2 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		ExpireTime: &expireTime,
	}
	fillDataForUpsertSecretParams(c, &sp2, coresecrets.SecretData{"foo2": "bar2", "hello": "world"})
	err = updateSecret(c.Context(), st, uri, sp2)
	c.Assert(err, tc.ErrorIsNil)

	md, revs, err := st.ListSecrets(ctx, uri, ptr(2), domainsecret.NilLabels)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md, tc.HasLen, 1)
	c.Assert(md[0].Version, tc.Equals, 1)
	c.Assert(md[0].Label, tc.Equals, value(sp.Label))
	c.Assert(md[0].Description, tc.Equals, value(sp.Description))
	c.Assert(md[0].LatestRevision, tc.Equals, 2)

	now := time.Now()
	c.Assert(md[0].UpdateTime, tc.Almost, now)

	c.Assert(revs, tc.HasLen, 1)
	c.Assert(revs[0], tc.HasLen, 1)
	rev := revs[0][0]
	c.Assert(rev.Revision, tc.Equals, 2)
	c.Assert(rev.ExpireTime, tc.NotNil)
	c.Assert(*rev.ExpireTime, tc.Equals, expireTime.UTC())

	content, valueRef, err := st.GetSecretValue(ctx, uri, 2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(valueRef, tc.IsNil)
	c.Assert(content, tc.DeepEquals, coresecrets.SecretData{"foo2": "bar2", "hello": "world"})

	// Revision 1 is obsolete.
	obsolete, pendingDelete := s.getObsolete(c, uri, 1)
	c.Check(obsolete, tc.IsTrue)
	c.Check(pendingDelete, tc.IsTrue)

	// But not revision 2.
	obsolete, pendingDelete = s.getObsolete(c, uri, 2)
	c.Check(obsolete, tc.IsFalse)
	c.Check(pendingDelete, tc.IsFalse)
}

func (s *stateSuite) TestUpdateSecretContentObsolete(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	// Create a consumer so revision 1 does not go obsolete.
	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 1,
	}

	err = st.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, tc.ErrorIsNil)

	expireTime := time.Now().Add(2 * time.Hour)
	sp2 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		ExpireTime: &expireTime,
		Data:       coresecrets.SecretData{"foo2": "bar2", "hello": "world"},
	}
	err = updateSecret(c.Context(), st, uri, sp2)
	c.Assert(err, tc.ErrorIsNil)

	md, revs, err := st.ListSecrets(ctx, uri, ptr(2), domainsecret.NilLabels)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md, tc.HasLen, 1)
	c.Assert(md[0].Version, tc.Equals, 1)
	c.Assert(md[0].Label, tc.Equals, value(sp.Label))
	c.Assert(md[0].Description, tc.Equals, value(sp.Description))
	c.Assert(md[0].LatestRevision, tc.Equals, 2)

	now := time.Now()
	c.Assert(md[0].UpdateTime, tc.Almost, now)

	c.Assert(revs, tc.HasLen, 1)
	c.Assert(revs[0], tc.HasLen, 1)
	rev := revs[0][0]
	c.Assert(rev.Revision, tc.Equals, 2)
	c.Assert(rev.ExpireTime, tc.NotNil)
	c.Assert(*rev.ExpireTime, tc.Equals, expireTime.UTC())

	content, valueRef, err := st.GetSecretValue(ctx, uri, 2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(valueRef, tc.IsNil)
	c.Assert(content, tc.DeepEquals, coresecrets.SecretData{"foo2": "bar2", "hello": "world"})

	// Latest revision is 3 now, revision 2 shoule be obsolete.
	sp3 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo3": "bar3", "hello": "world"},
	}
	err = updateSecret(c.Context(), st, uri, sp3)
	c.Assert(err, tc.ErrorIsNil)
	content, valueRef, err = st.GetSecretValue(ctx, uri, 3)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(valueRef, tc.IsNil)
	c.Assert(content, tc.DeepEquals, coresecrets.SecretData{"foo3": "bar3", "hello": "world"})

	md, _, err = st.ListSecrets(ctx, uri, ptr(2), domainsecret.NilLabels)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md, tc.HasLen, 1)
	c.Assert(md[0].Version, tc.Equals, 1)
	c.Assert(md[0].Label, tc.Equals, value(sp.Label))
	c.Assert(md[0].Description, tc.Equals, value(sp.Description))
	c.Assert(md[0].LatestRevision, tc.Equals, 3)

	var obsolete0, pendingDelete0 bool
	var obsolete1, pendingDelete1 bool
	var obsolete2, pendingDelete2 bool
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		// Revision 1 is NOT obsolete because it's been consumed.
		obsolete0, pendingDelete0 = s.getObsolete(c, uri, 1)

		// Revision 2 is obsolete.
		obsolete1, pendingDelete1 = s.getObsolete(c, uri, 2)

		// Revision 3 is NOT obsolete because it's the latest revision.
		obsolete2, pendingDelete2 = s.getObsolete(c, uri, 3)
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(obsolete0, tc.IsFalse)
	c.Check(pendingDelete0, tc.IsFalse)

	c.Check(obsolete1, tc.IsTrue)
	c.Check(pendingDelete1, tc.IsTrue)

	c.Check(obsolete2, tc.IsFalse)
	c.Check(pendingDelete2, tc.IsFalse)
}

func (s *stateSuite) getObsolete(c *tc.C, uri *coresecrets.URI, rev int) (bool, bool) {
	var obsolete, pendingDelete bool
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
SELECT obsolete, pending_delete
FROM secret_revision_obsolete sro
INNER JOIN secret_revision sr ON sro.revision_uuid = sr.uuid
WHERE sr.secret_id = ? AND sr.revision = ?`, uri.ID, rev)
		err := row.Scan(&obsolete, &pendingDelete)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	})
	c.Check(err, tc.ErrorIsNil)
	return obsolete, pendingDelete
}

func (s *stateSuite) TestUpdateSecretContentValueRef(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmUnitSecret(ctx, st, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		ValueRef:   &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "revision-id"},
	}
	err = updateSecret(c.Context(), st, uri, sp2)
	c.Assert(err, tc.ErrorIsNil)

	md, revs, err := st.ListSecrets(ctx, uri, ptr(2), domainsecret.NilLabels)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md, tc.HasLen, 1)
	c.Assert(md[0].Version, tc.Equals, 1)
	c.Assert(md[0].Label, tc.Equals, value(sp.Label))
	c.Assert(md[0].Description, tc.Equals, value(sp.Description))
	c.Assert(md[0].LatestRevision, tc.Equals, 2)

	now := time.Now()
	c.Assert(md[0].UpdateTime, tc.Almost, now)

	c.Assert(revs, tc.HasLen, 1)
	c.Assert(revs[0], tc.HasLen, 1)
	rev := revs[0][0]
	c.Assert(rev.Revision, tc.Equals, 2)
	c.Assert(rev.ExpireTime, tc.IsNil)

	content, valueRef, err := st.GetSecretValue(ctx, uri, 2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(valueRef, tc.DeepEquals, &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "revision-id"})
	c.Assert(content, tc.HasLen, 0)
}

func (s *stateSuite) TestUpdateSecretNoRotate(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:   ptr(uuid.MustNewUUID().String()),
		RotatePolicy: ptr(domainsecret.RotateDaily),
		Data:         coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID:   ptr(uuid.MustNewUUID().String()),
		RotatePolicy: ptr(domainsecret.RotateNever),
	}
	err = updateSecret(c.Context(), st, uri, sp2)
	c.Assert(err, tc.ErrorIsNil)

	md, err := st.GetSecret(ctx, uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md.RotatePolicy, tc.Equals, coresecrets.RotateNever)
	c.Assert(md.NextRotateTime, tc.IsNil)

	var count int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
SELECT count(*) FROM secret_rotation WHERE secret_id = ?
		`, uri.ID)
		if err := row.Scan(&count); err != nil {
			return err
		}
		return row.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(count, tc.Equals, 0)
}

func (s *stateSuite) TestSaveSecretRemoteConsumer(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		ValueRef:    &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		AutoPrune:   ptr(true),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	consumer := &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	}

	err = st.SaveSecretRemoteConsumer(ctx, uri, "remote-app/0", consumer)
	c.Assert(err, tc.ErrorIsNil)

	got, latest, err := st.GetSecretRemoteConsumer(ctx, uri, "remote-app/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, consumer)
	c.Assert(latest, tc.Equals, 1)
}

func (s *stateSuite) TestAllSecretRemoteConsumers(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		ValueRef:   &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		AutoPrune:  ptr(true),
	}
	sp2 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       map[string]string{"foo": "bar"},
	}
	ctx := c.Context()
	uri := coresecrets.NewURI().WithSource(s.modelUUID)
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)
	uri2 := coresecrets.NewURI().WithSource(s.modelUUID)
	err = createCharmUnitSecret(ctx, st, 1, uri2, "mysql/1", sp2)
	c.Assert(err, tc.ErrorIsNil)

	consumer := &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	}
	err = st.SaveSecretRemoteConsumer(ctx, uri, "remote-app/0", consumer)
	c.Assert(err, tc.ErrorIsNil)
	consumer = &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 668,
	}
	err = st.SaveSecretRemoteConsumer(ctx, uri2, "remote-app/1", consumer)
	c.Assert(err, tc.ErrorIsNil)
	consumer = &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 667,
	}
	err = st.SaveSecretRemoteConsumer(ctx, uri, "remote-app/1", consumer)
	c.Assert(err, tc.ErrorIsNil)

	got, err := st.AllSecretRemoteConsumers(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, map[string][]domainsecret.ConsumerInfo{
		uri.ID: {{
			SubjectTypeID:   domainsecret.SubjectUnit,
			SubjectID:       "remote-app/0",
			CurrentRevision: 666,
		}, {
			SubjectTypeID:   domainsecret.SubjectUnit,
			SubjectID:       "remote-app/1",
			CurrentRevision: 667,
		}},
		uri2.ID: {{
			SubjectTypeID:   domainsecret.SubjectUnit,
			SubjectID:       "remote-app/1",
			CurrentRevision: 668,
		}}},
	)
}

func (s *stateSuite) TestSaveSecretRemoteConsumerMarksObsolete(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		ValueRef:    &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		AutoPrune:   ptr(true),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)
	sp.Label = ptr("")
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = updateSecret(ctx, st, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	consumer := &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 1,
	}

	err = st.SaveSecretRemoteConsumer(ctx, uri, "remote-app/0", consumer)
	c.Assert(err, tc.ErrorIsNil)

	consumer.CurrentRevision = 2
	err = st.SaveSecretRemoteConsumer(ctx, uri, "remote-app/0", consumer)
	c.Assert(err, tc.ErrorIsNil)

	// Revision 1 is obsolete.
	obsolete, pendingDelete := s.getObsolete(c, uri, 1)
	c.Check(obsolete, tc.IsTrue)
	c.Check(pendingDelete, tc.IsTrue)

	// But not revision 2.
	obsolete, pendingDelete = s.getObsolete(c, uri, 2)
	c.Check(obsolete, tc.IsFalse)
	c.Check(pendingDelete, tc.IsFalse)
}

func (s *stateSuite) TestSaveSecretRemoteConsumerSecretNotExists(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	uri := coresecrets.NewURI().WithSource(s.modelUUID)
	ctx := c.Context()
	consumer := &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	}

	err := st.SaveSecretConsumer(ctx, uri, "remote-app/0", consumer)
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGetSecretRemoteConsumerFirstTime(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		ValueRef:    &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		AutoPrune:   ptr(true),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()

	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	_, latest, err := st.GetSecretRemoteConsumer(ctx, uri, "remote-app/0")
	c.Assert(err, tc.ErrorIs, secreterrors.SecretConsumerNotFound)
	c.Assert(latest, tc.Equals, 1)
}

func (s *stateSuite) TestGetSecretRemoteConsumerSecretNotExists(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	uri := coresecrets.NewURI()

	_, _, err := st.GetSecretRemoteConsumer(c.Context(), uri, "remite-app/0")
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestUpdateRemoteSecretRevision(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	uri := coresecrets.NewURI()

	getLatest := func() int {
		var got int
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			row := tx.QueryRowContext(ctx, `
			SELECT latest_revision FROM secret_reference WHERE secret_id = ?
		`, uri.ID)
			if err := row.Scan(&got); err != nil {
				return err
			}
			return row.Err()
		})
		c.Assert(err, tc.ErrorIsNil)
		return got
	}

	err := st.UpdateRemoteSecretRevision(c.Context(), uri, 666)
	c.Assert(err, tc.ErrorIsNil)
	got := getLatest()
	c.Assert(got, tc.Equals, 666)
	err = st.UpdateRemoteSecretRevision(c.Context(), uri, 667)
	c.Assert(err, tc.ErrorIsNil)
	got = getLatest()
	c.Assert(got, tc.Equals, 667)
}

func (s *stateSuite) TestGrantUnitAccess(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmUnitSecret(ctx, st, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeUnit,
		ScopeID:       "mysql/0",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIsNil)

	ap := domainsecret.AccessParams{
		SubjectTypeID: p.SubjectTypeID,
		SubjectID:     p.SubjectID,
	}
	role, err := st.GetSecretAccess(ctx, uri, ap)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(role, tc.Equals, "view")
}

func (s *stateSuite) TestGetUnitGrantAccessScope(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmUnitSecret(ctx, st, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeUnit,
		ScopeID:       "mysql/0",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIsNil)

	ap := domainsecret.AccessParams{
		SubjectTypeID: p.SubjectTypeID,
		SubjectID:     p.SubjectID,
	}
	scope, err := st.GetSecretAccessScope(ctx, uri, ap)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(scope, tc.DeepEquals, &domainsecret.AccessScope{
		ScopeTypeID: domainsecret.ScopeUnit,
		ScopeID:     "mysql/0",
	})
}

func (s *stateSuite) TestGrantApplicationAccess(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeApplication,
		ScopeID:       "mysql",
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIsNil)

	ap := domainsecret.AccessParams{
		SubjectTypeID: p.SubjectTypeID,
		SubjectID:     p.SubjectID,
	}
	role, err := st.GetSecretAccess(ctx, uri, ap)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(role, tc.Equals, "view")
}

func (s *stateSuite) TestGetApplicationGrantAccessScope(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeApplication,
		ScopeID:       "mysql",
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIsNil)

	ap := domainsecret.AccessParams{
		SubjectTypeID: p.SubjectTypeID,
		SubjectID:     p.SubjectID,
	}
	scope, err := st.GetSecretAccessScope(ctx, uri, ap)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(scope, tc.DeepEquals, &domainsecret.AccessScope{
		ScopeTypeID: domainsecret.ScopeApplication,
		ScopeID:     "mysql",
	})
}

func (s *stateSuite) TestGrantModelAccess(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeModel,
		ScopeID:       s.modelUUID,
		SubjectTypeID: domainsecret.SubjectModel,
		SubjectID:     s.modelUUID,
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIsNil)

	ap := domainsecret.AccessParams{
		SubjectTypeID: p.SubjectTypeID,
		SubjectID:     p.SubjectID,
	}
	role, err := st.GetSecretAccess(ctx, uri, ap)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(role, tc.Equals, "view")
}

func (s *stateSuite) TestGrantRelationScope(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmUnitSecret(ctx, st, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/1",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIsNil)

	ap := domainsecret.AccessParams{
		SubjectTypeID: p.SubjectTypeID,
		SubjectID:     p.SubjectID,
	}
	role, err := st.GetSecretAccess(ctx, uri, ap)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(role, tc.Equals, "view")
}

func (s *stateSuite) TestGetRelationGrantAccessScope(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmUnitSecret(ctx, st, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/1",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIsNil)

	ap := domainsecret.AccessParams{
		SubjectTypeID: p.SubjectTypeID,
		SubjectID:     p.SubjectID,
	}
	scope, err := st.GetSecretAccessScope(ctx, uri, ap)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(scope, tc.DeepEquals, &domainsecret.AccessScope{
		ScopeTypeID: domainsecret.ScopeRelation,
		ScopeID:     "mysql:db mediawiki:db",
	})
}

func (s *stateSuite) TestGrantAccessInvariantScope(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmUnitSecret(ctx, st, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeUnit,
		ScopeID:       "mysql/0",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIsNil)
	p.ScopeID = "mysql"
	p.ScopeTypeID = domainsecret.ScopeApplication
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIs, secreterrors.InvalidSecretPermissionChange)
}

func (s *stateSuite) TestGrantSecretNotFound(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	uri := coresecrets.NewURI()
	ctx := c.Context()

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeUnit,
		ScopeID:       "mysql/0",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}
	err := st.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGrantUnitNotFound(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmUnitSecret(ctx, st, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeUnit,
		ScopeID:       "mysql/0",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/2",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestGrantApplicationNotFound(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmUnitSecret(ctx, st, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeUnit,
		ScopeID:       "mysql/0",
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "postgresql",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *stateSuite) TestGrantScopeNotFound(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmUnitSecret(ctx, st, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeUnit,
		ScopeID:       "mysql/2",
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestGetAccessNoGrant(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmUnitSecret(ctx, st, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	ap := domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
	}
	role, err := st.GetSecretAccess(ctx, uri, ap)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(role, tc.Equals, "")
}

func (s *stateSuite) TestGetSecretGrantsNone(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	g, err := st.GetSecretGrants(ctx, uri, coresecrets.RoleView)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(g, tc.HasLen, 0)
}

func (s *stateSuite) TestGetSecretGrantsAppUnit(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/1",
		RoleID:        domainsecret.RoleManage,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIsNil)

	p2 := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p2)
	c.Assert(err, tc.ErrorIsNil)

	g, err := st.GetSecretGrants(ctx, uri, coresecrets.RoleView)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(g, tc.DeepEquals, []domainsecret.GrantParams{{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}})
}

func (s *stateSuite) TestGetSecretGrantsModel(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/1",
		RoleID:        domainsecret.RoleManage,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIsNil)

	p2 := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeModel,
		ScopeID:       s.modelUUID,
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p2)
	c.Assert(err, tc.ErrorIsNil)

	g, err := st.GetSecretGrants(ctx, uri, coresecrets.RoleView)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(g, tc.DeepEquals, []domainsecret.GrantParams{{
		ScopeTypeID:   domainsecret.ScopeModel,
		ScopeID:       s.modelUUID,
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}})
}

func (s *stateSuite) TestAllSecretGrants(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar"},
	}
	sp2 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar2"},
	}
	ctx := c.Context()
	uri := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)
	err = createCharmApplicationSecret(ctx, st, 1, uri2, "mysql", sp2)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/1",
		RoleID:        domainsecret.RoleManage,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIsNil)

	p2 := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p2)
	c.Assert(err, tc.ErrorIsNil)

	g, err := st.AllSecretGrants(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(g, tc.DeepEquals, map[string][]domainsecret.GrantParams{
		uri.ID: {{
			ScopeTypeID:   domainsecret.ScopeModel,
			ScopeID:       s.modelUUID,
			SubjectTypeID: domainsecret.SubjectModel,
			SubjectID:     s.modelUUID,
			RoleID:        domainsecret.RoleManage,
		}, {
			ScopeTypeID:   domainsecret.ScopeRelation,
			ScopeID:       "mysql:db mediawiki:db",
			SubjectTypeID: domainsecret.SubjectUnit,
			SubjectID:     "mysql/1",
			RoleID:        domainsecret.RoleManage,
		}, {
			ScopeTypeID:   domainsecret.ScopeRelation,
			ScopeID:       "mysql:db mediawiki:db",
			SubjectTypeID: domainsecret.SubjectUnit,
			SubjectID:     "mysql/0",
			RoleID:        domainsecret.RoleView,
		}},
		uri2.ID: {{
			ScopeTypeID:   domainsecret.ScopeApplication,
			ScopeID:       "mysql",
			SubjectTypeID: domainsecret.SubjectApplication,
			SubjectID:     "mysql",
			RoleID:        domainsecret.RoleManage,
		}}})
}

func (s *stateSuite) TestRevokeAccess(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/1",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIsNil)

	p2 := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p2)
	c.Assert(err, tc.ErrorIsNil)

	err = st.RevokeAccess(ctx, uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/1",
	})
	c.Assert(err, tc.ErrorIsNil)

	g, err := st.GetSecretGrants(ctx, uri, coresecrets.RoleView)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(g, tc.DeepEquals, []domainsecret.GrantParams{{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}})
}

func (s *stateSuite) TestListGrantedSecrets(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	ctx := c.Context()
	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	err := createUserSecret(ctx, st, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "revision-id",
		},
	}
	uri2 := coresecrets.NewURI()
	err = createUserSecret(ctx, st, 1, uri2, sp2)
	c.Assert(err, tc.ErrorIsNil)

	sp3 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "revision-id2",
		},
	}
	uri3 := coresecrets.NewURI()
	err = createUserSecret(ctx, st, 1, uri3, sp3)
	c.Assert(err, tc.ErrorIsNil)
	err = updateSecret(ctx, st, uri3, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "backend-id2",
			RevisionID: "revision-id3",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIsNil)
	err = st.GrantAccess(ctx, uri2, p)
	c.Assert(err, tc.ErrorIsNil)

	p2 := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri3, p2)
	c.Assert(err, tc.ErrorIsNil)

	accessors := []domainsecret.AccessParams{{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
	}, {
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
	}}
	result, err := st.ListGrantedSecretsForBackend(ctx, "backend-id", accessors, coresecrets.RoleView)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []*coresecrets.SecretRevisionRef{{
		URI:        uri2,
		RevisionID: "revision-id",
	}, {
		URI:        uri3,
		RevisionID: "revision-id2",
	}})
}

func (s *stateSuite) prepareSecretObsoleteRevisions(c *tc.C, st *State) (
	*coresecrets.URI, *coresecrets.URI, *coresecrets.URI, *coresecrets.URI,
) {
	ctx := c.Context()
	s.setupUnits(c, "mysql")
	s.setupUnits(c, "mediawiki")

	sp := domainsecret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri1 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err := createCharmApplicationSecret(ctx, st, 1, uri1, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)
	updateSecretContent(c, st, uri1)

	uri2 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = createCharmUnitSecret(ctx, st, 1, uri2, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)
	updateSecretContent(c, st, uri2)

	uri3 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = createCharmApplicationSecret(ctx, st, 1, uri3, "mediawiki", sp)
	c.Assert(err, tc.ErrorIsNil)
	updateSecretContent(c, st, uri3)

	uri4 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = createCharmUnitSecret(ctx, st, 1, uri4, "mediawiki/0", sp)
	c.Assert(err, tc.ErrorIsNil)
	updateSecretContent(c, st, uri4)
	return uri1, uri2, uri3, uri4
}

func (s *stateSuite) TestInitialWatchStatementForObsoleteRevision(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	uri1, uri2, uri3, uri4 := s.prepareSecretObsoleteRevisions(c, st)
	ctx := c.Context()

	tableName, f := st.InitialWatchStatementForObsoleteRevision(
		[]string{"mysql", "mediawiki"},
		[]string{"mysql/0", "mediawiki/0"},
	)
	c.Assert(tableName, tc.Equals, "secret_revision_obsolete")
	revisionUUIDs, err := f(ctx, s.TxnRunner())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(revisionUUIDs, tc.SameContents, []string{
		getRevUUID(c, s.DB(), uri1, 1),
		getRevUUID(c, s.DB(), uri2, 1),
		getRevUUID(c, s.DB(), uri3, 1),
		getRevUUID(c, s.DB(), uri4, 1),
	})
}

func updateSecretContent(c *tc.C, st *State, uri *coresecrets.URI) {
	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo-new": "bar-new"},
	}
	err := updateSecret(c.Context(), st, uri, sp)
	c.Assert(err, tc.ErrorIsNil)
}

func getRevUUID(c *tc.C, db *sql.DB, uri *coresecrets.URI, rev int) string {
	var uuid string
	row := db.QueryRowContext(c.Context(), `
SELECT uuid
FROM secret_revision
WHERE secret_id = ? AND revision = ?
`, uri.ID, rev)
	err := row.Scan(&uuid)
	c.Assert(err, tc.ErrorIsNil)
	return uuid
}

func (s *stateSuite) TestGetRevisionIDsForObsolete(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	uri1, uri2, uri3, uri4 := s.prepareSecretObsoleteRevisions(c, st)
	ctx := c.Context()

	result, err := st.GetRevisionIDsForObsolete(ctx,
		nil, nil,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)

	// no owners, revUUIDs.
	result, err = st.GetRevisionIDsForObsolete(ctx,
		nil, nil,
		getRevUUID(c, s.DB(), uri1, 1),
		getRevUUID(c, s.DB(), uri2, 1),
		getRevUUID(c, s.DB(), uri3, 1),
		getRevUUID(c, s.DB(), uri4, 1),
		getRevUUID(c, s.DB(), uri1, 2),
		getRevUUID(c, s.DB(), uri2, 2),
		getRevUUID(c, s.DB(), uri3, 2),
		getRevUUID(c, s.DB(), uri4, 2),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		revID(uri1, 1),
		revID(uri2, 1),
		revID(uri3, 1),
		revID(uri4, 1),
	})

	// appOwners, unitOwners, revUUIDs.
	result, err = st.GetRevisionIDsForObsolete(ctx,
		[]string{
			"mysql",
			"mediawiki",
		},
		[]string{
			"mysql/0",
			"mediawiki/0",
		},
		getRevUUID(c, s.DB(), uri1, 1),
		getRevUUID(c, s.DB(), uri2, 1),
		getRevUUID(c, s.DB(), uri3, 1),
		getRevUUID(c, s.DB(), uri4, 1),
		getRevUUID(c, s.DB(), uri1, 2),
		getRevUUID(c, s.DB(), uri2, 2),
		getRevUUID(c, s.DB(), uri3, 2),
		getRevUUID(c, s.DB(), uri4, 2),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		revID(uri1, 1),
		revID(uri2, 1),
		revID(uri3, 1),
		revID(uri4, 1),
	})

	// appOwners, unitOwners, no revisions.
	result, err = st.GetRevisionIDsForObsolete(ctx,
		[]string{
			"mysql",
			"mediawiki",
		},
		[]string{
			"mysql/0",
			"mediawiki/0",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		revID(uri1, 1),
		revID(uri2, 1),
		revID(uri3, 1),
		revID(uri4, 1),
	})

	// appOwners, unitOwners, revUUIDs(with unknown app owned revisions).
	result, err = st.GetRevisionIDsForObsolete(ctx,
		[]string{
			"mysql",
		},
		[]string{
			"mysql/0",
			"mediawiki/0",
		},
		getRevUUID(c, s.DB(), uri1, 1),
		getRevUUID(c, s.DB(), uri2, 1),
		getRevUUID(c, s.DB(), uri3, 1),
		getRevUUID(c, s.DB(), uri4, 1),
		getRevUUID(c, s.DB(), uri1, 2),
		getRevUUID(c, s.DB(), uri2, 2),
		getRevUUID(c, s.DB(), uri3, 2),
		getRevUUID(c, s.DB(), uri4, 2),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		revID(uri1, 1),
		revID(uri2, 1),
		revID(uri4, 1),
	})

	// appOwners, unitOwners, revUUIDs(with unknown unit owned revisions).
	result, err = st.GetRevisionIDsForObsolete(ctx,
		[]string{
			"mysql",
			"mediawiki",
		},
		[]string{
			"mysql/0",
		},
		getRevUUID(c, s.DB(), uri1, 1),
		getRevUUID(c, s.DB(), uri2, 1),
		getRevUUID(c, s.DB(), uri3, 1),
		getRevUUID(c, s.DB(), uri4, 1),
		getRevUUID(c, s.DB(), uri1, 2),
		getRevUUID(c, s.DB(), uri2, 2),
		getRevUUID(c, s.DB(), uri3, 2),
		getRevUUID(c, s.DB(), uri4, 2),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		revID(uri1, 1),
		revID(uri2, 1),
		revID(uri3, 1),
	})

	// appOwners, unitOwners, revUUIDs(with part of the owned revisions).
	result, err = st.GetRevisionIDsForObsolete(ctx,
		[]string{
			"mysql",
			"mediawiki",
		},
		[]string{
			"mysql/0",
			"mediawiki/0",
		},
		getRevUUID(c, s.DB(), uri1, 1),
		getRevUUID(c, s.DB(), uri1, 2),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		revID(uri1, 1),
	})
}

func revID(uri *coresecrets.URI, rev int) string {
	return fmt.Sprintf("%s/%d", uri.ID, rev)
}

func (s *stateSuite) TestDeleteObsoleteUserSecretRevisions(c *tc.C) {
	s.setupUnits(c, "mysql")
	st := newSecretState(c, s.TxnRunnerFactory())

	uriUser1 := coresecrets.NewURI()
	uriUser2 := coresecrets.NewURI()
	uriUser3 := coresecrets.NewURI()
	uriCharm := coresecrets.NewURI()
	ctx := c.Context()
	data := coresecrets.SecretData{"foo": "bar", "hello": "world"}

	err := createUserSecret(ctx, st, 1, uriUser1, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       data,
	})
	c.Assert(err, tc.ErrorIsNil)
	err = createUserSecret(ctx, st, 1, uriUser2, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       data,
		AutoPrune:  ptr(true),
	})
	c.Assert(err, tc.ErrorIsNil)
	err = createUserSecret(ctx, st, 1, uriUser3, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       data,
		AutoPrune:  ptr(true),
	})
	c.Assert(err, tc.ErrorIsNil)
	err = createCharmApplicationSecret(ctx, st, 1, uriCharm, "mysql", domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       data,
	})
	c.Assert(err, tc.ErrorIsNil)

	sp := domainsecret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo-new": "bar-new"},
	}
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = updateSecret(c.Context(), st, uriUser1, sp)
	c.Assert(err, tc.ErrorIsNil)
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = updateSecret(c.Context(), st, uriUser2, sp)
	c.Assert(err, tc.ErrorIsNil)
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = updateSecret(c.Context(), st, uriCharm, sp)
	c.Assert(err, tc.ErrorIsNil)

	expectedToBeDeleted := []string{
		getRevUUID(c, s.DB(), uriUser2, 1),
	}
	deletedRevisionIDs, err := st.DeleteObsoleteUserSecretRevisions(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(deletedRevisionIDs, tc.SameContents, expectedToBeDeleted)

	assertRevision(c, s.DB(), uriUser1, 1, true)
	assertRevision(c, s.DB(), uriUser1, 2, true)
	assertRevision(c, s.DB(), uriUser2, 1, false)
	assertRevision(c, s.DB(), uriUser2, 2, true)
	assertRevision(c, s.DB(), uriUser3, 1, true)
	assertRevision(c, s.DB(), uriCharm, 1, true)
	assertRevision(c, s.DB(), uriCharm, 2, true)
}

func assertRevision(c *tc.C, db *sql.DB, uri *coresecrets.URI, rev int, exist bool) {
	var uuid string
	row := db.QueryRowContext(c.Context(), `
SELECT uuid
FROM secret_revision
WHERE secret_id = ? AND revision = ?
`, uri.ID, rev)
	err := row.Scan(&uuid)
	if exist {
		c.Assert(err, tc.ErrorIsNil)
	} else {
		c.Assert(err, tc.ErrorIs, sql.ErrNoRows)
	}
}

func (s *stateSuite) TestDeleteSomeRevisions(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	expireTime := time.Now().Add(2 * time.Hour)
	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		ExpireTime:  ptr(expireTime),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmApplicationSecret(ctx, st, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	data, ref, err := st.GetSecretValue(ctx, uri, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ref, tc.IsNil)
	c.Assert(data, tc.DeepEquals, coresecrets.SecretData{"foo": "bar"})

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar2"},
	}
	err = updateSecret(ctx, st, uri, sp2)
	c.Assert(err, tc.ErrorIsNil)
	sp3 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar3"},
	}
	err = updateSecret(ctx, st, uri, sp3)
	c.Assert(err, tc.ErrorIsNil)

	err = st.RunAtomic(c.Context(), func(ctx domain.AtomicContext) error {
		return st.DeleteSecret(ctx, uri, []int{2})
	})
	c.Assert(err, tc.ErrorIsNil)

	_, _, err = st.ListSecrets(ctx, uri, ptr(1), domainsecret.NilLabels)
	c.Assert(err, tc.ErrorIsNil)
	_, _, err = st.ListSecrets(ctx, uri, ptr(2), domainsecret.NilLabels)
	c.Assert(err, tc.ErrorIs, secreterrors.SecretRevisionNotFound)
	_, _, err = st.ListSecrets(ctx, uri, ptr(3), domainsecret.NilLabels)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestDeleteAllRevisionsFromNil(c *tc.C) {
	s.assertDeleteAllRevisions(c, nil)
}

func (s *stateSuite) TestDeleteAllRevisions(c *tc.C) {
	s.assertDeleteAllRevisions(c, []int{1, 2, 3})
}

func (s *stateSuite) assertDeleteAllRevisions(c *tc.C, revs []int) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	expireTime := time.Now().Add(2 * time.Hour)
	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar"},
		ExpireTime: ptr(expireTime),
	}
	uri := coresecrets.NewURI().WithSource(s.modelUUID)
	ctx := c.Context()
	err := createCharmApplicationSecret(ctx, st, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar2"},
	}
	err = updateSecret(ctx, st, uri, sp2)
	c.Assert(err, tc.ErrorIsNil)
	sp3 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar3"},
	}
	err = updateSecret(ctx, st, uri, sp3)
	c.Assert(err, tc.ErrorIsNil)

	consumer := &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	}
	err = st.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, tc.ErrorIsNil)
	err = st.SaveSecretRemoteConsumer(ctx, uri, "remote-app/0", consumer)
	c.Assert(err, tc.ErrorIsNil)

	uri2 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = createCharmApplicationSecret(ctx, st, 1, uri2, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	err = st.RunAtomic(c.Context(), func(ctx domain.AtomicContext) error {
		return st.DeleteSecret(ctx, uri, revs)
	})
	c.Assert(err, tc.ErrorIsNil)

	for r := 1; r <= 3; r++ {
		_, _, err := st.ListSecrets(ctx, uri, ptr(r), domainsecret.NilLabels)
		c.Assert(err, tc.ErrorIs, secreterrors.SecretRevisionNotFound)
	}
	_, err = st.GetSecret(ctx, uri)
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
	_, _, err = st.GetSecretConsumer(ctx, uri, "someunit/0")
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)

	_, err = st.GetSecret(ctx, uri2)
	c.Assert(err, tc.ErrorIsNil)
	data, _, err := st.GetSecretValue(ctx, uri2, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(data, tc.DeepEquals, coresecrets.SecretData{"foo": "bar"})
}

func (s *stateSuite) TestGetSecretRevisionID(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	expireTime := time.Now().Add(2 * time.Hour)
	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar"},
		ExpireTime: ptr(expireTime),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := createCharmApplicationSecret(ctx, st, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	result, err := st.GetSecretRevisionID(ctx, uri, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, *sp.RevisionID)
}

func (s *stateSuite) TestGetSecretRevisionIDNotFound(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	uri := coresecrets.NewURI()
	ctx := c.Context()

	_, err := st.GetSecretRevisionID(ctx, uri, 1)
	c.Assert(err, tc.ErrorIs, secreterrors.SecretRevisionNotFound)
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf("secret revision not found: %s/%d", uri, 1))
}

func parseUUID(c *tc.C, s string) uuid.UUID {
	id, err := uuid.UUIDFromString(s)
	c.Assert(err, tc.ErrorIsNil)
	return id
}

func (s *stateSuite) prepareWatchForConsumedSecrets(c *tc.C, ctx context.Context, st *State) (*coresecrets.URI, *coresecrets.URI) {
	s.setupUnits(c, "mysql")
	s.setupUnits(c, "mediawiki")

	saveConsumer := func(uri *coresecrets.URI, revision int, consumerID string) {
		consumer := &coresecrets.SecretConsumerMetadata{
			CurrentRevision: revision,
		}
		unitName := unittesting.GenNewName(c, consumerID)
		err := st.SaveSecretConsumer(ctx, uri, unitName, consumer)
		c.Assert(err, tc.ErrorIsNil)
	}

	sp := domainsecret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	uri1 := coresecrets.NewURI()
	err := createCharmApplicationSecret(ctx, st, 1, uri1, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	uri2 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = createCharmApplicationSecret(ctx, st, 1, uri2, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	// The consumed revision 1.
	saveConsumer(uri1, 1, "mediawiki/0")
	// The consumed revision 1.
	saveConsumer(uri2, 1, "mediawiki/0")

	// create revision 2, so mediawiki/0 will receive a consumed secret change event for uri1.
	updateSecretContent(c, st, uri1)
	return uri1, uri2
}

func (s *stateSuite) TestInitialWatchStatementForConsumedSecrets(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := c.Context()
	uri1, _ := s.prepareWatchForConsumedSecrets(c, ctx, st)
	tableName, f := st.InitialWatchStatementForConsumedSecretsChange("mediawiki/0")

	c.Assert(tableName, tc.Equals, "secret_revision")
	consumerIDs, err := f(ctx, s.TxnRunner())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(consumerIDs, tc.SameContents, []string{
		getRevUUID(c, s.DB(), uri1, 2),
	})
}

func (s *stateSuite) TestGetConsumedSecretURIsWithChanges(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := c.Context()
	uri1, uri2 := s.prepareWatchForConsumedSecrets(c, ctx, st)

	result, err := st.GetConsumedSecretURIsWithChanges(ctx, "mediawiki/0",
		getRevUUID(c, s.DB(), uri1, 1),
		getRevUUID(c, s.DB(), uri1, 2),
		getRevUUID(c, s.DB(), uri2, 1),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Assert(result, tc.SameContents, []string{
		uri1.String(),
	})
}

func (s *stateSuite) prepareWatchForRemoteConsumedSecrets(c *tc.C, ctx context.Context, st *State) (*coresecrets.URI, *coresecrets.URI) {
	s.setupUnits(c, "mediawiki")

	saveConsumer := func(uri *coresecrets.URI, revision int, consumerID string) {
		consumer := &coresecrets.SecretConsumerMetadata{
			CurrentRevision: revision,
		}
		unitName := unittesting.GenNewName(c, consumerID)
		err := st.SaveSecretConsumer(ctx, uri, unitName, consumer)
		c.Assert(err, tc.ErrorIsNil)
	}

	sourceModelUUID := uuid.MustNewUUID()
	uri1 := coresecrets.NewURI()
	uri1.SourceUUID = sourceModelUUID.String()

	uri2 := coresecrets.NewURI()
	uri2.SourceUUID = sourceModelUUID.String()

	// The consumed revision 1.
	saveConsumer(uri1, 1, "mediawiki/0")
	// The consumed revision 1.
	saveConsumer(uri2, 1, "mediawiki/0")

	err := st.UpdateRemoteSecretRevision(ctx, uri1, 2)
	c.Assert(err, tc.ErrorIsNil)
	return uri1, uri2
}

func (s *stateSuite) TestInitialWatchStatementForConsumedRemoteSecretsChange(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := c.Context()
	uri1, _ := s.prepareWatchForRemoteConsumedSecrets(c, ctx, st)

	tableName, f := st.InitialWatchStatementForConsumedRemoteSecretsChange("mediawiki/0")
	c.Assert(tableName, tc.Equals, "secret_reference")
	result, err := f(ctx, s.TxnRunner())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []string{
		uri1.ID,
	})
}

func (s *stateSuite) TestGetConsumedRemoteSecretURIsWithChanges(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := c.Context()
	uri1, uri2 := s.prepareWatchForRemoteConsumedSecrets(c, ctx, st)

	result, err := st.GetConsumedRemoteSecretURIsWithChanges(ctx, "mediawiki/0",
		uri1.ID,
		uri2.ID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Assert(result, tc.SameContents, []string{
		uri1.String(),
	})
}

func (s *stateSuite) prepareWatchForRemoteConsumedSecretsChangesFromOfferingSide(c *tc.C, ctx context.Context, st *State) (*coresecrets.URI, *coresecrets.URI) {
	s.setupUnits(c, "mysql")

	saveRemoteConsumer := func(uri *coresecrets.URI, revision int, consumerID string) {
		consumer := &coresecrets.SecretConsumerMetadata{
			CurrentRevision: revision,
		}
		unitName := unittesting.GenNewName(c, consumerID)
		err := st.SaveSecretRemoteConsumer(ctx, uri, unitName, consumer)
		c.Assert(err, tc.ErrorIsNil)
	}

	sp := domainsecret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri1 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err := createCharmApplicationSecret(ctx, st, 1, uri1, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)
	uri1.SourceUUID = s.modelUUID

	uri2 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = createCharmApplicationSecret(ctx, st, 1, uri2, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)
	uri2.SourceUUID = s.modelUUID

	// The consumed revision 1.
	saveRemoteConsumer(uri1, 1, "mediawiki/0")
	// The consumed revision 1.
	saveRemoteConsumer(uri2, 1, "mediawiki/0")

	// create revision 2.
	updateSecretContent(c, st, uri1)

	err = st.UpdateRemoteSecretRevision(ctx, uri1, 2)
	c.Assert(err, tc.ErrorIsNil)
	return uri1, uri2
}

func (s *stateSuite) TestInitialWatchStatementForRemoteConsumedSecretsChangesFromOfferingSide(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := c.Context()
	uri1, _ := s.prepareWatchForRemoteConsumedSecretsChangesFromOfferingSide(c, ctx, st)

	tableName, f := st.InitialWatchStatementForRemoteConsumedSecretsChangesFromOfferingSide("mediawiki")
	c.Assert(tableName, tc.Equals, "secret_revision")
	result, err := f(ctx, s.TxnRunner())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []string{
		getRevUUID(c, s.DB(), uri1, 2),
	})
}

func (s *stateSuite) TestGetRemoteConsumedSecretURIsWithChangesFromOfferingSide(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := c.Context()
	uri1, uri2 := s.prepareWatchForRemoteConsumedSecretsChangesFromOfferingSide(c, ctx, st)

	result, err := st.GetRemoteConsumedSecretURIsWithChangesFromOfferingSide(ctx, "mediawiki",
		getRevUUID(c, s.DB(), uri1, 1),
		getRevUUID(c, s.DB(), uri1, 2),
		getRevUUID(c, s.DB(), uri2, 1),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Assert(result, tc.SameContents, []string{
		uri1.String(),
	})
}

func (s *stateSuite) prepareWatchForWatchStatementForSecretsRotationChanges(c *tc.C, ctx context.Context, st *State) (time.Time, *coresecrets.URI, *coresecrets.URI) {
	s.setupUnits(c, "mysql")
	s.setupUnits(c, "mediawiki")

	sp := domainsecret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri1 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err := createCharmApplicationSecret(ctx, st, 1, uri1, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	uri2 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = createCharmUnitSecret(ctx, st, 1, uri2, "mediawiki/0", sp)
	c.Assert(err, tc.ErrorIsNil)
	updateSecretContent(c, st, uri2)

	now := time.Now()
	err = st.SecretRotated(ctx, uri1, now.Add(1*time.Hour))
	c.Assert(err, tc.ErrorIsNil)
	err = st.SecretRotated(ctx, uri2, now.Add(2*time.Hour))
	c.Assert(err, tc.ErrorIsNil)

	return now, uri1, uri2
}

func (s *stateSuite) TestInitialWatchStatementForSecretsRotationChanges(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := c.Context()
	_, uri1, uri2 := s.prepareWatchForWatchStatementForSecretsRotationChanges(c, ctx, st)

	tableName, f := st.InitialWatchStatementForSecretsRotationChanges(domainsecret.ApplicationOwners{"mysql"}, domainsecret.UnitOwners{"mediawiki/0"})
	c.Check(tableName, tc.Equals, "secret_rotation")
	result, err := f(ctx, s.TxnRunner())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		uri1.ID, uri2.ID,
	})

	tableName, f = st.InitialWatchStatementForSecretsRotationChanges(domainsecret.ApplicationOwners{"mysql"}, nil)
	c.Check(tableName, tc.Equals, "secret_rotation")
	result, err = f(ctx, s.TxnRunner())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		uri1.ID,
	})

	tableName, f = st.InitialWatchStatementForSecretsRotationChanges(nil, domainsecret.UnitOwners{"mediawiki/0"})
	c.Check(tableName, tc.Equals, "secret_rotation")
	result, err = f(ctx, s.TxnRunner())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		uri2.ID,
	})

	tableName, f = st.InitialWatchStatementForSecretsRotationChanges(nil, nil)
	c.Check(tableName, tc.Equals, "secret_rotation")
	result, err = f(ctx, s.TxnRunner())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

func (s *stateSuite) TestGetSecretsRotationChanges(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := c.Context()
	now, uri1, uri2 := s.prepareWatchForWatchStatementForSecretsRotationChanges(c, ctx, st)

	result, err := st.GetSecretsRotationChanges(ctx, domainsecret.ApplicationOwners{"mysql"}, domainsecret.UnitOwners{"mediawiki/0"})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []domainsecret.RotationInfo{
		{
			URI:             uri1,
			Revision:        1,
			NextTriggerTime: now.Add(1 * time.Hour).UTC(),
		},
		{
			URI:             uri2,
			Revision:        2,
			NextTriggerTime: now.Add(2 * time.Hour).UTC(),
		},
	})

	result, err = st.GetSecretsRotationChanges(ctx,
		domainsecret.ApplicationOwners{"mysql", "mediawiki"}, domainsecret.UnitOwners{"mysql/0", "mediawiki/0"},
		uri1.ID,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []domainsecret.RotationInfo{
		{
			URI:             uri1,
			Revision:        1,
			NextTriggerTime: now.Add(1 * time.Hour).UTC(),
		},
	})

	result, err = st.GetSecretsRotationChanges(ctx,
		domainsecret.ApplicationOwners{"mysql", "mediawiki"}, domainsecret.UnitOwners{"mysql/0", "mediawiki/0"},
		uri2.ID,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []domainsecret.RotationInfo{
		{
			URI:             uri2,
			Revision:        2,
			NextTriggerTime: now.Add(2 * time.Hour).UTC(),
		},
	})

	result, err = st.GetSecretsRotationChanges(ctx, domainsecret.ApplicationOwners{"mysql"}, nil)
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []domainsecret.RotationInfo{
		{
			URI:             uri1,
			Revision:        1,
			NextTriggerTime: now.Add(1 * time.Hour).UTC(),
		},
	})

	// The uri2 is not owned by mysql, so it should not be returned.
	result, err = st.GetSecretsRotationChanges(ctx, domainsecret.ApplicationOwners{"mysql"}, nil, uri1.ID, uri2.ID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []domainsecret.RotationInfo{
		{
			URI:             uri1,
			Revision:        1,
			NextTriggerTime: now.Add(1 * time.Hour).UTC(),
		},
	})

	result, err = st.GetSecretsRotationChanges(ctx, nil, domainsecret.UnitOwners{"mediawiki/0"})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []domainsecret.RotationInfo{
		{
			URI:             uri2,
			Revision:        2,
			NextTriggerTime: now.Add(2 * time.Hour).UTC(),
		},
	})

	// The uri1 is not owned by mediawiki/0, so it should not be returned.
	result, err = st.GetSecretsRotationChanges(ctx, nil, domainsecret.UnitOwners{"mediawiki/0"}, uri1.ID, uri2.ID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []domainsecret.RotationInfo{
		{
			URI:             uri2,
			Revision:        2,
			NextTriggerTime: now.Add(2 * time.Hour).UTC(),
		},
	})

	result, err = st.GetSecretsRotationChanges(ctx, nil, nil)
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

func (s *stateSuite) prepareWatchForWatchStatementForSecretsRevisionExpiryChanges(c *tc.C, ctx context.Context, st *State) (time.Time, *coresecrets.URI, *coresecrets.URI) {
	s.setupUnits(c, "mysql")
	s.setupUnits(c, "mediawiki")

	now := time.Now()
	uri1 := coresecrets.NewURI()
	err := createCharmApplicationSecret(ctx, st, 1, uri1, "mysql", domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar", "hello": "world"},
		ExpireTime: ptr(now.Add(1 * time.Hour)),
	})
	c.Assert(err, tc.ErrorIsNil)

	uri2 := coresecrets.NewURI()
	err = createCharmUnitSecret(ctx, st, 1, uri2, "mediawiki/0", domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar", "hello": "world"},
	})
	c.Assert(err, tc.ErrorIsNil)
	err = updateSecret(c.Context(), st, uri2, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo-new": "bar-new"},
		ExpireTime: ptr(now.Add(2 * time.Hour)),
	})
	c.Assert(err, tc.ErrorIsNil)
	return now, uri1, uri2
}

func (s *stateSuite) TestInitialWatchStatementForSecretsRevisionExpiryChanges(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := c.Context()
	_, uri1, uri2 := s.prepareWatchForWatchStatementForSecretsRevisionExpiryChanges(c, ctx, st)

	tableName, f := st.InitialWatchStatementForSecretsRevisionExpiryChanges(domainsecret.ApplicationOwners{"mysql"}, domainsecret.UnitOwners{"mediawiki/0"})
	c.Check(tableName, tc.Equals, "secret_revision_expire")
	result, err := f(ctx, s.TxnRunner())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		getRevUUID(c, s.DB(), uri1, 1),
		getRevUUID(c, s.DB(), uri2, 2),
	})

	tableName, f = st.InitialWatchStatementForSecretsRevisionExpiryChanges(domainsecret.ApplicationOwners{"mysql"}, nil)
	c.Check(tableName, tc.Equals, "secret_revision_expire")
	result, err = f(ctx, s.TxnRunner())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		getRevUUID(c, s.DB(), uri1, 1),
	})

	tableName, f = st.InitialWatchStatementForSecretsRevisionExpiryChanges(nil, domainsecret.UnitOwners{"mediawiki/0"})
	c.Check(tableName, tc.Equals, "secret_revision_expire")
	result, err = f(ctx, s.TxnRunner())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		getRevUUID(c, s.DB(), uri2, 2),
	})

	tableName, f = st.InitialWatchStatementForSecretsRevisionExpiryChanges(nil, nil)
	c.Check(tableName, tc.Equals, "secret_revision_expire")
	result, err = f(ctx, s.TxnRunner())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

func (s *stateSuite) TestGetSecretsRevisionExpiryChanges(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := c.Context()
	now, uri1, uri2 := s.prepareWatchForWatchStatementForSecretsRevisionExpiryChanges(c, ctx, st)

	result, err := st.GetSecretsRevisionExpiryChanges(ctx, domainsecret.ApplicationOwners{"mysql"}, domainsecret.UnitOwners{"mediawiki/0"})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []domainsecret.ExpiryInfo{
		{
			URI:             uri1,
			Revision:        1,
			RevisionID:      getRevUUID(c, s.DB(), uri1, 1),
			NextTriggerTime: now.Add(1 * time.Hour).UTC(),
		},
		{
			URI:             uri2,
			Revision:        2,
			RevisionID:      getRevUUID(c, s.DB(), uri2, 2),
			NextTriggerTime: now.Add(2 * time.Hour).UTC(),
		},
	})

	result, err = st.GetSecretsRevisionExpiryChanges(ctx,
		domainsecret.ApplicationOwners{"mysql", "mediawiki"}, domainsecret.UnitOwners{"mysql/0", "mediawiki/0"},
		getRevUUID(c, s.DB(), uri1, 1),
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []domainsecret.ExpiryInfo{
		{
			URI:             uri1,
			Revision:        1,
			RevisionID:      getRevUUID(c, s.DB(), uri1, 1),
			NextTriggerTime: now.Add(1 * time.Hour).UTC(),
		},
	})

	result, err = st.GetSecretsRevisionExpiryChanges(ctx,
		domainsecret.ApplicationOwners{"mysql", "mediawiki"}, domainsecret.UnitOwners{"mysql/0", "mediawiki/0"},
		getRevUUID(c, s.DB(), uri2, 2),
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []domainsecret.ExpiryInfo{
		{
			URI:             uri2,
			Revision:        2,
			RevisionID:      getRevUUID(c, s.DB(), uri2, 2),
			NextTriggerTime: now.Add(2 * time.Hour).UTC(),
		},
	})

	result, err = st.GetSecretsRevisionExpiryChanges(ctx, domainsecret.ApplicationOwners{"mysql"}, nil)
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []domainsecret.ExpiryInfo{
		{
			URI:             uri1,
			Revision:        1,
			RevisionID:      getRevUUID(c, s.DB(), uri1, 1),
			NextTriggerTime: now.Add(1 * time.Hour).UTC(),
		},
	})

	// The uri2 is not owned by mysql, so it should not be returned.
	result, err = st.GetSecretsRevisionExpiryChanges(ctx, domainsecret.ApplicationOwners{"mysql"}, nil,
		getRevUUID(c, s.DB(), uri1, 1),
		getRevUUID(c, s.DB(), uri2, 2),
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []domainsecret.ExpiryInfo{
		{
			URI:             uri1,
			Revision:        1,
			RevisionID:      getRevUUID(c, s.DB(), uri1, 1),
			NextTriggerTime: now.Add(1 * time.Hour).UTC(),
		},
	})

	result, err = st.GetSecretsRevisionExpiryChanges(ctx, nil, domainsecret.UnitOwners{"mediawiki/0"})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []domainsecret.ExpiryInfo{
		{
			URI:             uri2,
			Revision:        2,
			RevisionID:      getRevUUID(c, s.DB(), uri2, 2),
			NextTriggerTime: now.Add(2 * time.Hour).UTC(),
		},
	})

	// The uri1 is not owned by mediawiki/0, so it should not be returned.
	result, err = st.GetSecretsRevisionExpiryChanges(ctx, nil, domainsecret.UnitOwners{"mediawiki/0"},
		getRevUUID(c, s.DB(), uri1, 1),
		getRevUUID(c, s.DB(), uri2, 2),
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []domainsecret.ExpiryInfo{
		{
			URI:             uri2,
			Revision:        2,
			RevisionID:      getRevUUID(c, s.DB(), uri2, 2),
			NextTriggerTime: now.Add(2 * time.Hour).UTC(),
		},
	})

	result, err = st.GetSecretsRevisionExpiryChanges(ctx, nil, nil)
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

func (s *stateSuite) TestSecretRotated(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := c.Context()

	s.setupUnits(c, "mysql")
	uri := coresecrets.NewURI()
	err := createCharmApplicationSecret(ctx, st, 1, uri, "mysql", domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar", "hello": "world"},
	})
	c.Assert(err, tc.ErrorIsNil)

	next := time.Now().Add(1 * time.Hour)
	err = st.SecretRotated(ctx, uri, next)
	c.Assert(err, tc.ErrorIsNil)

	row := s.DB().QueryRowContext(c.Context(), `
SELECT next_rotation_time
FROM secret_rotation
WHERE secret_id = ?`, uri.ID)
	var nextRotationTime time.Time
	err = row.Scan(&nextRotationTime)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(nextRotationTime.Equal(next), tc.IsTrue)
}

func (s *stateSuite) TestGetObsoleteUserSecretRevisionsReadyToPrune(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	ctx := c.Context()
	uri := coresecrets.NewURI()

	err := createUserSecret(ctx, st, 1, uri, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar", "hello": "world"},
	})
	c.Assert(err, tc.ErrorIsNil)

	// The secret is not obsolete yet.
	result, err := st.GetObsoleteUserSecretRevisionsReadyToPrune(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 0)

	// create revision 2 for user secret.
	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo-new": "bar-new"},
	}
	err = updateSecret(c.Context(), st, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	result, err = st.GetObsoleteUserSecretRevisionsReadyToPrune(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 0)

	sp = domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		AutoPrune:  ptr(true),
	}
	err = updateSecret(c.Context(), st, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	result, err = st.GetObsoleteUserSecretRevisionsReadyToPrune(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []string{uri.ID + "/1"})
}

func (s *stateSuite) TestChangeSecretBackend(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := c.Context()

	s.setupUnits(c, "mysql")
	uriCharm := coresecrets.NewURI()
	uriUser := coresecrets.NewURI()

	dataInput := coresecrets.SecretData{"foo": "bar", "hello": "world"}
	valueRefInput := &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "revision-id",
	}

	err := createCharmApplicationSecret(ctx, st, 1, uriCharm, "mysql", domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       dataInput,
	})
	c.Assert(err, tc.ErrorIsNil)
	data, valueRef, err := st.GetSecretValue(ctx, uriCharm, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(data, tc.DeepEquals, dataInput)
	c.Assert(valueRef, tc.IsNil)

	err = createUserSecret(ctx, st, 1, uriUser, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       dataInput,
	})
	c.Assert(err, tc.ErrorIsNil)
	data, valueRef, err = st.GetSecretValue(ctx, uriUser, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(data, tc.DeepEquals, dataInput)
	c.Assert(valueRef, tc.IsNil)

	// change to external backend.
	err = st.ChangeSecretBackend(ctx, parseUUID(c, getRevUUID(c, s.DB(), uriCharm, 1)), valueRefInput, nil)
	c.Assert(err, tc.ErrorIsNil)
	data, valueRef, err = st.GetSecretValue(ctx, uriCharm, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(data, tc.IsNil)
	c.Assert(valueRef, tc.DeepEquals, valueRefInput)

	// change back to internal backend.
	err = st.ChangeSecretBackend(ctx, parseUUID(c, getRevUUID(c, s.DB(), uriCharm, 1)), nil, dataInput)
	c.Assert(err, tc.ErrorIsNil)
	data, valueRef, err = st.GetSecretValue(ctx, uriCharm, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(data, tc.DeepEquals, dataInput)
	c.Assert(valueRef, tc.IsNil)

	// change to external backend for the user secret.
	err = st.ChangeSecretBackend(ctx, parseUUID(c, getRevUUID(c, s.DB(), uriUser, 1)), valueRefInput, nil)
	c.Assert(err, tc.ErrorIsNil)
	data, valueRef, err = st.GetSecretValue(ctx, uriUser, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(data, tc.IsNil)
	c.Assert(valueRef, tc.DeepEquals, valueRefInput)

	// change back to internal backend for the user secret.
	err = st.ChangeSecretBackend(ctx, parseUUID(c, getRevUUID(c, s.DB(), uriUser, 1)), nil, dataInput)
	c.Assert(err, tc.ErrorIsNil)
	data, valueRef, err = st.GetSecretValue(ctx, uriUser, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(data, tc.DeepEquals, dataInput)
	c.Assert(valueRef, tc.IsNil)
}

func (s *stateSuite) TestChangeSecretBackendFailed(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := c.Context()

	s.setupUnits(c, "mysql")

	dataInput := coresecrets.SecretData{"foo": "bar", "hello": "world"}
	valueRefInput := &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "revision-id",
	}

	err := st.ChangeSecretBackend(ctx, uuid.MustNewUUID(), nil, nil)
	c.Assert(err, tc.ErrorMatches, "either valueRef or data must be set")
	err = st.ChangeSecretBackend(ctx, uuid.MustNewUUID(), valueRefInput, dataInput)
	c.Assert(err, tc.ErrorMatches, "both valueRef and data cannot be set")
}

func (s *stateSuite) TestInitialWatchStatementForSecretMatadataGetForBothAppAndUnitOwners(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := c.Context()

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri1 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err := createCharmApplicationSecret(ctx, st, 1, uri1, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	uri2 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = createCharmUnitSecret(ctx, st, 1, uri2, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)
	updateSecretContent(c, st, uri2)

	tableName, f := st.InitialWatchStatementForOwnedSecrets(domainsecret.ApplicationOwners{"mysql"}, domainsecret.UnitOwners{"mysql/0"})
	c.Check(tableName, tc.Equals, "secret_metadata")
	result, err := f(ctx, s.TxnRunner())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		uri1.ID,
		uri2.ID,
	})
}

func (s *stateSuite) TestInitialWatchStatementForSecretMatadataGetForAppOwners(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := c.Context()

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri1 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err := createCharmApplicationSecret(ctx, st, 1, uri1, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	uri2 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = createCharmUnitSecret(ctx, st, 1, uri2, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)
	updateSecretContent(c, st, uri2)

	tableName, f := st.InitialWatchStatementForOwnedSecrets(domainsecret.ApplicationOwners{"mysql"}, nil)
	c.Check(tableName, tc.Equals, "secret_metadata")
	result, err := f(ctx, s.TxnRunner())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		uri1.ID,
	})

}

func (s *stateSuite) TestInitialWatchStatementForSecretMatadataGetForUnitOwners(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := c.Context()

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri1 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err := createCharmApplicationSecret(ctx, st, 1, uri1, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	uri2 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = createCharmUnitSecret(ctx, st, 1, uri2, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)
	updateSecretContent(c, st, uri2)

	tableName, f := st.InitialWatchStatementForOwnedSecrets(nil, domainsecret.UnitOwners{"mysql/0"})
	c.Check(tableName, tc.Equals, "secret_metadata")
	result, err := f(ctx, s.TxnRunner())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		uri2.ID,
	})
}

func (s *stateSuite) TestIsSecretOwnedBy(c *tc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := c.Context()

	s.setupUnits(c, "mysql")
	s.setupUnits(c, "mediawiki")

	sp := domainsecret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri1 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err := createCharmApplicationSecret(ctx, st, 1, uri1, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	uri2 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = createCharmUnitSecret(ctx, st, 1, uri2, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)
	updateSecretContent(c, st, uri2)

	// uri1 is owned by application mysql.
	owned, err := st.IsSecretOwnedBy(ctx, uri1, domainsecret.ApplicationOwners{"mysql"}, nil)
	c.Check(err, tc.ErrorIsNil)
	c.Check(owned, tc.IsTrue)

	// uri1 is not owned by the unit mysql/0.
	owned, err = st.IsSecretOwnedBy(ctx, uri1, nil, domainsecret.UnitOwners{"mysql/0"})
	c.Check(err, tc.ErrorIsNil)
	c.Check(owned, tc.IsFalse)

	// uri1 is not owned by the unit mediawiki/0 and application mediawiki.
	owned, err = st.IsSecretOwnedBy(ctx, uri1, domainsecret.ApplicationOwners{"mediawiki"}, domainsecret.UnitOwners{"mediawiki/0"})
	c.Check(err, tc.ErrorIsNil)
	c.Check(owned, tc.IsFalse)

	// uri2 is owned by the unit mysql/0.
	owned, err = st.IsSecretOwnedBy(ctx, uri2, nil, domainsecret.UnitOwners{"mysql/0"})
	c.Check(err, tc.ErrorIsNil)
	c.Check(owned, tc.IsTrue)

	// uri2 is not owned by the application mysql.
	owned, err = st.IsSecretOwnedBy(ctx, uri2, domainsecret.ApplicationOwners{"mysql"}, nil)
	c.Check(err, tc.ErrorIsNil)
	c.Check(owned, tc.IsFalse)

	// uri2 is not owned by the application mediawiki and unit mediawiki/0.
	owned, err = st.IsSecretOwnedBy(ctx, uri2, domainsecret.ApplicationOwners{"mediawiki"}, domainsecret.UnitOwners{"mediawiki/0"})
	c.Check(err, tc.ErrorIsNil)
	c.Check(owned, tc.IsFalse)

}
