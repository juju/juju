// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/testing"
)

type ContainersSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ContainersSuite{})

func (s *ContainersSuite) TestParse(c *gc.C) {

	specStr := `
omit-service-frontend: true
containers:
  - name: gitlab
    image-name: gitlab/latest
    ports:
    - container-port: 80
      protocol: TCP
    - container-port: 443
    config:
      attr: foo=bar; fred=blogs
      foo: bar
    files:
      - name: configuration
        mount-path: /var/lib/foo
        files:
          file1: |
            [config]
            foo: bar
  - name: gitlab-helper
    image-name: gitlab-helper/latest
    ports:
    - container-port: 8080
      protocol: TCP
`[1:]

	expectedFileContent := `
[config]
foo: bar
`[1:]

	spec, err := caas.ParsePodSpec(specStr)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spec, jc.DeepEquals, &caas.PodSpec{
		OmitServiceFrontend: true,
		Containers: []caas.ContainerSpec{{
			Name:      "gitlab",
			ImageName: "gitlab/latest",
			Ports: []caas.ContainerPort{
				{ContainerPort: 80, Protocol: "TCP"},
				{ContainerPort: 443},
			},
			Config: map[string]string{
				"attr": "foo=bar; fred=blogs",
				"foo":  "bar",
			},
			Files: []caas.FileSet{
				{
					Name:      "configuration",
					MountPath: "/var/lib/foo",
					Files: map[string]string{
						"file1": expectedFileContent,
					},
				},
			},
		}, {
			Name:      "gitlab-helper",
			ImageName: "gitlab-helper/latest",
			Ports: []caas.ContainerPort{
				{ContainerPort: 8080, Protocol: "TCP"},
			},
		}}})
}

func (s *ContainersSuite) TestParseMissingContainers(c *gc.C) {

	specStr := `
containers:
`[1:]

	_, err := caas.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, "require at least one pod spec")
}

func (s *ContainersSuite) TestParseMissingName(c *gc.C) {

	specStr := `
containers:
  - image-name: gitlab/latest
`[1:]

	_, err := caas.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, "spec name is missing")
}

func (s *ContainersSuite) TestParseMissingImage(c *gc.C) {

	specStr := `
containers:
  - name: gitlab
`[1:]

	_, err := caas.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, "spec image name is missing")
}

func (s *ContainersSuite) TestParseFileSetPath(c *gc.C) {

	specStr := `
containers:
  - name: gitlab
    image-name: gitlab/latest
    files:
      - files:
          file1: |-
            [config]
            foo: bar
`[1:]

	_, err := caas.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, `file set name is missing`)
}

func (s *ContainersSuite) TestParseMissingMountPath(c *gc.C) {

	specStr := `
containers:
  - name: gitlab
    image-name: gitlab/latest
    files:
      - name: configuration
        files:
          file1: |-
            [config]
            foo: bar
`[1:]

	_, err := caas.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, `mount path is missing for file set "configuration"`)
}
