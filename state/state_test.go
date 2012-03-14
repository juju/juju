// launchpad.net/juju/go/state
//
// Copyright (c) 2011-2012 Canonical Ltd.
package state_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju/go/charm"
	"launchpad.net/juju/go/state"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPackage integrates the tests into gotest.
func TestPackage(t *testing.T) {
	srv, dir := state.ZkSetUpEnvironment(t)
	defer state.ZkTearDownEnvironment(t, srv, dir)

	TestingT(t)
}

// charmDir returns a directory containing the given test charm.
func charmDir(name string) string {
	return filepath.Join("..", "charm", "testrepo", "series", name)
}

// readCharm returns a test charm by its name.
func readCharm(c *C, name string) charm.Charm {
	ch, err := charm.ReadDir(charmDir(name))
	c.Assert(err, IsNil)
	return ch
}

// localCharmURL returns the local URL of a charm.
func localCharmURL(ch charm.Charm) *charm.URL {
	url := fmt.Sprintf("local:series/%s-%d", ch.Meta().Name, ch.Revision())
	return charm.MustParseURL(url)
}

// addDummyCharm adds the 'dummy' charm state to st.
func addDummyCharm(c *C, st *state.State) (*state.Charm, *charm.URL) {
	ch := readCharm(c, "dummy")
	curl := localCharmURL(ch)
	bundleURL, err := url.Parse("http://bundle.url")
	c.Assert(err, IsNil)
	dummy, err := st.AddCharm(ch, curl, bundleURL)
	c.Assert(err, IsNil)
	return dummy, curl
}

// zkDeepCreate creates nested complete paths in ZooKeeper.
func zkDeepCreate(zk *zookeeper.Conn, path string) error {
	parts := strings.Split(path, "/")
	if len(parts) == 1 {
		_, err := zk.Create(path, "", 0, zookeeper.WorldACL(zookeeper.PERM_ALL))
		return err
	}
	current := ""
	for i := 1; i < len(parts); i++ {
		current = current + "/" + parts[i]
		_, err := zk.Create(current, "", 0, zookeeper.WorldACL(zookeeper.PERM_ALL))
		if err != nil {
			return err
		}
	}
	return nil
}

// zkRemoveTree recursively removes a tree.
func zkRemoveTree(zk *zookeeper.Conn, path string) error {
	// First recursively delete the children.
	children, _, err := zk.Children(path)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err = zkRemoveTree(zk, fmt.Sprintf("%s/%s", path, child)); err != nil {
			return err
		}
	}
	// Now delete the path itself.
	return zk.Delete(path, -1)
}

type StateSuite struct {
	zkServer   *zookeeper.Server
	zkTestRoot string
	zkTestPort int
	zkAddr     string
	zkConn     *zookeeper.Conn
	st         *state.State
}

var _ = Suite(&StateSuite{})

func (s *StateSuite) SetUpTest(c *C) {
	var err error
	s.st, err = state.Open(&state.Info{
		Addrs: []string{state.ZkAddr},
	})
	c.Assert(err, IsNil)
	err = s.st.Initialize()
	c.Assert(err, IsNil)
	s.zkConn = state.ZkConn(s.st)
}

func (s *StateSuite) TearDownTest(c *C) {
	// Delete possible nodes, ignore errors.
	zkRemoveTree(s.zkConn, "/topology")
	zkRemoveTree(s.zkConn, "/charms")
	zkRemoveTree(s.zkConn, "/services")
	zkRemoveTree(s.zkConn, "/machines")
	zkRemoveTree(s.zkConn, "/units")
	zkRemoveTree(s.zkConn, "/relations")
	zkRemoveTree(s.zkConn, "/initialized")
	s.zkConn.Close()
}

func (s StateSuite) TestAddCharm(c *C) {
	// Check that adding charms works correctly.
	dummyCharm := readCharm(c, "dummy")
	curl := localCharmURL(dummyCharm)
	bundleURL, err := url.Parse("http://bundle.url")
	c.Assert(err, IsNil)
	dummy, err := s.st.AddCharm(dummyCharm, curl, bundleURL)
	c.Assert(err, IsNil)
	c.Assert(dummy.URL().String(), Equals, curl.String())
	_, _, err = s.zkConn.Children("/charms")
	c.Assert(err, IsNil)
}

