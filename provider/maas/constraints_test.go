// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
)

func (*environSuite) TestConvertConstraints(c *gc.C) {
	for i, test := range []struct {
		cons     constraints.Value
		expected url.Values
	}{{
		cons:     constraints.Value{Arch: stringp("arm")},
		expected: url.Values{"arch": {"arm"}},
	}, {
		cons:     constraints.Value{CpuCores: uint64p(4)},
		expected: url.Values{"cpu_count": {"4"}},
	}, {
		cons:     constraints.Value{Mem: uint64p(1024)},
		expected: url.Values{"mem": {"1024"}},
	}, { // Spaces are converted to bindings and not_networks, but only in acquireNode
		cons:     constraints.Value{Spaces: stringslicep("foo", "bar", "^baz", "^oof")},
		expected: url.Values{},
	}, {
		cons: constraints.Value{Tags: stringslicep("tag1", "tag2", "^tag3", "^tag4")},
		expected: url.Values{
			"tags":     {"tag1,tag2"},
			"not_tags": {"tag3,tag4"},
		},
	}, { // CpuPower is ignored.
		cons:     constraints.Value{CpuPower: uint64p(1024)},
		expected: url.Values{},
	}, { // RootDisk is ignored.
		cons:     constraints.Value{RootDisk: uint64p(8192)},
		expected: url.Values{},
	}, {
		cons:     constraints.Value{Tags: stringslicep("foo", "bar")},
		expected: url.Values{"tags": {"foo,bar"}},
	}, {
		cons: constraints.Value{
			Arch:     stringp("arm"),
			CpuCores: uint64p(4),
			Mem:      uint64p(1024),
			CpuPower: uint64p(1024),
			RootDisk: uint64p(8192),
			Spaces:   stringslicep("foo", "^bar"),
			Tags:     stringslicep("^tag1", "tag2"),
		},
		expected: url.Values{
			"arch":      {"arm"},
			"cpu_count": {"4"},
			"mem":       {"1024"},
			"tags":      {"tag2"},
			"not_tags":  {"tag1"},
		},
	}} {
		c.Logf("test #%d: cons=%s", i, test.cons.String())
		c.Check(convertConstraints(test.cons), jc.DeepEquals, test.expected)
	}
}

func (*environSuite) TestConvertConstraints2(c *gc.C) {
	for i, test := range []struct {
		cons     constraints.Value
		expected gomaasapi.AllocateMachineArgs
	}{{
		cons:     constraints.Value{Arch: stringp("arm")},
		expected: gomaasapi.AllocateMachineArgs{Architecture: "arm"},
	}, {
		cons:     constraints.Value{CpuCores: uint64p(4)},
		expected: gomaasapi.AllocateMachineArgs{MinCPUCount: 4},
	}, {
		cons:     constraints.Value{Mem: uint64p(1024)},
		expected: gomaasapi.AllocateMachineArgs{MinMemory: 1024},
	}, { // Spaces are converted to bindings and not_networks, but only in acquireNode
		cons:     constraints.Value{Spaces: stringslicep("foo", "bar", "^baz", "^oof")},
		expected: gomaasapi.AllocateMachineArgs{},
	}, {
		cons: constraints.Value{Tags: stringslicep("tag1", "tag2", "^tag3", "^tag4")},
		expected: gomaasapi.AllocateMachineArgs{
			Tags:    []string{"tag1", "tag2"},
			NotTags: []string{"tag3", "tag4"},
		},
	}, { // CpuPower is ignored.
		cons:     constraints.Value{CpuPower: uint64p(1024)},
		expected: gomaasapi.AllocateMachineArgs{},
	}, { // RootDisk is ignored.
		cons:     constraints.Value{RootDisk: uint64p(8192)},
		expected: gomaasapi.AllocateMachineArgs{},
	}, {
		cons: constraints.Value{Tags: stringslicep("foo", "bar")},
		expected: gomaasapi.AllocateMachineArgs{
			Tags: []string{"foo", "bar"},
		},
	}, {
		cons: constraints.Value{
			Arch:     stringp("arm"),
			CpuCores: uint64p(4),
			Mem:      uint64p(1024),
			CpuPower: uint64p(1024),
			RootDisk: uint64p(8192),
			Spaces:   stringslicep("foo", "^bar"),
			Tags:     stringslicep("^tag1", "tag2"),
		},
		expected: gomaasapi.AllocateMachineArgs{
			Architecture: "arm",
			MinCPUCount:  4,
			MinMemory:    1024,
			Tags:         []string{"tag2"},
			NotTags:      []string{"tag1"},
		},
	}} {
		c.Logf("test #%d: cons2=%s", i, test.cons.String())
		c.Check(convertConstraints2(test.cons), jc.DeepEquals, test.expected)
	}
}

