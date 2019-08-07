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
activeDeadlineSeconds: 10
serviceAccountName: serviceAccount
restartPolicy: OnFailure
terminationGracePeriodSeconds: 20
automountServiceAccountToken: true
securityContext:
  runAsNonRoot: true
  supplementalGroups: [1,2]
hostname: host
subdomain: sub
priorityClassName: top
priority: 30
dnsPolicy: ClusterFirstWithHostNet
dnsConfig: 
  nameservers: [ns1, ns2]
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
      foo: bar
      restricted: 'yes'
      switch: on
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

	spec, err := provider.ParsePodSpec(specStr)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spec, jc.DeepEquals, &caas.PodSpec{
		OmitServiceFrontend: true,
		ProviderPod: &provider.K8sPodSpec{
			ActiveDeadlineSeconds:         int64Ptr(10),
			ServiceAccountName:            "serviceAccount",
			RestartPolicy:                 core.RestartPolicyOnFailure,
			TerminationGracePeriodSeconds: int64Ptr(20),
			AutomountServiceAccountToken:  boolPtr(true),
			SecurityContext: &core.PodSecurityContext{
				RunAsNonRoot:       boolPtr(true),
				SupplementalGroups: []int64{1, 2},
			},
			Hostname:          "host",
			Subdomain:         "sub",
			PriorityClassName: "top",
			Priority:          int32Ptr(30),
			DNSConfig: &core.PodDNSConfig{
				Nameservers: []string{"ns1", "ns2"},
			},
			DNSPolicy: "ClusterFirstWithHostNet",
			ReadinessGates: []core.PodReadinessGate{
				{ConditionType: core.PodScheduled},
			},
			Service: &provider.K8sServiceSpec{
				Annotations: map[string]string{"foo": "bar"},
			},
		},
		Containers: []caas.ContainerSpec{{
			Name:  "gitlab",
			Image: "gitlab/latest",
			Command: []string{"sh", "-c", `
set -ex
echo "do some stuff here for gitlab container"
`[1:]},
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
		InitContainers: []caas.ContainerSpec{{
			Name:  "gitlab-init",
			Image: "gitlab-init/latest",
			Command: []string{"sh", "-c", `
set -ex
echo "do some stuff here for gitlab-init container"
`[1:]},
			Args:       []string{"doIt", "--debug"},
			WorkingDir: "/path/to/here",
			Ports: []caas.ContainerPort{
				{ContainerPort: 80, Protocol: "TCP", Name: "fred"},
				{ContainerPort: 443, Name: "mary"},
			},
			Config: map[string]interface{}{
				"foo":        "bar",
				"restricted": "'yes'",
				"switch":     true,
			},
			ProviderContainer: &provider.K8sContainerSpec{
				ImagePullPolicy: "Always",
			},
		}},
		CustomResourceDefinitions: map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec{
			"tfjobs.kubeflow.org": {
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
	})
}

func float64Ptr(f float64) *float64 {
	return &f
}

func int32Ptr(i int32) *int32 {
	return &i
}

func int64Ptr(i int64) *int64 {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}

func (s *ContainersSuite) TestValidateMissingContainers(c *gc.C) {

	specStr := `
containers:
`[1:]

	_, err := provider.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, "require at least one container spec")
}

func (s *ContainersSuite) TestValidateMissingName(c *gc.C) {

	specStr := `
containers:
  - image: gitlab/latest
`[1:]

	_, err := provider.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, "spec name is missing")
}

func (s *ContainersSuite) TestValidateMissingImage(c *gc.C) {

	specStr := `
containers:
  - name: gitlab
`[1:]

	_, err := provider.ParsePodSpec(specStr)
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

	_, err := provider.ParsePodSpec(specStr)
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

	_, err := provider.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, `mount path is missing for file set "configuration"`)
}
