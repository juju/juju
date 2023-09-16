// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	coretesting "github.com/juju/juju/testing"
)

// linkLayerDevicesInternalSuite contains white-box tests for link-layer network
// devices' internals, which do not actually access mongo. The rest of the logic
// is tested in linkLayerDevicesStateSuite.
type linkLayerDevicesInternalSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&linkLayerDevicesInternalSuite{})

func (s *linkLayerDevicesInternalSuite) TestNewLinkLayerDeviceCreatesLinkLayerDevice(c *gc.C) {
	result := newLinkLayerDevice(nil, linkLayerDeviceDoc{})
	c.Assert(result, gc.NotNil)
	c.Assert(result.st, gc.IsNil)
	c.Assert(result.doc, jc.DeepEquals, linkLayerDeviceDoc{})
}

func (s *linkLayerDevicesInternalSuite) TestDocIDIncludesModelUUID(c *gc.C) {
	const localDocID = "foo"
	globalDocID := coretesting.ModelTag.Id() + ":" + localDocID

	result := s.newLinkLayerDeviceWithDummyState(linkLayerDeviceDoc{DocID: localDocID})
	c.Assert(result.DocID(), gc.Equals, globalDocID)

	result = s.newLinkLayerDeviceWithDummyState(linkLayerDeviceDoc{DocID: globalDocID})
	c.Assert(result.DocID(), gc.Equals, globalDocID)
}

func (s *linkLayerDevicesInternalSuite) newLinkLayerDeviceWithDummyState(doc linkLayerDeviceDoc) *LinkLayerDevice {
	// We only need the model UUID set for localID() and docID() to work.
	// The rest is tested in linkLayerDevicesStateSuite.
	dummyState := &State{modelTag: coretesting.ModelTag}
	return newLinkLayerDevice(dummyState, doc)
}

func (s *linkLayerDevicesInternalSuite) TestProviderIDIsEmptyWhenNotSet(c *gc.C) {
	result := s.newLinkLayerDeviceWithDummyState(linkLayerDeviceDoc{})
	c.Assert(result.ProviderID(), gc.Equals, network.Id(""))
}

func (s *linkLayerDevicesInternalSuite) TestProviderIDDoesNotIncludeModelUUIDWhenSet(c *gc.C) {
	const localProviderID = "foo"
	result := s.newLinkLayerDeviceWithDummyState(linkLayerDeviceDoc{ProviderID: localProviderID})
	c.Assert(result.ProviderID(), gc.Equals, network.Id(localProviderID))
}

func (s *linkLayerDevicesInternalSuite) TestParentDeviceReturnsNoErrorWhenParentNameNotSet(c *gc.C) {
	result := s.newLinkLayerDeviceWithDummyState(linkLayerDeviceDoc{})
	parent, err := result.ParentDevice()
	c.Check(parent, gc.IsNil)
	c.Check(err, jc.ErrorIsNil)
}

func (s *linkLayerDevicesInternalSuite) TestLinkLayerDeviceGlobalKeyHelper(c *gc.C) {
	result := linkLayerDeviceGlobalKey("42", "eno1")
	c.Assert(result, gc.Equals, "m#42#d#eno1")

	result = linkLayerDeviceGlobalKey("", "")
	c.Assert(result, gc.Equals, "")
}

func (s *linkLayerDevicesInternalSuite) TestParseLinkLayerParentNameAsGlobalKey(c *gc.C) {
	for i, test := range []struct {
		about              string
		input              string
		expectedError      string
		expectedMachineID  string
		expectedParentName string
	}{{
		about: "empty input - empty outputs and no error",
		input: "",
	}, {
		about: "name only as input - empty outputs and no error",
		input: "some-parent",
	}, {
		about:              "global key as input - parsed outputs and no error",
		input:              "m#42#d#br-eth1",
		expectedMachineID:  "42",
		expectedParentName: "br-eth1",
	}, {
		about:         "invalid name as input - empty outputs and NotValidError",
		input:         "some name with not enough # in it",
		expectedError: `ParentName "some name with not enough # in it" format not valid`,
	}, {
		about:         "almost a global key as input - empty outputs and NotValidError",
		input:         "x#foo#y#bar",
		expectedError: `ParentName "x#foo#y#bar" format not valid`,
	}} {
		c.Logf("test #%d: %q", i, test.about)
		gotMachineID, gotParentName, gotError := parseLinkLayerDeviceParentNameAsGlobalKey(test.input)
		if test.expectedError != "" {
			c.Check(gotError, gc.ErrorMatches, test.expectedError)
			c.Check(gotError, jc.ErrorIs, errors.NotValid)
		} else {
			c.Check(gotError, jc.ErrorIsNil)
		}
		c.Check(gotMachineID, gc.Equals, test.expectedMachineID)
		c.Check(gotParentName, gc.Equals, test.expectedParentName)
	}
}

func (s *linkLayerDevicesInternalSuite) TestStringIncludesTypeNameAndMachineID(c *gc.C) {
	doc := linkLayerDeviceDoc{
		MachineID: "42",
		Name:      "foo",
		Type:      network.BondDevice,
	}
	result := s.newLinkLayerDeviceWithDummyState(doc)
	expectedString := `bond device "foo" on machine "42"`

	c.Assert(result.String(), gc.Equals, expectedString)
}

func (s *linkLayerDevicesInternalSuite) TestRemainingSimpleGetterMethods(c *gc.C) {
	doc := linkLayerDeviceDoc{
		Name:        "bond0",
		MachineID:   "99",
		MTU:         uint(9000),
		Type:        network.BondDevice,
		MACAddress:  "aa:bb:cc:dd:ee:f0",
		IsAutoStart: true,
		IsUp:        true,
		ParentName:  "br-bond0",
	}
	result := s.newLinkLayerDeviceWithDummyState(doc)

	c.Check(result.Name(), gc.Equals, "bond0")
	c.Check(result.MachineID(), gc.Equals, "99")
	c.Check(result.MTU(), gc.Equals, uint(9000))
	c.Check(result.Type(), gc.Equals, network.BondDevice)
	c.Check(result.MACAddress(), gc.Equals, "aa:bb:cc:dd:ee:f0")
	c.Check(result.IsAutoStart(), jc.IsTrue)
	c.Check(result.IsUp(), jc.IsTrue)
	c.Check(result.ParentName(), gc.Equals, "br-bond0")
}
