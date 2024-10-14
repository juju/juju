// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coredatabase "github.com/juju/juju/core/database"
	coresecrets "github.com/juju/juju/core/secrets"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/schema/testing"
	domainsecret "github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	testing.ModelSuite

	modelUUID string
}

var _ = gc.Suite(&stateSuite{})

func newSecretState(c *gc.C, factory coredatabase.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    loggertesting.WrapCheckLog(c),
	}
}

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)
	s.modelUUID = s.setupModel(c)
}

func (s *stateSuite) setupModel(c *gc.C) string {
	modelUUID := uuid.MustNewUUID()
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO model (uuid, controller_uuid, target_agent_version, name, type, cloud, cloud_type)
VALUES (?, ?, ?, "test", "iaas", "fluffy", "ec2")
		`, modelUUID.String(), coretesting.ControllerTag.Id(), jujuversion.Current.String())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	return modelUUID.String()
}

func (s *stateSuite) TestGetModelUUID(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	got, err := st.GetModelUUID(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.Equals, s.modelUUID)
}

func (s *stateSuite) TestGetSecretNotFound(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	_, err := st.GetSecret(context.Background(), coresecrets.NewURI())
	c.Assert(err, jc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGetLatestRevisionNotFound(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	_, err := st.GetLatestRevision(context.Background(), coresecrets.NewURI())
	c.Assert(err, jc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGetLatestRevision(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)
	err = st.UpdateSecret(ctx, uri, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar1"},
	})
	c.Assert(err, jc.ErrorIsNil)
	latest, err := st.GetLatestRevision(ctx, uri)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(latest, gc.Equals, 2)
}

func (s *stateSuite) TestGetRotatePolicy(c *gc.C) {
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
	ctx := context.Background()
	err := st.CreateCharmApplicationSecret(ctx, 1, uri, "mysql", sp)
	c.Assert(err, jc.ErrorIsNil)

	result, err := st.GetRotatePolicy(context.Background(), uri)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, coresecrets.RotateYearly)
}

func (s *stateSuite) TestGetRotatePolicyNotFound(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	_, err := st.GetRotatePolicy(context.Background(), coresecrets.NewURI())
	c.Assert(err, jc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGetRotationExpiryInfo(c *gc.C) {
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
	ctx := context.Background()
	err := st.CreateCharmApplicationSecret(ctx, 1, uri, "mysql", sp)
	c.Assert(err, jc.ErrorIsNil)

	result, err := st.GetRotationExpiryInfo(context.Background(), uri)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, &domainsecret.RotationExpiryInfo{
		RotatePolicy:     coresecrets.RotateYearly,
		LatestExpireTime: ptr(expireTime.UTC()),
		NextRotateTime:   ptr(rotateTime.UTC()),
		LatestRevision:   1,
	})

	newExpireTime := expireTime.Add(2 * time.Hour)
	err = st.UpdateSecret(ctx, uri, domainsecret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo": "bar1"},
		ExpireTime: ptr(newExpireTime),
		RevisionID: ptr(uuid.MustNewUUID().String()),
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err = st.GetRotationExpiryInfo(context.Background(), uri)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, &domainsecret.RotationExpiryInfo{
		RotatePolicy:     coresecrets.RotateYearly,
		LatestExpireTime: ptr(newExpireTime.UTC()),
		NextRotateTime:   ptr(rotateTime.UTC()),
		LatestRevision:   2,
	})
}

func (s *stateSuite) TestGetRotationExpiryInfoNotFound(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	_, err := st.GetRotationExpiryInfo(context.Background(), coresecrets.NewURI())
	c.Assert(err, jc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGetSecretRevisionNotFound(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	_, _, err := st.GetSecretValue(context.Background(), coresecrets.NewURI(), 666)
	c.Assert(err, jc.ErrorIs, secreterrors.SecretRevisionNotFound)
}

func (s *stateSuite) TestCreateUserSecretLabelAlreadyExists(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   ptr(true),
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
}

func (s *stateSuite) TestCreateUserSecretFailedRevisionIDMissing(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   ptr(true),
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, gc.ErrorMatches, `*.revision ID must be provided`)
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

func (s *stateSuite) assertSecret(c *gc.C, st *State, uri *coresecrets.URI, sp domainsecret.UpsertSecretParams, revision int, owner coresecrets.Owner) {
	ctx := context.Background()
	md, revs, err := st.ListSecrets(ctx, uri, &revision, domainsecret.NilLabels)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md, gc.HasLen, 1)
	c.Assert(md[0].Version, gc.Equals, 1)
	c.Assert(md[0].Label, gc.Equals, value(sp.Label))
	c.Assert(md[0].Description, gc.Equals, value(sp.Description))
	c.Assert(md[0].LatestRevision, gc.Equals, 1)
	c.Assert(md[0].AutoPrune, gc.Equals, value(sp.AutoPrune))
	c.Assert(md[0].Owner, jc.DeepEquals, owner)
	if sp.RotatePolicy == nil {
		c.Assert(md[0].RotatePolicy, gc.Equals, coresecrets.RotateNever)
	} else {
		c.Assert(md[0].RotatePolicy, gc.Equals, fromDbRotatePolicy(*sp.RotatePolicy))
	}
	if sp.NextRotateTime == nil {
		c.Assert(md[0].NextRotateTime, gc.IsNil)
	} else {
		c.Assert(*md[0].NextRotateTime, gc.Equals, sp.NextRotateTime.UTC())
	}
	now := time.Now()
	c.Assert(md[0].CreateTime, jc.Almost, now)
	c.Assert(md[0].UpdateTime, jc.Almost, now)

	c.Assert(revs, gc.HasLen, 1)
	c.Assert(revs[0], gc.HasLen, 1)
	rev := revs[0][0]
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rev.Revision, gc.Equals, revision)
	c.Assert(rev.CreateTime, jc.Almost, now)
	if rev.ExpireTime == nil {
		c.Assert(md[0].LatestExpireTime, gc.IsNil)
	} else {
		c.Assert(*md[0].LatestExpireTime, gc.Equals, rev.ExpireTime.UTC())
	}
}

func (s *stateSuite) TestCreateUserSecretWithContent(c *gc.C) {
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
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)
	owner := coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: s.modelUUID}
	s.assertSecret(c, st, uri, sp, 1, owner)
	data, ref, err := st.GetSecretValue(ctx, uri, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ref, gc.IsNil)
	c.Assert(data, jc.DeepEquals, coresecrets.SecretData{"foo": "bar"})

	ap := domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectModel,
		SubjectID:     s.modelUUID,
	}
	access, err := st.GetSecretAccess(ctx, uri, ap)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, "manage")
}

func (s *stateSuite) TestCreateManyUserSecretsNoLabelClash(c *gc.C) {
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
		ctx := context.Background()
		err := st.CreateUserSecret(ctx, 1, uri, sp)
		c.Assert(err, jc.ErrorIsNil)
		owner := coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: s.modelUUID}
		s.assertSecret(c, st, uri, sp, 1, owner)
		data, ref, err := st.GetSecretValue(ctx, uri, 1)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ref, gc.IsNil)
		c.Assert(data, jc.DeepEquals, coresecrets.SecretData{"foo": content})
	}
	createAndCheck("my label")
	createAndCheck("")
	createAndCheck("")
	createAndCheck("another label")
}

func (s *stateSuite) TestCreateUserSecretWithValueReference(c *gc.C) {
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
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)
	owner := coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: s.modelUUID}
	s.assertSecret(c, st, uri, sp, 1, owner)
	data, ref, err := st.GetSecretValue(ctx, uri, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, gc.HasLen, 0)
	c.Assert(ref, jc.DeepEquals, &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"})
}

func (s *stateSuite) TestListExternalSecretRevisions(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	ctx := context.Background()
	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		ValueRef:    &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		ValueRef:    &coresecrets.ValueRef{BackendID: "some-backend2", RevisionID: "some-revision2"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	err = st.UpdateSecret(ctx, uri, sp2)
	c.Assert(err, jc.ErrorIsNil)

	sp3 := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri3 := coresecrets.NewURI()
	err = st.CreateUserSecret(ctx, 1, uri3, sp3)
	c.Assert(err, jc.ErrorIsNil)

	var refs []coresecrets.ValueRef
	err = st.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		refs, err = st.ListExternalSecretRevisions(ctx, uri)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(refs, jc.SameContents, []coresecrets.ValueRef{{
		BackendID:  "some-backend",
		RevisionID: "some-revision",
	}, {
		BackendID:  "some-backend2",
		RevisionID: "some-revision2",
	}})

	err = st.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		refs, err = st.ListExternalSecretRevisions(ctx, uri, 2)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(refs, jc.SameContents, []coresecrets.ValueRef{{
		BackendID:  "some-backend2",
		RevisionID: "some-revision2",
	}})

}

func (s *stateSuite) TestListSecretsNone(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	ctx := context.Background()
	secrets, revisions, err := st.ListSecrets(
		ctx, domainsecret.NilSecretURI, domainsecret.NilRevision, domainsecret.NilLabels)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(secrets), gc.Equals, 0)
	c.Assert(len(revisions), gc.Equals, 0)
}

func (s *stateSuite) TestListSecrets(c *gc.C) {
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

	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri[0], sp[0])
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateUserSecret(ctx, 1, uri[1], sp[1])
	c.Assert(err, jc.ErrorIsNil)

	secrets, revisions, err := st.ListSecrets(
		ctx, domainsecret.NilSecretURI, domainsecret.NilRevision, domainsecret.NilLabels)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(secrets), gc.Equals, 2)
	c.Assert(len(revisions), gc.Equals, 2)

	for i, md := range secrets {
		c.Assert(md.Version, gc.Equals, 1)
		c.Assert(md.LatestRevisionChecksum, gc.Equals, sp[i].Checksum)
		c.Assert(md.Label, gc.Equals, value(sp[i].Label))
		c.Assert(md.Description, gc.Equals, value(sp[i].Description))
		c.Assert(md.LatestRevision, gc.Equals, 1)
		c.Assert(md.AutoPrune, gc.Equals, value(sp[i].AutoPrune))
		c.Assert(md.Owner, jc.DeepEquals, coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: s.modelUUID})
		now := time.Now()
		c.Assert(md.CreateTime, jc.Almost, now)
		c.Assert(md.UpdateTime, jc.Almost, now)

		revs := revisions[i]
		c.Assert(revs, gc.HasLen, 1)
		c.Assert(revs[0].Revision, gc.Equals, 1)
		c.Assert(revs[0].CreateTime, jc.Almost, now)
	}
}

func (s *stateSuite) TestListSecretsByURI(c *gc.C) {

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

	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri[0], sp[0])
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateUserSecret(ctx, 1, uri[1], sp[1])
	c.Assert(err, jc.ErrorIsNil)

	secrets, revisions, err := st.ListSecrets(
		ctx, uri[0], domainsecret.NilRevision, domainsecret.NilLabels)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(secrets), gc.Equals, 1)
	c.Assert(len(revisions), gc.Equals, 1)

	md := secrets[0]
	c.Assert(md.Version, gc.Equals, 1)
	c.Assert(md.Label, gc.Equals, value(sp[0].Label))
	c.Assert(md.Description, gc.Equals, value(sp[0].Description))
	c.Assert(md.LatestRevision, gc.Equals, 1)
	c.Assert(md.AutoPrune, gc.Equals, value(sp[0].AutoPrune))
	c.Assert(md.Owner, jc.DeepEquals, coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: s.modelUUID})
	now := time.Now()
	c.Assert(md.CreateTime, jc.Almost, now)
	c.Assert(md.UpdateTime, jc.Almost, now)

	revs := revisions[0]
	c.Assert(revs, gc.HasLen, 1)
	c.Assert(revs[0].Revision, gc.Equals, 1)
	c.Assert(revs[0].CreateTime, jc.Almost, now)
}

func (s *stateSuite) setupUnits(c *gc.C, appName string) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		charmUUID := uuid.MustNewUUID().String()
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid)
VALUES (?);
`, charmUUID)
		if err != nil {
			return errors.Trace(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_metadata (charm_uuid, name)
VALUES (?, ?);
		`, charmUUID, appName)
		if err != nil {
			return errors.Trace(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_origin (charm_uuid, reference_name)
VALUES (?, ?);
		`, charmUUID, appName)
		if err != nil {
			return errors.Trace(err)
		}

		applicationUUID := uuid.MustNewUUID().String()
		_, err = tx.ExecContext(ctx, `
INSERT INTO application (uuid, charm_uuid, name, life_id)
VALUES (?, ?, ?, ?)
`, applicationUUID, charmUUID, appName, life.Alive)
		if err != nil {
			return errors.Trace(err)
		}

		// Do 2 units.
		for i := 0; i < 2; i++ {
			netNodeUUID := uuid.MustNewUUID().String()
			_, err = tx.ExecContext(ctx, "INSERT INTO net_node (uuid) VALUES (?)", netNodeUUID)
			if err != nil {
				return errors.Trace(err)
			}
			unitUUID := uuid.MustNewUUID().String()
			_, err = tx.ExecContext(ctx, `
INSERT INTO unit (uuid, life_id, name, net_node_uuid, application_uuid)
VALUES (?, ?, ?, ?, (SELECT uuid from application WHERE name = ?))
`, unitUUID, life.Alive, appName+fmt.Sprintf("/%d", i), netNodeUUID, appName)
			if err != nil {
				return errors.Trace(err)
			}
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestListCharmSecretsToDrainNone(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()

	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)

	toDrain, err := st.ListCharmSecretsToDrain(ctx, domainsecret.ApplicationOwners{"mariadb"}, domainsecret.NilUnitOwners)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(toDrain, gc.HasLen, 0)
}

func (s *stateSuite) TestListCharmSecretsToDrain(c *gc.C) {
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

	ctx := context.Background()
	err := st.CreateCharmApplicationSecret(ctx, 1, uri[0], "mysql", sp[0])
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateCharmUnitSecret(ctx, 1, uri[1], "mysql/0", sp[1])
	c.Assert(err, jc.ErrorIsNil)

	uri3 := coresecrets.NewURI()
	sp3 := domainsecret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	err = st.CreateUserSecret(ctx, 1, uri3, sp3)
	c.Assert(err, jc.ErrorIsNil)

	toDrain, err := st.ListCharmSecretsToDrain(ctx, domainsecret.ApplicationOwners{"mysql"}, domainsecret.UnitOwners{"mysql/0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(toDrain, jc.SameContents, []*coresecrets.SecretMetadataForDrain{{
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

func (s *stateSuite) TestListUserSecretsToDrainNone(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()

	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)

	toDrain, err := st.ListUserSecretsToDrain(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(toDrain, gc.HasLen, 0)
}

func (s *stateSuite) TestListUserSecretsToDrain(c *gc.C) {
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

	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri[0], sp[0])
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateUserSecret(ctx, 1, uri[1], sp[1])
	c.Assert(err, jc.ErrorIsNil)

	uri3 := coresecrets.NewURI()
	sp3 := domainsecret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	err = st.CreateCharmUnitSecret(ctx, 1, uri3, "mysql/0", sp3)
	c.Assert(err, jc.ErrorIsNil)

	toDrain, err := st.ListUserSecretsToDrain(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(toDrain, jc.SameContents, []*coresecrets.SecretMetadataForDrain{{
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

func (s *stateSuite) TestCreateCharmSecretAutoPrune(c *gc.C) {
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
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIs, secreterrors.AutoPruneNotSupported)
}

func (s *stateSuite) TestCreateCharmApplicationSecretWithContent(c *gc.C) {
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
	ctx := context.Background()
	err := st.CreateCharmApplicationSecret(ctx, 1, uri, "mysql", sp)
	c.Assert(err, jc.ErrorIsNil)
	owner := coresecrets.Owner{Kind: coresecrets.ApplicationOwner, ID: "mysql"}
	s.assertSecret(c, st, uri, sp, 1, owner)
	data, ref, err := st.GetSecretValue(ctx, uri, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ref, gc.IsNil)
	c.Assert(data, jc.DeepEquals, coresecrets.SecretData{"foo": "bar"})

	ap := domainsecret.AccessParams{
		SubjectID:     "mysql",
		SubjectTypeID: domainsecret.SubjectApplication,
	}
	access, err := st.GetSecretAccess(ctx, uri, ap)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, "manage")
}

func (s *stateSuite) TestCreateCharmApplicationSecretNotFound(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmApplicationSecret(ctx, 1, uri, "mysql", sp)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *stateSuite) TestCreateCharmApplicationSecretFailedRevisionIDMissing(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		Checksum:    "checksum-1234",
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmApplicationSecret(ctx, 1, uri, "mysql", sp)
	c.Assert(err, gc.ErrorMatches, `*.revision ID must be provided`)
}

func (s *stateSuite) TestCreateCharmUnitSecretWithContent(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)
	owner := coresecrets.Owner{Kind: coresecrets.UnitOwner, ID: "mysql/0"}
	s.assertSecret(c, st, uri, sp, 1, owner)
	data, ref, err := st.GetSecretValue(ctx, uri, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ref, gc.IsNil)
	c.Assert(data, jc.DeepEquals, coresecrets.SecretData{"foo": "bar"})

	ap := domainsecret.AccessParams{
		SubjectID:     "mysql/0",
		SubjectTypeID: domainsecret.SubjectUnit,
	}
	access, err := st.GetSecretAccess(ctx, uri, ap)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, "manage")
}

func (s *stateSuite) TestCreateCharmUnitSecretNotFound(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestCreateCharmUnitSecretFailedRevisionIDMissing(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, gc.ErrorMatches, `*.revision ID must be provided`)
}

func (s *stateSuite) TestCreateCharmApplicationSecretLabelAlreadyExists(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmApplicationSecret(ctx, 1, uri, "mysql", sp)
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateCharmApplicationSecret(ctx, 1, uri2, "mysql", sp)
	c.Assert(err, jc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
}

func (s *stateSuite) TestCreateCharmUnitSecretLabelAlreadyExists(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	uri3 := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateCharmUnitSecret(ctx, 1, uri2, "mysql/1", sp)
	c.Assert(err, jc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
	err = st.CreateCharmUnitSecret(ctx, 1, uri3, "mysql/0", sp)
	c.Assert(err, jc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
}

func (s *stateSuite) TestCreateCharmUnitSecretLabelAlreadyExistsForApplication(c *gc.C) {
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
	uri2 := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmApplicationSecret(ctx, 1, uri, "mysql", sp)
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateCharmUnitSecret(ctx, 1, uri2, "mysql/0", sp)
	c.Assert(err, jc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
}

func (s *stateSuite) TestCreateManyApplicationSecretsNoLabelClash(c *gc.C) {
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
		ctx := context.Background()
		err := st.CreateCharmApplicationSecret(ctx, 1, uri, "mysql", sp)
		c.Assert(err, jc.ErrorIsNil)
		owner := coresecrets.Owner{Kind: coresecrets.ApplicationOwner, ID: "mysql"}
		s.assertSecret(c, st, uri, sp, 1, owner)
		data, ref, err := st.GetSecretValue(ctx, uri, 1)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ref, gc.IsNil)
		c.Assert(data, jc.DeepEquals, coresecrets.SecretData{"foo": content})
	}
	createAndCheck("my label")
	createAndCheck("")
	createAndCheck("")
	createAndCheck("another label")
}

func (s *stateSuite) TestCreateManyUnitSecretsNoLabelClash(c *gc.C) {
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
		ctx := context.Background()
		err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
		c.Assert(err, jc.ErrorIsNil)
		owner := coresecrets.Owner{Kind: coresecrets.UnitOwner, ID: "mysql/0"}
		s.assertSecret(c, st, uri, sp, 1, owner)
		data, ref, err := st.GetSecretValue(ctx, uri, 1)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ref, gc.IsNil)
		c.Assert(data, jc.DeepEquals, coresecrets.SecretData{"foo": content})
	}
	createAndCheck("my label")
	createAndCheck("")
	createAndCheck("")
	createAndCheck("another label")
}

func (s *stateSuite) TestListCharmSecretsMissingOwners(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	_, _, err := st.ListCharmSecrets(context.Background(),
		domainsecret.NilApplicationOwners, domainsecret.NilUnitOwners)
	c.Assert(err, gc.ErrorMatches, "querying charm secrets: must supply at least one app owner or unit owner")
}

func (s *stateSuite) TestListCharmSecretsByUnit(c *gc.C) {
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

	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri[0], sp[0])
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateCharmUnitSecret(ctx, 1, uri[1], "mysql/0", sp[1])
	c.Assert(err, jc.ErrorIsNil)

	secrets, revisions, err := st.ListCharmSecrets(ctx,
		domainsecret.NilApplicationOwners, domainsecret.UnitOwners{"mysql/0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(secrets), gc.Equals, 1)
	c.Assert(len(revisions), gc.Equals, 1)

	now := time.Now()

	md := secrets[0]
	c.Assert(md.Version, gc.Equals, 1)
	c.Assert(md.LatestRevisionChecksum, gc.Equals, sp[1].Checksum)
	c.Assert(md.Label, gc.Equals, value(sp[1].Label))
	c.Assert(md.Description, gc.Equals, value(sp[1].Description))
	c.Assert(md.LatestRevision, gc.Equals, 1)
	c.Assert(md.AutoPrune, jc.IsFalse)
	c.Assert(md.Owner, jc.DeepEquals, coresecrets.Owner{Kind: coresecrets.UnitOwner, ID: "mysql/0"})
	c.Assert(md.CreateTime, jc.Almost, now)
	c.Assert(md.UpdateTime, jc.Almost, now)

	revs := revisions[0]
	c.Assert(revs, gc.HasLen, 1)
	c.Assert(revs[0].Revision, gc.Equals, 1)
	c.Assert(revs[0].ValueRef, jc.DeepEquals, &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "revision-id",
	})
	c.Assert(revs[0].CreateTime, jc.Almost, now)
}

func (s *stateSuite) TestListCharmSecretsByApplication(c *gc.C) {
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

	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri[0], sp[0])
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateCharmApplicationSecret(ctx, 1, uri[1], "mysql", sp[1])
	c.Assert(err, jc.ErrorIsNil)

	secrets, revisions, err := st.ListCharmSecrets(ctx,
		domainsecret.ApplicationOwners{"mysql"}, domainsecret.NilUnitOwners)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(secrets), gc.Equals, 1)
	c.Assert(len(revisions), gc.Equals, 1)

	now := time.Now()

	md := secrets[0]
	c.Assert(md.Version, gc.Equals, 1)
	c.Assert(md.Label, gc.Equals, value(sp[1].Label))
	c.Assert(md.Description, gc.Equals, value(sp[1].Description))
	c.Assert(md.LatestRevision, gc.Equals, 1)
	c.Assert(md.AutoPrune, jc.IsFalse)
	c.Assert(md.Owner, jc.DeepEquals, coresecrets.Owner{Kind: coresecrets.ApplicationOwner, ID: "mysql"})
	c.Assert(md.CreateTime, jc.Almost, now)
	c.Assert(md.UpdateTime, jc.Almost, now)

	revs := revisions[0]
	c.Assert(revs, gc.HasLen, 1)
	c.Assert(revs[0].Revision, gc.Equals, 1)
	c.Assert(revs[0].CreateTime, jc.Almost, now)
}

func (s *stateSuite) TestListCharmSecretsApplicationOrUnit(c *gc.C) {
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

	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri[0], sp[0])
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateCharmApplicationSecret(ctx, 1, uri[1], "mysql", sp[1])
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateCharmUnitSecret(ctx, 1, uri[2], "mysql/0", sp[2])
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateCharmUnitSecret(ctx, 1, uri[3], "postgresql/0", sp[3])
	c.Assert(err, jc.ErrorIsNil)

	secrets, revisions, err := st.ListCharmSecrets(ctx,
		domainsecret.ApplicationOwners{"mysql"}, domainsecret.UnitOwners{"mysql/0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(secrets), gc.Equals, 2)
	c.Assert(len(revisions), gc.Equals, 2)

	now := time.Now()

	first := 0
	second := 1
	if secrets[first].Label != value(sp[1].Label) {
		first = 1
		second = 0
	}

	md := secrets[first]
	c.Assert(md.Version, gc.Equals, 1)
	c.Assert(md.Label, gc.Equals, value(sp[1].Label))
	c.Assert(md.Description, gc.Equals, value(sp[1].Description))
	c.Assert(md.LatestRevision, gc.Equals, 1)
	c.Assert(md.AutoPrune, jc.IsFalse)
	c.Assert(md.RotatePolicy, gc.Equals, coresecrets.RotateDaily)
	c.Assert(*md.NextRotateTime, gc.Equals, rotateTime.UTC())
	c.Assert(*md.LatestExpireTime, gc.Equals, expireTime.UTC())
	c.Assert(md.Owner, jc.DeepEquals, coresecrets.Owner{Kind: coresecrets.ApplicationOwner, ID: "mysql"})
	c.Assert(md.CreateTime, jc.Almost, now)
	c.Assert(md.UpdateTime, jc.Almost, now)

	revs := revisions[first]
	c.Assert(revs, gc.HasLen, 1)
	c.Assert(revs[0].Revision, gc.Equals, 1)
	c.Assert(*revs[0].ExpireTime, gc.Equals, expireTime.UTC())
	c.Assert(revs[0].CreateTime, jc.Almost, now)

	md = secrets[second]
	c.Assert(md.Version, gc.Equals, 1)
	c.Assert(md.Label, gc.Equals, value(sp[2].Label))
	c.Assert(md.Description, gc.Equals, value(sp[2].Description))
	c.Assert(md.LatestRevision, gc.Equals, 1)
	c.Assert(md.AutoPrune, jc.IsFalse)
	c.Assert(md.RotatePolicy, gc.Equals, coresecrets.RotateNever)
	c.Assert(md.Owner, jc.DeepEquals, coresecrets.Owner{Kind: coresecrets.UnitOwner, ID: "mysql/0"})
	c.Assert(md.CreateTime, jc.Almost, now)
	c.Assert(md.UpdateTime, jc.Almost, now)

	revs = revisions[second]
	c.Assert(revs, gc.HasLen, 1)
	c.Assert(revs[0].Revision, gc.Equals, 1)
	c.Assert(revs[0].ExpireTime, gc.IsNil)
	c.Assert(revs[0].CreateTime, jc.Almost, now)
}

func (s *stateSuite) TestAllSecretConsumers(c *gc.C) {
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
	ctx := context.Background()
	uri := coresecrets.NewURI().WithSource(s.modelUUID)
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)
	uri2 := coresecrets.NewURI().WithSource(s.modelUUID)
	err = st.CreateCharmUnitSecret(ctx, 1, uri2, "mysql/1", sp2)
	c.Assert(err, jc.ErrorIsNil)

	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	}
	err = st.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, jc.ErrorIsNil)
	consumer = &coresecrets.SecretConsumerMetadata{
		Label:           "my label2",
		CurrentRevision: 668,
	}
	err = st.SaveSecretConsumer(ctx, uri2, "mysql/1", consumer)
	c.Assert(err, jc.ErrorIsNil)
	consumer = &coresecrets.SecretConsumerMetadata{
		Label:           "my label3",
		CurrentRevision: 667,
	}
	err = st.SaveSecretConsumer(ctx, uri, "mysql/1", consumer)
	c.Assert(err, jc.ErrorIsNil)

	got, err := st.AllSecretConsumers(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, map[string][]domainsecret.ConsumerInfo{
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

func (s *stateSuite) TestSaveSecretConsumer(c *gc.C) {
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
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	}

	err = st.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, jc.ErrorIsNil)

	got, latest, err := st.GetSecretConsumer(ctx, uri, "mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, consumer)
	c.Assert(latest, gc.Equals, 1)
}

func (s *stateSuite) TestSaveSecretConsumerMarksObsolete(c *gc.C) {
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
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	consumer := &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 1,
	}
	err = st.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, jc.ErrorIsNil)

	got, latest, err := st.GetSecretConsumer(ctx, uri, "mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, consumer)
	c.Assert(latest, gc.Equals, 1)

	// Latest revision is 3 now, revision 2 shoule be obsolete.
	sp2 := domainsecret.UpsertSecretParams{
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "new-backend",
			RevisionID: "new-revision",
		},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	err = st.UpdateSecret(context.Background(), uri, sp2)
	c.Assert(err, jc.ErrorIsNil)
	content, valueRef, err := st.GetSecretValue(ctx, uri, 2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(content, gc.IsNil)
	c.Assert(valueRef, jc.DeepEquals, &coresecrets.ValueRef{BackendID: "new-backend", RevisionID: "new-revision"})

	md, err := st.GetSecret(ctx, uri)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md.Version, gc.Equals, 1)
	c.Assert(md.Label, gc.Equals, value(sp.Label))
	c.Assert(md.Description, gc.Equals, value(sp.Description))
	c.Assert(md.LatestRevision, gc.Equals, 2)

	// Revision 1 now is been consumed by the unit, so it should NOT be obsolete.
	obsolete, pendingDelete := s.getObsolete(c, uri, 1)
	c.Check(obsolete, jc.IsFalse)
	c.Check(pendingDelete, jc.IsFalse)
	// Revision 2 is the latest revision, so it should be NOT obsolete.
	obsolete, pendingDelete = s.getObsolete(c, uri, 2)
	c.Check(obsolete, jc.IsFalse)
	c.Check(pendingDelete, jc.IsFalse)

	// Change to consume the revision 2, so revision 1 should go obsolete.
	consumer = &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 2,
	}
	err = st.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, jc.ErrorIsNil)

	obsolete, pendingDelete = s.getObsolete(c, uri, 1)
	c.Check(obsolete, jc.IsTrue)
	c.Check(pendingDelete, jc.IsTrue)
	obsolete, pendingDelete = s.getObsolete(c, uri, 2)
	c.Check(obsolete, jc.IsFalse)
	c.Check(pendingDelete, jc.IsFalse)
}

