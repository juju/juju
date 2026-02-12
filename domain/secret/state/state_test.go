// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	stdtesting "testing"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	coresecrets "github.com/juju/juju/core/secrets"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	domainsecret "github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/internal/errors"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	baseSuite

	modelUUID string

	relationCount int
}

func TestStateSuite(t *stdtesting.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)
	s.modelUUID = s.setupModel(c)
}

func (s *stateSuite) setupModel(c *tc.C) string {
	modelUUID := uuid.MustNewUUID()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
VALUES (?, ?, 'test', 'prod', 'iaas', 'fluffy', 'ec2')
		`, modelUUID.String(), coretesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return modelUUID.String()
}

func (s *stateSuite) TestGetModelUUID(c *tc.C) {

	got, err := s.state.GetModelUUID(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.String(), tc.Equals, s.modelUUID)
}

func (s *stateSuite) TestGetSecretNotFound(c *tc.C) {

	_, err := s.state.GetSecret(c.Context(), coresecrets.NewURI())
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestCheckApplicationSecretLabelExistsAlreadyUsedByApp(c *tc.C) {

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	err := s.createCharmApplicationSecret(c, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	appUUID, err := s.getApplicationUUID(c, "mysql")
	c.Assert(err, tc.ErrorIsNil)

	exists, err := s.checkApplicationSecretLabelExists(c, appUUID, "my label")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)
}

func (s *stateSuite) TestCheckApplicationSecretLabelExistsAlreadyUsedByUnit(c *tc.C) {

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	appUUID, err := s.getApplicationUUID(c, "mysql")
	c.Assert(err, tc.ErrorIsNil)

	exists, err := s.checkApplicationSecretLabelExists(c, appUUID, "my label")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)
}

func (s *stateSuite) TestCheckUnitSecretLabelExistsAlreadyUsedByUnit(c *tc.C) {

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	unitUUID0, err := s.getUnitUUID(c, "mysql/0")
	c.Assert(err, tc.ErrorIsNil)

	unitUUID1, err := s.getUnitUUID(c, "mysql/1")
	c.Assert(err, tc.ErrorIsNil)

	exists, err := s.checkUnitSecretLabelExists(c, unitUUID0, "my label")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)
	exists, err = s.checkUnitSecretLabelExists(c, unitUUID1, "my label")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsFalse)
}

func (s *stateSuite) TestCheckUnitSecretLabelExistsAlreadyUsedByApp(c *tc.C) {

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		Checksum:    "checksum-1234",
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	err := s.createCharmApplicationSecret(c, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	unitUUID0, err := s.getUnitUUID(c, "mysql/0")
	c.Assert(err, tc.ErrorIsNil)

	unitUUID1, err := s.getUnitUUID(c, "mysql/1")
	c.Assert(err, tc.ErrorIsNil)

	exists, err := s.checkUnitSecretLabelExists(c, unitUUID0, "my label")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)
	exists, err = s.checkUnitSecretLabelExists(c, unitUUID1, "my label")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)
}

func (s *stateSuite) TestCheckUserSecretLabelExists(c *tc.C) {

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   ptr(true),
	}
	uri := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	exists, err := s.checkUserSecretLabelExists(c, "my label")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)
}

func (s *stateSuite) TestGetLatestRevisionNotFound(c *tc.C) {

	_, err := s.state.GetLatestRevision(c.Context(), coresecrets.NewURI())
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGetLatestRevision(c *tc.C) {

	sp := domainsecret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.UpdateSecret(ctx, uri, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar1"},
	})
	c.Assert(err, tc.ErrorIsNil)
	latest, err := s.state.GetLatestRevision(ctx, uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(latest, tc.Equals, 2)
}

func (s *stateSuite) TestGetLatestRevisions(c *tc.C) {

	wantURIs := make([]*coresecrets.URI, 2)
	for i := range 3 {
		sp := domainsecret.UpsertSecretParams{
			Data:       coresecrets.SecretData{"foo": "bar"},
			RevisionID: ptr(uuid.MustNewUUID().String()),
		}
		uri := coresecrets.NewURI()
		ctx := c.Context()
		err := s.createUserSecret(c, 1, uri, sp)
		c.Assert(err, tc.ErrorIsNil)
		for r := range i + 1 {
			err = s.state.UpdateSecret(ctx, uri, domainsecret.UpsertSecretParams{
				RevisionID: ptr(uuid.MustNewUUID().String()),
				Data:       coresecrets.SecretData{"foo": fmt.Sprintf("bar%d", r)},
			})
			c.Assert(err, tc.ErrorIsNil)
		}
		if i < 2 {
			wantURIs[i] = uri
		}
	}
	latest, err := s.state.GetLatestRevisions(c.Context(), wantURIs)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(latest, tc.DeepEquals, map[string]int{
		wantURIs[0].ID: 2,
		wantURIs[1].ID: 3,
	})
}

func (s *stateSuite) TestGetLatestRevisionsSomeNotFound(c *tc.C) {

	wantURIs := make([]*coresecrets.URI, 3)
	for i := range 3 {
		sp := domainsecret.UpsertSecretParams{
			Data:       coresecrets.SecretData{"foo": "bar"},
			RevisionID: ptr(uuid.MustNewUUID().String()),
		}
		uri := coresecrets.NewURI()
		ctx := c.Context()
		err := s.createUserSecret(c, 1, uri, sp)
		c.Assert(err, tc.ErrorIsNil)
		err = s.state.UpdateSecret(ctx, uri, domainsecret.UpsertSecretParams{
			RevisionID: ptr(uuid.MustNewUUID().String()),
			Data:       coresecrets.SecretData{"foo": "bar1"},
		})
		c.Assert(err, tc.ErrorIsNil)
		if i < 2 {
			wantURIs[i] = uri
		}
	}
	// The not found URI.
	wantURIs[2] = coresecrets.NewURI()

	_, err := s.state.GetLatestRevisions(c.Context(), wantURIs)
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGetLatestRevisionsNone(c *tc.C) {

	got, err := s.state.GetLatestRevisions(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.HasLen, 0)
}

func (s *stateSuite) TestGetRotatePolicy(c *tc.C) {
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
	err := s.createCharmApplicationSecret(c, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.state.GetRotatePolicy(c.Context(), uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.Equals, coresecrets.RotateYearly)
}

func (s *stateSuite) TestGetRotatePolicyNotFound(c *tc.C) {

	_, err := s.state.GetRotatePolicy(c.Context(), coresecrets.NewURI())
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGetRotationExpiryInfo(c *tc.C) {
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
	err := s.createCharmApplicationSecret(c, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.state.GetRotationExpiryInfo(c.Context(), uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, &domainsecret.RotationExpiryInfo{
		RotatePolicy:     coresecrets.RotateYearly,
		LatestExpireTime: ptr(expireTime.UTC()),
		NextRotateTime:   ptr(rotateTime.UTC()),
		LatestRevision:   1,
	})

	newExpireTime := expireTime.Add(2 * time.Hour)
	err = s.state.UpdateSecret(ctx, uri, domainsecret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo": "bar1"},
		ExpireTime: ptr(newExpireTime),
		RevisionID: ptr(uuid.MustNewUUID().String()),
	})
	c.Assert(err, tc.ErrorIsNil)

	result, err = s.state.GetRotationExpiryInfo(c.Context(), uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, &domainsecret.RotationExpiryInfo{
		RotatePolicy:     coresecrets.RotateYearly,
		LatestExpireTime: ptr(newExpireTime.UTC()),
		NextRotateTime:   ptr(rotateTime.UTC()),
		LatestRevision:   2,
	})
}

func (s *stateSuite) TestGetRotationExpiryInfoNotFound(c *tc.C) {

	_, err := s.state.GetRotationExpiryInfo(c.Context(), coresecrets.NewURI())
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGetSecretRevisionNotFound(c *tc.C) {

	_, _, err := s.state.GetSecretValue(c.Context(), coresecrets.NewURI(), 666)
	c.Assert(err, tc.ErrorIs, secreterrors.SecretRevisionNotFound)
}

func (s *stateSuite) TestCreateUserSecretFailedRevisionIDMissing(c *tc.C) {

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   ptr(true),
	}
	uri := coresecrets.NewURI()
	err := s.createUserSecret(c, 1, uri, sp)
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

func (s *stateSuite) assertSecret(c *tc.C, st *State, uri *coresecrets.URI, sp domainsecret.UpsertSecretParams,
	revision int, owner coresecrets.Owner) {
	ctx := c.Context()
	md, revs, err := s.state.GetSecretByURI(ctx, *uri, &revision)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(md.Version, tc.Equals, 1)
	c.Check(md.Label, tc.Equals, value(sp.Label))
	c.Check(md.Description, tc.Equals, value(sp.Description))
	c.Check(md.LatestRevision, tc.Equals, 1)
	c.Check(md.AutoPrune, tc.Equals, value(sp.AutoPrune))
	c.Check(md.Owner, tc.DeepEquals, owner)
	if sp.RotatePolicy == nil {
		c.Check(md.RotatePolicy, tc.Equals, coresecrets.RotateNever)
	} else {
		c.Check(md.RotatePolicy, tc.Equals, fromDbRotatePolicy(*sp.RotatePolicy))
	}
	if sp.NextRotateTime == nil {
		c.Check(md.NextRotateTime, tc.IsNil)
	} else {
		c.Check(*md.NextRotateTime, tc.Equals, sp.NextRotateTime.UTC())
	}
	c.Check(md.CreateTime, tc.Equals, sp.CreateTime.UTC())
	c.Check(md.UpdateTime, tc.Equals, sp.UpdateTime.UTC())

	c.Assert(revs, tc.HasLen, 1)
	rev := revs[0]
	c.Assert(err, tc.ErrorIsNil)
	c.Check(rev.Revision, tc.Equals, revision)
	c.Check(rev.CreateTime, tc.Equals, sp.UpdateTime.UTC())
	c.Check(rev.UpdateTime, tc.Equals, sp.UpdateTime.UTC())
	if rev.ExpireTime == nil {
		c.Check(md.LatestExpireTime, tc.IsNil)
	} else {
		c.Check(*md.LatestExpireTime, tc.Equals, rev.ExpireTime.UTC())
	}
}

func (s *stateSuite) TestCreateUserSecretWithContent(c *tc.C) {

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
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)
	owner := coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: s.modelUUID}
	s.assertSecret(c, s.state, uri, sp, 1, owner)
	data, ref, err := s.state.GetSecretValue(ctx, uri, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ref, tc.IsNil)
	c.Check(data, tc.DeepEquals, coresecrets.SecretData{"foo": "bar"})

	ap := domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectModel,
		SubjectID:     s.modelUUID,
	}
	access, err := s.state.GetSecretAccess(ctx, uri, ap)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(access, tc.Equals, "manage")
}

func (s *stateSuite) TestCreateManyUserSecretsNoLabelClash(c *tc.C) {

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
		err := s.createUserSecret(c, 1, uri, sp)
		c.Assert(err, tc.ErrorIsNil)
		owner := coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: s.modelUUID}
		s.assertSecret(c, s.state, uri, sp, 1, owner)
		data, ref, err := s.state.GetSecretValue(ctx, uri, 1)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(ref, tc.IsNil)
		c.Check(data, tc.DeepEquals, coresecrets.SecretData{"foo": content})
	}
	createAndCheck("my label")
	createAndCheck("")
	createAndCheck("")
	createAndCheck("another label")
}

func (s *stateSuite) TestCreateUserSecretWithValueReference(c *tc.C) {

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
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)
	owner := coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: s.modelUUID}
	s.assertSecret(c, s.state, uri, sp, 1, owner)
	data, ref, err := s.state.GetSecretValue(ctx, uri, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(data, tc.HasLen, 0)
	c.Check(ref, tc.DeepEquals, &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"})
}

func (s *stateSuite) TestGetApplicationUUIDsForNames(c *tc.C) {

	appUUID, _ := s.setupUnits(c, "mysql")

	gotUUIDs, err := s.state.GetApplicationUUIDsForNames(c.Context(), []string{"mysql", "mariadb"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotUUIDs, tc.SameContents, []string{appUUID})
}

func (s *stateSuite) TestGetUnitUUIDsForNames(c *tc.C) {

	_, unitUUIDs := s.setupUnits(c, "mysql")

	gotUUIDs, err := s.state.GetUnitUUIDsForNames(c.Context(), []string{"mysql/0", "mysql/1", "mariadb/6"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotUUIDs, tc.SameContents, unitUUIDs)
}

type ownedSecretInfo struct {
	appUUID      string
	appSecretURI *coresecrets.URI

	unitUUID      string
	unitSecretURI *coresecrets.URI

	otherAppUUID  string
	otherUnitUUID string
}

func (s *stateSuite) createOwnedSecrets(c *tc.C) ownedSecretInfo {

	appUUID, unitUUIDs := s.setupUnits(c, "mysql")
	otherAppUUID, otherUnitUUIDs := s.setupUnits(c, "mariadb")

	uri1 := coresecrets.NewURI()
	sp := domainsecret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	err := s.createCharmApplicationSecret(c, 1, uri1, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	uri2 := coresecrets.NewURI()
	sp2 := domainsecret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	err = s.createCharmUnitSecret(c, 1, uri2, "mysql/1", sp2)
	c.Assert(err, tc.ErrorIsNil)
	return ownedSecretInfo{
		appUUID:       appUUID,
		appSecretURI:  uri1,
		unitUUID:      unitUUIDs[1],
		unitSecretURI: uri2,
		otherAppUUID:  otherAppUUID,
		otherUnitUUID: otherUnitUUIDs[0],
	}
}

func (s *stateSuite) TestGetOwnedSecretIDsForForAppOwners(c *tc.C) {

	info := s.createOwnedSecrets(c)
	gotURIs, err := s.state.GetOwnedSecretIDs(c.Context(), []string{info.appUUID}, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotURIs, tc.SameContents, []string{info.appSecretURI.ID})
}

func (s *stateSuite) TestGetOwnedSecretIDsForForUnitOwners(c *tc.C) {

	info := s.createOwnedSecrets(c)
	gotURIs, err := s.state.GetOwnedSecretIDs(c.Context(), nil, []string{info.unitUUID})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotURIs, tc.SameContents, []string{info.unitSecretURI.ID})
}

func (s *stateSuite) TestGetOwnedSecretIDsWithEmptyResult(c *tc.C) {

	info := s.createOwnedSecrets(c)
	gotURIs, err := s.state.GetOwnedSecretIDs(c.Context(), []string{info.otherAppUUID}, []string{info.otherUnitUUID})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotURIs, tc.HasLen, 0)
}

func (s *stateSuite) TestListAllSecretsNone(c *tc.C) {

	ctx := c.Context()
	secrets, revisions, err := s.state.ListAllSecrets(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(secrets), tc.Equals, 0)
	c.Check(len(revisions), tc.Equals, 0)
}

func (s *stateSuite) TestListAllSecrets(c *tc.C) {

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
	err := s.createUserSecret(c, 1, uri[0], sp[0])
	c.Assert(err, tc.ErrorIsNil)
	err = s.createUserSecret(c, 1, uri[1], sp[1])
	c.Assert(err, tc.ErrorIsNil)

	secrets, revisions, err := s.state.ListAllSecrets(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(secrets), tc.Equals, 2)
	c.Assert(len(revisions), tc.Equals, 2)

	for i, md := range secrets {
		c.Check(md.Version, tc.Equals, 1)
		c.Check(md.LatestRevisionChecksum, tc.Equals, sp[i].Checksum)
		c.Check(md.Label, tc.Equals, value(sp[i].Label))
		c.Check(md.Description, tc.Equals, value(sp[i].Description))
		c.Check(md.LatestRevision, tc.Equals, 1)
		c.Check(md.AutoPrune, tc.Equals, value(sp[i].AutoPrune))
		c.Check(md.Owner, tc.DeepEquals, coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: s.modelUUID})
		c.Check(md.CreateTime, tc.Equals, sp[i].CreateTime.UTC())
		c.Check(md.UpdateTime, tc.Equals, sp[i].UpdateTime.UTC())

		revs := revisions[i]
		c.Assert(revs, tc.HasLen, 1)
		c.Check(revs[0].Revision, tc.Equals, 1)
		c.Check(revs[0].CreateTime, tc.Equals, sp[i].UpdateTime.UTC())
		c.Check(revs[0].UpdateTime, tc.Equals, sp[i].UpdateTime.UTC())
	}
}

func (s *stateSuite) TestGetSecretByURI(c *tc.C) {

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
	err := s.createUserSecret(c, 1, uri[0], sp[0])
	c.Assert(err, tc.ErrorIsNil)
	err = s.createUserSecret(c, 1, uri[1], sp[1])
	c.Assert(err, tc.ErrorIsNil)

	md, revisions, err := s.state.GetSecretByURI(
		ctx, *uri[0], nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(revisions), tc.Equals, 1)

	c.Check(md.Version, tc.Equals, 1)
	c.Check(md.Label, tc.Equals, value(sp[0].Label))
	c.Check(md.Description, tc.Equals, value(sp[0].Description))
	c.Check(md.LatestRevision, tc.Equals, 1)
	c.Check(md.AutoPrune, tc.Equals, value(sp[0].AutoPrune))
	c.Check(md.Owner, tc.DeepEquals, coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: s.modelUUID})
	c.Check(md.CreateTime, tc.Equals, sp[0].CreateTime.UTC())
	c.Check(md.UpdateTime, tc.Equals, sp[0].UpdateTime.UTC())

	revs := revisions
	c.Assert(revs, tc.HasLen, 1)
	c.Check(revs[0].Revision, tc.Equals, 1)
	c.Check(revs[0].CreateTime, tc.Equals, sp[0].UpdateTime.UTC())
	c.Check(revs[0].UpdateTime, tc.Equals, sp[0].UpdateTime.UTC())
}

func (s *stateSuite) TestGetSecretsByLabels(c *tc.C) {

	sp := []domainsecret.UpsertSecretParams{{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   ptr(true),
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}, {
		Description: ptr("my secretMetadata2"),
		Label:       ptr("fetch-me"),
		Data:        coresecrets.SecretData{"foo": "bar2"},
		AutoPrune:   ptr(true),
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}}
	uri := []*coresecrets.URI{
		coresecrets.NewURI(),
		coresecrets.NewURI(),
	}

	ctx := c.Context()
	err := s.createUserSecret(c, 1, uri[0], sp[0])
	c.Assert(err, tc.ErrorIsNil)
	err = s.createUserSecret(c, 1, uri[1], sp[1])
	c.Assert(err, tc.ErrorIsNil)

	secrets, revisions, err := s.state.ListSecretsByLabels(
		ctx, domainsecret.Labels{"fetch-me"}, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(secrets), tc.Equals, 1)
	c.Assert(len(revisions), tc.Equals, 1)

	md := secrets[0]
	c.Check(md.Version, tc.Equals, 1)
	c.Check(md.Label, tc.Equals, value(sp[1].Label))
	c.Check(md.Description, tc.Equals, value(sp[1].Description))
	c.Check(md.LatestRevision, tc.Equals, 1)
	c.Check(md.AutoPrune, tc.Equals, value(sp[1].AutoPrune))
	c.Check(md.Owner, tc.DeepEquals, coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: s.modelUUID})
	c.Check(md.CreateTime, tc.Equals, sp[1].CreateTime.UTC())
	c.Check(md.UpdateTime, tc.Equals, sp[1].UpdateTime.UTC())

	revs := revisions[0]
	c.Assert(revs, tc.HasLen, 1)
	c.Check(revs[0].Revision, tc.Equals, 1)
	c.Check(revs[0].CreateTime, tc.Equals, sp[1].UpdateTime.UTC())
	c.Check(revs[0].UpdateTime, tc.Equals, sp[1].UpdateTime.UTC())
}

func (s *stateSuite) setupApplication(c *tc.C, appName string) (string, string) {
	applicationUUID := uuid.MustNewUUID().String()
	charmUUID := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
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

		_, err = tx.ExecContext(ctx, `
INSERT INTO application (uuid, charm_uuid, name, life_id, space_uuid)
VALUES (?, ?, ?, ?, ?)
`, applicationUUID, charmUUID, appName, life.Alive, network.AlphaSpaceId)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	return applicationUUID, charmUUID
}

func (s *stateSuite) setupUnits(c *tc.C, appName string) (string, []string) {
	applicationUUID, charmUUID := s.setupApplication(c, appName)
	unitUUIDs := s.addUnits(c, appName, charmUUID)
	return applicationUUID, unitUUIDs
}

func (s *stateSuite) addUnits(c *tc.C, appName, charmUUID string) []string {
	unitUUIDs := make([]string, 2)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		// Do 2 units.
		for i := 0; i < 2; i++ {
			netNodeUUID := uuid.MustNewUUID().String()
			_, err := tx.ExecContext(ctx, "INSERT INTO net_node (uuid) VALUES (?)", netNodeUUID)
			if err != nil {
				return errors.Capture(err)
			}
			unitUUID := uuid.MustNewUUID().String()
			unitUUIDs[i] = unitUUID
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
	return unitUUIDs
}

func (s *stateSuite) TestListCharmSecretsToDrainNone(c *tc.C) {

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()

	ctx := c.Context()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	toDrain, err := s.state.ListCharmSecretsToDrain(ctx, domainsecret.ApplicationOwners{"mariadb"},
		domainsecret.NilUnitOwners)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(toDrain, tc.HasLen, 0)
}

func (s *stateSuite) TestListCharmSecretsToDrain(c *tc.C) {

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
	err := s.createCharmApplicationSecret(c, 1, uri[0], "mysql", sp[0])
	c.Assert(err, tc.ErrorIsNil)
	err = s.createCharmUnitSecret(c, 1, uri[1], "mysql/0", sp[1])
	c.Assert(err, tc.ErrorIsNil)

	uri3 := coresecrets.NewURI()
	sp3 := domainsecret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	err = s.createUserSecret(c, 1, uri3, sp3)
	c.Assert(err, tc.ErrorIsNil)

	toDrain, err := s.state.ListCharmSecretsToDrain(ctx, domainsecret.ApplicationOwners{"mysql"},
		domainsecret.UnitOwners{"mysql/0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(toDrain, tc.SameContents, []*coresecrets.SecretMetadataForDrain{{
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

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()

	ctx := c.Context()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	toDrain, err := s.state.ListUserSecretsToDrain(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(toDrain, tc.HasLen, 0)
}

func (s *stateSuite) TestListUserSecretsToDrain(c *tc.C) {

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
	err := s.createUserSecret(c, 1, uri[0], sp[0])
	c.Assert(err, tc.ErrorIsNil)
	err = s.createUserSecret(c, 1, uri[1], sp[1])
	c.Assert(err, tc.ErrorIsNil)

	uri3 := coresecrets.NewURI()
	sp3 := domainsecret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	err = s.createCharmUnitSecret(c, 1, uri3, "mysql/0", sp3)
	c.Assert(err, tc.ErrorIsNil)

	toDrain, err := s.state.ListUserSecretsToDrain(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(toDrain, tc.SameContents, []*coresecrets.SecretMetadataForDrain{{
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

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
		AutoPrune:   ptr(true),
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIs, secreterrors.AutoPruneNotSupported)
}

func (s *stateSuite) TestCreateCharmApplicationSecretWithContent(c *tc.C) {

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
	err := s.createCharmApplicationSecret(c, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)
	owner := coresecrets.Owner{Kind: coresecrets.ApplicationOwner, ID: "mysql"}
	s.assertSecret(c, s.state, uri, sp, 1, owner)
	data, ref, err := s.state.GetSecretValue(ctx, uri, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ref, tc.IsNil)
	c.Check(data, tc.DeepEquals, coresecrets.SecretData{"foo": "bar"})

	ap := domainsecret.AccessParams{
		SubjectID:     "mysql",
		SubjectTypeID: domainsecret.SubjectApplication,
	}
	access, err := s.state.GetSecretAccess(ctx, uri, ap)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(access, tc.Equals, "manage")
}

func (s *stateSuite) TestCreateCharmApplicationSecretNotFound(c *tc.C) {

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	err := s.createCharmApplicationSecret(c, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *stateSuite) TestCreateCharmApplicationSecretFailedRevisionIDMissing(c *tc.C) {

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		Checksum:    "checksum-1234",
	}
	uri := coresecrets.NewURI()
	err := s.createCharmApplicationSecret(c, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorMatches, `*.revision ID must be provided`)
}

func (s *stateSuite) TestCreateCharmUnitSecretWithContent(c *tc.C) {
	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)
	owner := coresecrets.Owner{Kind: coresecrets.UnitOwner, ID: "mysql/0"}
	s.assertSecret(c, s.state, uri, sp, 1, owner)
	data, ref, err := s.state.GetSecretValue(ctx, uri, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ref, tc.IsNil)
	c.Check(data, tc.DeepEquals, coresecrets.SecretData{"foo": "bar"})

	ap := domainsecret.AccessParams{
		SubjectID:     "mysql/0",
		SubjectTypeID: domainsecret.SubjectUnit,
	}
	access, err := s.state.GetSecretAccess(ctx, uri, ap)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(access, tc.Equals, "manage")
}

func (s *stateSuite) TestOwnerKindModelSecret(c *tc.C) {
	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("model-kind-check"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	ownerInfo := s.queryRows(c, `SELECT owner_kind, owner_uuid, owner_name FROM v_secret_owner LIMIT 1`)
	c.Assert(ownerInfo, tc.HasLen, 1)
	c.Check(ownerInfo[0]["owner_kind"], tc.Equals, string(coresecrets.ModelOwner))
	c.Check(ownerInfo[0]["owner_uuid"], tc.Equals, s.modelUUID)
	c.Check(ownerInfo[0]["owner_name"], tc.Equals, "test")
}

func (s *stateSuite) TestOwnerKindApplicationSecret(c *tc.C) {
	// Ensure application exists
	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("app-kind-check"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	err := s.createCharmApplicationSecret(c, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	ownerInfo := s.queryRows(c, `SELECT owner_kind, owner_uuid, owner_name FROM v_secret_owner LIMIT 1`)
	appInfo := s.queryRows(c, `SELECT uuid FROM application WHERE name = 'mysql' LIMIT 1`)
	c.Assert(ownerInfo, tc.HasLen, 1)
	c.Check(ownerInfo[0]["owner_kind"], tc.Equals, string(coresecrets.ApplicationOwner))
	c.Check(ownerInfo[0]["owner_name"], tc.Equals, "mysql")
	c.Assert(appInfo, tc.HasLen, 1)
	c.Check(ownerInfo[0]["owner_uuid"], tc.Equals, appInfo[0]["uuid"])
}

func (s *stateSuite) TestOwnerKindUnitSecret(c *tc.C) {
	// Ensure unit exists
	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("unit-kind-check"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	ownerInfo := s.queryRows(c, `SELECT owner_kind, owner_uuid, owner_name FROM v_secret_owner LIMIT 1`)
	unitInfo := s.queryRows(c, `SELECT uuid FROM unit WHERE name = 'mysql/0' LIMIT 1`)
	c.Assert(ownerInfo, tc.HasLen, 1)
	c.Check(ownerInfo[0]["owner_kind"], tc.Equals, string(coresecrets.UnitOwner))
	c.Check(ownerInfo[0]["owner_name"], tc.Equals, "mysql/0")
	c.Assert(unitInfo, tc.HasLen, 1)
	c.Check(ownerInfo[0]["owner_uuid"], tc.Equals, unitInfo[0]["uuid"])
}

func (s *stateSuite) TestCreateCharmUnitSecretNotFound(c *tc.C) {

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestCreateCharmUnitSecretFailedRevisionIDMissing(c *tc.C) {

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorMatches, `*.revision ID must be provided`)
}

func (s *stateSuite) TestCreateManyApplicationSecretsNoLabelClash(c *tc.C) {

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
		err := s.createCharmApplicationSecret(c, 1, uri, "mysql", sp)
		c.Assert(err, tc.ErrorIsNil)
		owner := coresecrets.Owner{Kind: coresecrets.ApplicationOwner, ID: "mysql"}
		s.assertSecret(c, s.state, uri, sp, 1, owner)
		data, ref, err := s.state.GetSecretValue(ctx, uri, 1)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(ref, tc.IsNil)
		c.Check(data, tc.DeepEquals, coresecrets.SecretData{"foo": content})
	}
	createAndCheck("my label")
	createAndCheck("")
	createAndCheck("")
	createAndCheck("another label")
}

func (s *stateSuite) TestCreateManyUnitSecretsNoLabelClash(c *tc.C) {

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
		err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
		c.Assert(err, tc.ErrorIsNil)
		owner := coresecrets.Owner{Kind: coresecrets.UnitOwner, ID: "mysql/0"}
		s.assertSecret(c, s.state, uri, sp, 1, owner)
		data, ref, err := s.state.GetSecretValue(ctx, uri, 1)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(ref, tc.IsNil)
		c.Check(data, tc.DeepEquals, coresecrets.SecretData{"foo": content})
	}
	createAndCheck("my label")
	createAndCheck("")
	createAndCheck("")
	createAndCheck("another label")
}

func (s *stateSuite) TestCreateUnitSecretsSameLabelDifferentUnits(c *tc.C) {

	s.setupUnits(c, "mysql")

	const label = "shared-label"

	createAndCheckOnUnit := func(unit string) {
		content := label + "-" + unit
		sp := domainsecret.UpsertSecretParams{
			Description: ptr("my secretMetadata"),
			Label:       ptr(label),
			Data:        coresecrets.SecretData{"foo": content},
			RevisionID:  ptr(uuid.MustNewUUID().String()),
		}
		uri := coresecrets.NewURI()
		ctx := c.Context()
		err := s.createCharmUnitSecret(c, 1, uri, coreunit.Name(unit), sp)
		c.Assert(err, tc.ErrorIsNil)
		owner := coresecrets.Owner{Kind: coresecrets.UnitOwner, ID: unit}
		s.assertSecret(c, s.state, uri, sp, 1, owner)
		data, ref, err := s.state.GetSecretValue(ctx, uri, 1)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(ref, tc.IsNil)
		c.Check(data, tc.DeepEquals, coresecrets.SecretData{"foo": content})
	}
	createAndCheckOnUnit("mysql/0")
	createAndCheckOnUnit("mysql/1")
}

func (s *stateSuite) TestListCharmSecretsMissingOwners(c *tc.C) {
	_, _, err := s.state.ListCharmSecrets(c.Context(),
		domainsecret.NilApplicationOwners, domainsecret.NilUnitOwners)
	c.Assert(err, tc.ErrorMatches, "querying charm secrets: must supply at least one app owner or unit owner")
}

func (s *stateSuite) TestListCharmSecretsByUnit(c *tc.C) {

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
	err := s.createUserSecret(c, 1, uri[0], sp[0])
	c.Assert(err, tc.ErrorIsNil)
	err = s.createCharmUnitSecret(c, 1, uri[1], "mysql/0", sp[1])
	c.Assert(err, tc.ErrorIsNil)

	secrets, revisions, err := s.state.ListCharmSecrets(ctx,
		domainsecret.NilApplicationOwners, domainsecret.UnitOwners{"mysql/0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(secrets), tc.Equals, 1)
	c.Assert(len(revisions), tc.Equals, 1)

	md := secrets[0]
	c.Check(md.Version, tc.Equals, 1)
	c.Check(md.LatestRevisionChecksum, tc.Equals, sp[1].Checksum)
	c.Check(md.Label, tc.Equals, value(sp[1].Label))
	c.Check(md.Description, tc.Equals, value(sp[1].Description))
	c.Check(md.LatestRevision, tc.Equals, 1)
	c.Check(md.AutoPrune, tc.IsFalse)
	c.Check(md.Owner, tc.DeepEquals, coresecrets.Owner{Kind: coresecrets.UnitOwner, ID: "mysql/0"})
	c.Check(md.CreateTime, tc.Equals, sp[1].CreateTime.UTC())
	c.Check(md.UpdateTime, tc.Equals, sp[1].UpdateTime.UTC())

	revs := revisions[0]
	c.Assert(revs, tc.HasLen, 1)
	c.Check(revs[0].Revision, tc.Equals, 1)
	c.Check(revs[0].ValueRef, tc.DeepEquals, &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "revision-id",
	})
	c.Check(revs[0].CreateTime, tc.Equals, sp[1].UpdateTime.UTC())
	c.Check(revs[0].UpdateTime, tc.Equals, sp[1].UpdateTime.UTC())
}

func (s *stateSuite) TestListCharmSecretsByApplication(c *tc.C) {

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
	err := s.createUserSecret(c, 1, uri[0], sp[0])
	c.Assert(err, tc.ErrorIsNil)
	err = s.createCharmApplicationSecret(c, 1, uri[1], "mysql", sp[1])
	c.Assert(err, tc.ErrorIsNil)

	secrets, revisions, err := s.state.ListCharmSecrets(ctx,
		domainsecret.ApplicationOwners{"mysql"}, domainsecret.NilUnitOwners)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(secrets), tc.Equals, 1)
	c.Assert(len(revisions), tc.Equals, 1)

	md := secrets[0]
	c.Check(md.Version, tc.Equals, 1)
	c.Check(md.Label, tc.Equals, value(sp[1].Label))
	c.Check(md.Description, tc.Equals, value(sp[1].Description))
	c.Check(md.LatestRevision, tc.Equals, 1)
	c.Check(md.AutoPrune, tc.IsFalse)
	c.Check(md.Owner, tc.DeepEquals, coresecrets.Owner{Kind: coresecrets.ApplicationOwner, ID: "mysql"})
	c.Check(md.CreateTime, tc.Equals, sp[1].CreateTime.UTC())
	c.Check(md.UpdateTime, tc.Equals, sp[1].UpdateTime.UTC())

	revs := revisions[0]
	c.Assert(revs, tc.HasLen, 1)
	c.Check(revs[0].Revision, tc.Equals, 1)
	c.Check(revs[0].CreateTime, tc.Equals, sp[1].CreateTime.UTC())
	c.Check(revs[0].UpdateTime, tc.Equals, sp[1].CreateTime.UTC())
}

func (s *stateSuite) TestListCharmSecretsApplicationOrUnit(c *tc.C) {

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
	err := s.createUserSecret(c, 1, uri[0], sp[0])
	c.Assert(err, tc.ErrorIsNil)
	err = s.createCharmApplicationSecret(c, 1, uri[1], "mysql", sp[1])
	c.Assert(err, tc.ErrorIsNil)
	err = s.createCharmUnitSecret(c, 1, uri[2], "mysql/0", sp[2])
	c.Assert(err, tc.ErrorIsNil)
	err = s.createCharmUnitSecret(c, 1, uri[3], "postgresql/0", sp[3])
	c.Assert(err, tc.ErrorIsNil)

	secrets, revisions, err := s.state.ListCharmSecrets(ctx,
		domainsecret.ApplicationOwners{"mysql"}, domainsecret.UnitOwners{"mysql/0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(secrets), tc.Equals, 2)
	c.Assert(len(revisions), tc.Equals, 2)

	first := 0
	second := 1
	if secrets[first].Label != value(sp[1].Label) {
		first = 1
		second = 0
	}

	md := secrets[first]
	c.Check(md.Version, tc.Equals, 1)
	c.Check(md.Label, tc.Equals, value(sp[1].Label))
	c.Check(md.Description, tc.Equals, value(sp[1].Description))
	c.Check(md.LatestRevision, tc.Equals, 1)
	c.Check(md.AutoPrune, tc.IsFalse)
	c.Check(md.RotatePolicy, tc.Equals, coresecrets.RotateDaily)
	c.Check(*md.NextRotateTime, tc.Equals, rotateTime.UTC())
	c.Check(*md.LatestExpireTime, tc.Equals, expireTime.UTC())
	c.Check(md.Owner, tc.DeepEquals, coresecrets.Owner{Kind: coresecrets.ApplicationOwner, ID: "mysql"})
	c.Check(md.CreateTime, tc.Equals, sp[1].CreateTime.UTC())
	c.Check(md.UpdateTime, tc.Equals, sp[1].UpdateTime.UTC())

	revs := revisions[first]
	c.Assert(revs, tc.HasLen, 1)
	c.Check(revs[0].Revision, tc.Equals, 1)
	c.Check(*revs[0].ExpireTime, tc.Equals, expireTime.UTC())
	c.Check(revs[0].CreateTime, tc.Equals, sp[1].UpdateTime.UTC())
	c.Check(revs[0].UpdateTime, tc.Equals, sp[1].UpdateTime.UTC())

	md = secrets[second]
	c.Check(md.Version, tc.Equals, 1)
	c.Check(md.Label, tc.Equals, value(sp[2].Label))
	c.Check(md.Description, tc.Equals, value(sp[2].Description))
	c.Check(md.LatestRevision, tc.Equals, 1)
	c.Check(md.AutoPrune, tc.IsFalse)
	c.Check(md.RotatePolicy, tc.Equals, coresecrets.RotateNever)
	c.Check(md.Owner, tc.DeepEquals, coresecrets.Owner{Kind: coresecrets.UnitOwner, ID: "mysql/0"})
	c.Check(md.CreateTime, tc.Equals, sp[2].CreateTime.UTC())
	c.Check(md.UpdateTime, tc.Equals, sp[2].UpdateTime.UTC())

	revs = revisions[second]
	c.Assert(revs, tc.HasLen, 1)
	c.Check(revs[0].Revision, tc.Equals, 1)
	c.Check(revs[0].ExpireTime, tc.IsNil)
	c.Check(revs[0].CreateTime, tc.Equals, sp[2].UpdateTime.UTC())
	c.Check(revs[0].UpdateTime, tc.Equals, sp[2].UpdateTime.UTC())
}

func (s *stateSuite) TestAllSecretConsumers(c *tc.C) {

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
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)
	uri2 := coresecrets.NewURI().WithSource(s.modelUUID)
	err = s.createCharmUnitSecret(c, 1, uri2, "mysql/1", sp2)
	c.Assert(err, tc.ErrorIsNil)

	consumer := coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	}
	err = s.state.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, tc.ErrorIsNil)
	consumer = coresecrets.SecretConsumerMetadata{
		Label:           "my label2",
		CurrentRevision: 668,
	}
	err = s.state.SaveSecretConsumer(ctx, uri2, "mysql/1", consumer)
	c.Assert(err, tc.ErrorIsNil)
	consumer = coresecrets.SecretConsumerMetadata{
		Label:           "my label3",
		CurrentRevision: 667,
	}
	err = s.state.SaveSecretConsumer(ctx, uri, "mysql/1", consumer)
	c.Assert(err, tc.ErrorIsNil)

	got, err := s.state.AllSecretConsumers(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, map[string][]domainsecret.ConsumerInfo{
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
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	}

	err = s.state.SaveSecretConsumer(ctx, uri, "mysql/0", *consumer)
	c.Assert(err, tc.ErrorIsNil)

	got, latest, err := s.state.GetSecretConsumer(ctx, uri, "mysql/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, consumer)
	c.Check(latest, tc.Equals, 1)
}

func (s *stateSuite) TestSaveSecretConsumerMarksObsolete(c *tc.C) {

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
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	consumer := &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 1,
	}
	err = s.state.SaveSecretConsumer(ctx, uri, "mysql/0", *consumer)
	c.Assert(err, tc.ErrorIsNil)

	got, latest, err := s.state.GetSecretConsumer(ctx, uri, "mysql/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, consumer)
	c.Check(latest, tc.Equals, 1)

	// Latest revision is 3 now, revision 2 shoule be obsolete.
	sp2 := domainsecret.UpsertSecretParams{
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "new-backend",
			RevisionID: "new-revision",
		},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	err = s.state.UpdateSecret(c.Context(), uri, sp2)
	c.Assert(err, tc.ErrorIsNil)
	content, valueRef, err := s.state.GetSecretValue(ctx, uri, 2)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(content, tc.IsNil)
	c.Check(valueRef, tc.DeepEquals, &coresecrets.ValueRef{BackendID: "new-backend", RevisionID: "new-revision"})

	md, err := s.state.GetSecret(ctx, uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(md.Version, tc.Equals, 1)
	c.Check(md.Label, tc.Equals, value(sp.Label))
	c.Check(md.Description, tc.Equals, value(sp.Description))
	c.Check(md.LatestRevision, tc.Equals, 2)

	// Revision 1 now has been consumed by the unit, so it should NOT be obsolete.
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
	err = s.state.SaveSecretConsumer(ctx, uri, "mysql/0", *consumer)
	c.Assert(err, tc.ErrorIsNil)

	obsolete, pendingDelete = s.getObsolete(c, uri, 1)
	c.Check(obsolete, tc.IsTrue)
	c.Check(pendingDelete, tc.IsTrue)
	obsolete, pendingDelete = s.getObsolete(c, uri, 2)
	c.Check(obsolete, tc.IsFalse)
	c.Check(pendingDelete, tc.IsFalse)
}

func (s *stateSuite) TestSaveSecretConsumerSecretNotExists(c *tc.C) {

	s.setupUnits(c, "mysql")

	uri := coresecrets.NewURI().WithSource(s.modelUUID)
	ctx := c.Context()
	consumer := coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	}

	err := s.state.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestSaveSecretConsumerUnitNotExists(c *tc.C) {

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		ValueRef:    &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		AutoPrune:   ptr(true),
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI().WithSource(s.modelUUID)
	ctx := c.Context()

	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	consumer := coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	}

	err = s.state.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) saveSecretConsumer(c *tc.C, uri *coresecrets.URI, label string, revision int, consumerUUID string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO secret_unit_consumer(secret_id, unit_uuid, label, source_model_uuid, current_revision)
VALUES (?, ?, ?, ?, ?)`, uri.ID, consumerUUID, label, uri.SourceUUID, revision)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) updateRemoteSecretRevision(c *tc.C, uri *coresecrets.URI, latestRevision int, appUUID string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO secret (id) VALUES (?) ON CONFLICT(id) DO NOTHING`, uri.ID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO secret_reference (secret_id, latest_revision, owner_application_uuid) VALUES (?, ?, ?)
ON CONFLICT(secret_id) DO UPDATE SET
    latest_revision=excluded.latest_revision
`, uri.ID, latestRevision, appUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) saveSecretRemoteConsumer(c *tc.C, uri *coresecrets.URI, unitName string, currentRevision int) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO secret_remote_unit_consumer (secret_id, unit_name, current_revision) VALUES (?, ?, ?)
ON CONFLICT(secret_id, unit_name) DO UPDATE SET
    current_revision=excluded.current_revision
