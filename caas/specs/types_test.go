// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas/specs"
	"github.com/juju/juju/testing"
)

type typesSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&typesSuite{})

func (s *typesSuite) TestGetVersion(c *gc.C) {
	type testcase struct {
		strSpec string
		version specs.Version
	}

	for i, tc := range []testcase{
		{
			strSpec: `
`[1:],
			version: specs.Version(0),
		},
		{
			strSpec: `
version: 0
`[1:],
			version: specs.Version(0),
		},
		{
			strSpec: `
version: 2
`[1:],
			version: specs.Version(2),
		},
		{
			strSpec: `
version: 3
`[1:],
			version: specs.Version(3),
		},
	} {
		c.Logf("#%d: testing GetVersion: %d", i, tc.version)
		v, err := specs.GetVersion(tc.strSpec)
		c.Check(err, jc.ErrorIsNil)
		c.Check(v, gc.DeepEquals, tc.version)
	}
}

type validator interface {
	Validate() error
}

type validateTc struct {
	spec   validator
	errStr string
}

func (s *typesSuite) TestValidateFileSet(c *gc.C) {
	for i, tc := range []validateTc{
		{
			spec: &specs.FileSet{
				Name: "file1",
			},
			errStr: `mount path is missing for file set "file1"`,
		},
		{
			spec: &specs.FileSet{
				MountPath: "/foo/bar",
			},
			errStr: `file set name is missing`,
		},
	} {
		c.Logf("#%d: testing FileSet.Validate", i)
		c.Check(tc.spec.Validate(), gc.ErrorMatches, tc.errStr)
	}
}

func (s *typesSuite) TestValidateServiceSpec(c *gc.C) {
	spec := specs.ServiceSpec{
		ScalePolicy: "foo",
	}
	c.Assert(spec.Validate(), gc.ErrorMatches, `foo not supported`)

	spec = specs.ServiceSpec{
		ScalePolicy: "parallel",
	}
	c.Assert(spec.Validate(), jc.ErrorIsNil)

	spec = specs.ServiceSpec{
		ScalePolicy: "serial",
	}
	c.Assert(spec.Validate(), jc.ErrorIsNil)
}

func (s *typesSuite) TestValidateContainerSpec(c *gc.C) {
	for i, tc := range []validateTc{
		{
			spec: &specs.ContainerSpec{
				Name: "container1",
			},
			errStr: `spec image details is missing`,
		},
		{
			spec: &specs.ContainerSpec{
				Image: "gitlab",
			},
			errStr: `spec name is missing`,
		},
		{
			spec: &specs.ContainerSpec{
				ImageDetails: specs.ImageDetails{
					ImagePath: "gitlab",
				},
			},
			errStr: `spec name is missing`,
		},
		{
			spec: &specs.ContainerSpec{
				Name:  "container1",
				Image: "gitlab",
			},
			errStr: "",
		},
		{
			spec: &specs.ContainerSpec{
				Name: "container1",
				ImageDetails: specs.ImageDetails{
					ImagePath: "gitlab",
				},
			},
			errStr: "",
		},
	} {
		c.Logf("#%d: testing FileSet.Validate", i)
		err := tc.spec.Validate()
		if tc.errStr == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, tc.errStr)
		}
	}
}

func (s *typesSuite) TestValidatePodSpecBase(c *gc.C) {
	spec := specs.PodSpecBase{}
	c.Assert(spec.Validate(specs.VersionLegacy), jc.ErrorIsNil)
	spec.Version = specs.VersionLegacy
	c.Assert(spec.Validate(specs.VersionLegacy), jc.ErrorIsNil)

	c.Assert(spec.Validate(specs.Version2), gc.ErrorMatches, `expected version 2, but found 0`)
	spec.Version = specs.Version2
	c.Assert(spec.Validate(specs.Version2), jc.ErrorIsNil)
}