func (s *stateSuite) TestSaveSecretConsumerSecretNotExists(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	uri := coresecrets.NewURI().WithSource(s.modelUUID)
	ctx := context.Background()
	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	}

	err := st.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, jc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestSaveSecretConsumerUnitNotExists(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		ValueRef:    &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		AutoPrune:   ptr(true),
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI().WithSource(s.modelUUID)
	ctx := context.Background()

	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	}

	err = st.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestSaveSecretConsumerDifferentModel(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	uri := coresecrets.NewURI().WithSource("some-other-model")

	// Save the remote secret and its latest revision.
	err := st.UpdateRemoteSecretRevision(context.Background(), uri, 666)
	c.Assert(err, jc.ErrorIsNil)

	ctx := context.Background()
	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	}

	err = st.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, jc.ErrorIsNil)

	got, _, err := st.GetSecretConsumer(ctx, uri, "mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, consumer)
}

// TestSaveSecretConsumerDifferentModelFirstTime is the same as
// TestSaveSecretConsumerDifferentModel but there's no remote revision
// recorded yet.
func (s *stateSuite) TestSaveSecretConsumerDifferentModelFirstTime(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	uri := coresecrets.NewURI().WithSource("some-other-model")

	ctx := context.Background()
	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	}

	err := st.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, jc.ErrorIsNil)

	got, _, err := st.GetSecretConsumer(ctx, uri, "mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, consumer)

	var latest int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
SELECT latest_revision FROM secret_reference WHERE secret_id = ?
		`, uri.ID)
		if err := row.Scan(&latest); err != nil {
			return err
		}
		return row.Err()
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(latest, gc.Equals, 666)
}

func (s *stateSuite) TestAllRemoteSecrets(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	uri := coresecrets.NewURI().WithSource("some-other-model")

	// Save the remote secret and its latest revision.
	err := st.UpdateRemoteSecretRevision(context.Background(), uri, 666)
	c.Assert(err, jc.ErrorIsNil)

	ctx := context.Background()
	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 1,
	}
	err = st.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, jc.ErrorIsNil)

	consumer = &coresecrets.SecretConsumerMetadata{
		Label:           "my label2",
		CurrentRevision: 2,
	}
	err = st.SaveSecretConsumer(ctx, uri, "mysql/1", consumer)
	c.Assert(err, jc.ErrorIsNil)

	got, err := st.AllRemoteSecrets(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, []domainsecret.RemoteSecretInfo{{
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

func (s *stateSuite) TestGetSecretConsumerFirstTime(c *gc.C) {
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
	ctx := context.Background()

	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	_, latest, err := st.GetSecretConsumer(ctx, uri, "mysql/0")
	c.Assert(err, jc.ErrorIs, secreterrors.SecretConsumerNotFound)
	c.Assert(latest, gc.Equals, 1)
}

func (s *stateSuite) TestGetSecretConsumerRemoteSecretFirstTime(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	uri := coresecrets.NewURI().WithSource("some-other-model")
	ctx := context.Background()

	err := st.UpdateRemoteSecretRevision(ctx, uri, 666)
	c.Assert(err, jc.ErrorIsNil)

	_, latest, err := st.GetSecretConsumer(ctx, uri, "mysql/0")
	c.Assert(err, jc.ErrorIs, secreterrors.SecretConsumerNotFound)
	c.Assert(latest, gc.Equals, 666)
}

func (s *stateSuite) TestGetSecretConsumerSecretNotExists(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	uri := coresecrets.NewURI()

	_, _, err := st.GetSecretConsumer(context.Background(), uri, "mysql/0")
	c.Assert(err, jc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGetSecretConsumerUnitNotExists(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		ValueRef:    &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		AutoPrune:   ptr(true),
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()

	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = st.GetSecretConsumer(ctx, uri, "mysql/0")
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestGetUserSecretURIByLabel(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   ptr(true),
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	got, err := st.GetUserSecretURIByLabel(ctx, "my label")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got.ID, gc.Equals, uri.ID)
}

func (s *stateSuite) TestGetUserSecretURIByLabelSecretNotExists(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	_, err := st.GetUserSecretURIByLabel(context.Background(), "my label")
	c.Assert(err, jc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGetURIByConsumerLabel(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)
	err = st.SaveSecretConsumer(ctx, uri, "mysql/0", &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	})
	c.Assert(err, jc.ErrorIsNil)

	got, err := st.GetURIByConsumerLabel(ctx, "my label", "mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got.ID, gc.Equals, uri.ID)
	c.Assert(got.SourceUUID, gc.Equals, uri.SourceUUID)

	_, err = st.GetURIByConsumerLabel(ctx, "another label", "mysql/0")
	c.Assert(err, jc.ErrorIs, secreterrors.SecretNotFound)

}

func (s *stateSuite) TestGetURIByConsumerLabelUnitNotExists(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	_, err := st.GetURIByConsumerLabel(context.Background(), "my label", "mysql/2")
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestUpdateSecretNotFound(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	uri := coresecrets.NewURI()
	err := st.UpdateSecret(context.Background(), uri, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Label:      ptr("label"),
	})
	c.Assert(err, jc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestUpdateSecretNothingToDo(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	uri := coresecrets.NewURI()
	err := st.UpdateSecret(context.Background(), uri, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String())})
	c.Assert(err, gc.ErrorMatches, "must specify a new value or metadata to update a secret")
}

func (s *stateSuite) TestUpdateUserSecretMetadataOnly(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label2"),
	}
	err = st.UpdateSecret(context.Background(), uri, sp2)
	c.Assert(err, jc.ErrorIsNil)

	md, err := st.GetSecret(ctx, uri)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md.Version, gc.Equals, 1)
	c.Assert(md.Label, gc.Equals, value(sp2.Label))
	c.Assert(md.Description, gc.Equals, value(sp2.Description))
	c.Assert(md.LatestRevision, gc.Equals, 1)

	now := time.Now()
	c.Assert(md.UpdateTime, jc.Almost, now)
}

func (s *stateSuite) TestUpdateUserSecretLabelAlreadyExists(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   ptr(true),
	}
	sp2 := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label2"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   ptr(true),
	}
	uri := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateUserSecret(ctx, 1, uri2, sp2)
	c.Assert(err, jc.ErrorIsNil)

	sp = domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Label:      ptr("my label2"),
	}
	err = st.UpdateSecret(ctx, uri, sp)
	c.Assert(err, jc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
}

func (s *stateSuite) TestUpdateUserSecretFailedRevisionIDMissing(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   ptr(true),
	}

	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	sp = domainsecret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo": "something-else"},
	}
	err = st.UpdateSecret(ctx, uri, sp)
	c.Assert(err, gc.ErrorMatches, `*.revision ID must be provided`)
}

func (s *stateSuite) TestUpdateCharmApplicationSecretMetadataOnly(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmApplicationSecret(ctx, 1, uri, "mysql", sp)
	c.Assert(err, jc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label2"),
	}
	err = st.UpdateSecret(context.Background(), uri, sp2)
	c.Assert(err, jc.ErrorIsNil)

	md, err := st.GetSecret(ctx, uri)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md.Version, gc.Equals, 1)
	c.Assert(md.Label, gc.Equals, value(sp2.Label))
	c.Assert(md.Description, gc.Equals, value(sp2.Description))
	c.Assert(md.LatestRevision, gc.Equals, 1)

	now := time.Now()
	c.Assert(md.UpdateTime, jc.Almost, now)
}

func (s *stateSuite) TestUpdateCharmUnitSecretMetadataOnly(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label2"),
	}
	err = st.UpdateSecret(context.Background(), uri, sp2)
	c.Assert(err, jc.ErrorIsNil)

	md, err := st.GetSecret(ctx, uri)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md.Version, gc.Equals, 1)
	c.Assert(md.Label, gc.Equals, value(sp2.Label))
	c.Assert(md.Description, gc.Equals, value(sp2.Description))
	c.Assert(md.LatestRevision, gc.Equals, 1)

	now := time.Now()
	c.Assert(md.UpdateTime, jc.Almost, now)
}

func (s *stateSuite) TestUpdateCharmApplicationSecretLabelAlreadyExists(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	sp2 := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label2"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmApplicationSecret(ctx, 1, uri, "mysql", sp)
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateCharmUnitSecret(ctx, 1, uri2, "mysql/0", sp2)
	c.Assert(err, jc.ErrorIsNil)

	sp.Label = ptr("my label2")
	err = st.UpdateSecret(ctx, uri, sp)
	c.Assert(err, jc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
}

func (s *stateSuite) TestUpdateCharmUnitSecretLabelAlreadyExists(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	sp2 := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label2"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateCharmApplicationSecret(ctx, 1, uri2, "mysql", sp2)
	c.Assert(err, jc.ErrorIsNil)

	sp.Label = ptr("my label2")
	err = st.UpdateSecret(ctx, uri, sp)
	c.Assert(err, jc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
}

func fillDataForUpsertSecretParams(c *gc.C, p *domainsecret.UpsertSecretParams, data coresecrets.SecretData) {
	checksum, err := coresecrets.NewSecretValue(data).Checksum()
	c.Assert(err, jc.ErrorIsNil)
	p.Data = data
	p.Checksum = checksum
}

func (s *stateSuite) TestUpdateSecretContentNoOpsIfNoContentChange(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	fillDataForUpsertSecretParams(c, &sp, coresecrets.SecretData{"foo": "bar", "hello": "world"})
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)

	err = st.UpdateSecret(context.Background(), uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	md, revs, err := st.ListSecrets(ctx, uri, ptr(1), domainsecret.NilLabels)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md, gc.HasLen, 1)
	c.Assert(md[0].LatestRevision, gc.Equals, 1)

	c.Assert(revs, gc.HasLen, 1)
	c.Assert(revs[0], gc.HasLen, 1)
	rev := revs[0][0]
	c.Assert(rev.Revision, gc.Equals, 1)
}

func (s *stateSuite) TestUpdateSecretContent(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	fillDataForUpsertSecretParams(c, &sp, coresecrets.SecretData{"foo": "bar", "hello": "world"})
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)

	expireTime := time.Now().Add(2 * time.Hour)
	sp2 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		ExpireTime: &expireTime,
	}
	fillDataForUpsertSecretParams(c, &sp2, coresecrets.SecretData{"foo2": "bar2", "hello": "world"})
	err = st.UpdateSecret(context.Background(), uri, sp2)
	c.Assert(err, jc.ErrorIsNil)

	md, revs, err := st.ListSecrets(ctx, uri, ptr(2), domainsecret.NilLabels)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md, gc.HasLen, 1)
	c.Assert(md[0].Version, gc.Equals, 1)
	c.Assert(md[0].Label, gc.Equals, value(sp.Label))
	c.Assert(md[0].Description, gc.Equals, value(sp.Description))
	c.Assert(md[0].LatestRevision, gc.Equals, 2)

	now := time.Now()
	c.Assert(md[0].UpdateTime, jc.Almost, now)

	c.Assert(revs, gc.HasLen, 1)
	c.Assert(revs[0], gc.HasLen, 1)
	rev := revs[0][0]
	c.Assert(rev.Revision, gc.Equals, 2)
	c.Assert(rev.ExpireTime, gc.NotNil)
	c.Assert(*rev.ExpireTime, gc.Equals, expireTime.UTC())

	content, valueRef, err := st.GetSecretValue(ctx, uri, 2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(valueRef, gc.IsNil)
	c.Assert(content, jc.DeepEquals, coresecrets.SecretData{"foo2": "bar2", "hello": "world"})

	// Revision 1 is obsolete.
	obsolete, pendingDelete := s.getObsolete(c, uri, 1)
	c.Check(obsolete, jc.IsTrue)
	c.Check(pendingDelete, jc.IsTrue)

	// But not revision 2.
	obsolete, pendingDelete = s.getObsolete(c, uri, 2)
	c.Check(obsolete, jc.IsFalse)
	c.Check(pendingDelete, jc.IsFalse)
}

func (s *stateSuite) TestUpdateSecretContentObsolete(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	// Create a consumer so revision 1 does not go obsolete.
	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 1,
	}

	err = st.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, jc.ErrorIsNil)

	expireTime := time.Now().Add(2 * time.Hour)
	sp2 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		ExpireTime: &expireTime,
		Data:       coresecrets.SecretData{"foo2": "bar2", "hello": "world"},
	}
	err = st.UpdateSecret(context.Background(), uri, sp2)
	c.Assert(err, jc.ErrorIsNil)

	md, revs, err := st.ListSecrets(ctx, uri, ptr(2), domainsecret.NilLabels)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md, gc.HasLen, 1)
	c.Assert(md[0].Version, gc.Equals, 1)
	c.Assert(md[0].Label, gc.Equals, value(sp.Label))
	c.Assert(md[0].Description, gc.Equals, value(sp.Description))
	c.Assert(md[0].LatestRevision, gc.Equals, 2)

	now := time.Now()
	c.Assert(md[0].UpdateTime, jc.Almost, now)

	c.Assert(revs, gc.HasLen, 1)
	c.Assert(revs[0], gc.HasLen, 1)
	rev := revs[0][0]
	c.Assert(rev.Revision, gc.Equals, 2)
	c.Assert(rev.ExpireTime, gc.NotNil)
	c.Assert(*rev.ExpireTime, gc.Equals, expireTime.UTC())

	content, valueRef, err := st.GetSecretValue(ctx, uri, 2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(valueRef, gc.IsNil)
	c.Assert(content, jc.DeepEquals, coresecrets.SecretData{"foo2": "bar2", "hello": "world"})

	// Latest revision is 3 now, revision 2 shoule be obsolete.
	sp3 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo3": "bar3", "hello": "world"},
	}
	err = st.UpdateSecret(context.Background(), uri, sp3)
	c.Assert(err, jc.ErrorIsNil)
	content, valueRef, err = st.GetSecretValue(ctx, uri, 3)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(valueRef, gc.IsNil)
	c.Assert(content, jc.DeepEquals, coresecrets.SecretData{"foo3": "bar3", "hello": "world"})

	md, _, err = st.ListSecrets(ctx, uri, ptr(2), domainsecret.NilLabels)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md, gc.HasLen, 1)
	c.Assert(md[0].Version, gc.Equals, 1)
	c.Assert(md[0].Label, gc.Equals, value(sp.Label))
	c.Assert(md[0].Description, gc.Equals, value(sp.Description))
	c.Assert(md[0].LatestRevision, gc.Equals, 3)

	_ = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		// Revision 1 is NOT obsolete because it's been consumed.
		obsolete, pendingDelete := s.getObsolete(c, uri, 1)
		c.Check(obsolete, jc.IsFalse)
		c.Check(pendingDelete, jc.IsFalse)

		// Revision 2 is obsolete.
		obsolete, pendingDelete = s.getObsolete(c, uri, 2)
		c.Check(obsolete, jc.IsTrue)
		c.Check(pendingDelete, jc.IsTrue)

		// Revision 3 is NOT obsolete because it's the latest revision.
		obsolete, pendingDelete = s.getObsolete(c, uri, 3)
		c.Check(obsolete, jc.IsFalse)
		c.Check(pendingDelete, jc.IsFalse)
		return nil
	})
}

func (s *stateSuite) getObsolete(c *gc.C, uri *coresecrets.URI, rev int) (bool, bool) {
	var obsolete, pendingDelete bool
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
	c.Check(err, jc.ErrorIsNil)
	return obsolete, pendingDelete
}

func (s *stateSuite) TestUpdateSecretContentValueRef(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		ValueRef:   &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "revision-id"},
	}
	err = st.UpdateSecret(context.Background(), uri, sp2)
	c.Assert(err, jc.ErrorIsNil)

	md, revs, err := st.ListSecrets(ctx, uri, ptr(2), domainsecret.NilLabels)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md, gc.HasLen, 1)
	c.Assert(md[0].Version, gc.Equals, 1)
	c.Assert(md[0].Label, gc.Equals, value(sp.Label))
	c.Assert(md[0].Description, gc.Equals, value(sp.Description))
	c.Assert(md[0].LatestRevision, gc.Equals, 2)

	now := time.Now()
	c.Assert(md[0].UpdateTime, jc.Almost, now)

	c.Assert(revs, gc.HasLen, 1)
	c.Assert(revs[0], gc.HasLen, 1)
	rev := revs[0][0]
	c.Assert(rev.Revision, gc.Equals, 2)
	c.Assert(rev.ExpireTime, gc.IsNil)

	content, valueRef, err := st.GetSecretValue(ctx, uri, 2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(valueRef, jc.DeepEquals, &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "revision-id"})
	c.Assert(content, gc.HasLen, 0)
}

func (s *stateSuite) TestUpdateSecretNoRotate(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:   ptr(uuid.MustNewUUID().String()),
		RotatePolicy: ptr(domainsecret.RotateDaily),
		Data:         coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID:   ptr(uuid.MustNewUUID().String()),
		RotatePolicy: ptr(domainsecret.RotateNever),
	}
	err = st.UpdateSecret(context.Background(), uri, sp2)
	c.Assert(err, jc.ErrorIsNil)

	md, err := st.GetSecret(ctx, uri)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md.RotatePolicy, gc.Equals, coresecrets.RotateNever)
	c.Assert(md.NextRotateTime, gc.IsNil)

	var count int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
SELECT count(*) FROM secret_rotation WHERE secret_id = ?
		`, uri.ID)
		if err := row.Scan(&count); err != nil {
			return err
		}
		return row.Err()
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(count, gc.Equals, 0)
}

