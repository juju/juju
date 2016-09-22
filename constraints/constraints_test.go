// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constraints_test

import (
	"encoding/json"
	"fmt"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
)

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
		summary: "set container lxd",
		args:    []string{"container=lxd"},
	}, {
		summary: "set nonsense container",
		args:    []string{"container=foo"},
		err:     `bad "container" constraint: invalid container type "foo"`,
	}, {
		summary: "double set container together",
		args:    []string{"container=lxd container=lxd"},
		err:     `bad "container" constraint: already set`,
	}, {
		summary: "double set container separately",
		args:    []string{"container=lxd", "container="},
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

	// "cores" in detail.
	{
		summary: "set cores empty",
		args:    []string{"cores="},
	}, {
		summary: "set cores zero",
		args:    []string{"cores=0"},
	}, {
		summary: "set cores",
		args:    []string{"cores=4"},
	}, {
		summary: "set nonsense cores 1",
		args:    []string{"cores=cheese"},
		err:     `bad "cores" constraint: must be a non-negative integer`,
	}, {
		summary: "set nonsense cores 2",
		args:    []string{"cores=-1"},
		err:     `bad "cores" constraint: must be a non-negative integer`,
	}, {
		summary: "set nonsense cores 3",
		args:    []string{"cores=123.45"},
		err:     `bad "cores" constraint: must be a non-negative integer`,
	}, {
		summary: "double set cores together",
		args:    []string{"cores=128 cores=1"},
		err:     `bad "cores" constraint: already set`,
	}, {
		summary: "double set cores separately",
		args:    []string{"cores=128", "cores=1"},
		err:     `bad "cores" constraint: already set`,
	},

	// "cpu-cores"
	{
		summary: "set cpu-cores",
		args:    []string{"cpu-cores=4"},
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

	// instance type
	{
		summary: "set instance type",
		args:    []string{"instance-type=foo"},
	}, {
		summary: "instance type empty",
		args:    []string{"instance-type="},
	},

	// "virt-type" in detail.
	{
		summary: "set virt-type empty",
		args:    []string{"virt-type="},
	}, {
		summary: "set virt-type kvm",
		args:    []string{"virt-type=kvm"},
	}, {
		summary: "set virt-type lxd",
		args:    []string{"virt-type=lxd"},
	}, {
		summary: "double set virt-type together",
		args:    []string{"virt-type=kvm virt-type=kvm"},
		err:     `bad "virt-type" constraint: already set`,
	}, {
		summary: "double set virt-type separately",
		args:    []string{"virt-type=kvm", "virt-type="},
		err:     `bad "virt-type" constraint: already set`,
	},

	// Everything at once.
	{
		summary: "kitchen sink together",
		args: []string{
			"root-disk=8G mem=2T  arch=i386  cores=4096 cpu-power=9001 container=lxd " +
				"tags=foo,bar spaces=space1,^space2 instance-type=foo",
			"virt-type=kvm"},
	}, {
		summary: "kitchen sink separately",
		args: []string{
			"root-disk=8G", "mem=2T", "cores=4096", "cpu-power=9001", "arch=armhf",
			"container=lxd", "tags=foo,bar", "spaces=space1,^space2",
			"instance-type=foo", "virt-type=kvm"},
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

func (s *ConstraintsSuite) TestParseAliases(c *gc.C) {
	v, aliases, err := constraints.ParseWithAliases("cpu-cores=5 arch=amd64")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(v, gc.DeepEquals, constraints.Value{
		CpuCores: uint64p(5),
		Arch:     strp("amd64"),
	})
	c.Assert(aliases, gc.DeepEquals, map[string]string{
		"cpu-cores": "cores",
	})
}

func (s *ConstraintsSuite) TestMerge(c *gc.C) {
	con1 := constraints.MustParse("arch=amd64 mem=4G")
	con2 := constraints.MustParse("cores=42")
	con3 := constraints.MustParse(
		"root-disk=8G container=lxd spaces=space1,^space2",
	)
	merged, err := constraints.Merge(con1, con2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(merged, jc.DeepEquals, constraints.MustParse("arch=amd64 mem=4G cores=42"))
	merged, err = constraints.Merge(con1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(merged, jc.DeepEquals, con1)
	merged, err = constraints.Merge(con1, con2, con3)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(merged, jc.DeepEquals, constraints.
		MustParse("arch=amd64 mem=4G cores=42 root-disk=8G container=lxd spaces=space1,^space2"),
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

func (s *ConstraintsSuite) TestParseMissingTagsAndSpaces(c *gc.C) {
	con := constraints.MustParse("arch=amd64 mem=4G cores=1 root-disk=8G")
	c.Check(con.Tags, gc.IsNil)
	c.Check(con.Spaces, gc.IsNil)
}

func (s *ConstraintsSuite) TestParseNoTagsNoSpaces(c *gc.C) {
	con := constraints.MustParse(
		"arch=amd64 mem=4G cores=1 root-disk=8G tags= spaces=",
	)
	c.Assert(con.Tags, gc.Not(gc.IsNil))
	c.Assert(con.Spaces, gc.Not(gc.IsNil))
	c.Check(*con.Tags, gc.HasLen, 0)
	c.Check(*con.Spaces, gc.HasLen, 0)
}

func (s *ConstraintsSuite) TestIncludeExcludeAndHaveSpaces(c *gc.C) {
	con := constraints.MustParse("spaces=space1,^space2,space3,^space4")
	c.Assert(con.Spaces, gc.Not(gc.IsNil))
	c.Check(*con.Spaces, gc.HasLen, 4)
	c.Check(con.IncludeSpaces(), jc.SameContents, []string{"space1", "space3"})
	c.Check(con.ExcludeSpaces(), jc.SameContents, []string{"space2", "space4"})
	c.Check(con.HaveSpaces(), jc.IsTrue)
	con = constraints.MustParse("mem=4G")
	c.Check(con.HaveSpaces(), jc.IsFalse)
	con = constraints.MustParse("mem=4G spaces=space-foo,^space-bar")
	c.Check(con.IncludeSpaces(), jc.SameContents, []string{"space-foo"})
	c.Check(con.ExcludeSpaces(), jc.SameContents, []string{"space-bar"})
	c.Check(con.HaveSpaces(), jc.IsTrue)
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
	con = constraints.MustParse("mem=")
	c.Check(&con, gc.Not(jc.Satisfies), constraints.IsEmpty)
	con = constraints.MustParse("arch=")
	c.Check(&con, gc.Not(jc.Satisfies), constraints.IsEmpty)
	con = constraints.MustParse("root-disk=")
	c.Check(&con, gc.Not(jc.Satisfies), constraints.IsEmpty)
	con = constraints.MustParse("cpu-power=")
	c.Check(&con, gc.Not(jc.Satisfies), constraints.IsEmpty)
	con = constraints.MustParse("cores=")
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
	{"Container2", constraints.Value{Container: ctypep("lxd")}},
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
	{"InstanceType1", constraints.Value{InstanceType: strp("")}},
	{"InstanceType2", constraints.Value{InstanceType: strp("foo")}},
	{"All", constraints.Value{
		Arch:         strp("i386"),
		Container:    ctypep("lxd"),
		CpuCores:     uint64p(4096),
		CpuPower:     uint64p(9001),
		Mem:          uint64p(18000000000),
		RootDisk:     uint64p(24000000000),
		Tags:         &[]string{"foo", "bar"},
		Spaces:       &[]string{"space1", "^space2"},
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
		constraints:  "container=lxd",
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

const initialWithoutCons = "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cores=4 spaces=space1,^space2 tags=foo container=lxd instance-type=bar"

var withoutTests = []struct {
	initial string
	without []string
	final   string
}{{
	initial: initialWithoutCons,
	without: []string{"root-disk"},
	final:   "mem=4G arch=amd64 cpu-power=1000 cores=4 tags=foo spaces=space1,^space2 container=lxd instance-type=bar",
}, {
	initial: initialWithoutCons,
	without: []string{"mem"},
	final:   "root-disk=8G arch=amd64 cpu-power=1000 cores=4 tags=foo spaces=space1,^space2 container=lxd instance-type=bar",
}, {
	initial: initialWithoutCons,
	without: []string{"arch"},
	final:   "root-disk=8G mem=4G cpu-power=1000 cores=4 tags=foo spaces=space1,^space2 container=lxd instance-type=bar",
}, {
	initial: initialWithoutCons,
	without: []string{"cpu-power"},
	final:   "root-disk=8G mem=4G arch=amd64 cores=4 tags=foo spaces=space1,^space2 container=lxd instance-type=bar",
}, {
	initial: initialWithoutCons,
	without: []string{"cores"},
	final:   "root-disk=8G mem=4G arch=amd64 cpu-power=1000 tags=foo spaces=space1,^space2 container=lxd instance-type=bar",
}, {
	initial: initialWithoutCons,
	without: []string{"tags"},
	final:   "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cores=4 spaces=space1,^space2 container=lxd instance-type=bar",
}, {
	initial: initialWithoutCons,
	without: []string{"spaces"},
	final:   "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cores=4 tags=foo container=lxd instance-type=bar",
}, {
	initial: initialWithoutCons,
	without: []string{"container"},
	final:   "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cores=4 tags=foo spaces=space1,^space2 instance-type=bar",
}, {
	initial: initialWithoutCons,
	without: []string{"instance-type"},
	final:   "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cores=4 tags=foo spaces=space1,^space2 container=lxd",
}, {
	initial: initialWithoutCons,
	without: []string{"root-disk", "mem", "arch"},
	final:   "cpu-power=1000 cores=4 tags=foo spaces=space1,^space2 container=lxd instance-type=bar",
}}

func (s *ConstraintsSuite) TestWithout(c *gc.C) {
	for i, t := range withoutTests {
		c.Logf("test %d", i)
		initial := constraints.MustParse(t.initial)
		final := constraints.Without(initial, t.without...)
		c.Check(final, jc.DeepEquals, constraints.MustParse(t.final))
	}
}

var hasAnyTests = []struct {
	cons     string
	attrs    []string
	expected []string
}{
	{
		cons:     "root-disk=8G mem=4G arch=amd64 cpu-power=1000 spaces=space1,^space2 cores=4",
		attrs:    []string{"root-disk", "tags", "mem", "spaces"},
		expected: []string{"root-disk", "mem", "spaces"},
	},
	{
		cons:     "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cores=4",
		attrs:    []string{"root-disk", "tags", "mem"},
		expected: []string{"root-disk", "mem"},
	},
	{
		cons:     "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cores=4",
		attrs:    []string{"tags", "spaces"},
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
