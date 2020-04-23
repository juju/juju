// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs_test

import (
	"encoding/base64"
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	admissionregistration "k8s.io/api/admissionregistration/v1beta1"
	core "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"

	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/caas/specs"
	"github.com/juju/juju/testing"
)

type v3SpecsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&v3SpecsSuite{})

var version3Header = `
version: 3
`[1:]

func (s *v3SpecsSuite) TestParse(c *gc.C) {

	specStrBase := version3Header + `
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
    envConfig:
      attr: foo=bar; name["fred"]="blogs";
      foo: bar
      brackets: '["hello", "world"]'
      restricted: "yes"
      switch: on
      special: p@ssword's
      my-resource-limit:
        resource:
          container-name: container1
          resource: requests.cpu
          divisor: 1m
    volumeConfig:
      - name: configuration
        mountPath: /var/lib/foo
        files:
          - path: file1
            mode: 644
            content: |
              [config]
              foo: bar
      - name: myhostpath
        mountPath: /host/etc/cni/net.d
        hostPath:
          path: /etc/cni/net.d
          type: Directory
      - name: cache-volume
        mountPath: /empty-dir
        emptyDir:
          medium: Memory
      - name: log_level
        mountPath: /log-config/log_level
        configMap:
          name: log-config
          defaultMode: 511
          files:
            - key: log_level
              path: log_level
              mode: 511
      - name: mysecret2
        mountPath: /secrets
        secret:
          name: mysecret2
          defaultMode: 511
          files:
            - key: password
              path: my-group/my-password
              mode: 511
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
    envConfig:
      brackets: '["hello", "world"]'
      foo: bar
      restricted: "yes"
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
  roles:
    - global: true
      rules:
        - apiGroups: [""]
          resources: ["pods"]
          verbs: ["get", "watch", "list"]
kubernetesResources:
  serviceAccounts:
    - name: k8sServiceAccount1
      automountServiceAccountToken: true
      roles:
        - name: k8sRole
          rules:
            - apiGroups: [""]
              resources: ["pods"]
              verbs: ["get", "watch", "list"]
            - nonResourceURLs: ["/healthz", "/healthz/*"] # '*' in a nonResourceURL is a suffix glob match
              verbs: ["get", "post"]
            - apiGroups: ["rbac.authorization.k8s.io"]
              resources: ["clusterroles"]
              verbs: ["bind"]
              resourceNames: ["admin", "edit", "view"]
        - name: k8sClusterRole
          global: true
          rules:
            - apiGroups: [""]
              resources: ["pods"]
              verbs: ["get", "watch", "list"]
  pod:
    restartPolicy: OnFailure
    activeDeadlineSeconds: 10
    terminationGracePeriodSeconds: 20
    securityContext:
      runAsNonRoot: true
      supplementalGroups: [1, 2]
    readinessGates:
      - conditionType: PodScheduled
    dnsPolicy: ClusterFirstWithHostNet
    hostNetwork: true
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
    - name: tfjobs.kubeflow.org
      labels:
        foo: bar
        juju-global-resource-lifecycle: model
      spec:
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
        labels:
          foo: bar
          juju-global-resource-lifecycle: model
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
    - name: example-mutatingwebhookconfiguration
      labels:
        foo: bar
      annotations:
        juju.io/disable-name-prefix: "true"
      webhooks:
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
    - name: pod-policy.example.com
      labels:
        foo: bar
      annotations:
        juju.io/disable-name-prefix: "true"
      webhooks:
        - name: "pod-policy.example.com"
          rules:
            - apiGroups: [""]
              apiVersions: ["v1"]
              operations: ["CREATE"]
              resources: ["pods"]
              scope: "Namespaced"
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
			AutomountServiceAccountToken: boolPtr(true),
			Roles: []specs.Role{
				{
					Rules: []specs.PolicyRule{
						{
							APIGroups: []string{""},
							Resources: []string{"pods"},
							Verbs:     []string{"get", "watch", "list"},
						},
					},
					Global: true,
				},
			},
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
								{
									Path:    "file1",
									Content: expectedFileContent,
									Mode:    int32Ptr(644),
								},
							},
						},
					},
					{
						Name:      "myhostpath",
						MountPath: "/host/etc/cni/net.d",
						VolumeSource: specs.VolumeSource{
							HostPath: &specs.HostPathVol{
								Path: "/etc/cni/net.d",
								Type: "Directory",
							},
						},
					},
					{
						Name:      "cache-volume",
						MountPath: "/empty-dir",
						VolumeSource: specs.VolumeSource{
							EmptyDir: &specs.EmptyDirVol{
								Medium: "Memory",
							},
						},
					},
					{
						Name:      "log_level",
						MountPath: "/log-config/log_level",
						VolumeSource: specs.VolumeSource{
							ConfigMap: &specs.ResourceRefVol{
								Name:        "log-config",
								DefaultMode: int32Ptr(511),
								Files: []specs.FileRef{
									{
										Key:  "log_level",
										Path: "log_level",
										Mode: int32Ptr(511),
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
								Name:        "mysecret2",
								DefaultMode: int32Ptr(511),
								Files: []specs.FileRef{
									{
										Key:  "password",
										Path: "my-group/my-password",
										Mode: int32Ptr(511),
									},
								},
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

		rbacResources := k8sspecs.K8sRBACResources{
			ServiceAccounts: []k8sspecs.K8sServiceAccountSpec{
				{
					Name: "k8sServiceAccount1",
					ServiceAccountSpecV3: specs.ServiceAccountSpecV3{
						AutomountServiceAccountToken: boolPtr(true),
						Roles: []specs.Role{
							{
								Name:   "k8sRole",
								Global: false,
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
							{
								Name:   "k8sClusterRole",
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
				},
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
					Path:      strPtr("/apple"),
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
			TimeoutSeconds:          int32Ptr(5),
		}

		pSpecs.ProviderPod = &k8sspecs.K8sPodSpec{
			KubernetesResources: &k8sspecs.KubernetesResources{
				K8sRBACResources: rbacResources,
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
					DNSPolicy:   "ClusterFirstWithHostNet",
					HostNetwork: true,
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
				CustomResourceDefinitions: []k8sspecs.K8sCustomResourceDefinitionSpec{
					{
						Meta: k8sspecs.Meta{
							Name: "tfjobs.kubeflow.org",
							Labels: map[string]string{
								"foo":                            "bar",
								"juju-global-resource-lifecycle": "model",
							},
						},
						Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
							Group:   "kubeflow.org",
							Version: "v1",
							Versions: []apiextensionsv1beta1.CustomResourceDefinitionVersion{
								{Name: "v1", Served: true, Storage: true},
								{Name: "v1beta2", Served: true, Storage: false},
							},
							Scope:                 "Cluster",
							PreserveUnknownFields: boolPtr(false),
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
				},
				CustomResources: map[string][]unstructured.Unstructured{
					"tfjobs.kubeflow.org": {
						{
							Object: map[string]interface{}{
								"apiVersion": "kubeflow.org/v1",
								"metadata": map[string]interface{}{
									"name": "dist-mnist-for-e2e-test",
								},
								"labels": map[string]interface{}{
									"foo":                            "bar",
									"juju-global-resource-lifecycle": "model",
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
				MutatingWebhookConfigurations: []k8sspecs.K8sMutatingWebhookSpec{
					{
						Meta: k8sspecs.Meta{
							Name:        "example-mutatingwebhookconfiguration",
							Labels:      map[string]string{"foo": "bar"},
							Annotations: map[string]string{"juju.io/disable-name-prefix": "true"},
						},
						Webhooks: []admissionregistration.MutatingWebhook{webhook1},
					},
				},
				ValidatingWebhookConfigurations: []k8sspecs.K8sValidatingWebhookSpec{
					{
						Meta: k8sspecs.Meta{
							Name:        "pod-policy.example.com",
							Labels:      map[string]string{"foo": "bar"},
							Annotations: map[string]string{"juju.io/disable-name-prefix": "true"},
						},
						Webhooks: []admissionregistration.ValidatingWebhook{webhook2},
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

func (s *v3SpecsSuite) TestValidateMissingContainers(c *gc.C) {

	specStr := version3Header + `
containers:
`[1:]

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, "require at least one container spec")
}

func (s *v3SpecsSuite) TestValidateMissingName(c *gc.C) {

	specStr := version3Header + `
containers:
  - image: gitlab/latest
`[1:]

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, "spec name is missing")
}

func (s *v3SpecsSuite) TestValidateMissingImage(c *gc.C) {

	specStr := version3Header + `
containers:
  - name: gitlab
`[1:]

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, "spec image details is missing")
}

func (s *v3SpecsSuite) TestValidateFileSetPath(c *gc.C) {

	specStr := version3Header + `
containers:
  - name: gitlab
    image: gitlab/latest
    volumeConfig:
      - files:
        - path: file1
          mode: 644
          content: |
            [config]
            foo: bar
`[1:]

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, `file set name is missing`)
}

func (s *v3SpecsSuite) TestValidateMissingMountPath(c *gc.C) {

	specStr := version3Header + `
containers:
  - name: gitlab
    image: gitlab/latest
    volumeConfig:
      - name: configuration
        files:
         - path: file1
           mode: 644
           content: |
             [config]
             foo: bar
`[1:]

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, `mount path is missing for file set "configuration"`)
}

func (s *v3SpecsSuite) TestValidateServiceAccountShouldBeOmittedForEmptyValue(c *gc.C) {
	specStr := version3Header + `
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
	c.Assert(err, gc.ErrorMatches, `roles is required`)
}