func (s *stateSuite) TestUpdateCharmSecretAutoPrune(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		AutoPrune:  ptr(true),
	}
	err = st.UpdateSecret(context.Background(), uri, sp2)
	c.Assert(err, jc.ErrorIs, secreterrors.AutoPruneNotSupported)
}

func (s *stateSuite) TestSaveSecretRemoteConsumer(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		ValueRef:    &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		AutoPrune:   ptr(true),
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	consumer := &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	}

	err = st.SaveSecretRemoteConsumer(ctx, uri, "remote-app/0", consumer)
	c.Assert(err, jc.ErrorIsNil)

	got, latest, err := st.GetSecretRemoteConsumer(ctx, uri, "remote-app/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, consumer)
	c.Assert(latest, gc.Equals, 1)
}

func (s *stateSuite) TestAllSecretRemoteConsumers(c *gc.C) {
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
	ctx := context.Background()
	uri := coresecrets.NewURI().WithSource(s.modelUUID)
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)
	uri2 := coresecrets.NewURI().WithSource(s.modelUUID)
	err = st.CreateCharmUnitSecret(ctx, 1, uri2, "mysql/1", sp2)
	c.Assert(err, jc.ErrorIsNil)

	consumer := &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	}
	err = st.SaveSecretRemoteConsumer(ctx, uri, "remote-app/0", consumer)
	c.Assert(err, jc.ErrorIsNil)
	consumer = &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 668,
	}
	err = st.SaveSecretRemoteConsumer(ctx, uri2, "remote-app/1", consumer)
	c.Assert(err, jc.ErrorIsNil)
	consumer = &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 667,
	}
	err = st.SaveSecretRemoteConsumer(ctx, uri, "remote-app/1", consumer)
	c.Assert(err, jc.ErrorIsNil)

	got, err := st.AllSecretRemoteConsumers(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, map[string][]domainsecret.ConsumerInfo{
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

func (s *stateSuite) TestSaveSecretRemoteConsumerMarksObsolete(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		ValueRef:    &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		AutoPrune:   ptr(true),
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)
	sp.Label = ptr("")
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = st.UpdateSecret(ctx, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	consumer := &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 1,
	}

	err = st.SaveSecretRemoteConsumer(ctx, uri, "remote-app/0", consumer)
	c.Assert(err, jc.ErrorIsNil)

	consumer.CurrentRevision = 2
	err = st.SaveSecretRemoteConsumer(ctx, uri, "remote-app/0", consumer)
	c.Assert(err, jc.ErrorIsNil)

	// Revision 1 is obsolete.
	obsolete, pendingDelete := s.getObsolete(c, uri, 1)
	c.Check(obsolete, jc.IsTrue)
	c.Check(pendingDelete, jc.IsTrue)

	// But not revision 2.
	obsolete, pendingDelete = s.getObsolete(c, uri, 2)
	c.Check(obsolete, jc.IsFalse)
	c.Check(pendingDelete, jc.IsFalse)
}

func (s *stateSuite) TestSaveSecretRemoteConsumerSecretNotExists(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	uri := coresecrets.NewURI().WithSource(s.modelUUID)
	ctx := context.Background()
	consumer := &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	}

	err := st.SaveSecretConsumer(ctx, uri, "remote-app/0", consumer)
	c.Assert(err, jc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGetSecretRemoteConsumerFirstTime(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		ValueRef:    &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		AutoPrune:   ptr(true),
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()

	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	_, latest, err := st.GetSecretRemoteConsumer(ctx, uri, "remote-app/0")
	c.Assert(err, jc.ErrorIs, secreterrors.SecretConsumerNotFound)
	c.Assert(latest, gc.Equals, 1)
}

func (s *stateSuite) TestGetSecretRemoteConsumerSecretNotExists(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	uri := coresecrets.NewURI()

	_, _, err := st.GetSecretRemoteConsumer(context.Background(), uri, "remite-app/0")
	c.Assert(err, jc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestUpdateRemoteSecretRevision(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	uri := coresecrets.NewURI()

	getLatest := func() int {
		var got int
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			row := tx.QueryRowContext(ctx, `
			SELECT latest_revision FROM secret_reference WHERE secret_id = ?
		`, uri.ID)
			if err := row.Scan(&got); err != nil {
				return err
			}
			return row.Err()
		})
		c.Assert(err, jc.ErrorIsNil)
		return got
	}

	err := st.UpdateRemoteSecretRevision(context.Background(), uri, 666)
	c.Assert(err, jc.ErrorIsNil)
	got := getLatest()
	c.Assert(got, gc.Equals, 666)
	err = st.UpdateRemoteSecretRevision(context.Background(), uri, 667)
	c.Assert(err, jc.ErrorIsNil)
	got = getLatest()
	c.Assert(got, gc.Equals, 667)
}

func (s *stateSuite) TestGrantUnitAccess(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeUnit,
		ScopeID:       "mysql/0",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, jc.ErrorIsNil)

	ap := domainsecret.AccessParams{
		SubjectTypeID: p.SubjectTypeID,
		SubjectID:     p.SubjectID,
	}
	role, err := st.GetSecretAccess(ctx, uri, ap)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(role, gc.Equals, "view")
}

func (s *stateSuite) TestGetUnitGrantAccessScope(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeUnit,
		ScopeID:       "mysql/0",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, jc.ErrorIsNil)

	ap := domainsecret.AccessParams{
		SubjectTypeID: p.SubjectTypeID,
		SubjectID:     p.SubjectID,
	}
	scope, err := st.GetSecretAccessScope(ctx, uri, ap)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(scope, jc.DeepEquals, &domainsecret.AccessScope{
		ScopeTypeID: domainsecret.ScopeUnit,
		ScopeID:     "mysql/0",
	})
}

