// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/domain/unitstate"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type UniterSecretsSuite struct {
	testhelpers.IsolationSuite

	authorizer *facademocks.MockAuthorizer

	secretService *MockSecretService
	authTag       names.Tag
	clock         clock.Clock

	facade *UniterAPI
}

func TestUniterSecretsSuite(t *testing.T) {
	tc.Run(t, &UniterSecretsSuite{})
}

func (s *UniterSecretsSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.authTag = names.NewUnitTag("mariadb/0")
}

func (s *UniterSecretsSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)

	s.secretService = NewMockSecretService(ctrl)
	s.expectAuthUnitAgent()

	s.clock = testclock.NewClock(time.Now())

	var err error
	s.facade, err = NewTestAPI(c, s.authorizer, s.secretService, nil, s.clock)
	c.Assert(err, tc.ErrorIsNil)

	return ctrl
}

func (s *UniterSecretsSuite) expectAuthUnitAgent() {
	s.authorizer.EXPECT().AuthUnitAgent().Return(true)
	s.authorizer.EXPECT().GetAuthTag().Return(s.authTag).AnyTimes()
}

func (s *UniterSecretsSuite) TestCreateCharmSecrets(c *tc.C) {
	defer s.setupMocks(c).Finish()

	data := map[string]string{"foo": "bar"}
	checksum, err := coresecrets.NewSecretValue(data).Checksum()
	c.Assert(err, tc.ErrorIsNil)

	p := secret.CreateCharmSecretParams{
		Version:    secrets.Version,
		CharmOwner: secret.CharmSecretOwner{Kind: secret.ApplicationCharmSecretOwner, ID: "mariadb"},
		UpdateCharmSecretParams: secret.UpdateCharmSecretParams{
			Accessor: secret.SecretAccessor{
				Kind: secret.UnitAccessor,
				ID:   "mariadb/0",
			},
			RotatePolicy: new(coresecrets.RotateDaily),
			ExpireTime:   new(s.clock.Now()),
			Description:  new("my secret"),
			Label:        new("foobar"),
			Params:       map[string]any{"param": 1},
			Data:         data,
			Checksum:     checksum,
		},
	}
	var gotURI *coresecrets.URI
	s.secretService.EXPECT().CreateCharmSecret(gomock.Any(), gomock.Any(), p).DoAndReturn(
		func(ctx context.Context, uri *coresecrets.URI, p secret.CreateCharmSecretParams) error {
			gotURI = uri
			return nil
		},
	)

	results, err := s.facade.createSecrets(c.Context(), params.CreateSecretArgs{
		Args: []params.CreateSecretArg{{
			OwnerTag: "application-mariadb",
			UpsertSecretArg: params.UpsertSecretArg{
				RotatePolicy: new(coresecrets.RotateDaily),
				ExpireTime:   new(s.clock.Now()),
				Description:  new("my secret"),
				Label:        new("foobar"),
				Params:       map[string]any{"param": 1},
				Content:      params.SecretContentParams{Data: data, Checksum: checksum},
			},
		}, {
			UpsertSecretArg: params.UpsertSecretArg{},
		}, {
			OwnerTag: "application-mysql",
			UpsertSecretArg: params.UpsertSecretArg{
				Content: params.SecretContentParams{Data: data},
			},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.StringResults{
		Results: []params.StringResult{{
			Result: gotURI.String(),
		}, {
			Error: &params.Error{Message: `empty secret value not valid`, Code: params.CodeNotValid},
		}, {
			Error: &params.Error{Message: `permission denied`, Code: params.CodeUnauthorized},
		}},
	})
}

func (s *UniterSecretsSuite) TestCreateCharmSecretDuplicateLabel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	p := secret.CreateCharmSecretParams{
		Version:    secrets.Version,
		CharmOwner: secret.CharmSecretOwner{Kind: secret.ApplicationCharmSecretOwner, ID: "mariadb"},
		UpdateCharmSecretParams: secret.UpdateCharmSecretParams{
			Accessor: secret.SecretAccessor{
				Kind: secret.UnitAccessor,
				ID:   "mariadb/0",
			},
			Label: new("foobar"),
			Data:  map[string]string{"foo": "bar"},
		},
	}
	s.secretService.EXPECT().CreateCharmSecret(gomock.Any(), gomock.Any(), p).Return(
		fmt.Errorf("dup label %w", secreterrors.SecretLabelAlreadyExists),
	)

	results, err := s.facade.createSecrets(c.Context(), params.CreateSecretArgs{
		Args: []params.CreateSecretArg{{
			OwnerTag: "application-mariadb",
			UpsertSecretArg: params.UpsertSecretArg{
				Label:   new("foobar"),
				Content: params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
			},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.StringResults{
		Results: []params.StringResult{{
			Error: &params.Error{Message: `secret with label "foobar" already exists`, Code: params.CodeAlreadyExists},
		}},
	})
}

// --- prepareSecretUpdates tests ---

func (s *UniterSecretsSuite) TestPrepareSecretUpdatesInvalidURI(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	_, err := s.facade.prepareSecretUpdates(c.Context(), unitName, []params.UpdateSecretArg{{
		URI: "not-a-valid-uri-%%%",
	}})
	c.Assert(err, tc.ErrorMatches, `.*invalid URL escape.*`)
}

func (s *UniterSecretsSuite) TestPrepareSecretUpdatesNotFoundSkipped(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri, unitName).
		Return(secreterrors.SecretNotFound)

	result, err := s.facade.prepareSecretUpdates(c.Context(), unitName, []params.UpdateSecretArg{{
		URI: uri.String(),
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

func (s *UniterSecretsSuite) TestPrepareSecretUpdatesPermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri, unitName).
		Return(secreterrors.PermissionDenied)

	_, err := s.facade.prepareSecretUpdates(c.Context(), unitName, []params.UpdateSecretArg{{
		URI: uri.String(),
	}})
	c.Assert(err, tc.ErrorMatches, `updating secrets: permission denied`)
}

func (s *UniterSecretsSuite) TestPrepareSecretUpdatesOwnerFiltered(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()

	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri1, unitName).Return(nil)
	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri2, unitName).Return(nil)
	// Only uri1 returned — uri2 was concurrently deleted.
	s.secretService.EXPECT().GetSecretOwnerKinds(gomock.Any(), []*coresecrets.URI{uri1, uri2}).
		Return([]secret.SecretOwnerInfo{{
			SecretID:  uri1.ID,
			OwnerKind: secret.UnitCharmSecretOwner,
		}}, nil)

	result, err := s.facade.prepareSecretUpdates(c.Context(), unitName, []params.UpdateSecretArg{
		{URI: uri1.String(), UpsertSecretArg: params.UpsertSecretArg{Label: new("label1")}},
		{URI: uri2.String(), UpsertSecretArg: params.UpsertSecretArg{Label: new("label2")}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0].URI.ID, tc.Equals, uri1.ID)
	c.Check(result[0].OwnerKind, tc.Equals, secret.UnitCharmSecretOwner)
}

func (s *UniterSecretsSuite) TestPrepareSecretUpdatesSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uri := coresecrets.NewURI()
	data := map[string]string{"foo": "bar"}

	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri, unitName).Return(nil)
	s.secretService.EXPECT().GetSecretOwnerKinds(gomock.Any(), []*coresecrets.URI{uri}).
		Return([]secret.SecretOwnerInfo{{
			SecretID:  uri.ID,
			OwnerKind: secret.ApplicationCharmSecretOwner,
		}}, nil)

	result, err := s.facade.prepareSecretUpdates(c.Context(), unitName, []params.UpdateSecretArg{{
		URI: uri.String(),
		UpsertSecretArg: params.UpsertSecretArg{
			RotatePolicy: new(coresecrets.RotateDaily),
			ExpireTime:   new(s.clock.Now()),
			Description:  new("my secret"),
			Label:        new("foobar"),
			Params:       map[string]any{"param": 1},
			Content:      params.SecretContentParams{Data: data, Checksum: "checksum"},
		},
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0].URI.String(), tc.Equals, uri.String())
	c.Check(result[0].Data, tc.DeepEquals, coresecrets.SecretData(data))
	c.Check(result[0].Checksum, tc.Equals, "checksum")
	c.Check(result[0].OwnerKind, tc.Equals, secret.ApplicationCharmSecretOwner)
}

func (s *UniterSecretsSuite) TestPrepareSecretUpdatesNoAttributesSpecified(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri, unitName).Return(nil)

	_, err := s.facade.prepareSecretUpdates(c.Context(), unitName, []params.UpdateSecretArg{{
		URI:             uri.String(),
		UpsertSecretArg: params.UpsertSecretArg{},
	}})
	c.Assert(err, tc.ErrorMatches, `updating secrets: at least one attribute to update must be specified`)
}

// --- prepareSecretTrackLatest tests ---

func (s *UniterSecretsSuite) TestPrepareSecretTrackLatestEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	result, err := s.facade.prepareSecretTrackLatest(nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

func (s *UniterSecretsSuite) TestPrepareSecretTrackLatestInvalidURI(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.facade.prepareSecretTrackLatest([]string{"not-a-valid-uri-%%%"})
	c.Assert(err, tc.ErrorMatches, `.*invalid URL escape.*`)
}

func (s *UniterSecretsSuite) TestPrepareSecretTrackLatestSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	result, err := s.facade.prepareSecretTrackLatest([]string{uri1.String(), uri2.String()})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, []string{uri1.ID, uri2.ID})
}

// --- prepareSecretRevokes tests ---

func (s *UniterSecretsSuite) TestPrepareSecretRevokesInvalidURI(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	_, err := s.facade.prepareSecretRevokes(c.Context(), unitName, []params.GrantRevokeSecretArg{{
		URI:         "not-a-valid-uri-%%%",
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewApplicationTag("two").String()},
	}})
	c.Assert(err, tc.ErrorMatches, `.*invalid URL escape.*`)
}

func (s *UniterSecretsSuite) TestPrepareSecretRevokesNotFoundSkipped(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri, unitName).
		Return(secreterrors.SecretNotFound)

	result, err := s.facade.prepareSecretRevokes(c.Context(), unitName, []params.GrantRevokeSecretArg{{
		URI:         uri.String(),
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewApplicationTag("two").String()},
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

func (s *UniterSecretsSuite) TestPrepareSecretRevokesPermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri, unitName).
		Return(secreterrors.PermissionDenied)

	_, err := s.facade.prepareSecretRevokes(c.Context(), unitName, []params.GrantRevokeSecretArg{{
		URI:         uri.String(),
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewApplicationTag("two").String()},
	}})
	c.Assert(err, tc.ErrorMatches, `revoking secrets access: permission denied`)
}