func (s *v3SpecsSuite) TestValidateCustomResourceDefinitions(c *gc.C) {
	specStr := version3Header + `
containers:
  - name: gitlab-helper
    image: gitlab-helper/latest
    ports:
      - containerPort: 8080
        protocol: TCP
kubernetesResources:
  customResourceDefinitions:
    - name: tfjobs.kubeflow.org
      spec:
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

	specStr = version3Header + `
containers:
  - name: gitlab-helper
    image: gitlab-helper/latest
    ports:
      - containerPort: 8080
        protocol: TCP
kubernetesResources:
  customResourceDefinitions:
    - name: tfjobs.kubeflow.org
      annotations:
        foo: bar
      labels:
        /foo: bar
      spec:
        group: kubeflow.org
        version: v1alpha2
        scope: Cluster
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

	_, err = k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, `label key "/foo": prefix part must be non-empty not valid`)
}

func (s *v3SpecsSuite) TestValidateMutatingWebhookConfigurations(c *gc.C) {
	specStr := version3Header + `
containers:
  - name: gitlab-helper
    image: gitlab-helper/latest
    ports:
    - containerPort: 8080
      protocol: TCP
kubernetesResources:
  mutatingWebhookConfigurations:
    - name: example-mutatingwebhookconfiguration
`[1:]

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, `empty webhooks "example-mutatingwebhookconfiguration" not valid`)
}

