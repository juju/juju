// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coredatabase "github.com/juju/juju/core/database"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/schema/testing"
	domainsecret "github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	uniterrors "github.com/juju/juju/domain/unit/errors"
	"github.com/juju/juju/internal/uuid"
	coretesting "github.com/juju/juju/testing"
)

type stateSuite struct {
	testing.ModelSuite

	modelUUID string
}

var _ = gc.Suite(&stateSuite{})

func newSecretState(c *gc.C, factory coredatabase.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    coretesting.NewCheckLogger(c),
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
			INSERT INTO model (uuid, controller_uuid, name, type, cloud)
			VALUES (?, ?, "test", "iaas", "fluffy")
		`, modelUUID.String(), coretesting.ControllerTag.Id())
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

func (s *stateSuite) TestGetSecretRevisionNotFound(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	_, err := st.GetSecretRevision(context.Background(), coresecrets.NewURI(), 666)
	c.Assert(err, jc.ErrorIs, secreterrors.SecretRevisionNotFound)

	_, _, err = st.GetSecretValue(context.Background(), coresecrets.NewURI(), 666)
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
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
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
	md, err := st.GetSecret(ctx, uri)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md.Version, gc.Equals, 1)
	c.Assert(md.Label, gc.Equals, value(sp.Label))
	c.Assert(md.Description, gc.Equals, value(sp.Description))
	c.Assert(md.LatestRevision, gc.Equals, 1)
	c.Assert(md.AutoPrune, gc.Equals, value(sp.AutoPrune))
	c.Assert(md.Owner, jc.DeepEquals, owner)
	if sp.RotatePolicy == nil {
		c.Assert(md.RotatePolicy, gc.Equals, coresecrets.RotateNever)
	} else {
		c.Assert(md.RotatePolicy, gc.Equals, fromDbRotatePolicy(*sp.RotatePolicy))
	}
	if sp.NextRotateTime == nil {
		c.Assert(md.NextRotateTime, gc.IsNil)
	} else {
		c.Assert(*md.NextRotateTime, gc.Equals, sp.NextRotateTime.UTC())
	}
	now := time.Now()
	c.Assert(md.CreateTime, jc.Almost, now)
	c.Assert(md.UpdateTime, jc.Almost, now)

	rev, err := st.GetSecretRevision(ctx, uri, revision)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rev.Revision, gc.Equals, revision)
	c.Assert(rev.CreateTime, jc.Almost, now)
	c.Assert(rev.UpdateTime, jc.Almost, now)
	if rev.ExpireTime == nil {
		c.Assert(md.LatestExpireTime, gc.IsNil)
	} else {
		c.Assert(*md.LatestExpireTime, gc.Equals, rev.ExpireTime.UTC())
	}
}

func (s *stateSuite) TestCreateUserSecretWithContent(c *gc.C) {
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
		AutoPrune:   ptr(true),
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
		AutoPrune:   ptr(true),
	}, {
		Description: ptr("my secretMetadata2"),
		Label:       ptr("my label2"),
		Data:        coresecrets.SecretData{"foo": "bar2"},
		AutoPrune:   ptr(true),
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
		c.Assert(revs[0].UpdateTime, jc.Almost, now)
	}
}

func (s *stateSuite) TestListSecretsByURI(c *gc.C) {

	st := newSecretState(c, s.TxnRunnerFactory())

	sp := []domainsecret.UpsertSecretParams{{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   ptr(true),
	}, {
		Description: ptr("my secretMetadata2"),
		Label:       ptr("my label2"),
		Data:        coresecrets.SecretData{"foo": "bar2"},
		AutoPrune:   ptr(true),
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
	c.Assert(revs[0].UpdateTime, jc.Almost, now)
}

func (s *stateSuite) setupUnits(c *gc.C, appName string) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		applicationUUID := uuid.MustNewUUID().String()
		_, err := tx.ExecContext(context.Background(), `
INSERT INTO application (uuid, name, life_id)
VALUES (?, ?, ?)
`, applicationUUID, appName, life.Alive)
		c.Assert(err, jc.ErrorIsNil)

		// Do 2 units.
		for i := 0; i < 2; i++ {
			netNodeUUID := uuid.MustNewUUID().String()
			_, err = tx.ExecContext(context.Background(), "INSERT INTO net_node (uuid) VALUES (?)", netNodeUUID)
			c.Assert(err, jc.ErrorIsNil)
			unitUUID := uuid.MustNewUUID().String()
			_, err = tx.ExecContext(context.Background(), `
INSERT INTO unit (uuid, life_id, unit_id, net_node_uuid, application_uuid)
VALUES (?, ?, ?, ?, (SELECT uuid from application WHERE name = ?))
`, unitUUID, life.Alive, appName+fmt.Sprintf("/%d", i), netNodeUUID, appName)
			c.Assert(err, jc.ErrorIsNil)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestListUserSecretsNone(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)

	secrets, revisions, err := st.ListUserSecrets(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(secrets, gc.HasLen, 0)
	c.Assert(revisions, gc.HasLen, 0)
}

func (s *stateSuite) TestListUserSecrets(c *gc.C) {

	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := []domainsecret.UpsertSecretParams{{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   ptr(true),
	}, {
		Description: ptr("my secretMetadata2"),
		Label:       ptr("my label2"),
		Data:        coresecrets.SecretData{"foo": "bar2"},
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

	secrets, revisions, err := st.ListUserSecrets(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(secrets), gc.Equals, 1)
	c.Assert(len(revisions), gc.Equals, 1)

	now := time.Now()

	md := secrets[0]
	c.Assert(md.Version, gc.Equals, 1)
	c.Assert(md.Label, gc.Equals, value(sp[0].Label))
	c.Assert(md.Description, gc.Equals, value(sp[0].Description))
	c.Assert(md.LatestRevision, gc.Equals, 1)
	c.Assert(md.AutoPrune, gc.Equals, value(sp[0].AutoPrune))
	c.Assert(md.Owner, jc.DeepEquals, coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: s.modelUUID})
	c.Assert(md.CreateTime, jc.Almost, now)
	c.Assert(md.UpdateTime, jc.Almost, now)

	revs := revisions[0]
	c.Assert(revs, gc.HasLen, 1)
	c.Assert(revs[0].Revision, gc.Equals, 1)
	c.Assert(revs[0].CreateTime, jc.Almost, now)
	c.Assert(revs[0].UpdateTime, jc.Almost, now)
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
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmApplicationSecret(ctx, 1, uri, "mysql", sp)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *stateSuite) TestCreateCharmUserSecretWithContent(c *gc.C) {
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
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIs, uniterrors.NotFound)
}

func (s *stateSuite) TestCreateCharmApplicationSecretLabelAlreadyExists(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
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
		AutoPrune:   ptr(true),
	}, {
		Description: ptr("my secretMetadata2"),
		Label:       ptr("my label2"),
		Data:        coresecrets.SecretData{"foo": "bar2"},
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
	c.Assert(revs[0].CreateTime, jc.Almost, now)
	c.Assert(revs[0].UpdateTime, jc.Almost, now)
}

func (s *stateSuite) TestListCharmSecretsByApplication(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := []domainsecret.UpsertSecretParams{{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   ptr(true),
	}, {
		Description: ptr("my secretMetadata2"),
		Label:       ptr("my label2"),
		Data:        coresecrets.SecretData{"foo": "bar2"},
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
	c.Assert(revs[0].UpdateTime, jc.Almost, now)
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
	}, {
		Description:    ptr("my secretMetadata2"),
		Label:          ptr("my label2"),
		Data:           coresecrets.SecretData{"foo": "bar2"},
		RotatePolicy:   ptr(domainsecret.RotateDaily),
		ExpireTime:     ptr(expireTime),
		NextRotateTime: ptr(rotateTime),
	}, {
		Description: ptr("my secretMetadata3"),
		Label:       ptr("my label3"),
		Data:        coresecrets.SecretData{"foo": "bar3"},
	}, {
		Description: ptr("my secretMetadata4"),
		Label:       ptr("my label4"),
		Data:        coresecrets.SecretData{"foo": "bar4"},
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
	c.Assert(revs[0].UpdateTime, jc.Almost, now)

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
	c.Assert(revs[0].UpdateTime, jc.Almost, now)
}

func (s *stateSuite) TestSaveSecretConsumer(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
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
	}
	uri := coresecrets.NewURI()
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
		ValueRef: &coresecrets.ValueRef{BackendID: "new-backend", RevisionID: "new-revision"},
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
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()

	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	}

	err = st.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, jc.ErrorIs, uniterrors.NotFound)
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
// TestSaveSecretConsumerDifferentModel bu there's no remote revision
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
}

func (s *stateSuite) TestGetSecretConsumerFirstTime(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		ValueRef:    &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		AutoPrune:   ptr(true),
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
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()

	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = st.GetSecretConsumer(ctx, uri, "mysql/0")
	c.Assert(err, jc.ErrorIs, uniterrors.NotFound)
}

func (s *stateSuite) TestGetUserSecretURIByLabel(c *gc.C) {
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

	_, err = st.GetURIByConsumerLabel(ctx, "another label", "mysql/0")
	c.Assert(err, jc.ErrorIs, secreterrors.SecretNotFound)

}

func (s *stateSuite) TestGetURIByConsumerLabelUnitNotExists(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	_, err := st.GetURIByConsumerLabel(context.Background(), "my label", "mysql/2")
	c.Assert(err, jc.ErrorIs, uniterrors.NotFound)
}

func (s *stateSuite) TestUpdateSecretNotFound(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	uri := coresecrets.NewURI()
	err := st.UpdateSecret(context.Background(), uri, domainsecret.UpsertSecretParams{
		Label: ptr("label"),
	})
	c.Assert(err, jc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestUpdateSecretNothingToDo(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	uri := coresecrets.NewURI()
	err := st.UpdateSecret(context.Background(), uri, domainsecret.UpsertSecretParams{})
	c.Assert(err, gc.ErrorMatches, "must specify a new value or metadata to update a secret")
}

func (s *stateSuite) TestUpdateUserSecretMetadataOnly(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
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
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   ptr(true),
	}
	sp2 := domainsecret.UpsertSecretParams{
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
		Label: ptr("my label2"),
	}
	err = st.UpdateSecret(ctx, uri, sp)
	c.Assert(err, jc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
}

func (s *stateSuite) TestUpdateCharmApplicationSecretMetadataOnly(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmApplicationSecret(ctx, 1, uri, "mysql", sp)
	c.Assert(err, jc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
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
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
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
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	sp2 := domainsecret.UpsertSecretParams{
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
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	sp2 := domainsecret.UpsertSecretParams{
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

func (s *stateSuite) TestUpdateSecretContent(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)

	expireTime := time.Now().Add(2 * time.Hour)
	sp2 := domainsecret.UpsertSecretParams{
		ExpireTime: &expireTime,
		Data:       coresecrets.SecretData{"foo2": "bar2", "hello": "world"},
	}
	err = st.UpdateSecret(context.Background(), uri, sp2)
	c.Assert(err, jc.ErrorIsNil)

	md, err := st.GetSecret(ctx, uri)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md.Version, gc.Equals, 1)
	c.Assert(md.Label, gc.Equals, value(sp.Label))
	c.Assert(md.Description, gc.Equals, value(sp.Description))
	c.Assert(md.LatestRevision, gc.Equals, 2)

	now := time.Now()
	c.Assert(md.UpdateTime, jc.Almost, now)

	rev, err := st.GetSecretRevision(ctx, uri, 2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rev.Revision, gc.Equals, 2)
	c.Assert(rev.ExpireTime, gc.NotNil)
	c.Assert(*rev.ExpireTime, gc.Equals, expireTime.UTC())
	c.Assert(rev.UpdateTime, jc.Almost, now)

	content, valueRef, err := st.GetSecretValue(ctx, uri, 2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(valueRef, gc.IsNil)
	c.Assert(content, jc.DeepEquals, coresecrets.SecretData{"foo2": "bar2", "hello": "world"})

	// Revision 1 is obsolete.
	var count int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
			SELECT count(*) FROM secret_revision WHERE secret_id = ?
			AND revision = ? AND obsolete = True AND pending_delete = True
		`, uri.ID, 1)
		if err := row.Scan(&count); err != nil {
			return err
		}
		return row.Err()
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(count, gc.Equals, 1)

	// But not revision 2.
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
			SELECT count(*) FROM secret_revision WHERE secret_id = ?
			AND revision = ? AND obsolete = True AND pending_delete = True
		`, uri.ID, 2)
		if err := row.Scan(&count); err != nil {
			return err
		}
		return row.Err()
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(count, gc.Equals, 0)
}

func (s *stateSuite) TestUpdateSecretContentObsolete(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
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
		ExpireTime: &expireTime,
		Data:       coresecrets.SecretData{"foo2": "bar2", "hello": "world"},
	}
	err = st.UpdateSecret(context.Background(), uri, sp2)
	c.Assert(err, jc.ErrorIsNil)

	md, err := st.GetSecret(ctx, uri)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md.Version, gc.Equals, 1)
	c.Assert(md.Label, gc.Equals, value(sp.Label))
	c.Assert(md.Description, gc.Equals, value(sp.Description))
	c.Assert(md.LatestRevision, gc.Equals, 2)

	now := time.Now()
	c.Assert(md.UpdateTime, jc.Almost, now)

	rev, err := st.GetSecretRevision(ctx, uri, 2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rev.Revision, gc.Equals, 2)
	c.Assert(rev.ExpireTime, gc.NotNil)
	c.Assert(*rev.ExpireTime, gc.Equals, expireTime.UTC())
	c.Assert(rev.UpdateTime, jc.Almost, now)

	content, valueRef, err := st.GetSecretValue(ctx, uri, 2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(valueRef, gc.IsNil)
	c.Assert(content, jc.DeepEquals, coresecrets.SecretData{"foo2": "bar2", "hello": "world"})

	// Latest revision is 3 now, revision 2 shoule be obsolete.
	sp3 := domainsecret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo3": "bar3", "hello": "world"},
	}
	err = st.UpdateSecret(context.Background(), uri, sp3)
	c.Assert(err, jc.ErrorIsNil)
	content, valueRef, err = st.GetSecretValue(ctx, uri, 3)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(valueRef, gc.IsNil)
	c.Assert(content, jc.DeepEquals, coresecrets.SecretData{"foo3": "bar3", "hello": "world"})

	md, err = st.GetSecret(ctx, uri)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md.Version, gc.Equals, 1)
	c.Assert(md.Label, gc.Equals, value(sp.Label))
	c.Assert(md.Description, gc.Equals, value(sp.Description))
	c.Assert(md.LatestRevision, gc.Equals, 3)

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
	_ = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
SELECT obsolete, pending_delete
FROM secret_revision
WHERE secret_id = ? AND revision = ?`, uri.ID, rev)
		err := row.Scan(&obsolete, &pendingDelete)
		c.Check(err, jc.ErrorIsNil)
		return nil
	})
	return obsolete, pendingDelete
}