`,
			uri.ID, unitName, currentRevision)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestAllRemoteSecrets(c *tc.C) {

	appUUID, unitUUIDs := s.setupUnits(c, "mysql")

	uri := coresecrets.NewURI().WithSource("some-other-model")

	// Save the remote secret and its latest revision.
	s.updateRemoteSecretRevision(c, uri, 666, appUUID)
	s.saveSecretConsumer(c, uri, "my label", 1, unitUUIDs[0])
	s.saveSecretConsumer(c, uri, "my label2", 2, unitUUIDs[1])

	ctx := c.Context()

	got, err := s.state.AllRemoteSecrets(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, []domainsecret.RemoteSecretInfo{{
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

	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	_, latest, err := s.state.GetSecretConsumer(ctx, uri, "mysql/0")
	c.Assert(err, tc.ErrorIs, secreterrors.SecretConsumerNotFound)
	c.Check(latest, tc.Equals, 1)
}

func (s *stateSuite) TestGetSecretConsumerRemoteSecretFirstTime(c *tc.C) {

	appUUID, _ := s.setupUnits(c, "mysql")

	uri := coresecrets.NewURI().WithSource("some-other-model")
	s.updateRemoteSecretRevision(c, uri, 666, appUUID)

	_, latest, err := s.state.GetSecretConsumer(c.Context(), uri, "mysql/0")
	c.Assert(err, tc.ErrorIs, secreterrors.SecretConsumerNotFound)
	c.Check(latest, tc.Equals, 666)
}

func (s *stateSuite) TestGetSecretConsumerSecretNotExists(c *tc.C) {

	uri := coresecrets.NewURI()

	_, _, err := s.state.GetSecretConsumer(c.Context(), uri, "mysql/0")
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGetSecretConsumerUnitNotExists(c *tc.C) {

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		ValueRef:    &coresecrets.ValueRef{BackendID: "some-backend", RevisionID: "some-revision"},
		AutoPrune:   ptr(true),
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()

	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	_, _, err = s.state.GetSecretConsumer(ctx, uri, "mysql/0")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestGetUserSecretURIByLabel(c *tc.C) {

	sp := domainsecret.UpsertSecretParams{
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   ptr(true),
		RevisionID:  ptr(uuid.MustNewUUID().String()),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	got, err := s.state.GetUserSecretURIByLabel(ctx, "my label")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.ID, tc.Equals, uri.ID)
}

func (s *stateSuite) TestGetUserSecretURIByLabelSecretNotExists(c *tc.C) {

	_, err := s.state.GetUserSecretURIByLabel(c.Context(), "my label")
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGetURIByConsumerLabel(c *tc.C) {

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SaveSecretConsumer(ctx, uri, "mysql/0", coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	})
	c.Assert(err, tc.ErrorIsNil)

	got, err := s.state.GetURIByConsumerLabel(ctx, "my label", "mysql/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.ID, tc.Equals, uri.ID)
	c.Check(got.SourceUUID, tc.Equals, uri.SourceUUID)

	_, err = s.state.GetURIByConsumerLabel(ctx, "another label", "mysql/0")
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGetURIByConsumerLabelUnitNotExists(c *tc.C) {

	s.setupUnits(c, "mysql")

	_, err := s.state.GetURIByConsumerLabel(c.Context(), "my label", "mysql/2")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestGetSecretOwnerNotFound(c *tc.C) {

	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		_, err := s.state.getSecretOwner(ctx, tx, coresecrets.NewURI())
		return err
	})
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGetSecretOwnerUnitOwned(c *tc.C) {

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	unitUUID, err := s.getUnitUUID(c, "mysql/0")
	c.Assert(err, tc.ErrorIsNil)

	var owner domainsecret.Owner
	err = s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		owner, err = s.state.getSecretOwner(ctx, tx, uri)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(owner, tc.DeepEquals, domainsecret.Owner{Kind: domainsecret.UnitOwner, UUID: unitUUID.String()})
}

func (s *stateSuite) TestGetSecretOwnerApplicationOwned(c *tc.C) {

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	err := s.createCharmApplicationSecret(c, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	appUUID, err := s.getApplicationUUID(c, "mysql")
	c.Assert(err, tc.ErrorIsNil)

	var owner domainsecret.Owner
	err = s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		owner, err = s.state.getSecretOwner(ctx, tx, uri)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(owner, tc.DeepEquals, domainsecret.Owner{Kind: domainsecret.ApplicationOwner, UUID: appUUID.String()})
}

func (s *stateSuite) TestGetSecretOwnerUserSecret(c *tc.C) {

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	var owner domainsecret.Owner
	err = s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		owner, err = s.state.getSecretOwner(ctx, tx, uri)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(owner, tc.DeepEquals, domainsecret.Owner{Kind: domainsecret.ModelOwner, UUID: s.modelUUID})
}

func (s *stateSuite) TestUpdateSecretNotFound(c *tc.C) {

	uri := coresecrets.NewURI()
	err := s.state.UpdateSecret(c.Context(), uri, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Label:      ptr("label"),
	})
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestUpdateSecretNothingToDo(c *tc.C) {

	uri := coresecrets.NewURI()
	err := s.state.UpdateSecret(c.Context(), uri, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String())})
	c.Assert(err, tc.ErrorMatches, "must specify a new value or metadata to update a secret")
}

func (s *stateSuite) TestUpdateUserSecretMetadataOnly(c *tc.C) {

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label2"),
	}
	err = s.state.UpdateSecret(c.Context(), uri, sp2)
	c.Assert(err, tc.ErrorIsNil)

	md, err := s.state.GetSecret(ctx, uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md.Version, tc.Equals, 1)
	c.Assert(md.Label, tc.Equals, value(sp2.Label))
	c.Assert(md.Description, tc.Equals, value(sp2.Description))
	c.Assert(md.LatestRevision, tc.Equals, 1)

	c.Assert(md.UpdateTime, tc.Equals, sp2.UpdateTime.UTC())
}

func (s *stateSuite) TestUpdateUserSecretFailedLabelAlreadyExists(c *tc.C) {
	ctx := c.Context()

	// Create user secret with label "dup".
	uri1 := coresecrets.NewURI()
	sp1 := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Label:       ptr("dup"),
		Description: ptr("first"),
		Data:        coresecrets.SecretData{"k": "v"},
	}
	err := s.createUserSecret(c, 1, uri1, sp1)
	c.Assert(err, tc.ErrorIsNil)

	// Create second user secret with a different label initially.
	uri2 := coresecrets.NewURI()
	sp2 := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Label:       ptr("other"),
		Description: ptr("second"),
		Data:        coresecrets.SecretData{"k": "v2"},
	}
	err = s.createUserSecret(c, 1, uri2, sp2)
	c.Assert(err, tc.ErrorIsNil)

	// Attempt to update the second secret's label to the duplicate value.
	err = s.state.UpdateSecret(ctx, uri2, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Label:      ptr("dup"),
	})
	c.Assert(err, tc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
}

func (s *stateSuite) TestUpdateUserSecretExistingLabelSameID(c *tc.C) {
	ctx := c.Context()

	// Create a user secret with label "dup" on a given URI (ID).
	uri := coresecrets.NewURI()
	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Label:       ptr("dup"),
		Description: ptr("first"),
		Data:        coresecrets.SecretData{"k": "v"},
	}
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	// Update the same secret (same ID) keeping the same label. This should work.
	err = s.state.UpdateSecret(ctx, uri, domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Label:       ptr("dup"),
		Description: ptr("updated"),
	})
	c.Assert(err, tc.ErrorIsNil)

	md, err := s.state.GetSecret(ctx, uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md.Version, tc.Equals, 1)
	c.Assert(md.Label, tc.Equals, "dup")
	c.Assert(md.Description, tc.Equals, "updated")
	c.Assert(md.LatestRevision, tc.Equals, 1)

	c.Assert(md.UpdateTime, tc.Equals, sp.UpdateTime.UTC())
}

func (s *stateSuite) TestUpdateUserSecretFailedRevisionIDMissing(c *tc.C) {

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   ptr(true),
	}

	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	sp = domainsecret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo": "something-else"},
	}
	err = s.state.UpdateSecret(ctx, uri, sp)
	c.Assert(err, tc.ErrorMatches, `*.revision ID must be provided`)
}

func (s *stateSuite) TestUpdateCharmApplicationSecretMetadataOnly(c *tc.C) {

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createCharmApplicationSecret(c, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label2"),
	}
	err = s.state.UpdateSecret(c.Context(), uri, sp2)
	c.Assert(err, tc.ErrorIsNil)

	md, err := s.state.GetSecret(ctx, uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md.Version, tc.Equals, 1)
	c.Assert(md.Label, tc.Equals, value(sp2.Label))
	c.Assert(md.Description, tc.Equals, value(sp2.Description))
	c.Assert(md.LatestRevision, tc.Equals, 1)

	c.Assert(md.UpdateTime, tc.Equals, sp2.UpdateTime.UTC())
}

func (s *stateSuite) TestUpdateApplicationSecretFailedLabelAlreadyExists(c *tc.C) {
	// Setup an application so we can create application-owned secrets.
	s.setupUnits(c, "mysql")

	ctx := c.Context()

	// First application secret with label "dup".
	uri1 := coresecrets.NewURI()
	sp1 := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Label:       ptr("dup"),
		Description: ptr("first app secret"),
		Data:        coresecrets.SecretData{"a": "1"},
	}
	err := s.createCharmApplicationSecret(c, 1, uri1, "mysql", sp1)
	c.Assert(err, tc.ErrorIsNil)

	// Second application secret with a different label initially.
	uri2 := coresecrets.NewURI()
	sp2 := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Label:       ptr("other"),
		Description: ptr("second app secret"),
		Data:        coresecrets.SecretData{"a": "2"},
	}
	err = s.createCharmApplicationSecret(c, 1, uri2, "mysql", sp2)
	c.Assert(err, tc.ErrorIsNil)

	// Attempt to update the second secret's label to the duplicate value.
	err = s.state.UpdateSecret(ctx, uri2, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Label:      ptr("dup"),
	})
	c.Assert(err, tc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
}

func (s *stateSuite) TestUpdateApplicationSecretExistingLabelSameID(c *tc.C) {
	s.setupUnits(c, "mysql")

	ctx := c.Context()

	// Create an application secret with label "dup" on a given URI (ID).
	uri := coresecrets.NewURI()
	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Label:       ptr("dup"),
		Description: ptr("first"),
		Data:        coresecrets.SecretData{"k": "v"},
	}
	err := s.createCharmApplicationSecret(c, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	// Update the same secret (same ID) keeping the same label. This should work.
	err = s.state.UpdateSecret(ctx, uri, domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Label:       ptr("dup"),
		Description: ptr("updated"),
	})
	c.Assert(err, tc.ErrorIsNil)

	md, err := s.state.GetSecret(ctx, uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md.Version, tc.Equals, 1)
	c.Assert(md.Label, tc.Equals, "dup")
	c.Assert(md.Description, tc.Equals, "updated")
	c.Assert(md.LatestRevision, tc.Equals, 1)
	c.Assert(md.UpdateTime, tc.Equals, sp.UpdateTime.UTC())
}

func (s *stateSuite) TestUpdateCharmUnitSecretMetadataOnly(c *tc.C) {

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label2"),
	}
	err = s.state.UpdateSecret(c.Context(), uri, sp2)
	c.Assert(err, tc.ErrorIsNil)

	md, err := s.state.GetSecret(ctx, uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md.Version, tc.Equals, 1)
	c.Assert(md.Label, tc.Equals, value(sp2.Label))
	c.Assert(md.Description, tc.Equals, value(sp2.Description))
	c.Assert(md.LatestRevision, tc.Equals, 1)

	c.Assert(md.UpdateTime, tc.Equals, sp2.UpdateTime.UTC())
}

func (s *stateSuite) TestUpdateUnitSecretFailedLabelAlreadyExists(c *tc.C) {
	// Setup units so we can create unit-owned secrets.
	s.setupUnits(c, "mysql")

	ctx := c.Context()

	// First unit secret with label "dup".
	uri1 := coresecrets.NewURI()
	sp1 := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Label:       ptr("dup"),
		Description: ptr("first unit secret"),
		Data:        coresecrets.SecretData{"u": "1"},
	}
	err := s.createCharmUnitSecret(c, 1, uri1, "mysql/0", sp1)
	c.Assert(err, tc.ErrorIsNil)

	// Second unit secret with a different label initially.
	uri2 := coresecrets.NewURI()
	sp2 := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Label:       ptr("other"),
		Description: ptr("second unit secret"),
		Data:        coresecrets.SecretData{"u": "2"},
	}
	err = s.createCharmUnitSecret(c, 1, uri2, "mysql/0", sp2)
	c.Assert(err, tc.ErrorIsNil)

	// Attempt to update the second secret's label to the duplicate value.
	err = s.state.UpdateSecret(ctx, uri2, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Label:      ptr("dup"),
	})
	c.Assert(err, tc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
}

func (s *stateSuite) TestUpdateUnitSecretExistingLabelSameID(c *tc.C) {
	s.setupUnits(c, "mysql")

	ctx := c.Context()

	// Create a unit secret with label "dup" on a given URI (ID).
	uri := coresecrets.NewURI()
	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Label:       ptr("dup"),
		Description: ptr("first"),
		Data:        coresecrets.SecretData{"k": "v"},
	}
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	// Update the same secret (same ID) keeping the same label. This should work.
	err = s.state.UpdateSecret(ctx, uri, domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Label:       ptr("dup"),
		Description: ptr("updated"),
	})
	c.Assert(err, tc.ErrorIsNil)

	md, err := s.state.GetSecret(ctx, uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md.Version, tc.Equals, 1)
	c.Assert(md.Label, tc.Equals, "dup")
	c.Assert(md.Description, tc.Equals, "updated")
	c.Assert(md.LatestRevision, tc.Equals, 1)

	c.Assert(md.UpdateTime, tc.Equals, sp.UpdateTime.UTC())
}

func fillDataForUpsertSecretParams(c *tc.C, p *domainsecret.UpsertSecretParams, data coresecrets.SecretData) {
	checksum, err := coresecrets.NewSecretValue(data).Checksum()
	c.Assert(err, tc.ErrorIsNil)
	p.Data = data
	p.Checksum = checksum
}

func (s *stateSuite) TestUpdateSecretChecksumPreserved(c *tc.C) {
	ctx := c.Context()

	// Create a user secret with a checksum.
	uri := coresecrets.NewURI()
	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("original description"),
		Data:        coresecrets.SecretData{"k": "v"},
		Checksum:    "original-checksum",
	}
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	// Update only the description.
	sp2 := domainsecret.UpsertSecretParams{
		Description: ptr("updated description"),
	}
	err = s.state.UpdateSecret(ctx, uri, sp2)
	c.Assert(err, tc.ErrorIsNil)

	// Verify the checksum is still there.
	md, err := s.state.GetSecret(ctx, uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md.Description, tc.Equals, "updated description")
	c.Assert(md.LatestRevisionChecksum, tc.Equals, "original-checksum")
}

func (s *stateSuite) TestUpdateSecretContentNoOpsIfNoContentChange(c *tc.C) {

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	fillDataForUpsertSecretParams(c, &sp, coresecrets.SecretData{"foo": "bar", "hello": "world"})
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.UpdateSecret(c.Context(), uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	md, revs, err := s.state.GetSecretByURI(ctx, *uri, ptr(1))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(revs, tc.HasLen, 1)
	c.Assert(md.LatestRevision, tc.Equals, 1)

	rev := revs[0]
	c.Assert(rev.Revision, tc.Equals, 1)
}

func (s *stateSuite) TestUpdateSecretContent(c *tc.C) {

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	fillDataForUpsertSecretParams(c, &sp, coresecrets.SecretData{"foo": "bar", "hello": "world"})
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	expireTime := time.Now().Add(2 * time.Hour)
	sp2 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		ExpireTime: &expireTime,
	}
	fillDataForUpsertSecretParams(c, &sp2, coresecrets.SecretData{"foo2": "bar2", "hello": "world"})
	err = s.state.UpdateSecret(c.Context(), uri, sp2)
	c.Assert(err, tc.ErrorIsNil)

	md, revs, err := s.state.GetSecretByURI(ctx, *uri, ptr(2))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md.Version, tc.Equals, 1)
	c.Assert(md.Label, tc.Equals, value(sp.Label))
	c.Assert(md.Description, tc.Equals, value(sp.Description))
	c.Assert(md.LatestRevision, tc.Equals, 2)

	c.Assert(md.UpdateTime, tc.Equals, sp.UpdateTime.UTC())

	c.Assert(revs, tc.HasLen, 1)
	rev := revs[0]
	c.Assert(rev.Revision, tc.Equals, 2)
	c.Assert(rev.ExpireTime, tc.NotNil)
	c.Assert(*rev.ExpireTime, tc.Equals, expireTime.UTC())

	content, valueRef, err := s.state.GetSecretValue(ctx, uri, 2)
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

func (s *stateSuite) TestUpdateSecretContentNonUTCInput(c *tc.C) {

	s.setupUnits(c, "mysql")

	// Force the input location to NOT BE UTC
	loc, _ := time.LoadLocation("Australia/Brisbane")
	expireTime := time.Now().Add(2 * time.Hour).In(loc)
	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		CreateTime: time.Now().In(loc),
		UpdateTime: time.Now().In(loc),
		ExpireTime: &expireTime,
	}
	fillDataForUpsertSecretParams(c, &sp, coresecrets.SecretData{"foo": "bar", "hello": "world"})
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		UpdateTime: time.Now().In(loc),
		ExpireTime: &expireTime,
	}
	fillDataForUpsertSecretParams(c, &sp2, coresecrets.SecretData{"foo2": "bar2", "hello": "world"})
	err = s.state.UpdateSecret(c.Context(), uri, sp2)
	c.Assert(err, tc.ErrorIsNil)

	md, revs, err := s.state.GetSecretByURI(ctx, *uri, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(md.CreateTime, tc.Equals, sp.CreateTime.UTC())
	c.Check(md.UpdateTime, tc.Equals, sp2.UpdateTime.UTC())

	c.Assert(revs, tc.HasLen, 2)

	if c.Check(revs[0].ExpireTime, tc.NotNil) {
		c.Check(*revs[0].ExpireTime, tc.Equals, expireTime.UTC())
	}
	c.Check(revs[0].CreateTime, tc.Equals, sp.UpdateTime.UTC())
	c.Check(revs[0].UpdateTime, tc.Equals, sp.UpdateTime.UTC())

	if c.Check(revs[1].ExpireTime, tc.NotNil) {
		c.Check(*revs[1].ExpireTime, tc.Equals, expireTime.UTC())
	}
	c.Check(revs[1].CreateTime, tc.Equals, sp2.UpdateTime.UTC())
	c.Check(revs[1].UpdateTime, tc.Equals, sp2.UpdateTime.UTC())
}

func (s *stateSuite) TestUpdateSecretContentObsolete(c *tc.C) {

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	// Create a consumer so revision 1 does not go obsolete.
	consumer := coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 1,
	}

	err = s.state.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, tc.ErrorIsNil)

	expireTime := time.Now().Add(2 * time.Hour)
	sp2 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		ExpireTime: &expireTime,
		Data:       coresecrets.SecretData{"foo2": "bar2", "hello": "world"},
	}
	err = s.state.UpdateSecret(c.Context(), uri, sp2)
	c.Assert(err, tc.ErrorIsNil)

	md, revs, err := s.state.GetSecretByURI(ctx, *uri, ptr(2))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md.Version, tc.Equals, 1)
	c.Assert(md.Label, tc.Equals, value(sp.Label))
	c.Assert(md.Description, tc.Equals, value(sp.Description))
	c.Assert(md.LatestRevision, tc.Equals, 2)

	c.Assert(md.UpdateTime, tc.Equals, sp.UpdateTime.UTC())

	c.Assert(revs, tc.HasLen, 1)
	rev := revs[0]
	c.Assert(rev.Revision, tc.Equals, 2)
	c.Assert(rev.ExpireTime, tc.NotNil)
	c.Assert(*rev.ExpireTime, tc.Equals, expireTime.UTC())

	content, valueRef, err := s.state.GetSecretValue(ctx, uri, 2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(valueRef, tc.IsNil)
	c.Assert(content, tc.DeepEquals, coresecrets.SecretData{"foo2": "bar2", "hello": "world"})

	// Latest revision is 3 now, revision 2 shoule be obsolete.
	sp3 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo3": "bar3", "hello": "world"},
	}
	err = s.state.UpdateSecret(c.Context(), uri, sp3)
	c.Assert(err, tc.ErrorIsNil)
	content, valueRef, err = s.state.GetSecretValue(ctx, uri, 3)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(valueRef, tc.IsNil)
	c.Assert(content, tc.DeepEquals, coresecrets.SecretData{"foo3": "bar3", "hello": "world"})

	md, _, err = s.state.GetSecretByURI(ctx, *uri, ptr(2))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md.Version, tc.Equals, 1)
	c.Assert(md.Label, tc.Equals, value(sp.Label))
	c.Assert(md.Description, tc.Equals, value(sp.Description))
	c.Assert(md.LatestRevision, tc.Equals, 3)

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

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		ValueRef:   &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "revision-id"},
	}
	err = s.state.UpdateSecret(c.Context(), uri, sp2)
	c.Assert(err, tc.ErrorIsNil)

	md, revs, err := s.state.GetSecretByURI(ctx, *uri, ptr(2))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md.Version, tc.Equals, 1)
	c.Assert(md.Label, tc.Equals, value(sp.Label))
	c.Assert(md.Description, tc.Equals, value(sp.Description))
	c.Assert(md.LatestRevision, tc.Equals, 2)

	c.Assert(md.UpdateTime, tc.Equals, sp.UpdateTime.UTC())

	c.Assert(revs, tc.HasLen, 1)
	rev := revs[0]
	c.Assert(rev.Revision, tc.Equals, 2)
	c.Assert(rev.ExpireTime, tc.IsNil)

	content, valueRef, err := s.state.GetSecretValue(ctx, uri, 2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(valueRef, tc.DeepEquals, &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "revision-id"})
	c.Assert(content, tc.HasLen, 0)
}

func (s *stateSuite) TestUpdateSecretNoRotate(c *tc.C) {

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:   ptr(uuid.MustNewUUID().String()),
		RotatePolicy: ptr(domainsecret.RotateDaily),
		Data:         coresecrets.SecretData{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID:   ptr(uuid.MustNewUUID().String()),
		RotatePolicy: ptr(domainsecret.RotateNever),
	}
	err = s.state.UpdateSecret(c.Context(), uri, sp2)
	c.Assert(err, tc.ErrorIsNil)

	md, err := s.state.GetSecret(ctx, uri)
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

func (s *stateSuite) TestAllSecretRemoteConsumers(c *tc.C) {

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
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)
	uri2 := coresecrets.NewURI().WithSource(s.modelUUID)
	err = s.createCharmUnitSecret(c, 1, uri2, "mysql/1", sp2)
	c.Assert(err, tc.ErrorIsNil)

	s.saveSecretRemoteConsumer(c, uri, "remote-app/0", 666)
	s.saveSecretRemoteConsumer(c, uri2, "remote-app/1", 668)
	s.saveSecretRemoteConsumer(c, uri, "remote-app/1", 667)

	got, err := s.state.AllSecretRemoteConsumers(ctx)
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

func (s *stateSuite) TestGrantUnitAccess(c *tc.C) {

	_, unitUUIDs := s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeUnit,
		ScopeUUID:     unitUUIDs[0],
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectUUID:   unitUUIDs[0],
		RoleID:        domainsecret.RoleView,
	}
	err = s.state.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIsNil)

	ap := domainsecret.AccessParams{
		SubjectTypeID: p.SubjectTypeID,
		SubjectID:     "mysql/0",
	}
	role, err := s.state.GetSecretAccess(ctx, uri, ap)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(role, tc.Equals, "view")
}

func (s *stateSuite) TestGrantApplicationAccess(c *tc.C) {

	applicationUUID, _ := s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeApplication,
		ScopeUUID:     applicationUUID,
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectUUID:   applicationUUID,
		RoleID:        domainsecret.RoleView,
	}
	err = s.state.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIsNil)

	ap := domainsecret.AccessParams{
		SubjectTypeID: p.SubjectTypeID,
		SubjectID:     "mysql",
	}
	role, err := s.state.GetSecretAccess(ctx, uri, ap)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(role, tc.Equals, "view")
}

func (s *stateSuite) TestGrantModelAccess(c *tc.C) {

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeModel,
		ScopeUUID:     s.modelUUID,
		SubjectTypeID: domainsecret.SubjectModel,
		SubjectUUID:   s.modelUUID,
		RoleID:        domainsecret.RoleView,
	}
	err = s.state.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIsNil)

	ap := domainsecret.AccessParams{
		SubjectTypeID: p.SubjectTypeID,
		SubjectID:     s.modelUUID,
	}
	role, err := s.state.GetSecretAccess(ctx, uri, ap)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(role, tc.Equals, "view")
}

func encodeRoleID(role charm.RelationRole) int {
	return map[charm.RelationRole]int{
		charm.RoleProvider: 0,
		charm.RoleRequirer: 1,
		charm.RolePeer:     2,
	}[role]
}

func encodeScopeID(role charm.RelationScope) int {
	return map[charm.RelationScope]int{
		charm.ScopeGlobal:    0,
		charm.ScopeContainer: 1,
	}[role]
}

func (s *stateSuite) addCharmRelation(c *tc.C, charmUUID string, r charm.Relation) string {
	charmRelationUUID := tc.Must(c, uuid.NewUUID).String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, interface, optional, capacity, scope_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, charmRelationUUID, charmUUID, r.Name, encodeRoleID(r.Role), r.Interface, r.Optional, r.Limit, encodeScopeID(r.Scope))
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return charmRelationUUID
}

// addRelation inserts a new relation into the database with default relation
// and life IDs. Returns the relation UUID.
func (s *stateSuite) addRelation(c *tc.C) corerelation.UUID {
	relationUUID := tc.Must(c, corerelation.NewUUID)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO relation (uuid, life_id, relation_id, scope_id) 
VALUES (?, 0, ?, 0)
`, relationUUID, s.relationCount)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	s.relationCount++
	return relationUUID
}

