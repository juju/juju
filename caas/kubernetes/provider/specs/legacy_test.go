// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	// rbacv1 "k8s.io/api/rbac/v1"
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

	getExpectedPodSpecBase := func() *specs.PodSpec {
		pSpecs := &specs.PodSpec{}
		// always parse to latest version.
		pSpecs.Version = specs.CurrentVersion
		pSpecs.OmitServiceFrontend = true
		pSpecs.ProviderPod = &k8sspecs.K8sPodSpecLegacy{
			ActiveDeadlineSeconds:         int64Ptr(10),
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
			Service: &k8sspecs.K8sServiceSpec{
				Annotations: map[string]string{"foo": "bar"},
			},
		}
		pSpecs.Containers = []specs.ContainerSpec{{
			Name:  "gitlab",
			Image: "gitlab/latest",
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
				"attr":       "foo=bar; name['fred']='blogs';",
				"foo":        "bar",
				"restricted": "'yes'",
				"switch":     true,
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
		}}
		pSpecs.InitContainers = []specs.ContainerSpec{{
			Name:  "gitlab-init",
			Image: "gitlab-init/latest",
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
			},
			ProviderContainer: &k8sspecs.K8sContainerSpec{
				ImagePullPolicy: "Always",
			},
		}}

		pSpecs.ProviderPod = &k8sspecs.K8sPodSpec{
			ServiceAccount: &k8sspecs.ServiceAccountSpec{
				Name:                         "serviceAccount",
				AutomountServiceAccountToken: boolPtr(true),
			},
			ActiveDeadlineSeconds:         int64Ptr(10),
			RestartPolicy:                 core.RestartPolicyOnFailure,
			TerminationGracePeriodSeconds: int64Ptr(20),
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
			Service: &k8sspecs.K8sServiceSpec{
				Annotations: map[string]string{"foo": "bar"},
			},
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
		}
		return pSpecs
	}

	spec, err := k8sspecs.ParsePodSpec(specStrBase)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spec, jc.DeepEquals, getExpectedPodSpecBase())

	// 	expectedPodSpecWithServiceAccount := getExpectedPodSpecBase()
	// 	expectedPodSpecWithServiceAccountName := getExpectedPodSpecBase()

	// 	expectedPodSpecWithServiceAccountName.ProviderPod = &k8sspecs.K8sPodSpec{
	// 		ActiveDeadlineSeconds:         int64Ptr(10),
	// 		ServiceAccountName:            "serviceAccount",
	// 		RestartPolicy:                 core.RestartPolicyOnFailure,
	// 		TerminationGracePeriodSeconds: int64Ptr(20),
	// 		AutomountServiceAccountToken:  boolPtr(true),
	// 		SecurityContext: &core.PodSecurityContext{
	// 			RunAsNonRoot:       boolPtr(true),
	// 			SupplementalGroups: []int64{1, 2},
	// 		},
	// 		Hostname:          "host",
	// 		Subdomain:         "sub",
	// 		PriorityClassName: "top",
	// 		Priority:          int32Ptr(30),
	// 		DNSConfig: &core.PodDNSConfig{
	// 			Nameservers: []string{"ns1", "ns2"},
	// 		},
	// 		DNSPolicy: "ClusterFirstWithHostNet",
	// 		ReadinessGates: []core.PodReadinessGate{
	// 			{ConditionType: core.PodScheduled},
	// 		},
	// 		Service: &k8sspecs.K8sServiceSpec{
	// 			Annotations: map[string]string{"foo": "bar"},
	// 		},
	// 	}

	// 	expectedPodSpecWithServiceAccount.ServiceAccount = &k8sspecs.ServiceAccountSpec{
	// 		Name:                         "build-robot",
	// 		AutomountServiceAccountToken: boolPtr(true),
	// 		Capabilities: &k8sspecs.Capabilities{
	// 			RoleBinding: &k8sspecs.RoleBindingSpec{
	// 				Name: "read-pods",
	// 				Type: k8sspecs.ClusterRoleBinding,
	// 			},
	// 			Role: &k8sspecs.RoleSpec{
	// 				Name: "pod-reader",
	// 				Type: k8sspecs.ClusterRole,
	// 				Rules: []rbacv1.PolicyRule{
	// 					{
	// 						APIGroups: []string{""},
	// 						Resources: []string{"pods"},
	// 						Verbs:     []string{"get", "watch", "list"},
	// 					},
	// 				},
	// 			},
	// 		},
	// 	}
	// 	for i, tc := range []struct {
	// 		title, podSpecStr string
	// 		podSpec           *specs.PodSpec
	// 	}{
	// 		{
	// 			title: "reference to existing service account by using serviceAccountName",
	// 			podSpecStr: specStrBase + `
	// serviceAccountName: serviceAccount
	// `[1:],
	// 			podSpec: expectedPodSpecWithServiceAccountName,
	// 		},
	// 		{
	// 			title: "create new service account, role/clusterrole, and bindings by providing serviceaccount spec",
	// 			podSpecStr: specStrBase + `
	// serviceAccount:
	//   name: build-robot
	//   automountServiceAccountToken: true
	//   capabilities:
	//     roleBinding:
	//       name: read-pods
	//       type: ClusterRoleBinding
	//     role:
	//       name: pod-reader
	//       type: ClusterRole
	//       rules:
	//       - apiGroups: [""]
	//         resources: ["pods"]
	//         verbs: ["get", "watch", "list"]
	// `[1:],
	// 			podSpec: expectedPodSpecWithServiceAccount,
	// 		},
	// 	} {
	// 		c.Logf("%v: %s", i, tc.title)
	// 		spec, err := k8sspecs.ParsePodSpec(tc.podSpecStr)
	// 		c.Assert(err, jc.ErrorIsNil)
	// 		c.Assert(spec, jc.DeepEquals, tc.podSpec)
	// 	}
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

