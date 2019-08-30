// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"
	"gopkg.in/macaroon.v2-unstable"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/api/usermanager"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/network"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type stateSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&stateSuite{})

type slideSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&slideSuite{})

func (s *stateSuite) TestCloseMultipleOk(c *gc.C) {
	c.Assert(s.APIState.Close(), gc.IsNil)
	c.Assert(s.APIState.Close(), gc.IsNil)
	c.Assert(s.APIState.Close(), gc.IsNil)
}

// OpenAPIWithoutLogin connects to the API and returns an api.State without
// actually calling st.Login already. The returned strings are the "tag" and
// "password" that we would have used to login.
func (s *stateSuite) OpenAPIWithoutLogin(c *gc.C) (api.Connection, names.Tag, string) {
	info := s.APIInfo(c)
	tag := info.Tag
	password := info.Password
	info.Tag = nil
	info.Password = ""
	info.Macaroons = nil
	info.SkipLogin = true
	apistate, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	return apistate, tag, password
}

func (s *stateSuite) TestAPIHostPortsAlwaysIncludesTheConnection(c *gc.C) {
	hostportslist := s.APIState.APIHostPorts()
	c.Check(hostportslist, gc.HasLen, 1)
	serverhostports := hostportslist[0]
	c.Check(serverhostports, gc.HasLen, 1)
	// the other addresses, but always see this one as well.
	info := s.APIInfo(c)
	// We intentionally set this to invalid values
	badServer := network.NewHostPorts(1234, "0.1.2.3")
	badServer[0].Scope = network.ScopeMachineLocal
	s.State.SetAPIHostPorts([][]network.HostPort{badServer})
	apistate, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer apistate.Close()
	hostports := apistate.APIHostPorts()
	c.Check(hostports, gc.DeepEquals, [][]network.HostPort{
		serverhostports,
		badServer,
	})
}

