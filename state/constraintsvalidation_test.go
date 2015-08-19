// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

type constraintsValidationSuite struct {
	ConnSuite
}

var _ = gc.Suite(&constraintsValidationSuite{})

func (s *constraintsValidationSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.policy.GetConstraintsValidator = func(*config.Config) (constraints.Validator, error) {
		validator := constraints.NewValidator()
		validator.RegisterConflicts(
			[]string{constraints.InstanceType},
			[]string{constraints.Mem, constraints.Arch},
		)
		validator.RegisterUnsupported([]string{constraints.CpuPower})
		return validator, nil
	}
}

func (s *constraintsValidationSuite) addOneMachine(c *gc.C, cons constraints.Value) (*state.Machine, error) {
	return s.State.AddOneMachine(state.MachineTemplate{
		Series:      "quantal",
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: cons,
	})
}

var setConstraintsTests = []struct {
	about        string
	consToSet    string
	consFallback string

	effectiveEnvironCons string // environment constraints after setting consFallback
	effectiveServiceCons string // service constraints after setting consToSet
	effectiveUnitCons    string // unit constraints after setting consToSet on the service
	effectiveMachineCons string // machine constraints after setting consToSet
}{{
	about:        "(implictly) empty constraints are OK and stored as empty",
	consToSet:    "",
	consFallback: "",

	effectiveEnvironCons: "",
	effectiveServiceCons: "",
	effectiveUnitCons:    "",
	effectiveMachineCons: "",
}, {
	about:        "(implicitly) empty fallback constraints never override set constraints",
	consToSet:    "instance-type=foo-42 cpu-power=9001 spaces=bar networks=net1,^net2",
	consFallback: "",

	effectiveEnvironCons: "", // environment constraints are stored as empty.
	// i.e. there are no fallbacks and all the following cases are the same.
	effectiveServiceCons: "instance-type=foo-42 cpu-power=9001 spaces=bar networks=net1,^net2",
	effectiveUnitCons:    "instance-type=foo-42 cpu-power=9001 spaces=bar networks=net1,^net2",
	effectiveMachineCons: "instance-type=foo-42 cpu-power=9001 spaces=bar networks=net1,^net2",
}, {
	about:        "(implicitly) empty constraints override set fallbacks except for services",
	consToSet:    "",
	consFallback: "arch=amd64 cpu-cores=42 mem=2G tags=foo networks=net1,^net2",

	effectiveEnvironCons: "arch=amd64 cpu-cores=42 mem=2G tags=foo networks=net1,^net2",
	effectiveServiceCons: "", // set as given and not merged with fallbacks like below
	effectiveUnitCons:    "arch=amd64 cpu-cores=42 mem=2G tags=foo networks=net1,^net2",
	// set as given, then merged with fallbacks; since it's empty, it
	// inherits everything from fallbacks; like the unit, but only
	// because the service constraints are also empty.
	effectiveMachineCons: "arch=amd64 cpu-cores=42 mem=2G tags=foo networks=net1,^net2",
}, {
	about:        "(explicitly) empty constraints are OK and stored as empty",
	consToSet:    "cpu-cores= cpu-power= root-disk= instance-type= container= tags= spaces= networks=",
	consFallback: "",
	// cons.MustParse("") != cons.MustParse("mem= arch="), but when we
	// store them in a state document, they are considered equal and
	// empty values are never stored or read back as non-empty.
	// Explicit empty values like "mem=" are only respected when
	// merging given constraints with environment fallbacks (causing
	// matching fallback values to be dropped).
	effectiveEnvironCons: "",
	effectiveServiceCons: "",
	effectiveUnitCons:    "",
	effectiveMachineCons: "",
}, {
	about:        "(explicitly) empty fallback constraints are OK and stored as empty",
	consToSet:    "",
	consFallback: "cpu-cores= cpu-power= root-disk= instance-type= container= tags= spaces= networks=",
	// same reason as above, however since the environment are only
	// used as fallbacks there effectively is no difference between
	// setting "mem=", "mem=0", or omitting it altogether.
	effectiveEnvironCons: "",
	effectiveServiceCons: "",
	effectiveUnitCons:    "",
	effectiveMachineCons: "",
}, {
	about:        "(explicitly) empty constraints and fallbacks are OK and stored as empty",
	consToSet:    "arch= mem= cpu-cores= container=",
	consFallback: "cpu-cores= cpu-power= root-disk= instance-type= container= tags= spaces= networks=",
	// even though the explicitly empty values override the fallbacks,
	// all are empty so result is the same when stored.
	effectiveEnvironCons: "",
	effectiveServiceCons: "",
	effectiveUnitCons:    "",
	effectiveMachineCons: "",
}, {
	about:        "(explicitly) empty constraints override set fallbacks for provisioning",
	consToSet:    "cpu-cores= arch= spaces= networks= cpu-power=",
	consFallback: "cpu-cores=42 arch=amd64 tags=foo spaces=default,^dmz mem=4G",

	effectiveEnvironCons: "cpu-cores=42 arch=amd64 tags=foo spaces=default,^dmz mem=4G",
	effectiveServiceCons: "", // stored as empty, so units will inherit just the fallbacks
	effectiveUnitCons:    "cpu-cores=42 arch=amd64 tags=foo spaces=default,^dmz mem=4G",
	// machine constraints have no empty service constraints to inherit here,
	// so they will override any matching fallbacks (tags, cpu-core, arch, spaces)
	// with an explicit value (empty or not), while any not set explicitly will be
	// inherited from then fallbacks only.
	effectiveMachineCons: "tags=foo mem=4G",
	// we're also checking if m.SetConstraints() does the same with
	// regards to the effective constraints as AddMachine(), because
	// some of these tests proved they had different behavior (i.e.
	// container= was not set to empty)
}, {
	about:        "non-empty constraints always override empty or unset fallbacks",
	consToSet:    "cpu-cores=42 root-disk=20G arch=amd64 tags=foo,bar networks=^dmz,db",
	consFallback: "cpu-cores= arch= tags=",

	effectiveEnvironCons: "", // like the other cases when environ constraints end up empty.
	effectiveServiceCons: "cpu-cores=42 root-disk=20G arch=amd64 tags=foo,bar networks=^dmz,db",
	effectiveUnitCons:    "cpu-cores=42 root-disk=20G arch=amd64 tags=foo,bar networks=^dmz,db",
	effectiveMachineCons: "cpu-cores=42 root-disk=20G arch=amd64 tags=foo,bar networks=^dmz,db",
}, {
	about:        "non-empty constraints always override set fallbacks",
	consToSet:    "cpu-cores=42 root-disk=20G arch=amd64 tags=foo,bar networks=^dmz,db",
	consFallback: "cpu-cores=12 root-disk=10G arch=i386  tags=bar networks=net1,^net2",

	effectiveEnvironCons: "cpu-cores=12 root-disk=10G arch=i386  tags=bar networks=net1,^net2",
	effectiveServiceCons: "cpu-cores=42 root-disk=20G arch=amd64 tags=foo,bar networks=^dmz,db",
	effectiveUnitCons:    "cpu-cores=42 root-disk=20G arch=amd64 tags=foo,bar networks=^dmz,db",
	effectiveMachineCons: "cpu-cores=42 root-disk=20G arch=amd64 tags=foo,bar networks=^dmz,db",
}, {
	about:        "non-empty constraints override conflicting set fallbacks",
	consToSet:    "mem=8G arch=amd64 cpu-cores=4 tags=bar",
	consFallback: "instance-type=small cpu-power=1000", // instance-type conflicts mem, arch

	effectiveEnvironCons: "instance-type=small cpu-power=1000",
	effectiveServiceCons: "mem=8G arch=amd64 cpu-cores=4 tags=bar",
	// both of the following contain the explicitly set constraints after
	// resolving any conflicts with fallbacks (by dropping them).
	effectiveUnitCons:    "mem=8G arch=amd64 cpu-cores=4 tags=bar cpu-power=1000",
	effectiveMachineCons: "mem=8G arch=amd64 cpu-cores=4 tags=bar cpu-power=1000",
}, {
	about:        "set fallbacks are overriden differently for provisioning and deployment",
	consToSet:    "networks= tags= cpu-power= spaces=bar",
	consFallback: "networks=net1,^net3 tags=foo cpu-power=42",
	// a variation of the above case showing the difference between
	// deployment (service, unit) and provisioning (machine) constraints.
	effectiveEnvironCons: "networks=net1,^net3 tags=foo cpu-power=42",
	effectiveServiceCons: "spaces=bar", // set as given, empty values are dropped
	// unit constraints are the effective service constraints from above,
	// merged with the fallbacks, overriding them.
	effectiveUnitCons: "networks=net1,^net3 tags=foo cpu-power=42 spaces=bar",
	// machine provisioning constraints as set as given (before ignoring
	// explicitly unset values) and then merged with fallbacks, overriding
	// them for all matching explicit values.
	effectiveMachineCons: "spaces=bar",
}, {
	about:        "container type can only be used for deployment, not provisioning",
	consToSet:    "container=kvm arch=amd64",
	consFallback: "container=lxc mem=8G",
	// service deployment constraints are transformed into machine
	// provisioning constraints, and the container type only makes
	// sense currently as a deployment constraint, it's cleared when
	// merging service/environment deployment constraints into
	// effective machine provisioning constraints.
	effectiveEnvironCons: "container=lxc mem=8G",
	effectiveServiceCons: "container=kvm arch=amd64",
	effectiveUnitCons:    "container=kvm mem=8G arch=amd64",
	effectiveMachineCons: "mem=8G arch=amd64",
}}