func (s *legacySpecsSuite) TestParsePodSpecFailedBothServiceAccountAndServiceAccountNameProvided(c *gc.C) {
	specStr := `
serviceAccountName: an-existing-svc-account
serviceAccount:
  name: build-robot
  automountServiceAccountToken: true
  capabilities:
    roleBinding:
      name: read-pods
      type: ClusterRoleBinding
    role:
      name: pod-reader
      type: ClusterRole
      rules:
      - apiGroups: [""]
        resources: ["pods"]
        verbs: ["get", "watch", "list"]
`[1:]
	_, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, gc.ErrorMatches, "either use ServiceAccountName to reference existing service account or define ServiceAccount spec to create a new one")

}

type serviceAccountTestCase struct {
	Title, Spec, Err string
}

var serviceAccountValidationTestCases = []serviceAccountTestCase{
	{
		Title: "wrong role binding type",
		Spec: `
serviceAccount:
  name: build-robot
  automountServiceAccountToken: true
  capabilities:
    roleBinding:
      name: read-pods
      type: ClusterRoleBinding11
    role:
      name: pod-reader
      type: ClusterRole
      rules:
      - apiGroups: [""]
        resources: ["pods"]
        verbs: ["get", "watch", "list"]
`[1:],
		Err: "\"ClusterRoleBinding11\" not supported",
	},
	{
		Title: "wrong role type",
		Spec: `
serviceAccount:
  name: build-robot
  automountServiceAccountToken: true
  capabilities:
    roleBinding:
      name: read-pods
      type: ClusterRoleBinding
    role:
      name: pod-reader
      type: ClusterRole11
      rules:
      - apiGroups: [""]
        resources: ["pods"]
        verbs: ["get", "watch", "list"]
`[1:],
		Err: "\"ClusterRole11\" not supported",
	},
	{
		Title: "missing role",
		Spec: `
serviceAccount:
  name: build-robot
  automountServiceAccountToken: true
  capabilities:
    roleBinding:
      name: read-pods
      type: ClusterRoleBinding
`[1:],
		Err: `role is required for capabilities`,
	},
	{
		Title: "missing role binding",
		Spec: `
serviceAccount:
  name: build-robot
  automountServiceAccountToken: true
  capabilities:
    role:
      name: pod-reader
      type: ClusterRole11
      rules:
      - apiGroups: [""]
        resources: ["pods"]
        verbs: ["get", "watch", "list"]
`[1:],
		Err: `roleBinding is required for capabilities`,
	},
}

func (s *legacySpecsSuite) TestValidateServiceAccountFailed(c *gc.C) {
	containerSpec := `
containers:
  - name: gitlab-helper
    image: gitlab-helper/latest
    ports:
    - containerPort: 8080
      protocol: TCP
`[1:]

	for i, tc := range serviceAccountValidationTestCases {
		c.Logf("%v: %s", i, tc.Title)
		_, err := k8sspecs.ParsePodSpec(containerSpec + tc.Spec)
		c.Check(err, gc.ErrorMatches, tc.Err)
	}
}
