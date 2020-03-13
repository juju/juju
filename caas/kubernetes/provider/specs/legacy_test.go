// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"

	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/caas/specs"
	"github.com/juju/juju/testing"
)

type legacySpecsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&legacySpecsSuite{})

func (s *legacySpecsSuite) TestParse(c *gc.C) {

	specStrBase := `
omitServiceFrontend: true
activeDeadlineSeconds: 10
restartPolicy: OnFailure
terminationGracePeriodSeconds: 20
automountServiceAccountToken: true
serviceAccountName: serviceAccountFoo
securityContext:
  runAsNonRoot: true
  supplementalGroups: [1,2]
dnsPolicy: ClusterFirstWithHostNet
readinessGates:
  - conditionType: PodScheduled
containers:
  - name: gitlab
    image: gitlab/latest
    imagePullPolicy: Always
    command:
      - sh
      - -c
      - |
        set -ex
        echo "do some stuff here for gitlab container"
    args: ["doIt", "--debug"]
    workingDir: "/path/to/here"
    ports:
    - containerPort: 80
      name: fred
      protocol: TCP
    - containerPort: 443
      name: mary
    securityContext:
      runAsNonRoot: true
      privileged: true
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
      attr: foo=bar; name["fred"]="blogs";
      foo: bar
      brackets: '["hello", "world"]'
      restricted: 'yes'
      switch: on
      special: p@ssword's
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
initContainers:
  - name: gitlab-init
    image: gitlab-init/latest
    imagePullPolicy: Always
    command:
      - sh
      - -c
      - |
        set -ex
        echo "do some stuff here for gitlab-init container"
    args: ["doIt", "--debug"]
    workingDir: "/path/to/here"
    ports:
    - containerPort: 80
      name: fred
      protocol: TCP
    - containerPort: 443
      name: mary
    config:
      brackets: '["hello", "world"]'
      foo: bar
      restricted: 'yes'
      switch: on
      special: p@ssword's
service:
  annotations:
    foo: bar
customResourceDefinitions:
  tfjobs.kubeflow.org:
    group: kubeflow.org
    version: v1alpha2
    scope: Namespaced
    names:
      plural: "tfjobs"
      singular: "tfjob"
      kind: TFJob
    validation:
      openAPIV3Schema:
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
	getExpectedPodSpecBase := func() *specs.PodSpec {
		pSpecs := &specs.PodSpec{}
		// always parse to latest version.
		pSpecs.Version = specs.CurrentVersion
		pSpecs.OmitServiceFrontend = true

		pSpecs.Containers = []specs.ContainerSpec{
			{
				Name:            "gitlab",
				Image:           "gitlab/latest",
				ImagePullPolicy: "Always",
				Command: []string{"sh", "-c", `
set -ex
echo "do some stuff here for gitlab container"
`[1:]},
				Args:       []string{"doIt", "--debug"},
				WorkingDir: "/path/to/here",
				Ports: []specs.ContainerPort{
					{ContainerPort: 80, Protocol: "TCP", Name: "fred"},
					{ContainerPort: 443, Name: "mary"},
				},
				EnvConfig: map[string]interface{}{
					"attr":       `foo=bar; name["fred"]="blogs";`,
					"foo":        "bar",
					"restricted": "yes",
					"switch":     true,
					"brackets":   `["hello", "world"]`,
					"special":    "p@ssword's",
				},
				VolumeConfig: []specs.FileSet{
					{
						Name:      "configuration",
						MountPath: "/var/lib/foo",
						VolumeSource: specs.VolumeSource{
							Files: []specs.File{
								{Path: "file1", Content: expectedFileContent},
							},
						},
					},
				},
				ProviderContainer: &k8sspecs.K8sContainerSpec{
					SecurityContext: &core.SecurityContext{
						RunAsNonRoot: boolPtr(true),
						Privileged:   boolPtr(true),
					},
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
				Ports: []specs.ContainerPort{
					{ContainerPort: 8080, Protocol: "TCP"},
				},
			}, {
				Name: "secret-image-user",
				ImageDetails: specs.ImageDetails{
					ImagePath: "staging.registry.org/testing/testing-image@sha256:deed-beef",
					Username:  "docker-registry",
					Password:  "hunter2",
				},
			}, {
				Name: "just-image-details",
				ImageDetails: specs.ImageDetails{
					ImagePath: "testing/no-secrets-needed@sha256:deed-beef",
				},
			},
			{
				Name:            "gitlab-init",
				Image:           "gitlab-init/latest",
				ImagePullPolicy: "Always",
				Init:            true,
				Command: []string{"sh", "-c", `
set -ex
echo "do some stuff here for gitlab-init container"
`[1:]},
				Args:       []string{"doIt", "--debug"},
				WorkingDir: "/path/to/here",
				Ports: []specs.ContainerPort{
					{ContainerPort: 80, Protocol: "TCP", Name: "fred"},
					{ContainerPort: 443, Name: "mary"},
				},
				EnvConfig: map[string]interface{}{
					"foo":        "bar",
					"restricted": "yes",
					"switch":     true,
					"brackets":   `["hello", "world"]`,
					"special":    "p@ssword's",
				},
			},
		}

		pSpecs.Service = &specs.ServiceSpec{
			Annotations: map[string]string{"foo": "bar"},
		}
		pSpecs.ProviderPod = &k8sspecs.K8sPodSpec{
			KubernetesResources: &k8sspecs.KubernetesResources{
				Pod: &k8sspecs.PodSpec{
					RestartPolicy:                 core.RestartPolicyOnFailure,
					ActiveDeadlineSeconds:         int64Ptr(10),
					TerminationGracePeriodSeconds: int64Ptr(20),
					SecurityContext: &core.PodSecurityContext{
						RunAsNonRoot:       boolPtr(true),
						SupplementalGroups: []int64{1, 2},
					},
					ReadinessGates: []core.PodReadinessGate{
						{ConditionType: core.PodScheduled},
					},
					DNSPolicy: "ClusterFirstWithHostNet",
				},
				CustomResourceDefinitions: []k8sspecs.K8sCustomResourceDefinitionSpec{
					{
						Name: "tfjobs.kubeflow.org",
						Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
							Group:   "kubeflow.org",
							Version: "v1alpha2",
							Scope:   "Namespaced",
							Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
								Kind:     "TFJob",
								Plural:   "tfjobs",
								Singular: "tfjob",
							},
							Validation: &apiextensionsv1beta1.CustomResourceValidation{
								OpenAPIV3Schema: &apiextensionsv1beta1.JSONSchemaProps{
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
					},
				},
			},
		}
		return pSpecs
	}

	spec, err := k8sspecs.ParsePodSpec(specStrBase)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spec, jc.DeepEquals, getExpectedPodSpecBase())
}

func (s *legacySpecsSuite) TestValidateMissingContainers(c *gc.C) {

	specStr := `
containers:
`[1:]

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, "require at least one container spec")
}

func (s *legacySpecsSuite) TestValidateMissingName(c *gc.C) {

	specStr := `
containers:
  - image: gitlab/latest
`[1:]

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, "spec name is missing")
}

func (s *legacySpecsSuite) TestValidateMissingImage(c *gc.C) {

	specStr := `
containers:
  - name: gitlab
`[1:]

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, "spec image details is missing")
}

func (s *legacySpecsSuite) TestValidateFileSetPath(c *gc.C) {

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

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, `file set name is missing`)
}

func (s *legacySpecsSuite) TestValidateMissingMountPath(c *gc.C) {

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

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, `mount path is missing for file set "configuration"`)
}

func (s *legacySpecsSuite) TestValidateCustomResourceDefinitions(c *gc.C) {
	specStr := `
containers:
  - name: gitlab-helper
    image: gitlab-helper/latest
    ports:
    - containerPort: 8080
      protocol: TCP
customResourceDefinitions:
  tfjobs.kubeflow.org:
    group: kubeflow.org
    version: v1alpha2
    scope: invalid-scope
    names:
      plural: "tfjobs"
      singular: "tfjob"
      kind: TFJob
    validation:
      openAPIV3Schema:
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

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, `custom resource definition "tfjobs.kubeflow.org" scope "invalid-scope" is not supported, please use "Namespaced" or "Cluster" scope`)
}

func (s *legacySpecsSuite) TestUnknownFieldError(c *gc.C) {
	specStr := `
containers:
  - name: gitlab-helper
    image: gitlab-helper/latest
    ports:
    - containerPort: 8080
      protocol: TCP
foo: a-bad-guy
`[1:]

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, `json: unknown field "foo"`)
}