var nilStringSlice []string

func (*environSuite) TestConvertTagsToParams(c *gc.C) {
	for i, test := range []struct {
		tags     *[]string
		expected url.Values
	}{{
		tags:     nil,
		expected: url.Values{},
	}, {
		tags:     &nilStringSlice,
		expected: url.Values{},
	}, {
		tags:     &[]string{},
		expected: url.Values{},
	}, {
		tags:     stringslicep(""),
		expected: url.Values{},
	}, {
		tags: stringslicep("foo"),
		expected: url.Values{
			"tags": {"foo"},
		},
	}, {
		tags: stringslicep("^bar"),
		expected: url.Values{
			"not_tags": {"bar"},
		},
	}, {
		tags: stringslicep("foo", "^bar", "baz", "^oof"),
		expected: url.Values{
			"tags":     {"foo,baz"},
			"not_tags": {"bar,oof"},
		},
	}, {
		tags: stringslicep("", "^bar", "^", "^oof"),
		expected: url.Values{
			"not_tags": {"bar,oof"},
		},
	}, {
		tags: stringslicep("foo", "^", " b a z  ", "^^ ^"),
		expected: url.Values{
			"tags":     {"foo, b a z  "},
			"not_tags": {"^ ^"},
		},
	}, {
		tags: stringslicep("", "^bar", "  ", " ^ o of "),
		expected: url.Values{
			"tags":     {"  , ^ o of "},
			"not_tags": {"bar"},
		},
	}, {
		tags: stringslicep("foo", "foo", "^bar", "^bar"),
		expected: url.Values{
			"tags":     {"foo,foo"},
			"not_tags": {"bar,bar"},
		},
	}} {
		c.Logf("test #%d: tags=%v", i, test.tags)
		var vals = url.Values{}
		convertTagsToParams(vals, test.tags)
		c.Check(vals, jc.DeepEquals, test.expected)
	}
}

func uint64p(val uint64) *uint64 {
	return &val
}

func stringp(val string) *string {
	return &val
}

func stringslicep(values ...string) *[]string {
	return &values
}

func (suite *environSuite) TestSelectNodeValidZone(c *gc.C) {
	env := suite.makeEnviron()
	suite.testMAASObject.TestServer.NewNode(`{"system_id": "node0", "hostname": "host0", "zone": "bar"}`)

	snArgs := selectNodeArgs{
		AvailabilityZones: []string{"foo", "bar"},
		Constraints:       constraints.Value{},
	}

	node, err := env.selectNode(snArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(node, gc.NotNil)
}

func (suite *environSuite) TestSelectNodeInvalidZone(c *gc.C) {
	env := suite.makeEnviron()

	snArgs := selectNodeArgs{
		AvailabilityZones: []string{"foo", "bar"},
		Constraints:       constraints.Value{},
	}

	_, err := env.selectNode(snArgs)
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, `cannot run instances: ServerError: 409 Conflict \(\)`)
}

func (suite *environSuite) TestAcquireNode(c *gc.C) {
	env := suite.makeEnviron()
	suite.testMAASObject.TestServer.NewNode(`{"system_id": "node0", "hostname": "host0"}`)

	_, err := env.acquireNode("", "", constraints.Value{}, nil, nil)

	c.Check(err, jc.ErrorIsNil)
	operations := suite.testMAASObject.TestServer.NodeOperations()
	actions, found := operations["node0"]
	c.Assert(found, jc.IsTrue)
	c.Check(actions, gc.DeepEquals, []string{"acquire"})

	// no "name" parameter should have been passed through
	values := suite.testMAASObject.TestServer.NodeOperationRequestValues()["node0"][0]
	_, found = values["name"]
	c.Assert(found, jc.IsFalse)
}