func (s *stateSuite) TestGrantApplicationAccess(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeApplication,
		ScopeID:       "mysql",
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, jc.ErrorIsNil)

	ap := domainsecret.AccessParams{
		SubjectTypeID: p.SubjectTypeID,
		SubjectID:     p.SubjectID,
	}
	role, err := st.GetSecretAccess(ctx, uri, ap)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(role, gc.Equals, "view")
}

func (s *stateSuite) TestGetApplicationGrantAccessScope(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeApplication,
		ScopeID:       "mysql",
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, jc.ErrorIsNil)

	ap := domainsecret.AccessParams{
		SubjectTypeID: p.SubjectTypeID,
		SubjectID:     p.SubjectID,
	}
	scope, err := st.GetSecretAccessScope(ctx, uri, ap)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(scope, jc.DeepEquals, &domainsecret.AccessScope{
		ScopeTypeID: domainsecret.ScopeApplication,
		ScopeID:     "mysql",
	})
}

func (s *stateSuite) TestGrantModelAccess(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeModel,
		ScopeID:       s.modelUUID,
		SubjectTypeID: domainsecret.SubjectModel,
		SubjectID:     s.modelUUID,
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, jc.ErrorIsNil)

	ap := domainsecret.AccessParams{
		SubjectTypeID: p.SubjectTypeID,
		SubjectID:     p.SubjectID,
	}
	role, err := st.GetSecretAccess(ctx, uri, ap)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(role, gc.Equals, "view")
}

