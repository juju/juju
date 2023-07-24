// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/modelmanager"
	"github.com/juju/juju/api/client/usermanager"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/network"
	jujutesting "github.com/juju/juju/juju/testing"
	proxytest "github.com/juju/juju/proxy/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type stateSuite struct {
	jujutesting.ApiServerSuite
}

var _ = gc.Suite(&stateSuite{})

type slideSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&slideSuite{})

func (s *stateSuite) openAPI(c *gc.C) api.Connection {
	apiInfo := s.ControllerModelApiInfo()
	apiInfo.Tag = jujutesting.AdminUser
	apiInfo.Password = jujutesting.AdminSecret
	conn, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, gc.IsNil)
	return conn
}

func (s *stateSuite) TestCloseMultipleOk(c *gc.C) {
	conn := s.openAPI(c)
	c.Assert(conn.Close(), gc.IsNil)
	c.Assert(conn.Close(), gc.IsNil)
	c.Assert(conn.Close(), gc.IsNil)
}

// openAPIWithoutLogin connects to the API and returns an api.State without
// actually calling st.Login already.
func (s *stateSuite) openAPIWithoutLogin(c *gc.C) api.Connection {
	info := s.ControllerModelApiInfo()
	info.Tag = nil
	info.Password = ""
	info.Macaroons = nil
	info.SkipLogin = true
	conn, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	return conn
}

func (s *stateSuite) TestAPIHostPortsAlwaysIncludesTheConnection(c *gc.C) {
	conn := s.openAPI(c)
	hostportslist := conn.APIHostPorts()
	c.Check(hostportslist, gc.HasLen, 1)
	serverhostports := hostportslist[0]
	c.Check(serverhostports, gc.HasLen, 1)

	// We intentionally set this to invalid values
	badServers := network.NewSpaceHostPorts(1234, "0.1.2.3")
	badServers[0].Scope = network.ScopeMachineLocal
	err := s.ControllerModel(c).State().SetAPIHostPorts([]network.SpaceHostPorts{badServers}, controller.Config{})
	c.Assert(err, jc.ErrorIsNil)

	conn2 := s.openAPI(c)
	hp, err := network.ParseMachineHostPort(badServers[0].String())
	c.Assert(err, jc.ErrorIsNil)
	hp.Scope = badServers[0].Scope

	hostports := conn2.APIHostPorts()
	c.Check(hostports, gc.DeepEquals, []network.MachineHostPorts{
		serverhostports,
		{*hp},
	})
}

func (s *stateSuite) TestAPIHostPortsDoesNotIncludeConnectionProxy(c *gc.C) {
	conn := newRPCConnection()
	conn.response = &params.LoginResult{
		ControllerTag: coretesting.ControllerTag.String(),
		ServerVersion: "2.3-rc2",
		Servers: [][]params.HostPort{
			{
				params.HostPort{
					Address: params.Address{
						Value: "fe80:abcd::1",
						CIDR:  "128",
					},
					Port: 1234,
				},
			},
		},
	}

	broken := make(chan struct{})
	close(broken)
	testState := api.NewTestingState(api.TestingStateParams{
		RPCConnection: conn,
		Clock:         &fakeClock{},
		Address:       "localhost:1234",
		Broken:        broken,
		Closed:        make(chan struct{}),
		Proxier:       proxytest.NewMockTunnelProxier(),
	})
	err := testState.Login(names.NewUserTag("admin"), jujutesting.AdminSecret, "", nil)
	c.Assert(err, jc.ErrorIsNil)

	hostPortList := testState.APIHostPorts()
	c.Assert(len(hostPortList), gc.Equals, 1)
	c.Assert(len(hostPortList[0]), gc.Equals, 1)
	c.Assert(hostPortList[0][0].NetPort, gc.Equals, network.NetPort(1234))
	c.Assert(hostPortList[0][0].MachineAddress.Value, gc.Equals, "fe80:abcd::1")
}