func (suite *environSuite) TestAcquireNodeByName(c *gc.C) {
	env := suite.makeEnviron()
	suite.testMAASObject.TestServer.NewNode(`{"system_id": "node0", "hostname": "host0"}`)

	_, err := env.acquireNode("host0", "", constraints.Value{}, nil, nil)

	c.Check(err, jc.ErrorIsNil)
	operations := suite.testMAASObject.TestServer.NodeOperations()
	actions, found := operations["node0"]
	c.Assert(found, jc.IsTrue)
	c.Check(actions, gc.DeepEquals, []string{"acquire"})

	// no "name" parameter should have been passed through
	values := suite.testMAASObject.TestServer.NodeOperationRequestValues()["node0"][0]
	nodeName := values.Get("name")
	c.Assert(nodeName, gc.Equals, "host0")
}

func (suite *environSuite) TestAcquireNodeTakesConstraintsIntoAccount(c *gc.C) {
	env := suite.makeEnviron()
	suite.testMAASObject.TestServer.NewNode(
		`{"system_id": "node0", "hostname": "host0", "architecture": "arm/generic", "memory": 2048}`,
	)
	constraints := constraints.Value{Arch: stringp("arm"), Mem: uint64p(1024)}

	_, err := env.acquireNode("", "", constraints, nil, nil)

	c.Check(err, jc.ErrorIsNil)
	requestValues := suite.testMAASObject.TestServer.NodeOperationRequestValues()
	nodeRequestValues, found := requestValues["node0"]
	c.Assert(found, jc.IsTrue)
	c.Assert(nodeRequestValues[0].Get("arch"), gc.Equals, "arm")
	c.Assert(nodeRequestValues[0].Get("mem"), gc.Equals, "1024")
}

func (suite *environSuite) TestParseDelimitedValues(c *gc.C) {
	for i, test := range []struct {
		about     string
		input     []string
		positives []string
		negatives []string
	}{{
		about:     "nil input",
		input:     nil,
		positives: []string{},
		negatives: []string{},
	}, {
		about:     "empty input",
		input:     []string{},
		positives: []string{},
		negatives: []string{},
	}, {
		about:     "values list with embedded whitespace",
		input:     []string{"   val1  ", " val2", " ^ not Val 3  ", "  ", " ", "^", "", "^ notVal4   "},
		positives: []string{"   val1  ", " val2", " ^ not Val 3  ", "  ", " "},
		negatives: []string{" notVal4   "},
	}, {
		about:     "only positives",
		input:     []string{"val1", "val2", "val3"},
		positives: []string{"val1", "val2", "val3"},
		negatives: []string{},
	}, {
		about:     "only negatives",
		input:     []string{"^val1", "^val2", "^val3"},
		positives: []string{},
		negatives: []string{"val1", "val2", "val3"},
	}, {
		about:     "multi-caret negatives",
		input:     []string{"^foo^", "^v^a^l2", "  ^^ ^", "^v^al3", "^^", "^"},
		positives: []string{"  ^^ ^"},
		negatives: []string{"foo^", "v^a^l2", "v^al3", "^"},
	}, {
		about:     "both positives and negatives",
		input:     []string{"^val1", "val2", "^val3", "val4"},
		positives: []string{"val2", "val4"},
		negatives: []string{"val1", "val3"},
	}, {
		about:     "single positive value",
		input:     []string{"val1"},
		positives: []string{"val1"},
		negatives: []string{},
	}, {
		about:     "single negative value",
		input:     []string{"^val1"},
		positives: []string{},
		negatives: []string{"val1"},
	}} {
		c.Logf("test %d: %s", i, test.about)
		positives, negatives := parseDelimitedValues(test.input)
		c.Check(positives, jc.DeepEquals, test.positives)
		c.Check(negatives, jc.DeepEquals, test.negatives)
	}
}

