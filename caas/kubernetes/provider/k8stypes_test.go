// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
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
omitServiceFrontend: true
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
      attr: foo=bar; name['fred']='blogs';
      foo: bar
      restricted: 'yes'
      switch: on
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
customResourceDefinition:
  - group: kubeflow.org
    version: v1alpha2
    scope: Namespaced
    kind: TFJob
    validation:
      properties:
        tfReplicaSpecs:
          properties:
            Worker:
              properties:
                replicas:
                  type: integer
                  minimum: 1
            PS:
              properties:
                replicas:
                  type: integer
                  minimum: 1
            Chief:
              properties:
                replicas:
                  type: integer
                  minimum: 1
                  maximum: 1
`[1:]

	expectedFileContent := `
[config]
foo: bar
`[1:]

	spec, err := provider.ParseK8sPodSpec(specStr)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spec, jc.DeepEquals, &caas.PodSpec{
		OmitServiceFrontend: true,
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
			Config: map[string]interface{}{
				"attr":       "foo=bar; name['fred']='blogs';",
				"foo":        "bar",
				"restricted": "'yes'",
				"switch":     true,
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
		}},
		CustomResourceDefinitions: []caas.CustomResourceDefinition{
			{
				Kind:    "TFJob",
				Group:   "kubeflow.org",
				Version: "v1alpha2",
				Scope:   "Namespaced",
				Validation: caas.CustomResourceDefinitionValidation{
					Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
						"tfReplicaSpecs": {
							Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
								"PS": {
									Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
										"replicas": {
											Type: "integer", Minimum: float64Ptr(1),
										},
									},
								},
								"Chief": {
									Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
										"replicas": {
											Type:    "integer",
											Minimum: float64Ptr(1),
											Maximum: float64Ptr(1),
										},
									},
								},
								"Worker": {
									Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
										"replicas": {
											Type:    "integer",
											Minimum: float64Ptr(1),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})
}

func float64Ptr(f float64) *float64 {
	return &f
}

func (s *ContainersSuite) TestValidateMissingContainers(c *gc.C) {

	specStr := `
containers:
`[1:]

	_, err := provider.ParseK8sPodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, "require at least one container spec")
}

func (s *ContainersSuite) TestValidateMissingName(c *gc.C) {

	specStr := `
containers:
  - image: gitlab/latest
`[1:]

	spec, err := provider.ParseK8sPodSpec(specStr)
	c.Assert(err, jc.ErrorIsNil)
	err = spec.Validate()
	c.Assert(err, gc.ErrorMatches, "spec name is missing")
}

func (s *ContainersSuite) TestValidateMissingImage(c *gc.C) {

	specStr := `
containers:
  - name: gitlab
`[1:]

	spec, err := provider.ParseK8sPodSpec(specStr)
	c.Assert(err, jc.ErrorIsNil)
	err = spec.Validate()
	c.Assert(err, gc.ErrorMatches, "spec image details is missing")
}

func (s *ContainersSuite) TestValidateFileSetPath(c *gc.C) {

	specStr := `
containers:
  - name: gitlab
    image: gitlab/latest
    files:
      - files:
          file1: |-
            [config]
            foo: bar
`[1:]

	spec, err := provider.ParseK8sPodSpec(specStr)
	c.Assert(err, jc.ErrorIsNil)
	err = spec.Validate()
	c.Assert(err, gc.ErrorMatches, `file set name is missing`)
}

func (s *ContainersSuite) TestValidateMissingMountPath(c *gc.C) {

	specStr := `
containers:
  - name: gitlab
    image: gitlab/latest
    files:
      - name: configuration
        files:
          file1: |-
            [config]
            foo: bar
`[1:]

	spec, err := provider.ParseK8sPodSpec(specStr)
	c.Assert(err, jc.ErrorIsNil)
	err = spec.Validate()
	c.Assert(err, gc.ErrorMatches, `mount path is missing for file set "configuration"`)
}
