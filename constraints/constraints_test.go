// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constraints_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type ConstraintsSuite struct{}

var _ = gc.Suite(&ConstraintsSuite{})

var parseConstraintsTests = []struct {
	summary string
	args    []string
	err     string
}{
	// Simple errors.
	{
		summary: "nothing at all",
	}, {
		summary: "empty",
		args:    []string{"     "},
	}, {
		summary: "complete nonsense",
		args:    []string{"cheese"},
		err:     `malformed constraint "cheese"`,
	}, {
		summary: "missing name",
		args:    []string{"=cheese"},
		err:     `malformed constraint "=cheese"`,
	}, {
		summary: "unknown constraint",
		args:    []string{"cheese=edam"},
		err:     `unknown constraint "cheese"`,
	},

	// "container" in detail.
	{
		summary: "set container empty",
		args:    []string{"container="},
	}, {
		summary: "set container to none",
		args:    []string{"container=none"},
	}, {
		summary: "set container lxc",
		args:    []string{"container=lxc"},
	}, {
		summary: "set nonsense container",
		args:    []string{"container=foo"},
		err:     `bad "container" constraint: invalid container type "foo"`,
	}, {
		summary: "double set container together",
		args:    []string{"container=lxc container=lxc"},
		err:     `bad "container" constraint: already set`,
	}, {
		summary: "double set container separately",
		args:    []string{"container=lxc", "container="},
		err:     `bad "container" constraint: already set`,
	},

	// "arch" in detail.
	{
		summary: "set arch empty",
		args:    []string{"arch="},
	}, {
		summary: "set arch amd64",
		args:    []string{"arch=amd64"},
	}, {
		summary: "set arch i386",
		args:    []string{"arch=i386"},
	}, {
		summary: "set arch armhf",
		args:    []string{"arch=armhf"},
	}, {
		summary: "set nonsense arch 1",
		args:    []string{"arch=cheese"},
		err:     `bad "arch" constraint: "cheese" not recognized`,
	}, {
		summary: "set nonsense arch 2",
		args:    []string{"arch=123.45"},
		err:     `bad "arch" constraint: "123.45" not recognized`,
	}, {
		summary: "double set arch together",
		args:    []string{"arch=amd64 arch=amd64"},
		err:     `bad "arch" constraint: already set`,
	}, {
		summary: "double set arch separately",
		args:    []string{"arch=armhf", "arch="},
		err:     `bad "arch" constraint: already set`,
	},

	// "cpu-cores" in detail.
	{
		summary: "set cpu-cores empty",
		args:    []string{"cpu-cores="},
	}, {
		summary: "set cpu-cores zero",
		args:    []string{"cpu-cores=0"},
	}, {
		summary: "set cpu-cores",
		args:    []string{"cpu-cores=4"},
	}, {
		summary: "set nonsense cpu-cores 1",
		args:    []string{"cpu-cores=cheese"},
		err:     `bad "cpu-cores" constraint: must be a non-negative integer`,
	}, {
		summary: "set nonsense cpu-cores 2",
		args:    []string{"cpu-cores=-1"},
		err:     `bad "cpu-cores" constraint: must be a non-negative integer`,
	}, {
		summary: "set nonsense cpu-cores 3",
		args:    []string{"cpu-cores=123.45"},
		err:     `bad "cpu-cores" constraint: must be a non-negative integer`,
	}, {
		summary: "double set cpu-cores together",
		args:    []string{"cpu-cores=128 cpu-cores=1"},
		err:     `bad "cpu-cores" constraint: already set`,
	}, {
		summary: "double set cpu-cores separately",
		args:    []string{"cpu-cores=128", "cpu-cores=1"},
		err:     `bad "cpu-cores" constraint: already set`,
	},

	// "cpu-power" in detail.
	{
		summary: "set cpu-power empty",
		args:    []string{"cpu-power="},
	}, {
		summary: "set cpu-power zero",
		args:    []string{"cpu-power=0"},
	}, {
		summary: "set cpu-power",
		args:    []string{"cpu-power=44"},
	}, {
		summary: "set nonsense cpu-power 1",
		args:    []string{"cpu-power=cheese"},
		err:     `bad "cpu-power" constraint: must be a non-negative integer`,
	}, {
		summary: "set nonsense cpu-power 2",
		args:    []string{"cpu-power=-1"},
		err:     `bad "cpu-power" constraint: must be a non-negative integer`,
	}, {
		summary: "double set cpu-power together",
		args:    []string{"  cpu-power=300 cpu-power=1700 "},
		err:     `bad "cpu-power" constraint: already set`,
	}, {
		summary: "double set cpu-power separately",
		args:    []string{"cpu-power=300  ", "  cpu-power=1700"},
		err:     `bad "cpu-power" constraint: already set`,
	},

	// "mem" in detail.
	{
		summary: "set mem empty",
		args:    []string{"mem="},
	}, {
		summary: "set mem zero",
		args:    []string{"mem=0"},
	}, {
		summary: "set mem without suffix",
		args:    []string{"mem=512"},
	}, {
		summary: "set mem with M suffix",
		args:    []string{"mem=512M"},
	}, {
		summary: "set mem with G suffix",
		args:    []string{"mem=1.5G"},
	}, {
		summary: "set mem with T suffix",
		args:    []string{"mem=36.2T"},
	}, {
		summary: "set mem with P suffix",
		args:    []string{"mem=18.9P"},
	}, {
		summary: "set nonsense mem 1",
		args:    []string{"mem=cheese"},
		err:     `bad "mem" constraint: must be a non-negative float with optional M/G/T/P suffix`,
	}, {
		summary: "set nonsense mem 2",
		args:    []string{"mem=-1"},
		err:     `bad "mem" constraint: must be a non-negative float with optional M/G/T/P suffix`,
	}, {
		summary: "set nonsense mem 3",
		args:    []string{"mem=32Y"},
		err:     `bad "mem" constraint: must be a non-negative float with optional M/G/T/P suffix`,
	}, {
		summary: "double set mem together",
		args:    []string{"mem=1G  mem=2G"},
		err:     `bad "mem" constraint: already set`,
	}, {
		summary: "double set mem separately",
		args:    []string{"mem=1G", "mem=2G"},
		err:     `bad "mem" constraint: already set`,
	},

	// "root-disk" in detail.
	{
		summary: "set root-disk empty",
		args:    []string{"root-disk="},
	}, {
		summary: "set root-disk zero",
		args:    []string{"root-disk=0"},
	}, {
		summary: "set root-disk without suffix",
		args:    []string{"root-disk=512"},
	}, {
		summary: "set root-disk with M suffix",
		args:    []string{"root-disk=512M"},
	}, {
		summary: "set root-disk with G suffix",
		args:    []string{"root-disk=1.5G"},
	}, {
		summary: "set root-disk with T suffix",
		args:    []string{"root-disk=36.2T"},
	}, {
		summary: "set root-disk with P suffix",
		args:    []string{"root-disk=18.9P"},
	}, {
		summary: "set nonsense root-disk 1",
		args:    []string{"root-disk=cheese"},
		err:     `bad "root-disk" constraint: must be a non-negative float with optional M/G/T/P suffix`,
	}, {
		summary: "set nonsense root-disk 2",
		args:    []string{"root-disk=-1"},
		err:     `bad "root-disk" constraint: must be a non-negative float with optional M/G/T/P suffix`,
	}, {
		summary: "set nonsense root-disk 3",
		args:    []string{"root-disk=32Y"},
		err:     `bad "root-disk" constraint: must be a non-negative float with optional M/G/T/P suffix`,
	}, {
		summary: "double set root-disk together",
		args:    []string{"root-disk=1G  root-disk=2G"},
		err:     `bad "root-disk" constraint: already set`,
	}, {
		summary: "double set root-disk separately",
		args:    []string{"root-disk=1G", "root-disk=2G"},
		err:     `bad "root-disk" constraint: already set`,
	},

	// tags
	{
		summary: "single tag",
		args:    []string{"tags=foo"},
	}, {
		summary: "multiple tags",
		args:    []string{"tags=foo,bar"},
	}, {
		summary: "no tags",
		args:    []string{"tags="},
	},

	// spaces
	{
		summary: "single space",
		args:    []string{"spaces=space1"},
	}, {
		summary: "multiple spaces - positive",
		args:    []string{"spaces=space1,space2"},
	}, {
		summary: "multiple spaces - negative",
		args:    []string{"spaces=^dmz,^public"},
	}, {
		summary: "multiple spaces - positive and negative",
		args:    []string{"spaces=admin,^area52,dmz,^public"},
	}, {
		summary: "no spaces",
		args:    []string{"spaces="},
	},

	// networks
	{
		summary: "single network",
		args:    []string{"networks=net1"},
	}, {
		summary: "multiple networks - positive",
		args:    []string{"networks=net1,net2"},
	}, {
		summary: "multiple networks - negative",
		args:    []string{"networks=^net1,^net2"},
	}, {
		summary: "multiple networks - positive and negative",
		args:    []string{"networks=net1,^net2,net3,^net4"},
	}, {
		summary: "no networks",
		args:    []string{"networks="},
	},

	// instance type
	{
		summary: "set instance type",
		args:    []string{"instance-type=foo"},
	}, {
		summary: "instance type empty",
		args:    []string{"instance-type="},
	},

	// Everything at once.
	{
		summary: "kitchen sink together",
		args: []string{
			"root-disk=8G mem=2T  arch=i386  cpu-cores=4096 cpu-power=9001 container=lxc " +
				"tags=foo,bar spaces=space1,^space2 networks=net,^net2 instance-type=foo"},
	}, {
		summary: "kitchen sink separately",
		args: []string{
			"root-disk=8G", "mem=2T", "cpu-cores=4096", "cpu-power=9001", "arch=armhf",
			"container=lxc", "tags=foo,bar", "spaces=space1,^space2", "networks=net1,^net2",
			"instance-type=foo"},
	},
}