func (s StateSuite) TestCharmAttributes(c *C) {
	// Check that the basic (invariant) fields of the charm
	// are correctly in place.
	_, curl := addDummyCharm(c, s.st)

	dummy, err := s.st.Charm(curl)
	c.Assert(err, IsNil)
	c.Assert(dummy.URL().String(), Equals, curl.String())
	c.Assert(dummy.Revision(), Equals, 1)
	bundleURL, err := url.Parse("http://bundle.url")
	c.Assert(err, IsNil)
	c.Assert(dummy.BundleURL(), DeepEquals, bundleURL)
	meta := dummy.Meta()
	c.Assert(meta.Name, Equals, "dummy")
	config := dummy.Config()
	c.Assert(config.Options["title"], Equals,
		charm.Option{
			Default:     "My Title",
			Description: "A descriptive title used for the service.",
			Type:        "string",
		},
	)
}

func (s StateSuite) TestNonExistentCharmPriorToInitialization(c *C) {
	// Check that getting a charm before any other charm has been added fails nicely.
	curl, err := charm.ParseURL("local:series/dummy-1")
	c.Assert(err, IsNil)
	_, err = s.st.Charm(curl)
	c.Assert(err, ErrorMatches, `charm not found: "local:series/dummy-1"`)
}

func (s StateSuite) TestGetNonExistentCharm(c *C) {
	// Check that getting a non-existent charm fails nicely.
	addDummyCharm(c, s.st)

	curl := charm.MustParseURL("local:anotherseries/dummy-1")
	_, err := s.st.Charm(curl)
	c.Assert(err, ErrorMatches, `charm not found: "local:anotherseries/dummy-1"`)
}

func (s StateSuite) TestAddService(c *C) {
	// Check that adding services works correctly.
	dummy, curl := addDummyCharm(c, s.st)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")
	mysql, err := s.st.AddService("mysql", dummy)
	c.Assert(err, IsNil)
	c.Assert(mysql.Name(), Equals, "mysql")

	// Check that retrieving the new created services works correctly.
	wordpress, err = s.st.Service("wordpress")
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")
	url, err := wordpress.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(url.String(), Equals, curl.String())
	mysql, err = s.st.Service("mysql")
	c.Assert(err, IsNil)
	c.Assert(mysql.Name(), Equals, "mysql")
	url, err = mysql.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(url.String(), Equals, curl.String())
}

func (s StateSuite) TestRemoveService(c *C) {
	dummy, _ := addDummyCharm(c, s.st)
	service, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)

	// Check that removing the service works correctly.
	err = s.st.RemoveService(service)
	c.Assert(err, IsNil)
	service, err = s.st.Service("wordpress")
	c.Assert(err, ErrorMatches, `service with name "wordpress" not found`)
}

func (s StateSuite) TestReadNonExistentService(c *C) {
	_, err := s.st.Service("pressword")
	c.Assert(err, ErrorMatches, `service with name "pressword" not found`)
}

func (s StateSuite) TestAllServices(c *C) {
	// Check without existing services.
	services, err := s.st.AllServices()
	c.Assert(err, IsNil)
	c.Assert(len(services), Equals, 0)

	// Check that after adding services the result is ok.
	dummy, _ := addDummyCharm(c, s.st)
	_, err = s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	services, err = s.st.AllServices()
	c.Assert(err, IsNil)
	c.Assert(len(services), Equals, 1)

	_, err = s.st.AddService("mysql", dummy)
	c.Assert(err, IsNil)
	services, err = s.st.AllServices()
	c.Assert(err, IsNil)
	c.Assert(len(services), Equals, 2)

	// Check the returned service, order is defined by sorted keys.
	c.Assert(services[0].Name(), Equals, "wordpress")
	c.Assert(services[1].Name(), Equals, "mysql")
}

func (s StateSuite) TestServiceCharm(c *C) {
	dummy, curl := addDummyCharm(c, s.st)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)

	// Check that getting and setting the service charm URL works correctly.
	testcurl, err := wordpress.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(testcurl.String(), Equals, curl.String())
	testcurl, err = charm.ParseURL("local:myseries/mydummy-1")
	c.Assert(err, IsNil)
	err = wordpress.SetCharmURL(testcurl)
	c.Assert(err, IsNil)
	testcurl, err = wordpress.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(testcurl.String(), Equals, "local:myseries/mydummy-1")
}