func (s *constraintsValidationSuite) TestMachineConstraints(c *gc.C) {
	for i, t := range setConstraintsTests {
		c.Logf(
			"test %d: %s\nconsToSet: %q\nconsFallback: %q\n",
			i, t.about, t.consToSet, t.consFallback,
		)
		// Set fallbacks as environment constraints and verify them.
		err := s.State.SetEnvironConstraints(constraints.MustParse(t.consFallback))
		c.Check(err, jc.ErrorIsNil)
		econs, err := s.State.EnvironConstraints()
		c.Check(econs, jc.DeepEquals, constraints.MustParse(t.effectiveEnvironCons))
		// Set the machine provisioning constraints.
		m, err := s.addOneMachine(c, constraints.MustParse(t.consToSet))
		c.Check(err, jc.ErrorIsNil)
		// New machine provisioning constraints get merged with the fallbacks.
		cons, err := m.Constraints()
		c.Check(err, jc.ErrorIsNil)
		c.Check(cons, jc.DeepEquals, constraints.MustParse(t.effectiveMachineCons))
		// Changing them should result in the same result before provisioning.
		err = m.SetConstraints(constraints.MustParse(t.consToSet))
		c.Check(err, jc.ErrorIsNil)
		cons, err = m.Constraints()
		c.Check(err, jc.ErrorIsNil)
		c.Check(cons, jc.DeepEquals, constraints.MustParse(t.effectiveMachineCons))
	}
}