func (s *ConstraintsSuite) TestParseConstraints(c *gc.C) {
	// TODO(dimitern): This test is inadequate and needs to check for
	// more than just the reparsed output of String() matches the
	// expected.
	for i, t := range parseConstraintsTests {
		c.Logf("test %d: %s", i, t.summary)
		cons0, err := constraints.Parse(t.args...)
		if t.err == "" {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, t.err)
			continue
		}
		cons1, err := constraints.Parse(cons0.String())
		c.Check(err, jc.ErrorIsNil)
		c.Check(cons1, gc.DeepEquals, cons0)
	}
}

func (s *ConstraintsSuite) TestMerge(c *gc.C) {
	con1 := constraints.MustParse("arch=amd64 mem=4G")
	con2 := constraints.MustParse("cpu-cores=42")
	con3 := constraints.MustParse(
		"root-disk=8G container=lxc spaces=space1,^space2 networks=net1,^net2",
	)
	merged, err := constraints.Merge(con1, con2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(merged, jc.DeepEquals, constraints.MustParse("arch=amd64 mem=4G cpu-cores=42"))
	merged, err = constraints.Merge(con1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(merged, jc.DeepEquals, con1)
	merged, err = constraints.Merge(con1, con2, con3)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(merged, jc.DeepEquals, constraints.
		MustParse(
		"arch=amd64 mem=4G cpu-cores=42 root-disk=8G container=lxc spaces=space1,^space2 networks=net1,^net2"),
	)
	merged, err = constraints.Merge()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(merged, jc.DeepEquals, constraints.Value{})
	foo := "foo"
	merged, err = constraints.Merge(constraints.Value{Arch: &foo}, con2)
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, `bad "arch" constraint: "foo" not recognized`)
	c.Assert(merged, jc.DeepEquals, constraints.Value{})
	merged, err = constraints.Merge(con1, con1)
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, `bad "arch" constraint: already set`)
	c.Assert(merged, jc.DeepEquals, constraints.Value{})
}

