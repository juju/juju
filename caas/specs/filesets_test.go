// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs_test

import (
	// jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas/specs"
	// "github.com/juju/juju/testing"
)

type validatorFileSet interface {
	Validate() error
}

type validateFileSetTc struct {
	spec   validatorFileSet
	errStr string
}

func (s *typesSuite) TestValidateFileSet(c *gc.C) {
	badMultiSource := &specs.FileSet{
		Name:      "file1",
		MountPath: "/foo/bar",
	}
	badMultiSource.HostPath = &specs.HostPathVol{
		Path: "/foo/bar",
	}
	badMultiSource.EmptyDir = &specs.EmptyDirVol{}
	for i, tc := range []validateFileSetTc{
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
		{
			spec: &specs.FileSet{
				Name:      "file1",
				MountPath: "/foo/bar",
			},
			errStr: `file set "file1" requires volume source`,
		},
		{
			spec:   badMultiSource,
			errStr: `file set "file1" can only have one volume source`,
		},
	} {
		c.Logf("#%d: testing FileSet.Validate", i)
		c.Check(tc.spec.Validate(), gc.ErrorMatches, tc.errStr)
	}
}

type validatorVolumeSource interface {
	Validate(string) error
}

type validateVolumeSourceTc struct {
	spec   validatorVolumeSource
	errStr string
}

func (s *typesSuite) TestValidateFileSetVolumeSource(c *gc.C) {
	for i, tc := range []validateVolumeSourceTc{
		{
			spec: &specs.VolumeSource{
				HostPath: &specs.HostPathVol{},
			},
			errStr: `Path is missing for "fakeFileSet"`,
		},
		{
			spec: &specs.VolumeSource{
				Secret: &specs.ResourceRefVol{},
			},
			errStr: `Name is missing for "fakeFileSet"`,
		},
		{
			spec: &specs.VolumeSource{
				ConfigMap: &specs.ResourceRefVol{},
			},
			errStr: `Name is missing for "fakeFileSet"`,
		},
		{
			spec:   &specs.KeyToPath{},
			errStr: `Key is missing for "fakeFileSet"`,
		},
		{
			spec: &specs.KeyToPath{
				Key: "key1",
			},
			errStr: `Path is missing for "fakeFileSet"`,
		},
	} {
		c.Logf("#%d: testing VolumeSource.Validate", i)
		c.Check(tc.spec.Validate("fakeFileSet"), gc.ErrorMatches, tc.errStr)
	}
}