func (s *UniterSecretsSuite) TestPrepareSecretRevokesMultipleSubjectsPartialFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri, unitName).Return(nil)
	// Both subjects resolved in one call: first succeeds, second fails.
	s.secretService.EXPECT().ResolveRevokeParams(gomock.Any(), gomock.Any()).
		Return([]secret.RevokeResult{
			{RevokeParams: secret.RevokeParams{SubjectUUID: "uuid-1", SubjectTypeID: secret.SubjectApplication}},
			{Error: errors.New("resolve boom")},
		})

	_, err := s.facade.prepareSecretRevokes(c.Context(), unitName, []params.GrantRevokeSecretArg{{
		URI:         uri.String(),
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewApplicationTag("app1").String(), names.NewApplicationTag("app2").String()},
	}})
	c.Assert(err, tc.ErrorMatches, `revoking secrets access: resolve boom`)
}

func (s *UniterSecretsSuite) TestPrepareSecretRevokesOwnerFiltered(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()

	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri1, unitName).Return(nil)
	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri2, unitName).Return(nil)
	s.secretService.EXPECT().ResolveRevokeParams(gomock.Any(), gomock.Any()).
		Return([]secret.RevokeResult{{RevokeParams: secret.RevokeParams{SubjectUUID: "uuid-1", SubjectTypeID: secret.SubjectApplication}}}).Times(2)
	// Only uri1 is returned by GetSecretOwnerKinds — uri2 was concurrently deleted.
	s.secretService.EXPECT().GetSecretOwnerKinds(gomock.Any(), gomock.Any()).
		Return([]secret.SecretOwnerInfo{{
			SecretID:  uri1.ID,
			OwnerKind: secret.ApplicationCharmSecretOwner,
		}}, nil)

	result, err := s.facade.prepareSecretRevokes(c.Context(), unitName, []params.GrantRevokeSecretArg{
		{URI: uri1.String(), ScopeTag: names.NewRelationTag("one:db two:use").String(), SubjectTags: []string{names.NewApplicationTag("app1").String()}},
		{URI: uri2.String(), ScopeTag: names.NewRelationTag("one:db two:use").String(), SubjectTags: []string{names.NewApplicationTag("app2").String()}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0].URI.ID, tc.Equals, uri1.ID)
	c.Check(result[0].OwnerKind, tc.Equals, secret.ApplicationCharmSecretOwner)
}