func (s *stateSuite) TestGrantRelationScope(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/1",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, jc.ErrorIsNil)

	ap := domainsecret.AccessParams{
		SubjectTypeID: p.SubjectTypeID,
		SubjectID:     p.SubjectID,
	}
	role, err := st.GetSecretAccess(ctx, uri, ap)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(role, gc.Equals, "view")
}

func (s *stateSuite) TestGetRelationGrantAccessScope(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/1",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, jc.ErrorIsNil)

	ap := domainsecret.AccessParams{
		SubjectTypeID: p.SubjectTypeID,
		SubjectID:     p.SubjectID,
	}
	scope, err := st.GetSecretAccessScope(ctx, uri, ap)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(scope, jc.DeepEquals, &domainsecret.AccessScope{
		ScopeTypeID: domainsecret.ScopeRelation,
		ScopeID:     "mysql:db mediawiki:db",
	})
}

func (s *stateSuite) TestGrantAccessInvariantScope(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeUnit,
		ScopeID:       "mysql/0",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, jc.ErrorIsNil)
	p.ScopeID = "mysql"
	p.ScopeTypeID = domainsecret.ScopeApplication
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, jc.ErrorIs, secreterrors.InvalidSecretPermissionChange)
}

func (s *stateSuite) TestGrantSecretNotFound(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	uri := coresecrets.NewURI()
	ctx := context.Background()

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeUnit,
		ScopeID:       "mysql/0",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}
	err := st.GrantAccess(ctx, uri, p)
	c.Assert(err, jc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGrantUnitNotFound(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeUnit,
		ScopeID:       "mysql/0",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/2",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestGrantApplicationNotFound(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeUnit,
		ScopeID:       "mysql/0",
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "postgresql",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *stateSuite) TestGrantScopeNotFound(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeUnit,
		ScopeID:       "mysql/2",
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestGetAccessNoGrant(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)

	ap := domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
	}
	role, err := st.GetSecretAccess(ctx, uri, ap)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(role, gc.Equals, "")
}

func (s *stateSuite) TestGetSecretGrantsNone(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	g, err := st.GetSecretGrants(ctx, uri, coresecrets.RoleView)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(g, gc.HasLen, 0)
}

func (s *stateSuite) TestGetSecretGrantsAppUnit(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/1",
		RoleID:        domainsecret.RoleManage,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, jc.ErrorIsNil)

	p2 := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p2)
	c.Assert(err, jc.ErrorIsNil)

	g, err := st.GetSecretGrants(ctx, uri, coresecrets.RoleView)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(g, jc.DeepEquals, []domainsecret.GrantParams{{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}})
}