func (s *ConstraintsSuite) TestParseMissingTagsSpacesAndNetworks(c *gc.C) {
	con := constraints.MustParse("arch=amd64 mem=4G cpu-cores=1 root-disk=8G")
	c.Check(con.Tags, gc.IsNil)
	c.Check(con.Spaces, gc.IsNil)
	c.Check(con.Networks, gc.IsNil)
}

func (s *ConstraintsSuite) TestParseNoTagsNoSpacesNoNetworks(c *gc.C) {
	con := constraints.MustParse(
		"arch=amd64 mem=4G cpu-cores=1 root-disk=8G tags= spaces= networks=",
	)
	c.Assert(con.Tags, gc.Not(gc.IsNil))
	c.Assert(con.Spaces, gc.Not(gc.IsNil))
	c.Check(*con.Tags, gc.HasLen, 0)
	c.Check(*con.Spaces, gc.HasLen, 0)
	c.Check(*con.Networks, gc.HasLen, 0)
}

func (s *ConstraintsSuite) TestIncludeExcludeAndHaveSpaces(c *gc.C) {
	con := constraints.MustParse("spaces=space1,^space2,space3,^space4")
	c.Assert(con.Spaces, gc.Not(gc.IsNil))
	c.Check(*con.Spaces, gc.HasLen, 4)
	c.Check(con.IncludeSpaces(), jc.SameContents, []string{"space1", "space3"})
	c.Check(con.ExcludeSpaces(), jc.SameContents, []string{"space2", "space4"})
	c.Check(con.HaveSpaces(), jc.IsTrue)
	c.Check(con.HaveNetworks(), jc.IsFalse)
	con = constraints.MustParse("mem=4G")
	c.Check(con.HaveSpaces(), jc.IsFalse)
	con = constraints.MustParse("mem=4G spaces=space-foo,^space-bar")
	c.Check(con.IncludeSpaces(), jc.SameContents, []string{"space-foo"})
	c.Check(con.ExcludeSpaces(), jc.SameContents, []string{"space-bar"})
	c.Check(con.HaveSpaces(), jc.IsTrue)
	c.Check(con.HaveNetworks(), jc.IsFalse)
}