func (s *UniterSecretsSuite) TestPrepareSecretRevokesSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri, unitName).Return(nil)
	s.secretService.EXPECT().ResolveRevokeParams(gomock.Any(), gomock.Any()).
		Return([]secret.RevokeResult{{RevokeParams: secret.RevokeParams{SubjectUUID: "uuid-app", SubjectTypeID: secret.SubjectApplication}}})
	s.secretService.EXPECT().GetSecretOwnerKinds(gomock.Any(), []*coresecrets.URI{uri}).
		Return([]secret.SecretOwnerInfo{{
			SecretID:  uri.ID,
			OwnerKind: secret.UnitCharmSecretOwner,
		}}, nil)

	result, err := s.facade.prepareSecretRevokes(c.Context(), unitName, []params.GrantRevokeSecretArg{{
		URI:         uri.String(),
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewApplicationTag("two").String()},
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0].URI.String(), tc.Equals, uri.String())
	c.Check(result[0].SubjectUUID, tc.Equals, "uuid-app")
	c.Check(result[0].SubjectTypeID, tc.Equals, secret.SubjectApplication)
	c.Check(result[0].OwnerKind, tc.Equals, secret.UnitCharmSecretOwner)
}

// --- resolveRevokeSubjects tests ---

func (s *UniterSecretsSuite) TestResolveRevokeSubjectsInvalidScopeTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	accessor := secret.SecretAccessor{Kind: secret.UnitAccessor, ID: "mariadb/0"}
	_, err := s.facade.resolveRevokeSubjects(c.Context(), accessor, params.GrantRevokeSecretArg{
		ScopeTag:    "bad-tag",
		SubjectTags: []string{names.NewApplicationTag("app1").String()},
	})
	c.Assert(err, tc.ErrorMatches, `.*"bad-tag" is not a valid tag.*`)
}