func (s *stateSuite) TestUpdateSecretContentValueRef(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "revision-id"},
	}
	err = st.UpdateSecret(context.Background(), uri, sp2)
	c.Assert(err, jc.ErrorIsNil)

	md, err := st.GetSecret(ctx, uri)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md.Version, gc.Equals, 1)
	c.Assert(md.Label, gc.Equals, value(sp.Label))
	c.Assert(md.Description, gc.Equals, value(sp.Description))
	c.Assert(md.LatestRevision, gc.Equals, 2)

	now := time.Now()
	c.Assert(md.UpdateTime, jc.Almost, now)

	rev, err := st.GetSecretRevision(ctx, uri, 2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rev.Revision, gc.Equals, 2)
	c.Assert(rev.ExpireTime, gc.IsNil)
	c.Assert(rev.UpdateTime, jc.Almost, now)

	content, valueRef, err := st.GetSecretValue(ctx, uri, 2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(valueRef, jc.DeepEquals, &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "revision-id"})
	c.Assert(content, gc.HasLen, 0)
}

func (s *stateSuite) TestUpdateSecretNoRotate(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RotatePolicy: ptr(domainsecret.RotateDaily),
		Data:         coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
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
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateCharmUnitSecret(ctx, 1, uri, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		AutoPrune: ptr(true),
	}
	err = st.UpdateSecret(context.Background(), uri, sp2)
	c.Assert(err, jc.ErrorIs, secreterrors.AutoPruneNotSupported)
}