func (s StateSuite) TestServiceExposed(c *C) {
	dummy, _ := addDummyCharm(c, s.st)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)

	// Check that querying for the exposed flag works correctly.
	exposed, err := wordpress.IsExposed()
	c.Assert(err, IsNil)
	c.Assert(exposed, Equals, false)

	// Check that setting and clearing the exposed flag works correctly.
	err = wordpress.SetExposed()
	c.Assert(err, IsNil)
	exposed, err = wordpress.IsExposed()
	c.Assert(err, IsNil)
	c.Assert(exposed, Equals, true)
	err = wordpress.ClearExposed()
	c.Assert(err, IsNil)
	exposed, err = wordpress.IsExposed()
	c.Assert(err, IsNil)
	c.Assert(exposed, Equals, false)

	// Check that setting and clearing the exposed flag multiple doesn't fail.
	err = wordpress.SetExposed()
	c.Assert(err, IsNil)
	err = wordpress.SetExposed()
	c.Assert(err, IsNil)
	err = wordpress.ClearExposed()
	c.Assert(err, IsNil)
	err = wordpress.ClearExposed()
	c.Assert(err, IsNil)

	// Check that setting and clearing the exposed flag on removed services also doesn't fail.
	err = s.st.RemoveService(wordpress)
	c.Assert(err, IsNil)
	err = wordpress.ClearExposed()
	c.Assert(err, IsNil)
}

func (s StateSuite) TestAddUnit(c *C) {
	dummy, _ := addDummyCharm(c, s.st)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)

	// Check that adding units works.
	unitZero, err := wordpress.AddUnit()
	c.Assert(err, IsNil)
	c.Assert(unitZero.Name(), Equals, "wordpress/0")
	unitOne, err := wordpress.AddUnit()
	c.Assert(err, IsNil)
	c.Assert(unitOne.Name(), Equals, "wordpress/1")
}

func (s StateSuite) TestReadUnit(c *C) {
	dummy, _ := addDummyCharm(c, s.st)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	_, err = wordpress.AddUnit()
	c.Assert(err, IsNil)
	_, err = wordpress.AddUnit()
	c.Assert(err, IsNil)
	mysql, err := s.st.AddService("mysql", dummy)
	c.Assert(err, IsNil)
	_, err = mysql.AddUnit()
	c.Assert(err, IsNil)

	// Check that retrieving a unit works correctly.
	unit, err := wordpress.Unit("wordpress/0")
	c.Assert(err, IsNil)
	c.Assert(unit.Name(), Equals, "wordpress/0")

	// Check that retrieving a non-existent or an invalidly
	// named unit fail nicely.
	unit, err = wordpress.Unit("wordpress")
	c.Assert(err, ErrorMatches, `"wordpress" is not a valid unit name`)
	unit, err = wordpress.Unit("wordpress/0/0")
	c.Assert(err, ErrorMatches, `"wordpress/0/0" is not a valid unit name`)
	unit, err = wordpress.Unit("pressword/0")
	c.Assert(err, ErrorMatches, `can't find unit "pressword/0" on service "wordpress"`)
	unit, err = wordpress.Unit("mysql/0")
	c.Assert(err, ErrorMatches, `can't find unit "mysql/0" on service "wordpress"`)

	// Check that retrieving unit names works.
	unitNames, err := wordpress.UnitNames()
	c.Assert(err, IsNil)
	c.Assert(unitNames, DeepEquals, []string{"wordpress/0", "wordpress/1"})

	// Check that retrieving all units works.
	units, err := wordpress.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(len(units), Equals, 2)
	c.Assert(units[0].Name(), Equals, "wordpress/0")
	c.Assert(units[1].Name(), Equals, "wordpress/1")
}

func (s StateSuite) TestReadUnitWithChangingState(c *C) {
	dummy, _ := addDummyCharm(c, s.st)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)

	// Check that reading a unit after removing the service
	// fails nicely.
	err = s.st.RemoveService(wordpress)
	c.Assert(err, IsNil)
	_, err = s.st.Unit("wordpress/0")
	c.Assert(err, ErrorMatches, `service with name "wordpress" not found`)
}

