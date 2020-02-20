// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas/specs"
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

type comparerFileSet interface {
	Equal(specs.FileSet) bool
}

type comparerFileSetTc struct {
	f1    comparerFileSet
	f2    specs.FileSet
	equal bool
}

func (s *typesSuite) TestCompareFileSet(c *gc.C) {
	for i, tc := range []comparerFileSetTc{
		{
			f1: specs.FileSet{
				Name:      "file1",
				MountPath: "/foo/bar",
				VolumeSource: specs.VolumeSource{
					HostPath: &specs.HostPathVol{
						Path: "/foo/bar",
					},
				},
			},
			f2: specs.FileSet{
				Name:      "file1",
				MountPath: "/foo/bar",
				VolumeSource: specs.VolumeSource{
					HostPath: &specs.HostPathVol{
						Path: "/foo/bar",
					},
				},
			},
			equal: true,
		},
		{
			f1: specs.FileSet{
				Name:      "file1",
				MountPath: "/foo/bar",
				VolumeSource: specs.VolumeSource{
					HostPath: &specs.HostPathVol{
						Path: "/foo/bar",
					},
				},
			},
			f2: specs.FileSet{
				Name:      "file1",
				MountPath: "/foo/bar",
				VolumeSource: specs.VolumeSource{
					HostPath: &specs.HostPathVol{
						Path: "/foo/foo", // different path.
					},
				},
			},
			equal: false,
		},
		{
			f1: specs.FileSet{
				Name:      "file1",
				MountPath: "/foo/bar",
				VolumeSource: specs.VolumeSource{
					HostPath: &specs.HostPathVol{
						Path: "/foo/bar",
					},
				},
			},
			f2: specs.FileSet{
				Name:      "file1",
				MountPath: "/foo/bar",
				VolumeSource: specs.VolumeSource{ // different VolumeSource.
					EmptyDir: &specs.EmptyDirVol{},
				},
			},
			equal: false,
		},
	} {
		c.Logf("#%d: testing FileSet.Equal", i)
		c.Check(tc.f1.Equal(tc.f2), gc.DeepEquals, tc.equal)
	}
}

type comparerFileSetVol interface {
	EqualVolume(specs.FileSet) bool
}

type comparerFileSetVolTc struct {
	f1    comparerFileSetVol
	f2    specs.FileSet
	equal bool
}

func (s *typesSuite) TestCompareFileSetVolume(c *gc.C) {
	for i, tc := range []comparerFileSetVolTc{
		{
			// exactly same.
			f1: specs.FileSet{
				Name:      "file1",
				MountPath: "/foo/bar",
				VolumeSource: specs.VolumeSource{
					HostPath: &specs.HostPathVol{
						Path: "/foo/bar",
					},
				},
			},
			f2: specs.FileSet{
				Name:      "file1",
				MountPath: "/foo/bar",
				VolumeSource: specs.VolumeSource{
					HostPath: &specs.HostPathVol{
						Path: "/foo/bar",
					},
				},
			},
			equal: true,
		},
		{
			f1: specs.FileSet{
				Name:      "file1",
				MountPath: "/foo/bar",
				VolumeSource: specs.VolumeSource{
					HostPath: &specs.HostPathVol{
						Path: "/foo/bar",
					},
				},
			},
			f2: specs.FileSet{
				Name:      "file1",
				MountPath: "/foo/bla", // different mount path.
				VolumeSource: specs.VolumeSource{
					HostPath: &specs.HostPathVol{
						Path: "/foo/bar",
					},
				},
			},
			equal: true,
		},
		{
			// different name.
			f1: specs.FileSet{
				Name:      "file1",
				MountPath: "/foo/bar",
				VolumeSource: specs.VolumeSource{
					HostPath: &specs.HostPathVol{
						Path: "/foo/bar",
					},
				},
			},
			f2: specs.FileSet{
				Name:      "file2",
				MountPath: "/foo/bar",
				VolumeSource: specs.VolumeSource{
					HostPath: &specs.HostPathVol{
						Path: "/foo/bar",
					},
				},
			},
			equal: false,
		},
		{
			f1: specs.FileSet{
				Name:      "file1",
				MountPath: "/foo/bar",
				VolumeSource: specs.VolumeSource{
					HostPath: &specs.HostPathVol{
						Path: "/foo/bar",
					},
				},
			},
			f2: specs.FileSet{
				Name:      "file1",
				MountPath: "/foo/bar",
				VolumeSource: specs.VolumeSource{ // different VolumeSource.
					EmptyDir: &specs.EmptyDirVol{},
				},
			},
			equal: false,
		},
	} {
		c.Logf("#%d: testing FileSet.EqualVolume", i)
		c.Check(tc.f1.EqualVolume(tc.f2), gc.DeepEquals, tc.equal)
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
