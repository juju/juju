// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

import (
	"context"
	"testing"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakerytest"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/permission"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/domain/access/service"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/juju/testing"
)

type userAuthenticatorSuite struct {
	jujutesting.ApiServerSuite
}

func TestUserAuthenticatorSuite(t *testing.T) {
	tc.Run(t, &userAuthenticatorSuite{})
}

func (s *userAuthenticatorSuite) TestMachineLoginFails(c *tc.C) {

	authenticator := &authentication.LocalUserAuthenticator{}
	_, _, err := authenticator.Authenticate(c.Context(), authentication.AuthParams{
		AuthTag:     names.NewMachineTag("0"),
		Credentials: "I am a machine",
		Nonce:       "Ya nonce!",
	})
	c.Assert(err, tc.ErrorMatches, "invalid request")
}

func (s *userAuthenticatorSuite) TestUnitLoginFails(c *tc.C) {
	// Attempt unit login,
	authenticator := &authentication.LocalUserAuthenticator{}
	_, _, err := authenticator.Authenticate(c.Context(), authentication.AuthParams{
		AuthTag:     names.NewUnitTag("vault/0"),
		Credentials: "I am a unit",
	})
	c.Assert(err, tc.ErrorMatches, "invalid request")
}

func (s *userAuthenticatorSuite) TestValidUserLogin(c *tc.C) {
	userService := s.ControllerDomainServices(c).Access()
	_, _, err := userService.AddUser(c.Context(), service.AddUserArg{
		Name:        usertesting.GenNewName(c, "bobbrown"),
		DisplayName: "Bob Brown",
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword("password")),
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	authenticator := &authentication.LocalUserAuthenticator{
		UserService: s.ControllerDomainServices(c).Access(),
	}
	entity, _, err := authenticator.Authenticate(c.Context(), authentication.AuthParams{
		AuthTag:     names.NewUserTag("bobbrown"),
		Credentials: "password",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entity.Tag(), tc.Equals, names.NewUserTag("bobbrown"))
}

func (s *userAuthenticatorSuite) TestDisabledUserLogin(c *tc.C) {
	userService := s.ControllerDomainServices(c).Access()
	name := usertesting.GenNewName(c, "bobbrown")
	_, _, err := userService.AddUser(c.Context(), service.AddUserArg{
		Name:        name,
		DisplayName: "Bob Brown",
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword("password")),
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	err = userService.DisableUserAuthentication(c.Context(), name)
	c.Assert(err, tc.ErrorIsNil)

	authenticator := &authentication.LocalUserAuthenticator{
		UserService: s.ControllerDomainServices(c).Access(),
	}
	_, _, err = authenticator.Authenticate(c.Context(), authentication.AuthParams{
		AuthTag:     names.NewUserTag("bobbrown"),
		Credentials: "password",
	})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrUnauthorized)
}

func (s *userAuthenticatorSuite) TestRemovedUserLogin(c *tc.C) {
	userService := s.ControllerDomainServices(c).Access()
	name := usertesting.GenNewName(c, "bobbrown")
	_, _, err := userService.AddUser(c.Context(), service.AddUserArg{
		Name:        name,
		DisplayName: "Bob Brown",
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword("password")),
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	err = userService.RemoveUser(c.Context(), name)
	c.Assert(err, tc.ErrorIsNil)

	// User login
	authenticator := &authentication.LocalUserAuthenticator{
		UserService: s.ControllerDomainServices(c).Access(),
	}
	_, _, err = authenticator.Authenticate(c.Context(), authentication.AuthParams{
		AuthTag:     names.NewUserTag("bobbrown"),
		Credentials: "password",
	})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrUnauthorized)
}

func (s *userAuthenticatorSuite) TestUserLoginWrongPassword(c *tc.C) {
	userService := s.ControllerDomainServices(c).Access()
	name := usertesting.GenNewName(c, "bobbrown")
	_, _, err := userService.AddUser(c.Context(), service.AddUserArg{
		Name:        name,
		DisplayName: "Bob Brown",
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword("password")),
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	// User login
	authenticator := &authentication.LocalUserAuthenticator{
		UserService: s.ControllerDomainServices(c).Access(),
	}
	_, _, err = authenticator.Authenticate(c.Context(), authentication.AuthParams{
		AuthTag:     names.NewUserTag(name.Name()),
		Credentials: "wrongpassword",
	})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrUnauthorized)
}

func (s *userAuthenticatorSuite) TestValidMacaroonUserLogin(c *tc.C) {
	userService := s.ControllerDomainServices(c).Access()
	name := usertesting.GenNewName(c, "bob")
	_, _, err := userService.AddUser(c.Context(), service.AddUserArg{
		Name:        name,
		DisplayName: "Bob Brown",
		CreatorUUID: s.AdminUserUUID,
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	mac, err := macaroon.New(nil, nil, "", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	err = mac.AddFirstPartyCaveat([]byte("declared username bob"))
	c.Assert(err, tc.ErrorIsNil)
	macaroons := []macaroon.Slice{{mac}}
	bakeryService := mockBakeryService{}

	// User login
	authenticator := &authentication.LocalUserAuthenticator{
		UserService: s.ControllerDomainServices(c).Access(),
		Bakery:      &bakeryService,
		Clock:       testclock.NewClock(time.Time{}),
	}
	entity, _, err := authenticator.Authenticate(c.Context(), authentication.AuthParams{
		AuthTag:   names.NewUserTag(name.Name()),
		Macaroons: macaroons,
	})
	c.Assert(err, tc.ErrorIsNil)

	bakeryService.CheckCallNames(c, "Auth")
	call := bakeryService.Calls()[0]
	c.Assert(call.Args, tc.HasLen, 1)
	c.Assert(call.Args[0], tc.DeepEquals, macaroons)
	c.Check(entity.Tag(), tc.Equals, names.NewUserTag(name.Name()))
}

func (s *userAuthenticatorSuite) TestInvalidMacaroonUserLogin(c *tc.C) {
	userService := s.ControllerDomainServices(c).Access()
	_, _, err := userService.AddUser(c.Context(), service.AddUserArg{
		Name:        usertesting.GenNewName(c, "bobbrown"),
		DisplayName: "Bob Brown",
		CreatorUUID: s.AdminUserUUID,
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	mac, err := macaroon.New(nil, nil, "", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	err = mac.AddFirstPartyCaveat([]byte("declared username fred"))
	c.Assert(err, tc.ErrorIsNil)
	macaroons := []macaroon.Slice{{mac}}
	bakeryService := mockBakeryService{}

	// User login
	authenticator := &authentication.LocalUserAuthenticator{
		UserService: s.ControllerDomainServices(c).Access(),
		Bakery:      &bakeryService,
		Clock:       testclock.NewClock(time.Time{}),
	}
	_, _, err = authenticator.Authenticate(c.Context(), authentication.AuthParams{
		AuthTag:   names.NewUserTag("bob"),
		Macaroons: macaroons,
	})
	c.Assert(err, tc.ErrorIs, authentication.ErrInvalidLoginMacaroon)
}

func (s *userAuthenticatorSuite) TestDisabledMacaroonUserLogin(c *tc.C) {
	userService := s.ControllerDomainServices(c).Access()
	name := usertesting.GenNewName(c, "bobbrown")
	_, _, err := userService.AddUser(c.Context(), service.AddUserArg{
		Name:        name,
		DisplayName: "Bob Brown",
		CreatorUUID: s.AdminUserUUID,
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	err = userService.DisableUserAuthentication(c.Context(), name)
	c.Assert(err, tc.ErrorIsNil)

	mac, err := macaroon.New(nil, nil, "", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	err = mac.AddFirstPartyCaveat([]byte("declared username bob"))
	c.Assert(err, tc.ErrorIsNil)
	macaroons := []macaroon.Slice{{mac}}
	bakeryService := mockBakeryService{}

	// User login
	authenticator := &authentication.LocalUserAuthenticator{
		UserService: s.ControllerDomainServices(c).Access(),
		Bakery:      &bakeryService,
		Clock:       testclock.NewClock(time.Time{}),
	}
	_, _, err = authenticator.Authenticate(c.Context(), authentication.AuthParams{
		AuthTag:   names.NewUserTag("bob"),
		Macaroons: macaroons,
	})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrUnauthorized)
}

func (s *userAuthenticatorSuite) TestRemovedMacaroonUserLogin(c *tc.C) {
	userService := s.ControllerDomainServices(c).Access()
	name := usertesting.GenNewName(c, "bobbrown")
	_, _, err := userService.AddUser(c.Context(), service.AddUserArg{
		Name:        name,
		DisplayName: "Bob Brown",
		CreatorUUID: s.AdminUserUUID,
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	err = userService.RemoveUser(c.Context(), name)
	c.Assert(err, tc.ErrorIsNil)

	mac, err := macaroon.New(nil, nil, "", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	err = mac.AddFirstPartyCaveat([]byte("declared username bob"))
	c.Assert(err, tc.ErrorIsNil)
	macaroons := []macaroon.Slice{{mac}}
	bakeryService := mockBakeryService{}

	// User login.
	authenticator := &authentication.LocalUserAuthenticator{
		UserService: s.ControllerDomainServices(c).Access(),
		Bakery:      &bakeryService,
		Clock:       testclock.NewClock(time.Time{}),
	}
	_, _, err = authenticator.Authenticate(c.Context(), authentication.AuthParams{
		AuthTag:   names.NewUserTag("bob"),
		Macaroons: macaroons,
	})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrUnauthorized)
}

func (s *userAuthenticatorSuite) TestInvalidRelationLogin(c *tc.C) {
	authenticator := &authentication.LocalUserAuthenticator{}
	_, _, err := authenticator.Authenticate(c.Context(), authentication.AuthParams{
		AuthTag:     names.NewRelationTag("this-app:rel that-app:rel"),
		Credentials: "I am a relation",
	})
	c.Assert(err, tc.ErrorMatches, "invalid request")
}

func (s *userAuthenticatorSuite) TestCreateLocalLoginMacaroon(c *tc.C) {
	service := mockBakeryService{}
	clock := testclock.NewClock(time.Time{})
	_, err := authentication.CreateLocalLoginMacaroon(
		c.Context(),
		names.NewUserTag("bobbrown"), &service, clock, bakery.LatestVersion,
	)
	c.Assert(err, tc.ErrorIsNil)
	service.CheckCallNames(c, "NewMacaroon")
	service.CheckCall(c, 0, "NewMacaroon", []checkers.Caveat{
		{Condition: "is-authenticated-user bobbrown"},
		{Condition: "time-before 0001-01-01T00:02:00Z", Namespace: "std"},
	})
}

func (s *userAuthenticatorSuite) TestAuthenticateLocalLoginMacaroon(c *tc.C) {
	service := mockBakeryService{}
	clock := testclock.NewClock(time.Time{})
	authenticator := &authentication.LocalUserAuthenticator{
		Bakery:                    &service,
		Clock:                     clock,
		LocalUserIdentityLocation: "https://testing.invalid:1234/auth",
	}

	service.SetErrors(nil, &bakery.VerificationError{})
	_, _, err := authenticator.Authenticate(
		c.Context(),
		authentication.AuthParams{
			AuthTag: names.NewUserTag("bobbrown"),
		},
	)
	c.Assert(err, tc.FitsTypeOf, &apiservererrors.DischargeRequiredError{})

	service.CheckCallNames(c, "Auth", "ExpireStorageAfter", "NewMacaroon")
	calls := service.Calls()
	c.Assert(calls[1].Args, tc.DeepEquals, []interface{}{24 * time.Hour})
	c.Assert(calls[2].Args, tc.DeepEquals, []interface{}{
		[]checkers.Caveat{
			{Condition: "time-before 0001-01-02T00:00:00Z", Namespace: "std"},
			checkers.NeedDeclaredCaveat(
				checkers.Caveat{
					Location:  "https://testing.invalid:1234/auth",
					Condition: "is-authenticated-user bobbrown",
					Namespace: "std",
				},
				"username",
			),
		},
	})
}

type mockBakeryService struct {
	testhelpers.Stub
}

func (s *mockBakeryService) Auth(_ context.Context, mss ...macaroon.Slice) *bakery.AuthChecker {
	s.MethodCall(s, "Auth", mss)
	checker := bakery.NewChecker(bakery.CheckerParams{
		OpsAuthorizer:    mockAuthorizer{},
		MacaroonVerifier: mockVerifier{},
	})
	return checker.Auth(mss...)
}

func (s *mockBakeryService) NewMacaroon(ctx context.Context, version bakery.Version, caveats []checkers.Caveat, ops ...bakery.Op) (*bakery.Macaroon, error) {
	s.MethodCall(s, "NewMacaroon", caveats)
	mac, err := macaroon.New(nil, nil, "", macaroon.LatestVersion)
	if err != nil {
		return nil, err
	}
	return bakery.NewLegacyMacaroon(mac)
}

func (s *mockBakeryService) ExpireStorageAfter(t time.Duration) (authentication.ExpirableStorageBakery, error) {
	s.MethodCall(s, "ExpireStorageAfter", t)
	err := s.NextErr()
	return s, err
}

type mockAuthorizer struct{}

func (mockAuthorizer) AuthorizeOps(ctx context.Context, authorizedOp bakery.Op, queryOps []bakery.Op) ([]bool, []checkers.Caveat, error) {
	allowed := make([]bool, len(queryOps))
	for i := range allowed {
		allowed[i] = queryOps[i] == identchecker.LoginOp
	}
	return allowed, nil, nil
}

type mockVerifier struct{}

func (mockVerifier) VerifyMacaroon(ctx context.Context, ms macaroon.Slice) ([]bakery.Op, []string, error) {
	return []bakery.Op{identchecker.LoginOp}, []string{"declared username bob"}, nil
}

type macaroonAuthenticatorSuite struct {
	jujutesting.ApiServerSuite
	// username holds the username that will be
	// declared in the discharger's caveats.
	username string
}

func TestMacaroonAuthenticatorSuite(t *testing.T) {
	tc.Run(t, &macaroonAuthenticatorSuite{})
}

var authenticateSuccessTests = []struct {
	about              string
	dischargedUsername string
	expectTag          string
	expectError        string
}{{
	about:              "user that can be found",
	dischargedUsername: "bobbrown@somewhere",
	expectTag:          "user-bobbrown@somewhere",
}, {
	about:              "user with no @ domain",
	dischargedUsername: "bobbrown",
	expectTag:          "user-bobbrown@external",
}, {
	about:              "invalid user name",
	dischargedUsername: "--",
	expectError:        `"--" is an invalid user name`,
}, {
	about:              "ostensibly local name",
	dischargedUsername: "cheat@local",
	expectError:        `external identity provider has provided ostensibly local name "cheat@local"`,
}}

type alwaysIdent struct {
	IdentityLocation string
	username         string
}

// IdentityFromContext implements IdentityClient.IdentityFromContext.
func (m *alwaysIdent) IdentityFromContext(ctx context.Context) (identchecker.Identity, []checkers.Caveat, error) {
	return nil, []checkers.Caveat{checkers.DeclaredCaveat("username", m.username)}, nil
}

func (alwaysIdent) DeclaredIdentity(ctx context.Context, declared map[string]string) (identchecker.Identity, error) {
	user := declared["username"]
	return identchecker.SimpleIdentity(user), nil
}

func (s *macaroonAuthenticatorSuite) TestMacaroonAuthentication(c *tc.C) {
	discharger := bakerytest.NewDischarger(nil)
	defer discharger.Close()
	for i, test := range authenticateSuccessTests {
		c.Logf("\ntest %d; %s", i, test.about)
		s.username = test.dischargedUsername

		bakery := identchecker.NewBakery(identchecker.BakeryParams{
			Locator:        discharger,
			IdentityClient: &alwaysIdent{username: s.username},
		})
		authenticator := &authentication.ExternalMacaroonAuthenticator{
			Bakery:           bakery,
			IdentityLocation: discharger.Location(),
			Clock:            testclock.NewClock(time.Time{}),
		}

		// Authenticate once to obtain the macaroon to be discharged.
		_, _, err := authenticator.Authenticate(c.Context(), authentication.AuthParams{})

		// Discharge the macaroon.
		dischargeErr := errors.Cause(err).(*apiservererrors.DischargeRequiredError)
		client := httpbakery.NewClient()
		ms, err := client.DischargeAll(c.Context(), dischargeErr.Macaroon)
		c.Assert(err, tc.ErrorIsNil)

		// Authenticate again with the discharged macaroon.
		entity, _, err := authenticator.Authenticate(c.Context(), authentication.AuthParams{
			Macaroons: []macaroon.Slice{ms},
		})
		if test.expectError != "" {
			c.Assert(err, tc.ErrorMatches, test.expectError)
			c.Assert(entity, tc.Equals, nil)
		} else {
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(entity.Tag().String(), tc.Equals, test.expectTag)
		}
	}
}

func ptr[T any](t T) *T {
	return &t
}