func (s *stateSuite) addRelationEndpoint(c *tc.C, relationUUID, endpointUUID string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?, ?, ?)`, uuid.MustNewUUID().String(), relationUUID, endpointUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) addApplicationEndpoint(c *tc.C, applicationUUID string,
	charmRelationUUID string) string {
	applicationEndpointUUID := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid,space_uuid)
VALUES (?, ?, ?, ?)
`, applicationEndpointUUID, applicationUUID, charmRelationUUID, network.AlphaSpaceId)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return applicationEndpointUUID
}

func (s *stateSuite) setupRelation(c *tc.C, appUUID, charmUUID, appUUID2, charmUUID2 string) string {
	relation := charm.Relation{
		Name:      "db",
		Role:      charm.RoleProvider,
		Interface: "db",
		Scope:     charm.ScopeGlobal,
	}
	charmRelUUID := s.addCharmRelation(c, charmUUID, relation)
	endpointUUID := s.addApplicationEndpoint(c, appUUID, charmRelUUID)

	otherRelation := charm.Relation{
		Name:      "db",
		Role:      charm.RoleRequirer,
		Interface: "db",
		Scope:     charm.ScopeGlobal,
	}
	charmRelUUID2 := s.addCharmRelation(c, charmUUID2, otherRelation)
	endpointUUID2 := s.addApplicationEndpoint(c, appUUID2, charmRelUUID2)

	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID.String(), endpointUUID)
	s.addRelationEndpoint(c, relationUUID.String(), endpointUUID2)
	return relationUUID.String()
}

