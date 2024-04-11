// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coredatabase "github.com/juju/juju/core/database"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain"
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
}

var _ = gc.Suite(&stateSuite{})

func newSecretState(factory coredatabase.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
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
	modelUUID := s.setupModel(c)

	st := newSecretState(s.TxnRunnerFactory())

	got, err := st.GetModelUUID(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.Equals, modelUUID)
}

func (s *stateSuite) TestGetSecretNotFound(c *gc.C) {
	st := newSecretState(s.TxnRunnerFactory())

	_, err := st.GetSecret(context.Background(), coresecrets.NewURI())
	c.Assert(err, jc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGetSecretRevisionNotFound(c *gc.C) {
	st := newSecretState(s.TxnRunnerFactory())

	_, err := st.GetSecretRevision(context.Background(), coresecrets.NewURI(), 666)
	c.Assert(err, jc.ErrorIs, secreterrors.SecretRevisionNotFound)

	_, _, err = st.GetSecretValue(context.Background(), coresecrets.NewURI(), 666)
	c.Assert(err, jc.ErrorIs, secreterrors.SecretRevisionNotFound)
}

func (s *stateSuite) TestCreateUserSecretLabelAlreadyExists(c *gc.C) {
	s.setupModel(c)

	st := newSecretState(s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Description: "my secretMetadata",
		Label:       "my label",
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   true,
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
}

func (s *stateSuite) assertSecret(c *gc.C, st *State, uri *coresecrets.URI, sp domainsecret.UpsertSecretParams, revision int, owner coresecrets.Owner) {
	ctx := context.Background()
	md, err := st.GetSecret(ctx, uri)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md.Version, gc.Equals, 1)
	c.Assert(md.Label, gc.Equals, sp.Label)
	c.Assert(md.Description, gc.Equals, sp.Description)
	c.Assert(md.LatestRevision, gc.Equals, 1)
	c.Assert(md.AutoPrune, gc.Equals, sp.AutoPrune)
	c.Assert(md.Owner, jc.DeepEquals, owner)
	now := time.Now()
	c.Assert(md.CreateTime, jc.Almost, now)
	c.Assert(md.UpdateTime, jc.Almost, now)

	rev, err := st.GetSecretRevision(ctx, uri, revision)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rev.Revision, gc.Equals, revision)
	c.Assert(rev.CreateTime, jc.Almost, now)
	c.Assert(rev.UpdateTime, jc.Almost, now)
}

func (s *stateSuite) TestCreateUserSecretWithContent(c *gc.C) {
	modelUUID := s.setupModel(c)

	st := newSecretState(s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Description: "my secretMetadata",
		Label:       "my label",
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   true,
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)
	owner := coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: modelUUID}
	s.assertSecret(c, st, uri, sp, 1, owner)
	data, ref, err := st.GetSecretValue(ctx, uri, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ref, gc.IsNil)
	c.Assert(data, jc.DeepEquals, coresecrets.SecretData{"foo": "bar"})
}

func (s *stateSuite) TestCreateManyUserSecretsNoLabelClash(c *gc.C) {
	modelUUID := s.setupModel(c)

	st := newSecretState(s.TxnRunnerFactory())

	createAndCheck := func(label string) {
		content := label
		if content == "" {
			content = "empty"
		}
		sp := domainsecret.UpsertSecretParams{
			Description: "my secretMetadata",
			Label:       label,
			Data:        coresecrets.SecretData{"foo": content},
			AutoPrune:   true,
		}
		uri := coresecrets.NewURI()
		ctx := context.Background()
		err := st.CreateUserSecret(ctx, 1, uri, sp)
		c.Assert(err, jc.ErrorIsNil)
		owner := coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: modelUUID}
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
	modelUUID := s.setupModel(c)

	st := newSecretState(s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Description: "my secretMetadata",
		Label:       "my label",
		ValueRef:    &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		AutoPrune:   true,
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()
	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)
	owner := coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: modelUUID}
	s.assertSecret(c, st, uri, sp, 1, owner)
	data, ref, err := st.GetSecretValue(ctx, uri, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, gc.HasLen, 0)
	c.Assert(ref, jc.DeepEquals, &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"})
}

func (s *stateSuite) TestListSecretsNone(c *gc.C) {
	s.setupModel(c)

	st := newSecretState(s.TxnRunnerFactory())

	ctx := context.Background()
	secrets, revisions, err := st.ListSecrets(
		ctx, nil, domainsecret.NilRevisions, domainsecret.NilLabels, domainsecret.NilApplicationOwners, domainsecret.NilUnitOwners, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(secrets), gc.Equals, 0)
	c.Assert(len(revisions), gc.Equals, 0)
}