func (s *ConstraintsSuite) TestIncludeExcludeAndHaveNetworks(c *gc.C) {
	con := constraints.MustParse("networks=net1,^net2,net3,^net4")
	c.Assert(con.Networks, gc.Not(gc.IsNil))
	c.Check(*con.Networks, gc.HasLen, 4)
	c.Check(con.IncludeNetworks(), jc.SameContents, []string{"net1", "net3"})
	c.Check(con.ExcludeNetworks(), jc.SameContents, []string{"net2", "net4"})
	c.Check(con.HaveNetworks(), jc.IsTrue)
	c.Check(con.HaveSpaces(), jc.IsFalse)
	con = constraints.MustParse("mem=4G")
	c.Check(con.HaveNetworks(), jc.IsFalse)
	con = constraints.MustParse("mem=4G networks=net-foo,^net-bar")
	c.Check(con.IncludeNetworks(), jc.SameContents, []string{"net-foo"})
	c.Check(con.ExcludeNetworks(), jc.SameContents, []string{"net-bar"})
	c.Check(con.HaveNetworks(), jc.IsTrue)
	c.Check(con.HaveSpaces(), jc.IsFalse)
}

func (s *ConstraintsSuite) TestInvalidSpaces(c *gc.C) {
	invalidNames := []string{
		"%$pace", "^foo#2", "+", "tcp:ip",
		"^^myspace", "^^^^^^^^", "space^x",
		"&-foo", "space/3", "^bar=4", "&#!",
	}
	for _, name := range invalidNames {
		con, err := constraints.Parse("spaces=" + name)
		expectName := strings.TrimPrefix(name, "^")
		expectErr := fmt.Sprintf(`bad "spaces" constraint: %q is not a valid space name`, expectName)
		c.Check(err, gc.NotNil)
		c.Check(err.Error(), gc.Equals, expectErr)
		c.Check(con, jc.DeepEquals, constraints.Value{})
	}
}

