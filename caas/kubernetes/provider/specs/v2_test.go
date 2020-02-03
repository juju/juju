// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs_test

import (
	"encoding/base64"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	core "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"

	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/caas/specs"
	"github.com/juju/juju/testing"
)

type v2SpecsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&v2SpecsSuite{})

var versionHeader = `
version: 2
`[1:]

func (s *v2SpecsSuite) TestParse(c *gc.C) {

	specStrBase := versionHeader + `
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
    kubernetes:
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
  - name: gitlab-init
    image: gitlab-init/latest
    imagePullPolicy: Always
    init: true
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
configMaps:
  mydata:
    foo: bar
    hello: world
service:
  scalePolicy: serial
  annotations:
    foo: bar
serviceAccount:
  automountServiceAccountToken: true
  global: true
  rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "watch", "list"]
kubernetesResources:
  serviceAccounts:
  - name: k8sServiceAccount1
    automountServiceAccountToken: true
    global: true
    rules:
    - apiGroups: [""]
      resources: ["pods"]
      verbs: ["get", "watch", "list"]
    - nonResourceURLs: ["/healthz", "/healthz/*"] # '*' in a nonResourceURL is a suffix glob match
      verbs: ["get", "post"]
    - apiGroups: ["rbac.authorization.k8s.io"]
      resources: ["clusterroles"]
      verbs: ["bind"]
      resourceNames: ["admin","edit","view"]
  pod:
    restartPolicy: OnFailure
    activeDeadlineSeconds: 10
    terminationGracePeriodSeconds: 20
    securityContext:
      runAsNonRoot: true
      supplementalGroups: [1,2]
    readinessGates:
      - conditionType: PodScheduled
    dnsPolicy: ClusterFirstWithHostNet
  secrets:
    - name: build-robot-secret
      type: Opaque
      stringData:
          config.yaml: |-
              apiUrl: "https://my.api.com/api/v1"
              username: fred
              password: shhhh
    - name: another-build-robot-secret
      type: Opaque
      data:
          username: YWRtaW4=
          password: MWYyZDFlMmU2N2Rm
  customResourceDefinitions:
    tfjobs.kubeflow.org:
      group: kubeflow.org
      scope: Cluster
      names:
        kind: TFJob
        singular: tfjob
        plural: tfjobs
      version: v1
      versions:
      - name: v1
        served: true
        storage: true
      - name: v1beta2
        served: true
        storage: false
      validation:
        openAPIV3Schema:
          properties:
            spec:
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
  customResources:
    tfjobs.kubeflow.org:
      - apiVersion: "kubeflow.org/v1"
        kind: "TFJob"
        metadata:
          name: "dist-mnist-for-e2e-test"
        spec:
          tfReplicaSpecs:
            PS:
              replicas: 2
              restartPolicy: Never
              template:
                spec:
                  containers:
                    - name: tensorflow
                      image: kubeflow/tf-dist-mnist-test:1.0
            Worker:
              replicas: 4
              restartPolicy: Never
              template:
                spec:
                  containers:
                    - name: tensorflow
                      image: kubeflow/tf-dist-mnist-test:1.0
  ingressResources:
    - name: test-ingress
      labels:
        foo: bar
      annotations:
        nginx.ingress.kubernetes.io/rewrite-target: /
      spec:
        rules:
        - http:
            paths:
            - path: /testpath
              backend:
                serviceName: test
                servicePort: 80
  mutatingWebhookConfigurations:
    example-mutatingwebhookconfiguration:
      - name: "example.mutatingwebhookconfiguration.com"
        failurePolicy: Ignore
        clientConfig:
          service:
            name: apple-service
            namespace: apples
            path: /apple
          caBundle: "YXBwbGVz"
        namespaceSelector:
          matchExpressions:
          - key: production
            operator: DoesNotExist
        rules:
        - apiGroups:
          - ""
          apiVersions:
          - v1
          operations:
          - CREATE
          - UPDATE
          resources:
          - pods
`[1:]

	expectedFileContent := `
[config]
foo: bar
`[1:]

	sa1 := &specs.ServiceAccountSpec{}
	sa1.AutomountServiceAccountToken = boolPtr(true)
	sa1.Global = true
	sa1.Rules = []specs.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"get", "watch", "list"},
		},
	}
	getExpectedPodSpecBase := func() *specs.PodSpec {
		pSpecs := &specs.PodSpec{ServiceAccount: sa1}
		pSpecs.Service = &specs.ServiceSpec{
			ScalePolicy: "serial",
			Annotations: map[string]string{"foo": "bar"},
		}
		pSpecs.ConfigMaps = map[string]specs.ConfigMap{
			"mydata": {
				"foo":   "bar",
				"hello": "world",
			},
		}
		// always parse to latest version.
		pSpecs.Version = specs.CurrentVersion
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
				Config: map[string]interface{}{
					"attr":       `'foo=bar; name["fred"]="blogs";'`,
					"foo":        "bar",
					"restricted": "'yes'",
					"switch":     true,
					"brackets":   `'["hello", "world"]'`,
					"special":    "'p@ssword''s'",
				},
				Files: []specs.FileSet{
					{
						Name:      "configuration",
						MountPath: "/var/lib/foo",
						Files: map[string]string{
							"file1": expectedFileContent,
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
				Config: map[string]interface{}{
					"foo":        "bar",
					"restricted": "'yes'",
					"switch":     true,
					"brackets":   `'["hello", "world"]'`,
					"special":    "'p@ssword''s'",
				},
			},
		}

		sa2 := k8sspecs.K8sServiceAccountSpec{
			Name: "k8sServiceAccount1",
		}
		sa2.Global = true
		sa2.AutomountServiceAccountToken = boolPtr(true)
		sa2.Rules = []specs.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "watch", "list"},
			},
			{
				NonResourceURLs: []string{"/healthz", "/healthz/*"},
				Verbs:           []string{"get", "post"},
			},
			{
				APIGroups:     []string{"rbac.authorization.k8s.io"},
				Resources:     []string{"clusterroles"},
				Verbs:         []string{"bind"},
				ResourceNames: []string{"admin", "edit", "view"},
			},
		}

		ingress1Rule1 := extensionsv1beta1.IngressRule{
			IngressRuleValue: extensionsv1beta1.IngressRuleValue{
				HTTP: &extensionsv1beta1.HTTPIngressRuleValue{
					Paths: []extensionsv1beta1.HTTPIngressPath{
						{
							Path: "/testpath",
							Backend: extensionsv1beta1.IngressBackend{
								ServiceName: "test",
								ServicePort: intstr.IntOrString{IntVal: 80},
							},
						},
					},
				},
			},
		}
		ingress1 := k8sspecs.K8sIngressSpec{
			Name: "test-ingress",
			Labels: map[string]string{
				"foo": "bar",
			},
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/rewrite-target": "/",
			},
			Spec: extensionsv1beta1.IngressSpec{
				Rules: []extensionsv1beta1.IngressRule{ingress1Rule1},
			},
		}

		webhook1Rule1 := admissionregistrationv1beta1.Rule{
			APIGroups:   []string{""},
			APIVersions: []string{"v1"},
			Resources:   []string{"pods"},
		}
		webhookRuleWithOperations1 := admissionregistrationv1beta1.RuleWithOperations{
			Operations: []admissionregistrationv1beta1.OperationType{
				admissionregistrationv1beta1.Create,
				admissionregistrationv1beta1.Update,
			},
		}
		webhookRuleWithOperations1.Rule = webhook1Rule1
		CABundle, err := base64.StdEncoding.DecodeString("YXBwbGVz")
		c.Assert(err, jc.ErrorIsNil)
		webhook1FailurePolicy := admissionregistrationv1beta1.Ignore
		webhook1 := admissionregistrationv1beta1.Webhook{
			Name:          "example.mutatingwebhookconfiguration.com",
			FailurePolicy: &webhook1FailurePolicy,
			ClientConfig: admissionregistrationv1beta1.WebhookClientConfig{
				Service: &admissionregistrationv1beta1.ServiceReference{
					Name:      "apple-service",
					Namespace: "apples",
					Path:      strPtr("/apple"),
				},
				CABundle: CABundle,
			},
			NamespaceSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{Key: "production", Operator: metav1.LabelSelectorOpDoesNotExist},
				},
			},
			Rules: []admissionregistrationv1beta1.RuleWithOperations{webhookRuleWithOperations1},
		}

		pSpecs.ProviderPod = &k8sspecs.K8sPodSpec{
			KubernetesResources: &k8sspecs.KubernetesResources{
				ServiceAccounts: []k8sspecs.K8sServiceAccountSpec{
					sa2,
				},
				Pod: &k8sspecs.PodSpec{
					ActiveDeadlineSeconds:         int64Ptr(10),
					RestartPolicy:                 core.RestartPolicyOnFailure,
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
				Secrets: []k8sspecs.Secret{
					{
						Name: "build-robot-secret",
						Type: core.SecretTypeOpaque,
						StringData: map[string]string{
							"config.yaml": `
apiUrl: "https://my.api.com/api/v1"
username: fred
password: shhhh`[1:],
						},
					},
					{
						Name: "another-build-robot-secret",
						Type: core.SecretTypeOpaque,
						Data: map[string]string{
							"username": "YWRtaW4=",
							"password": "MWYyZDFlMmU2N2Rm",
						},
					},
				},
				CustomResourceDefinitions: map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec{
					"tfjobs.kubeflow.org": {
						Group:   "kubeflow.org",
						Version: "v1",
						Versions: []apiextensionsv1beta1.CustomResourceDefinitionVersion{
							{Name: "v1", Served: true, Storage: true},
							{Name: "v1beta2", Served: true, Storage: false},
						},
						Scope: "Cluster",
						Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
							Kind:     "TFJob",
							Plural:   "tfjobs",
							Singular: "tfjob",
						},
						Validation: &apiextensionsv1beta1.CustomResourceValidation{
							OpenAPIV3Schema: &apiextensionsv1beta1.JSONSchemaProps{
								Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
									"spec": {
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
				CustomResources: map[string][]unstructured.Unstructured{
					"tfjobs.kubeflow.org": {
						{
							Object: map[string]interface{}{
								"apiVersion": "kubeflow.org/v1",
								"metadata": map[string]interface{}{
									"name": "dist-mnist-for-e2e-test",
								},
								"kind": "TFJob",
								"spec": map[string]interface{}{
									"tfReplicaSpecs": map[string]interface{}{
										"PS": map[string]interface{}{
											"replicas":      int64(2),
											"restartPolicy": "Never",
											"template": map[string]interface{}{
												"spec": map[string]interface{}{
													"containers": []interface{}{
														map[string]interface{}{
															"name":  "tensorflow",
															"image": "kubeflow/tf-dist-mnist-test:1.0",
														},
													},
												},
											},
										},
										"Worker": map[string]interface{}{
											"replicas":      int64(4),
											"restartPolicy": "Never",
											"template": map[string]interface{}{
												"spec": map[string]interface{}{
													"containers": []interface{}{
														map[string]interface{}{
															"name":  "tensorflow",
															"image": "kubeflow/tf-dist-mnist-test:1.0",
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
				IngressResources: []k8sspecs.K8sIngressSpec{ingress1},
				MutatingWebhookConfigurations: map[string][]admissionregistrationv1beta1.Webhook{
					"example-mutatingwebhookconfiguration": {webhook1},
				},
			},
		}
		return pSpecs
	}

	spec, err := k8sspecs.ParsePodSpec(specStrBase)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spec, jc.DeepEquals, getExpectedPodSpecBase())
}

func (s *v2SpecsSuite) TestValidateMissingContainers(c *gc.C) {

	specStr := versionHeader + `
containers:
`[1:]

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, "require at least one container spec")
}

func (s *v2SpecsSuite) TestValidateMissingName(c *gc.C) {

	specStr := versionHeader + `
containers:
  - image: gitlab/latest
`[1:]

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, "spec name is missing")
}

func (s *v2SpecsSuite) TestValidateMissingImage(c *gc.C) {

	specStr := versionHeader + `
containers:
  - name: gitlab
`[1:]

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, "spec image details is missing")
}

func (s *v2SpecsSuite) TestValidateFileSetPath(c *gc.C) {

	specStr := versionHeader + `
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

func (s *v2SpecsSuite) TestValidateMissingMountPath(c *gc.C) {

	specStr := versionHeader + `
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

func (s *v2SpecsSuite) TestValidateServiceAccountShouldBeOmittedForEmptyValue(c *gc.C) {
	specStr := versionHeader + `
containers:
  - name: gitlab-helper
    image: gitlab-helper/latest
    ports:
    - containerPort: 8080
      protocol: TCP
serviceAccount:
  automountServiceAccountToken: true
`[1:]

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, `rules is required`)
}

func (s *v2SpecsSuite) TestValidateCustomResourceDefinitions(c *gc.C) {
	specStr := versionHeader + `
containers:
  - name: gitlab-helper
    image: gitlab-helper/latest
    ports:
    - containerPort: 8080
      protocol: TCP
kubernetesResources:
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

func (s *v2SpecsSuite) TestValidateMutatingWebhookConfigurations(c *gc.C) {
	specStr := versionHeader + `
containers:
  - name: gitlab-helper
    image: gitlab-helper/latest
    ports:
    - containerPort: 8080
      protocol: TCP
kubernetesResources:
  mutatingWebhookConfigurations:
    example-mutatingwebhookconfiguration:
`[1:]

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, `empty webhooks "example-mutatingwebhookconfiguration" not valid`)
}

func (s *v2SpecsSuite) TestValidateIngressResources(c *gc.C) {
	specStr := versionHeader + `
containers:
  - name: gitlab-helper
    image: gitlab-helper/latest
    ports:
    - containerPort: 8080
      protocol: TCP
kubernetesResources:
  ingressResources:
    - labels:
        foo: bar
      annotations:
        nginx.ingress.kubernetes.io/rewrite-target: /
      spec:
        rules:
        - http:
            paths:
            - path: /testpath
              backend:
                serviceName: test
                servicePort: 80
`[1:]

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, `ingress name is missing`)
}

func (s *v2SpecsSuite) TestUnknownFieldError(c *gc.C) {
	specStr := versionHeader + `
containers:
  - name: gitlab-helper
    image: gitlab-helper/latest
    ports:
    - containerPort: 8080
      protocol: TCP
bar: a-bad-guy
`[1:]

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, `json: unknown field "bar"`)
}

// TODO(caas): move these pointer related value change funcs to /testing package.
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

func strPtr(b string) *string {
	return &b
}