func (s *stateSuite) TestGetSecretGrantsModel(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/1",
		RoleID:        domainsecret.RoleManage,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, jc.ErrorIsNil)

	p2 := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeModel,
		ScopeID:       s.modelUUID,
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p2)
	c.Assert(err, jc.ErrorIsNil)

	g, err := st.GetSecretGrants(ctx, uri, coresecrets.RoleView)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(g, jc.DeepEquals, []domainsecret.GrantParams{{
		ScopeTypeID:   domainsecret.ScopeModel,
		ScopeID:       s.modelUUID,
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}})
}

func (s *stateSuite) TestAllSecretGrants(c *gc.C) {
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
	ctx := context.Background()
	uri := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateCharmApplicationSecret(ctx, 1, uri2, "mysql", sp2)
	c.Assert(err, jc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/1",
		RoleID:        domainsecret.RoleManage,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, jc.ErrorIsNil)

	p2 := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p2)
	c.Assert(err, jc.ErrorIsNil)

	g, err := st.AllSecretGrants(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(g, jc.DeepEquals, map[string][]domainsecret.GrantParams{
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

func (s *stateSuite) TestRevokeAccess(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/1",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, jc.ErrorIsNil)

	p2 := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p2)
	c.Assert(err, jc.ErrorIsNil)

	err = st.RevokeAccess(ctx, uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/1",
	})
	c.Assert(err, jc.ErrorIsNil)

	g, err := st.GetSecretGrants(ctx, uri, coresecrets.RoleView)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(g, jc.DeepEquals, []domainsecret.GrantParams{{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}})
}

func (s *stateSuite) TestListGrantedSecrets(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	ctx := context.Background()
	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "revision-id",
		},
	}
	uri2 := coresecrets.NewURI()
	err = st.CreateUserSecret(ctx, 1, uri2, sp2)
	c.Assert(err, jc.ErrorIsNil)

	sp3 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "revision-id2",
		},
	}
	uri3 := coresecrets.NewURI()
	err = st.CreateUserSecret(ctx, 1, uri3, sp3)
	c.Assert(err, jc.ErrorIsNil)
	err = st.UpdateSecret(ctx, uri3, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "backend-id2",
			RevisionID: "revision-id3",
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri, p)
	c.Assert(err, jc.ErrorIsNil)
	err = st.GrantAccess(ctx, uri2, p)
	c.Assert(err, jc.ErrorIsNil)

	p2 := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
		RoleID:        domainsecret.RoleView,
	}
	err = st.GrantAccess(ctx, uri3, p2)
	c.Assert(err, jc.ErrorIsNil)

	accessors := []domainsecret.AccessParams{{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
	}, {
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
	}}
	result, err := st.ListGrantedSecretsForBackend(ctx, "backend-id", accessors, coresecrets.RoleView)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.SameContents, []*coresecrets.SecretRevisionRef{{
		URI:        uri2,
		RevisionID: "revision-id",
	}, {
		URI:        uri3,
		RevisionID: "revision-id2",
	}})
}

func (s *stateSuite) prepareSecretObsoleteRevisions(c *gc.C, st *State) (
	*coresecrets.URI, *coresecrets.URI, *coresecrets.URI, *coresecrets.URI,
) {
	ctx := context.Background()
	s.setupUnits(c, "mysql")
	s.setupUnits(c, "mediawiki")

	sp := domainsecret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri1 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err := st.CreateCharmApplicationSecret(ctx, 1, uri1, "mysql", sp)
	c.Assert(err, jc.ErrorIsNil)
	updateSecretContent(c, st, uri1)

	uri2 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = st.CreateCharmUnitSecret(ctx, 1, uri2, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)
	updateSecretContent(c, st, uri2)

	uri3 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = st.CreateCharmApplicationSecret(ctx, 1, uri3, "mediawiki", sp)
	c.Assert(err, jc.ErrorIsNil)
	updateSecretContent(c, st, uri3)

	uri4 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = st.CreateCharmUnitSecret(ctx, 1, uri4, "mediawiki/0", sp)
	c.Assert(err, jc.ErrorIsNil)
	updateSecretContent(c, st, uri4)
	return uri1, uri2, uri3, uri4
}

func (s *stateSuite) TestInitialWatchStatementForObsoleteRevision(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	uri1, uri2, uri3, uri4 := s.prepareSecretObsoleteRevisions(c, st)
	ctx := context.Background()

	tableName, f := st.InitialWatchStatementForObsoleteRevision(
		[]string{"mysql", "mediawiki"},
		[]string{"mysql/0", "mediawiki/0"},
	)
	c.Assert(tableName, gc.Equals, "secret_revision_obsolete")
	revisionUUIDs, err := f(ctx, s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(revisionUUIDs, jc.SameContents, []string{
		getRevUUID(c, s.DB(), uri1, 1),
		getRevUUID(c, s.DB(), uri2, 1),
		getRevUUID(c, s.DB(), uri3, 1),
		getRevUUID(c, s.DB(), uri4, 1),
	})
}

func updateSecretContent(c *gc.C, st *State, uri *coresecrets.URI) {
	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo-new": "bar-new"},
	}
	err := st.UpdateSecret(context.Background(), uri, sp)
	c.Assert(err, jc.ErrorIsNil)
}

func getRevUUID(c *gc.C, db *sql.DB, uri *coresecrets.URI, rev int) string {
	var uuid string
	row := db.QueryRowContext(context.Background(), `
SELECT uuid
FROM secret_revision
WHERE secret_id = ? AND revision = ?
`, uri.ID, rev)
	err := row.Scan(&uuid)
	c.Assert(err, jc.ErrorIsNil)
	return uuid
}

func (s *stateSuite) TestGetRevisionIDsForObsolete(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	uri1, uri2, uri3, uri4 := s.prepareSecretObsoleteRevisions(c, st)
	ctx := context.Background()

	result, err := st.GetRevisionIDsForObsolete(ctx,
		nil, nil,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.HasLen, 0)

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
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []string{
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
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []string{
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
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []string{
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
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []string{
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
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []string{
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
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []string{
		revID(uri1, 1),
	})
}

func revID(uri *coresecrets.URI, rev int) string {
	return fmt.Sprintf("%s/%d", uri.ID, rev)
}

func (s *stateSuite) TestDeleteObsoleteUserSecretRevisions(c *gc.C) {
	s.setupUnits(c, "mysql")
	st := newSecretState(c, s.TxnRunnerFactory())

	uriUser1 := coresecrets.NewURI()
	uriUser2 := coresecrets.NewURI()
	uriUser3 := coresecrets.NewURI()
	uriCharm := coresecrets.NewURI()
	ctx := context.Background()
	data := coresecrets.SecretData{"foo": "bar", "hello": "world"}

	err := st.CreateUserSecret(ctx, 1, uriUser1, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       data,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateUserSecret(ctx, 1, uriUser2, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       data,
		AutoPrune:  ptr(true),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateUserSecret(ctx, 1, uriUser3, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       data,
		AutoPrune:  ptr(true),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateCharmApplicationSecret(ctx, 1, uriCharm, "mysql", domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       data,
	})
	c.Assert(err, jc.ErrorIsNil)

	sp := domainsecret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo-new": "bar-new"},
	}
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = st.UpdateSecret(context.Background(), uriUser1, sp)
	c.Assert(err, jc.ErrorIsNil)
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = st.UpdateSecret(context.Background(), uriUser2, sp)
	c.Assert(err, jc.ErrorIsNil)
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = st.UpdateSecret(context.Background(), uriCharm, sp)
	c.Assert(err, jc.ErrorIsNil)

	expectedToBeDeleted := []string{
		getRevUUID(c, s.DB(), uriUser2, 1),
	}
	deletedRevisionIDs, err := st.DeleteObsoleteUserSecretRevisions(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(deletedRevisionIDs, jc.SameContents, expectedToBeDeleted)

	assertRevision(c, s.DB(), uriUser1, 1, true)
	assertRevision(c, s.DB(), uriUser1, 2, true)
	assertRevision(c, s.DB(), uriUser2, 1, false)
	assertRevision(c, s.DB(), uriUser2, 2, true)
	assertRevision(c, s.DB(), uriUser3, 1, true)
	assertRevision(c, s.DB(), uriCharm, 1, true)
	assertRevision(c, s.DB(), uriCharm, 2, true)
}

func assertRevision(c *gc.C, db *sql.DB, uri *coresecrets.URI, rev int, exist bool) {
	var uuid string
	row := db.QueryRowContext(context.Background(), `
SELECT uuid
FROM secret_revision
WHERE secret_id = ? AND revision = ?
`, uri.ID, rev)
	err := row.Scan(&uuid)
	if exist {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, jc.ErrorIs, sql.ErrNoRows)
	}
}

func (s *stateSuite) TestDeleteSomeRevisions(c *gc.C) {
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
	ctx := context.Background()
	err := st.CreateCharmApplicationSecret(ctx, 1, uri, "mysql", sp)
	c.Assert(err, jc.ErrorIsNil)

	data, ref, err := st.GetSecretValue(ctx, uri, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ref, gc.IsNil)
	c.Assert(data, jc.DeepEquals, coresecrets.SecretData{"foo": "bar"})

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar2"},
	}
	err = st.UpdateSecret(ctx, uri, sp2)
	c.Assert(err, jc.ErrorIsNil)
	sp3 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar3"},
	}
	err = st.UpdateSecret(ctx, uri, sp3)
	c.Assert(err, jc.ErrorIsNil)

	expectedToBeDeleted := []string{
		getRevUUID(c, s.DB(), uri, 2),
	}
	var deletedRevisionIDs []string
	err = st.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		deletedRevisionIDs, err = st.DeleteSecret(ctx, uri, []int{2})
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(deletedRevisionIDs, jc.SameContents, expectedToBeDeleted)

	_, _, err = st.ListSecrets(ctx, uri, ptr(1), domainsecret.NilLabels)
	c.Assert(err, jc.ErrorIsNil)
	_, _, err = st.ListSecrets(ctx, uri, ptr(2), domainsecret.NilLabels)
	c.Assert(err, jc.ErrorIs, secreterrors.SecretRevisionNotFound)
	_, _, err = st.ListSecrets(ctx, uri, ptr(3), domainsecret.NilLabels)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestDeleteAllRevisionsFromNil(c *gc.C) {
	s.assertDeleteAllRevisions(c, nil)
}

func (s *stateSuite) TestDeleteAllRevisions(c *gc.C) {
	s.assertDeleteAllRevisions(c, []int{1, 2, 3})
}

func (s *stateSuite) assertDeleteAllRevisions(c *gc.C, revs []int) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	expireTime := time.Now().Add(2 * time.Hour)
	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar"},
		ExpireTime: ptr(expireTime),
	}
	uri := coresecrets.NewURI().WithSource(s.modelUUID)
	ctx := context.Background()
	err := st.CreateCharmApplicationSecret(ctx, 1, uri, "mysql", sp)
	c.Assert(err, jc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar2"},
	}
	err = st.UpdateSecret(ctx, uri, sp2)
	c.Assert(err, jc.ErrorIsNil)
	sp3 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar3"},
	}
	err = st.UpdateSecret(ctx, uri, sp3)
	c.Assert(err, jc.ErrorIsNil)

	consumer := &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	}
	err = st.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, jc.ErrorIsNil)
	err = st.SaveSecretRemoteConsumer(ctx, uri, "remote-app/0", consumer)
	c.Assert(err, jc.ErrorIsNil)

	uri2 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = st.CreateCharmApplicationSecret(ctx, 1, uri2, "mysql", sp)
	c.Assert(err, jc.ErrorIsNil)

	expectedToBeDeleted := []string{
		getRevUUID(c, s.DB(), uri, 1),
		getRevUUID(c, s.DB(), uri, 2),
		getRevUUID(c, s.DB(), uri, 3),
	}
	var deletedRevisionIDs []string
	err = st.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		deletedRevisionIDs, err = st.DeleteSecret(ctx, uri, revs)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(deletedRevisionIDs, jc.SameContents, expectedToBeDeleted)

	for r := 1; r <= 3; r++ {
		_, _, err := st.ListSecrets(ctx, uri, ptr(r), domainsecret.NilLabels)
		c.Assert(err, jc.ErrorIs, secreterrors.SecretRevisionNotFound)
	}
	_, err = st.GetSecret(ctx, uri)
	c.Assert(err, jc.ErrorIs, secreterrors.SecretNotFound)
	_, _, err = st.GetSecretConsumer(ctx, uri, "someunit/0")
	c.Assert(err, jc.ErrorIs, secreterrors.SecretNotFound)

	_, err = st.GetSecret(ctx, uri2)
	c.Assert(err, jc.ErrorIsNil)
	data, _, err := st.GetSecretValue(ctx, uri2, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, jc.DeepEquals, coresecrets.SecretData{"foo": "bar"})
}