func (s *ConstraintsSuite) TestInvalidNetworks(c *gc.C) {
	invalidNames := []string{
		"%ne$t", "^net#2", "+", "tcp:ip",
		"^^mynet", "^^^^^^^^", "net^x",
		"&-foo", "net/3", "^net=4", "&#!",
	}
	for _, name := range invalidNames {
		con, err := constraints.Parse("networks=" + name)
		expectName := strings.TrimPrefix(name, "^")
		expectErr := fmt.Sprintf(`bad "networks" constraint: %q is not a valid network name`, expectName)
		c.Check(err, gc.NotNil)
		c.Check(err.Error(), gc.Equals, expectErr)
		c.Check(con, jc.DeepEquals, constraints.Value{})
	}
}

func (s *ConstraintsSuite) TestIsEmpty(c *gc.C) {
	con := constraints.Value{}
	c.Check(&con, jc.Satisfies, constraints.IsEmpty)
	con = constraints.MustParse("arch=amd64")
	c.Check(&con, gc.Not(jc.Satisfies), constraints.IsEmpty)
	con = constraints.MustParse("")
	c.Check(&con, jc.Satisfies, constraints.IsEmpty)
	con = constraints.MustParse("tags=")
	c.Check(&con, gc.Not(jc.Satisfies), constraints.IsEmpty)
	con = constraints.MustParse("spaces=")
	c.Check(&con, gc.Not(jc.Satisfies), constraints.IsEmpty)
	con = constraints.MustParse("networks=")
	c.Check(&con, gc.Not(jc.Satisfies), constraints.IsEmpty)
	con = constraints.MustParse("mem=")
	c.Check(&con, gc.Not(jc.Satisfies), constraints.IsEmpty)
	con = constraints.MustParse("arch=")
	c.Check(&con, gc.Not(jc.Satisfies), constraints.IsEmpty)
	con = constraints.MustParse("root-disk=")
	c.Check(&con, gc.Not(jc.Satisfies), constraints.IsEmpty)
	con = constraints.MustParse("cpu-power=")
	c.Check(&con, gc.Not(jc.Satisfies), constraints.IsEmpty)
	con = constraints.MustParse("cpu-cores=")
	c.Check(&con, gc.Not(jc.Satisfies), constraints.IsEmpty)
	con = constraints.MustParse("container=")
	c.Check(&con, gc.Not(jc.Satisfies), constraints.IsEmpty)
	con = constraints.MustParse("instance-type=")
	c.Check(&con, gc.Not(jc.Satisfies), constraints.IsEmpty)
}

func uint64p(i uint64) *uint64 {
	return &i
}

func strp(s string) *string {
	return &s
}

func ctypep(ctype string) *instance.ContainerType {
	res := instance.ContainerType(ctype)
	return &res
}

type roundTrip struct {
	Name  string
	Value constraints.Value
}

