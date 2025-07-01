// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/domain/access"
	"github.com/juju/juju/domain/controllernode"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

func TestMacaroonLoginSuite(t *testing.T) {
	tc.Run(t, &macaroonLoginSuite{})
}

type macaroonLoginSuite struct {
	remoteUser user.Name
	jujutesting.MacaroonSuite
}

func (s *macaroonLoginSuite) SetUpTest(c *tc.C) {
	s.remoteUser = usertesting.GenNewName(c, "testuser@somewhere")
	s.MacaroonSuite.SetUpTest(c)
}

func (s *macaroonLoginSuite) TestPublicKeyLocatorErrorIsNotPersistent(c *tc.C) {
	s.AddModelUser(c, s.remoteUser)
	s.AddControllerUser(c, s.remoteUser, permission.LoginAccess)
	s.DischargerLogin = func() string {
		return s.remoteUser.Name()
	}
	workingTransport := http.DefaultTransport
	failingTransport := errorTransport{
		fallback: workingTransport,
		location: s.DischargerLocation(),
		err:      errors.New("some error"),
	}
	s.PatchValue(&http.DefaultTransport, failingTransport)
	info := s.ControllerModelApiInfo()
	_, err := s.login(c, info)
	c.Assert(err, tc.ErrorMatches, `.*: some error .*`)

	http.DefaultTransport = workingTransport

	// The error doesn't stick around.
	_, err = s.login(c, info)
	c.Assert(err, tc.ErrorIsNil)

	// Once we've succeeded, we shouldn't try again.
	http.DefaultTransport = failingTransport

	_, err = s.login(c, info)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *macaroonLoginSuite) setAPIAddresses(c *tc.C, info *api.Info) {
	controllerNodeService := s.ControllerDomainServices(c).ControllerNode()
	addrs := make(network.SpaceHostPorts, len(info.Addrs))
	for i, addr := range info.Addrs {
		parts := strings.Split(addr, ":")
		port, _ := strconv.Atoi(parts[1])
		addrs[i] = network.SpaceHostPort{
			SpaceAddress: network.SpaceAddress{
				MachineAddress: network.MachineAddress{
					Value: parts[0],
				},
			},
			NetPort: network.NetPort(port),
		}
	}
	c.Logf("heather %+v", addrs)
	err := controllerNodeService.SetAPIAddresses(c.Context(), controllernode.SetAPIAddressArgs{
		APIAddresses: map[string]network.SpaceHostPorts{
			"0": addrs,
		},
	})
	c.Assert(err, tc.IsNil)
}

