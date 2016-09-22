// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"regexp"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/state"
)

type applicationConstraintsSuite struct {
	ConnSuite

	applicationName string
	testCharm       *state.Charm
}

var _ = gc.Suite(&applicationConstraintsSuite{})

type constraintsValidationSuite struct {
	ConnSuite
}

var _ = gc.Suite(&constraintsValidationSuite{})

func (s *constraintsValidationSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.policy.GetConstraintsValidator = func() (constraints.Validator, error) {
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

	effectiveModelCons   string // model constraints after setting consFallback
	effectiveServiceCons string // service constraints after setting consToSet
	effectiveUnitCons    string // unit constraints after setting consToSet on the service
	effectiveMachineCons string // machine constraints after setting consToSet
}{{
	about:        "(implictly) empty constraints are OK and stored as empty",
	consToSet:    "",
	consFallback: "",

	effectiveModelCons:   "",
	effectiveServiceCons: "",
	effectiveUnitCons:    "",
	effectiveMachineCons: "",
}, {
	about:        "(implicitly) empty fallback constraints never override set constraints",
	consToSet:    "instance-type=foo-42 cpu-power=9001 spaces=bar",
	consFallback: "",

	effectiveModelCons: "", // model constraints are stored as empty.
	// i.e. there are no fallbacks and all the following cases are the same.
	effectiveServiceCons: "instance-type=foo-42 cpu-power=9001 spaces=bar",
	effectiveUnitCons:    "instance-type=foo-42 cpu-power=9001 spaces=bar",
	effectiveMachineCons: "instance-type=foo-42 cpu-power=9001 spaces=bar",
}, {
	about:        "(implicitly) empty constraints never override explictly set fallbacks",
	consToSet:    "",
	consFallback: "arch=amd64 cores=42 mem=2G tags=foo",

	effectiveModelCons:   "arch=amd64 cores=42 mem=2G tags=foo",
	effectiveServiceCons: "", // set as given.
	effectiveUnitCons:    "arch=amd64 cores=42 mem=2G tags=foo",
	// set as given, then merged with fallbacks; since consToSet is
	// empty, the effective values inherit everything from fallbacks;
	// like the unit, but only because the service constraints are
	// also empty.
	effectiveMachineCons: "arch=amd64 cores=42 mem=2G tags=foo",
}, {
	about:        "(explicitly) empty constraints are OK and stored as given",
	consToSet:    "cores= cpu-power= root-disk= instance-type= container= tags= spaces=",
	consFallback: "",

	effectiveModelCons:   "",
	effectiveServiceCons: "cores= cpu-power= root-disk= instance-type= container= tags= spaces=",
	effectiveUnitCons:    "cores= cpu-power= root-disk= instance-type= container= tags= spaces=",
	effectiveMachineCons: "cores= cpu-power= instance-type= root-disk= tags= spaces=", // container= is dropped
}, {
	about:        "(explicitly) empty fallback constraints are OK and stored as given",
	consToSet:    "",
	consFallback: "cores= cpu-power= root-disk= instance-type= container= tags= spaces=",

	effectiveModelCons:   "cores= cpu-power= root-disk= instance-type= container= tags= spaces=",
	effectiveServiceCons: "",
	effectiveUnitCons:    "cores= cpu-power= root-disk= instance-type= container= tags= spaces=",
	effectiveMachineCons: "cores= cpu-power= instance-type= root-disk= tags= spaces=", // container= is dropped
}, {
	about:                "(explicitly) empty constraints and fallbacks are OK and stored as given",
	consToSet:            "arch= mem= cores= container=",
	consFallback:         "cores= cpu-power= root-disk= instance-type= container= tags= spaces=",
	effectiveModelCons:   "cores= cpu-power= root-disk= instance-type= container= tags= spaces=",
	effectiveServiceCons: "arch= mem= cores= container=",
	effectiveUnitCons:    "arch= container= cores= cpu-power= mem= root-disk= tags= spaces=",
	effectiveMachineCons: "arch= cores= cpu-power= mem= root-disk= tags= spaces=", // container= is dropped
}, {
	about:        "(explicitly) empty constraints override set fallbacks for deployment and provisioning",
	consToSet:    "cores= arch= spaces= cpu-power=",
	consFallback: "cores=42 arch=amd64 tags=foo spaces=default,^dmz mem=4G",

	effectiveModelCons:   "cores=42 arch=amd64 tags=foo spaces=default,^dmz mem=4G",
	effectiveServiceCons: "cores= arch= spaces= cpu-power=",
	effectiveUnitCons:    "arch= cores= cpu-power= mem=4G tags=foo spaces=",
	effectiveMachineCons: "arch= cores= cpu-power= mem=4G tags=foo spaces=",
	// we're also checking if m.SetConstraints() does the same with
	// regards to the effective constraints as AddMachine(), because
	// some of these tests proved they had different behavior (i.e.
	// container= was not set to empty)
}, {
	about:        "non-empty constraints always override empty or unset fallbacks",
	consToSet:    "cores=42 root-disk=20G arch=amd64 tags=foo,bar",
	consFallback: "cores= arch= tags=",

	effectiveModelCons:   "cores= arch= tags=",
	effectiveServiceCons: "cores=42 root-disk=20G arch=amd64 tags=foo,bar",
	effectiveUnitCons:    "cores=42 root-disk=20G arch=amd64 tags=foo,bar",
	effectiveMachineCons: "cores=42 root-disk=20G arch=amd64 tags=foo,bar",
}, {
	about:        "non-empty constraints always override set fallbacks",
	consToSet:    "cores=42 root-disk=20G arch=amd64 tags=foo,bar",
	consFallback: "cores=12 root-disk=10G arch=i386  tags=bar",

	effectiveModelCons:   "cores=12 root-disk=10G arch=i386  tags=bar",
	effectiveServiceCons: "cores=42 root-disk=20G arch=amd64 tags=foo,bar",
	effectiveUnitCons:    "cores=42 root-disk=20G arch=amd64 tags=foo,bar",
	effectiveMachineCons: "cores=42 root-disk=20G arch=amd64 tags=foo,bar",
}, {
	about:        "non-empty constraints override conflicting set fallbacks",
	consToSet:    "mem=8G arch=amd64 cores=4 tags=bar",
	consFallback: "instance-type=small cpu-power=1000", // instance-type conflicts mem, arch

	effectiveModelCons:   "instance-type=small cpu-power=1000",
	effectiveServiceCons: "mem=8G arch=amd64 cores=4 tags=bar",
	// both of the following contain the explicitly set constraints after
	// resolving any conflicts with fallbacks (by dropping them).
	effectiveUnitCons:    "mem=8G arch=amd64 cores=4 tags=bar cpu-power=1000",
	effectiveMachineCons: "mem=8G arch=amd64 cores=4 tags=bar cpu-power=1000",
}, {
	about:        "set fallbacks are overriden the same way for provisioning and deployment",
	consToSet:    "tags= cpu-power= spaces=bar",
	consFallback: "tags=foo cpu-power=42",

	// a variation of the above case showing there's no difference
	// between deployment (service, unit) and provisioning (machine)
	// constraints when it comes to effective values.
	effectiveModelCons:   "tags=foo cpu-power=42",
	effectiveServiceCons: "cpu-power= tags= spaces=bar",
	effectiveUnitCons:    "cpu-power= tags= spaces=bar",
	effectiveMachineCons: "cpu-power= tags= spaces=bar",
}, {
	about:        "container type can only be used for deployment, not provisioning",
	consToSet:    "container=kvm arch=amd64",
	consFallback: "container=lxd mem=8G",

	// service deployment constraints are transformed into machine
	// provisioning constraints, and the container type only makes
	// sense currently as a deployment constraint, so it's cleared
	// when merging service/model deployment constraints into
	// effective machine provisioning constraints.
	effectiveModelCons:   "container=lxd mem=8G",
	effectiveServiceCons: "container=kvm arch=amd64",
	effectiveUnitCons:    "container=kvm mem=8G arch=amd64",
	effectiveMachineCons: "mem=8G arch=amd64",
}, {
	about:        "specify image virt-type when deploying applications on multi-hypervisor aware openstack",
	consToSet:    "virt-type=kvm",
	consFallback: "",

	// application deployment constraints are transformed into machine
	// provisioning constraints. Unit constraints must also have virt-type set
	// to ensure consistency in scalability.
	effectiveModelCons:   "",
	effectiveServiceCons: "virt-type=kvm",
	effectiveUnitCons:    "virt-type=kvm",
	effectiveMachineCons: "virt-type=kvm",
}, {
	about:        "ensure model and application constraints are separate",
	consToSet:    "virt-type=kvm",
	consFallback: "mem=2G",

	// application deployment constraints are transformed into machine
	// provisioning constraints. Unit constraints must also have virt-type set
	// to ensure consistency in scalability.
	effectiveModelCons:   "mem=2G",
	effectiveServiceCons: "virt-type=kvm",
	effectiveUnitCons:    "mem=2G virt-type=kvm",
	effectiveMachineCons: "mem=2G virt-type=kvm",
}}

func (s *constraintsValidationSuite) TestMachineConstraints(c *gc.C) {
	for i, t := range setConstraintsTests {
		c.Logf(
			"test %d: %s\nconsToSet: %q\nconsFallback: %q\n",
			i, t.about, t.consToSet, t.consFallback,
		)
		// Set fallbacks as model constraints and verify them.
		err := s.State.SetModelConstraints(constraints.MustParse(t.consFallback))
		c.Check(err, jc.ErrorIsNil)
		econs, err := s.State.ModelConstraints()
		c.Check(econs, jc.DeepEquals, constraints.MustParse(t.effectiveModelCons))
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
		// Set fallbacks as model constraints and verify them.
		err := s.State.SetModelConstraints(constraints.MustParse(t.consFallback))
		c.Check(err, jc.ErrorIsNil)
		econs, err := s.State.ModelConstraints()
		c.Check(econs, jc.DeepEquals, constraints.MustParse(t.effectiveModelCons))
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

func (s *applicationConstraintsSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.policy.GetConstraintsValidator = func() (constraints.Validator, error) {
		validator := constraints.NewValidator()
		validator.RegisterVocabulary(constraints.VirtType, []string{"kvm"})
		return validator, nil
	}
	s.applicationName = "wordpress"
	s.testCharm = s.AddTestingCharm(c, s.applicationName)
}

func (s *applicationConstraintsSuite) TestAddApplicationInvalidConstraints(c *gc.C) {
	cons := constraints.MustParse("virt-type=blah")
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:        s.applicationName,
		Series:      "",
		Charm:       s.testCharm,
		Constraints: cons,
	})
	c.Assert(errors.Cause(err), gc.ErrorMatches, regexp.QuoteMeta("invalid constraint value: virt-type=blah\nvalid values are: [kvm]"))
}

func (s *applicationConstraintsSuite) TestAddApplicationValidConstraints(c *gc.C) {
	cons := constraints.MustParse("virt-type=kvm")
	service, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:        s.applicationName,
		Series:      "",
		Charm:       s.testCharm,
		Constraints: cons,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(service, gc.NotNil)
}