func (s *UniterSecretsSuite) TestResolveRevokeSubjectsInvalidSubjectTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	accessor := secret.SecretAccessor{Kind: secret.UnitAccessor, ID: "mariadb/0"}
	_, err := s.facade.resolveRevokeSubjects(c.Context(), accessor, params.GrantRevokeSecretArg{
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{"not-a-tag"},
	})
	c.Assert(err, tc.ErrorMatches, `.*"not-a-tag" is not a valid tag.*`)
}

func (s *UniterSecretsSuite) TestResolveRevokeSubjectsUnsupportedTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	accessor := secret.SecretAccessor{Kind: secret.UnitAccessor, ID: "mariadb/0"}
	_, err := s.facade.resolveRevokeSubjects(c.Context(), accessor, params.GrantRevokeSecretArg{
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewMachineTag("0").String()},
	})
	c.Assert(err, tc.ErrorMatches, `.*tag kind "machine" not valid for secret accessor`)
}

func (s *UniterSecretsSuite) TestResolveRevokeSubjectsInvalidRole(c *tc.C) {
	defer s.setupMocks(c).Finish()

	accessor := secret.SecretAccessor{Kind: secret.UnitAccessor, ID: "mariadb/0"}
	_, err := s.facade.resolveRevokeSubjects(c.Context(), accessor, params.GrantRevokeSecretArg{
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewApplicationTag("app1").String()},
		Role:        "not-valid",
	})
	c.Assert(err, tc.ErrorMatches, `secret role "not-valid" not valid`)
}

func (s *UniterSecretsSuite) TestResolveRevokeSubjectsResolveParamsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	accessor := secret.SecretAccessor{Kind: secret.UnitAccessor, ID: "mariadb/0"}
	s.secretService.EXPECT().ResolveRevokeParams(gomock.Any(), gomock.Any()).
		Return([]secret.RevokeResult{{Error: errors.New("resolve failed")}})

	results, err := s.facade.resolveRevokeSubjects(c.Context(), accessor, params.GrantRevokeSecretArg{
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewApplicationTag("app1").String()},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0].err, tc.ErrorMatches, "resolve failed")
}

