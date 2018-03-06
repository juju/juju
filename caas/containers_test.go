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
name: gitlab
image-name: gitlab/latest
ports:
- container-port: 80
  protocol: TCP
- container-port: 443
config:
  attr: foo=bar; fred=blogs
  foo: bar
  mysecret:
    secret: logincreds
    key: password
files:
  - name: configuration
    mount-path: /var/lib/foo
    files:
      file1: |
        [config]
        foo: bar
    secret:
      name: anothersecret
  - name: more-configuration
    mount-path: /var/lib/foo
    secret:
      name: reallysecret
      keys:
        - key: password
          path: password.txt
        - key: rootpassword
          path: rootpassword.txt
`[1:]

	expectedFileContent := `
[config]
foo: bar
`[1:]

	spec, err := caas.ParseContainerSpec(specStr)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spec, jc.DeepEquals, &caas.ContainerSpec{
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
		ConfigSecrets: map[string]caas.ConfigSecret{
			"mysecret": {SecretName: "logincreds", Key: "password"},
		},
		Files: []caas.FileSet{
			{
				Name:      "configuration",
				MountPath: "/var/lib/foo",
				Files: map[string]string{
					"file1": expectedFileContent,
				},
				Secret: &caas.FileSecret{Name: "anothersecret"},
			}, {
				Name:      "more-configuration",
				MountPath: "/var/lib/foo",
				Secret: &caas.FileSecret{
					Name: "reallysecret",
					SecretKeys: []caas.KeyPath{
						{Key: "password", Path: "password.txt"},
						{Key: "rootpassword", Path: "rootpassword.txt"},
					},
				},
			},
		},
	})
}

func (s *ContainersSuite) TestParseMissingName(c *gc.C) {

	specStr := `
image-name: gitlab/latest
`[1:]

	_, err := caas.ParseContainerSpec(specStr)
	c.Assert(err, gc.ErrorMatches, "spec name is missing")
}

func (s *ContainersSuite) TestParseMissingImage(c *gc.C) {

	specStr := `
name: gitlab
`[1:]

	_, err := caas.ParseContainerSpec(specStr)
	c.Assert(err, gc.ErrorMatches, "spec image name is missing")
}

func (s *ContainersSuite) TestParseFileSetPath(c *gc.C) {

	specStr := `
name: gitlab
image-name: gitlab/latest
files:
  - files:
      file1: |-
        [config]
        foo: bar
`[1:]

	_, err := caas.ParseContainerSpec(specStr)
	c.Assert(err, gc.ErrorMatches, `file set name is missing`)
}

func (s *ContainersSuite) TestParseMissingMountPath(c *gc.C) {

	specStr := `
name: gitlab
image-name: gitlab/latest
files:
  - name: configuration
    files:
      file1: |-
        [config]
        foo: bar
`[1:]

	_, err := caas.ParseContainerSpec(specStr)
	c.Assert(err, gc.ErrorMatches, `mount path is missing for file set "configuration"`)
}

func (s *ContainersSuite) TestParseTooManySecretAttributes(c *gc.C) {

	specStr := `
name: gitlab
image-name: gitlab/latest
config:
  mysecret:
    secret: logincreds
    key: password
    extra: bad
`[1:]

	_, err := caas.ParseContainerSpec(specStr)
	c.Assert(err, gc.ErrorMatches, `expected 2 values in secret spec for "mysecret", got 3`)
}

func (s *ContainersSuite) TestParseMissingSecretName(c *gc.C) {

	specStr := `
name: gitlab
image-name: gitlab/latest
config:
  mysecret:
    extra: value
    key: password
`[1:]

	_, err := caas.ParseContainerSpec(specStr)
	c.Assert(err, gc.ErrorMatches, `missing secret name for secret "mysecret"`)
}

func (s *ContainersSuite) TestParseMissingSecretKey(c *gc.C) {

	specStr := `
name: gitlab
image-name: gitlab/latest
config:
  mysecret:
    secret: logincreds
    extra: value
`[1:]

	_, err := caas.ParseContainerSpec(specStr)
	c.Assert(err, gc.ErrorMatches, `missing key for secret "mysecret"`)
}