func (s StateSuite) TestRemoveUnit(c *C) {
	dummy, _ := addDummyCharm(c, s.st)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	_, err = wordpress.AddUnit()
	c.Assert(err, IsNil)
	_, err = wordpress.AddUnit()
	c.Assert(err, IsNil)

	// Check that removing a unit works.
	unit, err := wordpress.Unit("wordpress/0")
	c.Assert(err, IsNil)
	err = wordpress.RemoveUnit(unit)
	c.Assert(err, IsNil)
	unitNames, err := wordpress.UnitNames()
	c.Assert(err, IsNil)
	c.Assert(unitNames, DeepEquals, []string{"wordpress/1"})

	// Check that removing a non-existent unit fails nicely.
	err = wordpress.RemoveUnit(unit)
	c.Assert(err, ErrorMatches, "environment state has changed")
}

func (s StateSuite) TestGetSetPublicAddress(c *C) {
	dummy, _ := addDummyCharm(c, s.st)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)

	// Check that retrieving and setting of a public address works.
	address, err := unit.PublicAddress()
	c.Assert(err, ErrorMatches, "unit has no public address")
	err = unit.SetPublicAddress("example.foobar.com")
	c.Assert(err, IsNil)
	address, err = unit.PublicAddress()
	c.Assert(err, IsNil)
	c.Assert(address, Equals, "example.foobar.com")
}

func (s StateSuite) TestGetSetPrivateAddress(c *C) {
	dummy, _ := addDummyCharm(c, s.st)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)

	// Check that retrieving and setting of a private address works.
	address, err := unit.PrivateAddress()
	c.Assert(err, ErrorMatches, "unit has no private address")
	err = unit.SetPrivateAddress("example.local")
	c.Assert(err, IsNil)
	address, err = unit.PrivateAddress()
	c.Assert(err, IsNil)
	c.Assert(address, Equals, "example.local")
}

func (s StateSuite) TestUnitCharm(c *C) {
	dummy, curl := addDummyCharm(c, s.st)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)

	// Check that getting and setting the unit charm URL works correctly.
	testcurl, err := unit.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(testcurl.String(), Equals, curl.String())
	testcurl, err = charm.ParseURL("local:myseries/mydummy-1")
	c.Assert(err, IsNil)
	err = unit.SetCharmURL(testcurl)
	c.Assert(err, IsNil)
	testcurl, err = unit.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(testcurl.String(), Equals, "local:myseries/mydummy-1")
}

func (s StateSuite) TestUnassignUnitFromMachineWithoutBeingAssigned(c *C) {
	// When unassigning a machine from a unit, it is possible that
	// the machine has not been previously assigned, or that it
	// was assigned but the state changed beneath us.  In either
	// case, the end state is the intended state, so we simply
	// move forward without any errors here, to avoid having to
	// handle the extra complexity of dealing with the concurrency
	// problems.
	dummy, _ := addDummyCharm(c, s.st)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)

	err = unit.UnassignFromMachine()
	c.Assert(err, IsNil)

	// Check that the unit has no machine assigned.
	wordpress, err = s.st.Service("wordpress")
	c.Assert(err, IsNil)
	units, err := wordpress.AllUnits()
	c.Assert(err, IsNil)
	unit = units[0]
	_, err = unit.AssignedMachineId()
	c.Assert(err, ErrorMatches, `unit not assigned to machine`)
}

func (s StateSuite) TestAssignUnitToMachineAgainFails(c *C) {
	// Check that assigning an already assigned unit to
	// a machine fails if it isn't precisely the same
	// machine. 
	dummy, _ := addDummyCharm(c, s.st)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)
	machineOne, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	machineTwo, err := s.st.AddMachine()
	c.Assert(err, IsNil)

	err = unit.AssignToMachine(machineOne)
	c.Assert(err, IsNil)

	// Assigning the unit to the same machine should return no error.
	err = unit.AssignToMachine(machineOne)
	c.Assert(err, IsNil)

	// Assigning the unit to a different machine should fail.
	err = unit.AssignToMachine(machineTwo)
	c.Assert(err, ErrorMatches, `unit "wordpress/0" already assigned to machine 0`)

	machineId, err := unit.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(machineId, Equals, 0)
}