func (s *stateSuite) TestTags(c *gc.C) {
	conn := s.openAPIWithoutLogin(c)
	defer conn.Close()
	// Even though we haven't called Login, the model tag should
	// still be set.
	modelTag, ok := conn.ModelTag()
	c.Check(ok, jc.IsTrue)
	model := s.ControllerModel(c)
	c.Check(modelTag, gc.Equals, model.ModelTag())
	err := conn.Login(jujutesting.AdminUser, jujutesting.AdminSecret, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	// Now that we've logged in, ModelTag should still be the same.
	modelTag, ok = conn.ModelTag()
	c.Check(ok, jc.IsTrue)
	c.Check(modelTag, gc.Equals, model.ModelTag())
	controllerTag := conn.ControllerTag()
	c.Check(controllerTag, gc.Equals, coretesting.ControllerTag)
}

func (s *stateSuite) TestLoginSetsControllerAccess(c *gc.C) {
	// The default user has admin access.
	conn := s.OpenControllerModelAPI(c)
	c.Assert(conn.ControllerAccess(), gc.Equals, "superuser")

	manager := usermanager.NewClient(s.OpenControllerAPI(c))
	defer manager.Close()
	usertag, _, err := manager.AddUser("ro", "ro", "ro-password")
	c.Assert(err, jc.ErrorIsNil)
	mmanager := modelmanager.NewClient(s.OpenControllerAPI(c))
	defer mmanager.Close()
	modeltag, ok := conn.ModelTag()
	c.Assert(ok, jc.IsTrue)
	err = mmanager.GrantModel(usertag.Id(), "read", modeltag.Id())
	c.Assert(err, jc.ErrorIsNil)
	conn = s.OpenControllerAPIAs(c, usertag, "ro-password")
	c.Assert(conn.ControllerAccess(), gc.Equals, "login")
}

func (s *stateSuite) TestLoginToMigratedModel(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	modelOwner := f.MakeUser(c, &factory.UserParams{
		Password: "secret",
	})
	modelState := f.MakeModel(c, &factory.ModelParams{
		Owner: modelOwner.UserTag(),
	})
	defer modelState.Close()
	model, err := modelState.Model()
	c.Assert(err, jc.ErrorIsNil)

	controllerTag := names.NewControllerTag(utils.MustNewUUID().String())

	// Migrate the model and delete it from the state
	mig, err := modelState.CreateMigration(state.MigrationSpec{
		InitiatedBy: names.NewUserTag("admin"),
		TargetInfo: migration.TargetInfo{
			ControllerTag: controllerTag,
			Addrs:         []string{"1.2.3.4:5555"},
			CACert:        coretesting.CACert,
			AuthTag:       names.NewUserTag("user2"),
			Password:      "secret",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	for _, phase := range migration.SuccessfulMigrationPhases() {
		c.Assert(mig.SetPhase(phase), jc.ErrorIsNil)
	}
	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(modelState.RemoveDyingModel(), jc.ErrorIsNil)

	// Attempt to open an API connection to the migrated model as a user
	// that had access to the model before it got migrated.
	info := s.ModelApiInfo(model.UUID())
	info.Tag = modelOwner.Tag()
	info.Password = "secret"
	_, err = api.Open(info, api.DialOpts{})

	redirErr, ok := errors.Cause(err).(*api.RedirectError)
	c.Assert(ok, gc.Equals, true)

	nhp := network.NewMachineHostPorts(5555, "1.2.3.4")
	c.Assert(redirErr.Servers, jc.DeepEquals, []network.MachineHostPorts{nhp})
	c.Assert(redirErr.CACert, gc.Equals, coretesting.CACert)
	c.Assert(redirErr.FollowRedirect, gc.Equals, false)
	c.Assert(redirErr.ControllerTag, gc.Equals, controllerTag)
}

func (s *stateSuite) TestLoginMacaroonInvalidId(c *gc.C) {
	conn := s.openAPIWithoutLogin(c)
	defer conn.Close()
	mac, err := macaroon.New([]byte("root-key"), []byte("id"), "juju", macaroon.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)
	err = conn.Login(jujutesting.AdminUser, "", "", []macaroon.Slice{{mac}})
	c.Assert(err, gc.ErrorMatches, "interaction required but not possible")
}

func (s *stateSuite) TestBestFacadeVersion(c *gc.C) {
	conn := s.OpenControllerModelAPI(c)
	c.Check(conn.BestFacadeVersion("Client"), gc.Equals, 6)
}

func (s *stateSuite) TestAPIHostPortsMovesConnectedValueFirst(c *gc.C) {
	conn := s.OpenControllerAPI(c)
	hostPortsList := conn.APIHostPorts()
	c.Check(hostPortsList, gc.HasLen, 1)
	serverHostPorts := hostPortsList[0]
	c.Check(serverHostPorts, gc.HasLen, 1)
	goodAddress := serverHostPorts[0]

	// We intentionally set this to invalid values
	badValue := network.MachineHostPort{
		MachineAddress: network.NewMachineAddress("0.1.2.3", network.WithScope(network.ScopeMachineLocal)),
		NetPort:        1234,
	}
	badServer := []network.MachineHostPort{badValue}

	extraAddress := network.MachineHostPort{
		MachineAddress: network.NewMachineAddress("0.1.2.4", network.WithScope(network.ScopeMachineLocal)),
		NetPort:        5678,
	}
	extraAddress2 := network.MachineHostPort{
		MachineAddress: network.NewMachineAddress("0.1.2.1", network.WithScope(network.ScopeMachineLocal)),
		NetPort:        9012,
	}

	current := []network.SpaceHostPorts{
		{
			network.SpaceHostPort{
				SpaceAddress: network.SpaceAddress{MachineAddress: badValue.MachineAddress},
				NetPort:      badValue.NetPort,
			},
		},
		{
			network.SpaceHostPort{
				SpaceAddress: network.SpaceAddress{MachineAddress: extraAddress.MachineAddress},
				NetPort:      extraAddress.NetPort,
			},
			network.SpaceHostPort{
				SpaceAddress: network.SpaceAddress{MachineAddress: goodAddress.MachineAddress},
				NetPort:      goodAddress.NetPort,
			},
			network.SpaceHostPort{
				SpaceAddress: network.SpaceAddress{MachineAddress: extraAddress2.MachineAddress},
				NetPort:      extraAddress2.NetPort,
			},
		},
	}

	st := s.ControllerModel(c).State()
	controllerConfig, err := st.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)

	err = st.SetAPIHostPorts(current, controllerConfig)
	c.Assert(err, jc.ErrorIsNil)

	conn2 := s.OpenControllerAPI(c)
	hostPorts := conn2.APIHostPorts()
	// We should have rotate the server we connected to as the first item,
	// and the address of that server as the first address
	sortedServer := []network.MachineHostPort{
		goodAddress, extraAddress, extraAddress2,
	}
	expected := []network.MachineHostPorts{sortedServer, badServer}
	c.Check(hostPorts, gc.DeepEquals, expected)
}

var exampleHostPorts = []network.MachineHostPort{
	{MachineAddress: network.NewMachineAddress("0.1.2.3"), NetPort: 1234},
	{MachineAddress: network.NewMachineAddress("0.1.2.4"), NetPort: 5678},
	{MachineAddress: network.NewMachineAddress("0.1.2.1"), NetPort: 9012},
	{MachineAddress: network.NewMachineAddress("0.1.9.1"), NetPort: 8888},
}

func (s *slideSuite) TestSlideToFrontNoOp(c *gc.C) {
	servers := []network.MachineHostPorts{
		{exampleHostPorts[0]},
		{exampleHostPorts[1]},
	}
	// order should not have changed
	expected := []network.MachineHostPorts{
		{exampleHostPorts[0]},
		{exampleHostPorts[1]},
	}
	api.SlideAddressToFront(servers, 0, 0)
	c.Check(servers, gc.DeepEquals, expected)
}

func (s *slideSuite) TestSlideToFrontAddress(c *gc.C) {
	servers := []network.MachineHostPorts{
		{exampleHostPorts[0], exampleHostPorts[1], exampleHostPorts[2]},
		{exampleHostPorts[3]},
	}
	// server order should not change, but ports should be switched
	expected := []network.MachineHostPorts{
		{exampleHostPorts[1], exampleHostPorts[0], exampleHostPorts[2]},
		{exampleHostPorts[3]},
	}
	api.SlideAddressToFront(servers, 0, 1)
	c.Check(servers, gc.DeepEquals, expected)
}

func (s *slideSuite) TestSlideToFrontServer(c *gc.C) {
	servers := []network.MachineHostPorts{
		{exampleHostPorts[0], exampleHostPorts[1]},
		{exampleHostPorts[2]},
		{exampleHostPorts[3]},
	}
	// server 1 should be slid to the front
	expected := []network.MachineHostPorts{
		{exampleHostPorts[2]},
		{exampleHostPorts[0], exampleHostPorts[1]},
		{exampleHostPorts[3]},
	}
	api.SlideAddressToFront(servers, 1, 0)
	c.Check(servers, gc.DeepEquals, expected)
}

func (s *slideSuite) TestSlideToFrontBoth(c *gc.C) {
	servers := []network.MachineHostPorts{
		{exampleHostPorts[0]},
		{exampleHostPorts[1], exampleHostPorts[2]},
		{exampleHostPorts[3]},
	}
	// server 1 should be slid to the front
	expected := []network.MachineHostPorts{
		{exampleHostPorts[2], exampleHostPorts[1]},
		{exampleHostPorts[0]},
		{exampleHostPorts[3]},
	}
	api.SlideAddressToFront(servers, 1, 1)
	c.Check(servers, gc.DeepEquals, expected)
}
