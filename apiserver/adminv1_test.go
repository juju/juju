// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/macaroon"
	"github.com/juju/macaroon/bakery"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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
	validUsers map[string]struct{}
}

func newLoggedInChecker(validUsers ...string) *loggedInChecker {
	c := &loggedInChecker{
		validUsers: make(map[string]struct{}),
	}
	for _, user := range validUsers {
		c.validUsers[user] = struct{}{}
	}
	return c
}

func (c *loggedInChecker) isValidUser(user string) bool {
	_, ok := c.validUsers[user]
	return ok
}

// CheckThirdPartyCaveat implements the macaroon.ThirdPartyChecker interface.
func (c *loggedInChecker) CheckThirdPartyCaveat(caveatId, condition string) ([]bakery.Caveat, error) {
	fields := strings.Split(condition, " ")
	if len(fields) < 1 {
		return nil, errors.Errorf("empty caveat")
	}
	if fields[0] == "is-authorized-user?" {
		if len(fields) < 2 {
			return nil, errors.Errorf("%s: missing user tag", fields[0])
		}
		tag, err := names.ParseTag(fields[1])
		if err != nil {
			return nil, errors.Errorf("%s: invalid user tag %q: %v", fields[0], fields[1], err)
		}
		if userTag, ok := tag.(names.UserTag); !ok {
			return nil, errors.Errorf("%s: %q is not a user tag", fields[0], fields[1])
		} else if c.isValidUser(userTag.Name()) {
			return nil, nil
		}
		return nil, errors.Errorf("invalid user")
	}
	return nil, errors.Errorf("unrecognized condition")
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

func (s *remoteLoginSuite) TestRemoteLoginReauth(c *gc.C) {
	info, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()
	st := s.openAPIWithoutLogin(c, info)

	// Try to log in as a remote identity.
	remoteUser := names.NewUserTag("bob")
	reauth, err := st.Login(remoteUser.String(), "", "")
	c.Assert(err, gc.IsNil)
	c.Assert(reauth, gc.NotNil)

	// No API facade versions. We're not logged in yet.
	c.Check(st.AllFacadeVersions(), gc.HasLen, 0)

	// As the remote client, decode the reauth request, obtain a discharge
	// macaroon from the identity-providing service, bind and serialize the
	// followup credential.
	var remoteCreds authentication.RemoteCredentials
	err = remoteCreds.UnmarshalText([]byte(reauth.Prompt))
	c.Assert(err, gc.IsNil)
	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	remoteCreds.Discharges, err = bakery.DischargeAll(remoteCreds.Primary,
		func(loc string, cav macaroon.Caveat) (*macaroon.Macaroon, error) {
			// The first-party location is the target Juju environment's tag.
			c.Assert(loc, gc.Equals, env.EnvironTag().String())
			return s.remoteIdService.Discharge(newLoggedInChecker("bob"), cav.Id)
		},
	)
	c.Assert(err, gc.IsNil)
	remoteCreds.Bind()
	credBytes, err := remoteCreds.MarshalText()
	c.Assert(err, gc.IsNil)

	// Retry the remote login request
	reauth, err = st.Login(remoteUser.String(), string(credBytes), reauth.Nonce)
	c.Assert(err, gc.IsNil)
	c.Assert(reauth, gc.IsNil)

	// Should be logged in
	c.Assert(st.Ping(), gc.IsNil)
	c.Assert(len(st.AllFacadeVersions()), jc.GreaterThan, 0)
}

type emptyCredReauthHandler struct{}

// HandleReauth simulates a bad reauth scenario in which an empty credential is returned,
// which triggers another reauth.
func (h emptyCredReauthHandler) HandleReauth(reauth *params.ReauthRequest) (string, string, error) {
	return "", reauth.Nonce, nil
}

type testReauthHandler struct {
	*remoteLoginSuite
	*gc.C
	skipBind          bool
	thirdPartyChecker bakery.ThirdPartyChecker
}

func failReauth(err error) (string, string, error) {
	return "", "", errors.Trace(err)
}

// HandleReauth implements a fully-functional reauthentication handler capable
// of discharging the third-party caveat challenge issued by Juju. It also
// contains logic to force failure modes for testing.
func (h testReauthHandler) HandleReauth(reauth *params.ReauthRequest) (string, string, error) {
	var remoteCreds authentication.RemoteCredentials
	err := remoteCreds.UnmarshalText([]byte(reauth.Prompt))
	if err != nil {
		return failReauth(err)
	}
	remoteCreds.Discharges, err = bakery.DischargeAll(remoteCreds.Primary,
		func(loc string, cav macaroon.Caveat) (*macaroon.Macaroon, error) {
			return h.remoteIdService.Discharge(h.thirdPartyChecker, cav.Id)
		},
	)
	if err != nil {
		return failReauth(err)
	}
	if !h.skipBind {
		remoteCreds.Bind()
	}
	credBytes, err := remoteCreds.MarshalText()
	if err != nil {
		return failReauth(err)
	}
	return string(credBytes), reauth.Nonce, nil

}

func (s *remoteLoginSuite) TestReauthHandler(c *gc.C) {
	info, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()
	info.Tag = names.NewUserTag("bob")
	st, err := api.Open(info, api.DialOpts{
		ReauthHandler: testReauthHandler{
			remoteLoginSuite:  s,
			C:                 c,
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
		setup   func() func()
		handler api.ReauthHandler
		pattern string
	}{
		{
			handler: testReauthHandler{
				remoteLoginSuite:  s,
				C:                 c,
				skipBind:          true,
				thirdPartyChecker: newLoggedInChecker("bob"),
			},
			pattern: "verification failed: signature mismatch after caveat verification",
		},
		{
			setup: func() func() {
				originalTtl := apiserver.MaxMacaroonTtl
				apiserver.MaxMacaroonTtl = -24 * time.Hour
				return func() {
					apiserver.MaxMacaroonTtl = originalTtl
				}
			},
			handler: testReauthHandler{
				remoteLoginSuite:  s,
				C:                 c,
				thirdPartyChecker: newLoggedInChecker("bob"),
			},
			pattern: `verification failed: is-before-time\?: authorization expired at .*`,
		},
		{
			handler: testReauthHandler{
				remoteLoginSuite:  s,
				C:                 c,
				thirdPartyChecker: newLoggedInChecker(),
			},
			pattern: "cannot get discharge from \"remote-service-location\": invalid user",
		},
		{
			handler: testReauthHandler{
				remoteLoginSuite:  s,
				C:                 c,
				thirdPartyChecker: &failConditionChecker{},
			},
			pattern: "cannot get discharge from \"remote-service-location\": unrecognized condition",
		},
		{
			handler: emptyCredReauthHandler{},
			pattern: "reauthentication failed",
		},
	}
	for i, testCase := range testCases {
		func() {
			c.Log("test#", i)
			if testCase.setup != nil {
				cleanup := testCase.setup()
				defer cleanup()
			}
			info, cleanup := s.setupServerWithValidator(c, nil)
			defer cleanup()
			info.Tag = names.NewUserTag("bob")
			_, err := api.Open(info, api.DialOpts{
				ReauthHandler: testCase.handler,
			})
			c.Check(err, gc.ErrorMatches, testCase.pattern)
		}()
	}
}