func (s StateSuite) TestUnassignUnitFromMachineWithChangingState(c *C) {
	// Check that unassigning while the state changes fails nicely.
	dummy, _ := addDummyCharm(c, s.st)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)

	// Remove the unit for the tests.
	wordpress, err = s.st.Service("wordpress")
	c.Assert(err, IsNil)
	units, err := wordpress.AllUnits()
	c.Assert(err, IsNil)
	unit = units[0]
	err = wordpress.RemoveUnit(unit)
	c.Assert(err, IsNil)

	err = unit.UnassignFromMachine()
	c.Assert(err, ErrorMatches, "environment state has changed")
	_, err = unit.AssignedMachineId()
	c.Assert(err, ErrorMatches, "environment state has changed")

	err = s.st.RemoveService(wordpress)
	c.Assert(err, IsNil)

	err = unit.UnassignFromMachine()
	c.Assert(err, ErrorMatches, "environment state has changed")
	_, err = unit.AssignedMachineId()
	c.Assert(err, ErrorMatches, "environment state has changed")
}

func (s StateSuite) TestAssignUnitToUnusedMachine(c *C) {
	// Create root machine that shouldn't be useds.
	_, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	// Check that a unit can be assigned to an unused machine.
	dummy, _ := addDummyCharm(c, s.st)
	mysqlService, err := s.st.AddService("mysql", dummy)
	c.Assert(err, IsNil)
	mysqlUnit, err := mysqlService.AddUnit()
	c.Assert(err, IsNil)
	mysqlMachine, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	err = mysqlUnit.AssignToMachine(mysqlMachine)
	c.Assert(err, IsNil)
	err = s.st.RemoveService(mysqlService)
	c.Assert(err, IsNil)

	wordpressService, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	wordpressUnit, err := wordpressService.AddUnit()
	c.Assert(err, IsNil)
	wordpressMachine, err := wordpressUnit.AssignToUnusedMachine()
	c.Assert(err, IsNil)

	c.Assert(wordpressMachine.Id(), Equals, mysqlMachine.Id())
}

func (s StateSuite) TestAssignUnitToUnusedMachineWithChangingService(c *C) {
	// Create root machine that shouldn't be useds.
	_, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	// Check for a 'state changed' error if a service is manipulated
	// during reuse.
	dummy, _ := addDummyCharm(c, s.st)
	mysqlService, err := s.st.AddService("mysql", dummy)
	c.Assert(err, IsNil)
	mysqlUnit, err := mysqlService.AddUnit()
	c.Assert(err, IsNil)
	mysqlMachine, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	err = mysqlUnit.AssignToMachine(mysqlMachine)
	c.Assert(err, IsNil)
	err = s.st.RemoveService(mysqlService)
	c.Assert(err, IsNil)

	wordpressService, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	wordpressUnit, err := wordpressService.AddUnit()
	c.Assert(err, IsNil)
	err = s.st.RemoveService(wordpressService)
	c.Assert(err, IsNil)

	_, err = wordpressUnit.AssignToUnusedMachine()
	c.Assert(err, ErrorMatches, "environment state has changed")
}

func (s StateSuite) TestAssignUnitToUnusedMachineWithChangingUnit(c *C) {
	// Create root machine that shouldn't be useds.
	_, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	// Check for a 'state changed' error if a unit is manipulated
	// during reuse.
	dummy, _ := addDummyCharm(c, s.st)
	mysqlService, err := s.st.AddService("mysql", dummy)
	c.Assert(err, IsNil)
	mysqlUnit, err := mysqlService.AddUnit()
	c.Assert(err, IsNil)
	mysqlMachine, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	err = mysqlUnit.AssignToMachine(mysqlMachine)
	c.Assert(err, IsNil)
	err = s.st.RemoveService(mysqlService)
	c.Assert(err, IsNil)

	wordpressService, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	wordpressUnit, err := wordpressService.AddUnit()
	c.Assert(err, IsNil)
	err = wordpressService.RemoveUnit(wordpressUnit)
	c.Assert(err, IsNil)

	_, err = wordpressUnit.AssignToUnusedMachine()
	c.Assert(err, ErrorMatches, "environment state has changed")
}

