// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"testing"

	gomock "github.com/canonical/gomock/gomock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	domainmodel "github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

// migratedModelRedirectSuite tests the login-time decision of whether a login
// to a model that has migrated away from this controller should be redirected
// to the controller now hosting it. The decision is pure branching over what
// the model domain reports, so it is exercised against a mocked
// [ModelRedirectService] rather than a live database.
//
// The domain-level responsibilities the decision relies on are tested where
// they live: that [ModelRedirectService.ModelRedirectUsers] excludes users who
// can no longer log in (disabled, removed or vanished since the migration
// snapshot) is covered by the model state tests. Here we only assert how the
// login flow reacts to the users it is given.
type migratedModelRedirectSuite struct {
	modelService *MockModelRedirectService
}

func TestMigratedModelRedirectSuite(t *testing.T) {
	tc.Run(t, &migratedModelRedirectSuite{})
}

func (s *migratedModelRedirectSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.modelService = NewMockModelRedirectService(ctrl)
	c.Cleanup(func() {
		s.modelService = nil
	})
	return ctrl
}

// redirectTarget returns a representative redirect snapshot for a model that
// has migrated to another controller.
func redirectTarget() domainmodel.ModelRedirection {
	return domainmodel.ModelRedirection{
		Addresses:       []string{"10.10.10.10:17070"},
		CACert:          "target-ca-cert",
		ControllerUUID:  uuid.MustNewUUID().String(),
		ControllerAlias: "target-alias",
	}
}

// assertRedirectedTo asserts err is a [apiservererrors.RedirectError] carrying
// the given target's coordinates.
func (s *migratedModelRedirectSuite) assertRedirectedTo(c *tc.C, err error, target domainmodel.ModelRedirection) {
	rErr, ok := errors.AsType[*apiservererrors.RedirectError](err)
	c.Assert(ok, tc.IsTrue, tc.Commentf("expected a redirect error, got %v", err))
	c.Check(rErr.CACert, tc.Equals, target.CACert)
	c.Check(rErr.ControllerAlias, tc.Equals, target.ControllerAlias)
	c.Check(rErr.ControllerTag, tc.Equals, names.NewControllerTag(target.ControllerUUID))

	expectedServers, err := network.ParseProviderHostPorts(target.Addresses...)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(rErr.Servers, tc.DeepEquals, []network.ProviderHostPorts{expectedServers})
}

// TestLocalUserWithCapturedAccessRedirected asserts a local user whose model
// access was captured at migration time is redirected to the target controller.
func (s *migratedModelRedirectSuite) TestLocalUserWithCapturedAccessRedirected(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	target := redirectTarget()
	s.modelService.EXPECT().ModelRedirection(gomock.Any(), modelUUID).Return(target, nil)
	s.modelService.EXPECT().ModelRedirectUsers(gomock.Any(), modelUUID).Return([]domainmodel.RedirectUser{
		{UserName: "someone-else", Access: "read"},
		{UserName: "charlie", Access: "admin"},
	}, nil)

	req := params.LoginRequest{AuthTag: names.NewUserTag("charlie").String()}
	err := redirectErrorForMigratedModel(c.Context(), s.modelService, modelUUID, req)
	s.assertRedirectedTo(c, err, target)
}

// TestLocalUserWithoutCapturedAccessNotRedirected asserts a local user with no
// captured model access is not redirected: the migrated model's new location
// must not leak to users who had no access to it.
func (s *migratedModelRedirectSuite) TestLocalUserWithoutCapturedAccessNotRedirected(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.modelService.EXPECT().ModelRedirection(gomock.Any(), modelUUID).Return(redirectTarget(), nil)
	s.modelService.EXPECT().ModelRedirectUsers(gomock.Any(), modelUUID).Return([]domainmodel.RedirectUser{
		{UserName: "someone-else", Access: "admin"},
	}, nil)

	req := params.LoginRequest{AuthTag: names.NewUserTag("charlie").String()}
	err := redirectErrorForMigratedModel(c.Context(), s.modelService, modelUUID, req)
	c.Assert(err, tc.ErrorIsNil)
}

// TestDisabledUserNotRedirected asserts a local user who can no longer log in
// (disabled or removed since the snapshot) is not redirected. Such users are
// filtered out of ModelRedirectUsers by the state layer, so from the login
// flow's point of view they simply have no captured access.
func (s *migratedModelRedirectSuite) TestDisabledUserNotRedirected(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.modelService.EXPECT().ModelRedirection(gomock.Any(), modelUUID).Return(redirectTarget(), nil)
	// The disabled user has been dropped from the captured set.
	s.modelService.EXPECT().ModelRedirectUsers(gomock.Any(), modelUUID).Return(nil, nil)

	req := params.LoginRequest{AuthTag: names.NewUserTag("carol").String()}
	err := redirectErrorForMigratedModel(c.Context(), s.modelService, modelUUID, req)
	c.Assert(err, tc.ErrorIsNil)
}

