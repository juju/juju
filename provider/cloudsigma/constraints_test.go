// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"github.com/altoros/gosigma"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/testing"
)

const (
	validImageId = "473adb38-3b64-43b2-93bd-f1a3443c19ea"
)

type constraintsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&constraintsSuite{})

type strv struct{ v string }
type uint64v struct{ v uint64 }

var defConstraints = map[string]sigmaConstraints{
	"bootstrap-trusty": sigmaConstraints{
		driveTemplate: validImageId,
		driveSize:     5 * gosigma.Gigabyte,
		cores:         1,
		power:         2000,
		mem:           2 * gosigma.Gigabyte,
	},
	"trusty": sigmaConstraints{
		driveTemplate: validImageId,
		driveSize:     0,
		cores:         1,
		power:         2000,
		mem:           2 * gosigma.Gigabyte,
	},
	"trusty-c2-p4000": sigmaConstraints{
		driveTemplate: validImageId,
		driveSize:     0,
		cores:         2,
		power:         4000,
		mem:           2 * gosigma.Gigabyte,
	},
}

var img = &imagemetadata.ImageMetadata{
	Id: validImageId,
}

var newConstraintTests = []struct {
	bootstrap bool
	arch      *strv
	cores     *uint64v
	power     *uint64v
	mem       *uint64v
	disk      *uint64v
	series    string
	expected  sigmaConstraints
	err       *strv
}{
	{true, nil, nil, nil, nil, nil, "trusty", defConstraints["bootstrap-trusty"], nil},
	{false, nil, nil, nil, nil, nil, "trusty", defConstraints["trusty"], nil},
	{true, &strv{"amd64"}, nil, nil, nil, nil, "trusty", defConstraints["bootstrap-trusty"], nil},
	{false, &strv{"amd64"}, nil, nil, nil, nil, "trusty", defConstraints["trusty"], nil},
	{true, nil, &uint64v{1}, nil, nil, nil, "trusty", defConstraints["bootstrap-trusty"], nil},
	{false, nil, &uint64v{1}, nil, nil, nil, "trusty", defConstraints["trusty"], nil},
	{false, nil, &uint64v{2}, nil, nil, nil, "trusty", defConstraints["trusty-c2-p4000"], nil},
	{false, nil, &uint64v{2}, &uint64v{4000}, nil, nil, "trusty", defConstraints["trusty-c2-p4000"], nil},
	{false, nil, nil, nil, &uint64v{2 * 1024}, nil, "trusty", defConstraints["trusty"], nil},
	{false, nil, nil, nil, nil, &uint64v{5 * 1024}, "trusty", defConstraints["bootstrap-trusty"], nil},
}

func (s *constraintsSuite) TestConstraints(c *gc.C) {
	for i, t := range newConstraintTests {
		var cv constraints.Value
		if t.arch != nil {
			cv.Arch = &t.arch.v
		}
		if t.cores != nil {
			cv.CpuCores = &t.cores.v
		}
		if t.power != nil {
			cv.CpuPower = &t.power.v
		}
		if t.mem != nil {
			cv.Mem = &t.mem.v
		}
		if t.disk != nil {
			cv.RootDisk = &t.disk.v
		}
		v := newConstraints(t.bootstrap, cv, img)
		if !c.Check(*v, gc.Equals, t.expected) {
			c.Logf("test (%d): %+v", i, t)
		}
	}
}

func (s *constraintsSuite) TestConstraintsArch(c *gc.C) {
	var cv constraints.Value
	var expected = sigmaConstraints{
		driveTemplate: validImageId,
		driveSize:     5 * gosigma.Gigabyte,
		cores:         1,
		power:         2000,
		mem:           2 * gosigma.Gigabyte,
	}

	sc := newConstraints(true, cv, img)
	c.Check(*sc, gc.Equals, expected)
}
