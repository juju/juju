// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs_test

import (
	"encoding/base64"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	admissionregistration "k8s.io/api/admissionregistration/v1beta1"
	core "k8s.io/api/core/v1"
	networkingv1beta1 "k8s.io/api/networking/v1beta1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"

	k8sspecs "github.com/juju/juju/v3/caas/kubernetes/provider/specs"
	"github.com/juju/juju/v3/caas/specs"
	"github.com/juju/juju/v3/testing"
)

type v2SpecsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&v2SpecsSuite{})

var version2Header = `
version: 2
`[1:]

func (s *v2SpecsSuite) TestParse(c *gc.C) {

	specStrBase := version2Header + `
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
      startupProbe:
        httpGet:
          path: /healthz
          port: liveness-port
        failureThreshold: 30
        periodSeconds: 10
    config:
      attr: foo=bar; name["fred"]="blogs";
      foo: bar
      brackets: '["hello", "world"]'
      restricted: 'yes'
      switch: on
      special: p@ssword's
      my-resource-limit:
        resource:
          container-name: container1
          resource: requests.cpu
          divisor: 1m
    files:
      - name: configuration
        mountPath: /var/lib/foo
        files:
          file1: |
            [config]
            foo: bar
          file: |
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
  annotations:
    foo: bar
  scalePolicy: serial
  updateStrategy:
    type: Recreate
    rollingUpdate:
      maxUnavailable: 10%
      maxSurge: 25%
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
    annotations:
      foo: baz
    labels:
      foo: bax
    restartPolicy: OnFailure
    activeDeadlineSeconds: 10
    terminationGracePeriodSeconds: 20
    securityContext:
      runAsNonRoot: true
      supplementalGroups: [1,2]
    readinessGates:
      - conditionType: PodScheduled
    dnsPolicy: ClusterFirstWithHostNet
    hostNetwork: true
    hostPID: true
    priorityClassName: system-cluster-critical
    priority: 2000000000
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
      conversion:
        strategy: None
      preserveUnknownFields: false
      additionalPrinterColumns:
      - name: Worker
        type: integer
        description: Worker attribute.
        jsonPath: .spec.tfReplicaSpecs.Worker
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
  validatingWebhookConfigurations:
    pod-policy.example.com:
      - name: "pod-policy.example.com"
        rules:
        - apiGroups:   [""]
          apiVersions: ["v1"]
          operations:  ["CREATE"]
          resources:   ["pods"]
          scope:       "Namespaced"
        clientConfig:
          service:
            namespace: "example-namespace"
            name: "example-service"
          caBundle: "YXBwbGVz"
        admissionReviewVersions: ["v1", "v1beta1"]
        sideEffects: None
        timeoutSeconds: 5
`[1:]

	expectedFileContent := `
[config]
foo: bar
`[1:]

	sa1 := &specs.PrimeServiceAccountSpecV3{
		ServiceAccountSpecV3: specs.ServiceAccountSpecV3{
			AutomountServiceAccountToken: pointer.BoolPtr(true),
			Roles: []specs.Role{
				{
					Global: true,
					Rules: []specs.PolicyRule{
						{
							APIGroups: []string{""},
							Resources: []string{"pods"},
							Verbs:     []string{"get", "watch", "list"},
						},
					},
				},
			},
		},
	}

	getExpectedPodSpecBase := func() *specs.PodSpec {
		pSpecs := &specs.PodSpec{ServiceAccount: sa1}
		pSpecs.Service = &specs.ServiceSpec{
			Annotations: map[string]string{"foo": "bar"},
			ScalePolicy: "serial",
			UpdateStrategy: &specs.UpdateStrategy{
				Type: "Recreate",
				RollingUpdate: &specs.RollingUpdateSpec{
					MaxUnavailable: &specs.IntOrString{Type: specs.String, StrVal: "10%"},
					MaxSurge:       &specs.IntOrString{Type: specs.String, StrVal: "25%"},
				},
			},
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
				EnvConfig: map[string]interface{}{
					"attr":       `foo=bar; name["fred"]="blogs";`,
					"foo":        "bar",
					"restricted": "yes",
					"switch":     true,
					"brackets":   `["hello", "world"]`,
					"special":    "p@ssword's",
					"my-resource-limit": map[string]interface{}{
						"resource": map[string]interface{}{
							"container-name": "container1",
							"resource":       "requests.cpu",
							"divisor":        "1m",
						},
					},
				},
				VolumeConfig: []specs.FileSet{
					{
						Name:      "configuration",
						MountPath: "/var/lib/foo",
						VolumeSource: specs.VolumeSource{
							Files: []specs.File{
								{Path: "file", Content: expectedFileContent},
								{Path: "file1", Content: expectedFileContent},
							},
						},
					},
				},
				ProviderContainer: &k8sspecs.K8sContainerSpec{
					SecurityContext: &core.SecurityContext{
						RunAsNonRoot: pointer.BoolPtr(true),
						Privileged:   pointer.BoolPtr(true),
					},
					LivenessProbe: &core.Probe{
						InitialDelaySeconds: 10,
						ProbeHandler: core.ProbeHandler{
							HTTPGet: &core.HTTPGetAction{
								Path: "/ping",
								Port: intstr.IntOrString{IntVal: 8080},
							},
						},
					},
					ReadinessProbe: &core.Probe{
						InitialDelaySeconds: 10,
						ProbeHandler: core.ProbeHandler{
							HTTPGet: &core.HTTPGetAction{
								Path: "/pingReady",
								Port: intstr.IntOrString{StrVal: "www", Type: 1},
							},
						},
					},
					StartupProbe: &core.Probe{
						PeriodSeconds:    10,
						FailureThreshold: 30,
						ProbeHandler: core.ProbeHandler{
							HTTPGet: &core.HTTPGetAction{
								Path: "/healthz",
								Port: intstr.IntOrString{
									Type:   intstr.String,
									StrVal: "liveness-port",
								},
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

		rbacResources := k8sspecs.K8sRBACResources{
			ServiceAccounts: []k8sspecs.K8sServiceAccountSpec{
				{
					Name: "k8sServiceAccount1",
					ServiceAccountSpecV3: specs.ServiceAccountSpecV3{
						AutomountServiceAccountToken: pointer.BoolPtr(true),
						Roles: []specs.Role{
							{
								Global: true,
								Rules: []specs.PolicyRule{
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
								},
							},
						},
					},
				},
			},
		}

		ingress1Rule1 := networkingv1beta1.IngressRule{
			IngressRuleValue: networkingv1beta1.IngressRuleValue{
				HTTP: &networkingv1beta1.HTTPIngressRuleValue{
					Paths: []networkingv1beta1.HTTPIngressPath{
						{
							Path: "/testpath",
							Backend: networkingv1beta1.IngressBackend{
								ServiceName: "test",
								ServicePort: intstr.IntOrString{IntVal: 80},
							},
						},
					},
				},
			},
		}
		ingress1 := k8sspecs.K8sIngress{
			Meta: k8sspecs.Meta{
				Name: "test-ingress",
				Labels: map[string]string{
					"foo": "bar",
				},
				Annotations: map[string]string{
					"nginx.ingress.kubernetes.io/rewrite-target": "/",
				},
			},
			Spec: k8sspecs.K8sIngressSpec{
				Version: k8sspecs.K8sIngressV1Beta1,
				SpecV1Beta1: networkingv1beta1.IngressSpec{
					Rules: []networkingv1beta1.IngressRule{ingress1Rule1},
				},
			},
		}

		webhookRule1 := admissionregistration.Rule{
			APIGroups:   []string{""},
			APIVersions: []string{"v1"},
			Resources:   []string{"pods"},
		}
		webhookRuleWithOperations1 := admissionregistration.RuleWithOperations{
			Operations: []admissionregistration.OperationType{
				admissionregistration.Create,
				admissionregistration.Update,
			},
		}
		webhookRuleWithOperations1.Rule = webhookRule1
		CABundle1, err := base64.StdEncoding.DecodeString("YXBwbGVz")
		c.Assert(err, jc.ErrorIsNil)
		webhookFailurePolicy1 := admissionregistration.Ignore
		webhook1 := admissionregistration.MutatingWebhook{
			Name:          "example.mutatingwebhookconfiguration.com",
			FailurePolicy: &webhookFailurePolicy1,
			ClientConfig: admissionregistration.WebhookClientConfig{
				Service: &admissionregistration.ServiceReference{
					Name:      "apple-service",
					Namespace: "apples",
					Path:      pointer.StringPtr("/apple"),
				},
				CABundle: CABundle1,
			},
			NamespaceSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{Key: "production", Operator: metav1.LabelSelectorOpDoesNotExist},
				},
			},
			Rules: []admissionregistration.RuleWithOperations{webhookRuleWithOperations1},
		}

		scope := admissionregistration.NamespacedScope
		webhookRule2 := admissionregistration.Rule{
			APIGroups:   []string{""},
			APIVersions: []string{"v1"},
			Resources:   []string{"pods"},
			Scope:       &scope,
		}
		webhookRuleWithOperations2 := admissionregistration.RuleWithOperations{
			Operations: []admissionregistration.OperationType{
				admissionregistration.Create,
			},
		}
		webhookRuleWithOperations2.Rule = webhookRule2
		sideEffects := admissionregistration.SideEffectClassNone
		webhook2 := admissionregistration.ValidatingWebhook{
			Name:  "pod-policy.example.com",
			Rules: []admissionregistration.RuleWithOperations{webhookRuleWithOperations2},
			ClientConfig: admissionregistration.WebhookClientConfig{
				Service: &admissionregistration.ServiceReference{
					Name:      "example-service",
					Namespace: "example-namespace",
				},
				CABundle: CABundle1,
			},
			AdmissionReviewVersions: []string{"v1", "v1beta1"},
			SideEffects:             &sideEffects,
			TimeoutSeconds:          pointer.Int32Ptr(5),
		}

		pSpecs.ProviderPod = &k8sspecs.K8sPodSpec{
			KubernetesResources: &k8sspecs.KubernetesResources{
				K8sRBACResources: rbacResources,
				Pod: &k8sspecs.PodSpec{
					Labels:                        map[string]string{"foo": "bax"},
					Annotations:                   map[string]string{"foo": "baz"},
					ActiveDeadlineSeconds:         pointer.Int64Ptr(10),
					RestartPolicy:                 core.RestartPolicyOnFailure,
					TerminationGracePeriodSeconds: pointer.Int64Ptr(20),
					SecurityContext: &core.PodSecurityContext{
						RunAsNonRoot:       pointer.BoolPtr(true),
						SupplementalGroups: []int64{1, 2},
					},
					ReadinessGates: []core.PodReadinessGate{
						{ConditionType: core.PodScheduled},
					},
					DNSPolicy:         "ClusterFirstWithHostNet",
					HostNetwork:       true,
					HostPID:           true,
					PriorityClassName: "system-cluster-critical",
					Priority:          pointer.Int32Ptr(2000000000),
				},
				Secrets: []k8sspecs.K8sSecret{
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
				CustomResourceDefinitions: []k8sspecs.K8sCustomResourceDefinition{
					{
						Meta: k8sspecs.Meta{Name: "tfjobs.kubeflow.org"},
						Spec: k8sspecs.K8sCustomResourceDefinitionSpec{
							Version: k8sspecs.K8sCustomResourceDefinitionV1Beta1,
							SpecV1Beta1: apiextensionsv1beta1.CustomResourceDefinitionSpec{
								Group:   "kubeflow.org",
								Version: "v1",
								Versions: []apiextensionsv1beta1.CustomResourceDefinitionVersion{
									{Name: "v1", Served: true, Storage: true},
									{Name: "v1beta2", Served: true, Storage: false},
								},
								Scope:                 "Cluster",
								PreserveUnknownFields: pointer.BoolPtr(false),
								Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
									Kind:     "TFJob",
									Plural:   "tfjobs",
									Singular: "tfjob",
								},
								Conversion: &apiextensionsv1beta1.CustomResourceConversion{
									Strategy: apiextensionsv1beta1.NoneConverter,
								},
								AdditionalPrinterColumns: []apiextensionsv1beta1.CustomResourceColumnDefinition{
									{
										Name:        "Worker",
										Type:        "integer",
										Description: "Worker attribute.",
										JSONPath:    ".spec.tfReplicaSpecs.Worker",
									},
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
																		Type: "integer", Minimum: pointer.Float64Ptr(1),
																	},
																},
															},
															"Chief": {
																Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
																	"replicas": {
																		Type:    "integer",
																		Minimum: pointer.Float64Ptr(1),
																		Maximum: pointer.Float64Ptr(1),
																	},
																},
															},
															"Worker": {
																Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
																	"replicas": {
																		Type:    "integer",
																		Minimum: pointer.Float64Ptr(1),
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
				IngressResources: []k8sspecs.K8sIngress{ingress1},
				MutatingWebhookConfigurations: []k8sspecs.K8sMutatingWebhook{
					{
						Meta: k8sspecs.Meta{Name: "example-mutatingwebhookconfiguration"},
						Webhooks: []k8sspecs.K8sMutatingWebhookSpec{
							{
								Version:     k8sspecs.K8sWebhookV1Beta1,
								SpecV1Beta1: webhook1,
							},
						},
					},
				},
				ValidatingWebhookConfigurations: []k8sspecs.K8sValidatingWebhook{
					{
						Meta: k8sspecs.Meta{Name: "pod-policy.example.com"},
						Webhooks: []k8sspecs.K8sValidatingWebhookSpec{
							{
								Version:     k8sspecs.K8sWebhookV1Beta1,
								SpecV1Beta1: webhook2,
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

func (s *v2SpecsSuite) TestValidateMissingContainers(c *gc.C) {

	specStr := version2Header + `
containers:
`[1:]

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, "require at least one container spec")
}

func (s *v2SpecsSuite) TestValidateMissingName(c *gc.C) {

	specStr := version2Header + `
containers:
  - image: gitlab/latest
`[1:]

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, "spec name is missing")
}

func (s *v2SpecsSuite) TestValidateMissingImage(c *gc.C) {

	specStr := version2Header + `
containers:
  - name: gitlab
`[1:]

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, "spec image details is missing")
}

func (s *v2SpecsSuite) TestValidateFileSetPath(c *gc.C) {

	specStr := version2Header + `
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

	specStr := version2Header + `
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
	specStr := version2Header + `
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
	c.Assert(err, gc.ErrorMatches, `invalid primary service account: rules is required`)
}

func (s *v2SpecsSuite) TestValidateCustomResourceDefinitions(c *gc.C) {
	specStr := version2Header + `
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
	specStr := version2Header + `
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

func (s *v2SpecsSuite) TestValidateValidatingWebhookConfigurations(c *gc.C) {
	specStr := version2Header + `
containers:
  - name: gitlab-helper
    image: gitlab-helper/latest
    ports:
    - containerPort: 8080
      protocol: TCP
kubernetesResources:
  validatingWebhookConfigurations:
    example-validatingwebhookconfiguration:
`[1:]

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, `empty webhooks "example-validatingwebhookconfiguration" not valid`)
}

func (s *v2SpecsSuite) TestValidateIngressResources(c *gc.C) {
	specStr := version2Header + `
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
	c.Assert(err, gc.ErrorMatches, `name is missing`)

	specStr = version3Header + `
containers:
  - name: gitlab-helper
    image: gitlab-helper/latest
    ports:
    - containerPort: 8080
      protocol: TCP
kubernetesResources:
  ingressResources:
    - name: test-ingress
      labels:
        /foo: bar
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

	_, err = k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, `label key "/foo": prefix part must be non-empty not valid`)
}

func (s *v2SpecsSuite) TestUnknownFieldError(c *gc.C) {
	specStr := version2Header + `
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