func (s *stateSuite) TestGetRegularRelationUUIDByEndpointIdentifiers(c *tc.C) {

	appUUID, charmUUID := s.setupApplication(c, "mysql")
	appUUID2, charmUUID2 := s.setupApplication(c, "mediawiki")
	relUUID := s.setupRelation(c, appUUID, charmUUID, appUUID2, charmUUID2)

	got, err := s.state.GetRegularRelationUUIDByEndpointIdentifiers(
		c.Context(),
		corerelation.EndpointIdentifier{
			ApplicationName: "mediawiki",
			EndpointName:    "db",
		},
		corerelation.EndpointIdentifier{
			ApplicationName: "mysql",
			EndpointName:    "db",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.Equals, relUUID)
}

func (s *stateSuite) TestGetRelationEndpoint(c *tc.C) {

	appUUID, charmUUID := s.setupApplication(c, "mysql")
	appUUID2, charmUUID2 := s.setupApplication(c, "mediawiki")
	relUUID := s.setupRelation(c, appUUID, charmUUID, appUUID2, charmUUID2)

	got, err := s.state.GetRelationEndpoints(c.Context(), relUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.SameContents, []corerelation.EndpointIdentifier{
		{
			ApplicationName: "mediawiki",
			EndpointName:    "db",
			Role:            "requirer",
		},
		{
			ApplicationName: "mysql",
			EndpointName:    "db",
			Role:            "provider",
		},
	})
}

func (s *stateSuite) TestGrantRelationScope(c *tc.C) {

	appUUID, charmUUID := s.setupApplication(c, "mysql")
	appUUID2, charmUUID2 := s.setupApplication(c, "mediawiki")
	relUUID := s.setupRelation(c, appUUID, charmUUID, appUUID2, charmUUID2)
	unitUUIDS := s.addUnits(c, "mysql", charmUUID)

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeUUID:     relUUID,
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectUUID:   unitUUIDS[1],
		RoleID:        domainsecret.RoleView,
	}
	err = s.state.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIsNil)

	ap := domainsecret.AccessParams{
		SubjectTypeID: p.SubjectTypeID,
		SubjectID:     "mysql/1",
	}
	role, err := s.state.GetSecretAccess(ctx, uri, ap)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(role, tc.Equals, "view")
}