func (s *stateSuite) TestGetSecretRevisionID(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	expireTime := time.Now().Add(2 * time.Hour)
	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar"},
		ExpireTime: ptr(expireTime),
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmApplicationSecret(ctx, 1, uri, "mysql", sp)
	c.Assert(err, jc.ErrorIsNil)

	result, err := st.GetSecretRevisionID(ctx, uri, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, *sp.RevisionID)
}

func (s *stateSuite) TestGetSecretRevisionIDNotFound(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	uri := coresecrets.NewURI()
	ctx := context.Background()

	_, err := st.GetSecretRevisionID(ctx, uri, 1)
	c.Assert(err, jc.ErrorIs, secreterrors.SecretRevisionNotFound)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("secret revision not found: %s/%d", uri, 1))
}

func parseUUID(c *gc.C, s string) uuid.UUID {
	id, err := uuid.UUIDFromString(s)
	c.Assert(err, jc.ErrorIsNil)
	return id
}

func (s *stateSuite) prepareWatchForConsumedSecrets(c *gc.C, ctx context.Context, st *State) (*coresecrets.URI, *coresecrets.URI) {
	s.setupUnits(c, "mysql")
	s.setupUnits(c, "mediawiki")

	saveConsumer := func(uri *coresecrets.URI, revision int, consumerID string) {
		consumer := &coresecrets.SecretConsumerMetadata{
			CurrentRevision: revision,
		}
		err := st.SaveSecretConsumer(ctx, uri, consumerID, consumer)
		c.Assert(err, jc.ErrorIsNil)
	}

	sp := domainsecret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	uri1 := coresecrets.NewURI()
	err := st.CreateCharmApplicationSecret(ctx, 1, uri1, "mysql", sp)
	c.Assert(err, jc.ErrorIsNil)

	uri2 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = st.CreateCharmApplicationSecret(ctx, 1, uri2, "mysql", sp)
	c.Assert(err, jc.ErrorIsNil)

	// The consumed revision 1.
	saveConsumer(uri1, 1, "mediawiki/0")
	// The consumed revision 1.
	saveConsumer(uri2, 1, "mediawiki/0")

	// create revision 2, so mediawiki/0 will receive a consumed secret change event for uri1.
	updateSecretContent(c, st, uri1)
	return uri1, uri2
}

func (s *stateSuite) TestInitialWatchStatementForConsumedSecrets(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := context.Background()
	uri1, _ := s.prepareWatchForConsumedSecrets(c, ctx, st)
	tableName, f := st.InitialWatchStatementForConsumedSecretsChange("mediawiki/0")

	c.Assert(tableName, gc.Equals, "secret_revision")
	consumerIDs, err := f(ctx, s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(consumerIDs, jc.SameContents, []string{
		getRevUUID(c, s.DB(), uri1, 2),
	})
}

func (s *stateSuite) TestGetConsumedSecretURIsWithChanges(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := context.Background()
	uri1, uri2 := s.prepareWatchForConsumedSecrets(c, ctx, st)

	result, err := st.GetConsumedSecretURIsWithChanges(ctx, "mediawiki/0",
		getRevUUID(c, s.DB(), uri1, 1),
		getRevUUID(c, s.DB(), uri1, 2),
		getRevUUID(c, s.DB(), uri2, 1),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Assert(result, jc.SameContents, []string{
		uri1.String(),
	})
}

func (s *stateSuite) prepareWatchForRemoteConsumedSecrets(c *gc.C, ctx context.Context, st *State) (*coresecrets.URI, *coresecrets.URI) {
	s.setupUnits(c, "mediawiki")

	saveConsumer := func(uri *coresecrets.URI, revision int, consumerID string) {
		consumer := &coresecrets.SecretConsumerMetadata{
			CurrentRevision: revision,
		}
		err := st.SaveSecretConsumer(ctx, uri, consumerID, consumer)
		c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	return uri1, uri2
}

func (s *stateSuite) TestInitialWatchStatementForConsumedRemoteSecretsChange(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := context.Background()
	uri1, _ := s.prepareWatchForRemoteConsumedSecrets(c, ctx, st)

	tableName, f := st.InitialWatchStatementForConsumedRemoteSecretsChange("mediawiki/0")
	c.Assert(tableName, gc.Equals, "secret_reference")
	result, err := f(ctx, s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.SameContents, []string{
		uri1.ID,
	})
}

func (s *stateSuite) TestGetConsumedRemoteSecretURIsWithChanges(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := context.Background()
	uri1, uri2 := s.prepareWatchForRemoteConsumedSecrets(c, ctx, st)

	result, err := st.GetConsumedRemoteSecretURIsWithChanges(ctx, "mediawiki/0",
		uri1.ID,
		uri2.ID,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Assert(result, jc.SameContents, []string{
		uri1.String(),
	})
}

func (s *stateSuite) prepareWatchForRemoteConsumedSecretsChangesFromOfferingSide(c *gc.C, ctx context.Context, st *State) (*coresecrets.URI, *coresecrets.URI) {
	s.setupUnits(c, "mysql")

	saveRemoteConsumer := func(uri *coresecrets.URI, revision int, consumerID string) {
		consumer := &coresecrets.SecretConsumerMetadata{
			CurrentRevision: revision,
		}
		err := st.SaveSecretRemoteConsumer(ctx, uri, consumerID, consumer)
		c.Assert(err, jc.ErrorIsNil)
	}

	sp := domainsecret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri1 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err := st.CreateCharmApplicationSecret(ctx, 1, uri1, "mysql", sp)
	c.Assert(err, jc.ErrorIsNil)
	uri1.SourceUUID = s.modelUUID

	uri2 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = st.CreateCharmApplicationSecret(ctx, 1, uri2, "mysql", sp)
	c.Assert(err, jc.ErrorIsNil)
	uri2.SourceUUID = s.modelUUID

	// The consumed revision 1.
	saveRemoteConsumer(uri1, 1, "mediawiki/0")
	// The consumed revision 1.
	saveRemoteConsumer(uri2, 1, "mediawiki/0")

	// create revision 2.
	updateSecretContent(c, st, uri1)

	err = st.UpdateRemoteSecretRevision(ctx, uri1, 2)
	c.Assert(err, jc.ErrorIsNil)
	return uri1, uri2
}

func (s *stateSuite) TestInitialWatchStatementForRemoteConsumedSecretsChangesFromOfferingSide(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := context.Background()
	uri1, _ := s.prepareWatchForRemoteConsumedSecretsChangesFromOfferingSide(c, ctx, st)

	tableName, f := st.InitialWatchStatementForRemoteConsumedSecretsChangesFromOfferingSide("mediawiki")
	c.Assert(tableName, gc.Equals, "secret_revision")
	result, err := f(ctx, s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.SameContents, []string{
		getRevUUID(c, s.DB(), uri1, 2),
	})
}

func (s *stateSuite) TestGetRemoteConsumedSecretURIsWithChangesFromOfferingSide(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := context.Background()
	uri1, uri2 := s.prepareWatchForRemoteConsumedSecretsChangesFromOfferingSide(c, ctx, st)

	result, err := st.GetRemoteConsumedSecretURIsWithChangesFromOfferingSide(ctx, "mediawiki",
		getRevUUID(c, s.DB(), uri1, 1),
		getRevUUID(c, s.DB(), uri1, 2),
		getRevUUID(c, s.DB(), uri2, 1),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Assert(result, jc.SameContents, []string{
		uri1.String(),
	})
}

func (s *stateSuite) prepareWatchForWatchStatementForSecretsRotationChanges(c *gc.C, ctx context.Context, st *State) (time.Time, *coresecrets.URI, *coresecrets.URI) {
	s.setupUnits(c, "mysql")
	s.setupUnits(c, "mediawiki")

	sp := domainsecret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri1 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err := st.CreateCharmApplicationSecret(ctx, 1, uri1, "mysql", sp)
	c.Assert(err, jc.ErrorIsNil)

	uri2 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = st.CreateCharmUnitSecret(ctx, 1, uri2, "mediawiki/0", sp)
	c.Assert(err, jc.ErrorIsNil)
	updateSecretContent(c, st, uri2)

	now := time.Now()
	err = st.SecretRotated(ctx, uri1, now.Add(1*time.Hour))
	c.Assert(err, jc.ErrorIsNil)
	err = st.SecretRotated(ctx, uri2, now.Add(2*time.Hour))
	c.Assert(err, jc.ErrorIsNil)

	return now, uri1, uri2
}

func (s *stateSuite) TestInitialWatchStatementForSecretsRotationChanges(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := context.Background()
	_, uri1, uri2 := s.prepareWatchForWatchStatementForSecretsRotationChanges(c, ctx, st)

	tableName, f := st.InitialWatchStatementForSecretsRotationChanges(domainsecret.ApplicationOwners{"mysql"}, domainsecret.UnitOwners{"mediawiki/0"})
	c.Check(tableName, gc.Equals, "secret_rotation")
	result, err := f(ctx, s.TxnRunner())
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []string{
		uri1.ID, uri2.ID,
	})

	tableName, f = st.InitialWatchStatementForSecretsRotationChanges(domainsecret.ApplicationOwners{"mysql"}, nil)
	c.Check(tableName, gc.Equals, "secret_rotation")
	result, err = f(ctx, s.TxnRunner())
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []string{
		uri1.ID,
	})

	tableName, f = st.InitialWatchStatementForSecretsRotationChanges(nil, domainsecret.UnitOwners{"mediawiki/0"})
	c.Check(tableName, gc.Equals, "secret_rotation")
	result, err = f(ctx, s.TxnRunner())
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []string{
		uri2.ID,
	})

	tableName, f = st.InitialWatchStatementForSecretsRotationChanges(nil, nil)
	c.Check(tableName, gc.Equals, "secret_rotation")
	result, err = f(ctx, s.TxnRunner())
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, gc.HasLen, 0)
}

func (s *stateSuite) TestGetSecretsRotationChanges(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := context.Background()
	now, uri1, uri2 := s.prepareWatchForWatchStatementForSecretsRotationChanges(c, ctx, st)

	result, err := st.GetSecretsRotationChanges(ctx, domainsecret.ApplicationOwners{"mysql"}, domainsecret.UnitOwners{"mediawiki/0"})
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []domainsecret.RotationInfo{
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
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []domainsecret.RotationInfo{
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
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []domainsecret.RotationInfo{
		{
			URI:             uri2,
			Revision:        2,
			NextTriggerTime: now.Add(2 * time.Hour).UTC(),
		},
	})

	result, err = st.GetSecretsRotationChanges(ctx, domainsecret.ApplicationOwners{"mysql"}, nil)
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []domainsecret.RotationInfo{
		{
			URI:             uri1,
			Revision:        1,
			NextTriggerTime: now.Add(1 * time.Hour).UTC(),
		},
	})

	// The uri2 is not owned by mysql, so it should not be returned.
	result, err = st.GetSecretsRotationChanges(ctx, domainsecret.ApplicationOwners{"mysql"}, nil, uri1.ID, uri2.ID)
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []domainsecret.RotationInfo{
		{
			URI:             uri1,
			Revision:        1,
			NextTriggerTime: now.Add(1 * time.Hour).UTC(),
		},
	})

	result, err = st.GetSecretsRotationChanges(ctx, nil, domainsecret.UnitOwners{"mediawiki/0"})
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []domainsecret.RotationInfo{
		{
			URI:             uri2,
			Revision:        2,
			NextTriggerTime: now.Add(2 * time.Hour).UTC(),
		},
	})

	// The uri1 is not owned by mediawiki/0, so it should not be returned.
	result, err = st.GetSecretsRotationChanges(ctx, nil, domainsecret.UnitOwners{"mediawiki/0"}, uri1.ID, uri2.ID)
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []domainsecret.RotationInfo{
		{
			URI:             uri2,
			Revision:        2,
			NextTriggerTime: now.Add(2 * time.Hour).UTC(),
		},
	})

	result, err = st.GetSecretsRotationChanges(ctx, nil, nil)
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, gc.HasLen, 0)
}