func (s *v3SpecsSuite) TestValidateValidatingWebhookConfigurations(c *gc.C) {
	specStr := version3Header + `
containers:
  - name: gitlab-helper
    image: gitlab-helper/latest
    ports:
    - containerPort: 8080
      protocol: TCP
kubernetesResources:
  validatingWebhookConfigurations:
    - name: example-validatingwebhookconfiguration
`[1:]

	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, `empty webhooks "example-validatingwebhookconfiguration" not valid`)
}

func (s *v3SpecsSuite) TestValidateIngressResources(c *gc.C) {
	specStr := version3Header + `
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

func (s *v3SpecsSuite) TestPrimeServiceAccountToK8sRBACResources(c *gc.C) {
	primeSA := specs.PrimeServiceAccountSpecV3{
		ServiceAccountSpecV3: specs.ServiceAccountSpecV3{
			AutomountServiceAccountToken: boolPtr(true),
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
	c.Assert(primeSA.Validate(), jc.ErrorIsNil)
	primeSA.SetName("test-app-rbac")
	c.Assert(primeSA.Validate(), jc.ErrorIsNil)
	c.Assert(primeSA.GetName(), gc.DeepEquals, "test-app-rbac")
	c.Assert(primeSA.Roles[0].Name, gc.DeepEquals, "test-app-rbac")

	sa, err := k8sspecs.PrimeServiceAccountToK8sRBACResources(primeSA)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sa.Validate(), jc.ErrorIsNil)
	c.Assert(sa, gc.DeepEquals, &k8sspecs.K8sRBACResources{
		ServiceAccounts: []k8sspecs.K8sServiceAccountSpec{
			{
				Name: "test-app-rbac",
				ServiceAccountSpecV3: specs.ServiceAccountSpecV3{
					AutomountServiceAccountToken: boolPtr(true),
					Roles: []specs.Role{
						{
							Name:   "test-app-rbac",
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
			},
		},
	})
}

type tcK8sRBACResources struct {
	Spec   k8sspecs.K8sRBACResources
	ErrStr string
}

func (s *v3SpecsSuite) TestK8sRBACResourcesValidate(c *gc.C) {
	for i, tc := range []tcK8sRBACResources{
		{
			Spec: k8sspecs.K8sRBACResources{
				ServiceAccounts: []k8sspecs.K8sServiceAccountSpec{
					{
						Name: "sa2",
						ServiceAccountSpecV3: specs.ServiceAccountSpecV3{
							AutomountServiceAccountToken: boolPtr(true),
							Roles: []specs.Role{
								{
									Name:   "cluster-role2",
									Global: true,
									Rules: []specs.PolicyRule{
										{
											APIGroups: []string{""},
											Resources: []string{"pods"},
											Verbs:     []string{"get", "watch", "list"},
										},
									},
								},
								{
									Name:   "cluster-role2",
									Global: true,
									Rules: []specs.PolicyRule{
										{
											NonResourceURLs: []string{"/healthz", "/healthz/*"},
											Verbs:           []string{"get", "post"},
										},
									},
								},
							},
						},
					},
				},
			},
			ErrStr: `duplicated role name "cluster-role2" not valid`,
		},
		{
			Spec: k8sspecs.K8sRBACResources{
				ServiceAccounts: []k8sspecs.K8sServiceAccountSpec{
					{
						Name: "sa2",
						ServiceAccountSpecV3: specs.ServiceAccountSpecV3{
							AutomountServiceAccountToken: boolPtr(true),
							Roles: []specs.Role{
								{
									Name:   "cluster-role2",
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
					},
					{
						Name: "sa2",
						ServiceAccountSpecV3: specs.ServiceAccountSpecV3{
							AutomountServiceAccountToken: boolPtr(true),
							Roles: []specs.Role{
								{
									Name:   "cluster-role3",
									Global: true,
									Rules: []specs.PolicyRule{
										{
											APIGroups: []string{""},
											Verbs:     []string{"get", "watch", "list"},
										},
									},
								},
							},
						},
					},
				},
			},
			ErrStr: `duplicated service account name "sa2" not valid`,
		},
		{
			Spec: k8sspecs.K8sRBACResources{
				ServiceAccounts: []k8sspecs.K8sServiceAccountSpec{
					{
						Name: "sa2",
						ServiceAccountSpecV3: specs.ServiceAccountSpecV3{
							AutomountServiceAccountToken: boolPtr(true),
							Roles: []specs.Role{
								{
									Name:   "cluster-role2",
									Global: true,
									Rules: []specs.PolicyRule{
										{
											APIGroups: []string{""},
											Resources: []string{"pods"},
											Verbs:     []string{"get", "watch", "list"},
										},
									},
								},
								{
									Name:   "",
									Global: true,
									Rules: []specs.PolicyRule{
										{
											NonResourceURLs: []string{"/healthz", "/healthz/*"},
											Verbs:           []string{"get", "post"},
										},
									},
								},
							},
						},
					},
				},
			},
			ErrStr: `either all or none of the roles of the service account "sa2" should have a name set`,
		},
	} {
		c.Logf("checking K8sRBACResources Validate %d", i)
		c.Check(tc.Spec.Validate(), gc.ErrorMatches, tc.ErrStr)
	}
}

func (s *v3SpecsSuite) TestK8sRBACResourcesToK8s(c *gc.C) {
	namespace := "test"
	appName := "app-name"
	annotations := map[string]string{
		"fred":               "mary",
		"juju.io/controller": testing.ControllerTag.Id(),
	}
	prefixNameSpace := func(name string) string {
		return fmt.Sprintf("%s-%s", namespace, name)
	}
	getBindingName := func(sa, cR k8sspecs.NameGetter) string {
		return fmt.Sprintf("%s-%s", sa.GetName(), cR.GetName())
	}
	getSAMeta := func(name string) metav1.ObjectMeta {
		return metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      map[string]string{"juju-app": appName},
			Annotations: annotations,
		}
	}
	getRoleClusterRoleName := func(roleName, serviceAccountName string, index int) string {
		if roleName != "" {
			return roleName
		}
		roleName = fmt.Sprintf("%s-%s", appName, serviceAccountName)
		if index == 0 {
			return roleName
		}
		return fmt.Sprintf("%s%d", roleName, index)
	}
	getRoleMeta := func(roleName, serviceAccountName string, index int) metav1.ObjectMeta {
		return metav1.ObjectMeta{
			Name:        getRoleClusterRoleName(roleName, serviceAccountName, index),
			Namespace:   namespace,
			Labels:      map[string]string{"juju-app": appName},
			Annotations: annotations,
		}
	}
	getClusterRoleMeta := func(roleName, serviceAccountName string, index int) metav1.ObjectMeta {
		roleName = getRoleClusterRoleName(roleName, serviceAccountName, index)
		return metav1.ObjectMeta{
			Name:        prefixNameSpace(roleName),
			Namespace:   namespace,
			Labels:      map[string]string{"juju-app": appName, "juju-model": namespace},
			Annotations: annotations,
		}
	}
	getBindingMeta := func(sa, role k8sspecs.NameGetter) metav1.ObjectMeta {
		return metav1.ObjectMeta{
			Name:        getBindingName(sa, role),
			Namespace:   namespace,
			Labels:      map[string]string{"juju-app": appName},
			Annotations: annotations,
		}
	}
	getClusterBindingMeta := func(sa, clusterRole k8sspecs.NameGetter) metav1.ObjectMeta {
		return metav1.ObjectMeta{
			Name:        getBindingName(sa, clusterRole),
			Namespace:   namespace,
			Labels:      map[string]string{"juju-app": appName, "juju-model": namespace},
			Annotations: annotations,
		}
	}

	rbacResource := k8sspecs.K8sRBACResources{
		ServiceAccounts: []k8sspecs.K8sServiceAccountSpec{
			{
				Name: "sa1",
				ServiceAccountSpecV3: specs.ServiceAccountSpecV3{
					AutomountServiceAccountToken: boolPtr(true),
					Roles: []specs.Role{
						{
							Name: "role1",
							Rules: []specs.PolicyRule{
								{
									APIGroups: []string{""},
									Resources: []string{"pods"},
									Verbs:     []string{"get", "watch", "list"},
								},
							},
						},
						{
							Name:   "cluster-role2",
							Global: true,
							Rules: []specs.PolicyRule{
								{
									NonResourceURLs: []string{"/healthz", "/healthz/*"},
									Verbs:           []string{"get", "post"},
								},
							},
						},
						{
							Name:   "cluster-role3",
							Global: true,
							Rules: []specs.PolicyRule{
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
			{
				Name: "sa-foo",
				ServiceAccountSpecV3: specs.ServiceAccountSpecV3{
					Roles: []specs.Role{
						{
							Rules: []specs.PolicyRule{
								{
									APIGroups: []string{""},
									Resources: []string{"pods"},
									Verbs:     []string{"get", "watch", "list"},
								},
							},
						},
						{
							Rules: []specs.PolicyRule{
								{
									APIGroups: []string{""},
									Resources: []string{"pods"},
									Verbs:     []string{"get", "watch"},
								},
							},
						},
						{
							Global: true,
							Rules: []specs.PolicyRule{
								{
									NonResourceURLs: []string{"/healthz", "/healthz/*"},
									Verbs:           []string{"get", "post"},
								},
							},
						},
						{
							Global: true,
							Rules: []specs.PolicyRule{
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
	c.Assert(rbacResource.Validate(), jc.ErrorIsNil)
	serviceAccounts, roles, clusterroles, roleBindings, clusterRoleBindings := rbacResource.ToK8s(
		getSAMeta,
		getRoleMeta,
		getClusterRoleMeta,
		getBindingMeta,
		getClusterBindingMeta,
	)
	c.Assert(serviceAccounts, gc.DeepEquals, []core.ServiceAccount{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "sa1",
				Namespace:   "test",
				Labels:      map[string]string{"juju-app": "app-name"},
				Annotations: annotations,
			},
			AutomountServiceAccountToken: boolPtr(true),
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "sa-foo",
				Namespace:   "test",
				Labels:      map[string]string{"juju-app": "app-name"},
				Annotations: annotations,
			},
		},
	})
	c.Assert(roles, gc.DeepEquals, []rbacv1.Role{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "role1",
				Namespace:   "test",
				Labels:      map[string]string{"juju-app": "app-name"},
				Annotations: annotations,
			},
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods"},
					Verbs:     []string{"get", "watch", "list"},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "app-name-sa-foo",
				Namespace:   "test",
				Labels:      map[string]string{"juju-app": "app-name"},
				Annotations: annotations,
			},
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods"},
					Verbs:     []string{"get", "watch", "list"},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "app-name-sa-foo1",
				Namespace:   "test",
				Labels:      map[string]string{"juju-app": "app-name"},
				Annotations: annotations,
			},
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods"},
					Verbs:     []string{"get", "watch"},
				},
			},
		},
	})
	c.Assert(clusterroles, gc.DeepEquals, []rbacv1.ClusterRole{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "test-cluster-role2",
				Namespace:   "test",
				Labels:      map[string]string{"juju-app": "app-name", "juju-model": "test"},
				Annotations: annotations,
			},
			Rules: []rbacv1.PolicyRule{
				{
					NonResourceURLs: []string{"/healthz", "/healthz/*"},
					Verbs:           []string{"get", "post"},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "test-cluster-role3",
				Namespace:   "test",
				Labels:      map[string]string{"juju-app": "app-name", "juju-model": "test"},
				Annotations: annotations,
			},
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups:     []string{"rbac.authorization.k8s.io"},
					Resources:     []string{"clusterroles"},
					Verbs:         []string{"bind"},
					ResourceNames: []string{"admin", "edit", "view"},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "test-app-name-sa-foo2",
				Namespace:   "test",
				Labels:      map[string]string{"juju-app": "app-name", "juju-model": "test"},
				Annotations: annotations,
			},
			Rules: []rbacv1.PolicyRule{
				{
					NonResourceURLs: []string{"/healthz", "/healthz/*"},
					Verbs:           []string{"get", "post"},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "test-app-name-sa-foo3",
				Namespace:   "test",
				Labels:      map[string]string{"juju-app": "app-name", "juju-model": "test"},
				Annotations: annotations,
			},
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups:     []string{"rbac.authorization.k8s.io"},
					Resources:     []string{"clusterroles"},
					Verbs:         []string{"bind"},
					ResourceNames: []string{"admin", "edit", "view"},
				},
			},
		},
	})
	c.Assert(roleBindings, gc.DeepEquals, []rbacv1.RoleBinding{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "sa1-role1",
				Namespace:   "test",
				Labels:      map[string]string{"juju-app": "app-name"},
				Annotations: annotations,
			},
			RoleRef: rbacv1.RoleRef{
				Name: "role1",
				Kind: "Role",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      rbacv1.ServiceAccountKind,
					Name:      "sa1",
					Namespace: "test",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "sa-foo-app-name-sa-foo",
				Namespace:   "test",
				Labels:      map[string]string{"juju-app": "app-name"},
				Annotations: annotations,
			},
			RoleRef: rbacv1.RoleRef{
				Name: "app-name-sa-foo",
				Kind: "Role",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      rbacv1.ServiceAccountKind,
					Name:      "sa-foo",
					Namespace: "test",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "sa-foo-app-name-sa-foo1",
				Namespace:   "test",
				Labels:      map[string]string{"juju-app": "app-name"},
				Annotations: annotations,
			},
			RoleRef: rbacv1.RoleRef{
				Name: "app-name-sa-foo1",
				Kind: "Role",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      rbacv1.ServiceAccountKind,
					Name:      "sa-foo",
					Namespace: "test",
				},
			},
		},
	})
	c.Assert(clusterRoleBindings, gc.DeepEquals, []rbacv1.ClusterRoleBinding{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "sa1-test-cluster-role2",
				Namespace:   "test",
				Labels:      map[string]string{"juju-app": "app-name", "juju-model": "test"},
				Annotations: annotations,
			},
			RoleRef: rbacv1.RoleRef{
				Name: "test-cluster-role2",
				Kind: "ClusterRole",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      rbacv1.ServiceAccountKind,
					Name:      "sa1",
					Namespace: "test",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "sa1-test-cluster-role3",
				Namespace:   "test",
				Labels:      map[string]string{"juju-app": "app-name", "juju-model": "test"},
				Annotations: annotations,
			},
			RoleRef: rbacv1.RoleRef{
				Name: "test-cluster-role3",
				Kind: "ClusterRole",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      rbacv1.ServiceAccountKind,
					Name:      "sa1",
					Namespace: "test",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "sa-foo-test-app-name-sa-foo2",
				Namespace:   "test",
				Labels:      map[string]string{"juju-app": "app-name", "juju-model": "test"},
				Annotations: annotations,
			},
			RoleRef: rbacv1.RoleRef{
				Name: "test-app-name-sa-foo2",
				Kind: "ClusterRole",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      rbacv1.ServiceAccountKind,
					Name:      "sa-foo",
					Namespace: "test",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "sa-foo-test-app-name-sa-foo3",
				Namespace:   "test",
				Labels:      map[string]string{"juju-app": "app-name", "juju-model": "test"},
				Annotations: annotations,
			},
			RoleRef: rbacv1.RoleRef{
				Name: "test-app-name-sa-foo3",
				Kind: "ClusterRole",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      rbacv1.ServiceAccountKind,
					Name:      "sa-foo",
					Namespace: "test",
				},
			},
		},
	})
}

func (s *v3SpecsSuite) TestUnknownFieldError(c *gc.C) {
	specStr := version3Header + `
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