func (s *macaroonLoginSuite) login(c *tc.C, info *api.Info) (params.LoginResult, error) {
	cookieJar := jujutesting.NewClearableCookieJar()
	s.setAPIAddresses(c, info)

	infoSkipLogin := *info
	infoSkipLogin.SkipLogin = true
	infoSkipLogin.Macaroons = nil
	client := s.OpenAPI(c, &infoSkipLogin, cookieJar)
	defer client.Close()

	var (
		request params.LoginRequest
		result  params.LoginResult
	)
	err := client.APICall(c.Context(), "Admin", 3, "", "Login", &request, &result)
	if err != nil {
		return params.LoginResult{}, errors.Annotatef(err, "cannot log in")
	}

	cookieURL := &url.URL{
		Scheme: "https",
		Host:   "localhost",
		Path:   "/",
	}

	bakeryClient := httpbakery.NewClient()

	mac := result.BakeryDischargeRequired
	if mac == nil {
		var err error
		mac, err = bakery.NewLegacyMacaroon(result.DischargeRequired)
		c.Assert(err, tc.ErrorIsNil)
	}
	err = bakeryClient.HandleError(c.Context(), cookieURL, &httpbakery.Error{
		Message: result.DischargeRequiredReason,
		Code:    httpbakery.ErrDischargeRequired,
		Info: &httpbakery.ErrorInfo{
			Macaroon:     mac,
			MacaroonPath: "/",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	// Add the macaroons that have been saved by HandleError to our login request.
	request.Macaroons = httpbakery.MacaroonsForURL(bakeryClient.Client.Jar, cookieURL)

	err = client.APICall(c.Context(), "Admin", 3, "", "Login", &request, &result)
	return result, err
}

func (s *macaroonLoginSuite) TestRemoteUserLoginToControllerNoAccess(c *tc.C) {
	s.DischargerLogin = func() string {
		return s.remoteUser.Name()
	}
	info := s.APIInfo(c)
	// Log in to the controller, not the model.
	info.ModelTag = names.ModelTag{}

	_, err := s.login(c, info)
	assertPermissionDenied(c, err)
}

func (s *macaroonLoginSuite) TestRemoteUserLoginToControllerLoginAccess(c *tc.C) {
	s.AddControllerUser(c, permission.EveryoneUserName, permission.LoginAccess)

	s.DischargerLogin = func() string {
		return s.remoteUser.Name()
	}
	info := s.APIInfo(c)
	// Log in to the controller, not the model.
	info.ModelTag = names.ModelTag{}

	result, err := s.login(c, info)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(result.UserInfo, tc.NotNil)
	c.Check(result.UserInfo.Identity, tc.Equals, names.NewUserTag(s.remoteUser.Name()).String())
	c.Check(result.UserInfo.ControllerAccess, tc.Equals, "login")
	c.Check(result.UserInfo.ModelAccess, tc.Equals, "")
	c.Check(result.Servers, tc.DeepEquals, params.FromProviderHostsPorts(parseHostPortsFromAddress(c, info.Addrs...)))
}

func parseHostPortsFromAddress(c *tc.C, addresses ...string) []network.ProviderHostPorts {
	hps := make([]network.ProviderHostPorts, len(addresses))
	for i, add := range addresses {
		hp, err := network.ParseProviderHostPorts(add)
		c.Assert(err, tc.ErrorIsNil)
		hps[i] = hp
	}
	return hps
}

func (s *macaroonLoginSuite) TestRemoteUserLoginToControllerSuperuserAccess(c *tc.C) {
	s.AddControllerUser(c, permission.EveryoneUserName, permission.SuperuserAccess)
	var remoteUserTag = names.NewUserTag(s.remoteUser.Name())

	s.DischargerLogin = func() string {
		return s.remoteUser.Name()
	}
	info := s.APIInfo(c)
	// Log in to the controller, not the model.
	info.ModelTag = names.ModelTag{}

	result, err := s.login(c, info)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(result.UserInfo, tc.NotNil)
	c.Check(result.UserInfo.Identity, tc.Equals, remoteUserTag.String())
	c.Check(result.UserInfo.ControllerAccess, tc.Equals, "superuser")
	c.Check(result.UserInfo.ModelAccess, tc.Equals, "")
}

func (s *macaroonLoginSuite) TestRemoteUserLoginToModelNoExplicitAccess(c *tc.C) {
	// If we have a remote user which the controller knows nothing about,
	// and the macaroon is discharged successfully, and the user is attempting
	// to log into a model, that is permission denied.
	s.AddControllerUser(c, permission.EveryoneUserName, permission.LoginAccess)
	s.DischargerLogin = func() string {
		return s.remoteUser.Name()
	}
	info := s.APIInfo(c)

	_, err := s.login(c, info)
	assertPermissionDenied(c, err)
}

func (s *macaroonLoginSuite) TestRemoteUserLoginToModelWithExplicitAccess(c *tc.C) {
	s.testRemoteUserLoginToModelWithExplicitAccess(c, false)
}

func (s *macaroonLoginSuite) TestRemoteUserLoginToModelWithExplicitAccessAndAllowModelAccess(c *tc.C) {
	s.testRemoteUserLoginToModelWithExplicitAccess(c, true)
}

func (s *macaroonLoginSuite) testRemoteUserLoginToModelWithExplicitAccess(c *tc.C, allowModelAccess bool) {
	apiserver.SetAllowModelAccess(s.Server, allowModelAccess)

	accessService := s.ControllerDomainServices(c).Access()
	err := accessService.UpdatePermission(c.Context(), access.UpdatePermissionArgs{
		Subject: s.remoteUser,
		Change:  permission.Grant,
		AccessSpec: permission.AccessSpec{
			Target: permission.ID{
				ObjectType: permission.Model,
				Key:        s.ControllerModelUUID(),
			},
			Access: permission.WriteAccess,
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	s.DischargerLogin = func() string {
		return s.remoteUser.Name()
	}

	_, err = s.login(c, s.ControllerModelApiInfo())
	if allowModelAccess {
		c.Assert(err, tc.ErrorIsNil)
	} else {
		assertPermissionDenied(c, err)
	}
}

func (s *macaroonLoginSuite) TestRemoteUserLoginToModelWithControllerAccess(c *tc.C) {
	s.AddModelUser(c, s.remoteUser)
	s.AddControllerUser(c, s.remoteUser, permission.SuperuserAccess)

	s.DischargerLogin = func() string {
		return s.remoteUser.Name()
	}
	info := s.APIInfo(c)

	result, err := s.login(c, info)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.UserInfo, tc.NotNil)
	c.Check(result.UserInfo.Identity, tc.Equals, names.NewUserTag(s.remoteUser.Name()).String())
	c.Check(result.UserInfo.ControllerAccess, tc.Equals, "superuser")
	c.Check(result.UserInfo.ModelAccess, tc.Equals, "write")
}

func (s *macaroonLoginSuite) TestLoginToModelSuccess(c *tc.C) {
	s.AddModelUser(c, s.remoteUser)
	s.AddControllerUser(c, s.remoteUser, permission.LoginAccess)
	s.DischargerLogin = func() string {
		return s.remoteUser.Name()
	}
	s.setAPIAddresses(c, s.APIInfo(c))
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)
	client, err := api.Open(c.Context(), s.APIInfo(c), api.DialOpts{})
	c.Assert(err, tc.ErrorIsNil)
	defer client.Close()

	// The auth tag has been correctly returned by the server.
	c.Assert(client.AuthTag(), tc.Equals, names.NewUserTag(s.remoteUser.Name()))
}

func (s *macaroonLoginSuite) TestFailedToObtainDischargeLogin(c *tc.C) {
	s.DischargerLogin = func() string {
		return ""
	}
	client, err := api.Open(c.Context(), s.APIInfo(c), api.DialOpts{})
	c.Assert(err, tc.ErrorMatches, `cannot get discharge from "https://.*": third party refused discharge: cannot discharge: login denied by discharger`)
	c.Assert(client, tc.Equals, nil)
}

func (s *macaroonLoginSuite) TestConnectStream(c *tc.C) {
	s.AddModelUser(c, s.remoteUser)
	s.AddControllerUser(c, s.remoteUser, permission.LoginAccess)
	s.setAPIAddresses(c, s.APIInfo(c))

	catcher := api.UrlCatcher{}
	s.PatchValue(&api.WebsocketDial, catcher.RecordLocation)

	dischargeCount := 0
	s.DischargerLogin = func() string {
		dischargeCount++
		return s.remoteUser.Name()
	}

	// First log into the regular API.
	client, err := api.Open(c.Context(), s.APIInfo(c), api.DialOpts{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dischargeCount, tc.Equals, 1)

	// Then check that ConnectStream works OK and that it doesn't need
	// to discharge again.
	conn, err := client.ConnectStream(c.Context(), "/path", nil)
	c.Assert(err, tc.IsNil)
	defer conn.Close()

	connectURL, err := url.Parse(catcher.Location())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(connectURL.Path, tc.Equals, "/model/"+s.ControllerModelUUID()+"/path")
	c.Check(dischargeCount, tc.Equals, 1)
}

func (s *macaroonLoginSuite) TestConnectStreamFailedDischarge(c *tc.C) {
	s.AddModelUser(c, s.remoteUser)
	s.AddControllerUser(c, s.remoteUser, permission.LoginAccess)
	s.setAPIAddresses(c, s.APIInfo(c))

	// This is really a test for ConnectStream, but to test ConnectStream's
	// discharge failing logic, we need an actual endpoint to test against,
	// and the debug-log endpoint makes a convenient example.

	var dischargeError bool
	s.DischargerLogin = func() string {
		if dischargeError {
			return ""
		}
		return s.remoteUser.Name()
	}

	// Make an API connection that uses a cookie jar
	// that allows us to remove all cookies.
	jar := jujutesting.NewClearableCookieJar()
	client := s.OpenAPI(c, nil, jar)

	// Ensure that the discharger won't discharge and try
	// logging in again. We should succeed in getting past
	// authorization because we have the cookies (but
	// the actual debug-log endpoint will return an error).
	dischargeError = true
	logArgs := url.Values{"noTail": []string{"true"}}
	conn, err := client.ConnectStream(c.Context(), "/log", logArgs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(conn, tc.NotNil)
	conn.Close()

	// Then delete all the cookies by deleting the cookie jar
	// and try again. The login should fail.
	jar.Clear()

	conn, err = client.ConnectStream(c.Context(), "/log", logArgs)
	c.Assert(err, tc.ErrorMatches, `cannot get discharge from "https://.*": third party refused discharge: cannot discharge: login denied by discharger`)
	c.Assert(conn, tc.IsNil)
}

func (s *macaroonLoginSuite) TestConnectStreamWithDischargedMacaroons(c *tc.C) {
	s.AddModelUser(c, s.remoteUser)
	s.AddControllerUser(c, s.remoteUser, permission.LoginAccess)
	s.setAPIAddresses(c, s.APIInfo(c))

	// If the connection was created with already-discharged macaroons
	// (rather than acquiring them through the discharge dance), they
	// wouldn't get attached to the websocket request.
	// https://bugs.launchpad.net/juju/+bug/1650451
	catcher := api.UrlCatcher{}
	s.PatchValue(&api.WebsocketDial, catcher.RecordLocation)

	mac, err := macaroon.New([]byte("abc-123"), []byte("aurora gone"), "shankil butchers", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)

	s.DischargerLogin = func() string {
		return s.remoteUser.Name()
	}

	info := s.APIInfo(c)
	info.Macaroons = []macaroon.Slice{{mac}}
	client := s.OpenAPI(c, info, nil)

	host := api.PreferredHost(info)
	if host == "" {
		host = info.Addrs[0]
	}

	bClient, ok := client.BakeryClient().(*httpbakery.Client)
	c.Assert(ok, tc.IsTrue)
	dischargedMacaroons := httpbakery.MacaroonsForURL(bClient.Jar, api.CookieURLFromHost(host))
	c.Assert(len(dischargedMacaroons), tc.Equals, 1)

	// Mirror the situation in migration logtransfer - the macaroon is
	// now stored in the auth service (so no further discharge is
	// needed), but we use a different client to connect to the log
	// stream, so the macaroon isn't in the cookie jar despite being
	// in the connection info.

	// Then check that ConnectStream works OK and that it doesn't need
	// to discharge again.
	s.DischargerLogin = nil

	info2 := s.APIInfo(c)
	info2.Macaroons = dischargedMacaroons

	client2 := s.OpenAPI(c, info2, nil)
	conn, err := client2.ConnectStream(c.Context(), "/path", nil)
	c.Assert(err, tc.IsNil)
	defer conn.Close()

	headers := catcher.Headers()
	c.Assert(headers.Get(httpbakery.BakeryProtocolHeader), tc.Equals, "3")
	c.Assert(headers.Get("Cookie"), tc.HasPrefix, "macaroon-")
	assertHeaderMatchesMacaroon(c, headers, dischargedMacaroons[0])
}

func assertHeaderMatchesMacaroon(c *tc.C, header http.Header, macaroon macaroon.Slice) {
	req := http.Request{Header: header}
	actualCookie := req.Cookies()[0]
	expectedCookie, err := httpbakery.NewCookie(nil, macaroon)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(actualCookie.Name, tc.Equals, expectedCookie.Name)
	c.Assert(actualCookie.Value, tc.Equals, expectedCookie.Value)
}
