// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"fmt"
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v0/bakery"
	"gopkg.in/macaroon-bakery.v0/bakery/checkers"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

type remoteLoginSuite struct {
	loginSuite

	info            state.StateServingInfo
	remoteIdKey     *bakery.KeyPair
	remoteIdService *bakery.Service
}

type loggedInChecker struct {
	user string
}

func newLoggedInChecker(user string) *loggedInChecker {
	return &loggedInChecker{user: user}
}

// CheckThirdPartyCaveat implements the macaroon.ThirdPartyChecker interface.
func (c *loggedInChecker) CheckThirdPartyCaveat(caveatId, condition string) ([]bakery.Caveat, error) {
	question, _, err := checkers.ParseCaveat(condition)
	if err != nil {
		return nil, errors.Trace(err)
	}

	switch question {
	case "is-authenticated-user":
		return []bakery.Caveat{{Condition: "declared-user " + c.user}}, nil
	}
	return nil, errors.New("unrecognized condition")
}

var reauthDialOpts = api.DialOpts{}

var _ = gc.Suite(&remoteLoginSuite{
	// Extend the base login suite, ensuring we test fallback to non-remote
	// authentication.
	loginSuite: loginSuite{
		baseLoginSuite{
			setAdminApi: func(srv *apiserver.Server) {
				apiserver.SetAdminApiVersions(srv, 1)
			},
		},
	},
})

func (s *remoteLoginSuite) SetUpTest(c *gc.C) {
	s.loginSuite.SetUpTest(c)

	var err error
	s.remoteIdKey, err = bakery.GenerateKey()
	c.Assert(err, gc.IsNil)
	s.remoteIdService, err = bakery.NewService(bakery.NewServiceParams{
		Location: "remote-service-location",
		Key:      s.remoteIdKey,
	})
	c.Assert(err, gc.IsNil)

	// Configure state server to trust this remote identity provider
	// and have its own target service public key identity.
	s.info, err = s.State.StateServingInfo()
	c.Assert(err, gc.IsNil)
	s.info.IdentityProvider = &state.IdentityProvider{
		PublicKey: s.remoteIdKey.Public,
		Location:  "remote-service-location",
	}
	s.info.TargetKeyPair, err = bakery.GenerateKey()
	c.Assert(err, gc.IsNil)
	err = s.State.SetStateServingInfo(s.info)
	c.Assert(err, gc.IsNil)
}

func (s *remoteLoginSuite) TearDownTest(c *gc.C) {
	info, err := s.State.StateServingInfo()
	c.Assert(err, gc.IsNil)
	info.IdentityProvider = nil
	info.TargetKeyPair = nil
	err = s.State.SetStateServingInfo(info)
	c.Assert(err, gc.IsNil)

	s.loginSuite.TearDownTest(c)
}