func (s *UniterSecretsSuite) TestResolveRevokeSubjectsSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	accessor := secret.SecretAccessor{Kind: secret.UnitAccessor, ID: "mariadb/0"}
	s.secretService.EXPECT().ResolveRevokeParams(gomock.Any(), gomock.Any()).
		Return([]secret.RevokeResult{
			{RevokeParams: secret.RevokeParams{SubjectUUID: "uuid-1", SubjectTypeID: secret.SubjectApplication}},
			{RevokeParams: secret.RevokeParams{SubjectUUID: "uuid-2", SubjectTypeID: secret.SubjectUnit}},
		})

	results, err := s.facade.resolveRevokeSubjects(c.Context(), accessor, params.GrantRevokeSecretArg{
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewApplicationTag("app1").String(), names.NewUnitTag("app1/0").String()},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 2)
	c.Check(results[0].SubjectUUID, tc.Equals, "uuid-1")
	c.Check(results[0].SubjectTypeID, tc.Equals, secret.SubjectApplication)
	c.Check(results[0].err, tc.ErrorIsNil)
	c.Check(results[1].SubjectUUID, tc.Equals, "uuid-2")
	c.Check(results[1].SubjectTypeID, tc.Equals, secret.SubjectUnit)
	c.Check(results[1].err, tc.ErrorIsNil)
}

// --- resolveSecretOwnerKinds tests ---

func (s *UniterSecretsSuite) TestResolveSecretOwnerKindsEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// No service call expected for empty input.
	result, err := s.facade.resolveSecretOwnerKinds(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

func (s *UniterSecretsSuite) TestResolveSecretOwnerKindsServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.secretService.EXPECT().GetSecretOwnerKinds(gomock.Any(), []*coresecrets.URI{uri}).
		Return(nil, errors.New("db gone"))

	_, err := s.facade.resolveSecretOwnerKinds(c.Context(), []unitstate.RevokeSecretArg{{
		URI: uri,
	}})
	c.Assert(err, tc.ErrorMatches, "db gone")
}

func (s *UniterSecretsSuite) TestResolveSecretOwnerKindsFiltersDisappeared(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()

	// Only return info for uri1 — uri2 "disappeared".
	s.secretService.EXPECT().GetSecretOwnerKinds(gomock.Any(), gomock.Any()).
		Return([]secret.SecretOwnerInfo{{
			SecretID:  uri1.ID,
			OwnerKind: secret.ApplicationCharmSecretOwner,
		}}, nil)

	result, err := s.facade.resolveSecretOwnerKinds(c.Context(), []unitstate.RevokeSecretArg{
		{URI: uri1, SubjectUUID: "a"},
		{URI: uri2, SubjectUUID: "b"},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0].URI.ID, tc.Equals, uri1.ID)
	c.Check(result[0].OwnerKind, tc.Equals, secret.ApplicationCharmSecretOwner)
}

// --- prepareSecretGrants tests ---

func (s *UniterSecretsSuite) TestPrepareSecretGrantsSecretNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uri := coresecrets.NewURI()

	// Secret not found — should be silently skipped.
	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri, unitName).
		Return(secreterrors.SecretNotFound)

	result, err := s.facade.prepareSecretGrants(c.Context(), unitName, []params.GrantRevokeSecretArg{{
		URI:         uri.String(),
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewApplicationTag("two").String()},
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 0)
}

func (s *UniterSecretsSuite) TestPrepareSecretGrantsPermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri, unitName).
		Return(secreterrors.PermissionDenied)

	_, err := s.facade.prepareSecretGrants(c.Context(), unitName, []params.GrantRevokeSecretArg{{
		URI:         uri.String(),
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewApplicationTag("two").String()},
	}})
	c.Assert(err, tc.ErrorMatches, `granting secrets access: permission denied`)
}

func (s *UniterSecretsSuite) TestPrepareSecretGrantsInvalidRole(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri, unitName).Return(nil)

	_, err := s.facade.prepareSecretGrants(c.Context(), unitName, []params.GrantRevokeSecretArg{{
		URI:         uri.String(),
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewApplicationTag("two").String()},
		Role:        "bad-role",
	}})
	c.Assert(err, tc.ErrorMatches, `secret role "bad-role" not valid`)
}