var constraintsRoundtripTests = []roundTrip{
	{"empty", constraints.Value{}},
	{"Arch1", constraints.Value{Arch: strp("")}},
	{"Arch2", constraints.Value{Arch: strp("amd64")}},
	{"Container1", constraints.Value{Container: ctypep("")}},
	{"Container2", constraints.Value{Container: ctypep("lxc")}},
	{"Container3", constraints.Value{Container: nil}},
	{"CpuCores1", constraints.Value{CpuCores: nil}},
	{"CpuCores2", constraints.Value{CpuCores: uint64p(0)}},
	{"CpuCores3", constraints.Value{CpuCores: uint64p(128)}},
	{"CpuPower1", constraints.Value{CpuPower: nil}},
	{"CpuPower2", constraints.Value{CpuPower: uint64p(0)}},
	{"CpuPower3", constraints.Value{CpuPower: uint64p(250)}},
	{"Mem1", constraints.Value{Mem: nil}},
	{"Mem2", constraints.Value{Mem: uint64p(0)}},
	{"Mem3", constraints.Value{Mem: uint64p(98765)}},
	{"RootDisk1", constraints.Value{RootDisk: nil}},
	{"RootDisk2", constraints.Value{RootDisk: uint64p(0)}},
	{"RootDisk2", constraints.Value{RootDisk: uint64p(109876)}},
	{"Tags1", constraints.Value{Tags: nil}},
	{"Tags2", constraints.Value{Tags: &[]string{}}},
	{"Tags3", constraints.Value{Tags: &[]string{"foo", "bar"}}},
	{"Spaces1", constraints.Value{Spaces: nil}},
	{"Spaces2", constraints.Value{Spaces: &[]string{}}},
	{"Spaces3", constraints.Value{Spaces: &[]string{"space1", "^space2"}}},
	{"Networks1", constraints.Value{Networks: nil}},
	{"Networks2", constraints.Value{Networks: &[]string{}}},
	{"Networks3", constraints.Value{Networks: &[]string{"net1", "^net2"}}},
	{"InstanceType1", constraints.Value{InstanceType: strp("")}},
	{"InstanceType2", constraints.Value{InstanceType: strp("foo")}},
	{"All", constraints.Value{
		Arch:         strp("i386"),
		Container:    ctypep("lxc"),
		CpuCores:     uint64p(4096),
		CpuPower:     uint64p(9001),
		Mem:          uint64p(18000000000),
		RootDisk:     uint64p(24000000000),
		Tags:         &[]string{"foo", "bar"},
		Spaces:       &[]string{"space1", "^space2"},
		Networks:     &[]string{"net1", "^net2"},
		InstanceType: strp("foo"),
	}},
}

func (s *ConstraintsSuite) TestRoundtripGnuflagValue(c *gc.C) {
	for _, t := range constraintsRoundtripTests {
		c.Logf("test %s", t.Name)
		var cons constraints.Value
		val := constraints.ConstraintsValue{&cons}
		err := val.Set(t.Value.String())
		c.Check(err, jc.ErrorIsNil)
		c.Check(cons, jc.DeepEquals, t.Value)
	}
}

func (s *ConstraintsSuite) TestRoundtripString(c *gc.C) {
	for _, t := range constraintsRoundtripTests {
		c.Logf("test %s", t.Name)
		cons, err := constraints.Parse(t.Value.String())
		c.Check(err, jc.ErrorIsNil)
		c.Check(cons, jc.DeepEquals, t.Value)
	}
}

func (s *ConstraintsSuite) TestRoundtripJson(c *gc.C) {
	for _, t := range constraintsRoundtripTests {
		c.Logf("test %s", t.Name)
		data, err := json.Marshal(t.Value)
		c.Assert(err, jc.ErrorIsNil)
		var cons constraints.Value
		err = json.Unmarshal(data, &cons)
		c.Check(err, jc.ErrorIsNil)
		c.Check(cons, jc.DeepEquals, t.Value)
	}
}

func (s *ConstraintsSuite) TestRoundtripYaml(c *gc.C) {
	for _, t := range constraintsRoundtripTests {
		c.Logf("test %s", t.Name)
		data, err := goyaml.Marshal(t.Value)
		c.Assert(err, jc.ErrorIsNil)
		var cons constraints.Value
		err = goyaml.Unmarshal(data, &cons)
		c.Check(err, jc.ErrorIsNil)
		c.Check(cons, jc.DeepEquals, t.Value)
	}
}