func (s *stateSuite) TestGetRelationGrantAccessScope(c *tc.C) {

	appUUID, charmUUID := s.setupApplication(c, "mysql")
	appUUID2, charmUUID2 := s.setupApplication(c, "mediawiki")
	relUUID := s.setupRelation(c, appUUID, charmUUID, appUUID2, charmUUID2)
	unitUUIDS := s.addUnits(c, "mysql", charmUUID)

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeUUID:     relUUID,
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectUUID:   unitUUIDS[1],
		RoleID:        domainsecret.RoleView,
	}
	err = s.state.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIsNil)

	ap := domainsecret.AccessParams{
		SubjectTypeID: p.SubjectTypeID,
		SubjectID:     "mysql/1",
	}
	got, err := s.state.GetSecretAccessRelationScope(ctx, uri, ap)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.Equals, relUUID)
}

func (s *stateSuite) TestGrantAccessInvariantScope(c *tc.C) {

	applicationUUID, unitUUIDs := s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeUnit,
		ScopeUUID:     unitUUIDs[0],
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectUUID:   unitUUIDs[0],
		RoleID:        domainsecret.RoleView,
	}
	err = s.state.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIsNil)
	p.ScopeUUID = applicationUUID
	p.ScopeTypeID = domainsecret.ScopeApplication
	err = s.state.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIs, secreterrors.InvalidSecretPermissionChange)
}

