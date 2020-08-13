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

type validator interface {
	Validate() error
}

type validateTc struct {
	spec   validator
	errStr string
}

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

func (s *typesSuite) TestValidateServiceSpec(c *gc.C) {
	spec := specs.ServiceSpec{
		ScalePolicy: "bar",
	}
	c.Assert(spec.Validate(), gc.ErrorMatches, `scale policy "bar" not supported`)

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
	minSpecs := specs.PodSpecBase{}
	minSpecs.Containers = []specs.ContainerSpec{
		{
			Name:  "gitlab-helper",
			Image: "gitlab-helper/latest",
			Ports: []specs.ContainerPort{
				{ContainerPort: 8080, Protocol: "TCP"},
			},
		},
	}
	c.Assert(minSpecs.Validate(specs.VersionLegacy), jc.ErrorIsNil)
	minSpecs.Version = specs.VersionLegacy
	c.Assert(minSpecs.Validate(specs.VersionLegacy), jc.ErrorIsNil)

	c.Assert(minSpecs.Validate(specs.Version2), gc.ErrorMatches, `expected version 2, but found 0`)
	minSpecs.Version = specs.Version2
	c.Assert(minSpecs.Validate(specs.Version2), jc.ErrorIsNil)
}

func (s *typesSuite) TestValidateCaaSContainers(c *gc.C) {
	k8sSpec := specs.CaasContainers{}
	fileSet1 := specs.FileSet{
		Name:      "file1",
		MountPath: "/foo/file1",
		VolumeSource: specs.VolumeSource{
			Files: []specs.File{
				{Path: "foo", Content: "bar"},
			},
		},
	}
	fileSet2 := specs.FileSet{
		Name:      "file2",
		MountPath: "/foo/file2",
		VolumeSource: specs.VolumeSource{
			Files: []specs.File{
				{Path: "foo", Content: "bar"},
			},
		},
	}

	k8sSpec.Containers = []specs.ContainerSpec{
		{
			Name:  "gitlab-helper",
			Image: "gitlab-helper/latest",
			Ports: []specs.ContainerPort{
				{ContainerPort: 8080, Protocol: "TCP"},
			},
			VolumeConfig: []specs.FileSet{
				fileSet1, fileSet2,
			},
		},
	}
	c.Assert(k8sSpec.Validate(), jc.ErrorIsNil)

	k8sSpec = specs.CaasContainers{}
	k8sSpec.Containers = []specs.ContainerSpec{
		{
			Name:  "gitlab-helper",
			Image: "gitlab-helper/latest",
			Ports: []specs.ContainerPort{
				{ContainerPort: 8080, Protocol: "TCP"},
			},
			VolumeConfig: []specs.FileSet{
				fileSet1, fileSet1,
			},
		},
	}
	c.Assert(k8sSpec.Validate(), gc.ErrorMatches, `duplicated file "file1" in container "gitlab-helper" not valid`)

	k8sSpec = specs.CaasContainers{}
	k8sSpec.Containers = []specs.ContainerSpec{
		{
			Name:  "gitlab-helper",
			Image: "gitlab-helper/latest",
			Ports: []specs.ContainerPort{
				{ContainerPort: 8080, Protocol: "TCP"},
			},
			VolumeConfig: []specs.FileSet{
				{
					Name:      "file1",
					MountPath: "/same-mount-path",
					VolumeSource: specs.VolumeSource{
						Files: []specs.File{
							{Path: "foo", Content: "bar"},
						},
					},
				},
				{
					Name:      "file2",
					MountPath: "/same-mount-path",
					VolumeSource: specs.VolumeSource{
						HostPath: &specs.HostPathVol{
							Path: "/foo/bar",
						},
					},
				},
			},
		},
	}
	c.Assert(k8sSpec.Validate(), gc.ErrorMatches, `duplicated mount path "/same-mount-path" in container "gitlab-helper" not valid`)

	k8sSpec = specs.CaasContainers{}
	k8sSpec.Containers = []specs.ContainerSpec{
		{
			Name:  "gitlab-helper",
			Image: "gitlab-helper/latest",
			Ports: []specs.ContainerPort{
				{ContainerPort: 8080, Protocol: "TCP"},
			},
			VolumeConfig: []specs.FileSet{
				{
					Name:      "file1",
					MountPath: "/etc/config",
					VolumeSource: specs.VolumeSource{
						Files: []specs.File{
							{Path: "foo", Content: "bar"},
						},
					},
				},
			},
		},
		{
			Name:  "busybox",
			Image: "busybox",
			Ports: []specs.ContainerPort{
				{ContainerPort: 80, Protocol: "TCP"},
			},
			VolumeConfig: []specs.FileSet{
				{
					Name:      "file1",
					MountPath: "/etc/config",
					VolumeSource: specs.VolumeSource{
						HostPath: &specs.HostPathVol{
							Path: "/foo/bar",
						},
					},
				},
			},
		},
	}
	c.Assert(k8sSpec.Validate(), gc.ErrorMatches, `duplicated file "file1" with different volume spec not valid`)

	k8sSpec = specs.CaasContainers{}
	k8sSpec.Containers = []specs.ContainerSpec{
		{
			Name:  "gitlab-helper",
			Image: "gitlab-helper/latest",
			Ports: []specs.ContainerPort{
				{ContainerPort: 8080, Protocol: "TCP"},
			},
			VolumeConfig: []specs.FileSet{
				{
					Name:      "file1",
					MountPath: "/foo/file1",
					VolumeSource: specs.VolumeSource{
						Files: []specs.File{
							{Path: "foo", Content: "bar"},
						},
					},
				},
				{
					Name:      "file1", // same file in same container mount to different path.
					MountPath: "/foo/another-file1",
					VolumeSource: specs.VolumeSource{
						Files: []specs.File{
							{Path: "foo", Content: "bar"},
						},
					},
				},
				{
					Name:      "file2",
					MountPath: "/foo/file2",
					VolumeSource: specs.VolumeSource{
						Files: []specs.File{
							{Path: "foo", Content: "bar"},
						},
					},
				},
				{
					Name:      "host-path-1",
					MountPath: "/etc/host-path",
					VolumeSource: specs.VolumeSource{
						HostPath: &specs.HostPathVol{
							Path: "/foo/bar",
						},
					},
				},
				{
					Name:      "empty-dir-1",
					MountPath: "/etc/empty-dir",
					VolumeSource: specs.VolumeSource{
						EmptyDir: &specs.EmptyDirVol{
							Medium: "Memory",
						},
					},
				},
				{
					Name:      "config-map-1",
					MountPath: "/etc/config",
					VolumeSource: specs.VolumeSource{
						ConfigMap: &specs.ResourceRefVol{
							Name: "log-config",
							Files: []specs.FileRef{
								{
									Key:  "log_level",
									Path: "log_level",
								},
							},
						},
					},
				},
				{
					Name:      "mysecret2",
					MountPath: "/secrets",
					VolumeSource: specs.VolumeSource{
						Secret: &specs.ResourceRefVol{
							Name: "mysecret2",
							Files: []specs.FileRef{
								{
									Key:  "password",
									Path: "my-group/my-password",
								},
							},
						},
					},
				},
			},
		},
		{
			Name:  "busybox",
			Image: "busybox",
			Ports: []specs.ContainerPort{
				{ContainerPort: 80, Protocol: "TCP"},
			},
			VolumeConfig: []specs.FileSet{
				{
					Name:      "file1", // exact same file1 can be mounted to same path in a different container.
					MountPath: "/foo/file1",
					VolumeSource: specs.VolumeSource{
						Files: []specs.File{
							{Path: "foo", Content: "bar"},
						},
					},
				},
			},
		},
	}
	c.Assert(k8sSpec.Validate(), jc.ErrorIsNil)
}