func (suite *environSuite) TestAcquireNodePassedAgentName(c *gc.C) {
	env := suite.makeEnviron()
	suite.testMAASObject.TestServer.NewNode(`{"system_id": "node0", "hostname": "host0"}`)

	_, err := env.acquireNode("", "", constraints.Value{}, nil, nil)

	c.Check(err, jc.ErrorIsNil)
	requestValues := suite.testMAASObject.TestServer.NodeOperationRequestValues()
	nodeRequestValues, found := requestValues["node0"]
	c.Assert(found, jc.IsTrue)
	c.Assert(nodeRequestValues[0].Get("agent_name"), gc.Equals, env.Config().UUID())
}

func (suite *environSuite) TestAcquireNodePassesPositiveAndNegativeTags(c *gc.C) {
	env := suite.makeEnviron()
	suite.testMAASObject.TestServer.NewNode(`{"system_id": "node0"}`)

	_, err := env.acquireNode(
		"", "",
		constraints.Value{Tags: stringslicep("tag1", "^tag2", "tag3", "^tag4")},
		nil, nil,
	)

	c.Check(err, jc.ErrorIsNil)
	requestValues := suite.testMAASObject.TestServer.NodeOperationRequestValues()
	nodeValues, found := requestValues["node0"]
	c.Assert(found, jc.IsTrue)
	c.Assert(nodeValues[0].Get("tags"), gc.Equals, "tag1,tag3")
	c.Assert(nodeValues[0].Get("not_tags"), gc.Equals, "tag2,tag4")
}

func (suite *environSuite) TestAcquireNodePassesPositiveAndNegativeSpaces(c *gc.C) {
	suite.createFourSpaces(c)
	env := suite.makeEnviron()
	suite.testMAASObject.TestServer.NewNode(`{"system_id": "node0"}`)

	_, err := env.acquireNode(
		"", "",
		constraints.Value{Spaces: stringslicep("space-1", "^space-2", "space-3", "^space-4")},
		nil, nil,
	)
	c.Check(err, jc.ErrorIsNil)
	requestValues := suite.testMAASObject.TestServer.NodeOperationRequestValues()
	nodeValues, found := requestValues["node0"]
	c.Assert(found, jc.IsTrue)
	c.Check(nodeValues[0].Get("interfaces"), gc.Equals, "0:space=2;1:space=4")
	c.Check(nodeValues[0].Get("not_networks"), gc.Equals, "space:3,space:5")
}

func (suite *environSuite) createFourSpaces(c *gc.C) {
	server := suite.testMAASObject.TestServer
	server.SetVersionJSON(`{"capabilities": ["network-deployment-ubuntu"]}`)
	server.NewSpace(spaceJSON(gomaasapi.CreateSpace{Name: "space-1"}))
	suite.addSubnet(c, 1, 1, "node1")
	server.NewSpace(spaceJSON(gomaasapi.CreateSpace{Name: "space-2"}))
	suite.addSubnet(c, 2, 2, "node1")
	server.NewSpace(spaceJSON(gomaasapi.CreateSpace{Name: "space-3"}))
	suite.addSubnet(c, 3, 3, "node1")
	server.NewSpace(spaceJSON(gomaasapi.CreateSpace{Name: "space-4"}))
	suite.addSubnet(c, 4, 4, "node1")
}

func (suite *environSuite) TestAcquireNodeDisambiguatesNamedLabelsFromIndexedUpToALimit(c *gc.C) {
	suite.createFourSpaces(c)
	var shortLimit uint = 0
	suite.PatchValue(&numericLabelLimit, shortLimit)
	env := suite.makeEnviron()
	suite.testMAASObject.TestServer.NewNode(`{"system_id": "node0"}`)

	_, err := env.acquireNode(
		"", "",
		constraints.Value{Spaces: stringslicep("space-1", "^space-2", "space-3", "^space-4")},
		[]interfaceBinding{{"0", "first-clash"}, {"1", "final-clash"}},
		nil,
	)
	c.Assert(err, gc.ErrorMatches, `too many conflicting numeric labels, giving up.`)
}