func (s *UniterSecretsSuite) TestPrepareSecretGrantsMultipleSubjectsPartialFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri, unitName).Return(nil)
	// Both subjects resolved in one call: first succeeds, second fails.
	s.secretService.EXPECT().ResolveGrantParams(gomock.Any(), gomock.Any()).
		Return([]secret.GrantResult{
			{GrantParams: secret.GrantParams{SubjectUUID: "uuid-1", SubjectTypeID: secret.SubjectApplication,
				ScopeUUID: "scope-1", ScopeTypeID: secret.ScopeApplication, RoleID: secret.RoleView}},
			{Error: errors.New("resolve boom")},
		})

	_, err := s.facade.prepareSecretGrants(c.Context(), unitName, []params.GrantRevokeSecretArg{{
		URI:         uri.String(),
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewApplicationTag("app1").String(), names.NewApplicationTag("app2").String()},
		Role:        "view",
	}})
	c.Assert(err, tc.ErrorMatches, `granting secrets access: resolve boom`)
}

func (s *UniterSecretsSuite) TestPrepareSecretGrantsOwnerFiltered(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()

	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri1, unitName).Return(nil)
	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri2, unitName).Return(nil)
	s.secretService.EXPECT().ResolveGrantParams(gomock.Any(), gomock.Any()).
		Return([]secret.GrantResult{{GrantParams: secret.GrantParams{
			SubjectUUID: "uuid-1", SubjectTypeID: secret.SubjectApplication,
			ScopeUUID: "scope-1", ScopeTypeID: secret.ScopeApplication, RoleID: secret.RoleView,
		}}}).Times(2)
	// Only uri1 returned — uri2 was concurrently deleted.
	s.secretService.EXPECT().GetSecretOwnerKinds(gomock.Any(), gomock.Any()).
		Return([]secret.SecretOwnerInfo{{
			SecretID:  uri1.ID,
			OwnerKind: secret.ApplicationCharmSecretOwner,
		}}, nil)

	result, err := s.facade.prepareSecretGrants(c.Context(), unitName, []params.GrantRevokeSecretArg{
		{URI: uri1.String(), ScopeTag: names.NewApplicationTag("app1").String(),
			SubjectTags: []string{names.NewApplicationTag("app1").String()}, Role: "view"},
		{URI: uri2.String(), ScopeTag: names.NewApplicationTag("app1").String(),
			SubjectTags: []string{names.NewApplicationTag("app2").String()}, Role: "view"},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0].URI.ID, tc.Equals, uri1.ID)
	c.Check(result[0].OwnerKind, tc.Equals, secret.ApplicationCharmSecretOwner)
}

func (s *UniterSecretsSuite) TestPrepareSecretGrantsSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri, unitName).Return(nil)
	s.secretService.EXPECT().ResolveGrantParams(gomock.Any(), gomock.Any()).
		Return([]secret.GrantResult{{GrantParams: secret.GrantParams{
			SubjectUUID: "uuid-app", SubjectTypeID: secret.SubjectApplication,
			ScopeUUID: "scope-rel", ScopeTypeID: secret.ScopeRelation, RoleID: secret.RoleView,
		}}})
	s.secretService.EXPECT().GetSecretOwnerKinds(gomock.Any(), []*coresecrets.URI{uri}).
		Return([]secret.SecretOwnerInfo{{
			SecretID:  uri.ID,
			OwnerKind: secret.UnitCharmSecretOwner,
		}}, nil)

	result, err := s.facade.prepareSecretGrants(c.Context(), unitName, []params.GrantRevokeSecretArg{{
		URI:         uri.String(),
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewApplicationTag("two").String()},
		Role:        "view",
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0].URI.String(), tc.Equals, uri.String())
	c.Check(result[0].SubjectUUID, tc.Equals, "uuid-app")
	c.Check(result[0].SubjectTypeID, tc.Equals, secret.SubjectApplication)
	c.Check(result[0].ScopeUUID, tc.Equals, "scope-rel")
	c.Check(result[0].ScopeTypeID, tc.Equals, secret.ScopeRelation)
	c.Check(result[0].RoleID, tc.Equals, secret.RoleView)
	c.Check(result[0].OwnerKind, tc.Equals, secret.UnitCharmSecretOwner)
}

// --- resolveGrantSubjects tests ---

func (s *UniterSecretsSuite) TestResolveGrantSubjectsInvalidScopeTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	accessor := secret.SecretAccessor{Kind: secret.UnitAccessor, ID: "mariadb/0"}
	_, err := s.facade.resolveGrantSubjects(c.Context(), accessor, params.GrantRevokeSecretArg{
		ScopeTag:    "bad-tag",
		SubjectTags: []string{names.NewApplicationTag("app1").String()},
	})
	c.Assert(err, tc.ErrorMatches, `.*"bad-tag" is not a valid tag.*`)
}