var hasContainerTests = []struct {
	constraints  string
	hasContainer bool
}{
	{
		hasContainer: false,
	}, {
		constraints:  "container=lxc",
		hasContainer: true,
	}, {
		constraints:  "container=none",
		hasContainer: false,
	},
}

func (s *ConstraintsSuite) TestHasContainer(c *gc.C) {
	for i, t := range hasContainerTests {
		c.Logf("test %d", i)
		cons := constraints.MustParse(t.constraints)
		c.Check(cons.HasContainer(), gc.Equals, t.hasContainer)
	}
}

func (s *ConstraintsSuite) TestHasInstanceType(c *gc.C) {
	cons := constraints.MustParse("arch=amd64")
	c.Check(cons.HasInstanceType(), jc.IsFalse)
	cons = constraints.MustParse("arch=amd64 instance-type=foo")
	c.Check(cons.HasInstanceType(), jc.IsTrue)
}

const initialWithoutCons = "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cpu-cores=4 spaces=space1,^space2 networks=net1,^net2 tags=foo container=lxc instance-type=bar"

var withoutTests = []struct {
	initial string
	without []string
	final   string
}{{
	initial: initialWithoutCons,
	without: []string{"root-disk"},
	final:   "mem=4G arch=amd64 cpu-power=1000 cpu-cores=4 tags=foo spaces=space1,^space2 networks=net1,^net2 container=lxc instance-type=bar",
}, {
	initial: initialWithoutCons,
	without: []string{"mem"},
	final:   "root-disk=8G arch=amd64 cpu-power=1000 cpu-cores=4 tags=foo spaces=space1,^space2 networks=net1,^net2 container=lxc instance-type=bar",
}, {
	initial: initialWithoutCons,
	without: []string{"arch"},
	final:   "root-disk=8G mem=4G cpu-power=1000 cpu-cores=4 tags=foo spaces=space1,^space2 networks=net1,^net2 container=lxc instance-type=bar",
}, {
	initial: initialWithoutCons,
	without: []string{"cpu-power"},
	final:   "root-disk=8G mem=4G arch=amd64 cpu-cores=4 tags=foo spaces=space1,^space2 networks=net1,^net2 container=lxc instance-type=bar",
}, {
	initial: initialWithoutCons,
	without: []string{"cpu-cores"},
	final:   "root-disk=8G mem=4G arch=amd64 cpu-power=1000 tags=foo spaces=space1,^space2 networks=net1,^net2 container=lxc instance-type=bar",
}, {
	initial: initialWithoutCons,
	without: []string{"tags"},
	final:   "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cpu-cores=4 spaces=space1,^space2 networks=net1,^net2 container=lxc instance-type=bar",
}, {
	initial: initialWithoutCons,
	without: []string{"spaces"},
	final:   "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cpu-cores=4 tags=foo networks=net1,^net2 container=lxc instance-type=bar",
}, {
	initial: initialWithoutCons,
	without: []string{"networks"},
	final:   "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cpu-cores=4 tags=foo spaces=space1,^space2 container=lxc instance-type=bar",
}, {
	initial: initialWithoutCons,
	without: []string{"container"},
	final:   "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cpu-cores=4 tags=foo spaces=space1,^space2 networks=net1,^net2 instance-type=bar",
}, {
	initial: initialWithoutCons,
	without: []string{"instance-type"},
	final:   "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cpu-cores=4 tags=foo spaces=space1,^space2 container=lxc networks=net1,^net2",
}, {
	initial: initialWithoutCons,
	without: []string{"root-disk", "mem", "arch"},
	final:   "cpu-power=1000 cpu-cores=4 tags=foo spaces=space1,^space2 networks=net1,^net2 container=lxc instance-type=bar",
}}