func (suite *environSuite) TestAcquireNodeStorage(c *gc.C) {
	server := suite.testMAASObject.TestServer
	for i, test := range []struct {
		volumes  []volumeInfo
		expected string
	}{{
		volumes:  nil,
		expected: "",
	}, {
		volumes:  []volumeInfo{{"volume-1", 1234, nil}},
		expected: "volume-1:1234",
	}, {
		volumes:  []volumeInfo{{"", 1234, []string{"tag1", "tag2"}}},
		expected: "1234(tag1,tag2)",
	}, {
		volumes:  []volumeInfo{{"volume-1", 1234, []string{"tag1", "tag2"}}},
		expected: "volume-1:1234(tag1,tag2)",
	}, {
		volumes: []volumeInfo{
			{"volume-1", 1234, []string{"tag1", "tag2"}},
			{"volume-2", 4567, []string{"tag1", "tag3"}},
		},
		expected: "volume-1:1234(tag1,tag2),volume-2:4567(tag1,tag3)",
	}} {
		c.Logf("test #%d: volumes=%v", i, test.volumes)
		env := suite.makeEnviron()
		server.NewSpace(spaceJSON(gomaasapi.CreateSpace{Name: "space-1"}))
		server.NewNode(`{"system_id": "node0", "hostname": "host0"}`)
		suite.addSubnet(c, 1, 1, "node0")
		_, err := env.acquireNode("", "", constraints.Value{}, nil, test.volumes)
		c.Check(err, jc.ErrorIsNil)
		requestValues := server.NodeOperationRequestValues()
		nodeRequestValues, found := requestValues["node0"]
		if c.Check(found, jc.IsTrue) {
			c.Check(nodeRequestValues[0].Get("storage"), gc.Equals, test.expected)
		}
		suite.testMAASObject.TestServer.Clear()
	}
}

func (suite *environSuite) TestAcquireNodeInterfaces(c *gc.C) {
	server := suite.testMAASObject.TestServer
	// Add some constraints, including spaces to verify specified bindings
	// always override any spaces constraints.
	cons := constraints.Value{
		Spaces: stringslicep("foo", "^bar"),
	}
	// In the tests below "space:5" means foo, "space:6" means bar.
	for i, test := range []struct {
		interfaces        []interfaceBinding
		expectedPositives string
		expectedNegatives string
		expectedError     string
	}{{ // without specified bindings, spaces constraints are used instead.
		interfaces:        nil,
		expectedPositives: "0:space=5",
		expectedNegatives: "space:6",
		expectedError:     "",
	}, {
		interfaces:        []interfaceBinding{{"name-1", "space-1"}},
		expectedPositives: "name-1:space=space-1;0:space=5",
		expectedNegatives: "space:6",
	}, {
		interfaces: []interfaceBinding{
			{"name-1", "1"},
			{"name-2", "2"},
			{"name-3", "3"},
		},
		expectedPositives: "name-1:space=1;name-2:space=2;name-3:space=3;0:space=5",
		expectedNegatives: "space:6",
	}, {
		interfaces:    []interfaceBinding{{"", "anything"}},
		expectedError: "interface bindings cannot have empty names",
	}, {
		interfaces:    []interfaceBinding{{"shared-db", "6"}},
		expectedError: `negative space "bar" from constraints clashes with interface bindings`,
	}, {
		interfaces: []interfaceBinding{
			{"shared-db", "1"},
			{"db", "1"},
		},
		expectedPositives: "shared-db:space=1;db:space=1;0:space=5",
		expectedNegatives: "space:6",
	}, {
		interfaces:    []interfaceBinding{{"", ""}},
		expectedError: "interface bindings cannot have empty names",
	}, {
		interfaces: []interfaceBinding{
			{"valid", "ok"},
			{"", "valid-but-ignored-space"},
			{"valid-name-empty-space", ""},
			{"", ""},
		},
		expectedError: "interface bindings cannot have empty names",
	}, {
		interfaces:    []interfaceBinding{{"foo", ""}},
		expectedError: `invalid interface binding "foo": space provider ID is required`,
	}, {
		interfaces: []interfaceBinding{
			{"bar", ""},
			{"valid", "ok"},
			{"", "valid-but-ignored-space"},
			{"", ""},
		},
		expectedError: `invalid interface binding "bar": space provider ID is required`,
	}, {
		interfaces: []interfaceBinding{
			{"dup-name", "1"},
			{"dup-name", "2"},
		},
		expectedError: `duplicated interface binding "dup-name"`,
	}, {
		interfaces: []interfaceBinding{
			{"valid-1", "0"},
			{"dup-name", "1"},
			{"dup-name", "2"},
			{"valid-2", "3"},
		},
		expectedError: `duplicated interface binding "dup-name"`,
	}} {
		suite.testMAASObject.TestServer.Clear()
		c.Logf("test #%d: interfaces=%v", i, test.interfaces)
		suite.createFourSpaces(c)
		server.NewSpace(spaceJSON(gomaasapi.CreateSpace{Name: "foo"}))
		suite.addSubnetWithSpace(c, 6, 6, "foo", "node1")
		server.NewSpace(spaceJSON(gomaasapi.CreateSpace{Name: "bar"}))
		suite.addSubnetWithSpace(c, 7, 7, "bar", "node1")
		env := suite.makeEnviron()
		server.NewNode(`{"system_id": "node0", "hostname": "host0"}`)
		_, err := env.acquireNode("", "", cons, test.interfaces, nil)
		if test.expectedError != "" {
			c.Check(err, gc.ErrorMatches, test.expectedError)
			c.Check(err, jc.Satisfies, errors.IsNotValid)
			continue
		}
		c.Check(err, jc.ErrorIsNil)
		requestValues := server.NodeOperationRequestValues()
		nodeRequestValues, found := requestValues["node0"]
		if c.Check(found, jc.IsTrue) {

			c.Check(nodeRequestValues[0].Get("interfaces"), gc.Equals, test.expectedPositives)
			c.Check(nodeRequestValues[0].Get("not_networks"), gc.Equals, test.expectedNegatives)
		}
	}
}

