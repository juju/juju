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
	"github.com/juju/juju/domain/schema/testing"
	domainsecret "github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
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
