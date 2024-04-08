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
	// TODO(secrets) - sqlair doesn't like subselects
	//c.Assert(md.LatestRevision, gc.Equals, 1)
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