func (s *UniterSecretsSuite) TestResolveGrantSubjectsInvalidRole(c *tc.C) {
	defer s.setupMocks(c).Finish()

	accessor := secret.SecretAccessor{Kind: secret.UnitAccessor, ID: "mariadb/0"}
	_, err := s.facade.resolveGrantSubjects(c.Context(), accessor, params.GrantRevokeSecretArg{
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewApplicationTag("app1").String()},
		Role:        "not-valid",
	})
	c.Assert(err, tc.ErrorMatches, `secret role "not-valid" not valid`)
}

func (s *UniterSecretsSuite) TestResolveGrantSubjectsResolveParamsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	accessor := secret.SecretAccessor{Kind: secret.UnitAccessor, ID: "mariadb/0"}
	s.secretService.EXPECT().ResolveGrantParams(gomock.Any(), gomock.Any()).
		Return([]secret.GrantResult{{Error: errors.New("resolve failed")}})

	results, err := s.facade.resolveGrantSubjects(c.Context(), accessor, params.GrantRevokeSecretArg{
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewApplicationTag("app1").String()},
		Role:        "view",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0].err, tc.ErrorMatches, "resolve failed")
}

func (s *UniterSecretsSuite) TestResolveGrantSubjectsSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	accessor := secret.SecretAccessor{Kind: secret.UnitAccessor, ID: "mariadb/0"}
	s.secretService.EXPECT().ResolveGrantParams(gomock.Any(), gomock.Any()).
		Return([]secret.GrantResult{
			{GrantParams: secret.GrantParams{
				SubjectUUID: "uuid-app", SubjectTypeID: secret.SubjectApplication,
				ScopeUUID: "scope-rel", ScopeTypeID: secret.ScopeRelation, RoleID: secret.RoleView,
			}},
			{GrantParams: secret.GrantParams{
				SubjectUUID: "uuid-unit", SubjectTypeID: secret.SubjectUnit,
				ScopeUUID: "scope-rel", ScopeTypeID: secret.ScopeRelation, RoleID: secret.RoleView,
			}},
		})

	results, err := s.facade.resolveGrantSubjects(c.Context(), accessor, params.GrantRevokeSecretArg{
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewApplicationTag("app1").String(), names.NewUnitTag("app1/0").String()},
		Role:        "view",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 2)
	c.Check(results[0].SubjectUUID, tc.Equals, "uuid-app")
	c.Check(results[0].ScopeUUID, tc.Equals, "scope-rel")
	c.Check(results[0].RoleID, tc.Equals, secret.RoleView)
	c.Check(results[0].err, tc.ErrorIsNil)
	c.Check(results[1].SubjectUUID, tc.Equals, "uuid-unit")
	c.Check(results[1].err, tc.ErrorIsNil)
}

// --- resolveGrantOwnerKinds tests ---

func (s *UniterSecretsSuite) TestResolveGrantOwnerKindsEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	result, err := s.facade.resolveGrantOwnerKinds(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

func (s *UniterSecretsSuite) TestResolveGrantOwnerKindsServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.secretService.EXPECT().GetSecretOwnerKinds(gomock.Any(), []*coresecrets.URI{uri}).
		Return(nil, errors.New("db gone"))

	_, err := s.facade.resolveGrantOwnerKinds(c.Context(), []unitstate.GrantSecretArg{{
		URI: uri,
	}})
	c.Assert(err, tc.ErrorMatches, "db gone")
}

func (s *UniterSecretsSuite) TestResolveGrantOwnerKindsFiltersDisappeared(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()

	s.secretService.EXPECT().GetSecretOwnerKinds(gomock.Any(), gomock.Any()).
		Return([]secret.SecretOwnerInfo{{
			SecretID:  uri1.ID,
			OwnerKind: secret.ApplicationCharmSecretOwner,
		}}, nil)

	result, err := s.facade.resolveGrantOwnerKinds(c.Context(), []unitstate.GrantSecretArg{
		{URI: uri1, SubjectUUID: "a"},
		{URI: uri2, SubjectUUID: "b"},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0].URI.ID, tc.Equals, uri1.ID)
	c.Check(result[0].OwnerKind, tc.Equals, secret.ApplicationCharmSecretOwner)
}

