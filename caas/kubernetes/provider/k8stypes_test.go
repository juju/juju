// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/testing"
)

type ContainersSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ContainersSuite{})

func (s *ContainersSuite) TestParse(c *gc.C) {

	specStr := `
containers:
  - name: gitlab
    image: gitlab/latest
    imagePullPolicy: Always
    command: ["sh", "-c"]
    args: ["doIt", "--debug"]
    workingDir: "/path/to/here"
    ports:
    - containerPort: 80
      name: fred
      protocol: TCP
    - containerPort: 443
      name: mary
    livenessProbe:
      initialDelaySeconds: 10
      httpGet:
        path: /ping
        port: 8080
    readinessProbe:
      initialDelaySeconds: 10
      httpGet:
        path: /pingReady
        port: www
    config:
      attr: foo=bar; fred=blogs
      foo: bar
    files:
      - name: configuration
        mountPath: /var/lib/foo
        files:
          file1: |
            [config]
            foo: bar
  - name: gitlab-helper
    image: gitlab-helper/latest
    ports:
    - containerPort: 8080
      protocol: TCP
  - name: secret-image-user
    imageDetails:
        imagePath: staging.registry.org/testing/testing-image@sha256:deed-beef
        username: docker-registry
        password: hunter2
  - name: just-image-details
    imageDetails:
        imagePath: testing/no-secrets-needed@sha256:deed-beef
`[1:]

	expectedFileContent := `
[config]
foo: bar
`[1:]

	spec, err := provider.ParseK8sPodSpec(specStr)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spec, jc.DeepEquals, &caas.PodSpec{
		Containers: []caas.ContainerSpec{{
			Name:       "gitlab",
			Image:      "gitlab/latest",
			Command:    []string{"sh", "-c"},
			Args:       []string{"doIt", "--debug"},
			WorkingDir: "/path/to/here",
			Ports: []caas.ContainerPort{
				{ContainerPort: 80, Protocol: "TCP", Name: "fred"},
				{ContainerPort: 443, Name: "mary"},
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
			ProviderContainer: &provider.K8sContainerSpec{
				ImagePullPolicy: "Always",
				LivenessProbe: &core.Probe{
					InitialDelaySeconds: 10,
					Handler: core.Handler{
						HTTPGet: &core.HTTPGetAction{
							Path: "/ping",
							Port: intstr.IntOrString{IntVal: 8080},
						},
					},
				},
				ReadinessProbe: &core.Probe{
					InitialDelaySeconds: 10,
					Handler: core.Handler{
						HTTPGet: &core.HTTPGetAction{
							Path: "/pingReady",
							Port: intstr.IntOrString{StrVal: "www", Type: 1},
						},
					},
				},
			},
		}, {
			Name:  "gitlab-helper",
			Image: "gitlab-helper/latest",
			Ports: []caas.ContainerPort{
				{ContainerPort: 8080, Protocol: "TCP"},
			},
		}, {
			Name: "secret-image-user",
			ImageDetails: caas.ImageDetails{
				ImagePath: "staging.registry.org/testing/testing-image@sha256:deed-beef",
				Username:  "docker-registry",
				Password:  "hunter2",
			},
		}, {
			Name: "just-image-details",
			ImageDetails: caas.ImageDetails{
				ImagePath: "testing/no-secrets-needed@sha256:deed-beef",
			},
		}}})
}