// TestAnonymousUserRedirected asserts anonymous logins are always redirected so
// cross-model relations keep working after migration, without consulting the
// captured user access.
func (s *migratedModelRedirectSuite) TestAnonymousUserRedirected(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	target := redirectTarget()
	// ModelRedirectUsers must not be consulted for anonymous logins.
	s.modelService.EXPECT().ModelRedirection(gomock.Any(), modelUUID).Return(target, nil)

	req := params.LoginRequest{AuthTag: names.NewUserTag(api.AnonymousUsername).String()}
	err := redirectErrorForMigratedModel(c.Context(), s.modelService, modelUUID, req)
	s.assertRedirectedTo(c, err, target)
}

// TestTokenLoginRedirected asserts a token (JWT) login, which carries no auth
// tag, is always redirected: the token's issuer (e.g. JAAS) is the authority
// for the user's model access, so there is no local record to consult.
func (s *migratedModelRedirectSuite) TestTokenLoginRedirected(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	target := redirectTarget()
	s.modelService.EXPECT().ModelRedirection(gomock.Any(), modelUUID).Return(target, nil)

	req := params.LoginRequest{Token: "a-jwt-from-an-external-authority"}
	err := redirectErrorForMigratedModel(c.Context(), s.modelService, modelUUID, req)
	s.assertRedirectedTo(c, err, target)
}

// TestExternalUserRedirected asserts an external user is redirected even
// without captured model access: external users may hold access via their
// identity provider without any record on this controller.
func (s *migratedModelRedirectSuite) TestExternalUserRedirected(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	target := redirectTarget()
	s.modelService.EXPECT().ModelRedirection(gomock.Any(), modelUUID).Return(target, nil)

	req := params.LoginRequest{AuthTag: names.NewUserTag("fred@external").String()}
	err := redirectErrorForMigratedModel(c.Context(), s.modelService, modelUUID, req)
	s.assertRedirectedTo(c, err, target)
}

// TestModelNotRedirected asserts that when the model is still served by this
// controller (no redirect snapshot) the login is not redirected.
func (s *migratedModelRedirectSuite) TestModelNotRedirected(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.modelService.EXPECT().ModelRedirection(gomock.Any(), modelUUID).
		Return(domainmodel.ModelRedirection{}, modelerrors.ModelNotRedirected)

	req := params.LoginRequest{AuthTag: names.NewUserTag("charlie").String()}
	err := redirectErrorForMigratedModel(c.Context(), s.modelService, modelUUID, req)
	c.Assert(err, tc.ErrorIsNil)
}

// TestAgentLoginNotRedirected asserts a machine (agent) login is never
// redirected: agents follow a migration via the migrationminion protocol, not
// a login redirect. The redirect lookup is not even consulted.
func (s *migratedModelRedirectSuite) TestAgentLoginNotRedirected(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	// No ModelRedirection expectation: an agent tag returns before the lookup.

	req := params.LoginRequest{AuthTag: names.NewMachineTag("0").String()}
	err := redirectErrorForMigratedModel(c.Context(), s.modelService, modelUUID, req)
	c.Assert(err, tc.ErrorIsNil)
}

// TestNoAuthTagOrTokenNotRedirected asserts a request bearing neither an auth
// tag nor a token is not redirected and does not consult the redirect lookup.
func (s *migratedModelRedirectSuite) TestNoAuthTagOrTokenNotRedirected(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)

	err := redirectErrorForMigratedModel(c.Context(), s.modelService, modelUUID, params.LoginRequest{})
	c.Assert(err, tc.ErrorIsNil)
}

// TestInvalidAuthTagErrors asserts an unparseable auth tag surfaces an error
// rather than silently proceeding.
func (s *migratedModelRedirectSuite) TestInvalidAuthTagErrors(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)

	req := params.LoginRequest{AuthTag: "not-a-valid-tag"}
	err := redirectErrorForMigratedModel(c.Context(), s.modelService, modelUUID, req)
	c.Assert(err, tc.NotNil)
}

// TestModelRedirectionErrorPropagated asserts an unexpected error from the
// redirect lookup is propagated rather than swallowed.
func (s *migratedModelRedirectSuite) TestModelRedirectionErrorPropagated(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	boom := errors.New("boom")
	s.modelService.EXPECT().ModelRedirection(gomock.Any(), modelUUID).
		Return(domainmodel.ModelRedirection{}, boom)

	req := params.LoginRequest{AuthTag: names.NewUserTag("charlie").String()}
	err := redirectErrorForMigratedModel(c.Context(), s.modelService, modelUUID, req)
	c.Assert(err, tc.ErrorIs, boom)
}

// TestModelRedirectUsersErrorPropagated asserts an error while reading the
// captured users is propagated.
func (s *migratedModelRedirectSuite) TestModelRedirectUsersErrorPropagated(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	boom := errors.New("boom")
	s.modelService.EXPECT().ModelRedirection(gomock.Any(), modelUUID).Return(redirectTarget(), nil)
	s.modelService.EXPECT().ModelRedirectUsers(gomock.Any(), modelUUID).Return(nil, boom)

	req := params.LoginRequest{AuthTag: names.NewUserTag("charlie").String()}
	err := redirectErrorForMigratedModel(c.Context(), s.modelService, modelUUID, req)
	c.Assert(err, tc.ErrorIs, boom)
}