// --- prepareSecretDeletes tests ---

func (s *UniterSecretsSuite) TestPrepareSecretDeletesInvalidURI(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	_, err := s.facade.prepareSecretDeletes(c.Context(), unitName, []params.DeleteSecretArg{{
		URI: "not-a-valid-uri-%%%",
	}})
	c.Assert(err, tc.ErrorMatches, `.*invalid URL escape.*`)
}

func (s *UniterSecretsSuite) TestPrepareSecretDeletesNotFoundSkipped(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri, unitName).
		Return(secreterrors.SecretNotFound)

	result, err := s.facade.prepareSecretDeletes(c.Context(), unitName, []params.DeleteSecretArg{{
		URI: uri.String(),
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

func (s *UniterSecretsSuite) TestPrepareSecretDeletesPermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri, unitName).
		Return(secreterrors.PermissionDenied)

	_, err := s.facade.prepareSecretDeletes(c.Context(), unitName, []params.DeleteSecretArg{{
		URI: uri.String(),
	}})
	c.Assert(err, tc.ErrorMatches, `removing secrets: permission denied`)
}

func (s *UniterSecretsSuite) TestPrepareSecretDeletesMixed(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uriA := coresecrets.NewURI()
	uriB := coresecrets.NewURI()
	uriC := coresecrets.NewURI()

	// A: access granted
	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uriA, unitName).Return(nil)
	// B: not found (silently skipped)
	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uriB, unitName).
		Return(secreterrors.SecretNotFound)
	// C: permission denied (error collected)
	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uriC, unitName).
		Return(secreterrors.PermissionDenied)

	_, err := s.facade.prepareSecretDeletes(c.Context(), unitName, []params.DeleteSecretArg{
		{URI: uriA.String(), Revisions: []int{1}},
		{URI: uriB.String()},
		{URI: uriC.String()},
	})
	c.Assert(err, tc.ErrorMatches, `removing secrets: permission denied`)
}

func (s *UniterSecretsSuite) TestPrepareSecretDeletesSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri, unitName).Return(nil)
	s.secretService.EXPECT().GetSecretOwnerKinds(gomock.Any(), []*coresecrets.URI{uri}).
		Return([]secret.SecretOwnerInfo{{
			SecretID:  uri.ID,
			OwnerKind: secret.UnitCharmSecretOwner,
		}}, nil)

	result, err := s.facade.prepareSecretDeletes(c.Context(), unitName, []params.DeleteSecretArg{{
		URI:       uri.String(),
		Revisions: []int{1, 3},
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0].URI.String(), tc.Equals, uri.String())
	c.Check(result[0].Revisions, tc.DeepEquals, []int{1, 3})
	c.Check(result[0].OwnerKind, tc.Equals, secret.UnitCharmSecretOwner)
}

// --- resolveDeleteOwnerKinds tests ---

func (s *UniterSecretsSuite) TestResolveDeleteOwnerKindsEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	result, err := s.facade.resolveDeleteOwnerKinds(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

func (s *UniterSecretsSuite) TestResolveDeleteOwnerKindsServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.secretService.EXPECT().GetSecretOwnerKinds(gomock.Any(), []*coresecrets.URI{uri}).
		Return(nil, errors.New("db gone"))

	_, err := s.facade.resolveDeleteOwnerKinds(c.Context(), []unitstate.DeleteSecretArg{{
		URI: uri,
	}})
	c.Assert(err, tc.ErrorMatches, "db gone")
}

func (s *UniterSecretsSuite) TestResolveDeleteOwnerKindsFiltersDisappeared(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()

	s.secretService.EXPECT().GetSecretOwnerKinds(gomock.Any(), gomock.Any()).
		Return([]secret.SecretOwnerInfo{{
			SecretID:  uri1.ID,
			OwnerKind: secret.ApplicationCharmSecretOwner,
		}}, nil)

	result, err := s.facade.resolveDeleteOwnerKinds(c.Context(), []unitstate.DeleteSecretArg{
		{URI: uri1},
		{URI: uri2},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0].URI.ID, tc.Equals, uri1.ID)
	c.Check(result[0].OwnerKind, tc.Equals, secret.ApplicationCharmSecretOwner)
}