func (s *stateSuite) TestListSecrets(c *gc.C) {
	modelUUID := s.setupModel(c)

	st := newSecretState(s.TxnRunnerFactory())

	sp := []domainsecret.UpsertSecretParams{{
		Description: "my secretMetadata",
		Label:       "my label",
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   true,
	}, {
		Description: "my secretMetadata2",
		Label:       "my label2",
		Data:        coresecrets.SecretData{"foo": "bar2"},
		AutoPrune:   true,
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
		ctx, nil, domainsecret.NilRevisions, domainsecret.NilLabels, domainsecret.NilApplicationOwners, domainsecret.NilUnitOwners, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(secrets), gc.Equals, 2)
	c.Assert(len(revisions), gc.Equals, 2)

	for i, md := range secrets {
		c.Assert(md.Version, gc.Equals, 1)
		c.Assert(md.Label, gc.Equals, sp[i].Label)
		c.Assert(md.Description, gc.Equals, sp[i].Description)
		c.Assert(md.LatestRevision, gc.Equals, 1)
		c.Assert(md.AutoPrune, gc.Equals, sp[i].AutoPrune)
		c.Assert(md.Owner, jc.DeepEquals, coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: modelUUID})
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
	modelUUID := s.setupModel(c)

	st := newSecretState(s.TxnRunnerFactory())

	sp := []domainsecret.UpsertSecretParams{{
		Description: "my secretMetadata",
		Label:       "my label",
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   true,
	}, {
		Description: "my secretMetadata2",
		Label:       "my label2",
		Data:        coresecrets.SecretData{"foo": "bar2"},
		AutoPrune:   true,
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
		ctx, uri[0], domainsecret.NilRevisions, domainsecret.NilLabels, domainsecret.NilApplicationOwners, domainsecret.NilUnitOwners, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(secrets), gc.Equals, 1)
	c.Assert(len(revisions), gc.Equals, 1)

	md := secrets[0]
	c.Assert(md.Version, gc.Equals, 1)
	c.Assert(md.Label, gc.Equals, sp[0].Label)
	c.Assert(md.Description, gc.Equals, sp[0].Description)
	c.Assert(md.LatestRevision, gc.Equals, 1)
	c.Assert(md.AutoPrune, gc.Equals, sp[0].AutoPrune)
	c.Assert(md.Owner, jc.DeepEquals, coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: modelUUID})
	now := time.Now()
	c.Assert(md.CreateTime, jc.Almost, now)
	c.Assert(md.UpdateTime, jc.Almost, now)

	revs := revisions[0]
	c.Assert(revs, gc.HasLen, 1)
	c.Assert(revs[0].Revision, gc.Equals, 1)
	c.Assert(revs[0].CreateTime, jc.Almost, now)
	c.Assert(revs[0].UpdateTime, jc.Almost, now)
}

func (s *stateSuite) setupUnit(c *gc.C, appName string) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		applicationUUID := uuid.MustNewUUID().String()
		_, err := tx.ExecContext(context.Background(), `
INSERT INTO application (uuid, name, life_id)
VALUES (?, ?, ?)
`, applicationUUID, appName, life.Alive)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(err, jc.ErrorIsNil)

		netNodeUUID := uuid.MustNewUUID().String()
		_, err = tx.ExecContext(context.Background(), "INSERT INTO net_node (uuid) VALUES (?)", netNodeUUID)
		c.Assert(err, jc.ErrorIsNil)
		machineUUID := uuid.MustNewUUID().String()
		_, err = tx.ExecContext(context.Background(), `
INSERT INTO unit (uuid, life_id, unit_id, net_node_uuid, application_uuid)
VALUES (?, ?, ?, ?, (SELECT uuid from application WHERE name = ?))
`, machineUUID, life.Alive, appName+"/0", netNodeUUID, appName)
		c.Assert(err, jc.ErrorIsNil)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestSaveSecretConsumer(c *gc.C) {
	st := newSecretState(s.TxnRunnerFactory())

	s.setupUnit(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: "my secretMetadata",
		Label:       "my label",
		ValueRef:    &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		AutoPrune:   true,
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

func (s *stateSuite) TestSaveSecretConsumerSecretNotExists(c *gc.C) {
	modelUUID := s.setupModel(c)

	st := newSecretState(s.TxnRunnerFactory())

	s.setupUnit(c, "mysql")

	uri := coresecrets.NewURI().WithSource(modelUUID)
	ctx := context.Background()
	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	}

	err := st.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, jc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestSaveSecretConsumerUnitNotExists(c *gc.C) {
	st := newSecretState(s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Description: "my secretMetadata",
		Label:       "my label",
		ValueRef:    &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		AutoPrune:   true,
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
	s.setupModel(c)

	st := newSecretState(s.TxnRunnerFactory())

	s.setupUnit(c, "mysql")

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
	st := newSecretState(s.TxnRunnerFactory())

	s.setupUnit(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: "my secretMetadata",
		Label:       "my label",
		ValueRef:    &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		AutoPrune:   true,
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()

	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	_, latest, err := st.GetSecretConsumer(ctx, uri, "mysql/0")
	c.Assert(err, jc.ErrorIs, secreterrors.SecretConsumerNotFound)
	c.Assert(latest, gc.Equals, 1)
}

func (s *stateSuite) TestGetSecretConsumerSecretNotExists(c *gc.C) {
	st := newSecretState(s.TxnRunnerFactory())

	uri := coresecrets.NewURI()

	_, _, err := st.GetSecretConsumer(context.Background(), uri, "mysql/0")
	c.Assert(err, jc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGetSecretConsumerUnitNotExists(c *gc.C) {
	st := newSecretState(s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Description: "my secretMetadata",
		Label:       "my label",
		ValueRef:    &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		AutoPrune:   true,
	}
	uri := coresecrets.NewURI()
	ctx := context.Background()

	err := st.CreateUserSecret(ctx, 1, uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = st.GetSecretConsumer(ctx, uri, "mysql/0")
	c.Assert(err, jc.ErrorIs, uniterrors.NotFound)
}

func (s *stateSuite) TestGetUserSecretURIByLabel(c *gc.C) {
	s.setupModel(c)

	st := newSecretState(s.TxnRunnerFactory())

	sp := domainsecret.UpsertSecretParams{
		Description: "my secretMetadata",
		Label:       "my label",
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   true,
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
	s.setupModel(c)

	st := newSecretState(s.TxnRunnerFactory())

	_, err := st.GetUserSecretURIByLabel(context.Background(), "my label")
	c.Assert(err, jc.ErrorIs, secreterrors.SecretNotFound)
}
