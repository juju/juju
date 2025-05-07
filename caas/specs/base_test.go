// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/caas/specs"
	"github.com/juju/juju/internal/testing"
)

type baseSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&baseSuite{})

type validator interface {
	Validate() error
}

type validateTc struct {
	spec   validator
	errStr string
}

func (s *baseSuite) TestGetVersion(c *tc.C) {
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
		c.Check(err, tc.ErrorIsNil)
		c.Check(v, tc.DeepEquals, tc.version)
	}
}

func (s *baseSuite) TestValidateServiceSpec(c *tc.C) {
	spec := specs.ServiceSpec{
		ScalePolicy: "bar",
	}
	c.Assert(spec.Validate(), tc.ErrorMatches, `scale policy "bar" not supported`)

	spec = specs.ServiceSpec{
		ScalePolicy: "parallel",
	}
	c.Assert(spec.Validate(), tc.ErrorIsNil)

	spec = specs.ServiceSpec{
		ScalePolicy: "serial",
	}
	c.Assert(spec.Validate(), tc.ErrorIsNil)

	spec = specs.ServiceSpec{
		UpdateStrategy: &specs.UpdateStrategy{
			Type: "Recreate",
			RollingUpdate: &specs.RollingUpdateSpec{
				MaxUnavailable: &specs.IntOrString{Type: specs.String, StrVal: "10%"},
				MaxSurge:       &specs.IntOrString{Type: specs.String, StrVal: "25%"},
			},
		},
	}
	c.Assert(spec.Validate(), tc.ErrorIsNil)

	spec = specs.ServiceSpec{
		UpdateStrategy: &specs.UpdateStrategy{
			Type: "",
			RollingUpdate: &specs.RollingUpdateSpec{
				MaxUnavailable: &specs.IntOrString{Type: specs.String, StrVal: "10%"},
				MaxSurge:       &specs.IntOrString{Type: specs.String, StrVal: "25%"},
			},
		},
	}
	c.Assert(spec.Validate(), tc.ErrorMatches, `type is required`)

	spec = specs.ServiceSpec{
		UpdateStrategy: &specs.UpdateStrategy{
			Type: "Recreate",
		},
	}
	c.Assert(spec.Validate(), tc.ErrorIsNil)

	var partition int32 = 3
	spec = specs.ServiceSpec{
		UpdateStrategy: &specs.UpdateStrategy{
			Type: "Recreate",
			RollingUpdate: &specs.RollingUpdateSpec{
				Partition: &partition,
				MaxSurge:  &specs.IntOrString{Type: specs.String, StrVal: "25%"},
			},
		},
	}
	c.Assert(spec.Validate(), tc.ErrorMatches, `partion can not be defined with maxUnavailable or maxSurge together`)

	spec = specs.ServiceSpec{
		UpdateStrategy: &specs.UpdateStrategy{
			Type: "Recreate",
			RollingUpdate: &specs.RollingUpdateSpec{
				Partition:      &partition,
				MaxUnavailable: &specs.IntOrString{Type: specs.String, StrVal: "10%"},
			},
		},
	}
	c.Assert(spec.Validate(), tc.ErrorMatches, `partion can not be defined with maxUnavailable or maxSurge together`)

	spec = specs.ServiceSpec{
		UpdateStrategy: &specs.UpdateStrategy{
			Type: "Recreate",
			RollingUpdate: &specs.RollingUpdateSpec{
				Partition:      &partition,
				MaxUnavailable: &specs.IntOrString{Type: specs.String, StrVal: "10%"},
				MaxSurge:       &specs.IntOrString{Type: specs.String, StrVal: "25%"},
			},
		},
	}
	c.Assert(spec.Validate(), tc.ErrorMatches, `partion can not be defined with maxUnavailable or maxSurge together`)
}

func (s *baseSuite) TestValidateContainerSpec(c *tc.C) {
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
			c.Check(err, tc.ErrorIsNil)
		} else {
			c.Check(err, tc.ErrorMatches, tc.errStr)
		}
	}
}

func (s *baseSuite) TestValidatePodSpecBase(c *tc.C) {
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
	c.Assert(minSpecs.Validate(specs.VersionLegacy), tc.ErrorIsNil)
	minSpecs.Version = specs.VersionLegacy
	c.Assert(minSpecs.Validate(specs.VersionLegacy), tc.ErrorIsNil)

	c.Assert(minSpecs.Validate(specs.Version2), tc.ErrorMatches, `expected version 2, but found 0`)
	minSpecs.Version = specs.Version2
	c.Assert(minSpecs.Validate(specs.Version2), tc.ErrorIsNil)
}

func (s *baseSuite) TestValidateCaaSContainers(c *tc.C) {
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
	c.Assert(k8sSpec.Validate(), tc.ErrorIsNil)

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
	c.Assert(k8sSpec.Validate(), tc.ErrorMatches, `duplicated file "file1" in container "gitlab-helper" not valid`)

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
	c.Assert(k8sSpec.Validate(), tc.ErrorMatches, `duplicated mount path "/same-mount-path" in container "gitlab-helper" not valid`)

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
	c.Assert(k8sSpec.Validate(), tc.ErrorMatches, `duplicated file "file1" with different volume spec not valid`)

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
	c.Assert(k8sSpec.Validate(), tc.ErrorIsNil)
}