func (s *stateSuite) TestSaveSecretRemoteConsumer(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
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
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)
	sp.Label = ptr("")
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
	var count int

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
			SELECT count(*) FROM secret_revision WHERE secret_id = ?
			AND revision = ? AND obsolete = True AND pending_delete = True
		`, uri.ID, 1)
		if err := row.Scan(&count); err != nil {
			return err
		}
		return row.Err()
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(count, gc.Equals, 1)

	// But not revision 2.
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
			SELECT count(*) FROM secret_revision WHERE secret_id = ?
			AND revision = ? AND obsolete = True AND pending_delete = True
		`, uri.ID, 2)
		if err := row.Scan(&count); err != nil {
			return err
		}
		return row.Err()
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(count, gc.Equals, 0)
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
	c.Assert(err, jc.ErrorIs, uniterrors.NotFound)
}

func (s *stateSuite) TestGrantApplicationNotFound(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
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
	c.Assert(err, jc.ErrorIs, uniterrors.NotFound)
}

func (s *stateSuite) TestGetAccessNoGrant(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
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

func (s *stateSuite) TestRevokeAccess(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
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
	err := st.CreateCharmApplicationSecret(ctx, 1, uri1, "mysql", sp)
	c.Assert(err, jc.ErrorIsNil)
	updateSecretContent(c, st, uri1)

	uri2 := coresecrets.NewURI()
	err = st.CreateCharmUnitSecret(ctx, 1, uri2, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)
	updateSecretContent(c, st, uri2)

	uri3 := coresecrets.NewURI()
	err = st.CreateCharmApplicationSecret(ctx, 1, uri3, "mediawiki", sp)
	c.Assert(err, jc.ErrorIsNil)
	updateSecretContent(c, st, uri3)

	uri4 := coresecrets.NewURI()
	err = st.CreateCharmUnitSecret(ctx, 1, uri4, "mediawiki/0", sp)
	c.Assert(err, jc.ErrorIsNil)
	updateSecretContent(c, st, uri4)
	return uri1, uri2, uri3, uri4
}

func assertRevUUIDsForSuppliedQuery(c *gc.C, db *sql.DB, query string, expected []string) {
	rows, err := db.Query(query)
	c.Assert(err, jc.ErrorIsNil)
	defer rows.Close()
	var revUUIDs []string
	for rows.Next() {
		var revUUID string
		err := rows.Scan(&revUUID)
		c.Check(err, jc.ErrorIsNil)
		revUUIDs = append(revUUIDs, revUUID)
	}
	c.Check(revUUIDs, jc.SameContents, expected)
}

func (s *stateSuite) TestInitialWatchStatementForObsoleteRevisionWithBothAppAndUnitOwners(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	uri1, uri2, uri3, uri4 := s.prepareSecretObsoleteRevisions(c, st)
	tableName, q := st.InitialWatchStatementForObsoleteRevision(context.Background(),
		[]string{"mysql", "mediawiki"},
		[]string{"mysql/0", "mediawiki/0"},
	)
	c.Check(tableName, gc.Equals, "secret_revision")
	c.Check(q, gc.DeepEquals, `
SELECT sr.uuid
FROM secret_revision sr
LEFT JOIN secret_application_owner sao ON sr.secret_id = sao.secret_id
LEFT JOIN application ON application.uuid = sao.application_uuid
LEFT JOIN secret_unit_owner suo ON sr.secret_id = suo.secret_id
LEFT JOIN unit ON unit.uuid = suo.unit_uuid
WHERE sr.obsolete = true
AND (
    sao.application_uuid IS NOT NULL AND application.name IN ('mysql','mediawiki')
    OR suo.unit_uuid IS NOT NULL AND unit.unit_id IN ('mysql/0','mediawiki/0')
)`[1:])

	assertRevUUIDsForSuppliedQuery(c, s.DB(), q, []string{
		getRevUUID(c, s.DB(), uri1, 1),
		getRevUUID(c, s.DB(), uri2, 1),
		getRevUUID(c, s.DB(), uri3, 1),
		getRevUUID(c, s.DB(), uri4, 1),
	})
}

func (s *stateSuite) TestInitialWatchStatementForObsoleteRevisionWithAppOwners(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	uri1, _, uri3, _ := s.prepareSecretObsoleteRevisions(c, st)
	tableName, q := st.InitialWatchStatementForObsoleteRevision(context.Background(),
		[]string{"mysql", "mediawiki"},
		nil,
	)
	c.Check(tableName, gc.Equals, "secret_revision")
	c.Check(q, gc.DeepEquals, `
SELECT sr.uuid
FROM secret_revision sr
LEFT JOIN secret_application_owner sao ON sr.secret_id = sao.secret_id
LEFT JOIN application ON application.uuid = sao.application_uuid
WHERE sr.obsolete = true
AND (sao.application_uuid IS NOT NULL AND application.name IN ('mysql','mediawiki'))`[1:])
	assertRevUUIDsForSuppliedQuery(c, s.DB(), q, []string{
		getRevUUID(c, s.DB(), uri1, 1),
		getRevUUID(c, s.DB(), uri3, 1),
	})
}

func (s *stateSuite) TestInitialWatchStatementForObsoleteRevisionWithUnitOwners(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	_, uri2, _, uri4 := s.prepareSecretObsoleteRevisions(c, st)
	tableName, q := st.InitialWatchStatementForObsoleteRevision(context.Background(),
		nil,
		[]string{"mysql/0", "mediawiki/0"},
	)
	c.Check(tableName, gc.Equals, "secret_revision")
	c.Logf("q: \n%q", q)
	c.Check(q, gc.DeepEquals, `
SELECT sr.uuid
FROM secret_revision sr
LEFT JOIN secret_unit_owner suo ON sr.secret_id = suo.secret_id
LEFT JOIN unit ON unit.uuid = suo.unit_uuid
WHERE sr.obsolete = true
AND suo.unit_uuid IS NOT NULL AND unit.unit_id IN ('mysql/0','mediawiki/0')`[1:])
	assertRevUUIDsForSuppliedQuery(c, s.DB(), q, []string{
		getRevUUID(c, s.DB(), uri2, 1),
		getRevUUID(c, s.DB(), uri4, 1),
	})
}

func (s *stateSuite) TestInitialWatchStatementForObsoleteRevisionWithNoOwners(c *gc.C) {
	st := newSecretState(c, s.TxnRunnerFactory())
	uri1, uri2, uri3, uri4 := s.prepareSecretObsoleteRevisions(c, st)
	tableName, q := st.InitialWatchStatementForObsoleteRevision(context.Background(), nil, nil)
	c.Check(tableName, gc.Equals, "secret_revision")
	c.Check(q, gc.DeepEquals, `
SELECT uuid
FROM secret_revision
WHERE obsolete = true`[1:])
	assertRevUUIDsForSuppliedQuery(c, s.DB(), q, []string{
		getRevUUID(c, s.DB(), uri1, 1),
		getRevUUID(c, s.DB(), uri2, 1),
		getRevUUID(c, s.DB(), uri3, 1),
		getRevUUID(c, s.DB(), uri4, 1),
	})
}

func updateSecretContent(c *gc.C, st *State, uri *coresecrets.URI) {
	sp := domainsecret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo-new": "bar-new"},
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