func (s *stateSuite) prepareWatchForWatchStatementForSecretsRevisionExpiryChanges(c *gc.C, ctx context.Context, st *State) (time.Time, *coresecrets.URI, *coresecrets.URI) {
	s.setupUnits(c, "mysql")
	s.setupUnits(c, "mediawiki")

	now := time.Now()
	uri1 := coresecrets.NewURI()
	err := st.CreateCharmApplicationSecret(ctx, 1, uri1, "mysql", domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar", "hello": "world"},
		ExpireTime: ptr(now.Add(1 * time.Hour)),
	})
	c.Assert(err, jc.ErrorIsNil)

	uri2 := coresecrets.NewURI()
	err = st.CreateCharmUnitSecret(ctx, 1, uri2, "mediawiki/0", domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar", "hello": "world"},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = st.UpdateSecret(context.Background(), uri2, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo-new": "bar-new"},
		ExpireTime: ptr(now.Add(2 * time.Hour)),
	})
	c.Assert(err, jc.ErrorIsNil)
	return now, uri1, uri2
}

func (s *stateSuite) TestInitialWatchStatementForSecretsRevisionExpiryChanges(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := context.Background()
	_, uri1, uri2 := s.prepareWatchForWatchStatementForSecretsRevisionExpiryChanges(c, ctx, st)

	tableName, f := st.InitialWatchStatementForSecretsRevisionExpiryChanges(domainsecret.ApplicationOwners{"mysql"}, domainsecret.UnitOwners{"mediawiki/0"})
	c.Check(tableName, gc.Equals, "secret_revision_expire")
	result, err := f(ctx, s.TxnRunner())
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []string{
		getRevUUID(c, s.DB(), uri1, 1),
		getRevUUID(c, s.DB(), uri2, 2),
	})

	tableName, f = st.InitialWatchStatementForSecretsRevisionExpiryChanges(domainsecret.ApplicationOwners{"mysql"}, nil)
	c.Check(tableName, gc.Equals, "secret_revision_expire")
	result, err = f(ctx, s.TxnRunner())
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []string{
		getRevUUID(c, s.DB(), uri1, 1),
	})

	tableName, f = st.InitialWatchStatementForSecretsRevisionExpiryChanges(nil, domainsecret.UnitOwners{"mediawiki/0"})
	c.Check(tableName, gc.Equals, "secret_revision_expire")
	result, err = f(ctx, s.TxnRunner())
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []string{
		getRevUUID(c, s.DB(), uri2, 2),
	})

	tableName, f = st.InitialWatchStatementForSecretsRevisionExpiryChanges(nil, nil)
	c.Check(tableName, gc.Equals, "secret_revision_expire")
	result, err = f(ctx, s.TxnRunner())
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, gc.HasLen, 0)
}

func (s *stateSuite) TestGetSecretsRevisionExpiryChanges(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := context.Background()
	now, uri1, uri2 := s.prepareWatchForWatchStatementForSecretsRevisionExpiryChanges(c, ctx, st)

	result, err := st.GetSecretsRevisionExpiryChanges(ctx, domainsecret.ApplicationOwners{"mysql"}, domainsecret.UnitOwners{"mediawiki/0"})
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []domainsecret.ExpiryInfo{
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
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []domainsecret.ExpiryInfo{
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
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []domainsecret.ExpiryInfo{
		{
			URI:             uri2,
			Revision:        2,
			RevisionID:      getRevUUID(c, s.DB(), uri2, 2),
			NextTriggerTime: now.Add(2 * time.Hour).UTC(),
		},
	})

	result, err = st.GetSecretsRevisionExpiryChanges(ctx, domainsecret.ApplicationOwners{"mysql"}, nil)
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []domainsecret.ExpiryInfo{
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
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []domainsecret.ExpiryInfo{
		{
			URI:             uri1,
			Revision:        1,
			RevisionID:      getRevUUID(c, s.DB(), uri1, 1),
			NextTriggerTime: now.Add(1 * time.Hour).UTC(),
		},
	})

	result, err = st.GetSecretsRevisionExpiryChanges(ctx, nil, domainsecret.UnitOwners{"mediawiki/0"})
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []domainsecret.ExpiryInfo{
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
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []domainsecret.ExpiryInfo{
		{
			URI:             uri2,
			Revision:        2,
			RevisionID:      getRevUUID(c, s.DB(), uri2, 2),
			NextTriggerTime: now.Add(2 * time.Hour).UTC(),
		},
	})

	result, err = st.GetSecretsRevisionExpiryChanges(ctx, nil, nil)
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, gc.HasLen, 0)
}

func (s *stateSuite) TestSecretRotated(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := context.Background()

	s.setupUnits(c, "mysql")
	uri := coresecrets.NewURI()
	err := st.CreateCharmApplicationSecret(ctx, 1, uri, "mysql", domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar", "hello": "world"},
	})
	c.Assert(err, jc.ErrorIsNil)

	next := time.Now().Add(1 * time.Hour)
	err = st.SecretRotated(ctx, uri, next)
	c.Assert(err, jc.ErrorIsNil)

	row := s.DB().QueryRowContext(context.Background(), `
SELECT next_rotation_time
FROM secret_rotation
WHERE secret_id = ?`, uri.ID)
	var nextRotationTime time.Time
	err = row.Scan(&nextRotationTime)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nextRotationTime.Equal(next), jc.IsTrue)
}

func (s *stateSuite) TestGetObsoleteUserSecretRevisionsReadyToPrune(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	ctx := context.Background()
	uri := coresecrets.NewURI()

	err := st.CreateUserSecret(ctx, 1, uri, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar", "hello": "world"},
	})
	c.Assert(err, jc.ErrorIsNil)

	// The secret is not obsolete yet.
	result, err := st.GetObsoleteUserSecretRevisionsReadyToPrune(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 0)

	// create revision 2 for user secret.
	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo-new": "bar-new"},
	}
	err = st.UpdateSecret(context.Background(), uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	result, err = st.GetObsoleteUserSecretRevisionsReadyToPrune(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 0)

	sp = domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		AutoPrune:  ptr(true),
	}
	err = st.UpdateSecret(context.Background(), uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	result, err = st.GetObsoleteUserSecretRevisionsReadyToPrune(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.SameContents, []string{uri.ID + "/1"})
}

func (s *stateSuite) TestChangeSecretBackend(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := context.Background()

	s.setupUnits(c, "mysql")
	uriCharm := coresecrets.NewURI()
	uriUser := coresecrets.NewURI()

	dataInput := coresecrets.SecretData{"foo": "bar", "hello": "world"}
	valueRefInput := &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "revision-id",
	}

	err := st.CreateCharmApplicationSecret(ctx, 1, uriCharm, "mysql", domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       dataInput,
	})
	c.Assert(err, jc.ErrorIsNil)
	data, valueRef, err := st.GetSecretValue(ctx, uriCharm, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, jc.DeepEquals, dataInput)
	c.Assert(valueRef, gc.IsNil)

	err = st.CreateUserSecret(ctx, 1, uriUser, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       dataInput,
	})
	c.Assert(err, jc.ErrorIsNil)
	data, valueRef, err = st.GetSecretValue(ctx, uriUser, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, jc.DeepEquals, dataInput)
	c.Assert(valueRef, gc.IsNil)

	// change to external backend.
	err = st.ChangeSecretBackend(ctx, parseUUID(c, getRevUUID(c, s.DB(), uriCharm, 1)), valueRefInput, nil)
	c.Assert(err, jc.ErrorIsNil)
	data, valueRef, err = st.GetSecretValue(ctx, uriCharm, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, gc.IsNil)
	c.Assert(valueRef, gc.DeepEquals, valueRefInput)

	// change back to internal backend.
	err = st.ChangeSecretBackend(ctx, parseUUID(c, getRevUUID(c, s.DB(), uriCharm, 1)), nil, dataInput)
	c.Assert(err, jc.ErrorIsNil)
	data, valueRef, err = st.GetSecretValue(ctx, uriCharm, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, jc.DeepEquals, dataInput)
	c.Assert(valueRef, gc.IsNil)

	// change to external backend for the user secret.
	err = st.ChangeSecretBackend(ctx, parseUUID(c, getRevUUID(c, s.DB(), uriUser, 1)), valueRefInput, nil)
	c.Assert(err, jc.ErrorIsNil)
	data, valueRef, err = st.GetSecretValue(ctx, uriUser, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, gc.IsNil)
	c.Assert(valueRef, gc.DeepEquals, valueRefInput)

	// change back to internal backend for the user secret.
	err = st.ChangeSecretBackend(ctx, parseUUID(c, getRevUUID(c, s.DB(), uriUser, 1)), nil, dataInput)
	c.Assert(err, jc.ErrorIsNil)
	data, valueRef, err = st.GetSecretValue(ctx, uriUser, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, jc.DeepEquals, dataInput)
	c.Assert(valueRef, gc.IsNil)
}

func (s *stateSuite) TestChangeSecretBackendFailed(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	ctx := context.Background()

	s.setupUnits(c, "mysql")

	dataInput := coresecrets.SecretData{"foo": "bar", "hello": "world"}
	valueRefInput := &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "revision-id",
	}

	err := st.ChangeSecretBackend(ctx, uuid.MustNewUUID(), nil, nil)
	c.Assert(err, gc.ErrorMatches, "either valueRef or data must be set")
	err = st.ChangeSecretBackend(ctx, uuid.MustNewUUID(), valueRefInput, dataInput)
	c.Assert(err, gc.ErrorMatches, "both valueRef and data cannot be set")
}