func (s *stateSuite) TestGrantSecretNotFound(c *tc.C) {

	_, unitUUIDs := s.setupUnits(c, "mysql")

	uri := coresecrets.NewURI()
	ctx := c.Context()

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeUnit,
		ScopeUUID:     unitUUIDs[0],
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectUUID:   unitUUIDs[0],
		RoleID:        domainsecret.RoleView,
	}
	err := s.state.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *stateSuite) TestGrantUnitNotFound(c *tc.C) {

	_, unitUUIDs := s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeUnit,
		ScopeUUID:     unitUUIDs[0],
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectUUID:   uuid.MustNewUUID().String(),
		RoleID:        domainsecret.RoleView,
	}
	err = s.state.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestGrantApplicationNotFound(c *tc.C) {

	_, unitUUIDs := s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeUnit,
		ScopeUUID:     unitUUIDs[0],
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectUUID:   uuid.MustNewUUID().String(),
		RoleID:        domainsecret.RoleView,
	}
	err = s.state.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *stateSuite) TestGrantScopeNotFound(c *tc.C) {

	applicationUUID, _ := s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeUnit,
		ScopeUUID:     uuid.MustNewUUID().String(),
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectUUID:   applicationUUID,
		RoleID:        domainsecret.RoleView,
	}
	err = s.state.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestGetAccessNoGrant(c *tc.C) {

	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	ap := domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
	}
	role, err := s.state.GetSecretAccess(ctx, uri, ap)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(role, tc.Equals, "")
}

func (s *stateSuite) TestGetSecretGrantsNone(c *tc.C) {

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	g, err := s.state.GetSecretGrants(ctx, uri, coresecrets.RoleView)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(g, tc.HasLen, 0)
}

func (s *stateSuite) TestGetSecretGrantsAppUnit(c *tc.C) {

	appUUID, charmUUID := s.setupApplication(c, "mysql")
	appUUID2, charmUUID2 := s.setupApplication(c, "mediawiki")
	relUUID := s.setupRelation(c, appUUID, charmUUID, appUUID2, charmUUID2)
	unitUUIDs := s.addUnits(c, "mysql", charmUUID)

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeUUID:     relUUID,
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectUUID:   unitUUIDs[1],
		RoleID:        domainsecret.RoleManage,
	}
	err = s.state.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIsNil)

	p2 := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeUUID:     relUUID,
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectUUID:   unitUUIDs[0],
		RoleID:        domainsecret.RoleView,
	}
	err = s.state.GrantAccess(ctx, uri, p2)
	c.Assert(err, tc.ErrorIsNil)

	g, err := s.state.GetSecretGrants(ctx, uri, coresecrets.RoleView)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(g, tc.DeepEquals, []domainsecret.GrantDetails{{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeUUID:     relUUID,
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}})
}

func (s *stateSuite) TestGetSecretGrantsModel(c *tc.C) {

	appUUID, charmUUID := s.setupApplication(c, "mysql")
	appUUID2, charmUUID2 := s.setupApplication(c, "mediawiki")
	relUUID := s.setupRelation(c, appUUID, charmUUID, appUUID2, charmUUID2)
	unitUUIDs := s.addUnits(c, "mysql", charmUUID)

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeUUID:     relUUID,
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectUUID:   unitUUIDs[1],
		RoleID:        domainsecret.RoleManage,
	}
	err = s.state.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIsNil)

	p2 := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeModel,
		ScopeUUID:     s.modelUUID,
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectUUID:   unitUUIDs[0],
		RoleID:        domainsecret.RoleView,
	}
	err = s.state.GrantAccess(ctx, uri, p2)
	c.Assert(err, tc.ErrorIsNil)

	g, err := s.state.GetSecretGrants(ctx, uri, coresecrets.RoleView)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(g, tc.DeepEquals, []domainsecret.GrantDetails{{
		ScopeTypeID:   domainsecret.ScopeModel,
		ScopeID:       s.modelUUID,
		ScopeUUID:     s.modelUUID,
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}})
}

func (s *stateSuite) TestAllSecretGrants(c *tc.C) {

	appUUID, charmUUID := s.setupApplication(c, "mysql")
	appUUID2, charmUUID2 := s.setupApplication(c, "mediawiki")
	relUUID := s.setupRelation(c, appUUID, charmUUID, appUUID2, charmUUID2)
	unitUUIDs := s.addUnits(c, "mysql", charmUUID)

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
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)
	err = s.createCharmApplicationSecret(c, 1, uri2, "mysql", sp2)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeUUID:     relUUID,
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectUUID:   unitUUIDs[1],
		RoleID:        domainsecret.RoleManage,
	}
	err = s.state.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIsNil)

	p2 := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeUUID:     relUUID,
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectUUID:   unitUUIDs[0],
		RoleID:        domainsecret.RoleView,
	}
	err = s.state.GrantAccess(ctx, uri, p2)
	c.Assert(err, tc.ErrorIsNil)

	g, err := s.state.AllSecretGrants(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(g, tc.DeepEquals, map[string][]domainsecret.GrantDetails{
		uri.ID: {{
			ScopeTypeID:   domainsecret.ScopeModel,
			ScopeUUID:     s.modelUUID,
			ScopeID:       s.modelUUID,
			SubjectTypeID: domainsecret.SubjectModel,
			SubjectID:     s.modelUUID,
			RoleID:        domainsecret.RoleManage,
		}, {
			ScopeTypeID:   domainsecret.ScopeRelation,
			ScopeUUID:     relUUID,
			SubjectTypeID: domainsecret.SubjectUnit,
			SubjectID:     "mysql/1",
			RoleID:        domainsecret.RoleManage,
		}, {
			ScopeTypeID:   domainsecret.ScopeRelation,
			ScopeUUID:     relUUID,
			SubjectTypeID: domainsecret.SubjectUnit,
			SubjectID:     "mysql/0",
			RoleID:        domainsecret.RoleView,
		}},
		uri2.ID: {{
			ScopeTypeID:   domainsecret.ScopeApplication,
			ScopeUUID:     appUUID,
			ScopeID:       "mysql",
			SubjectTypeID: domainsecret.SubjectApplication,
			SubjectID:     "mysql",
			RoleID:        domainsecret.RoleManage,
		}}})
}

func (s *stateSuite) TestRevokeAccess(c *tc.C) {

	appUUID, charmUUID := s.setupApplication(c, "mysql")
	appUUID2, charmUUID2 := s.setupApplication(c, "mediawiki")
	relUUID := s.setupRelation(c, appUUID, charmUUID, appUUID2, charmUUID2)
	unitUUIDs := s.addUnits(c, "mysql", charmUUID)

	sp := domainsecret.UpsertSecretParams{
		RevisionID:  ptr(uuid.MustNewUUID().String()),
		Description: ptr("my secretMetadata"),
		Label:       ptr("my label"),
		Data:        coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeUUID:     relUUID,
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectUUID:   unitUUIDs[1],
		RoleID:        domainsecret.RoleView,
	}
	err = s.state.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIsNil)

	p2 := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeUUID:     relUUID,
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectUUID:   unitUUIDs[0],
		RoleID:        domainsecret.RoleView,
	}
	err = s.state.GrantAccess(ctx, uri, p2)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.RevokeAccess(ctx, uri, domainsecret.RevokeParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectUUID:   unitUUIDs[1],
	})
	c.Assert(err, tc.ErrorIsNil)

	g, err := s.state.GetSecretGrants(ctx, uri, coresecrets.RoleView)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(g, tc.DeepEquals, []domainsecret.GrantDetails{{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeUUID:     relUUID,
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleView,
	}})
}

func (s *stateSuite) TestListGrantedSecrets(c *tc.C) {

	appUUID, charmUUID := s.setupApplication(c, "mysql")
	appUUID2, charmUUID2 := s.setupApplication(c, "mediawiki")
	relUUID := s.setupRelation(c, appUUID, charmUUID, appUUID2, charmUUID2)
	unitUUIDs := s.addUnits(c, "mysql", charmUUID)

	ctx := c.Context()
	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri := coresecrets.NewURI()
	err := s.createUserSecret(c, 1, uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "revision-id",
		},
	}
	uri2 := coresecrets.NewURI()
	err = s.createUserSecret(c, 1, uri2, sp2)
	c.Assert(err, tc.ErrorIsNil)

	sp3 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "revision-id2",
		},
	}
	uri3 := coresecrets.NewURI()
	err = s.createUserSecret(c, 1, uri3, sp3)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.UpdateSecret(ctx, uri3, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "backend-id2",
			RevisionID: "revision-id3",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	p := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeUUID:     relUUID,
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectUUID:   unitUUIDs[0],
		RoleID:        domainsecret.RoleView,
	}
	err = s.state.GrantAccess(ctx, uri, p)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.GrantAccess(ctx, uri2, p)
	c.Assert(err, tc.ErrorIsNil)

	p2 := domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeUUID:     relUUID,
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectUUID:   appUUID,
		RoleID:        domainsecret.RoleView,
	}
	err = s.state.GrantAccess(ctx, uri3, p2)
	c.Assert(err, tc.ErrorIsNil)

	accessors := []domainsecret.AccessParams{{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
	}, {
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
	}}
	result, err := s.state.ListGrantedSecretsForBackend(ctx, "backend-id", accessors, []domainsecret.Role{domainsecret.RoleView})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []*coresecrets.SecretRevisionRef{{
		URI:        uri2,
		RevisionID: "revision-id",
	}, {
		URI:        uri3,
		RevisionID: "revision-id2",
	}})
}

// TestListGrantedSecretsForBackendWithMultipleRoles verifies that when
// multiple roles are passed, secrets with any of those roles are returned.
// The "manage implies view" business logic is in the service layer which
// expands the requested role to include all satisfying roles.
func (s *stateSuite) TestListGrantedSecretsForBackendWithMultipleRoles(c *tc.C) {
	s.setupUnits(c, "mysql")

	ctx := c.Context()

	// Create an application-owned secret with external backend reference.
	// When created, the application is automatically granted RoleManage (not
	// RoleView).
	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "revision-id",
		},
	}
	uri := coresecrets.NewURI()
	err := s.createCharmApplicationSecret(c, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	accessors := []domainsecret.AccessParams{{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
	}}

	// Query with only RoleView - should NOT return the secret since the app
	// has RoleManage, not RoleView.
	result, err := s.state.ListGrantedSecretsForBackend(ctx, "backend-id", accessors, []domainsecret.Role{domainsecret.RoleView})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)

	// Query with only RoleManage - should return the secret.
	result, err = s.state.ListGrantedSecretsForBackend(ctx, "backend-id", accessors, []domainsecret.Role{domainsecret.RoleManage})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, []*coresecrets.SecretRevisionRef{{
		URI:        uri,
		RevisionID: "revision-id",
	}})

	// Query with both RoleView and RoleManage (as the service layer would
	// expand for a view request) - should return the secret.
	result, err = s.state.ListGrantedSecretsForBackend(ctx, "backend-id", accessors, []domainsecret.Role{domainsecret.RoleView, domainsecret.RoleManage})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, []*coresecrets.SecretRevisionRef{{
		URI:        uri,
		RevisionID: "revision-id",
	}})
}