func (s *constraintsValidationSuite) TestServiceConstraints(c *gc.C) {
	charm := s.AddTestingCharm(c, "wordpress")
	service := s.AddTestingService(c, "wordpress", charm)
	for i, t := range setConstraintsTests {
		c.Logf(
			"test %d: %s\nconsToSet: %q\nconsFallback: %q\n",
			i, t.about, t.consToSet, t.consFallback,
		)
		// Set fallbacks as environment constraints and verify them.
		err := s.State.SetEnvironConstraints(constraints.MustParse(t.consFallback))
		c.Check(err, jc.ErrorIsNil)
		econs, err := s.State.EnvironConstraints()
		c.Check(econs, jc.DeepEquals, constraints.MustParse(t.effectiveEnvironCons))
		// Set the service deployment constraints.
		err = service.SetConstraints(constraints.MustParse(t.consToSet))
		c.Check(err, jc.ErrorIsNil)
		u, err := service.AddUnit()
		c.Check(err, jc.ErrorIsNil)
		// New unit deployment constraints get merged with the fallbacks.
		ucons, err := u.Constraints()
		c.Check(err, jc.ErrorIsNil)
		c.Check(*ucons, jc.DeepEquals, constraints.MustParse(t.effectiveUnitCons))
		// Service constraints remain as set.
		scons, err := service.Constraints()
		c.Check(err, jc.ErrorIsNil)
		c.Check(scons, jc.DeepEquals, constraints.MustParse(t.effectiveServiceCons))
	}
}