func (s *remoteLoginSuite) dischargeReauth(user string, reauth *params.ReauthRequest) ([]byte, error) {
	// As the remote client, decode the reauth request, obtain a discharge
	// macaroon from the identity-providing service, bind and serialize the
	// followup credential.
	var remoteCreds authentication.RemoteCredentials
	err := remoteCreds.UnmarshalText([]byte(reauth.Prompt))
	if err != nil {
		return nil, errors.Trace(err)
	}
	env, err := s.State.Environment()
	if err != nil {
		return nil, errors.Trace(err)
	}
	remoteCreds.Discharges, err = bakery.DischargeAll(remoteCreds.Primary,
		func(loc string, cav macaroon.Caveat) (*macaroon.Macaroon, error) {
			// The first-party location should be the target Juju environment's tag.
			if loc != env.EnvironTag().Id() {
				return nil, errors.Errorf("invalid first-party location")
			}
			return s.remoteIdService.Discharge(newLoggedInChecker(user), cav.Id)
		},
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	remoteCreds.Bind()
	credBytes, err := remoteCreds.MarshalText()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return credBytes, nil
}

func (s *remoteLoginSuite) TestRemoteLoginReauth(c *gc.C) {
	info, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()
	st := s.openAPIWithoutLogin(c, info)

	// Try to log in as a remote identity.
	reauth, err := st.Login("", "", "")
	c.Assert(err, gc.IsNil)
	c.Assert(reauth, gc.NotNil)

	// No API facade versions. We're not logged in yet.
	c.Check(st.AllFacadeVersions(), gc.HasLen, 0)

	// Obtain follow-up credentials for the reauth challenge
	remoteUser := names.NewUserTag("bob")
	credBytes, err := s.dischargeReauth("bob", reauth)
	c.Assert(err, gc.IsNil)

	// Retry the remote login request
	reauth, err = st.Login(remoteUser.String(), string(credBytes), reauth.Nonce)
	c.Assert(err, gc.IsNil)
	c.Assert(reauth, gc.IsNil)

	// Should be logged in
	c.Assert(st.Ping(), gc.IsNil)
	c.Assert(len(st.AllFacadeVersions()), jc.GreaterThan, 0)
}

func (s *remoteLoginSuite) TestReauthInvalidUser(c *gc.C) {
	info, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()
	st := s.openAPIWithoutLogin(c, info)

	// Bob starts the login process.
	reauth, err := st.Login("", "", "")
	c.Assert(err, gc.IsNil)
	c.Assert(reauth, gc.NotNil)

	// No API facade versions. We're not logged in yet.
	c.Check(st.AllFacadeVersions(), gc.HasLen, 0)

	// Obtain follow-up credentials for the reauth challenge
	credBytes, err := s.dischargeReauth("bob", reauth)
	c.Assert(err, gc.IsNil)

	// Eve has been listening all along and tries to steal the
	// connection away from Bob!
	_, err = st.Login(names.NewUserTag("eve").String(), string(credBytes), reauth.Nonce)

	// Not so fast, Mallory!
	c.Assert(err, gc.ErrorMatches, "invalid user")
}

type emptyReauthHandler struct{}

// HandleReauth simulates a bad reauth scenario in which an empty user and
// credential is returned, which should produce an error.
func (h emptyReauthHandler) HandleReauth(reauth *params.ReauthRequest) (string, string, error) {
	return "", "", nil
}

type emptyCredReauthHandler struct{}

// HandleReauth simulates a bad reauth scenario in which an empty credential is
// returned, which should produce an error.
func (h emptyCredReauthHandler) HandleReauth(reauth *params.ReauthRequest) (string, string, error) {
	return "zarpedon", "", nil
}

type testReauthHandler struct {
	*remoteLoginSuite
	skipBind          bool
	thirdPartyChecker bakery.ThirdPartyChecker
}

func failReauth(err error) (string, string, error) {
	fmt.Fprintln(os.Stderr, "failReauth:", err)
	return "", "", errors.Trace(err)
}

// HandleReauth implements a reauthentication handler capable of discharging
// the third-party caveat challenge issued by Juju. It also contains logic to
// force failure modes for testing.
func (h testReauthHandler) HandleReauth(reauth *params.ReauthRequest) (string, string, error) {
	var remoteCreds authentication.RemoteCredentials
	err := remoteCreds.UnmarshalText([]byte(reauth.Prompt))
	if err != nil {
		return failReauth(errors.Trace(err))
	}
	remoteCreds.Discharges, err = bakery.DischargeAll(remoteCreds.Primary,
		func(loc string, cav macaroon.Caveat) (*macaroon.Macaroon, error) {
			return h.remoteIdService.Discharge(h.thirdPartyChecker, cav.Id)
		},
	)
	if err != nil {
		return failReauth(errors.Trace(err))
	}
	remoteUser, err := remoteCreds.RemoteUser()
	if err != nil {
		return failReauth(errors.Trace(err))
	}
	if !h.skipBind {
		remoteCreds.Bind()
	}
	credBytes, err := remoteCreds.MarshalText()
	if err != nil {
		return failReauth(errors.Trace(err))
	}
	return remoteUser.Tag().Id(), string(credBytes), nil
}

func (s *remoteLoginSuite) TestNoReauthHandler(c *gc.C) {
	info, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()
	st, err := api.Open(info, api.DialOpts{})
	c.Assert(err, gc.IsNil)
	// not logged in
	c.Check(st.AllFacadeVersions(), gc.HasLen, 0)
}

func (s *remoteLoginSuite) TestReauthHandler(c *gc.C) {
	info, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()
	info.Tag = nil
	st, err := api.Open(info, api.DialOpts{
		ReauthHandler: testReauthHandler{
			remoteLoginSuite:  s,
			thirdPartyChecker: newLoggedInChecker("bob"),
		},
	})
	c.Assert(err, gc.IsNil)

	// Should be logged in
	c.Assert(st.Ping(), gc.IsNil)
	c.Assert(st.AllFacadeVersions(), gc.Not(gc.HasLen), 0)
}

type failConditionChecker struct{}

// CheckThirdPartyCaveat implements the macaroon.ThirdPartyChecker interface.
func (*failConditionChecker) CheckThirdPartyCaveat(caveatId, condition string) ([]bakery.Caveat, error) {
	return nil, errors.Errorf("unrecognized condition")
}

func (s *remoteLoginSuite) TestBadReauthHandlers(c *gc.C) {
	testCases := []struct {
		setUp   func() testing.Restorer
		handler api.ReauthHandler
		pattern string
	}{{
		handler: testReauthHandler{
			remoteLoginSuite:  s,
			skipBind:          true,
			thirdPartyChecker: newLoggedInChecker("bob"),
		},
		pattern: "verification failed: signature mismatch after caveat verification",
	}, {
		setUp: func() testing.Restorer {
			return testing.PatchValue(&apiserver.MaxMacaroonTTL, -24*time.Hour)
		},
		handler: testReauthHandler{
			remoteLoginSuite:  s,
			thirdPartyChecker: newLoggedInChecker("bob"),
		},
		pattern: `verification failed: after expiry time`,
		/*
			}, {
				handler: testReauthHandler{
					remoteLoginSuite:  s,
					thirdPartyChecker: newLoggedInChecker("mallory"),
				},
				pattern: "cannot get discharge from \"remote-service-location\": invalid user",
		*/
	}, {
		handler: testReauthHandler{
			remoteLoginSuite:  s,
			thirdPartyChecker: &failConditionChecker{},
		},
		pattern: "cannot get discharge from \"remote-service-location\": unrecognized condition",
	}, {
		handler: emptyReauthHandler{},
		pattern: "not a valid user: \"\"",
	}, {
		handler: emptyCredReauthHandler{},
		pattern: "malformed remote credentials",
	}}
	for i, testCase := range testCases {
		func() {
			c.Log("test#", i)
			if testCase.setUp != nil {
				defer testCase.setUp().Restore()
			}
			info, cleanup := s.setupServerWithValidator(c, nil)
			defer cleanup()
			_, err := api.Open(info, api.DialOpts{
				ReauthHandler: testCase.handler,
			})
			c.Check(err, gc.ErrorMatches, testCase.pattern)
		}()
	}
}