// TestListGrantedSecretsForBackendNoGrants verifies that an application
// without any grants gets no results.
func (s *stateSuite) TestListGrantedSecretsForBackendNoGrants(c *tc.C) {
	s.setupUnits(c, "mysql")
	s.setupUnits(c, "mediawiki")

	ctx := c.Context()

	// Create a secret owned by mysql.
	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "revision-id",
		},
	}
	uri := coresecrets.NewURI()
	err := s.createCharmApplicationSecret(c, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	// Query as mediawiki which has no grants to any secrets.
	accessors := []domainsecret.AccessParams{{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mediawiki",
	}}
	result, err := s.state.ListGrantedSecretsForBackend(ctx, "backend-id", accessors, []domainsecret.Role{domainsecret.RoleView, domainsecret.RoleManage})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

type obsoleteSecretInfo struct {
	appUUID  string
	unitUUID string

	app2UUID  string
	unit2UUID string

	uri1 *coresecrets.URI
	uri2 *coresecrets.URI
	uri3 *coresecrets.URI
	uri4 *coresecrets.URI
}

func (s *stateSuite) prepareSecretObsoleteRevisions(c *tc.C, st *State) obsoleteSecretInfo {
	appUUID, unitUUIDs := s.setupUnits(c, "mysql")
	app2UUID, unit2UUIDs := s.setupUnits(c, "mediawiki")

	sp := domainsecret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri1 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err := s.createCharmApplicationSecret(c, 1, uri1, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)
	updateSecretContent(c, s.state, uri1)

	uri2 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = s.createCharmUnitSecret(c, 1, uri2, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)
	updateSecretContent(c, s.state, uri2)

	uri3 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = s.createCharmApplicationSecret(c, 1, uri3, "mediawiki", sp)
	c.Assert(err, tc.ErrorIsNil)
	updateSecretContent(c, s.state, uri3)

	uri4 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = s.createCharmUnitSecret(c, 1, uri4, "mediawiki/0", sp)
	c.Assert(err, tc.ErrorIsNil)
	updateSecretContent(c, s.state, uri4)
	return obsoleteSecretInfo{
		appUUID:   appUUID,
		unitUUID:  unitUUIDs[0],
		app2UUID:  app2UUID,
		unit2UUID: unit2UUIDs[0],
		uri1:      uri1,
		uri2:      uri2,
		uri3:      uri3,
		uri4:      uri4,
	}
}

func (s *stateSuite) TestInitialWatchStatementForObsoleteRevision(c *tc.C) {

	info := s.prepareSecretObsoleteRevisions(c, s.state)
	ctx := c.Context()

	tableName, f := s.state.InitialWatchStatementForObsoleteRevision(
		[]string{info.appUUID, info.app2UUID},
		[]string{info.unitUUID, info.unit2UUID},
	)
	c.Assert(tableName, tc.Equals, "secret_revision_obsolete")
	revisionUUIDs, err := f(ctx, s.TxnRunner())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(revisionUUIDs, tc.SameContents, []string{
		revID(info.uri1, 1),
		revID(info.uri2, 1),
		revID(info.uri3, 1),
		revID(info.uri4, 1),
	})
}

func updateSecretContent(c *tc.C, st *State, uri *coresecrets.URI) {
	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo-new": "bar-new"},
	}
	err := st.UpdateSecret(c.Context(), uri, sp)
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

	info := s.prepareSecretObsoleteRevisions(c, s.state)
	ctx := c.Context()

	// appOwners, unitOwners, revUUIDs.
	result, err := s.state.GetRevisionIDsForObsolete(ctx,
		[]string{
			info.appUUID,
			info.app2UUID,
		},
		[]string{
			info.unitUUID,
			info.unit2UUID,
		},
		[]string{
			getRevUUID(c, s.DB(), info.uri1, 1),
			getRevUUID(c, s.DB(), info.uri2, 1),
			getRevUUID(c, s.DB(), info.uri3, 1),
			getRevUUID(c, s.DB(), info.uri4, 1),
			getRevUUID(c, s.DB(), info.uri1, 2),
			getRevUUID(c, s.DB(), info.uri2, 2),
			getRevUUID(c, s.DB(), info.uri3, 2),
			getRevUUID(c, s.DB(), info.uri4, 2),
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		revID(info.uri1, 1),
		revID(info.uri2, 1),
		revID(info.uri3, 1),
		revID(info.uri4, 1),
	})

	// appOwners, unitOwners, revUUIDs(with unknown app owned revisions).
	result, err = s.state.GetRevisionIDsForObsolete(ctx,
		[]string{
			info.appUUID,
		},
		[]string{
			info.unitUUID,
			info.unit2UUID,
		},
		[]string{
			getRevUUID(c, s.DB(), info.uri1, 1),
			getRevUUID(c, s.DB(), info.uri2, 1),
			getRevUUID(c, s.DB(), info.uri3, 1),
			getRevUUID(c, s.DB(), info.uri4, 1),
			getRevUUID(c, s.DB(), info.uri1, 2),
			getRevUUID(c, s.DB(), info.uri2, 2),
			getRevUUID(c, s.DB(), info.uri3, 2),
			getRevUUID(c, s.DB(), info.uri4, 2),
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		revID(info.uri1, 1),
		revID(info.uri2, 1),
		revID(info.uri4, 1),
	})

	// appOwners, unitOwners, revUUIDs(with unknown unit owned revisions).
	result, err = s.state.GetRevisionIDsForObsolete(ctx,
		[]string{
			info.appUUID,
			info.app2UUID,
		},
		[]string{
			info.unitUUID,
		},
		[]string{
			getRevUUID(c, s.DB(), info.uri1, 1),
			getRevUUID(c, s.DB(), info.uri2, 1),
			getRevUUID(c, s.DB(), info.uri3, 1),
			getRevUUID(c, s.DB(), info.uri4, 1),
			getRevUUID(c, s.DB(), info.uri1, 2),
			getRevUUID(c, s.DB(), info.uri2, 2),
			getRevUUID(c, s.DB(), info.uri3, 2),
			getRevUUID(c, s.DB(), info.uri4, 2),
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		revID(info.uri1, 1),
		revID(info.uri2, 1),
		revID(info.uri3, 1),
	})

	// appOwners, unitOwners, revUUIDs(with part of the owned revisions).
	result, err = s.state.GetRevisionIDsForObsolete(ctx,
		[]string{
			info.appUUID,
			info.app2UUID,
		},
		[]string{
			info.unitUUID,
			info.unit2UUID,
		},
		[]string{
			getRevUUID(c, s.DB(), info.uri1, 1),
			getRevUUID(c, s.DB(), info.uri1, 2),
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		revID(info.uri1, 1),
	})
}

func revID(uri *coresecrets.URI, rev int) string {
	return fmt.Sprintf("%s/%d", uri.ID, rev)
}

func (s *stateSuite) TestDeleteObsoleteUserSecretRevisions(c *tc.C) {
	s.setupUnits(c, "mysql")

	uriUser1 := coresecrets.NewURI()
	uriUser2 := coresecrets.NewURI()
	uriUser3 := coresecrets.NewURI()
	uriCharm := coresecrets.NewURI()
	ctx := c.Context()
	data := coresecrets.SecretData{"foo": "bar", "hello": "world"}

	err := s.createUserSecret(c, 1, uriUser1, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       data,
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.createUserSecret(c, 1, uriUser2, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       data,
		AutoPrune:  ptr(true),
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.createUserSecret(c, 1, uriUser3, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       data,
		AutoPrune:  ptr(true),
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.createCharmApplicationSecret(c, 1, uriCharm, "mysql", domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       data,
	})
	c.Assert(err, tc.ErrorIsNil)

	sp := domainsecret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo-new": "bar-new"},
	}
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = s.state.UpdateSecret(c.Context(), uriUser1, sp)
	c.Assert(err, tc.ErrorIsNil)
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = s.state.UpdateSecret(c.Context(), uriUser2, sp)
	c.Assert(err, tc.ErrorIsNil)
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = s.state.UpdateSecret(c.Context(), uriCharm, sp)
	c.Assert(err, tc.ErrorIsNil)

	expectedToBeDeleted := []string{
		getRevUUID(c, s.DB(), uriUser2, 1),
	}
	deletedRevisionIDs, err := s.state.DeleteObsoleteUserSecretRevisions(ctx)
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
	err := s.createCharmApplicationSecret(c, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	data, ref, err := s.state.GetSecretValue(ctx, uri, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ref, tc.IsNil)
	c.Assert(data, tc.DeepEquals, coresecrets.SecretData{"foo": "bar"})

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar2"},
	}
	err = s.state.UpdateSecret(ctx, uri, sp2)
	c.Assert(err, tc.ErrorIsNil)
	sp3 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar3"},
	}
	err = s.state.UpdateSecret(ctx, uri, sp3)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.DeleteSecret(c.Context(), uri, []int{2})
	c.Assert(err, tc.ErrorIsNil)

	_, revs, err := s.state.GetSecretByURI(ctx, *uri, ptr(1))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(revs, tc.HasLen, 1)
	_, _, err = s.state.GetSecretByURI(ctx, *uri, ptr(2))
	c.Assert(err, tc.ErrorIs, secreterrors.SecretRevisionNotFound)
	_, revs, err = s.state.GetSecretByURI(ctx, *uri, ptr(3))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(revs, tc.HasLen, 1)
}

func (s *stateSuite) TestDeleteAllRevisionsFromNil(c *tc.C) {
	s.assertDeleteAllRevisions(c, nil)
}

func (s *stateSuite) TestDeleteAllRevisions(c *tc.C) {
	s.assertDeleteAllRevisions(c, []int{1, 2, 3})
}

func (s *stateSuite) assertDeleteAllRevisions(c *tc.C, revs []int) {

	s.setupUnits(c, "mysql")

	expireTime := time.Now().Add(2 * time.Hour)
	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar"},
		ExpireTime: ptr(expireTime),
	}
	uri := coresecrets.NewURI().WithSource(s.modelUUID)
	ctx := c.Context()
	err := s.createCharmApplicationSecret(c, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	sp2 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar2"},
	}
	err = s.state.UpdateSecret(ctx, uri, sp2)
	c.Assert(err, tc.ErrorIsNil)
	sp3 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar3"},
	}
	err = s.state.UpdateSecret(ctx, uri, sp3)
	c.Assert(err, tc.ErrorIsNil)

	consumer := coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	}
	err = s.state.SaveSecretConsumer(ctx, uri, "mysql/0", consumer)
	c.Assert(err, tc.ErrorIsNil)
	s.saveSecretRemoteConsumer(c, uri, "remote-app/0", 666)

	uri2 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = s.createCharmApplicationSecret(c, 1, uri2, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.DeleteSecret(c.Context(), uri, revs)
	c.Assert(err, tc.ErrorIsNil)

	for r := 1; r <= 3; r++ {
		_, _, err := s.state.GetSecretByURI(ctx, *uri, ptr(r))
		c.Assert(err, tc.ErrorIs, secreterrors.SecretRevisionNotFound)
	}
	_, err = s.state.GetSecret(ctx, uri)
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
	_, _, err = s.state.GetSecretConsumer(ctx, uri, "someunit/0")
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)

	_, err = s.state.GetSecret(ctx, uri2)
	c.Assert(err, tc.ErrorIsNil)
	data, _, err := s.state.GetSecretValue(ctx, uri2, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(data, tc.DeepEquals, coresecrets.SecretData{"foo": "bar"})
}

func (s *stateSuite) TestGetSecretRevisionID(c *tc.C) {

	s.setupUnits(c, "mysql")

	expireTime := time.Now().Add(2 * time.Hour)
	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar"},
		ExpireTime: ptr(expireTime),
	}
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createCharmApplicationSecret(c, 1, uri, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.state.GetSecretRevisionID(ctx, uri, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, *sp.RevisionID)
}

func (s *stateSuite) TestGetSecretRevisionIDNotFound(c *tc.C) {

	uri := coresecrets.NewURI()
	ctx := c.Context()

	_, err := s.state.GetSecretRevisionID(ctx, uri, 1)
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
		consumer := coresecrets.SecretConsumerMetadata{
			CurrentRevision: revision,
		}
		unitName := unittesting.GenNewName(c, consumerID)
		err := s.state.SaveSecretConsumer(ctx, uri, unitName, consumer)
		c.Assert(err, tc.ErrorIsNil)
	}

	sp := domainsecret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	uri1 := coresecrets.NewURI()
	err := s.createCharmApplicationSecret(c, 1, uri1, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	uri2 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = s.createCharmApplicationSecret(c, 1, uri2, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	// The consumed revision 1.
	saveConsumer(uri1, 1, "mediawiki/0")
	// The consumed revision 1.
	saveConsumer(uri2, 1, "mediawiki/0")

	// create revision 2, so mediawiki/0 will receive a consumed secret change event for uri1.
	updateSecretContent(c, s.state, uri1)
	return uri1, uri2
}

func (s *stateSuite) TestInitialWatchStatementForConsumedSecrets(c *tc.C) {
	ctx := c.Context()
	uri1, _ := s.prepareWatchForConsumedSecrets(c, ctx, s.state)
	tableName, f := s.state.InitialWatchStatementForConsumedSecretsChange("mediawiki/0")

	c.Assert(tableName, tc.Equals, "secret_revision")
	consumerIDs, err := f(ctx, s.TxnRunner())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(consumerIDs, tc.SameContents, []string{
		getRevUUID(c, s.DB(), uri1, 2),
	})
}

func (s *stateSuite) TestGetConsumedSecretURIsWithChanges(c *tc.C) {
	ctx := c.Context()
	uri1, uri2 := s.prepareWatchForConsumedSecrets(c, ctx, s.state)

	result, err := s.state.GetConsumedSecretURIsWithChanges(ctx, "mediawiki/0",
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

func (s *stateSuite) prepareWatchForRemoteConsumedSecrets(c *tc.C) (*coresecrets.URI, *coresecrets.URI) {
	appUUID, unitUUIDs := s.setupUnits(c, "mediawiki")

	sourceModelUUID := uuid.MustNewUUID()
	uri1 := coresecrets.NewURI()
	uri1.SourceUUID = sourceModelUUID.String()

	uri2 := coresecrets.NewURI()
	uri2.SourceUUID = sourceModelUUID.String()

	// The consumed revision 1.
	s.updateRemoteSecretRevision(c, uri1, 1, appUUID)
	s.saveSecretConsumer(c, uri1, "", 1, unitUUIDs[0])
	// The consumed revision 1.
	s.updateRemoteSecretRevision(c, uri2, 1, appUUID)
	s.saveSecretConsumer(c, uri2, "", 1, unitUUIDs[0])

	s.updateRemoteSecretRevision(c, uri1, 2, appUUID)
	return uri1, uri2
}

func (s *stateSuite) TestInitialWatchStatementForConsumedRemoteSecretsChange(c *tc.C) {
	ctx := c.Context()
	uri1, _ := s.prepareWatchForRemoteConsumedSecrets(c)

	tableName, f := s.state.InitialWatchStatementForConsumedRemoteSecretsChange("mediawiki/0")
	c.Assert(tableName, tc.Equals, "secret_reference")
	result, err := f(ctx, s.TxnRunner())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []string{
		uri1.ID,
	})
}

func (s *stateSuite) TestGetConsumedRemoteSecretURIsWithChanges(c *tc.C) {
	ctx := c.Context()
	uri1, uri2 := s.prepareWatchForRemoteConsumedSecrets(c)

	result, err := s.state.GetConsumedRemoteSecretURIsWithChanges(ctx, "mediawiki/0",
		uri1.ID,
		uri2.ID,
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
	err := s.createCharmApplicationSecret(c, 1, uri1, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	uri2 := coresecrets.NewURI()
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err = s.createCharmUnitSecret(c, 1, uri2, "mediawiki/0", sp)
	c.Assert(err, tc.ErrorIsNil)
	updateSecretContent(c, s.state, uri2)

	now := time.Now()
	err = s.state.SecretRotated(ctx, uri1, now.Add(1*time.Hour))
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SecretRotated(ctx, uri2, now.Add(2*time.Hour))
	c.Assert(err, tc.ErrorIsNil)

	return now, uri1, uri2
}

func (s *stateSuite) TestInitialWatchStatementForSecretsRotationChanges(c *tc.C) {
	ctx := c.Context()
	_, uri1, uri2 := s.prepareWatchForWatchStatementForSecretsRotationChanges(c, ctx, s.state)

	tableName, f := s.state.InitialWatchStatementForSecretsRotationChanges(domainsecret.ApplicationOwners{"mysql"},
		domainsecret.UnitOwners{"mediawiki/0"})
	c.Check(tableName, tc.Equals, "secret_rotation")
	result, err := f(ctx, s.TxnRunner())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		uri1.ID, uri2.ID,
	})

	tableName, f = s.state.InitialWatchStatementForSecretsRotationChanges(domainsecret.ApplicationOwners{"mysql"}, nil)
	c.Check(tableName, tc.Equals, "secret_rotation")
	result, err = f(ctx, s.TxnRunner())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		uri1.ID,
	})

	tableName, f = s.state.InitialWatchStatementForSecretsRotationChanges(nil, domainsecret.UnitOwners{"mediawiki/0"})
	c.Check(tableName, tc.Equals, "secret_rotation")
	result, err = f(ctx, s.TxnRunner())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		uri2.ID,
	})

	tableName, f = s.state.InitialWatchStatementForSecretsRotationChanges(nil, nil)
	c.Check(tableName, tc.Equals, "secret_rotation")
	result, err = f(ctx, s.TxnRunner())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