func (s *stateSuite) TestTags(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	apistate, tag, password := s.OpenAPIWithoutLogin(c)
	defer apistate.Close()
	// Even though we haven't called Login, the model tag should
	// still be set.
	modelTag, ok := apistate.ModelTag()
	c.Check(ok, jc.IsTrue)
	c.Check(modelTag, gc.Equals, model.ModelTag())
	err = apistate.Login(tag, password, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	// Now that we've logged in, ModelTag should still be the same.
	modelTag, ok = apistate.ModelTag()
	c.Check(ok, jc.IsTrue)
	c.Check(modelTag, gc.Equals, model.ModelTag())
	controllerTag := apistate.ControllerTag()
	c.Check(controllerTag, gc.Equals, coretesting.ControllerTag)
}

func (s *stateSuite) TestLoginSetsModelAccess(c *gc.C) {
	// The default user has admin access.
	c.Assert(s.APIState.ModelAccess(), gc.Equals, "admin")

	manager := usermanager.NewClient(s.OpenControllerAPI(c))
	defer manager.Close()
	usertag, _, err := manager.AddUser("ro", "ro", "ro-password")
	c.Assert(err, jc.ErrorIsNil)
	mmanager := modelmanager.NewClient(s.OpenControllerAPI(c))
	defer mmanager.Close()
	modeltag, ok := s.APIState.ModelTag()
	c.Assert(ok, jc.IsTrue)
	err = mmanager.GrantModel(usertag.Id(), "read", modeltag.Id())
	c.Assert(err, jc.ErrorIsNil)
	conn := s.OpenAPIAs(c, usertag, "ro-password")
	c.Assert(conn.ModelAccess(), gc.Equals, "read")
}

func (s *stateSuite) TestLoginSetsControllerAccess(c *gc.C) {
	// The default user has admin access.
	c.Assert(s.APIState.ControllerAccess(), gc.Equals, "superuser")

	manager := usermanager.NewClient(s.OpenControllerAPI(c))
	defer manager.Close()
	usertag, _, err := manager.AddUser("ro", "ro", "ro-password")
	c.Assert(err, jc.ErrorIsNil)
	mmanager := modelmanager.NewClient(s.OpenControllerAPI(c))
	defer mmanager.Close()
	modeltag, ok := s.APIState.ModelTag()
	c.Assert(ok, jc.IsTrue)
	err = mmanager.GrantModel(usertag.Id(), "read", modeltag.Id())
	c.Assert(err, jc.ErrorIsNil)
	conn := s.OpenAPIAs(c, usertag, "ro-password")
	c.Assert(conn.ControllerAccess(), gc.Equals, "login")
}

func (s *stateSuite) TestLoginToMigratedModel(c *gc.C) {
	modelOwner := s.Factory.MakeUser(c, &factory.UserParams{
		Password: "secret",
	})
	modelState := s.Factory.MakeModel(c, &factory.ModelParams{
		Owner: modelOwner.UserTag(),
	})
	defer modelState.Close()
	model, err := modelState.Model()
	c.Assert(err, jc.ErrorIsNil)

	// Migrate the model and delete it from the state
	mig, err := modelState.CreateMigration(state.MigrationSpec{
		InitiatedBy: names.NewUserTag("admin"),
		TargetInfo: migration.TargetInfo{
			ControllerTag: names.NewControllerTag(utils.MustNewUUID().String()),
			Addrs:         []string{"1.2.3.4:5555"},
			CACert:        coretesting.CACert,
			AuthTag:       names.NewUserTag("user2"),
			Password:      "secret",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	phases := []migration.Phase{migration.IMPORT, migration.VALIDATION, migration.SUCCESS, migration.LOGTRANSFER, migration.REAP, migration.DONE}
	for _, phase := range phases {
		c.Assert(mig.SetPhase(phase), jc.ErrorIsNil)
	}
	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(modelState.RemoveDyingModel(), jc.ErrorIsNil)

	// Attempt to open an API connection to the migrated model as a user
	// that had access to the model before it got migrated.
	info := s.APIInfo(c)
	info.ModelTag = model.ModelTag()
	info.Tag = modelOwner.Tag()
	info.Password = "secret"
	_, err = api.Open(info, api.DialOpts{})

	redirErr, ok := errors.Cause(err).(*api.RedirectError)
	c.Assert(ok, gc.Equals, true)

	nhp := network.NewHostPorts(5555, "1.2.3.4")
	c.Assert(redirErr.Servers, jc.DeepEquals, [][]network.HostPort{nhp})
	c.Assert(redirErr.CACert, gc.Equals, coretesting.CACert)
	c.Assert(redirErr.FollowRedirect, gc.Equals, false)
}

func (s *stateSuite) TestLoginMacaroonInvalidId(c *gc.C) {
	apistate, tag, _ := s.OpenAPIWithoutLogin(c)
	defer apistate.Close()
	mac, err := macaroon.New([]byte("root-key"), []byte("id"), "juju")
	c.Assert(err, jc.ErrorIsNil)
	err = apistate.Login(tag, "", "", []macaroon.Slice{{mac}})
	c.Assert(err, gc.ErrorMatches, "interaction required but not possible")
}

func (s *stateSuite) TestLoginTracksFacadeVersions(c *gc.C) {
	apistate, tag, password := s.OpenAPIWithoutLogin(c)
	defer apistate.Close()
	// We haven't called Login yet, so the Facade Versions should be empty
	c.Check(apistate.AllFacadeVersions(), gc.HasLen, 0)
	err := apistate.Login(tag, password, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	// Now that we've logged in, AllFacadeVersions should be updated.
	allVersions := apistate.AllFacadeVersions()
	c.Check(allVersions, gc.Not(gc.HasLen), 0)
	// For sanity checking, ensure that we have a v2 of the Client facade
	c.Assert(allVersions["Client"], gc.Not(gc.HasLen), 0)
	c.Check(allVersions["Client"][0], gc.Equals, 1)
}

func (s *stateSuite) TestAllFacadeVersionsSafeFromMutation(c *gc.C) {
	allVersions := s.APIState.AllFacadeVersions()
	clients := allVersions["Client"]
	origClients := make([]int, len(clients))
	copy(origClients, clients)
	// Mutating the dict should not affect the cached versions
	allVersions["Client"] = append(allVersions["Client"], 2597)
	newVersions := s.APIState.AllFacadeVersions()
	newClientVers := newVersions["Client"]
	c.Check(newClientVers, gc.DeepEquals, origClients)
	c.Check(newClientVers[len(newClientVers)-1], gc.Not(gc.Equals), 2597)
}

func (s *stateSuite) TestBestFacadeVersion(c *gc.C) {
	c.Check(s.APIState.BestFacadeVersion("Client"), gc.Equals, 2)
}

func (s *stateSuite) TestAPIHostPortsMovesConnectedValueFirst(c *gc.C) {
	hostportslist := s.APIState.APIHostPorts()
	c.Check(hostportslist, gc.HasLen, 1)
	serverhostports := hostportslist[0]
	c.Check(serverhostports, gc.HasLen, 1)
	goodAddress := serverhostports[0]
	// the other addresses, but always see this one as well.
	info := s.APIInfo(c)
	// We intentionally set this to invalid values
	badValue := network.HostPort{
		Address: network.Address{
			Value: "0.1.2.3",
			Type:  network.IPv4Address,
			Scope: network.ScopeMachineLocal,
		},
		Port: 1234,
	}
	badServer := []network.HostPort{badValue}
	extraAddress := network.HostPort{
		Address: network.Address{
			Value: "0.1.2.4",
			Type:  network.IPv4Address,
			Scope: network.ScopeMachineLocal,
		},
		Port: 5678,
	}
	extraAddress2 := network.HostPort{
		Address: network.Address{
			Value: "0.1.2.1",
			Type:  network.IPv4Address,
			Scope: network.ScopeMachineLocal,
		},
		Port: 9012,
	}
	serverExtra := []network.HostPort{
		extraAddress, goodAddress, extraAddress2,
	}
	current := [][]network.HostPort{badServer, serverExtra}
	s.State.SetAPIHostPorts(current)
	apistate, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer apistate.Close()
	hostports := apistate.APIHostPorts()
	// We should have rotate the server we connected to as the first item,
	// and the address of that server as the first address
	sortedServer := []network.HostPort{
		goodAddress, extraAddress, extraAddress2,
	}
	expected := [][]network.HostPort{sortedServer, badServer}
	c.Check(hostports, gc.DeepEquals, expected)
}

var exampleHostPorts = []network.HostPort{{
	Address: network.NewAddress("0.1.2.3"),
	Port:    1234,
}, {
	Address: network.NewAddress("0.1.2.4"),
	Port:    5678,
}, {
	Address: network.NewAddress("0.1.2.1"),
	Port:    9012,
}, {
	Address: network.NewAddress("0.1.9.1"),
	Port:    8888,
}}

func (s *slideSuite) TestSlideToFrontNoOp(c *gc.C) {
	servers := [][]network.HostPort{
		{exampleHostPorts[0]},
		{exampleHostPorts[1]},
	}
	// order should not have changed
	expected := [][]network.HostPort{
		{exampleHostPorts[0]},
		{exampleHostPorts[1]},
	}
	api.SlideAddressToFront(servers, 0, 0)
	c.Check(servers, gc.DeepEquals, expected)
}

func (s *slideSuite) TestSlideToFrontAddress(c *gc.C) {
	servers := [][]network.HostPort{
		{exampleHostPorts[0], exampleHostPorts[1], exampleHostPorts[2]},
		{exampleHostPorts[3]},
	}
	// server order should not change, but ports should be switched
	expected := [][]network.HostPort{
		{exampleHostPorts[1], exampleHostPorts[0], exampleHostPorts[2]},
		{exampleHostPorts[3]},
	}
	api.SlideAddressToFront(servers, 0, 1)
	c.Check(servers, gc.DeepEquals, expected)
}

func (s *slideSuite) TestSlideToFrontServer(c *gc.C) {
	servers := [][]network.HostPort{
		{exampleHostPorts[0], exampleHostPorts[1]},
		{exampleHostPorts[2]},
		{exampleHostPorts[3]},
	}
	// server 1 should be slid to the front
	expected := [][]network.HostPort{
		{exampleHostPorts[2]},
		{exampleHostPorts[0], exampleHostPorts[1]},
		{exampleHostPorts[3]},
	}
	api.SlideAddressToFront(servers, 1, 0)
	c.Check(servers, gc.DeepEquals, expected)
}

func (s *slideSuite) TestSlideToFrontBoth(c *gc.C) {
	servers := [][]network.HostPort{
		{exampleHostPorts[0]},
		{exampleHostPorts[1], exampleHostPorts[2]},
		{exampleHostPorts[3]},
	}
	// server 1 should be slid to the front
	expected := [][]network.HostPort{
		{exampleHostPorts[2], exampleHostPorts[1]},
		{exampleHostPorts[0]},
		{exampleHostPorts[3]},
	}
	api.SlideAddressToFront(servers, 1, 1)
	c.Check(servers, gc.DeepEquals, expected)
}