func (suite *environSuite) createFooBarSpaces(c *gc.C) {
	server := suite.testMAASObject.TestServer
	server.SetVersionJSON(`{"capabilities": ["network-deployment-ubuntu"]}`)
	server.NewSpace(spaceJSON(gomaasapi.CreateSpace{Name: "foo"}))
	suite.addSubnetWithSpace(c, 1, 2, "foo", "node1")
	server.NewSpace(spaceJSON(gomaasapi.CreateSpace{Name: "bar"}))
	suite.addSubnetWithSpace(c, 2, 3, "bar", "node1")
}

func (suite *environSuite) TestAcquireNodeConvertsSpaceNames(c *gc.C) {
	server := suite.testMAASObject.TestServer
	suite.createFooBarSpaces(c)
	cons := constraints.Value{
		Spaces: stringslicep("foo", "^bar"),
	}
	env := suite.makeEnviron()
	server.NewNode(`{"system_id": "node0", "hostname": "host0"}`)
	_, err := env.acquireNode("", "", cons, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	requestValues := server.NodeOperationRequestValues()
	nodeRequestValues, found := requestValues["node0"]
	c.Assert(found, jc.IsTrue)
	c.Check(nodeRequestValues[0].Get("interfaces"), gc.Equals, "0:space=2")
	c.Check(nodeRequestValues[0].Get("not_networks"), gc.Equals, "space:3")
}

func (suite *environSuite) TestAcquireNodeTranslatesSpaceNames(c *gc.C) {
	server := suite.testMAASObject.TestServer
	suite.createFooBarSpaces(c)
	cons := constraints.Value{
		Spaces: stringslicep("foo-1", "^bar-3"),
	}
	env := suite.makeEnviron()
	server.NewNode(`{"system_id": "node0", "hostname": "host0"}`)
	_, err := env.acquireNode("", "", cons, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	requestValues := server.NodeOperationRequestValues()
	nodeRequestValues, found := requestValues["node0"]
	c.Assert(found, jc.IsTrue)
	c.Check(nodeRequestValues[0].Get("interfaces"), gc.Equals, "0:space=2")
	c.Check(nodeRequestValues[0].Get("not_networks"), gc.Equals, "space:3")
}

func (suite *environSuite) TestAcquireNodeUnrecognisedSpace(c *gc.C) {
	server := suite.testMAASObject.TestServer
	suite.createFooBarSpaces(c)
	cons := constraints.Value{
		Spaces: stringslicep("baz"),
	}
	env := suite.makeEnviron()
	server.NewNode(`{"system_id": "node0", "hostname": "host0"}`)
	_, err := env.acquireNode("", "", cons, nil, nil)
	c.Assert(err, gc.ErrorMatches, `unrecognised space in constraint "baz"`)
}