func (s StateSuite) TestAssignUnitToUnusedMachineOnlyZero(c *C) {
	// Create root machine that shouldn't be useds.
	_, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	// Check that the unit can't be assigned to machine zero.
	dummy, _ := addDummyCharm(c, s.st)
	wordpressService, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	wordpressUnit, err := wordpressService.AddUnit()
	c.Assert(err, IsNil)

	_, err = wordpressUnit.AssignToUnusedMachine()
	c.Assert(err, ErrorMatches, "no unused machine found")
}

func (s StateSuite) TestAssignUnitToUnusedMachineNoneAvailable(c *C) {
	// Create machine 0, that shouldn't be used.
	_, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	// Check that assigning without unused machine fails.	
	dummy, _ := addDummyCharm(c, s.st)
	mysqlService, err := s.st.AddService("mysql", dummy)
	c.Assert(err, IsNil)
	mysqlUnit, err := mysqlService.AddUnit()
	c.Assert(err, IsNil)
	mysqlMachine, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	err = mysqlUnit.AssignToMachine(mysqlMachine)
	c.Assert(err, IsNil)

	wordpressService, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	wordpressUnit, err := wordpressService.AddUnit()
	c.Assert(err, IsNil)

	_, err = wordpressUnit.AssignToUnusedMachine()
	c.Assert(err, ErrorMatches, "no unused machine found")
}

func (s StateSuite) TestGetSetClearUnitUpgrate(c *C) {
	// Check that setting and clearing an upgrade flag on a unit works.
	dummy, _ := addDummyCharm(c, s.st)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)

	// Defaults to false.
	upgrade, err := unit.NeedsUpgrade()
	c.Assert(err, IsNil)
	c.Assert(upgrade, Equals, false)

	// Can be set.
	err = unit.SetNeedsUpgrade()
	c.Assert(err, IsNil)
	upgrade, err = unit.NeedsUpgrade()
	c.Assert(err, IsNil)
	c.Assert(upgrade, Equals, true)

	// Can be set multiple times.
	err = unit.SetNeedsUpgrade()
	c.Assert(err, IsNil)
	upgrade, err = unit.NeedsUpgrade()
	c.Assert(err, IsNil)
	c.Assert(upgrade, Equals, true)

	// Can be cleared.
	err = unit.ClearNeedsUpgrade()
	c.Assert(err, IsNil)
	upgrade, err = unit.NeedsUpgrade()
	c.Assert(err, IsNil)
	c.Assert(upgrade, Equals, false)

	// Can be cleared multiple times
	err = unit.ClearNeedsUpgrade()
	c.Assert(err, IsNil)
	upgrade, err = unit.NeedsUpgrade()
	c.Assert(err, IsNil)
	c.Assert(upgrade, Equals, false)
}

func (s StateSuite) TestGetSetClearResolved(c *C) {
	// Check that setting and clearing the resolved setting on a unit works.
	dummy, _ := addDummyCharm(c, s.st)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)

	setting, err := unit.Resolved()
	c.Assert(err, IsNil)
	c.Assert(setting, Equals, state.ResolvedNone)

	err = unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, IsNil)
	err = unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, ErrorMatches, `unit "wordpress/0" resolved flag already set`)
	retry, err := unit.Resolved()
	c.Assert(err, IsNil)
	c.Assert(retry, Equals, state.ResolvedNoHooks)

	err = unit.ClearResolved()
	c.Assert(err, IsNil)
	setting, err = unit.Resolved()
	c.Assert(err, IsNil)
	c.Assert(setting, Equals, state.ResolvedNone)
	err = unit.ClearResolved()
	c.Assert(err, IsNil)

	err = unit.SetResolved(state.ResolvedMode(999))
	c.Assert(err, ErrorMatches, `invalid error resolution mode: 999`)
}