func (s *stateSuite) TestGetSecretsRotationChanges(c *tc.C) {
	ctx := c.Context()
	now, uri1, uri2 := s.prepareWatchForWatchStatementForSecretsRotationChanges(c, ctx, s.state)

	result, err := s.state.GetSecretsRotationChanges(ctx, domainsecret.ApplicationOwners{"mysql"},
		domainsecret.UnitOwners{"mediawiki/0"})
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

	result, err = s.state.GetSecretsRotationChanges(ctx,
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

	result, err = s.state.GetSecretsRotationChanges(ctx,
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

	result, err = s.state.GetSecretsRotationChanges(ctx, domainsecret.ApplicationOwners{"mysql"}, nil)
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []domainsecret.RotationInfo{
		{
			URI:             uri1,
			Revision:        1,
			NextTriggerTime: now.Add(1 * time.Hour).UTC(),
		},
	})

	// The uri2 is not owned by mysql, so it should not be returned.
	result, err = s.state.GetSecretsRotationChanges(ctx, domainsecret.ApplicationOwners{"mysql"}, nil, uri1.ID, uri2.ID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []domainsecret.RotationInfo{
		{
			URI:             uri1,
			Revision:        1,
			NextTriggerTime: now.Add(1 * time.Hour).UTC(),
		},
	})

	result, err = s.state.GetSecretsRotationChanges(ctx, nil, domainsecret.UnitOwners{"mediawiki/0"})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []domainsecret.RotationInfo{
		{
			URI:             uri2,
			Revision:        2,
			NextTriggerTime: now.Add(2 * time.Hour).UTC(),
		},
	})

	// The uri1 is not owned by mediawiki/0, so it should not be returned.
	result, err = s.state.GetSecretsRotationChanges(ctx, nil, domainsecret.UnitOwners{"mediawiki/0"}, uri1.ID, uri2.ID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []domainsecret.RotationInfo{
		{
			URI:             uri2,
			Revision:        2,
			NextTriggerTime: now.Add(2 * time.Hour).UTC(),
		},
	})

	result, err = s.state.GetSecretsRotationChanges(ctx, nil, nil)
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

func (s *stateSuite) prepareWatchForWatchStatementForSecretsRevisionExpiryChanges(c *tc.C, ctx context.Context, st *State) (time.Time, *coresecrets.URI, *coresecrets.URI) {
	s.setupUnits(c, "mysql")
	s.setupUnits(c, "mediawiki")

	now := time.Now()
	uri1 := coresecrets.NewURI()
	err := s.createCharmApplicationSecret(c, 1, uri1, "mysql", domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar", "hello": "world"},
		ExpireTime: ptr(now.Add(1 * time.Hour)),
	})
	c.Assert(err, tc.ErrorIsNil)

	uri2 := coresecrets.NewURI()
	err = s.createCharmUnitSecret(c, 1, uri2, "mediawiki/0", domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar", "hello": "world"},
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.UpdateSecret(c.Context(), uri2, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo-new": "bar-new"},
		ExpireTime: ptr(now.Add(2 * time.Hour)),
	})
	c.Assert(err, tc.ErrorIsNil)
	return now, uri1, uri2
}

func (s *stateSuite) TestInitialWatchStatementForSecretsRevisionExpiryChanges(c *tc.C) {
	ctx := c.Context()
	_, uri1, uri2 := s.prepareWatchForWatchStatementForSecretsRevisionExpiryChanges(c, ctx, s.state)

	tableName, f := s.state.InitialWatchStatementForSecretsRevisionExpiryChanges(domainsecret.ApplicationOwners{"mysql"},
		domainsecret.UnitOwners{"mediawiki/0"})
	c.Check(tableName, tc.Equals, "secret_revision_expire")
	result, err := f(ctx, s.TxnRunner())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		getRevUUID(c, s.DB(), uri1, 1),
		getRevUUID(c, s.DB(), uri2, 2),
	})

	tableName, f = s.state.InitialWatchStatementForSecretsRevisionExpiryChanges(domainsecret.ApplicationOwners{"mysql"},
		nil)
	c.Check(tableName, tc.Equals, "secret_revision_expire")
	result, err = f(ctx, s.TxnRunner())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		getRevUUID(c, s.DB(), uri1, 1),
	})

	tableName, f = s.state.InitialWatchStatementForSecretsRevisionExpiryChanges(nil,
		domainsecret.UnitOwners{"mediawiki/0"})
	c.Check(tableName, tc.Equals, "secret_revision_expire")
	result, err = f(ctx, s.TxnRunner())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		getRevUUID(c, s.DB(), uri2, 2),
	})

	tableName, f = s.state.InitialWatchStatementForSecretsRevisionExpiryChanges(nil, nil)
	c.Check(tableName, tc.Equals, "secret_revision_expire")
	result, err = f(ctx, s.TxnRunner())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

func (s *stateSuite) TestGetSecretsRevisionExpiryChanges(c *tc.C) {
	ctx := c.Context()
	now, uri1, uri2 := s.prepareWatchForWatchStatementForSecretsRevisionExpiryChanges(c, ctx, s.state)

	result, err := s.state.GetSecretsRevisionExpiryChanges(ctx, domainsecret.ApplicationOwners{"mysql"},
		domainsecret.UnitOwners{"mediawiki/0"})
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

	result, err = s.state.GetSecretsRevisionExpiryChanges(ctx,
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

	result, err = s.state.GetSecretsRevisionExpiryChanges(ctx,
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

	result, err = s.state.GetSecretsRevisionExpiryChanges(ctx, domainsecret.ApplicationOwners{"mysql"}, nil)
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
	result, err = s.state.GetSecretsRevisionExpiryChanges(ctx, domainsecret.ApplicationOwners{"mysql"}, nil,
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

	result, err = s.state.GetSecretsRevisionExpiryChanges(ctx, nil, domainsecret.UnitOwners{"mediawiki/0"})
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
	result, err = s.state.GetSecretsRevisionExpiryChanges(ctx, nil, domainsecret.UnitOwners{"mediawiki/0"},
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

	result, err = s.state.GetSecretsRevisionExpiryChanges(ctx, nil, nil)
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

func (s *stateSuite) TestSecretRotated(c *tc.C) {
	ctx := c.Context()

	s.setupUnits(c, "mysql")
	uri := coresecrets.NewURI()
	err := s.createCharmApplicationSecret(c, 1, uri, "mysql", domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar", "hello": "world"},
	})
	c.Assert(err, tc.ErrorIsNil)

	next := time.Now().Add(1 * time.Hour)
	err = s.state.SecretRotated(ctx, uri, next)
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

	ctx := c.Context()
	uri := coresecrets.NewURI()

	err := s.createUserSecret(c, 1, uri, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo": "bar", "hello": "world"},
	})
	c.Assert(err, tc.ErrorIsNil)

	// The secret is not obsolete yet.
	result, err := s.state.GetObsoleteUserSecretRevisionsReadyToPrune(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 0)

	// create revision 2 for user secret.
	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       coresecrets.SecretData{"foo-new": "bar-new"},
	}
	err = s.state.UpdateSecret(c.Context(), uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	result, err = s.state.GetObsoleteUserSecretRevisionsReadyToPrune(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 0)

	sp = domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		AutoPrune:  ptr(true),
	}
	err = s.state.UpdateSecret(c.Context(), uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	result, err = s.state.GetObsoleteUserSecretRevisionsReadyToPrune(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []string{uri.ID + "/1"})
}

func (s *stateSuite) TestChangeSecretBackend(c *tc.C) {
	ctx := c.Context()

	s.setupUnits(c, "mysql")
	uriCharm := coresecrets.NewURI()
	uriUser := coresecrets.NewURI()

	dataInput := coresecrets.SecretData{"foo": "bar", "hello": "world"}
	valueRefInput := &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "revision-id",
	}

	err := s.createCharmApplicationSecret(c, 1, uriCharm, "mysql", domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       dataInput,
	})
	c.Assert(err, tc.ErrorIsNil)
	data, valueRef, err := s.state.GetSecretValue(ctx, uriCharm, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(data, tc.DeepEquals, dataInput)
	c.Assert(valueRef, tc.IsNil)

	err = s.createUserSecret(c, 1, uriUser, domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
		Data:       dataInput,
	})
	c.Assert(err, tc.ErrorIsNil)
	data, valueRef, err = s.state.GetSecretValue(ctx, uriUser, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(data, tc.DeepEquals, dataInput)
	c.Assert(valueRef, tc.IsNil)

	// change to external backend.
	err = s.state.ChangeSecretBackend(ctx, parseUUID(c, getRevUUID(c, s.DB(), uriCharm, 1)), valueRefInput, nil)
	c.Assert(err, tc.ErrorIsNil)
	data, valueRef, err = s.state.GetSecretValue(ctx, uriCharm, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(data, tc.IsNil)
	c.Assert(valueRef, tc.DeepEquals, valueRefInput)

	// change back to internal backend.
	err = s.state.ChangeSecretBackend(ctx, parseUUID(c, getRevUUID(c, s.DB(), uriCharm, 1)), nil, dataInput)
	c.Assert(err, tc.ErrorIsNil)
	data, valueRef, err = s.state.GetSecretValue(ctx, uriCharm, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(data, tc.DeepEquals, dataInput)
	c.Assert(valueRef, tc.IsNil)

	// change to external backend for the user secret.
	err = s.state.ChangeSecretBackend(ctx, parseUUID(c, getRevUUID(c, s.DB(), uriUser, 1)), valueRefInput, nil)
	c.Assert(err, tc.ErrorIsNil)
	data, valueRef, err = s.state.GetSecretValue(ctx, uriUser, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(data, tc.IsNil)
	c.Assert(valueRef, tc.DeepEquals, valueRefInput)

	// change back to internal backend for the user secret.
	err = s.state.ChangeSecretBackend(ctx, parseUUID(c, getRevUUID(c, s.DB(), uriUser, 1)), nil, dataInput)
	c.Assert(err, tc.ErrorIsNil)
	data, valueRef, err = s.state.GetSecretValue(ctx, uriUser, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(data, tc.DeepEquals, dataInput)
	c.Assert(valueRef, tc.IsNil)
}

func (s *stateSuite) TestChangeSecretBackendFailed(c *tc.C) {
	ctx := c.Context()

	s.setupUnits(c, "mysql")

	dataInput := coresecrets.SecretData{"foo": "bar", "hello": "world"}
	valueRefInput := &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "revision-id",
	}

	err := s.state.ChangeSecretBackend(ctx, uuid.MustNewUUID(), nil, nil)
	c.Assert(err, tc.ErrorMatches, "either valueRef or data must be set")
	err = s.state.ChangeSecretBackend(ctx, uuid.MustNewUUID(), valueRefInput, dataInput)
	c.Assert(err, tc.ErrorMatches, "both valueRef and data cannot be set")
}

func (s *stateSuite) TestUpdateSecretContentWithEmptyValues(c *tc.C) {
	s.setupUnits(c, "mysql")

	sp := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	fillDataForUpsertSecretParams(c, &sp, coresecrets.SecretData{"foo": "bar", "empty": ""})
	uri := coresecrets.NewURI()
	ctx := c.Context()
	err := s.createCharmUnitSecret(c, 1, uri, "mysql/0", sp)
	c.Assert(err, tc.ErrorIsNil)

	content, _, err := s.state.GetSecretValue(ctx, uri, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(content, tc.DeepEquals, coresecrets.SecretData{"foo": "bar", "empty": ""})

	// Now update it, providing an empty value for an existing key and a new key.
	sp2 := domainsecret.UpsertSecretParams{
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	fillDataForUpsertSecretParams(c, &sp2, coresecrets.SecretData{"foo": "", "new": "value", "another_empty": ""})
	err = s.state.UpdateSecret(ctx, uri, sp2)
	c.Assert(err, tc.ErrorIsNil)

	// Verify that only "new" is in the second revision.
	content, _, err = s.state.GetSecretValue(ctx, uri, 2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(content, tc.DeepEquals, coresecrets.SecretData{"foo": "", "new": "value", "another_empty": ""})
}