func (s *ConstraintsSuite) TestWithout(c *gc.C) {
	for i, t := range withoutTests {
		c.Logf("test %d", i)
		initial := constraints.MustParse(t.initial)
		final, err := constraints.Without(initial, t.without...)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(final, jc.DeepEquals, constraints.MustParse(t.final))
	}
}

func (s *ConstraintsSuite) TestWithoutError(c *gc.C) {
	cons := constraints.MustParse("mem=4G")
	_, err := constraints.Without(cons, "foo")
	c.Assert(err, gc.ErrorMatches, `unknown constraint "foo"`)
}

func (s *ConstraintsSuite) TestAttributesWithValues(c *gc.C) {
	for i, consStr := range []string{
		"",
		"root-disk=8G mem=4G arch=amd64 cpu-power=1000 cpu-cores=4 instance-type=foo tags=foo,bar spaces=space1,^space2",
	} {
		c.Logf("test %d", i)
		cons := constraints.MustParse(consStr)
		obtained := constraints.AttributesWithValues(cons)
		assertMissing := func(attrName string) {
			_, ok := obtained[attrName]
			c.Check(ok, jc.IsFalse)
		}
		if cons.Arch != nil {
			c.Check(obtained["arch"], gc.Equals, *cons.Arch)
		} else {
			assertMissing("arch")
		}
		if cons.Mem != nil {
			c.Check(obtained["mem"], gc.Equals, *cons.Mem)
		} else {
			assertMissing("mem")
		}
		if cons.CpuCores != nil {
			c.Check(obtained["cpu-cores"], gc.Equals, *cons.CpuCores)
		} else {
			assertMissing("cpu-cores")
		}
		if cons.CpuPower != nil {
			c.Check(obtained["cpu-power"], gc.Equals, *cons.CpuPower)
		} else {
			assertMissing("cpu-power")
		}
		if cons.RootDisk != nil {
			c.Check(obtained["root-disk"], gc.Equals, *cons.RootDisk)
		} else {
			assertMissing("root-disk")
		}
		if cons.Tags != nil {
			c.Check(obtained["tags"], gc.DeepEquals, *cons.Tags)
		} else {
			assertMissing("tags")
		}
		if cons.Spaces != nil {
			c.Check(obtained["spaces"], gc.DeepEquals, *cons.Spaces)
		} else {
			assertMissing("spaces")
		}
		if cons.Networks != nil {
			c.Check(obtained["networks"], gc.DeepEquals, *cons.Networks)
		} else {
			assertMissing("networks")
		}
		if cons.InstanceType != nil {
			c.Check(obtained["instance-type"], gc.Equals, *cons.InstanceType)
		} else {
			assertMissing("instance-type")
		}
	}
}

var hasAnyTests = []struct {
	cons     string
	attrs    []string
	expected []string
}{
	{
		cons:     "root-disk=8G mem=4G arch=amd64 cpu-power=1000 spaces=space1,^space2 cpu-cores=4",
		attrs:    []string{"root-disk", "tags", "mem", "spaces"},
		expected: []string{"root-disk", "mem", "spaces"},
	},
	{
		cons:     "root-disk=8G mem=4G arch=amd64 cpu-power=1000 networks=net1,^net2 cpu-cores=4",
		attrs:    []string{"root-disk", "tags", "mem", "networks"},
		expected: []string{"root-disk", "mem", "networks"},
	},
	{
		cons:     "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cpu-cores=4",
		attrs:    []string{"tags", "spaces", "networks"},
		expected: []string{},
	},
}

func (s *ConstraintsSuite) TestHasAny(c *gc.C) {
	for i, t := range hasAnyTests {
		c.Logf("test %d", i)
		cons := constraints.MustParse(t.cons)
		obtained := constraints.HasAny(cons, t.attrs...)
		c.Check(obtained, jc.DeepEquals, t.expected)
	}
}