func (s StateSuite) TestGetOpenPorts(c *C) {
	// Check that changes to the open ports of units work porperly.
	dummy, _ := addDummyCharm(c, s.st)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)

	// Verify no open ports before activity.
	open, err := unit.OpenPorts()
	c.Assert(err, IsNil)
	c.Assert(open, HasLen, 0)

	// Now open and close port.
	err = unit.OpenPort("tcp", 80)
	c.Assert(err, IsNil)
	open, err = unit.OpenPorts()
	c.Assert(err, IsNil)
	c.Assert(open, DeepEquals, []state.Port{
		{"tcp", 80},
	})

	err = unit.OpenPort("udp", 53)
	c.Assert(err, IsNil)
	open, err = unit.OpenPorts()
	c.Assert(err, IsNil)
	c.Assert(open, DeepEquals, []state.Port{
		{"tcp", 80},
		{"udp", 53},
	})

	err = unit.OpenPort("tcp", 53)
	c.Assert(err, IsNil)
	open, err = unit.OpenPorts()
	c.Assert(err, IsNil)
	c.Assert(open, DeepEquals, []state.Port{
		{"tcp", 80},
		{"udp", 53},
		{"tcp", 53},
	})

	err = unit.OpenPort("tcp", 443)
	c.Assert(err, IsNil)
	open, err = unit.OpenPorts()
	c.Assert(err, IsNil)
	c.Assert(open, DeepEquals, []state.Port{
		{"tcp", 80},
		{"udp", 53},
		{"tcp", 53},
		{"tcp", 443},
	})

	err = unit.ClosePort("tcp", 80)
	c.Assert(err, IsNil)
	open, err = unit.OpenPorts()
	c.Assert(err, IsNil)
	c.Assert(open, DeepEquals, []state.Port{
		{"udp", 53},
		{"tcp", 53},
		{"tcp", 443},
	})
}

type AgentSuite struct {
	zkConn     *zookeeper.Conn
	st         *state.State
	path       string
}

var _ = Suite(&AgentSuite{})

func (s *AgentSuite) SetUpTest(c *C) {
	// Connect the server.
	st, err := state.Open(&state.Info{
		Addrs: []string{state.ZkAddr},
	})
	c.Assert(err, IsNil)
	s.st = st
	s.zkConn = state.ZkConn(st)
	// Prepare path for dummy entity.
	s.path = "/dummy/key-0000000001"
	zkDeepCreate(s.zkConn, s.path)
}

func (s *AgentSuite) TearDownTest(c *C) {
	if s.zkConn != nil {
		// Delete possible nodes, ignore errors.
		// zkRemoveTree(s.zkConn, "/dummy")
		zkRemoveTree(s.zkConn, "/dummy")
		s.zkConn.Close()
	}
}

func (s AgentSuite) TestHasAgent(c *C) {
	d := state.NewAgentProcessableEntitiy(s.st)
	exists, err := d.HasAgent()
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, false)

	err = d.ConnectAgent()
	c.Assert(err, IsNil)

	exists, err = d.HasAgent()
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)
}

func (s AgentSuite) TestWatchAgent(c *C) {
	d := state.NewAgentProcessableEntitiy(s.st)
	aw, err := d.WatchAgent()
	c.Assert(err, IsNil)

	set, err := aw.IsSet(25 * time.Millisecond)
	c.Assert(err, ErrorMatches, "watch timed out")
	c.Assert(set, Equals, false)

	err = d.ConnectAgent()
	c.Assert(err, IsNil)

	set, err = aw.IsSet(25 * time.Millisecond)
	c.Assert(err, IsNil)
	c.Assert(set, Equals, true)

	zkRemoveTree(s.zkConn, "/dummy")

	set, err = aw.IsSet(25 * time.Millisecond)
	c.Assert(err, IsNil)
	c.Assert(set, Equals, false)
}

func (s *AgentSuite) TestConnectAgent(c *C) {
	zkState, watch, err := s.zkConn.ExistsW(s.path + "/agent")
	c.Assert(err, IsNil)
	c.Assert(zkState, IsNil)

	d := state.NewAgentProcessableEntitiy(s.st)
	err = d.ConnectAgent()
	c.Assert(err, IsNil)

	e := <-watch
	c.Assert(e.Type, Equals, zookeeper.EVENT_CREATED)

	zkState, watch, err = s.zkConn.ExistsW(s.path + "/agent")
	c.Assert(err, IsNil)
	c.Assert(zkState, Not(IsNil))

	// Force close to get a 'closed' event. Differs from Python
	// client which returns a 'deleted' event.
	s.zkConn.Close()
	s.zkConn = nil

	e = <-watch
	c.Assert(e.Type, Equals, zookeeper.EVENT_CLOSED)
}
