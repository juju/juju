// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	stdcontext "context"
	"fmt"
	"strings"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiextensionsclientsetfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/dynamic"
	k8sdynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8srestfake "k8s.io/client-go/rest/fake"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/pointer"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	k8sutils "github.com/juju/juju/caas/kubernetes/provider/utils"
	k8swatcher "github.com/juju/juju/caas/kubernetes/provider/watcher"
	k8swatchertest "github.com/juju/juju/caas/kubernetes/provider/watcher/test"
	"github.com/juju/juju/caas/specs"
	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/network"
	coreresources "github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type K8sSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&K8sSuite{})

func (s *K8sSuite) TestPrepareWorkloadSpecNoConfigConfig(c *gc.C) {
	podSpec := specs.PodSpec{
		ServiceAccount: primeServiceAccount,
	}

	podSpec.ProviderPod = &k8sspecs.K8sPodSpec{
		KubernetesResources: &k8sspecs.KubernetesResources{
			Pod: &k8sspecs.PodSpec{
				RestartPolicy:                 core.RestartPolicyOnFailure,
				ActiveDeadlineSeconds:         pointer.Int64Ptr(10),
				TerminationGracePeriodSeconds: pointer.Int64Ptr(20),
				SecurityContext: &core.PodSecurityContext{
					RunAsNonRoot:       pointer.BoolPtr(true),
					SupplementalGroups: []int64{1, 2},
				},
				ReadinessGates: []core.PodReadinessGate{
					{ConditionType: core.PodInitialized},
				},
				DNSPolicy:         core.DNSClusterFirst,
				HostNetwork:       true,
				HostPID:           true,
				PriorityClassName: "system-cluster-critical",
				Priority:          pointer.Int32Ptr(2000000000),
			},
		},
	}
	podSpec.Containers = []specs.ContainerSpec{
		{
			Name:            "test",
			Ports:           []specs.ContainerPort{{ContainerPort: 80, Protocol: "TCP"}},
			Image:           "juju-repo.local/juju/image",
			ImagePullPolicy: specs.PullPolicy("Always"),
			ProviderContainer: &k8sspecs.K8sContainerSpec{
				ReadinessProbe: &core.Probe{
					InitialDelaySeconds: 10,
					ProbeHandler:        core.ProbeHandler{HTTPGet: &core.HTTPGetAction{Path: "/ready"}},
				},
				LivenessProbe: &core.Probe{
					SuccessThreshold: 20,
					ProbeHandler:     core.ProbeHandler{HTTPGet: &core.HTTPGetAction{Path: "/liveready"}},
				},
				SecurityContext: &core.SecurityContext{
					RunAsNonRoot: pointer.BoolPtr(true),
					Privileged:   pointer.BoolPtr(true),
				},
			},
		}, {
			Name:  "test2",
			Ports: []specs.ContainerPort{{ContainerPort: 8080, Protocol: "TCP"}},
			Image: "juju-repo.local/juju/image2",
		},
	}

	spec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", &podSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.Pod(spec), jc.DeepEquals, k8sspecs.PodSpecWithAnnotations{
		Labels:      map[string]string{},
		Annotations: annotations.Annotation{},
		PodSpec: core.PodSpec{
			RestartPolicy:                 core.RestartPolicyOnFailure,
			ActiveDeadlineSeconds:         pointer.Int64Ptr(10),
			TerminationGracePeriodSeconds: pointer.Int64Ptr(20),
			SecurityContext: &core.PodSecurityContext{
				RunAsNonRoot:       pointer.BoolPtr(true),
				SupplementalGroups: []int64{1, 2},
			},
			ReadinessGates: []core.PodReadinessGate{
				{ConditionType: core.PodInitialized},
			},
			DNSPolicy:                    core.DNSClusterFirst,
			HostNetwork:                  true,
			HostPID:                      true,
			PriorityClassName:            "system-cluster-critical",
			Priority:                     pointer.Int32Ptr(2000000000),
			ServiceAccountName:           "app-name",
			AutomountServiceAccountToken: pointer.BoolPtr(true),
			InitContainers:               initContainers(),
			Containers: []core.Container{
				{
					Name:            "test",
					Image:           "juju-repo.local/juju/image",
					Ports:           []core.ContainerPort{{ContainerPort: int32(80), Protocol: core.ProtocolTCP}},
					ImagePullPolicy: core.PullAlways,
					SecurityContext: &core.SecurityContext{
						RunAsNonRoot: pointer.BoolPtr(true),
						Privileged:   pointer.BoolPtr(true),
					},
					ReadinessProbe: &core.Probe{
						InitialDelaySeconds: 10,
						ProbeHandler:        core.ProbeHandler{HTTPGet: &core.HTTPGetAction{Path: "/ready"}},
					},
					LivenessProbe: &core.Probe{
						SuccessThreshold: 20,
						ProbeHandler:     core.ProbeHandler{HTTPGet: &core.HTTPGetAction{Path: "/liveready"}},
					},
					VolumeMounts: dataVolumeMounts(),
				}, {
					Name:  "test2",
					Image: "juju-repo.local/juju/image2",
					Ports: []core.ContainerPort{{ContainerPort: int32(8080), Protocol: core.ProtocolTCP}},
					// Defaults since not specified.
					SecurityContext: &core.SecurityContext{
						RunAsNonRoot:             pointer.BoolPtr(false),
						ReadOnlyRootFilesystem:   pointer.BoolPtr(false),
						AllowPrivilegeEscalation: pointer.BoolPtr(true),
					},
					VolumeMounts: dataVolumeMounts(),
				},
			},
			Volumes: dataVolumes(),
		},
	})
}

func (s *K8sSuite) TestPrepareWorkloadSpecWithEnvAndEnvFrom(c *gc.C) {

	podSpec := specs.PodSpec{
		ServiceAccount: primeServiceAccount,
	}

	podSpec.ProviderPod = &k8sspecs.K8sPodSpec{
		KubernetesResources: &k8sspecs.KubernetesResources{
			Pod: &k8sspecs.PodSpec{
				RestartPolicy:                 core.RestartPolicyOnFailure,
				ActiveDeadlineSeconds:         pointer.Int64Ptr(10),
				TerminationGracePeriodSeconds: pointer.Int64Ptr(20),
				SecurityContext: &core.PodSecurityContext{
					RunAsNonRoot:       pointer.BoolPtr(true),
					SupplementalGroups: []int64{1, 2},
				},
				ReadinessGates: []core.PodReadinessGate{
					{ConditionType: core.PodInitialized},
				},
				DNSPolicy: core.DNSClusterFirst,
			},
		},
	}

	envVarThing := core.EnvVar{
		Name: "thing",
		ValueFrom: &core.EnvVarSource{
			SecretKeyRef: &core.SecretKeySelector{Key: "bar"},
		},
	}
	envVarThing.ValueFrom.SecretKeyRef.Name = "foo"

	envVarThing1 := core.EnvVar{
		Name: "thing1",
		ValueFrom: &core.EnvVarSource{
			ConfigMapKeyRef: &core.ConfigMapKeySelector{Key: "bar"},
		},
	}
	envVarThing1.ValueFrom.ConfigMapKeyRef.Name = "foo"

	envFromSourceSecret1 := core.EnvFromSource{
		SecretRef: &core.SecretEnvSource{Optional: pointer.BoolPtr(true)},
	}
	envFromSourceSecret1.SecretRef.Name = "secret1"

	envFromSourceSecret2 := core.EnvFromSource{
		SecretRef: &core.SecretEnvSource{},
	}
	envFromSourceSecret2.SecretRef.Name = "secret2"

	envFromSourceConfigmap1 := core.EnvFromSource{
		ConfigMapRef: &core.ConfigMapEnvSource{Optional: pointer.BoolPtr(true)},
	}
	envFromSourceConfigmap1.ConfigMapRef.Name = "configmap1"

	envFromSourceConfigmap2 := core.EnvFromSource{
		ConfigMapRef: &core.ConfigMapEnvSource{},
	}
	envFromSourceConfigmap2.ConfigMapRef.Name = "configmap2"

	podSpec.Containers = []specs.ContainerSpec{
		{
			Name:            "test",
			Ports:           []specs.ContainerPort{{ContainerPort: 80, Protocol: "TCP"}},
			Image:           "juju-repo.local/juju/image",
			ImagePullPolicy: specs.PullPolicy("Always"),
			EnvConfig: map[string]interface{}{
				"restricted": "yes",
				"secret1": map[string]interface{}{
					"secret": map[string]interface{}{
						"optional": bool(true),
						"name":     "secret1",
					},
				},
				"secret2": map[string]interface{}{
					"secret": map[string]interface{}{
						"name": "secret2",
					},
				},
				"special": "p@ssword's",
				"switch":  bool(true),
				"MY_NODE_NAME": map[string]interface{}{
					"field": map[string]interface{}{
						"path": "spec.nodeName",
					},
				},
				"my-resource-limit": map[string]interface{}{
					"resource": map[string]interface{}{
						"container-name": "container1",
						"resource":       "requests.cpu",
						"divisor":        "1m",
					},
				},
				"attr": "foo=bar; name[\"fred\"]=\"blogs\";",
				"configmap1": map[string]interface{}{
					"config-map": map[string]interface{}{
						"name":     "configmap1",
						"optional": bool(true),
					},
				},
				"configmap2": map[string]interface{}{
					"config-map": map[string]interface{}{
						"name": "configmap2",
					},
				},
				"float": float64(111.11111111),
				"thing1": map[string]interface{}{
					"config-map": map[string]interface{}{
						"key":  "bar",
						"name": "foo",
					},
				},
				"brackets": "[\"hello\", \"world\"]",
				"foo":      "bar",
				"int":      float64(111),
				"thing": map[string]interface{}{
					"secret": map[string]interface{}{
						"key":  "bar",
						"name": "foo",
					},
				},
			},
			ProviderContainer: &k8sspecs.K8sContainerSpec{
				ReadinessProbe: &core.Probe{
					InitialDelaySeconds: 10,
					ProbeHandler:        core.ProbeHandler{HTTPGet: &core.HTTPGetAction{Path: "/ready"}},
				},
				LivenessProbe: &core.Probe{
					SuccessThreshold: 20,
					ProbeHandler:     core.ProbeHandler{HTTPGet: &core.HTTPGetAction{Path: "/liveready"}},
				},
				SecurityContext: &core.SecurityContext{
					RunAsNonRoot: pointer.BoolPtr(true),
					Privileged:   pointer.BoolPtr(true),
				},
			},
		}, {
			Name:  "test2",
			Ports: []specs.ContainerPort{{ContainerPort: 8080, Protocol: "TCP"}},
			Image: "juju-repo.local/juju/image2",
		},
	}

	spec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", &podSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.Pod(spec), jc.DeepEquals, k8sspecs.PodSpecWithAnnotations{
		Labels:      map[string]string{},
		Annotations: annotations.Annotation{},
		PodSpec: core.PodSpec{
			RestartPolicy:                 core.RestartPolicyOnFailure,
			ActiveDeadlineSeconds:         pointer.Int64Ptr(10),
			TerminationGracePeriodSeconds: pointer.Int64Ptr(20),
			SecurityContext: &core.PodSecurityContext{
				RunAsNonRoot:       pointer.BoolPtr(true),
				SupplementalGroups: []int64{1, 2},
			},
			ReadinessGates: []core.PodReadinessGate{
				{ConditionType: core.PodInitialized},
			},
			DNSPolicy:                    core.DNSClusterFirst,
			ServiceAccountName:           "app-name",
			AutomountServiceAccountToken: pointer.BoolPtr(true),
			InitContainers:               initContainers(),
			Containers: []core.Container{
				{
					Name:            "test",
					Image:           "juju-repo.local/juju/image",
					Ports:           []core.ContainerPort{{ContainerPort: int32(80), Protocol: core.ProtocolTCP}},
					ImagePullPolicy: core.PullAlways,
					SecurityContext: &core.SecurityContext{
						RunAsNonRoot: pointer.BoolPtr(true),
						Privileged:   pointer.BoolPtr(true),
					},
					ReadinessProbe: &core.Probe{
						InitialDelaySeconds: 10,
						ProbeHandler:        core.ProbeHandler{HTTPGet: &core.HTTPGetAction{Path: "/ready"}},
					},
					LivenessProbe: &core.Probe{
						SuccessThreshold: 20,
						ProbeHandler:     core.ProbeHandler{HTTPGet: &core.HTTPGetAction{Path: "/liveready"}},
					},
					VolumeMounts: dataVolumeMounts(),
					Env: []core.EnvVar{
						{Name: "MY_NODE_NAME", ValueFrom: &core.EnvVarSource{FieldRef: &core.ObjectFieldSelector{FieldPath: "spec.nodeName"}}},
						{Name: "attr", Value: `foo=bar; name["fred"]="blogs";`},
						{Name: "brackets", Value: `["hello", "world"]`},
						{Name: "float", Value: "111.11111111"},
						{Name: "foo", Value: "bar"},
						{Name: "int", Value: "111"},
						{
							Name: "my-resource-limit",
							ValueFrom: &core.EnvVarSource{
								ResourceFieldRef: &core.ResourceFieldSelector{
									ContainerName: "container1",
									Resource:      "requests.cpu",
									Divisor:       resource.MustParse("1m"),
								},
							},
						},
						{Name: "restricted", Value: "yes"},
						{Name: "special", Value: "p@ssword's"},
						{Name: "switch", Value: "true"},
						envVarThing,
						envVarThing1,
					},
					EnvFrom: []core.EnvFromSource{
						envFromSourceConfigmap1,
						envFromSourceConfigmap2,
						envFromSourceSecret1,
						envFromSourceSecret2,
					},
				}, {
					Name:  "test2",
					Image: "juju-repo.local/juju/image2",
					Ports: []core.ContainerPort{{ContainerPort: int32(8080), Protocol: core.ProtocolTCP}},
					// Defaults since not specified.
					SecurityContext: &core.SecurityContext{
						RunAsNonRoot:             pointer.BoolPtr(false),
						ReadOnlyRootFilesystem:   pointer.BoolPtr(false),
						AllowPrivilegeEscalation: pointer.BoolPtr(true),
					},
					VolumeMounts: dataVolumeMounts(),
				},
			},
			Volumes: dataVolumes(),
		},
	})
}

func (s *K8sSuite) TestPrepareWorkloadSpecWithInitContainers(c *gc.C) {
	podSpec := specs.PodSpec{}
	podSpec.Containers = []specs.ContainerSpec{
		{
			Name:            "test",
			Ports:           []specs.ContainerPort{{ContainerPort: 80, Protocol: "TCP"}},
			Image:           "juju-repo.local/juju/image",
			ImagePullPolicy: specs.PullPolicy("Always"),
			ProviderContainer: &k8sspecs.K8sContainerSpec{
				ReadinessProbe: &core.Probe{
					InitialDelaySeconds: 10,
					ProbeHandler:        core.ProbeHandler{HTTPGet: &core.HTTPGetAction{Path: "/ready"}},
				},
				LivenessProbe: &core.Probe{
					SuccessThreshold: 20,
					ProbeHandler:     core.ProbeHandler{HTTPGet: &core.HTTPGetAction{Path: "/liveready"}},
				},
				SecurityContext: &core.SecurityContext{
					RunAsNonRoot: pointer.BoolPtr(true),
					Privileged:   pointer.BoolPtr(true),
				},
			},
		}, {
			Name:  "test2",
			Ports: []specs.ContainerPort{{ContainerPort: 8080, Protocol: "TCP"}},
			Image: "juju-repo.local/juju/image2",
		},
		{
			Name:            "test-init",
			Init:            true,
			Ports:           []specs.ContainerPort{{ContainerPort: 90, Protocol: "TCP"}},
			Image:           "juju-repo.local/juju/image-init",
			ImagePullPolicy: specs.PullPolicy("Always"),
			WorkingDir:      "/path/to/here",
			Command:         []string{"sh", "ls"},
		},
	}

	spec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", &podSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.Pod(spec), jc.DeepEquals, k8sspecs.PodSpecWithAnnotations{
		PodSpec: core.PodSpec{
			Containers: []core.Container{
				{
					Name:            "test",
					Image:           "juju-repo.local/juju/image",
					Ports:           []core.ContainerPort{{ContainerPort: int32(80), Protocol: core.ProtocolTCP}},
					ImagePullPolicy: core.PullAlways,
					ReadinessProbe: &core.Probe{
						InitialDelaySeconds: 10,
						ProbeHandler:        core.ProbeHandler{HTTPGet: &core.HTTPGetAction{Path: "/ready"}},
					},
					LivenessProbe: &core.Probe{
						SuccessThreshold: 20,
						ProbeHandler:     core.ProbeHandler{HTTPGet: &core.HTTPGetAction{Path: "/liveready"}},
					},
					SecurityContext: &core.SecurityContext{
						RunAsNonRoot: pointer.BoolPtr(true),
						Privileged:   pointer.BoolPtr(true),
					},
					VolumeMounts: dataVolumeMounts(),
				}, {
					Name:  "test2",
					Image: "juju-repo.local/juju/image2",
					Ports: []core.ContainerPort{{ContainerPort: int32(8080), Protocol: core.ProtocolTCP}},
					// Defaults since not specified.
					SecurityContext: &core.SecurityContext{
						RunAsNonRoot:             pointer.BoolPtr(false),
						ReadOnlyRootFilesystem:   pointer.BoolPtr(false),
						AllowPrivilegeEscalation: pointer.BoolPtr(true),
					},
					VolumeMounts: dataVolumeMounts(),
				},
			},
			InitContainers: append([]core.Container{
				{
					Name:            "test-init",
					Image:           "juju-repo.local/juju/image-init",
					Ports:           []core.ContainerPort{{ContainerPort: int32(90), Protocol: core.ProtocolTCP}},
					WorkingDir:      "/path/to/here",
					Command:         []string{"sh", "ls"},
					ImagePullPolicy: core.PullAlways,
					// Defaults since not specified.
					SecurityContext: &core.SecurityContext{
						RunAsNonRoot:             pointer.BoolPtr(false),
						ReadOnlyRootFilesystem:   pointer.BoolPtr(false),
						AllowPrivilegeEscalation: pointer.BoolPtr(true),
					},
				},
			}, initContainers()...),
			Volumes: dataVolumes(),
		},
	})
}

func (s *K8sSuite) TestPrepareWorkloadSpec(c *gc.C) {

	podSpec := specs.PodSpec{
		ServiceAccount: primeServiceAccount,
	}

	podSpec.ProviderPod = &k8sspecs.K8sPodSpec{
		KubernetesResources: &k8sspecs.KubernetesResources{
			Pod: &k8sspecs.PodSpec{
				Labels:                        map[string]string{"foo": "bax"},
				Annotations:                   map[string]string{"foo": "baz"},
				RestartPolicy:                 core.RestartPolicyOnFailure,
				ActiveDeadlineSeconds:         pointer.Int64Ptr(10),
				TerminationGracePeriodSeconds: pointer.Int64Ptr(20),
				SecurityContext: &core.PodSecurityContext{
					RunAsNonRoot:       pointer.BoolPtr(true),
					SupplementalGroups: []int64{1, 2},
				},
				ReadinessGates: []core.PodReadinessGate{
					{ConditionType: core.PodInitialized},
				},
				DNSPolicy:   core.DNSClusterFirst,
				HostNetwork: true,
				HostPID:     true,
			},
		},
	}
	podSpec.Containers = []specs.ContainerSpec{
		{
			Name:            "test",
			Ports:           []specs.ContainerPort{{ContainerPort: 80, Protocol: "TCP"}},
			Image:           "juju-repo.local/juju/image",
			ImagePullPolicy: "Always",
		},
	}

	spec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", &podSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.Pod(spec), jc.DeepEquals, k8sspecs.PodSpecWithAnnotations{
		Labels:      map[string]string{"foo": "bax"},
		Annotations: map[string]string{"foo": "baz"},
		PodSpec: core.PodSpec{
			RestartPolicy:                 core.RestartPolicyOnFailure,
			ActiveDeadlineSeconds:         pointer.Int64Ptr(10),
			TerminationGracePeriodSeconds: pointer.Int64Ptr(20),
			ReadinessGates: []core.PodReadinessGate{
				{ConditionType: core.PodInitialized},
			},
			DNSPolicy:                    core.DNSClusterFirst,
			ServiceAccountName:           "app-name",
			AutomountServiceAccountToken: pointer.BoolPtr(true),
			HostNetwork:                  true,
			HostPID:                      true,
			InitContainers:               initContainers(),
			SecurityContext: &core.PodSecurityContext{
				RunAsNonRoot:       pointer.BoolPtr(true),
				SupplementalGroups: []int64{1, 2},
			},
			Containers: []core.Container{
				{
					Name:            "test",
					Image:           "juju-repo.local/juju/image",
					Ports:           []core.ContainerPort{{ContainerPort: int32(80), Protocol: core.ProtocolTCP}},
					ImagePullPolicy: core.PullAlways,
					SecurityContext: &core.SecurityContext{
						RunAsNonRoot:             pointer.BoolPtr(false),
						ReadOnlyRootFilesystem:   pointer.BoolPtr(false),
						AllowPrivilegeEscalation: pointer.BoolPtr(true),
					},
					VolumeMounts: dataVolumeMounts(),
				},
			},
			Volumes: dataVolumes(),
		},
	})
}

func (s *K8sSuite) TestPrepareWorkloadSpecPrimarySA(c *gc.C) {
	podSpec := specs.PodSpec{ServiceAccount: primeServiceAccount}
	podSpec.Containers = []specs.ContainerSpec{
		{
			Name:            "test",
			Ports:           []specs.ContainerPort{{ContainerPort: 80, Protocol: "TCP"}},
			Image:           "juju-repo.local/juju/image",
			ImagePullPolicy: "Always",
		},
	}

	spec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", &podSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.Pod(spec), jc.DeepEquals, k8sspecs.PodSpecWithAnnotations{
		PodSpec: core.PodSpec{
			ServiceAccountName:           "app-name",
			AutomountServiceAccountToken: pointer.BoolPtr(true),
			InitContainers:               initContainers(),
			Containers: []core.Container{
				{
					Name:            "test",
					Image:           "juju-repo.local/juju/image",
					Ports:           []core.ContainerPort{{ContainerPort: int32(80), Protocol: core.ProtocolTCP}},
					ImagePullPolicy: core.PullAlways,
					SecurityContext: &core.SecurityContext{
						RunAsNonRoot:             pointer.BoolPtr(false),
						ReadOnlyRootFilesystem:   pointer.BoolPtr(false),
						AllowPrivilegeEscalation: pointer.BoolPtr(true),
					},
					VolumeMounts: dataVolumeMounts(),
				},
			},
			Volumes: dataVolumes(),
		},
	})
}

func getBasicPodspec() *specs.PodSpec {
	pSpecs := &specs.PodSpec{}
	pSpecs.Containers = []specs.ContainerSpec{{
		Name:         "test",
		Ports:        []specs.ContainerPort{{ContainerPort: 80, Protocol: "TCP"}},
		ImageDetails: specs.ImageDetails{ImagePath: "juju-repo.local/juju/image", Username: "fred", Password: "secret"},
		Command:      []string{"sh", "-c"},
		Args:         []string{"doIt", "--debug"},
		WorkingDir:   "/path/to/here",
		EnvConfig: map[string]interface{}{
			"foo":        "bar",
			"restricted": "yes",
			"bar":        true,
			"switch":     true,
			"brackets":   `["hello", "world"]`,
		},
	}, {
		Name:  "test2",
		Ports: []specs.ContainerPort{{ContainerPort: 8080, Protocol: "TCP", Name: "fred"}},
		Image: "juju-repo.local/juju/image2",
	}}
	return pSpecs
}

var basicServiceArg = &core.Service{
	ObjectMeta: v1.ObjectMeta{
		Name:        "app-name",
		Labels:      map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
		Annotations: map[string]string{"controller.juju.is/id": testing.ControllerTag.Id()}},
	Spec: core.ServiceSpec{
		Selector: map[string]string{"app.kubernetes.io/name": "app-name"},
		Type:     "LoadBalancer",
		Ports: []core.ServicePort{
			{Port: 80, TargetPort: intstr.FromInt(80), Protocol: "TCP"},
			{Port: 8080, Protocol: "TCP", Name: "fred"},
		},
		LoadBalancerIP: "10.0.0.1",
		ExternalName:   "ext-name",
	},
}

var basicHeadlessServiceArg = &core.Service{
	ObjectMeta: v1.ObjectMeta{
		Name:   "app-name-endpoints",
		Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
		Annotations: map[string]string{
			"controller.juju.is/id":                                  testing.ControllerTag.Id(),
			"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
		},
	},
	Spec: core.ServiceSpec{
		Selector:                 map[string]string{"app.kubernetes.io/name": "app-name"},
		Type:                     "ClusterIP",
		ClusterIP:                "None",
		PublishNotReadyAddresses: true,
	},
}

var primeServiceAccount = &specs.PrimeServiceAccountSpecV3{
	ServiceAccountSpecV3: specs.ServiceAccountSpecV3{
		AutomountServiceAccountToken: pointer.BoolPtr(true),
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
		},
	},
}

func (s *K8sBrokerSuite) getOCIImageSecret(c *gc.C, annotations map[string]string) *core.Secret {
	details := getBasicPodspec().Containers[0].ImageDetails
	secretData, err := k8sutils.CreateDockerConfigJSON(details.Username, details.Password, details.ImagePath)
	c.Assert(err, jc.ErrorIsNil)
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations["controller.juju.is/id"] = testing.ControllerTag.Id()

	return &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:        "app-name-test-secret",
			Namespace:   "test",
			Labels:      map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: annotations,
		},
		Type: "kubernetes.io/dockerconfigjson",
		Data: map[string][]byte{".dockerconfigjson": secretData},
	}
}

func (s *K8sSuite) TestPrepareWorkloadSpecConfigPairs(c *gc.C) {
	spec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", getBasicPodspec(), coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.Pod(spec), jc.DeepEquals, k8sspecs.PodSpecWithAnnotations{
		PodSpec: core.PodSpec{
			ImagePullSecrets: []core.LocalObjectReference{{Name: "app-name-test-secret"}},
			InitContainers:   initContainers(),
			Containers: []core.Container{
				{
					Name:       "test",
					Image:      "juju-repo.local/juju/image",
					Ports:      []core.ContainerPort{{ContainerPort: int32(80), Protocol: core.ProtocolTCP}},
					Command:    []string{"sh", "-c"},
					Args:       []string{"doIt", "--debug"},
					WorkingDir: "/path/to/here",
					Env: []core.EnvVar{
						{Name: "bar", Value: "true"},
						{Name: "brackets", Value: `["hello", "world"]`},
						{Name: "foo", Value: "bar"},
						{Name: "restricted", Value: "yes"},
						{Name: "switch", Value: "true"},
					},
					// Defaults since not specified.
					SecurityContext: &core.SecurityContext{
						RunAsNonRoot:             pointer.BoolPtr(false),
						ReadOnlyRootFilesystem:   pointer.BoolPtr(false),
						AllowPrivilegeEscalation: pointer.BoolPtr(true),
					},
					VolumeMounts: dataVolumeMounts(),
				}, {
					Name:  "test2",
					Image: "juju-repo.local/juju/image2",
					Ports: []core.ContainerPort{{ContainerPort: int32(8080), Protocol: core.ProtocolTCP, Name: "fred"}},
					// Defaults since not specified.
					SecurityContext: &core.SecurityContext{
						RunAsNonRoot:             pointer.BoolPtr(false),
						ReadOnlyRootFilesystem:   pointer.BoolPtr(false),
						AllowPrivilegeEscalation: pointer.BoolPtr(true),
					},
					VolumeMounts: dataVolumeMounts(),
				},
			},
			Volumes: dataVolumes(),
		},
	})
}

func (s *K8sSuite) TestPrepareWorkloadSpecWithRegistryCredentials(c *gc.C) {
	spec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", getBasicPodspec(),
		coreresources.DockerImageDetails{
			RegistryPath: "example.com/operator/image-path",
			ImageRepoDetails: docker.ImageRepoDetails{
				Repository:      "example.com",
				BasicAuthConfig: docker.BasicAuthConfig{Username: "foo", Password: "bar"},
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	initContainerSpec := initContainers()[0]
	initContainerSpec.Image = "example.com/operator/image-path"
	c.Assert(provider.Pod(spec), jc.DeepEquals, k8sspecs.PodSpecWithAnnotations{
		PodSpec: core.PodSpec{
			ImagePullSecrets: []core.LocalObjectReference{
				{Name: "app-name-test-secret"},
				{Name: "juju-image-pull-secret"},
			},
			InitContainers: []core.Container{initContainerSpec},
			Containers: []core.Container{
				{
					Name:       "test",
					Image:      "juju-repo.local/juju/image",
					Ports:      []core.ContainerPort{{ContainerPort: int32(80), Protocol: core.ProtocolTCP}},
					Command:    []string{"sh", "-c"},
					Args:       []string{"doIt", "--debug"},
					WorkingDir: "/path/to/here",
					Env: []core.EnvVar{
						{Name: "bar", Value: "true"},
						{Name: "brackets", Value: `["hello", "world"]`},
						{Name: "foo", Value: "bar"},
						{Name: "restricted", Value: "yes"},
						{Name: "switch", Value: "true"},
					},
					// Defaults since not specified.
					SecurityContext: &core.SecurityContext{
						RunAsNonRoot:             pointer.BoolPtr(false),
						ReadOnlyRootFilesystem:   pointer.BoolPtr(false),
						AllowPrivilegeEscalation: pointer.BoolPtr(true),
					},
					VolumeMounts: dataVolumeMounts(),
				}, {
					Name:  "test2",
					Image: "juju-repo.local/juju/image2",
					Ports: []core.ContainerPort{{ContainerPort: int32(8080), Protocol: core.ProtocolTCP, Name: "fred"}},
					// Defaults since not specified.
					SecurityContext: &core.SecurityContext{
						RunAsNonRoot:             pointer.BoolPtr(false),
						ReadOnlyRootFilesystem:   pointer.BoolPtr(false),
						AllowPrivilegeEscalation: pointer.BoolPtr(true),
					},
					VolumeMounts: dataVolumeMounts(),
				},
			},
			Volumes: dataVolumes(),
		},
	})
}

type K8sBrokerSuite struct {
	BaseSuite
}

var _ = gc.Suite(&K8sBrokerSuite{})

type fileSetToVolumeResultChecker func(core.Volume, error)

func (s *K8sBrokerSuite) assertFileSetToVolume(c *gc.C, fs specs.FileSet, resultChecker fileSetToVolumeResultChecker, assertCalls ...any) {

	cfgMapName := func(n string) string { return n }

	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", getBasicPodspec(), coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	workloadSpec.ConfigMaps = map[string]specs.ConfigMap{
		"log-config": map[string]string{
			"log_level": "INFO",
		},
	}
	workloadSpec.Secrets = []k8sspecs.K8sSecret{
		{Name: "mysecret2"},
	}

	annotations := map[string]string{
		"fred":                  "mary",
		"controller.juju.is/id": testing.ControllerTag.Id(),
	}

	gomock.InOrder(
		assertCalls...,
	)
	vol, err := s.broker.FileSetToVolume(
		"app-name", annotations,
		workloadSpec, fs, cfgMapName,
	)
	resultChecker(vol, err)
}

func (s *K8sBrokerSuite) TestNoNamespaceBroker(c *gc.C) {
	ctrl := gomock.NewController(c)

	s.clock = testclock.NewClock(time.Time{})

	newK8sClientFunc, newK8sRestFunc := s.setupK8sRestClient(c, ctrl, "")
	randomPrefixFunc := func() (string, error) {
		return "appuuid", nil
	}
	watcherFn := k8swatcher.NewK8sWatcherFunc(func(i cache.SharedIndexInformer, n string, c jujuclock.Clock) (k8swatcher.KubernetesNotifyWatcher, error) {
		return nil, errors.NewNotFound(nil, "undefined k8sWatcherFn for base test")
	})
	stringsWatcherFn := k8swatcher.NewK8sStringsWatcherFunc(func(i cache.SharedIndexInformer, n string, c jujuclock.Clock, e []string,
		f k8swatcher.K8sStringsWatcherFilterFunc) (k8swatcher.KubernetesStringsWatcher, error) {
		return nil, errors.NewNotFound(nil, "undefined k8sStringsWatcherFn for base test")
	})

	var err error
	s.broker, err = provider.NewK8sBroker(testing.ControllerTag.Id(), s.k8sRestConfig, s.cfg, "", newK8sClientFunc, newK8sRestFunc,
		watcherFn, stringsWatcherFn, randomPrefixFunc, s.clock)
	c.Assert(err, jc.ErrorIsNil)

	// Test namespace is actually empty string and a namespaced method fails.
	_, err = s.broker.GetPod("test")
	c.Assert(err, gc.ErrorMatches, `bootstrap broker or no namespace not provisioned`)

	nsInput := s.ensureJujuNamespaceAnnotations(false, &core.Namespace{
		ObjectMeta: v1.ObjectMeta{
			Name: "test",
		},
	})

	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get(gomock.Any(), "test", v1.GetOptions{}).Times(2).
			Return(nsInput, nil),
	)

	// Check a cluster wide resource is still accessible.
	ns, err := s.broker.GetNamespace("test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ns, gc.DeepEquals, nsInput)
}

func (s *K8sBrokerSuite) TestEnsureNamespaceAnnotationForControllerUUIDMigrated(c *gc.C) {
	ctrl := gomock.NewController(c)

	newK8sClientFunc, newK8sRestFunc := s.setupK8sRestClient(c, ctrl, s.getNamespace())
	randomPrefixFunc := func() (string, error) {
		return "appuuid", nil
	}

	newControllerUUID := names.NewControllerTag("deadbeef-1bad-500d-9000-4b1d0d06f00e").Id()
	nsBefore := s.ensureJujuNamespaceAnnotations(false, &core.Namespace{
		ObjectMeta: v1.ObjectMeta{
			Name:   "test",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "model.juju.is/name": "test"},
		},
	})
	nsAfter := *nsBefore
	nsAfter.SetAnnotations(annotations.New(nsAfter.GetAnnotations()).Add(
		k8sutils.AnnotationControllerUUIDKey(1), newControllerUUID,
	))
	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get(gomock.Any(), s.getNamespace(), v1.GetOptions{}).Times(2).
			Return(nsBefore, nil),
		s.mockNamespaces.EXPECT().Update(gomock.Any(), &nsAfter, v1.UpdateOptions{}).Times(1).
			Return(&nsAfter, nil),
	)
	s.setupBroker(c, ctrl, newControllerUUID, newK8sClientFunc, newK8sRestFunc, randomPrefixFunc, "").Finish()
}

func (s *K8sBrokerSuite) TestEnsureNamespaceAnnotationForControllerUUIDNotMigrated(c *gc.C) {
	ctrl := gomock.NewController(c)

	newK8sClientFunc, newK8sRestFunc := s.setupK8sRestClient(c, ctrl, s.getNamespace())
	randomPrefixFunc := func() (string, error) {
		return "appuuid", nil
	}

	ns := s.ensureJujuNamespaceAnnotations(false, &core.Namespace{
		ObjectMeta: v1.ObjectMeta{
			Name:   "test",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "model.juju.is/name": "test"},
		},
	})
	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get(gomock.Any(), s.getNamespace(), v1.GetOptions{}).Times(2).
			Return(ns, nil),
	)
	s.setupBroker(c, ctrl, testing.ControllerTag.Id(), newK8sClientFunc, newK8sRestFunc, randomPrefixFunc, "").Finish()
}

func (s *K8sBrokerSuite) TestEnsureNamespaceAnnotationForControllerUUIDNameSpaceNotCreatedYet(c *gc.C) {
	ctrl := gomock.NewController(c)

	newK8sClientFunc, newK8sRestFunc := s.setupK8sRestClient(c, ctrl, s.getNamespace())
	randomPrefixFunc := func() (string, error) {
		return "appuuid", nil
	}

	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get(gomock.Any(), s.getNamespace(), v1.GetOptions{}).Times(2).
			Return(nil, s.k8sNotFoundError()),
	)
	s.setupBroker(c, ctrl, testing.ControllerTag.Id(), newK8sClientFunc, newK8sRestFunc, randomPrefixFunc, "").Finish()
}

func (s *K8sBrokerSuite) TestEnsureNamespaceAnnotationForControllerUUIDNameSpaceExists(c *gc.C) {
	ctrl := gomock.NewController(c)

	newK8sClientFunc, newK8sRestFunc := s.setupK8sRestClient(c, ctrl, s.getNamespace())
	randomPrefixFunc := func() (string, error) {
		return "appuuid", nil
	}

	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get(gomock.Any(), s.getNamespace(), v1.GetOptions{}).Times(2).
			Return(&core.Namespace{
				ObjectMeta: v1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						"model.juju.is/name": "test",
						"model.juju.is/id":   "deadbeef-1bad-500d-9000-4b1d0d06f00d",
					},
				},
			}, nil),
	)
	s.setupBroker(c, ctrl, testing.ControllerTag.Id(), newK8sClientFunc, newK8sRestFunc, randomPrefixFunc, "").Finish()
}

func (s *K8sBrokerSuite) TestFileSetToVolumeFiles(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	fs := specs.FileSet{
		Name:      "configuration",
		MountPath: "/var/lib/foo",
		VolumeSource: specs.VolumeSource{
			Files: []specs.File{
				{Path: "file1", Content: "foo=bar"},
			},
		},
	}
	cm := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:   "configuration",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		Data: map[string]string{
			"file1": `foo=bar`,
		},
	}
	s.assertFileSetToVolume(
		c, fs,
		func(vol core.Volume, err error) {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(vol, gc.DeepEquals, core.Volume{
				Name: "configuration",
				VolumeSource: core.VolumeSource{
					ConfigMap: &core.ConfigMapVolumeSource{
						LocalObjectReference: core.LocalObjectReference{
							Name: "configuration",
						},
						Items: []core.KeyToPath{
							{
								Key:  "file1",
								Path: "file1",
							},
						},
					},
				},
			})
		},
		s.mockConfigMaps.EXPECT().Update(gomock.Any(), cm, v1.UpdateOptions{}).Return(cm, nil),
	)
}

func (s *K8sBrokerSuite) TestFileSetToVolumeNonFiles(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	type tc struct {
		fs            specs.FileSet
		resultChecker fileSetToVolumeResultChecker
	}

	hostPathType := core.HostPathDirectory

	for i, t := range []tc{
		{
			fs: specs.FileSet{
				Name:      "myhostpath",
				MountPath: "/host/etc/cni/net.d",
				VolumeSource: specs.VolumeSource{
					HostPath: &specs.HostPathVol{
						Path: "/etc/cni/net.d",
						Type: "Directory",
					},
				},
			},
			resultChecker: func(vol core.Volume, err error) {
				c.Check(err, jc.ErrorIsNil)
				c.Check(vol, gc.DeepEquals, core.Volume{
					Name: "myhostpath",
					VolumeSource: core.VolumeSource{
						HostPath: &core.HostPathVolumeSource{
							Path: "/etc/cni/net.d",
							Type: &hostPathType,
						},
					},
				})
			},
		},
		{
			fs: specs.FileSet{
				Name:      "cache-volume",
				MountPath: "/empty-dir",
				VolumeSource: specs.VolumeSource{
					EmptyDir: &specs.EmptyDirVol{
						Medium: "Memory",
					},
				},
			},
			resultChecker: func(vol core.Volume, err error) {
				c.Check(err, jc.ErrorIsNil)
				c.Check(vol, gc.DeepEquals, core.Volume{
					Name: "cache-volume",
					VolumeSource: core.VolumeSource{
						EmptyDir: &core.EmptyDirVolumeSource{
							Medium: core.StorageMediumMemory,
						},
					},
				})
			},
		},
		{
			fs: specs.FileSet{
				Name:      "log_level",
				MountPath: "/log-config/log_level",
				VolumeSource: specs.VolumeSource{
					ConfigMap: &specs.ResourceRefVol{
						Name:        "log-config",
						DefaultMode: pointer.Int32Ptr(511),
						Files: []specs.FileRef{
							{
								Key:  "log_level",
								Path: "log_level",
								Mode: pointer.Int32Ptr(511),
							},
						},
					},
				},
			},
			resultChecker: func(vol core.Volume, err error) {
				c.Check(err, jc.ErrorIsNil)
				c.Check(vol, gc.DeepEquals, core.Volume{
					Name: "log_level",
					VolumeSource: core.VolumeSource{
						ConfigMap: &core.ConfigMapVolumeSource{
							LocalObjectReference: core.LocalObjectReference{
								Name: "log-config",
							},
							DefaultMode: pointer.Int32Ptr(511),
							Items: []core.KeyToPath{
								{
									Key:  "log_level",
									Path: "log_level",
									Mode: pointer.Int32Ptr(511),
								},
							},
						},
					},
				})
			},
		},
		{
			fs: specs.FileSet{
				Name:      "log_level",
				MountPath: "/log-config/log_level",
				VolumeSource: specs.VolumeSource{
					ConfigMap: &specs.ResourceRefVol{
						Name:        "non-existing-config-map",
						DefaultMode: pointer.Int32Ptr(511),
						Files: []specs.FileRef{
							{
								Key:  "log_level",
								Path: "log_level",
								Mode: pointer.Int32Ptr(511),
							},
						},
					},
				},
			},
			resultChecker: func(_ core.Volume, err error) {
				c.Check(err, gc.ErrorMatches, `cannot mount a volume using a config map if the config map "non-existing-config-map" is not specified in the pod spec YAML`)
			},
		},
		{
			fs: specs.FileSet{
				Name:      "mysecret2",
				MountPath: "/secrets",
				VolumeSource: specs.VolumeSource{
					Secret: &specs.ResourceRefVol{
						Name:        "mysecret2",
						DefaultMode: pointer.Int32Ptr(511),
						Files: []specs.FileRef{
							{
								Key:  "password",
								Path: "my-group/my-password",
								Mode: pointer.Int32Ptr(511),
							},
						},
					},
				},
			},
			resultChecker: func(vol core.Volume, err error) {
				c.Check(err, jc.ErrorIsNil)
				c.Check(vol, gc.DeepEquals, core.Volume{
					Name: "mysecret2",
					VolumeSource: core.VolumeSource{
						Secret: &core.SecretVolumeSource{
							SecretName:  "mysecret2",
							DefaultMode: pointer.Int32Ptr(511),
							Items: []core.KeyToPath{
								{
									Key:  "password",
									Path: "my-group/my-password",
									Mode: pointer.Int32Ptr(511),
								},
							},
						},
					},
				})
			},
		},
		{
			fs: specs.FileSet{
				Name:      "mysecret2",
				MountPath: "/secrets",
				VolumeSource: specs.VolumeSource{
					Secret: &specs.ResourceRefVol{
						Name:        "non-existing-secret",
						DefaultMode: pointer.Int32Ptr(511),
						Files: []specs.FileRef{
							{
								Key:  "password",
								Path: "my-group/my-password",
								Mode: pointer.Int32Ptr(511),
							},
						},
					},
				},
			},
			resultChecker: func(_ core.Volume, err error) {
				c.Check(err, gc.ErrorMatches, `cannot mount a volume using a secret if the secret "non-existing-secret" is not specified in the pod spec YAML`)
			},
		},
	} {
		c.Logf("#%d: testing FileSetToVolume", i)
		s.assertFileSetToVolume(
			c, t.fs, t.resultChecker,
		)
	}
}

func (s *K8sBrokerSuite) TestConfigurePodFiles(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	cfgMapName := func(n string) string { return n }

	basicPodSpec := getBasicPodspec()
	basicPodSpec.Containers = []specs.ContainerSpec{{
		Name:         "test",
		Ports:        []specs.ContainerPort{{ContainerPort: 80, Protocol: "TCP"}},
		ImageDetails: specs.ImageDetails{ImagePath: "juju-repo.local/juju/image", Username: "fred", Password: "secret"},
		Command:      []string{"sh", "-c"},
		Args:         []string{"doIt", "--debug"},
		WorkingDir:   "/path/to/here",
		EnvConfig: map[string]interface{}{
			"foo":        "bar",
			"restricted": "yes",
			"bar":        true,
			"switch":     true,
			"brackets":   `["hello", "world"]`,
		},
		VolumeConfig: []specs.FileSet{
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
				// same volume can be mounted to `different` paths in same container.
				Name:      "cache-volume",
				MountPath: "/another-empty-dir",
				VolumeSource: specs.VolumeSource{
					EmptyDir: &specs.EmptyDirVol{
						Medium: "Memory",
					},
				},
			},
		},
	}, {
		Name:  "test2",
		Ports: []specs.ContainerPort{{ContainerPort: 8080, Protocol: "TCP", Name: "fred"}},
		Init:  true,
		Image: "juju-repo.local/juju/image2",
		VolumeConfig: []specs.FileSet{
			{
				// exact same volume can be mounted to same path in different container.
				Name:      "myhostpath",
				MountPath: "/host/etc/cni/net.d",
				VolumeSource: specs.VolumeSource{
					HostPath: &specs.HostPathVol{
						Path: "/etc/cni/net.d",
						Type: "Directory",
					},
				},
			},
		},
	}}
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	workloadSpec.ConfigMaps = map[string]specs.ConfigMap{
		"log-config": map[string]string{
			"log_level": "INFO",
		},
	}
	workloadSpec.Secrets = []k8sspecs.K8sSecret{
		{Name: "mysecret2"},
	}

	// before populate volumes to pod and volume mounts to containers.
	c.Assert(workloadSpec.Pod.Volumes, gc.DeepEquals, dataVolumes())
	workloadSpec.Pod.Containers = []core.Container{
		{
			Name:            "test",
			Image:           "juju-repo.local/juju/image",
			Ports:           []core.ContainerPort{{ContainerPort: int32(80), Protocol: core.ProtocolTCP}},
			ImagePullPolicy: core.PullAlways,
			SecurityContext: &core.SecurityContext{
				RunAsNonRoot: pointer.BoolPtr(true),
				Privileged:   pointer.BoolPtr(true),
			},
			ReadinessProbe: &core.Probe{
				InitialDelaySeconds: 10,
				ProbeHandler:        core.ProbeHandler{HTTPGet: &core.HTTPGetAction{Path: "/ready"}},
			},
			LivenessProbe: &core.Probe{
				SuccessThreshold: 20,
				ProbeHandler:     core.ProbeHandler{HTTPGet: &core.HTTPGetAction{Path: "/liveready"}},
			},
			VolumeMounts: dataVolumeMounts(),
		},
	}
	workloadSpec.Pod.InitContainers = []core.Container{
		{
			Name:  "test2",
			Image: "juju-repo.local/juju/image2",
			Ports: []core.ContainerPort{{ContainerPort: int32(8080), Protocol: core.ProtocolTCP}},
			// Defaults since not specified.
			SecurityContext: &core.SecurityContext{
				RunAsNonRoot:             pointer.BoolPtr(false),
				ReadOnlyRootFilesystem:   pointer.BoolPtr(false),
				AllowPrivilegeEscalation: pointer.BoolPtr(true),
			},
			VolumeMounts: dataVolumeMounts(),
		},
	}

	annotations := map[string]string{
		"fred":                  "mary",
		"controller.juju.is/id": testing.ControllerTag.Id(),
	}

	err = s.broker.ConfigurePodFiles(
		"app-name", annotations, workloadSpec, basicPodSpec.Containers, cfgMapName,
	)
	c.Assert(err, jc.ErrorIsNil)
	hostPathType := core.HostPathDirectory
	c.Assert(workloadSpec.Pod.Volumes, gc.DeepEquals, append(dataVolumes(), []core.Volume{
		{
			Name: "myhostpath",
			VolumeSource: core.VolumeSource{
				HostPath: &core.HostPathVolumeSource{
					Path: "/etc/cni/net.d",
					Type: &hostPathType,
				},
			},
		},
		{
			Name: "cache-volume",
			VolumeSource: core.VolumeSource{
				EmptyDir: &core.EmptyDirVolumeSource{
					Medium: core.StorageMediumMemory,
				},
			},
		},
	}...))
	c.Assert(workloadSpec.Pod.Containers, gc.DeepEquals, []core.Container{
		{
			Name:            "test",
			Image:           "juju-repo.local/juju/image",
			Ports:           []core.ContainerPort{{ContainerPort: int32(80), Protocol: core.ProtocolTCP}},
			ImagePullPolicy: core.PullAlways,
			SecurityContext: &core.SecurityContext{
				RunAsNonRoot: pointer.BoolPtr(true),
				Privileged:   pointer.BoolPtr(true),
			},
			ReadinessProbe: &core.Probe{
				InitialDelaySeconds: 10,
				ProbeHandler:        core.ProbeHandler{HTTPGet: &core.HTTPGetAction{Path: "/ready"}},
			},
			LivenessProbe: &core.Probe{
				SuccessThreshold: 20,
				ProbeHandler:     core.ProbeHandler{HTTPGet: &core.HTTPGetAction{Path: "/liveready"}},
			},
			VolumeMounts: append(dataVolumeMounts(), []core.VolumeMount{
				{Name: "myhostpath", MountPath: "/host/etc/cni/net.d"},
				{Name: "cache-volume", MountPath: "/empty-dir"},
				{Name: "cache-volume", MountPath: "/another-empty-dir"},
			}...),
		},
	})
	c.Assert(workloadSpec.Pod.InitContainers, gc.DeepEquals, []core.Container{
		{
			Name:  "test2",
			Image: "juju-repo.local/juju/image2",
			Ports: []core.ContainerPort{{ContainerPort: int32(8080), Protocol: core.ProtocolTCP}},
			// Defaults since not specified.
			SecurityContext: &core.SecurityContext{
				RunAsNonRoot:             pointer.BoolPtr(false),
				ReadOnlyRootFilesystem:   pointer.BoolPtr(false),
				AllowPrivilegeEscalation: pointer.BoolPtr(true),
			},
			VolumeMounts: append(dataVolumeMounts(), []core.VolumeMount{
				{Name: "myhostpath", MountPath: "/host/etc/cni/net.d"},
			}...),
		},
	})
}

func (s *K8sBrokerSuite) TestAPIVersion(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockDiscovery.EXPECT().ServerVersion().Return(&k8sversion.Info{
			Major: "1", Minor: "16",
		}, nil),
	)

	ver, err := s.broker.APIVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ver, gc.DeepEquals, "1.16.0")
}

func (s *K8sBrokerSuite) TestConfig(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	c.Assert(s.broker.Config(), jc.DeepEquals, s.cfg)
}

func (s *K8sBrokerSuite) TestSetConfig(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	err := s.broker.SetConfig(s.cfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestBootstrapNoOperatorStorage(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ctx := envtesting.BootstrapContext(stdcontext.TODO(), c)
	callCtx := &context.CloudCallContext{}
	bootstrapParams := environs.BootstrapParams{
		ControllerConfig:        testing.FakeControllerConfig(),
		BootstrapConstraints:    constraints.MustParse("mem=3.5G"),
		SupportedBootstrapBases: testing.FakeSupportedJujuBases,
	}

	_, err := s.broker.Bootstrap(ctx, callCtx, bootstrapParams)
	c.Assert(err, gc.NotNil)
	msg := strings.Replace(err.Error(), "\n", "", -1)
	c.Assert(msg, gc.Matches, "config without operator-storage value not valid.*")
}

func (s *K8sBrokerSuite) TestBootstrap(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	// Ensure the broker is configured with operator storage.
	s.setupOperatorStorageConfig(c)

	ctx := envtesting.BootstrapContext(stdcontext.TODO(), c)
	callCtx := &context.CloudCallContext{}
	bootstrapParams := environs.BootstrapParams{
		ControllerConfig:        testing.FakeControllerConfig(),
		BootstrapConstraints:    constraints.MustParse("mem=3.5G"),
		SupportedBootstrapBases: testing.FakeSupportedJujuBases,
	}

	sc := &storagev1.StorageClass{
		ObjectMeta: v1.ObjectMeta{
			Name: "some-storage",
		},
	}
	gomock.InOrder(
		// Check the operator storage exists.
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "test-some-storage", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "some-storage", v1.GetOptions{}).
			Return(sc, nil),
	)
	result, err := s.broker.Bootstrap(ctx, callCtx, bootstrapParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Arch, gc.Equals, "amd64")
	c.Assert(result.CaasBootstrapFinalizer, gc.NotNil)

	bootstrapParams.BootstrapBase = base.MustParseBaseFromString("ubuntu@18.04")
	_, err = s.broker.Bootstrap(ctx, callCtx, bootstrapParams)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *K8sBrokerSuite) setupOperatorStorageConfig(c *gc.C) {
	cfg := s.broker.Config()
	var err error
	cfg, err = cfg.Apply(map[string]interface{}{"operator-storage": "some-storage"})
	c.Assert(err, jc.ErrorIsNil)
	err = s.broker.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestPrepareForBootstrap(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	// Ensure the broker is configured with operator storage.
	s.setupOperatorStorageConfig(c)

	sc := &storagev1.StorageClass{
		ObjectMeta: v1.ObjectMeta{
			Name: "some-storage",
		},
	}

	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get(gomock.Any(), "controller-ctrl-1", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockNamespaces.EXPECT().List(gomock.Any(), v1.ListOptions{}).
			Return(&core.NamespaceList{Items: []core.Namespace{}}, nil),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "controller-ctrl-1-some-storage", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "some-storage", v1.GetOptions{}).
			Return(sc, nil),
	)
	ctx := envtesting.BootstrapContext(stdcontext.TODO(), c)
	c.Assert(
		s.broker.PrepareForBootstrap(ctx, "ctrl-1"), jc.ErrorIsNil,
	)
	c.Assert(s.broker.Namespace(), jc.DeepEquals, "controller-ctrl-1")
}

func (s *K8sBrokerSuite) TestPrepareForBootstrapAlreadyExistNamespaceError(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ns := &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: "controller-ctrl-1"}}
	s.ensureJujuNamespaceAnnotations(true, ns)
	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get(gomock.Any(), "controller-ctrl-1", v1.GetOptions{}).
			Return(ns, nil),
	)
	ctx := envtesting.BootstrapContext(stdcontext.TODO(), c)
	c.Assert(
		s.broker.PrepareForBootstrap(ctx, "ctrl-1"), jc.Satisfies, errors.IsAlreadyExists,
	)
}

func (s *K8sBrokerSuite) TestPrepareForBootstrapAlreadyExistControllerAnnotations(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ns := &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: "controller-ctrl-1"}}
	s.ensureJujuNamespaceAnnotations(true, ns)
	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get(gomock.Any(), "controller-ctrl-1", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockNamespaces.EXPECT().List(gomock.Any(), v1.ListOptions{}).
			Return(&core.NamespaceList{Items: []core.Namespace{*ns}}, nil),
	)
	ctx := envtesting.BootstrapContext(stdcontext.TODO(), c)
	c.Assert(
		s.broker.PrepareForBootstrap(ctx, "ctrl-1"), jc.Satisfies, errors.IsAlreadyExists,
	)
}

func (s *K8sBrokerSuite) TestGetNamespace(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ns := &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: "test"}}
	s.ensureJujuNamespaceAnnotations(false, ns)
	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get(gomock.Any(), "test", v1.GetOptions{}).
			Return(ns, nil),
	)

	out, err := s.broker.GetNamespace("test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, ns)
}

func (s *K8sBrokerSuite) TestGetNamespaceNotFound(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get(gomock.Any(), "unknown-namespace", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
	)

	out, err := s.broker.GetNamespace("unknown-namespace")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(out, gc.IsNil)
}

func (s *K8sBrokerSuite) TestNamespaces(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ns1 := s.ensureJujuNamespaceAnnotations(false, &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: "test"}})
	ns2 := s.ensureJujuNamespaceAnnotations(false, &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: "test2"}})
	gomock.InOrder(
		s.mockNamespaces.EXPECT().List(gomock.Any(), v1.ListOptions{}).
			Return(&core.NamespaceList{Items: []core.Namespace{*ns1, *ns2}}, nil),
	)

	result, err := s.broker.Namespaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.SameContents, []string{"test", "test2"})
}

func (s *K8sBrokerSuite) assertDestroy(c *gc.C, isController bool, destroyFunc func() error) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	// CRs of this Cluster scope CRD will get deleted.
	crdClusterScope := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:      "tfjobs.kubeflow.org",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d", "model.juju.is/name": "test"},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "kubeflow.org",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
				{
					Name: "v1alpha2", Served: true, Storage: false,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"tfReplicaSpecs": {
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"Worker": {
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"replicas": {
													Type:    "integer",
													Minimum: pointer.Float64Ptr(1),
												},
											},
										},
										"PS": {
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"replicas": {
													Type: "integer", Minimum: pointer.Float64Ptr(1),
												},
											},
										},
										"Chief": {
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"replicas": {
													Type:    "integer",
													Minimum: pointer.Float64Ptr(1),
													Maximum: pointer.Float64Ptr(1),
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
			Scope: apiextensionsv1.ClusterScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   "tfjobs",
				Kind:     "TFJob",
				Singular: "tfjob",
			},
		},
	}
	// CRs of this namespaced scope CRD will be skipped.
	crdNamespacedScope := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:      "tfjobs.kubeflow.org",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d", "model.juju.is/name": "test"},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "kubeflow.org",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
				{
					Name: "v1alpha2", Served: true, Storage: false,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"tfReplicaSpecs": {
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"Worker": {
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"replicas": {
													Type:    "integer",
													Minimum: pointer.Float64Ptr(1),
												},
											},
										},
										"PS": {
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"replicas": {
													Type: "integer", Minimum: pointer.Float64Ptr(1),
												},
											},
										},
										"Chief": {
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"replicas": {
													Type:    "integer",
													Minimum: pointer.Float64Ptr(1),
													Maximum: pointer.Float64Ptr(1),
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
			Scope: apiextensionsv1.NamespaceScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   "tfjobs",
				Kind:     "TFJob",
				Singular: "tfjob",
			},
		},
	}

	ns := &core.Namespace{}
	ns.Name = "test"
	s.ensureJujuNamespaceAnnotations(isController, ns)
	namespaceWatcher, namespaceFirer := k8swatchertest.NewKubernetesTestWatcher()
	s.k8sWatcherFn = k8swatchertest.NewKubernetesTestWatcherFunc(namespaceWatcher)

	// timer +1.
	s.mockClusterRoleBindings.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"}).
		Return(&rbacv1.ClusterRoleBindingList{}, nil).
		After(
			s.mockClusterRoleBindings.EXPECT().DeleteCollection(gomock.Any(),
				s.deleteOptions(v1.DeletePropagationForeground, ""),
				v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
			).Return(s.k8sNotFoundError()),
		)

	// timer +1.
	s.mockClusterRoles.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"}).
		Return(&rbacv1.ClusterRoleList{}, nil).
		After(
			s.mockClusterRoles.EXPECT().DeleteCollection(gomock.Any(),
				s.deleteOptions(v1.DeletePropagationForeground, ""),
				v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
			).Return(s.k8sNotFoundError()),
		)

	// timer +1.
	s.mockNamespaceableResourceClient.EXPECT().List(gomock.Any(),
		// list all custom resources for crd "v1alpha2".
		v1.ListOptions{LabelSelector: "juju-resource-lifecycle notin (persistent),model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
	).Return(&unstructured.UnstructuredList{}, nil).After(
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crdClusterScope.Spec.Group,
				Version:  "v1alpha2",
				Resource: crdClusterScope.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient),
	).After(
		// list all custom resources for crd "v1".
		s.mockNamespaceableResourceClient.EXPECT().List(gomock.Any(),
			v1.ListOptions{LabelSelector: "juju-resource-lifecycle notin (persistent),model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
		).Return(&unstructured.UnstructuredList{}, nil),
	).After(
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crdClusterScope.Spec.Group,
				Version:  "v1",
				Resource: crdClusterScope.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient),
	).After(
		// list cluster wide all custom resource definitions for listing custom resources.
		s.mockCustomResourceDefinitionV1.EXPECT().List(gomock.Any(), v1.ListOptions{}).AnyTimes().
			Return(&apiextensionsv1.CustomResourceDefinitionList{Items: []apiextensionsv1.CustomResourceDefinition{*crdClusterScope, *crdNamespacedScope}}, nil),
	).After(
		// delete all custom resources for crd "v1alpha2".
		s.mockNamespaceableResourceClient.EXPECT().DeleteCollection(gomock.Any(),
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "juju-resource-lifecycle notin (persistent),model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
		).Return(nil),
	).After(
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crdClusterScope.Spec.Group,
				Version:  "v1alpha2",
				Resource: crdClusterScope.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient),
	).After(
		// delete all custom resources for crd "v1".
		s.mockNamespaceableResourceClient.EXPECT().DeleteCollection(gomock.Any(),
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "juju-resource-lifecycle notin (persistent),model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
		).Return(nil),
	).After(
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crdClusterScope.Spec.Group,
				Version:  "v1",
				Resource: crdClusterScope.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient),
	).After(
		// list cluster wide all custom resource definitions for deleting custom resources.
		s.mockCustomResourceDefinitionV1.EXPECT().List(gomock.Any(), v1.ListOptions{}).AnyTimes().
			Return(&apiextensionsv1.CustomResourceDefinitionList{Items: []apiextensionsv1.CustomResourceDefinition{*crdClusterScope, *crdNamespacedScope}}, nil),
	)

	// timer +1.
	s.mockCustomResourceDefinitionV1.EXPECT().List(gomock.Any(), v1.ListOptions{
		LabelSelector: "juju-resource-lifecycle notin (persistent),model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test",
	}).AnyTimes().
		Return(&apiextensionsv1.CustomResourceDefinitionList{}, nil).
		After(
			s.mockCustomResourceDefinitionV1.EXPECT().DeleteCollection(gomock.Any(),
				s.deleteOptions(v1.DeletePropagationForeground, ""),
				v1.ListOptions{LabelSelector: "juju-resource-lifecycle notin (persistent),model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
			).Return(s.k8sNotFoundError()),
		)

	// timer +1.
	s.mockMutatingWebhookConfigurationV1.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"}).
		Return(&admissionregistrationv1.MutatingWebhookConfigurationList{}, nil).
		After(
			s.mockMutatingWebhookConfigurationV1.EXPECT().DeleteCollection(gomock.Any(),
				s.deleteOptions(v1.DeletePropagationForeground, ""),
				v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
			).Return(s.k8sNotFoundError()),
		)

	// timer +1.
	s.mockValidatingWebhookConfigurationV1.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"}).
		Return(&admissionregistrationv1.ValidatingWebhookConfigurationList{}, nil).
		After(
			s.mockValidatingWebhookConfigurationV1.EXPECT().DeleteCollection(gomock.Any(),
				s.deleteOptions(v1.DeletePropagationForeground, ""),
				v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
			).Return(s.k8sNotFoundError()),
		)

	// timer +1.
	s.mockStorageClass.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"}).
		Return(&storagev1.StorageClassList{}, nil).
		After(
			s.mockStorageClass.EXPECT().DeleteCollection(gomock.Any(),
				s.deleteOptions(v1.DeletePropagationForeground, ""),
				v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
			).Return(nil),
		)

	s.mockNamespaces.EXPECT().Get(gomock.Any(), "test", v1.GetOptions{}).
		Return(ns, nil)
	s.mockNamespaces.EXPECT().Delete(gomock.Any(), "test", s.deleteOptions(v1.DeletePropagationForeground, "")).
		Return(nil)
	// still terminating.
	s.mockNamespaces.EXPECT().Get(gomock.Any(), "test", v1.GetOptions{}).
		DoAndReturn(func(_, _, _ interface{}) (*core.Namespace, error) {
			namespaceFirer()
			return ns, nil
		})
	// terminated, not found returned.
	s.mockNamespaces.EXPECT().Get(gomock.Any(), "test", v1.GetOptions{}).
		Return(nil, s.k8sNotFoundError())

	errCh := make(chan error)
	go func() {
		errCh <- destroyFunc()
	}()

	err := s.clock.WaitAdvance(time.Second, testing.ShortWait, 6)
	c.Assert(err, jc.ErrorIsNil)
	err = s.clock.WaitAdvance(time.Second, testing.ShortWait, 1)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case err := <-errCh:
		c.Assert(err, jc.ErrorIsNil)
		for _, watcher := range s.watchers {
			c.Assert(workertest.CheckKilled(c, watcher), jc.ErrorIsNil)
		}
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for destroyFunc return")
	}
}

func (s *K8sBrokerSuite) TestDestroyController(c *gc.C) {
	s.assertDestroy(c, true, func() error {
		return s.broker.DestroyController(context.NewEmptyCloudCallContext(), testing.ControllerTag.Id())
	})
}

func (s *K8sBrokerSuite) TestEnsureImageRepoSecret(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	imageRepo := docker.ImageRepoDetails{
		Repository:    "test-account",
		ServerAddress: "quay.io",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: docker.NewToken("xxxxx=="),
		},
	}

	data, err := imageRepo.SecretData()
	c.Assert(err, jc.ErrorIsNil)

	secret := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-image-pull-secret",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju"},
			Annotations: map[string]string{
				"controller.juju.is/id": testing.ControllerTag.Id(),
				"model.juju.is/id":      testing.ModelTag.Id(),
			},
		},
		Type: core.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			core.DockerConfigJsonKey: data,
		},
	}

	gomock.InOrder(
		s.mockSecrets.EXPECT().Create(gomock.Any(), secret, v1.CreateOptions{}).
			Return(secret, nil),
	)
	err = s.broker.EnsureImageRepoSecret(imageRepo)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestDestroy(c *gc.C) {
	s.assertDestroy(c, false, func() error { return s.broker.Destroy(context.NewEmptyCloudCallContext()) })
}

func (s *K8sBrokerSuite) TestGetCurrentNamespace(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()
	c.Assert(s.broker.Namespace(), jc.DeepEquals, s.getNamespace())
}

func (s *K8sBrokerSuite) TestCreate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ns := s.ensureJujuNamespaceAnnotations(false, &core.Namespace{
		ObjectMeta: v1.ObjectMeta{
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d", "model.juju.is/name": "test"},
			Name:   "test",
		},
	})
	gomock.InOrder(
		s.mockNamespaces.EXPECT().Create(gomock.Any(), ns, v1.CreateOptions{}).
			Return(ns, nil),
	)

	err := s.broker.Create(
		&context.CloudCallContext{},
		environs.CreateParams{},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestCreateAlreadyExists(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ns := s.ensureJujuNamespaceAnnotations(false, &core.Namespace{
		ObjectMeta: v1.ObjectMeta{
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d", "model.juju.is/name": "test"},
			Name:   "test",
		},
	})
	gomock.InOrder(
		s.mockNamespaces.EXPECT().Create(gomock.Any(), ns, v1.CreateOptions{}).
			Return(nil, s.k8sAlreadyExistsError()),
	)

	err := s.broker.Create(
		&context.CloudCallContext{},
		environs.CreateParams{},
	)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
}

func unitStatefulSetArg(numUnits int32, scName string, podSpec core.PodSpec) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"app.juju.is/uuid":               "appuuid",
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"charm.juju.is/modified-version": "0",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"controller.juju.is/id":                    testing.ControllerTag.Id(),
						"charm.juju.is/modified-version":           "0",
					},
				},
				Spec: podSpec,
			},
			VolumeClaimTemplates: []core.PersistentVolumeClaim{{
				ObjectMeta: v1.ObjectMeta{
					Name:   "database-appuuid",
					Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "storage.juju.is/name": "database"},
					Annotations: map[string]string{
						"foo":                  "bar",
						"storage.juju.is/name": "database",
					}},
				Spec: core.PersistentVolumeClaimSpec{
					StorageClassName: &scName,
					AccessModes:      []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
					Resources: core.VolumeResourceRequirements{
						Requests: core.ResourceList{
							core.ResourceStorage: resource.MustParse("100Mi"),
						},
					},
				},
			}},
			PodManagementPolicy: appsv1.ParallelPodManagement,
			ServiceName:         "app-name-endpoints",
		},
	}
}

func (s *K8sBrokerSuite) TestDeleteServiceForApplication(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:      "tfjobs.kubeflow.org",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.kubernetes.io/name": "test"},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "kubeflow.org",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
				{
					Name: "v1alpha2", Served: true, Storage: false,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"tfReplicaSpecs": {
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"Worker": {
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"replicas": {
													Type:    "integer",
													Minimum: pointer.Float64Ptr(1),
												},
											},
										},
										"PS": {
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"replicas": {
													Type: "integer", Minimum: pointer.Float64Ptr(1),
												},
											},
										},
										"Chief": {
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"replicas": {
													Type:    "integer",
													Minimum: pointer.Float64Ptr(1),
													Maximum: pointer.Float64Ptr(1),
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
			Scope: "Namespaced",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   "tfjobs",
				Kind:     "TFJob",
				Singular: "tfjob",
			},
		},
	}

	// Delete operations below return a not found to ensure it's treated as a no-op.
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-test", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),

		s.mockServices.EXPECT().Delete(gomock.Any(), "test", s.deleteOptions(v1.DeletePropagationForeground, "")).
			Return(s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Delete(gomock.Any(), "test", s.deleteOptions(v1.DeletePropagationForeground, "")).
			Return(s.k8sNotFoundError()),
		s.mockServices.EXPECT().Delete(gomock.Any(), "test-endpoints", s.deleteOptions(v1.DeletePropagationForeground, "")).
			Return(s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Delete(gomock.Any(), "test", s.deleteOptions(v1.DeletePropagationForeground, "")).
			Return(s.k8sNotFoundError()),

		s.mockStatefulSets.EXPECT().DeleteCollection(gomock.Any(),
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=test"},
		).Return(nil),
		s.mockDeployments.EXPECT().DeleteCollection(gomock.Any(),
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=test"},
		).Return(nil),

		s.mockServices.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=test"}).
			Return(&core.ServiceList{}, nil),

		// delete secrets.
		s.mockSecrets.EXPECT().DeleteCollection(gomock.Any(),
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=test"},
		).Return(nil),

		// delete configmaps.
		s.mockConfigMaps.EXPECT().DeleteCollection(gomock.Any(),
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=test"},
		).Return(nil),

		// delete RBAC resources.
		s.mockRoleBindings.EXPECT().DeleteCollection(gomock.Any(),
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=test"},
		).Return(nil),
		s.mockClusterRoleBindings.EXPECT().DeleteCollection(gomock.Any(),
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=test,model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
		).Return(nil),
		s.mockRoles.EXPECT().DeleteCollection(gomock.Any(),
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=test"},
		).Return(nil),
		s.mockClusterRoles.EXPECT().DeleteCollection(gomock.Any(),
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=test,model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
		).Return(nil),
		s.mockServiceAccounts.EXPECT().DeleteCollection(gomock.Any(),
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=test"},
		).Return(nil),

		// list cluster wide all custom resource definitions for deleting custom resources.
		s.mockCustomResourceDefinitionV1.EXPECT().List(gomock.Any(), v1.ListOptions{}).
			Return(&apiextensionsv1.CustomResourceDefinitionList{Items: []apiextensionsv1.CustomResourceDefinition{*crd}}, nil),
		// delete all custom resources for crd "v1".
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crd.Spec.Group,
				Version:  "v1",
				Resource: crd.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient),
		s.mockResourceClient.EXPECT().DeleteCollection(gomock.Any(),
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=test,juju-resource-lifecycle notin (model,persistent)"},
		).Return(nil),
		// delete all custom resources for crd "v1alpha2".
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crd.Spec.Group,
				Version:  "v1alpha2",
				Resource: crd.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient),
		s.mockResourceClient.EXPECT().DeleteCollection(gomock.Any(),
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=test,juju-resource-lifecycle notin (model,persistent)"},
		).Return(nil),

		// delete all custom resource definitions.
		s.mockCustomResourceDefinitionV1.EXPECT().DeleteCollection(gomock.Any(),
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=test,juju-resource-lifecycle notin (model,persistent),model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
		).Return(nil),

		// delete all mutating webhook configurations.
		s.mockMutatingWebhookConfigurationV1.EXPECT().DeleteCollection(gomock.Any(),
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=test,model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
		).Return(nil),

		// delete all validating webhook configurations.
		s.mockValidatingWebhookConfigurationV1.EXPECT().DeleteCollection(gomock.Any(),
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=test,model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
		).Return(nil),

		// delete all ingress resources.
		s.mockIngressV1.EXPECT().DeleteCollection(gomock.Any(),
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=test"},
		).Return(nil),

		// delete all daemon set resources.
		s.mockDaemonSets.EXPECT().DeleteCollection(gomock.Any(),
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=test"},
		).Return(nil),
	)

	err := s.broker.DeleteService("test")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceNoUnits(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	two := int32(2)
	dc := &appsv1.Deployment{ObjectMeta: v1.ObjectMeta{Name: "juju-unit-storage"}, Spec: appsv1.DeploymentSpec{Replicas: &two}}
	zero := int32(0)
	emptyDc := dc
	emptyDc.Spec.Replicas = &zero
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(dc, nil),
		s.mockDeployments.EXPECT().Update(gomock.Any(), emptyDc, v1.UpdateOptions{}).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: getBasicPodspec(),
	}
	err := s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 0, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceNoSpecProvided(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
	)

	params := &caas.ServiceParams{}
	err := s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 1, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceBothPodSpecAndRawK8sSpecProvided(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
	)

	params := &caas.ServiceParams{
		PodSpec:    getBasicPodspec(),
		RawK8sSpec: `fake raw spec`,
	}
	err := s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 1, nil)
	c.Assert(err, gc.ErrorMatches, `both pod spec and raw k8s spec provided not valid`)
}

func (s *K8sBrokerSuite) TestEnsureServiceNoStorage(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	numUnits := int32(2)
	basicPodSpec := getBasicPodspec()
	basicPodSpec.ProviderPod = &k8sspecs.K8sPodSpec{
		KubernetesResources: &k8sspecs.KubernetesResources{
			Pod: &k8sspecs.PodSpec{Annotations: map[string]string{"foo": "baz"}},
		},
	}
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec

	deploymentArg := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                           "mary",
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"app.juju.is/uuid":               "appuuid",
				"charm.juju.is/modified-version": "0",
			}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels:       map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"foo": "baz",
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"fred":                           "mary",
						"controller.juju.is/id":          testing.ControllerTag.Id(),
						"charm.juju.is/modified-version": "0",
					},
				},
				Spec: podSpec,
			},
		},
	}
	serviceArg := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id": testing.ControllerTag.Id(),
				"fred":                  "mary",
				"a":                     "b",
			}},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"app.kubernetes.io/name": "app-name"},
			Type:     "LoadBalancer",
			Ports: []core.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt(80), Protocol: "TCP"},
				{Port: 8080, Protocol: "TCP", Name: "fred"},
			},
			LoadBalancerIP: "10.0.0.1",
			ExternalName:   "ext-name",
		},
	}

	ociImageSecret := s.getOCIImageSecret(c, map[string]string{"fred": "mary"})
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, v1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), serviceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), serviceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockDeployments.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Create(gomock.Any(), deploymentArg, v1.CreateOptions{}).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec:      basicPodSpec,
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
			"fred":                 "mary",
		},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
		"kubernetes-service-annotations":     map[string]interface{}{"a": "b"},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceUpgrade(c *gc.C) {
	// TODO: use this instead of gomock inside k8s testing.
	k8sClientSet := k8sfake.NewSimpleClientset()
	extClientSet := apiextensionsclientsetfake.NewSimpleClientset()
	dynamicClientSet := k8sdynamicfake.NewSimpleDynamicClient(k8sruntime.NewScheme())
	restClient := &k8srestfake.RESTClient{}

	newK8sClientFunc := func(cfg *rest.Config) (kubernetes.Interface, apiextensionsclientset.Interface, dynamic.Interface, error) {
		c.Assert(cfg.Username, gc.Equals, "fred")
		c.Assert(cfg.Password, gc.Equals, "secret")
		c.Assert(cfg.Host, gc.Equals, "some-host")
		c.Assert(cfg.TLSClientConfig, jc.DeepEquals, rest.TLSClientConfig{
			CertData: []byte("cert-data"),
			KeyData:  []byte("cert-key"),
			CAData:   []byte(testing.CACert),
		})
		return k8sClientSet, extClientSet, dynamicClientSet, nil
	}
	newK8sRestFunc := func(cfg *rest.Config) (rest.Interface, error) {
		return restClient, nil
	}
	randomPrefixFunc := func() (string, error) {
		return "appuuid", nil
	}
	s.setupBroker(c, nil, testing.ControllerTag.Id(), newK8sClientFunc, newK8sRestFunc, randomPrefixFunc, "")

	basicPodSpec := getBasicPodspec()
	basicPodSpec.ProviderPod = &k8sspecs.K8sPodSpec{
		KubernetesResources: &k8sspecs.KubernetesResources{
			Pod: &k8sspecs.PodSpec{Annotations: map[string]string{"foo": "baz"}},
		},
	}
	params := &caas.ServiceParams{
		PodSpec: basicPodSpec,
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
			"fred":                 "mary",
		},
	}
	err := s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
		"kubernetes-service-annotations":     map[string]interface{}{"a": "b"},
	})
	c.Assert(err, jc.ErrorIsNil)

	listFirst, err := k8sClientSet.AppsV1().Deployments(s.getNamespace()).List(stdcontext.TODO(), v1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(listFirst.Items, gc.HasLen, 1)

	// Update and swap the ports between containers
	basicPodSpec2 := getBasicPodspec()
	basicPodSpec2.ProviderPod = &k8sspecs.K8sPodSpec{
		KubernetesResources: &k8sspecs.KubernetesResources{
			Pod: &k8sspecs.PodSpec{Annotations: map[string]string{"foo": "baz"}},
		},
	}
	swap := basicPodSpec2.Containers[0].Ports
	basicPodSpec2.Containers[0].Ports = basicPodSpec2.Containers[1].Ports
	basicPodSpec2.Containers[1].Ports = swap
	params2 := &caas.ServiceParams{
		PodSpec: basicPodSpec2,
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
			"fred":                 "mary",
		},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params2, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
		"kubernetes-service-annotations":     map[string]interface{}{"a": "b"},
	})
	c.Assert(err, jc.ErrorIsNil)

	listLast, err := k8sClientSet.AppsV1().Deployments(s.getNamespace()).List(stdcontext.TODO(), v1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(listFirst.Items, gc.HasLen, 1)

	before := listFirst.Items[0]
	after := listLast.Items[0]

	// Check the containers swapped their ports between them.
	mc := jc.NewMultiChecker()
	mc.AddExpr(`_.Spec.Template.Spec.Containers[_].Ports[_].Name`, jc.Ignore)
	mc.AddExpr(`_.Spec.Template.Spec.Containers[_].Ports[_].ContainerPort`, jc.Ignore)
	c.Assert(after, mc, before)
	c.Assert(before.Spec.Template.Spec.Containers[0].Ports[0], gc.DeepEquals, core.ContainerPort{
		Name:          "",
		ContainerPort: 80,
		Protocol:      core.ProtocolTCP,
	})
	c.Assert(before.Spec.Template.Spec.Containers[1].Ports[0], gc.DeepEquals, core.ContainerPort{
		Name:          "fred",
		ContainerPort: 8080,
		Protocol:      core.ProtocolTCP,
	})
	c.Assert(after.Spec.Template.Spec.Containers[0].Ports[0], gc.DeepEquals, core.ContainerPort{
		Name:          "fred",
		ContainerPort: 8080,
		Protocol:      core.ProtocolTCP,
	})
	c.Assert(after.Spec.Template.Spec.Containers[1].Ports[0], gc.DeepEquals, core.ContainerPort{
		Name:          "",
		ContainerPort: 80,
		Protocol:      core.ProtocolTCP,
	})
}

func (s *K8sBrokerSuite) TestEnsureServiceForDeploymentWithUpdateStrategy(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	numUnits := int32(2)
	basicPodSpec := getBasicPodspec()

	basicPodSpec.Service = &specs.ServiceSpec{
		UpdateStrategy: &specs.UpdateStrategy{
			Type: "RollingUpdate",
			RollingUpdate: &specs.RollingUpdateSpec{
				MaxUnavailable: &specs.IntOrString{IntVal: 10},
				MaxSurge:       &specs.IntOrString{IntVal: 20},
			},
		},
	}

	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec

	deploymentArg := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                           "mary",
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"app.juju.is/uuid":               "appuuid",
				"charm.juju.is/modified-version": "0",
			}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels:       map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"fred":                           "mary",
						"controller.juju.is/id":          testing.ControllerTag.Id(),
						"charm.juju.is/modified-version": "0",
					},
				},
				Spec: podSpec,
			},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &intstr.IntOrString{IntVal: 10},
					MaxSurge:       &intstr.IntOrString{IntVal: 20},
				},
			},
		},
	}
	serviceArg := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id": testing.ControllerTag.Id(),
				"fred":                  "mary",
				"a":                     "b",
			}},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"app.kubernetes.io/name": "app-name"},
			Type:     "LoadBalancer",
			Ports: []core.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt(80), Protocol: "TCP"},
				{Port: 8080, Protocol: "TCP", Name: "fred"},
			},
			LoadBalancerIP: "10.0.0.1",
			ExternalName:   "ext-name",
		},
	}

	ociImageSecret := s.getOCIImageSecret(c, map[string]string{"fred": "mary"})
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, v1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), serviceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), serviceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockDeployments.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Create(gomock.Any(), deploymentArg, v1.CreateOptions{}).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec:      basicPodSpec,
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
			"fred":                 "mary",
		},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
		"kubernetes-service-annotations":     map[string]interface{}{"a": "b"},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceStatelessWithScalePolicyInvalid(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	basicPodSpec.Service = &specs.ServiceSpec{
		// ScalePolicy is only for statefulset.
		ScalePolicy: specs.SerialScale,
	}

	ociImageSecret := s.getOCIImageSecret(c, map[string]string{"fred": "mary"})
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, v1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Delete(gomock.Any(), ociImageSecret.GetName(), s.deleteOptions(v1.DeletePropagationForeground, "")).
			Return(nil),
	)

	params := &caas.ServiceParams{
		PodSpec:      basicPodSpec,
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
			"fred":                 "mary",
		},
	}
	err := s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
		"kubernetes-service-annotations":     map[string]interface{}{"a": "b"},
	})
	c.Assert(err, gc.ErrorMatches, `ScalePolicy is only supported for stateful applications`)
}

func (s *K8sBrokerSuite) TestEnsureServiceWithExtraServicesConfigMapAndSecretsCreate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	numUnits := int32(2)
	basicPodSpec := getBasicPodspec()
	basicPodSpec.ConfigMaps = map[string]specs.ConfigMap{
		"myData": {
			"foo":   "bar",
			"hello": "world",
		},
	}
	basicPodSpec.ProviderPod = &k8sspecs.K8sPodSpec{
		KubernetesResources: &k8sspecs.KubernetesResources{
			Services: []k8sspecs.K8sService{
				{
					Meta: k8sspecs.Meta{
						Name:        "my-service1",
						Labels:      map[string]string{"foo": "bar"},
						Annotations: map[string]string{"cloud.google.com/load-balancer-type": "Internal"},
					},
					Spec: core.ServiceSpec{
						Selector: map[string]string{"app": "MyApp"},
						Ports: []core.ServicePort{
							{
								Protocol:   core.ProtocolTCP,
								Port:       80,
								TargetPort: intstr.IntOrString{IntVal: 9376},
							},
						},
						Type: core.ServiceTypeLoadBalancer,
					},
				},
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
		},
	}

	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec

	deploymentArg := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"fred":                           "mary",
				"app.juju.is/uuid":               "appuuid",
				"charm.juju.is/modified-version": "0",
			}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels:       map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"fred":                           "mary",
						"controller.juju.is/id":          testing.ControllerTag.Id(),
						"charm.juju.is/modified-version": "0",
					},
				},
				Spec: podSpec,
			},
		},
	}
	serviceArg := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id": testing.ControllerTag.Id(),
				"fred":                  "mary",
				"a":                     "b",
			}},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"app.kubernetes.io/name": "app-name"},
			Type:     "LoadBalancer",
			Ports: []core.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt(80), Protocol: "TCP"},
				{Port: 8080, Protocol: "TCP", Name: "fred"},
			},
			LoadBalancerIP: "10.0.0.1",
			ExternalName:   "ext-name",
		},
	}
	svc1 := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      "my-service1",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "foo": "bar"},
			Annotations: map[string]string{
				"controller.juju.is/id":               testing.ControllerTag.Id(),
				"fred":                                "mary",
				"cloud.google.com/load-balancer-type": "Internal",
			}},
		Spec: core.ServiceSpec{
			Selector: k8sutils.LabelForKeyValue("app", "MyApp"),
			Type:     core.ServiceTypeLoadBalancer,
			Ports: []core.ServicePort{
				{
					Protocol:   core.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.IntOrString{IntVal: 9376},
				},
			},
		},
	}

	cm := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      "myData",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id": testing.ControllerTag.Id(),
				"fred":                  "mary",
			},
		},
		Data: map[string]string{
			"foo":   "bar",
			"hello": "world",
		},
	}
	secrets1 := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "build-robot-secret",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id": testing.ControllerTag.Id(),
				"fred":                  "mary",
			},
		},
		Type: core.SecretTypeOpaque,
		StringData: map[string]string{
			"config.yaml": `
apiUrl: "https://my.api.com/api/v1"
username: fred
password: shhhh`[1:],
		},
	}

	secrets2Data, err := provider.ProcessSecretData(
		map[string]string{
			"username": "YWRtaW4=",
			"password": "MWYyZDFlMmU2N2Rm",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	secrets2 := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "another-build-robot-secret",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id": testing.ControllerTag.Id(),
				"fred":                  "mary",
			},
		},
		Type: core.SecretTypeOpaque,
		Data: secrets2Data,
	}

	ociImageSecret := s.getOCIImageSecret(c, map[string]string{"fred": "mary"})
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),

		// ensure services.
		s.mockServices.EXPECT().Get(gomock.Any(), svc1.GetName(), v1.GetOptions{}).
			Return(svc1, nil),
		s.mockServices.EXPECT().Update(gomock.Any(), svc1, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), svc1, v1.CreateOptions{}).
			Return(svc1, nil),

		// ensure configmaps.
		s.mockConfigMaps.EXPECT().Create(gomock.Any(), cm, v1.CreateOptions{}).
			Return(cm, nil),

		// ensure secrets.
		s.mockSecrets.EXPECT().Create(gomock.Any(), secrets1, v1.CreateOptions{}).
			Return(secrets1, nil),
		s.mockSecrets.EXPECT().Create(gomock.Any(), secrets2, v1.CreateOptions{}).
			Return(secrets2, nil),

		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, v1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), serviceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), serviceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockDeployments.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Create(gomock.Any(), deploymentArg, v1.CreateOptions{}).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec:      basicPodSpec,
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
			"fred":                 "mary",
		},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
		"kubernetes-service-annotations":     map[string]interface{}{"a": "b"},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceWithExtraServicesConfigMapAndSecretsUpdate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	numUnits := int32(2)
	basicPodSpec := getBasicPodspec()
	basicPodSpec.ConfigMaps = map[string]specs.ConfigMap{
		"myData": {
			"foo":   "bar",
			"hello": "world",
		},
	}
	basicPodSpec.ProviderPod = &k8sspecs.K8sPodSpec{
		KubernetesResources: &k8sspecs.KubernetesResources{
			Services: []k8sspecs.K8sService{
				{
					Meta: k8sspecs.Meta{
						Name:        "my-service1",
						Labels:      map[string]string{"foo": "bar"},
						Annotations: map[string]string{"cloud.google.com/load-balancer-type": "Internal"},
					},
					Spec: core.ServiceSpec{
						Selector: map[string]string{"app": "MyApp"},
						Ports: []core.ServicePort{
							{
								Protocol:   core.ProtocolTCP,
								Port:       80,
								TargetPort: intstr.IntOrString{IntVal: 9376},
							},
						},
						Type: core.ServiceTypeLoadBalancer,
					},
				},
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
		},
	}

	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec

	deploymentArg := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"fred":                           "mary",
				"app.juju.is/uuid":               "appuuid",
				"charm.juju.is/modified-version": "0",
			}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels:       map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"fred":                           "mary",
						"controller.juju.is/id":          testing.ControllerTag.Id(),
						"charm.juju.is/modified-version": "0",
					},
				},
				Spec: podSpec,
			},
		},
	}
	serviceArg := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id": testing.ControllerTag.Id(),
				"fred":                  "mary",
				"a":                     "b",
			}},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"app.kubernetes.io/name": "app-name"},
			Type:     "LoadBalancer",
			Ports: []core.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt(80), Protocol: "TCP"},
				{Port: 8080, Protocol: "TCP", Name: "fred"},
			},
			LoadBalancerIP: "10.0.0.1",
			ExternalName:   "ext-name",
		},
	}

	svc1 := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      "my-service1",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "foo": "bar"},
			Annotations: map[string]string{
				"controller.juju.is/id":               testing.ControllerTag.Id(),
				"fred":                                "mary",
				"cloud.google.com/load-balancer-type": "Internal",
			}},
		Spec: core.ServiceSpec{
			Selector: k8sutils.LabelForKeyValue("app", "MyApp"),
			Type:     core.ServiceTypeLoadBalancer,
			Ports: []core.ServicePort{
				{
					Protocol:   core.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.IntOrString{IntVal: 9376},
				},
			},
		},
	}

	cm := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      "myData",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id": testing.ControllerTag.Id(),
				"fred":                  "mary",
			},
		},
		Data: map[string]string{
			"foo":   "bar",
			"hello": "world",
		},
	}
	secrets1 := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "build-robot-secret",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id": testing.ControllerTag.Id(),
				"fred":                  "mary",
			},
		},
		Type: core.SecretTypeOpaque,
		StringData: map[string]string{
			"config.yaml": `
apiUrl: "https://my.api.com/api/v1"
username: fred
password: shhhh`[1:],
		},
	}

	secrets2Data, err := provider.ProcessSecretData(
		map[string]string{
			"username": "YWRtaW4=",
			"password": "MWYyZDFlMmU2N2Rm",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	secrets2 := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "another-build-robot-secret",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id": testing.ControllerTag.Id(),
				"fred":                  "mary",
			},
		},
		Type: core.SecretTypeOpaque,
		Data: secrets2Data,
	}

	ociImageSecret := s.getOCIImageSecret(c, map[string]string{"fred": "mary"})
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),

		// ensure services.
		s.mockServices.EXPECT().Get(gomock.Any(), svc1.GetName(), v1.GetOptions{}).
			Return(svc1, nil),
		s.mockServices.EXPECT().Update(gomock.Any(), svc1, v1.UpdateOptions{}).
			Return(svc1, nil),

		// ensure configmaps.
		s.mockConfigMaps.EXPECT().Create(gomock.Any(), cm, v1.CreateOptions{}).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockConfigMaps.EXPECT().Update(gomock.Any(), cm, v1.UpdateOptions{}).
			Return(cm, nil),

		// ensure secrets.
		s.mockSecrets.EXPECT().Create(gomock.Any(), secrets1, v1.CreateOptions{}).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockSecrets.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=app-name"}).
			Return(&core.SecretList{Items: []core.Secret{*secrets1}}, nil),
		s.mockSecrets.EXPECT().Update(gomock.Any(), secrets1, v1.UpdateOptions{}).
			Return(secrets1, nil),
		s.mockSecrets.EXPECT().Create(gomock.Any(), secrets2, v1.CreateOptions{}).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockSecrets.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=app-name"}).
			Return(&core.SecretList{Items: []core.Secret{*secrets2}}, nil),
		s.mockSecrets.EXPECT().Update(gomock.Any(), secrets2, v1.UpdateOptions{}).
			Return(secrets2, nil),

		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, v1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), serviceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), serviceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockDeployments.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Create(gomock.Any(), deploymentArg, v1.CreateOptions{}).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: basicPodSpec,
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
			"fred":                 "mary",
		},
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
		"kubernetes-service-annotations":     map[string]interface{}{"a": "b"},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestVersion(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockDiscovery.EXPECT().ServerVersion().Return(&k8sversion.Info{
			Major: "1", Minor: "15+",
		}, nil),
	)

	ver, err := s.broker.Version()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ver, gc.DeepEquals, &version.Number{Major: 1, Minor: 15})
}

func (s *K8sBrokerSuite) TestSupportedFeatures(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockDiscovery.EXPECT().ServerVersion().Return(&k8sversion.Info{
			Major: "1", Minor: "15+",
		}, nil),
	)

	fs, err := s.broker.SupportedFeatures()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fs.AsList(), gc.DeepEquals, []assumes.Feature{
		{
			Name:        "k8s-api",
			Description: "the Kubernetes API lets charms query and manipulate the state of API objects in a Kubernetes cluster",
			Version:     &version.Number{Major: 1, Minor: 15},
		},
	})
}

func (s *K8sBrokerSuite) TestGetServiceSvcNotFound(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockServices.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=app-name"}).
			Return(&core.ServiceList{Items: []core.Service{}}, nil),

		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),

		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDaemonSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
	)

	caasSvc, err := s.broker.GetService("app-name", caas.ModeWorkload, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(caasSvc, gc.DeepEquals, &caas.Service{})
}

func (s *K8sBrokerSuite) assertGetService(c *gc.C, mode caas.DeploymentMode, expectedSvcResult *caas.Service, assertCalls ...any) {
	selectorLabels := map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"}
	if mode == caas.ModeOperator {
		selectorLabels = map[string]string{
			"app.kubernetes.io/managed-by": "juju", "operator.juju.is/name": "app-name", "operator.juju.is/target": "application"}
	}
	labels := k8sutils.LabelsMerge(selectorLabels, k8sutils.LabelsJuju)

	selector := k8sutils.LabelsToSelector(labels).String()
	svc := core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: labels,
			Annotations: map[string]string{
				"controller.juju.is/id": testing.ControllerTag.Id(),
				"fred":                  "mary",
				"a":                     "b",
			}},
		Spec: core.ServiceSpec{
			Selector: selectorLabels,
			Type:     core.ServiceTypeLoadBalancer,
			Ports: []core.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt(80), Protocol: "TCP"},
				{Port: 8080, Protocol: "TCP", Name: "fred"},
			},
			LoadBalancerIP: "10.0.0.1",
			ExternalName:   "ext-name",
		},
		Status: core.ServiceStatus{
			LoadBalancer: core.LoadBalancerStatus{
				Ingress: []core.LoadBalancerIngress{{
					Hostname: "host.com.au",
				}},
			},
		},
	}
	svc.SetUID("uid-xxxxx")
	svcHeadless := core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name-endpoints",
			Labels: labels,
			Annotations: map[string]string{
				"juju.io/controller": testing.ControllerTag.Id(),
				"fred":               "mary",
				"a":                  "b",
			}},
		Spec: core.ServiceSpec{
			Selector: labels,
			Type:     core.ServiceTypeClusterIP,
			Ports: []core.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt(80), Protocol: "TCP"},
			},
			ClusterIP: "192.168.1.1",
		},
	}
	gomock.InOrder(
		append([]any{
			s.mockServices.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: selector}).
				Return(&core.ServiceList{Items: []core.Service{svcHeadless, svc}}, nil),

			s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
				Return(nil, s.k8sNotFoundError()),
		}, assertCalls...)...,
	)

	caasSvc, err := s.broker.GetService("app-name", mode, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(caasSvc, gc.DeepEquals, expectedSvcResult)
}

func (s *K8sBrokerSuite) TestGetServiceSvcFoundNoWorkload(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()
	s.assertGetService(c,
		caas.ModeWorkload,
		&caas.Service{
			Id: "uid-xxxxx",
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("10.0.0.1", network.WithScope(network.ScopePublic)).AsProviderAddress(),
				network.NewMachineAddress("host.com.au", network.WithScope(network.ScopePublic)).AsProviderAddress(),
			},
		},
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDaemonSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
	)
}

func (s *K8sBrokerSuite) TestGetServiceSvcFoundWithStatefulSet(c *gc.C) {
	for _, mode := range []caas.DeploymentMode{caas.ModeOperator, caas.ModeWorkload} {
		s.assertGetServiceSvcFoundWithStatefulSet(c, mode)
	}
}

func (s *K8sBrokerSuite) assertGetServiceSvcFoundWithStatefulSet(c *gc.C, mode caas.DeploymentMode) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	basicPodSpec.Service = &specs.ServiceSpec{
		ScalePolicy: "serial",
	}

	appName := "app-name"
	if mode == caas.ModeOperator {
		appName += "-operator"
	}

	workloadSpec, err := provider.PrepareWorkloadSpec(
		appName, "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec

	numUnits := int32(2)
	workload := &appsv1.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name:   appName,
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"app.juju.is/uuid":               "appuuid",
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"charm.juju.is/modified-version": "0",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"controller.juju.is/id":                    testing.ControllerTag.Id(),
						"charm.juju.is/modified-version":           "0",
					},
				},
				Spec: podSpec,
			},
			PodManagementPolicy: appsv1.PodManagementPolicyType("OrderedReady"),
			ServiceName:         "app-name-endpoints",
		},
	}
	workload.SetGeneration(1)

	var expectedCalls []any
	if mode == caas.ModeOperator {
		expectedCalls = append(expectedCalls,
			s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name-operator", v1.GetOptions{}).
				Return(nil, s.k8sNotFoundError()),
		)
	}
	expectedCalls = append(expectedCalls,
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), appName, v1.GetOptions{}).
			Return(workload, nil),
		s.mockEvents.EXPECT().List(gomock.Any(),
			listOptionsFieldSelectorMatcher(fmt.Sprintf("involvedObject.name=%s,involvedObject.kind=StatefulSet", appName)),
		).Return(&core.EventList{}, nil),
	)

	s.assertGetService(c,
		mode,
		&caas.Service{
			Id: "uid-xxxxx",
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("10.0.0.1", network.WithScope(network.ScopePublic)).AsProviderAddress(),
				network.NewMachineAddress("host.com.au", network.WithScope(network.ScopePublic)).AsProviderAddress(),
			},
			Scale:      k8sutils.IntPtr(2),
			Generation: pointer.Int64Ptr(1),
			Status: status.StatusInfo{
				Status: status.Active,
			},
		},
		expectedCalls...,
	)
}

func (s *K8sBrokerSuite) TestGetServiceSvcFoundWithDeployment(c *gc.C) {
	for _, mode := range []caas.DeploymentMode{caas.ModeOperator, caas.ModeWorkload} {
		s.assertGetServiceSvcFoundWithDeployment(c, mode)
	}
}

func (s *K8sBrokerSuite) assertGetServiceSvcFoundWithDeployment(c *gc.C, mode caas.DeploymentMode) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	basicPodSpec.Service = &specs.ServiceSpec{
		ScalePolicy: "serial",
	}

	appName := "app-name"
	if mode == caas.ModeOperator {
		appName += "-operator"
	}

	workloadSpec, err := provider.PrepareWorkloadSpec(
		appName, "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec

	numUnits := int32(2)
	workload := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:   appName,
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"fred":                           "mary",
				"charm.juju.is/modified-version": "0",
			}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels:       map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"fred":                           "mary",
						"controller.juju.is/id":          testing.ControllerTag.Id(),
						"charm.juju.is/modified-version": "0",
					},
				},
				Spec: podSpec,
			},
		},
	}
	workload.SetGeneration(1)

	var expectedCalls []any
	if mode == caas.ModeOperator {
		expectedCalls = append(expectedCalls,
			s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name-operator", v1.GetOptions{}).
				Return(nil, s.k8sNotFoundError()),
		)
	}
	expectedCalls = append(expectedCalls,
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), appName, v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Get(gomock.Any(), appName, v1.GetOptions{}).
			Return(workload, nil),
		s.mockEvents.EXPECT().List(gomock.Any(),
			listOptionsFieldSelectorMatcher(fmt.Sprintf("involvedObject.name=%s,involvedObject.kind=Deployment", appName)),
		).Return(&core.EventList{}, nil),
	)

	s.assertGetService(c,
		mode,
		&caas.Service{
			Id: "uid-xxxxx",
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("10.0.0.1", network.WithScope(network.ScopePublic)).AsProviderAddress(),
				network.NewMachineAddress("host.com.au", network.WithScope(network.ScopePublic)).AsProviderAddress(),
			},
			Scale:      k8sutils.IntPtr(2),
			Generation: pointer.Int64Ptr(1),
			Status: status.StatusInfo{
				Status: status.Active,
			},
		},
		expectedCalls...,
	)
}

func (s *K8sBrokerSuite) TestGetServiceSvcFoundWithDaemonSet(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	basicPodSpec.Service = &specs.ServiceSpec{
		ScalePolicy: "serial",
	}
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec

	workload := &appsv1.DaemonSet{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"charm.juju.is/modified-version": "0",
			},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels:       map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"controller.juju.is/id":                    testing.ControllerTag.Id(),
						"charm.juju.is/modified-version":           "0",
					},
				},
				Spec: podSpec,
			},
		},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 2,
			NumberReady:            2,
		},
	}
	workload.SetGeneration(1)

	s.assertGetService(c,
		caas.ModeWorkload,
		&caas.Service{
			Id: "uid-xxxxx",
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("10.0.0.1", network.WithScope(network.ScopePublic)).AsProviderAddress(),
				network.NewMachineAddress("host.com.au", network.WithScope(network.ScopePublic)).AsProviderAddress(),
			},
			Scale:      k8sutils.IntPtr(2),
			Generation: pointer.Int64Ptr(1),
			Status: status.StatusInfo{
				Status: status.Active,
			},
		},
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDaemonSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(workload, nil),
		s.mockEvents.EXPECT().List(gomock.Any(),
			listOptionsFieldSelectorMatcher("involvedObject.name=app-name,involvedObject.kind=DaemonSet"),
		).Return(&core.EventList{}, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureServiceNoStorageStateful(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	basicPodSpec.Service = &specs.ServiceSpec{
		ScalePolicy: "serial",
	}
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec

	numUnits := int32(2)
	statefulSetArg := &appsv1.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"app.juju.is/uuid":               "appuuid",
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"charm.juju.is/modified-version": "0",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"controller.juju.is/id":                    testing.ControllerTag.Id(),
						"charm.juju.is/modified-version":           "0",
					},
				},
				Spec: podSpec,
			},
			PodManagementPolicy: appsv1.PodManagementPolicyType("OrderedReady"),
			ServiceName:         "app-name-endpoints",
		},
	}

	serviceArg := *basicServiceArg
	serviceArg.Spec.Type = core.ServiceTypeClusterIP
	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, v1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), &serviceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), &serviceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name-endpoints", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicHeadlessServiceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicHeadlessServiceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Create(gomock.Any(), statefulSetArg, v1.CreateOptions{}).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: basicPodSpec,
		Deployment: caas.DeploymentParams{
			DeploymentType: caas.DeploymentStateful,
		},
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
		ResourceTags: map[string]string{"juju-controller-uuid": testing.ControllerTag.Id()},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceCustomType(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec

	numUnits := int32(2)
	statefulSetArg := &appsv1.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"app.juju.is/uuid":               "appuuid",
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"charm.juju.is/modified-version": "0",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"controller.juju.is/id":                    testing.ControllerTag.Id(),
						"charm.juju.is/modified-version":           "0",
					},
				},
				Spec: podSpec,
			},
			PodManagementPolicy: appsv1.ParallelPodManagement,
			ServiceName:         "app-name-endpoints",
		},
	}

	serviceArg := *basicServiceArg
	serviceArg.Spec.Type = core.ServiceTypeExternalName
	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, v1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(&appsv1.StatefulSet{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{"app.juju.is/uuid": "appuuid"}}}, nil),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), &serviceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), &serviceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name-endpoints", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicHeadlessServiceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicHeadlessServiceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(statefulSetArg, nil),
		s.mockStatefulSets.EXPECT().Create(gomock.Any(), statefulSetArg, v1.CreateOptions{}).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: basicPodSpec,
		Deployment: caas.DeploymentParams{
			ServiceType: caas.ServiceExternal,
		},
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
		ResourceTags: map[string]string{"juju-controller-uuid": testing.ControllerTag.Id()},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceServiceWithoutPortsNotValid(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	serviceArg := *basicServiceArg
	serviceArg.Spec.Type = core.ServiceTypeExternalName
	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, v1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockSecrets.EXPECT().Delete(gomock.Any(), "app-name-test-secret", s.deleteOptions(v1.DeletePropagationForeground, "")).
			Return(nil),
	)
	caasPodSpec := getBasicPodspec()
	for k, v := range caasPodSpec.Containers {
		v.Ports = []specs.ContainerPort{}
		caasPodSpec.Containers[k] = v
	}
	c.Assert(caasPodSpec.OmitServiceFrontend, jc.IsFalse)
	for _, v := range caasPodSpec.Containers {
		c.Check(len(v.Ports), jc.DeepEquals, 0)
	}
	params := &caas.ServiceParams{
		PodSpec: caasPodSpec,
		Deployment: caas.DeploymentParams{
			DeploymentType: caas.DeploymentStateful,
			ServiceType:    caas.ServiceExternal,
		},
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
		ResourceTags: map[string]string{"juju-controller-uuid": testing.ControllerTag.Id()},
	}
	err := s.broker.EnsureService(
		"app-name",
		func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil },
		params, 2,
		config.ConfigAttributes{
			"kubernetes-service-loadbalancer-ip": "10.0.0.1",
			"kubernetes-service-externalname":    "ext-name",
		},
	)
	c.Assert(err, gc.ErrorMatches, `ports are required for kubernetes service "app-name"`)
}

func (s *K8sBrokerSuite) TestEnsureServiceWithServiceAccountNewRoleCreate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	podSpec := getBasicPodspec()
	podSpec.ServiceAccount = primeServiceAccount

	numUnits := int32(2)
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", podSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)

	deploymentArg := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                           "mary",
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"app.juju.is/uuid":               "appuuid",
				"charm.juju.is/modified-version": "0",
			}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels:       map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"fred":                           "mary",
						"controller.juju.is/id":          testing.ControllerTag.Id(),
						"charm.juju.is/modified-version": "0",
					},
				},
				Spec: provider.Pod(workloadSpec).PodSpec,
			},
		},
	}
	serviceArg := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"a":                     "b",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			}},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"app.kubernetes.io/name": "app-name"},
			Type:     "LoadBalancer",
			Ports: []core.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt(80), Protocol: "TCP"},
				{Port: 8080, Protocol: "TCP", Name: "fred"},
			},
			LoadBalancerIP: "10.0.0.1",
			ExternalName:   "ext-name",
		},
	}

	svcAccount := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		AutomountServiceAccountToken: pointer.BoolPtr(true),
	}
	role := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "watch", "list"},
			},
		},
	}
	rb := &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			Name: "app-name",
			Kind: "Role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "app-name",
				Namespace: "test",
			},
		},
	}

	secretArg := s.getOCIImageSecret(c, map[string]string{"fred": "mary"})
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServiceAccounts.EXPECT().Create(gomock.Any(), svcAccount, v1.CreateOptions{}).Return(svcAccount, nil),
		s.mockRoles.EXPECT().Create(gomock.Any(), role, v1.CreateOptions{}).Return(role, nil),
		s.mockRoleBindings.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockRoleBindings.EXPECT().Create(gomock.Any(), rb, v1.CreateOptions{}).Return(rb, nil),
		s.mockSecrets.EXPECT().Create(gomock.Any(), secretArg, v1.CreateOptions{}).Return(secretArg, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), serviceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), serviceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockDeployments.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Create(gomock.Any(), deploymentArg, v1.CreateOptions{}).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: podSpec,
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
			"fred":                 "mary",
		},
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
		"kubernetes-service-annotations":     map[string]interface{}{"a": "b"},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceWithServiceAccountNewRoleUpdate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	podSpec := getBasicPodspec()
	podSpec.ServiceAccount = primeServiceAccount

	numUnits := int32(2)
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", podSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)

	deploymentArg := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                           "mary",
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"app.juju.is/uuid":               "appuuid",
				"charm.juju.is/modified-version": "0",
			}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels:       map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"fred":                           "mary",
						"controller.juju.is/id":          testing.ControllerTag.Id(),
						"charm.juju.is/modified-version": "0",
					},
				},
				Spec: provider.Pod(workloadSpec).PodSpec,
			},
		},
	}
	serviceArg := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"a":                     "b",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			}},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"app.kubernetes.io/name": "app-name"},
			Type:     "LoadBalancer",
			Ports: []core.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt(80), Protocol: "TCP"},
				{Port: 8080, Protocol: "TCP", Name: "fred"},
			},
			LoadBalancerIP: "10.0.0.1",
			ExternalName:   "ext-name",
		},
	}

	svcAccount := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		AutomountServiceAccountToken: pointer.BoolPtr(true),
	}
	role := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "watch", "list"},
			},
		},
	}
	rb := &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			Name: "app-name",
			Kind: "Role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "app-name",
				Namespace: "test",
			},
		},
	}

	secretArg := s.getOCIImageSecret(c, map[string]string{"fred": "mary"})
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServiceAccounts.EXPECT().Create(gomock.Any(), svcAccount, v1.CreateOptions{}).Return(nil, s.k8sAlreadyExistsError()),
		s.mockServiceAccounts.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(svcAccount, nil),
		s.mockServiceAccounts.EXPECT().Update(gomock.Any(), svcAccount, v1.UpdateOptions{}).Return(svcAccount, nil),
		s.mockRoles.EXPECT().Create(gomock.Any(), role, v1.CreateOptions{}).Return(nil, s.k8sAlreadyExistsError()),
		s.mockRoles.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(role, nil),
		s.mockRoles.EXPECT().Update(gomock.Any(), role, v1.UpdateOptions{}).Return(role, nil),
		s.mockRoleBindings.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(rb, nil),
		s.mockSecrets.EXPECT().Create(gomock.Any(), secretArg, v1.CreateOptions{}).Return(secretArg, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), serviceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), serviceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockDeployments.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Create(gomock.Any(), deploymentArg, v1.CreateOptions{}).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: podSpec,
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
			"fred":                 "mary",
		},
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	}

	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
		"kubernetes-service-annotations":     map[string]interface{}{"a": "b"},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceWithServiceAccountNewClusterRoleCreate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	podSpec := getBasicPodspec()
	podSpec.ServiceAccount = &specs.PrimeServiceAccountSpecV3{
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

	numUnits := int32(2)
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", podSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)

	deploymentArg := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                           "mary",
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"app.juju.is/uuid":               "appuuid",
				"charm.juju.is/modified-version": "0",
			}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels:       map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"fred":                           "mary",
						"controller.juju.is/id":          testing.ControllerTag.Id(),
						"charm.juju.is/modified-version": "0",
					},
				},
				Spec: provider.Pod(workloadSpec).PodSpec,
			},
		},
	}
	serviceArg := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"a":                     "b",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			}},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"app.kubernetes.io/name": "app-name"},
			Type:     "LoadBalancer",
			Ports: []core.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt(80), Protocol: "TCP"},
				{Port: 8080, Protocol: "TCP", Name: "fred"},
			},
			LoadBalancerIP: "10.0.0.1",
			ExternalName:   "ext-name",
		},
	}

	svcAccount := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		AutomountServiceAccountToken: pointer.BoolPtr(true),
	}
	cr := &rbacv1.ClusterRole{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-app-name",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d", "model.juju.is/name": "test"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "watch", "list"},
			},
		},
	}
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name-test-app-name",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d", "model.juju.is/name": "test"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			Name: "test-app-name",
			Kind: "ClusterRole",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "app-name",
				Namespace: "test",
			},
		},
	}

	secretArg := s.getOCIImageSecret(c, map[string]string{"fred": "mary"})
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServiceAccounts.EXPECT().Create(gomock.Any(), svcAccount, v1.CreateOptions{}).Return(svcAccount, nil),
		s.mockClusterRoles.EXPECT().Get(gomock.Any(), cr.Name, gomock.Any()).Return(nil, s.k8sNotFoundError()),
		s.mockClusterRoles.EXPECT().Patch(
			gomock.Any(), cr.Name, types.StrategicMergePatchType, gomock.Any(), v1.PatchOptions{FieldManager: "juju"},
		).Return(nil, s.k8sNotFoundError()),
		s.mockClusterRoles.EXPECT().Create(gomock.Any(), cr, gomock.Any()).Return(cr, nil),
		s.mockClusterRoleBindings.EXPECT().Get(gomock.Any(), crb.Name, gomock.Any()).Return(nil, s.k8sNotFoundError()),
		s.mockClusterRoleBindings.EXPECT().Patch(
			gomock.Any(), crb.Name, types.StrategicMergePatchType, gomock.Any(), v1.PatchOptions{FieldManager: "juju"},
		).Return(nil, s.k8sNotFoundError()),
		s.mockClusterRoleBindings.EXPECT().Create(gomock.Any(), crb, gomock.Any()).Return(crb, nil),
		s.mockSecrets.EXPECT().Create(gomock.Any(), secretArg, v1.CreateOptions{}).Return(secretArg, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), serviceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), serviceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockDeployments.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Create(gomock.Any(), deploymentArg, v1.CreateOptions{}).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: podSpec,
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
			"fred":                 "mary",
		},
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
		"kubernetes-service-annotations":     map[string]interface{}{"a": "b"},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceWithServiceAccountNewClusterRoleUpdate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	podSpec := getBasicPodspec()
	podSpec.ServiceAccount = &specs.PrimeServiceAccountSpecV3{
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

	numUnits := int32(2)
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", podSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)

	deploymentArg := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                           "mary",
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"app.juju.is/uuid":               "appuuid",
				"charm.juju.is/modified-version": "0",
			}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels:       map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"fred":                           "mary",
						"controller.juju.is/id":          testing.ControllerTag.Id(),
						"charm.juju.is/modified-version": "0",
					},
				},
				Spec: provider.Pod(workloadSpec).PodSpec,
			},
		},
	}
	serviceArg := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"a":                     "b",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			}},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"app.kubernetes.io/name": "app-name"},
			Type:     "LoadBalancer",
			Ports: []core.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt(80), Protocol: "TCP"},
				{Port: 8080, Protocol: "TCP", Name: "fred"},
			},
			LoadBalancerIP: "10.0.0.1",
			ExternalName:   "ext-name",
		},
	}

	svcAccount := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		AutomountServiceAccountToken: pointer.BoolPtr(true),
	}
	cr := &rbacv1.ClusterRole{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-app-name",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d", "model.juju.is/name": "test"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "watch", "list"},
			},
		},
	}
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name-test-app-name",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d", "model.juju.is/name": "test"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			Name: "test-app-name",
			Kind: "ClusterRole",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "app-name",
				Namespace: "test",
			},
		},
	}

	secretArg := s.getOCIImageSecret(c, map[string]string{"fred": "mary"})
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServiceAccounts.EXPECT().Create(gomock.Any(), svcAccount, v1.CreateOptions{}).Return(nil, s.k8sAlreadyExistsError()),
		s.mockServiceAccounts.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(svcAccount, nil),
		s.mockServiceAccounts.EXPECT().Update(gomock.Any(), svcAccount, v1.UpdateOptions{}).Return(svcAccount, nil),
		s.mockClusterRoles.EXPECT().Get(gomock.Any(), cr.Name, gomock.Any()).Return(cr, nil),
		s.mockClusterRoles.EXPECT().Update(gomock.Any(), cr, gomock.Any()).Return(cr, nil),
		s.mockClusterRoleBindings.EXPECT().Get(gomock.Any(), crb.Name, gomock.Any()).Return(nil, s.k8sNotFoundError()),
		s.mockClusterRoleBindings.EXPECT().Patch(
			gomock.Any(), crb.Name, types.StrategicMergePatchType, gomock.Any(), v1.PatchOptions{FieldManager: "juju"},
		).Return(nil, s.k8sNotFoundError()),
		s.mockClusterRoleBindings.EXPECT().Create(gomock.Any(), crb, gomock.Any()).Return(crb, nil),
		s.mockSecrets.EXPECT().Create(gomock.Any(), secretArg, v1.CreateOptions{}).Return(secretArg, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), serviceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), serviceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockDeployments.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Create(gomock.Any(), deploymentArg, v1.CreateOptions{}).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: podSpec,
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
			"fred":                 "mary",
		},
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	}

	errChan := make(chan error)
	go func() {
		errChan <- s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
			"kubernetes-service-type":            "loadbalancer",
			"kubernetes-service-loadbalancer-ip": "10.0.0.1",
			"kubernetes-service-externalname":    "ext-name",
			"kubernetes-service-annotations":     map[string]interface{}{"a": "b"},
		})
	}()

	select {
	case err := <-errChan:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for EnsureService return")
	}
}

func (s *K8sBrokerSuite) TestEnsureServiceWithServiceAccountAndK8sServiceAccountNameSpaced(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	podSpec := getBasicPodspec()
	podSpec.ServiceAccount = primeServiceAccount

	podSpec.ProviderPod = &k8sspecs.K8sPodSpec{
		KubernetesResources: &k8sspecs.KubernetesResources{
			K8sRBACResources: k8sspecs.K8sRBACResources{
				ServiceAccounts: []k8sspecs.K8sServiceAccountSpec{
					{
						Name: "sa2",
						ServiceAccountSpecV3: specs.ServiceAccountSpecV3{

							AutomountServiceAccountToken: pointer.BoolPtr(true),
							Roles: []specs.Role{
								{
									Name: "role2",
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
			},
		},
	}

	numUnits := int32(2)
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", podSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)

	deploymentArg := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                           "mary",
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"app.juju.is/uuid":               "appuuid",
				"charm.juju.is/modified-version": "0",
			}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels:       map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"fred":                           "mary",
						"controller.juju.is/id":          testing.ControllerTag.Id(),
						"charm.juju.is/modified-version": "0",
					},
				},
				Spec: provider.Pod(workloadSpec).PodSpec,
			},
		},
	}
	serviceArg := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"a":                     "b",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			}},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"app.kubernetes.io/name": "app-name"},
			Type:     "LoadBalancer",
			Ports: []core.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt(80), Protocol: "TCP"},
				{Port: 8080, Protocol: "TCP", Name: "fred"},
			},
			LoadBalancerIP: "10.0.0.1",
			ExternalName:   "ext-name",
		},
	}

	svcAccount1 := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		AutomountServiceAccountToken: pointer.BoolPtr(true),
	}
	role1 := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "watch", "list"},
			},
		},
	}
	rb1 := &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			Name: "app-name",
			Kind: "Role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "app-name",
				Namespace: "test",
			},
		},
	}

	svcAccount2 := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      "sa2",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		AutomountServiceAccountToken: pointer.BoolPtr(true),
	}
	role2 := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:      "role2",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "watch", "list"},
			},
		},
	}
	rb2 := &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:      "sa2-role2",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			Name: "role2",
			Kind: "Role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "sa2",
				Namespace: "test",
			},
		},
	}

	secretArg := s.getOCIImageSecret(c, map[string]string{"fred": "mary"})
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),

		s.mockServiceAccounts.EXPECT().Create(gomock.Any(), svcAccount1, v1.CreateOptions{}).Return(svcAccount1, nil),
		s.mockRoles.EXPECT().Create(gomock.Any(), role1, v1.CreateOptions{}).Return(role1, nil),
		s.mockRoleBindings.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockRoleBindings.EXPECT().Create(gomock.Any(), rb1, v1.CreateOptions{}).Return(rb1, nil),

		s.mockServiceAccounts.EXPECT().Create(gomock.Any(), svcAccount2, v1.CreateOptions{}).Return(svcAccount2, nil),
		s.mockRoles.EXPECT().Create(gomock.Any(), role2, v1.CreateOptions{}).Return(role2, nil),
		s.mockRoleBindings.EXPECT().Get(gomock.Any(), "sa2-role2", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockRoleBindings.EXPECT().Create(gomock.Any(), rb2, v1.CreateOptions{}).Return(rb2, nil),

		s.mockSecrets.EXPECT().Create(gomock.Any(), secretArg, v1.CreateOptions{}).Return(secretArg, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), serviceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), serviceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockDeployments.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Create(gomock.Any(), deploymentArg, v1.CreateOptions{}).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: podSpec,
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
			"fred":                 "mary",
		},
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
		"kubernetes-service-annotations":     map[string]interface{}{"a": "b"},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceWithServiceAccountAndK8sServiceAccountClusterScoped(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	podSpec := getBasicPodspec()
	podSpec.ServiceAccount = primeServiceAccount

	podSpec.ProviderPod = &k8sspecs.K8sPodSpec{
		KubernetesResources: &k8sspecs.KubernetesResources{
			K8sRBACResources: k8sspecs.K8sRBACResources{
				ServiceAccounts: []k8sspecs.K8sServiceAccountSpec{
					{
						Name: "sa2",
						ServiceAccountSpecV3: specs.ServiceAccountSpecV3{
							AutomountServiceAccountToken: pointer.BoolPtr(true),
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
			},
		},
	}

	numUnits := int32(2)
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", podSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)

	deploymentArg := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"fred":                           "mary",
				"app.juju.is/uuid":               "appuuid",
				"charm.juju.is/modified-version": "0",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels:       map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"controller.juju.is/id":                    testing.ControllerTag.Id(),
						"fred":                                     "mary",
						"charm.juju.is/modified-version":           "0",
					},
				},
				Spec: provider.Pod(workloadSpec).PodSpec,
			},
		},
	}
	serviceArg := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"a":                     "b",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			}},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"app.kubernetes.io/name": "app-name"},
			Type:     "LoadBalancer",
			Ports: []core.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt(80), Protocol: "TCP"},
				{Port: 8080, Protocol: "TCP", Name: "fred"},
			},
			LoadBalancerIP: "10.0.0.1",
			ExternalName:   "ext-name",
		},
	}

	svcAccount1 := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		AutomountServiceAccountToken: pointer.BoolPtr(true),
	}
	role1 := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "watch", "list"},
			},
		},
	}
	rb1 := &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			Name: "app-name",
			Kind: "Role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "app-name",
				Namespace: "test",
			},
		},
	}

	svcAccount2 := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      "sa2",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		AutomountServiceAccountToken: pointer.BoolPtr(true),
	}
	clusterrole2 := &rbacv1.ClusterRole{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-cluster-role2",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d", "model.juju.is/name": "test"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		Rules: []rbacv1.PolicyRule{
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
	}
	crb2 := &rbacv1.ClusterRoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:      "sa2-test-cluster-role2",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d", "model.juju.is/name": "test"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			Name: "test-cluster-role2",
			Kind: "ClusterRole",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "sa2",
				Namespace: "test",
			},
		},
	}

	secretArg := s.getOCIImageSecret(c, map[string]string{"fred": "mary"})
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),

		s.mockServiceAccounts.EXPECT().Create(gomock.Any(), svcAccount1, v1.CreateOptions{}).Return(svcAccount1, nil),
		s.mockRoles.EXPECT().Create(gomock.Any(), role1, v1.CreateOptions{}).Return(role1, nil),
		s.mockRoleBindings.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockRoleBindings.EXPECT().Create(gomock.Any(), rb1, v1.CreateOptions{}).Return(rb1, nil),

		s.mockServiceAccounts.EXPECT().Create(gomock.Any(), svcAccount2, v1.CreateOptions{}).Return(svcAccount2, nil),
		s.mockClusterRoles.EXPECT().Get(gomock.Any(), clusterrole2.Name, gomock.Any()).Return(clusterrole2, nil),
		s.mockClusterRoles.EXPECT().Update(gomock.Any(), clusterrole2, gomock.Any()).Return(clusterrole2, nil),
		s.mockClusterRoleBindings.EXPECT().Get(gomock.Any(), crb2.Name, gomock.Any()).Return(nil, s.k8sNotFoundError()),
		s.mockClusterRoleBindings.EXPECT().Patch(
			gomock.Any(), crb2.Name, types.StrategicMergePatchType, gomock.Any(), v1.PatchOptions{FieldManager: "juju"},
		).Return(nil, s.k8sNotFoundError()),
		s.mockClusterRoleBindings.EXPECT().Create(gomock.Any(), crb2, gomock.Any()).Return(crb2, nil),

		s.mockSecrets.EXPECT().Create(gomock.Any(), secretArg, v1.CreateOptions{}).Return(secretArg, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), serviceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), serviceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockDeployments.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Create(gomock.Any(), deploymentArg, v1.CreateOptions{}).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: podSpec,
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
			"fred":                 "mary",
		},
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
		"kubernetes-service-annotations":     map[string]interface{}{"a": "b"},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceWithServiceAccountAndK8sServiceAccountWithoutRoleNames(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	podSpec := getBasicPodspec()
	podSpec.ServiceAccount = primeServiceAccount

	podSpec.ProviderPod = &k8sspecs.K8sPodSpec{
		KubernetesResources: &k8sspecs.KubernetesResources{
			K8sRBACResources: k8sspecs.K8sRBACResources{
				ServiceAccounts: []k8sspecs.K8sServiceAccountSpec{
					{
						Name: "sa-foo",
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
								{
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
			},
		},
	}

	numUnits := int32(2)
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", podSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)

	deploymentArg := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"fred":                           "mary",
				"app.juju.is/uuid":               "appuuid",
				"charm.juju.is/modified-version": "0",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels:       map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"controller.juju.is/id":                    testing.ControllerTag.Id(),
						"fred":                                     "mary",
						"charm.juju.is/modified-version":           "0",
					},
				},
				Spec: provider.Pod(workloadSpec).PodSpec,
			},
		},
	}
	serviceArg := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"a":                     "b",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			}},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"app.kubernetes.io/name": "app-name"},
			Type:     "LoadBalancer",
			Ports: []core.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt(80), Protocol: "TCP"},
				{Port: 8080, Protocol: "TCP", Name: "fred"},
			},
			LoadBalancerIP: "10.0.0.1",
			ExternalName:   "ext-name",
		},
	}

	svcAccount1 := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		AutomountServiceAccountToken: pointer.BoolPtr(true),
	}
	role1 := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "watch", "list"},
			},
		},
	}
	rb1 := &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			Name: "app-name",
			Kind: "Role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "app-name",
				Namespace: "test",
			},
		},
	}

	svcAccount2 := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      "sa-foo",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		AutomountServiceAccountToken: pointer.BoolPtr(true),
	}
	clusterrole2 := &rbacv1.ClusterRole{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-sa-foo",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d", "model.juju.is/name": "test"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		Rules: []rbacv1.PolicyRule{
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
	}
	role2 := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:      "sa-foo1",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "watch", "list"},
			},
		},
	}
	crb2 := &rbacv1.ClusterRoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:      "sa-foo-test-sa-foo",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d", "model.juju.is/name": "test"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			Name: "test-sa-foo",
			Kind: "ClusterRole",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "sa-foo",
				Namespace: "test",
			},
		},
	}
	rb2 := &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:      "sa-foo-sa-foo1",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			Name: "sa-foo1",
			Kind: "Role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "sa-foo",
				Namespace: "test",
			},
		},
	}

	secretArg := s.getOCIImageSecret(c, map[string]string{"fred": "mary"})
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),

		s.mockServiceAccounts.EXPECT().Create(gomock.Any(), svcAccount1, v1.CreateOptions{}).Return(svcAccount1, nil),
		s.mockRoles.EXPECT().Create(gomock.Any(), role1, v1.CreateOptions{}).Return(role1, nil),
		s.mockRoleBindings.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockRoleBindings.EXPECT().Create(gomock.Any(), rb1, v1.CreateOptions{}).Return(rb1, nil),

		s.mockServiceAccounts.EXPECT().Create(gomock.Any(), svcAccount2, v1.CreateOptions{}).Return(svcAccount2, nil),
		s.mockRoles.EXPECT().Create(gomock.Any(), role2, v1.CreateOptions{}).Return(role2, nil),
		s.mockRoleBindings.EXPECT().Get(gomock.Any(), "sa-foo-sa-foo1", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockRoleBindings.EXPECT().Create(gomock.Any(), rb2, v1.CreateOptions{}).Return(rb2, nil),
		s.mockClusterRoles.EXPECT().Get(gomock.Any(), clusterrole2.Name, gomock.Any()).Return(clusterrole2, nil),
		s.mockClusterRoles.EXPECT().Update(gomock.Any(), clusterrole2, gomock.Any()).Return(clusterrole2, nil),
		s.mockClusterRoleBindings.EXPECT().Get(gomock.Any(), crb2.Name, gomock.Any()).Return(nil, s.k8sNotFoundError()),
		s.mockClusterRoleBindings.EXPECT().Patch(
			gomock.Any(), crb2.Name, types.StrategicMergePatchType, gomock.Any(), v1.PatchOptions{FieldManager: "juju"},
		).Return(nil, s.k8sNotFoundError()),
		s.mockClusterRoleBindings.EXPECT().Create(gomock.Any(), crb2, gomock.Any()).Return(crb2, nil),

		s.mockSecrets.EXPECT().Create(gomock.Any(), secretArg, v1.CreateOptions{}).Return(secretArg, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), serviceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), serviceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockDeployments.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Create(gomock.Any(), deploymentArg, v1.CreateOptions{}).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: podSpec,
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
			"fred":                 "mary",
		},
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
		"kubernetes-service-annotations":     map[string]interface{}{"a": "b"},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceWithStorage(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	basicPodSpec.ProviderPod = &k8sspecs.K8sPodSpec{
		KubernetesResources: &k8sspecs.KubernetesResources{
			Pod: &k8sspecs.PodSpec{
				Labels:      map[string]string{"foo": "bax"},
				Annotations: map[string]string{"foo": "baz"},
			},
		},
	}
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec
	podSpec.Containers[0].VolumeMounts = append(dataVolumeMounts(), core.VolumeMount{
		Name:      "database-appuuid",
		MountPath: "path/to/here",
	}, core.VolumeMount{
		Name:      "logs-1",
		MountPath: "path/to/there",
	})
	size, err := resource.ParseQuantity("200Mi")
	c.Assert(err, jc.ErrorIsNil)
	podSpec.Volumes = append(podSpec.Volumes, core.Volume{
		Name: "logs-1",
		VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{
			SizeLimit: &size,
			Medium:    "Memory",
		}},
	})
	statefulSetArg := unitStatefulSetArg(2, "workload-storage", podSpec)
	statefulSetArg.Spec.Template.Annotations["foo"] = "baz"
	statefulSetArg.Spec.Template.Labels["foo"] = "bax"
	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, v1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicServiceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicServiceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name-endpoints", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicHeadlessServiceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicHeadlessServiceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(&appsv1.StatefulSet{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{"app.juju.is/uuid": "appuuid"}}}, nil),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "test-workload-storage", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "workload-storage", v1.GetOptions{}).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "workload-storage"}}, nil),
		s.mockStatefulSets.EXPECT().Create(gomock.Any(), statefulSetArg, v1.CreateOptions{}).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec:      basicPodSpec,
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
		},
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
			Provider:    "kubernetes",
			Attributes:  map[string]interface{}{"storage-class": "workload-storage"},
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/here",
			},
			ResourceTags: map[string]string{"foo": "bar"},
		}, {
			StorageName: "logs",
			Size:        200,
			Provider:    "tmpfs",
			Attributes:  map[string]interface{}{"storage-medium": "Memory"},
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/there",
			},
		}},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceForStatefulSetWithUpdateStrategy(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()

	basicPodSpec.Service = &specs.ServiceSpec{
		UpdateStrategy: &specs.UpdateStrategy{
			Type: "RollingUpdate",
			RollingUpdate: &specs.RollingUpdateSpec{
				Partition: pointer.Int32Ptr(10),
			},
		},
	}

	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec
	podSpec.Containers[0].VolumeMounts = append(dataVolumeMounts(), core.VolumeMount{
		Name:      "database-appuuid",
		MountPath: "path/to/here",
	}, core.VolumeMount{
		Name:      "logs-1",
		MountPath: "path/to/there",
	})
	size, err := resource.ParseQuantity("200Mi")
	c.Assert(err, jc.ErrorIsNil)
	podSpec.Volumes = append(podSpec.Volumes, core.Volume{
		Name: "logs-1",
		VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{
			SizeLimit: &size,
			Medium:    "Memory",
		}},
	})
	statefulSetArg := unitStatefulSetArg(2, "workload-storage", podSpec)
	statefulSetArg.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{
		Type: appsv1.RollingUpdateStatefulSetStrategyType,
		RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
			Partition: pointer.Int32Ptr(10),
		},
	}

	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, v1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicServiceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicServiceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name-endpoints", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicHeadlessServiceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicHeadlessServiceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(&appsv1.StatefulSet{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{"app.juju.is/uuid": "appuuid"}}}, nil),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "test-workload-storage", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "workload-storage", v1.GetOptions{}).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "workload-storage"}}, nil),
		s.mockStatefulSets.EXPECT().Create(gomock.Any(), statefulSetArg, v1.CreateOptions{}).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec:      basicPodSpec,
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
		},
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
			Provider:    "kubernetes",
			Attributes:  map[string]interface{}{"storage-class": "workload-storage"},
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/here",
			},
			ResourceTags: map[string]string{"foo": "bar"},
		}, {
			StorageName: "logs",
			Size:        200,
			Provider:    "tmpfs",
			Attributes:  map[string]interface{}{"storage-medium": "Memory"},
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/there",
			},
		}},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceForDeploymentWithDevices(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	numUnits := int32(2)
	basicPodSpec := getBasicPodspec()
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec
	podSpec.NodeSelector = map[string]string{"accelerator": "nvidia-tesla-p100"}
	for i := range podSpec.Containers {
		podSpec.Containers[i].Resources = core.ResourceRequirements{
			Limits: core.ResourceList{
				"nvidia.com/gpu": *resource.NewQuantity(3, resource.DecimalSI),
			},
			Requests: core.ResourceList{
				"nvidia.com/gpu": *resource.NewQuantity(3, resource.DecimalSI),
			},
		}
	}

	deploymentArg := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"app.juju.is/uuid":               "appuuid",
				"charm.juju.is/modified-version": "0",
			}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels:       map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"controller.juju.is/id":                    testing.ControllerTag.Id(),
						"charm.juju.is/modified-version":           "0",
					},
				},
				Spec: podSpec,
			},
		},
	}
	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, v1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicServiceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicServiceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockDeployments.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Create(gomock.Any(), deploymentArg, v1.CreateOptions{}).
			Return(deploymentArg, nil),
	)

	params := &caas.ServiceParams{
		PodSpec:      basicPodSpec,
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
		Devices: []devices.KubernetesDeviceParams{
			{
				Type:       "nvidia.com/gpu",
				Count:      3,
				Attributes: map[string]string{"gpu": "nvidia-tesla-p100"},
			},
		},
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
		},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceForDeploymentWithStorageCreate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	numUnits := int32(2)
	basicPodSpec := getBasicPodspec()
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec
	podSpec.Containers[0].VolumeMounts = append(dataVolumeMounts(), core.VolumeMount{
		Name:      "database-appuuid",
		MountPath: "path/to/here",
	}, core.VolumeMount{
		Name:      "logs-1",
		MountPath: "path/to/there",
	})

	pvc := &core.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{
			Name:   "database-appuuid",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "storage.juju.is/name": "database"},
			Annotations: map[string]string{
				"foo":                  "bar",
				"storage.juju.is/name": "database",
			},
		},
		Spec: core.PersistentVolumeClaimSpec{
			StorageClassName: pointer.StringPtr("workload-storage"),
			Resources: core.VolumeResourceRequirements{
				Requests: core.ResourceList{
					core.ResourceStorage: resource.MustParse("100Mi"),
				},
			},
			AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
		},
	}
	podSpec.Volumes = append(podSpec.Volumes, core.Volume{
		Name: "database-appuuid",
		VolumeSource: core.VolumeSource{
			PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
				ClaimName: pvc.GetName(),
			}},
	})

	size, err := resource.ParseQuantity("200Mi")
	c.Assert(err, jc.ErrorIsNil)
	podSpec.Volumes = append(podSpec.Volumes, core.Volume{
		Name: "logs-1",
		VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{
			SizeLimit: &size,
			Medium:    "Memory",
		}},
	})

	deploymentArg := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"app.juju.is/uuid":               "appuuid",
				"charm.juju.is/modified-version": "0",
			}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels:       map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"controller.juju.is/id":                    testing.ControllerTag.Id(),
						"charm.juju.is/modified-version":           "0",
					},
				},
				Spec: podSpec,
			},
		},
	}
	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, v1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicServiceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicServiceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockDeployments.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "test-workload-storage", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "workload-storage", v1.GetOptions{}).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "workload-storage"}}, nil),
		s.mockPersistentVolumeClaims.EXPECT().Create(gomock.Any(), pvc, v1.CreateOptions{}).
			Return(pvc, nil),
		s.mockDeployments.EXPECT().Create(gomock.Any(), deploymentArg, v1.CreateOptions{}).
			Return(deploymentArg, nil),
	)

	params := &caas.ServiceParams{
		Deployment: caas.DeploymentParams{
			DeploymentType: caas.DeploymentStateless,
		},
		PodSpec:      basicPodSpec,
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
		},
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
			Provider:    "kubernetes",
			Attributes:  map[string]interface{}{"storage-class": "workload-storage"},
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/here",
			},
			ResourceTags: map[string]string{"foo": "bar"},
		}, {
			StorageName: "logs",
			Size:        200,
			Provider:    "tmpfs",
			Attributes:  map[string]interface{}{"storage-medium": "Memory"},
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/there",
			},
		}},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceForDeploymentWithStorageUpdate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	numUnits := int32(2)
	basicPodSpec := getBasicPodspec()
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec
	podSpec.Containers[0].VolumeMounts = append(dataVolumeMounts(), core.VolumeMount{
		Name:      "database-appuuid",
		MountPath: "path/to/here",
	}, core.VolumeMount{
		Name:      "logs-1",
		MountPath: "path/to/there",
	})

	pvc := &core.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{
			Name:   "database-appuuid",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "storage.juju.is/name": "database"},
			Annotations: map[string]string{
				"foo":                  "bar",
				"storage.juju.is/name": "database",
			},
		},
		Spec: core.PersistentVolumeClaimSpec{
			StorageClassName: pointer.StringPtr("workload-storage"),
			Resources: core.VolumeResourceRequirements{
				Requests: core.ResourceList{
					core.ResourceStorage: resource.MustParse("100Mi"),
				},
			},
			AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
		},
	}
	podSpec.Volumes = append(podSpec.Volumes, core.Volume{
		Name: "database-appuuid",
		VolumeSource: core.VolumeSource{
			PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
				ClaimName: pvc.GetName(),
			}},
	})

	size, err := resource.ParseQuantity("200Mi")
	c.Assert(err, jc.ErrorIsNil)
	podSpec.Volumes = append(podSpec.Volumes, core.Volume{
		Name: "logs-1",
		VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{
			SizeLimit: &size,
			Medium:    "Memory",
		}},
	})

	deploymentArg := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"app.juju.is/uuid":               "appuuid",
				"charm.juju.is/modified-version": "0",
			}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels:       map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"controller.juju.is/id":                    testing.ControllerTag.Id(),
						"charm.juju.is/modified-version":           "0",
					},
				},
				Spec: podSpec,
			},
		},
	}
	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, v1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicServiceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicServiceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockDeployments.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(deploymentArg, nil),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "test-workload-storage", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "workload-storage", v1.GetOptions{}).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "workload-storage"}}, nil),
		s.mockPersistentVolumeClaims.EXPECT().Create(gomock.Any(), pvc, v1.CreateOptions{}).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockPersistentVolumeClaims.EXPECT().Get(gomock.Any(), "database-appuuid", v1.GetOptions{}).
			Return(pvc, nil),
		s.mockPersistentVolumeClaims.EXPECT().Update(gomock.Any(), pvc, v1.UpdateOptions{}).
			Return(pvc, nil),
		s.mockDeployments.EXPECT().Create(gomock.Any(), deploymentArg, v1.CreateOptions{}).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockDeployments.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(deploymentArg, nil),
		s.mockDeployments.EXPECT().Update(gomock.Any(), deploymentArg, v1.UpdateOptions{}).
			Return(deploymentArg, nil),
	)

	params := &caas.ServiceParams{
		Deployment: caas.DeploymentParams{
			DeploymentType: caas.DeploymentStateless,
		},
		PodSpec:      basicPodSpec,
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
		},
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
			Provider:    "kubernetes",
			Attributes:  map[string]interface{}{"storage-class": "workload-storage"},
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/here",
			},
			ResourceTags: map[string]string{"foo": "bar"},
		}, {
			StorageName: "logs",
			Size:        200,
			Provider:    "tmpfs",
			Attributes:  map[string]interface{}{"storage-medium": "Memory"},
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/there",
			},
		}},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceForDaemonSetWithStorageCreate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	basicPodSpec.ProviderPod = &k8sspecs.K8sPodSpec{
		KubernetesResources: &k8sspecs.KubernetesResources{
			Pod: &k8sspecs.PodSpec{
				Labels:      map[string]string{"foo": "bax"},
				Annotations: map[string]string{"foo": "baz"},
			},
		},
	}
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec
	podSpec.Affinity = &core.Affinity{
		NodeAffinity: &core.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &core.NodeSelector{
				NodeSelectorTerms: []core.NodeSelectorTerm{{
					MatchExpressions: []core.NodeSelectorRequirement{{
						Key:      "bar",
						Operator: core.NodeSelectorOpNotIn,
						Values:   []string{"d", "e", "f"},
					}, {
						Key:      "foo",
						Operator: core.NodeSelectorOpNotIn,
						Values:   []string{"g", "h"},
					}, {
						Key:      "foo",
						Operator: core.NodeSelectorOpIn,
						Values:   []string{"a", "b", "c"},
					}},
				}},
			},
		},
		PodAffinity: &core.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []core.PodAffinityTerm{{
				LabelSelector: &v1.LabelSelector{
					MatchExpressions: []v1.LabelSelectorRequirement{{
						Key:      "bar",
						Operator: v1.LabelSelectorOpNotIn,
						Values:   []string{"4", "5", "6"},
					}, {
						Key:      "foo",
						Operator: v1.LabelSelectorOpIn,
						Values:   []string{"1", "2", "3"},
					}},
				},
				TopologyKey: "some-key",
			}},
		},
		PodAntiAffinity: &core.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []core.PodAffinityTerm{{
				LabelSelector: &v1.LabelSelector{
					MatchExpressions: []v1.LabelSelectorRequirement{{
						Key:      "abar",
						Operator: v1.LabelSelectorOpNotIn,
						Values:   []string{"7", "8", "9"},
					}, {
						Key:      "afoo",
						Operator: v1.LabelSelectorOpIn,
						Values:   []string{"x", "y", "z"},
					}},
				},
				TopologyKey: "another-key",
			}},
		},
	}
	podSpec.Containers[0].VolumeMounts = append(dataVolumeMounts(), core.VolumeMount{
		Name:      "database-appuuid",
		MountPath: "path/to/here",
	}, core.VolumeMount{
		Name:      "logs-1",
		MountPath: "path/to/there",
	})

	pvc := &core.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{
			Name:   "database-appuuid",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "storage.juju.is/name": "database"},
			Annotations: map[string]string{
				"foo":                  "bar",
				"storage.juju.is/name": "database",
			},
		},
		Spec: core.PersistentVolumeClaimSpec{
			StorageClassName: pointer.StringPtr("workload-storage"),
			Resources: core.VolumeResourceRequirements{
				Requests: core.ResourceList{
					core.ResourceStorage: resource.MustParse("100Mi"),
				},
			},
			AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
		},
	}
	podSpec.Volumes = append(podSpec.Volumes, core.Volume{
		Name: "database-appuuid",
		VolumeSource: core.VolumeSource{
			PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
				ClaimName: pvc.GetName(),
			}},
	})

	size, err := resource.ParseQuantity("200Mi")
	c.Assert(err, jc.ErrorIsNil)
	podSpec.Volumes = append(podSpec.Volumes, core.Volume{
		Name: "logs-1",
		VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{
			SizeLimit: &size,
			Medium:    "Memory",
		}},
	})

	daemonSetArg := &appsv1.DaemonSet{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"app.juju.is/uuid":               "appuuid",
				"charm.juju.is/modified-version": "0",
			}},
		Spec: appsv1.DaemonSetSpec{
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels:       map[string]string{"app.kubernetes.io/name": "app-name", "foo": "bax"},
					Annotations: map[string]string{
						"foo": "baz",
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"controller.juju.is/id":                    testing.ControllerTag.Id(),
						"charm.juju.is/modified-version":           "0",
					},
				},
				Spec: podSpec,
			},
		},
	}

	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, v1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicServiceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicServiceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockDaemonSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "test-workload-storage", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "workload-storage", v1.GetOptions{}).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "workload-storage"}}, nil),
		s.mockPersistentVolumeClaims.EXPECT().Create(gomock.Any(), pvc, v1.CreateOptions{}).
			Return(pvc, nil),
		s.mockDaemonSets.EXPECT().Create(gomock.Any(), daemonSetArg, v1.CreateOptions{}).
			Return(daemonSetArg, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: basicPodSpec,
		Deployment: caas.DeploymentParams{
			DeploymentType: caas.DeploymentDaemon,
		},
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
		},
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
			Provider:    "kubernetes",
			Attributes:  map[string]interface{}{"storage-class": "workload-storage"},
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/here",
			},
			ResourceTags: map[string]string{"foo": "bar"},
		}, {
			StorageName: "logs",
			Size:        200,
			Provider:    "tmpfs",
			Attributes:  map[string]interface{}{"storage-medium": "Memory"},
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/there",
			},
		}},
		Constraints: constraints.MustParse(`tags=node.foo=a|b|c,^bar=d|e|f,^foo=g|h,pod.foo=1|2|3,^pod.bar=4|5|6,pod.topology-key=some-key,anti-pod.afoo=x|y|z,^anti-pod.abar=7|8|9,anti-pod.topology-key=another-key`),
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceForDaemonSetWithUpdateStrategy(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()

	basicPodSpec.Service = &specs.ServiceSpec{
		UpdateStrategy: &specs.UpdateStrategy{
			Type: "RollingUpdate",
			RollingUpdate: &specs.RollingUpdateSpec{
				MaxUnavailable: &specs.IntOrString{IntVal: 10},
			},
		},
	}

	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec
	podSpec.Affinity = &core.Affinity{
		NodeAffinity: &core.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &core.NodeSelector{
				NodeSelectorTerms: []core.NodeSelectorTerm{{
					MatchExpressions: []core.NodeSelectorRequirement{{
						Key:      "bar",
						Operator: core.NodeSelectorOpNotIn,
						Values:   []string{"d", "e", "f"},
					}, {
						Key:      "foo",
						Operator: core.NodeSelectorOpNotIn,
						Values:   []string{"g", "h"},
					}, {
						Key:      "foo",
						Operator: core.NodeSelectorOpIn,
						Values:   []string{"a", "b", "c"},
					}},
				}},
			},
		},
	}
	podSpec.Containers[0].VolumeMounts = append(dataVolumeMounts(), core.VolumeMount{
		Name:      "database-appuuid",
		MountPath: "path/to/here",
	}, core.VolumeMount{
		Name:      "logs-1",
		MountPath: "path/to/there",
	})

	pvc := &core.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{
			Name:   "database-appuuid",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "storage.juju.is/name": "database"},
			Annotations: map[string]string{
				"foo":                  "bar",
				"storage.juju.is/name": "database",
			},
		},
		Spec: core.PersistentVolumeClaimSpec{
			StorageClassName: pointer.StringPtr("workload-storage"),
			Resources: core.VolumeResourceRequirements{
				Requests: core.ResourceList{
					core.ResourceStorage: resource.MustParse("100Mi"),
				},
			},
			AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
		},
	}
	podSpec.Volumes = append(podSpec.Volumes, core.Volume{
		Name: "database-appuuid",
		VolumeSource: core.VolumeSource{
			PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
				ClaimName: pvc.GetName(),
			}},
	})

	size, err := resource.ParseQuantity("200Mi")
	c.Assert(err, jc.ErrorIsNil)
	podSpec.Volumes = append(podSpec.Volumes, core.Volume{
		Name: "logs-1",
		VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{
			SizeLimit: &size,
			Medium:    "Memory",
		}},
	})

	daemonSetArg := &appsv1.DaemonSet{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"app.juju.is/uuid":               "appuuid",
				"charm.juju.is/modified-version": "0",
			}},
		Spec: appsv1.DaemonSetSpec{
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels:       map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"controller.juju.is/id":                    testing.ControllerTag.Id(),
						"charm.juju.is/modified-version":           "0",
					},
				},
				Spec: podSpec,
			},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDaemonSet{
					MaxUnavailable: &intstr.IntOrString{IntVal: 10},
				},
			},
		},
	}

	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, v1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicServiceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicServiceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockDaemonSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "test-workload-storage", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "workload-storage", v1.GetOptions{}).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "workload-storage"}}, nil),
		s.mockPersistentVolumeClaims.EXPECT().Create(gomock.Any(), pvc, v1.CreateOptions{}).
			Return(pvc, nil),
		s.mockDaemonSets.EXPECT().Create(gomock.Any(), daemonSetArg, v1.CreateOptions{}).
			Return(daemonSetArg, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: basicPodSpec,
		Deployment: caas.DeploymentParams{
			DeploymentType: caas.DeploymentDaemon,
		},
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
		},
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
			Provider:    "kubernetes",
			Attributes:  map[string]interface{}{"storage-class": "workload-storage"},
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/here",
			},
			ResourceTags: map[string]string{"foo": "bar"},
		}, {
			StorageName: "logs",
			Size:        200,
			Provider:    "tmpfs",
			Attributes:  map[string]interface{}{"storage-medium": "Memory"},
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/there",
			},
		}},
		Constraints: constraints.MustParse(`tags=foo=a|b|c,^bar=d|e|f,^foo=g|h`),
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceForDaemonSetWithStorageUpdate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec
	podSpec.Affinity = &core.Affinity{
		NodeAffinity: &core.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &core.NodeSelector{
				NodeSelectorTerms: []core.NodeSelectorTerm{{
					MatchExpressions: []core.NodeSelectorRequirement{{
						Key:      "bar",
						Operator: core.NodeSelectorOpNotIn,
						Values:   []string{"d", "e", "f"},
					}, {
						Key:      "foo",
						Operator: core.NodeSelectorOpNotIn,
						Values:   []string{"g", "h"},
					}, {
						Key:      "foo",
						Operator: core.NodeSelectorOpIn,
						Values:   []string{"a", "b", "c"},
					}},
				}},
			},
		},
	}
	podSpec.Containers[0].VolumeMounts = append(dataVolumeMounts(), core.VolumeMount{
		Name:      "database-appuuid",
		MountPath: "path/to/here",
	}, core.VolumeMount{
		Name:      "logs-1",
		MountPath: "path/to/there",
	})

	pvc := &core.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{
			Name:   "database-appuuid",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "storage.juju.is/name": "database"},
			Annotations: map[string]string{
				"foo":                  "bar",
				"storage.juju.is/name": "database",
			},
		},
		Spec: core.PersistentVolumeClaimSpec{
			StorageClassName: pointer.StringPtr("workload-storage"),
			Resources: core.VolumeResourceRequirements{
				Requests: core.ResourceList{
					core.ResourceStorage: resource.MustParse("100Mi"),
				},
			},
			AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
		},
	}
	podSpec.Volumes = append(podSpec.Volumes, core.Volume{
		Name: "database-appuuid",
		VolumeSource: core.VolumeSource{
			PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
				ClaimName: pvc.GetName(),
			}},
	})

	size, err := resource.ParseQuantity("200Mi")
	c.Assert(err, jc.ErrorIsNil)
	podSpec.Volumes = append(podSpec.Volumes, core.Volume{
		Name: "logs-1",
		VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{
			SizeLimit: &size,
			Medium:    "Memory",
		}},
	})

	daemonSetArg := &appsv1.DaemonSet{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"app.juju.is/uuid":               "appuuid",
				"charm.juju.is/modified-version": "0",
			}},
		Spec: appsv1.DaemonSetSpec{
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels:       map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"controller.juju.is/id":                    testing.ControllerTag.Id(),
						"charm.juju.is/modified-version":           "0",
					},
				},
				Spec: podSpec,
			},
		},
	}

	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, v1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicServiceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicServiceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockDaemonSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "test-workload-storage", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "workload-storage", v1.GetOptions{}).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "workload-storage"}}, nil),
		s.mockPersistentVolumeClaims.EXPECT().Create(gomock.Any(), pvc, v1.CreateOptions{}).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockPersistentVolumeClaims.EXPECT().Get(gomock.Any(), "database-appuuid", v1.GetOptions{}).
			Return(pvc, nil),
		s.mockPersistentVolumeClaims.EXPECT().Update(gomock.Any(), pvc, v1.UpdateOptions{}).
			Return(pvc, nil),
		s.mockDaemonSets.EXPECT().Create(gomock.Any(), daemonSetArg, v1.CreateOptions{}).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockDaemonSets.EXPECT().List(gomock.Any(), v1.ListOptions{
			LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=app-name",
		}).Return(&appsv1.DaemonSetList{Items: []appsv1.DaemonSet{*daemonSetArg}}, nil),
		s.mockDaemonSets.EXPECT().Update(gomock.Any(), daemonSetArg, v1.UpdateOptions{}).
			Return(daemonSetArg, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: basicPodSpec,
		Deployment: caas.DeploymentParams{
			DeploymentType: caas.DeploymentDaemon,
		},
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
		},
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
			Provider:    "kubernetes",
			Attributes:  map[string]interface{}{"storage-class": "workload-storage"},
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/here",
			},
			ResourceTags: map[string]string{"foo": "bar"},
		}, {
			StorageName: "logs",
			Size:        200,
			Provider:    "tmpfs",
			Attributes:  map[string]interface{}{"storage-medium": "Memory"},
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/there",
			},
		}},
		Constraints: constraints.MustParse(`tags=foo=a|b|c,^bar=d|e|f,^foo=g|h`),
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceForDaemonSetWithDevicesAndConstraintsCreate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec
	podSpec.NodeSelector = map[string]string{"accelerator": "nvidia-tesla-p100"}
	for i := range podSpec.Containers {
		podSpec.Containers[i].Resources = core.ResourceRequirements{
			Limits: core.ResourceList{
				"nvidia.com/gpu": *resource.NewQuantity(3, resource.DecimalSI),
			},
			Requests: core.ResourceList{
				"nvidia.com/gpu": *resource.NewQuantity(3, resource.DecimalSI),
			},
		}
	}
	podSpec.Affinity = &core.Affinity{
		NodeAffinity: &core.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &core.NodeSelector{
				NodeSelectorTerms: []core.NodeSelectorTerm{{
					MatchExpressions: []core.NodeSelectorRequirement{{
						Key:      "bar",
						Operator: core.NodeSelectorOpNotIn,
						Values:   []string{"d", "e", "f"},
					}, {
						Key:      "foo",
						Operator: core.NodeSelectorOpNotIn,
						Values:   []string{"g", "h"},
					}, {
						Key:      "foo",
						Operator: core.NodeSelectorOpIn,
						Values:   []string{"a", "b", "c"},
					}},
				}},
			},
		},
	}

	daemonSetArg := &appsv1.DaemonSet{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"app.juju.is/uuid":               "appuuid",
				"charm.juju.is/modified-version": "0",
			}},
		Spec: appsv1.DaemonSetSpec{
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels:       map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"controller.juju.is/id":                    testing.ControllerTag.Id(),
						"charm.juju.is/modified-version":           "0",
					},
				},
				Spec: podSpec,
			},
		},
	}

	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, v1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicServiceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicServiceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockDaemonSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDaemonSets.EXPECT().Create(gomock.Any(), daemonSetArg, v1.CreateOptions{}).
			Return(daemonSetArg, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: basicPodSpec,
		Deployment: caas.DeploymentParams{
			DeploymentType: caas.DeploymentDaemon,
		},
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
		Devices: []devices.KubernetesDeviceParams{
			{
				Type:       "nvidia.com/gpu",
				Count:      3,
				Attributes: map[string]string{"gpu": "nvidia-tesla-p100"},
			},
		},
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
		},
		Constraints: constraints.MustParse(`tags=foo=a|b|c,^bar=d|e|f,^foo=g|h`),
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceForDaemonSetWithDevicesAndConstraintsUpdate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec
	podSpec.NodeSelector = map[string]string{"accelerator": "nvidia-tesla-p100"}
	for i := range podSpec.Containers {
		podSpec.Containers[i].Resources = core.ResourceRequirements{
			Limits: core.ResourceList{
				"nvidia.com/gpu": *resource.NewQuantity(3, resource.DecimalSI),
			},
			Requests: core.ResourceList{
				"nvidia.com/gpu": *resource.NewQuantity(3, resource.DecimalSI),
			},
		}
	}
	podSpec.Affinity = &core.Affinity{
		NodeAffinity: &core.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &core.NodeSelector{
				NodeSelectorTerms: []core.NodeSelectorTerm{{
					MatchExpressions: []core.NodeSelectorRequirement{{
						Key:      "bar",
						Operator: core.NodeSelectorOpNotIn,
						Values:   []string{"d", "e", "f"},
					}, {
						Key:      "foo",
						Operator: core.NodeSelectorOpNotIn,
						Values:   []string{"g", "h"},
					}, {
						Key:      "foo",
						Operator: core.NodeSelectorOpIn,
						Values:   []string{"a", "b", "c"},
					}},
				}},
			},
		},
	}

	daemonSetArg := &appsv1.DaemonSet{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"app.juju.is/uuid":               "appuuid",
				"charm.juju.is/modified-version": "0",
			}},
		Spec: appsv1.DaemonSetSpec{
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels:       map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"controller.juju.is/id":                    testing.ControllerTag.Id(),
						"charm.juju.is/modified-version":           "0",
					},
				},
				Spec: podSpec,
			},
		},
	}

	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, v1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicServiceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicServiceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockDaemonSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(daemonSetArg, nil),
		s.mockDaemonSets.EXPECT().Create(gomock.Any(), daemonSetArg, v1.CreateOptions{}).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockDaemonSets.EXPECT().List(gomock.Any(), v1.ListOptions{
			LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=app-name",
		}).Return(&appsv1.DaemonSetList{Items: []appsv1.DaemonSet{*daemonSetArg}}, nil),
		s.mockDaemonSets.EXPECT().Update(gomock.Any(), daemonSetArg, v1.UpdateOptions{}).
			Return(daemonSetArg, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: basicPodSpec,
		Deployment: caas.DeploymentParams{
			DeploymentType: caas.DeploymentDaemon,
		},
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
		Devices: []devices.KubernetesDeviceParams{
			{
				Type:       "nvidia.com/gpu",
				Count:      3,
				Attributes: map[string]string{"gpu": "nvidia-tesla-p100"},
			},
		},
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
		},
		Constraints: constraints.MustParse(`tags=foo=a|b|c,^bar=d|e|f,^foo=g|h`),
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceForStatefulSetWithDevices(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec
	podSpec.Containers[0].VolumeMounts = append(dataVolumeMounts(), core.VolumeMount{
		Name:      "database-appuuid",
		MountPath: "path/to/here",
	})
	podSpec.NodeSelector = map[string]string{"accelerator": "nvidia-tesla-p100"}
	for i := range podSpec.Containers {
		podSpec.Containers[i].Resources = core.ResourceRequirements{
			Limits: core.ResourceList{
				"nvidia.com/gpu": *resource.NewQuantity(3, resource.DecimalSI),
			},
			Requests: core.ResourceList{
				"nvidia.com/gpu": *resource.NewQuantity(3, resource.DecimalSI),
			},
		}
	}
	statefulSetArg := unitStatefulSetArg(2, "workload-storage", podSpec)
	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, v1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicServiceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicServiceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name-endpoints", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicHeadlessServiceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicHeadlessServiceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(&appsv1.StatefulSet{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{"app.juju.is/uuid": "appuuid"}}}, nil),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "test-workload-storage", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "workload-storage", v1.GetOptions{}).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "workload-storage"}}, nil),
		s.mockStatefulSets.EXPECT().Create(gomock.Any(), statefulSetArg, v1.CreateOptions{}).
			Return(statefulSetArg, nil),
	)

	params := &caas.ServiceParams{
		PodSpec:      basicPodSpec,
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
			Provider:    "kubernetes",
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/here",
			},
			Attributes:   map[string]interface{}{"storage-class": "workload-storage"},
			ResourceTags: map[string]string{"foo": "bar"},
		}},
		Devices: []devices.KubernetesDeviceParams{
			{
				Type:       "nvidia.com/gpu",
				Count:      3,
				Attributes: map[string]string{"gpu": "nvidia-tesla-p100"},
			},
		},
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
		},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceForStatefulSetUpdate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	basicPodSpec.Containers[0].VolumeConfig = []specs.FileSet{
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
	}
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec
	podSpec.Containers[0].VolumeMounts = append(dataVolumeMounts(), core.VolumeMount{
		Name:      "database-appuuid",
		MountPath: "path/to/here",
	})
	podSpec.NodeSelector = map[string]string{"accelerator": "nvidia-tesla-p100"}
	for i := range podSpec.Containers {
		podSpec.Containers[i].Resources = core.ResourceRequirements{
			Limits: core.ResourceList{
				"nvidia.com/gpu": *resource.NewQuantity(3, resource.DecimalSI),
			},
			Requests: core.ResourceList{
				"nvidia.com/gpu": *resource.NewQuantity(3, resource.DecimalSI),
			},
		}
	}
	statefulSetArg := unitStatefulSetArg(2, "workload-storage", podSpec)
	statefulSetArgUpdate := *statefulSetArg
	hostPathType := core.HostPathDirectory
	statefulSetArgUpdate.Spec.Template.Spec.Volumes = append(
		statefulSetArgUpdate.Spec.Template.Spec.Volumes,
		core.Volume{
			Name: "myhostpath",
			VolumeSource: core.VolumeSource{
				HostPath: &core.HostPathVolumeSource{
					Path: "/etc/cni/net.d",
					Type: &hostPathType,
				},
			},
		},
	)
	statefulSetArgUpdate.Spec.Template.Spec.Containers[0].VolumeMounts = []core.VolumeMount{
		{
			Name:      "juju-data-dir",
			MountPath: "/var/lib/juju",
		},
		{
			Name:      "juju-data-dir",
			MountPath: "/usr/bin/juju-exec",
			SubPath:   "tools/jujud",
		},
		{
			Name:      "myhostpath",
			MountPath: "/host/etc/cni/net.d",
		},
		{
			Name:      "database-appuuid",
			MountPath: "path/to/here",
		},
	}
	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, v1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicServiceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicServiceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name-endpoints", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicHeadlessServiceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicHeadlessServiceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(&appsv1.StatefulSet{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{"app.juju.is/uuid": "appuuid"}}}, nil),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "test-workload-storage", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "workload-storage", v1.GetOptions{}).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "workload-storage"}}, nil),
		s.mockStatefulSets.EXPECT().Create(gomock.Any(), &statefulSetArgUpdate, v1.CreateOptions{}).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), statefulSetArg.GetName(), v1.GetOptions{}).
			Return(statefulSetArg, nil),
		s.mockStatefulSets.EXPECT().Update(gomock.Any(), &statefulSetArgUpdate, v1.UpdateOptions{}).
			Return(&statefulSetArgUpdate, nil),
	)

	params := &caas.ServiceParams{
		PodSpec:      basicPodSpec,
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
			Provider:    "kubernetes",
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/here",
			},
			Attributes:   map[string]interface{}{"storage-class": "workload-storage"},
			ResourceTags: map[string]string{"foo": "bar"},
		}},
		Devices: []devices.KubernetesDeviceParams{
			{
				Type:       "nvidia.com/gpu",
				Count:      3,
				Attributes: map[string]string{"gpu": "nvidia-tesla-p100"},
			},
		},
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
		},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceWithConstraints(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec
	podSpec.Containers[0].VolumeMounts = append(dataVolumeMounts(), core.VolumeMount{
		Name:      "database-appuuid",
		MountPath: "path/to/here",
	})
	podSpec.NodeSelector = map[string]string{
		"kubernetes.io/arch": "amd64",
	}
	for i := range podSpec.Containers {
		podSpec.Containers[i].Resources = core.ResourceRequirements{
			Requests: core.ResourceList{
				"memory": resource.MustParse("64Mi"),
				"cpu":    resource.MustParse("500m"),
			},
		}
		break
	}
	statefulSetArg := unitStatefulSetArg(2, "workload-storage", podSpec)
	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, v1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicServiceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicServiceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name-endpoints", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicHeadlessServiceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicHeadlessServiceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(&appsv1.StatefulSet{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{"app.juju.is/uuid": "appuuid"}}}, nil),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "test-workload-storage", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "workload-storage", v1.GetOptions{}).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "workload-storage"}}, nil),
		s.mockStatefulSets.EXPECT().Create(gomock.Any(), statefulSetArg, v1.CreateOptions{}).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec:      basicPodSpec,
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
			Provider:    "kubernetes",
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/here",
			},
			Attributes:   map[string]interface{}{"storage-class": "workload-storage"},
			ResourceTags: map[string]string{"foo": "bar"},
		}},
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
		},
		Constraints: constraints.MustParse("mem=64 cpu-power=500 arch=amd64"),
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceWithNodeAffinity(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec
	podSpec.Containers[0].VolumeMounts = append(dataVolumeMounts(), core.VolumeMount{
		Name:      "database-appuuid",
		MountPath: "path/to/here",
	})
	podSpec.Affinity = &core.Affinity{
		NodeAffinity: &core.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &core.NodeSelector{
				NodeSelectorTerms: []core.NodeSelectorTerm{{
					MatchExpressions: []core.NodeSelectorRequirement{{
						Key:      "bar",
						Operator: core.NodeSelectorOpNotIn,
						Values:   []string{"d", "e", "f"},
					}, {
						Key:      "foo",
						Operator: core.NodeSelectorOpNotIn,
						Values:   []string{"g", "h"},
					}, {
						Key:      "foo",
						Operator: core.NodeSelectorOpIn,
						Values:   []string{"a", "b", "c"},
					}},
				}},
			},
		},
	}
	statefulSetArg := unitStatefulSetArg(2, "workload-storage", podSpec)
	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, v1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicServiceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicServiceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name-endpoints", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicHeadlessServiceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicHeadlessServiceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(&appsv1.StatefulSet{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{"app.juju.is/uuid": "appuuid"}}}, nil),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "test-workload-storage", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "workload-storage", v1.GetOptions{}).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "workload-storage"}}, nil),
		s.mockStatefulSets.EXPECT().Create(gomock.Any(), statefulSetArg, v1.CreateOptions{}).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec:      basicPodSpec,
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
			Provider:    "kubernetes",
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/here",
			},
			Attributes:   map[string]interface{}{"storage-class": "workload-storage"},
			ResourceTags: map[string]string{"foo": "bar"},
		}},
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
		},
		Constraints: constraints.MustParse(`tags=foo=a|b|c,^bar=d|e|f,^foo=g|h`),
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceWithZones(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec
	podSpec.Containers[0].VolumeMounts = append(dataVolumeMounts(), core.VolumeMount{
		Name:      "database-appuuid",
		MountPath: "path/to/here",
	})
	podSpec.Affinity = &core.Affinity{
		NodeAffinity: &core.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &core.NodeSelector{
				NodeSelectorTerms: []core.NodeSelectorTerm{{
					MatchExpressions: []core.NodeSelectorRequirement{{
						Key:      "failure-domain.beta.kubernetes.io/zone",
						Operator: core.NodeSelectorOpIn,
						Values:   []string{"a", "b", "c"},
					}},
				}},
			},
		},
	}
	statefulSetArg := unitStatefulSetArg(2, "workload-storage", podSpec)
	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, v1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicServiceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicServiceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name-endpoints", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicHeadlessServiceArg, v1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicHeadlessServiceArg, v1.CreateOptions{}).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(&appsv1.StatefulSet{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{"app.juju.is/uuid": "appuuid"}}}, nil),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "test-workload-storage", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "workload-storage", v1.GetOptions{}).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "workload-storage"}}, nil),
		s.mockStatefulSets.EXPECT().Create(gomock.Any(), statefulSetArg, v1.CreateOptions{}).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec:      basicPodSpec,
		ImageDetails: coreresources.DockerImageDetails{RegistryPath: "operator/image-path"},
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
			Provider:    "kubernetes",
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/here",
			},
			Attributes:   map[string]interface{}{"storage-class": "workload-storage"},
			ResourceTags: map[string]string{"foo": "bar"},
		}},
		ResourceTags: map[string]string{
			"juju-controller-uuid": testing.ControllerTag.Id(),
		},
		Constraints: constraints.MustParse(`zones=a,b,c`),
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, config.ConfigAttributes{
		"kubernetes-service-type":            "loadbalancer",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestUnits(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	podWithStorage := core.Pod{
		TypeMeta: v1.TypeMeta{},
		ObjectMeta: v1.ObjectMeta{
			Name:              "pod-name",
			UID:               types.UID("uuid"),
			DeletionTimestamp: &v1.Time{},
			OwnerReferences:   []v1.OwnerReference{{Kind: "StatefulSet"}},
		},
		Status: core.PodStatus{
			Message: "running",
			PodIP:   "10.0.0.1",
		},
		Spec: core.PodSpec{
			Containers: []core.Container{{
				Ports: []core.ContainerPort{{
					ContainerPort: 666,
					Protocol:      "TCP",
				}},
				VolumeMounts: []core.VolumeMount{{
					Name:      "v1",
					MountPath: "/path/to/here",
					ReadOnly:  true,
				}},
			}},
			Volumes: []core.Volume{{
				Name: "v1",
				VolumeSource: core.VolumeSource{
					PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
						ClaimName: "v1-claim",
					},
				},
			}},
		},
	}
	podList := &core.PodList{
		Items: []core.Pod{{
			TypeMeta: v1.TypeMeta{},
			ObjectMeta: v1.ObjectMeta{
				Name: "pod-name",
				UID:  types.UID("uuid"),
			},
			Status: core.PodStatus{
				Message: "running",
			},
			Spec: core.PodSpec{
				Containers: []core.Container{{}},
			},
		}, podWithStorage},
	}

	pvc := &core.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{
			UID:    "pvc-uuid",
			Labels: map[string]string{"juju-storage": "database"},
		},
		Spec: core.PersistentVolumeClaimSpec{VolumeName: "v1"},
		Status: core.PersistentVolumeClaimStatus{
			Conditions: []core.PersistentVolumeClaimCondition{{Message: "mounted"}},
			Phase:      core.ClaimBound,
		},
	}
	pv := &core.PersistentVolume{
		Spec: core.PersistentVolumeSpec{
			Capacity: core.ResourceList{
				"size": resource.MustParse("10Mi"),
			},
		},
		Status: core.PersistentVolumeStatus{
			Message: "vol-mounted",
			Phase:   core.VolumeBound,
		},
	}
	gomock.InOrder(
		s.mockPods.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: "app.kubernetes.io/name=app-name"}).Return(podList, nil),
		s.mockPersistentVolumeClaims.EXPECT().Get(gomock.Any(), "v1-claim", v1.GetOptions{}).
			Return(pvc, nil),
		s.mockPersistentVolumes.EXPECT().Get(gomock.Any(), "v1", v1.GetOptions{}).
			Return(pv, nil),
	)

	units, err := s.broker.Units("app-name", caas.ModeWorkload)
	c.Assert(err, jc.ErrorIsNil)
	now := s.clock.Now()
	c.Assert(units, jc.DeepEquals, []caas.Unit{{
		Id:       "uuid",
		Address:  "",
		Ports:    nil,
		Dying:    false,
		Stateful: false,
		Status: status.StatusInfo{
			Status:  "unknown",
			Message: "running",
			Since:   &now,
		},
		FilesystemInfo: nil,
	}, {
		Id:       "pod-name",
		Address:  "10.0.0.1",
		Ports:    []string{"666/TCP"},
		Dying:    true,
		Stateful: true,
		Status: status.StatusInfo{
			Status:  "terminated",
			Message: "running",
			Since:   &now,
		},
		FilesystemInfo: []caas.FilesystemInfo{{
			StorageName:  "database",
			FilesystemId: "pvc-uuid",
			Size:         uint64(podWithStorage.Spec.Volumes[0].PersistentVolumeClaim.Size()),
			MountPoint:   "/path/to/here",
			ReadOnly:     true,
			Status: status.StatusInfo{
				Status:  "attached",
				Message: "mounted",
				Since:   &now,
			},
			Volume: caas.VolumeInfo{
				Size: uint64(pv.Size()),
				Status: status.StatusInfo{
					Status:  "attached",
					Message: "vol-mounted",
					Since:   &now,
				},
				Persistent: true,
			},
		}},
	}})
}

func (s *K8sBrokerSuite) TestWatchServiceAggregate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ticklers := []func(){}

	s.k8sWatcherFn = func(_ cache.SharedIndexInformer, _ string, _ jujuclock.Clock) (k8swatcher.KubernetesNotifyWatcher, error) {
		w, f := k8swatchertest.NewKubernetesTestWatcher()
		ticklers = append(ticklers, f)
		return w, nil
	}

	w, err := s.broker.WatchService("test", caas.ModeWorkload)
	c.Assert(err, jc.ErrorIsNil)

	// Consume first dummy watcher event
	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for event")
	}

	// Poke each of the watcher channels to make sure they come through
	for _, tickler := range ticklers {
		tickler()
		select {
		case _, ok := <-w.Changes():
			c.Assert(ok, jc.IsTrue)
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for event")
		}
	}
}

func (s *K8sBrokerSuite) TestWatchService(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	s.k8sWatcherFn = func(_ cache.SharedIndexInformer, _ string, _ jujuclock.Clock) (k8swatcher.KubernetesNotifyWatcher, error) {
		w, _ := k8swatchertest.NewKubernetesTestWatcher()
		return w, nil
	}

	w, err := s.broker.WatchService("test", caas.ModeWorkload)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for event")
	}
}

func (s *K8sBrokerSuite) TestAnnotateUnit(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	pod := &core.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name: "pod-name",
		},
	}

	updatePod := &core.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:        "pod-name",
			Annotations: map[string]string{"unit.juju.is/id": "appname/0"},
		},
	}

	patch := []byte(`{"metadata":{"annotations":{"unit.juju.is/id":"appname/0"}}}`)

	gomock.InOrder(
		s.mockPods.EXPECT().Get(gomock.Any(), "pod-name", v1.GetOptions{}).Return(pod, nil),
		s.mockPods.EXPECT().Patch(gomock.Any(), "pod-name", types.MergePatchType, patch, v1.PatchOptions{}).Return(updatePod, nil),
	)

	err := s.broker.AnnotateUnit("appname", caas.ModeWorkload, "pod-name", names.NewUnitTag("appname/0"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestAnnotateUnitByUID(c *gc.C) {
	for _, mode := range []caas.DeploymentMode{caas.ModeOperator, caas.ModeWorkload} {
		s.assertAnnotateUnitByUID(c, mode)
	}
}

func (s *K8sBrokerSuite) assertAnnotateUnitByUID(c *gc.C, mode caas.DeploymentMode) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	podList := &core.PodList{
		Items: []core.Pod{{ObjectMeta: v1.ObjectMeta{
			Name: "pod-name",
			UID:  types.UID("uuid"),
		}}},
	}

	updatePod := &core.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:        "pod-name",
			UID:         types.UID("uuid"),
			Annotations: map[string]string{"unit.juju.is/id": "appname/0"},
		},
	}

	patch := []byte(`{"metadata":{"annotations":{"unit.juju.is/id":"appname/0"}}}`)

	labelSelector := "app.kubernetes.io/name=appname"
	if mode == caas.ModeOperator {
		labelSelector = "operator.juju.is/name=appname,operator.juju.is/target=application"
	}
	gomock.InOrder(
		s.mockPods.EXPECT().Get(gomock.Any(), "uuid", v1.GetOptions{}).Return(nil, s.k8sNotFoundError()),
		s.mockPods.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: labelSelector}).Return(podList, nil),
		s.mockPods.EXPECT().Patch(gomock.Any(), "pod-name", types.MergePatchType, patch, v1.PatchOptions{}).Return(updatePod, nil),
	)

	err := s.broker.AnnotateUnit("appname", mode, "uuid", names.NewUnitTag("appname/0"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestWatchUnits(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	podWatcher, podFirer := k8swatchertest.NewKubernetesTestWatcher()
	s.k8sWatcherFn = func(si cache.SharedIndexInformer, n string, _ jujuclock.Clock) (k8swatcher.KubernetesNotifyWatcher, error) {
		c.Assert(n, gc.Equals, "test")
		return podWatcher, nil
	}

	w, err := s.broker.WatchUnits("test", caas.ModeWorkload)
	c.Assert(err, jc.ErrorIsNil)

	podFirer()

	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for event")
	}
}

func (s *K8sBrokerSuite) TestWatchContainerStart(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	podWatcher, podFirer := k8swatchertest.NewKubernetesTestStringsWatcher()
	var filter k8swatcher.K8sStringsWatcherFilterFunc
	s.k8sStringsWatcherFn = func(_ cache.SharedIndexInformer,
		_ string,
		_ jujuclock.Clock,
		_ []string,
		ff k8swatcher.K8sStringsWatcherFilterFunc) (k8swatcher.KubernetesStringsWatcher, error) {
		filter = ff
		return podWatcher, nil
	}

	podList := &core.PodList{
		Items: []core.Pod{{
			ObjectMeta: v1.ObjectMeta{
				Name: "test-0",
				OwnerReferences: []v1.OwnerReference{
					{Kind: "StatefulSet"},
				},
				Annotations: map[string]string{
					"unit.juju.is/id": "test-0",
				},
			},
			Status: core.PodStatus{
				InitContainerStatuses: []core.ContainerStatus{
					{Name: "juju-pod-init", State: core.ContainerState{Waiting: &core.ContainerStateWaiting{}}},
				},
				Phase: core.PodPending,
			},
		}},
	}

	gomock.InOrder(
		s.mockPods.EXPECT().List(gomock.Any(),
			listOptionsLabelSelectorMatcher("app.kubernetes.io/name=test"),
		).DoAndReturn(func(stdcontext.Context, v1.ListOptions) (*core.PodList, error) {
			return podList, nil
		}),
	)

	w, err := s.broker.WatchContainerStart("test", caas.InitContainerName)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case v, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
		c.Assert(v, gc.HasLen, 0)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for event")
	}

	pod := &core.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name: "test-0",
			OwnerReferences: []v1.OwnerReference{
				{Kind: "StatefulSet"},
			},
			Annotations: map[string]string{
				"unit.juju.is/id": "test-0",
			},
		},
		Status: core.PodStatus{
			InitContainerStatuses: []core.ContainerStatus{
				{Name: "juju-pod-init", State: core.ContainerState{Running: &core.ContainerStateRunning{}}},
			},
			Phase: core.PodPending,
		},
	}

	evt, ok := filter(k8swatcher.WatchEventUpdate, pod)
	c.Assert(ok, jc.IsTrue)
	podFirer([]string{evt})

	select {
	case v, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
		c.Assert(v, gc.DeepEquals, []string{"test-0"})
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for event")
	}
}

func (s *K8sBrokerSuite) TestWatchContainerStartRegex(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	podWatcher, podFirer := k8swatchertest.NewKubernetesTestStringsWatcher()
	var filter k8swatcher.K8sStringsWatcherFilterFunc
	s.k8sStringsWatcherFn = func(_ cache.SharedIndexInformer,
		_ string,
		_ jujuclock.Clock,
		_ []string,
		ff k8swatcher.K8sStringsWatcherFilterFunc) (k8swatcher.KubernetesStringsWatcher, error) {
		filter = ff
		return podWatcher, nil
	}

	pod := core.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name: "test-0",
			OwnerReferences: []v1.OwnerReference{
				{Kind: "StatefulSet"},
			},
			Annotations: map[string]string{
				"unit.juju.is/id": "test-0",
			},
		},
		Status: core.PodStatus{
			ContainerStatuses: []core.ContainerStatus{
				{Name: "first-container", State: core.ContainerState{Waiting: &core.ContainerStateWaiting{}}},
				{Name: "second-container", State: core.ContainerState{Waiting: &core.ContainerStateWaiting{}}},
				{Name: "third-container", State: core.ContainerState{Waiting: &core.ContainerStateWaiting{}}},
			},
			Phase: core.PodPending,
		},
	}
	copyPod := func(pod core.Pod) *core.Pod {
		return &pod
	}

	podList := &core.PodList{
		Items: []core.Pod{pod},
	}

	gomock.InOrder(
		s.mockPods.EXPECT().List(gomock.Any(),
			listOptionsLabelSelectorMatcher("app.kubernetes.io/name=test"),
		).Return(podList, nil),
	)

	w, err := s.broker.WatchContainerStart("test", "(?:first|third)-container")
	c.Assert(err, jc.ErrorIsNil)

	// Send an event to one of the watchers; multi-watcher should fire.
	select {
	case v, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
		c.Assert(v, gc.HasLen, 0)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for event")
	}

	// test first-container fires
	pod.Status = core.PodStatus{
		ContainerStatuses: []core.ContainerStatus{
			{Name: "first-container", State: core.ContainerState{Running: &core.ContainerStateRunning{}}},
			{Name: "second-container", State: core.ContainerState{Waiting: &core.ContainerStateWaiting{}}},
			{Name: "third-container", State: core.ContainerState{Waiting: &core.ContainerStateWaiting{}}},
		},
		Phase: core.PodPending,
	}
	evt, ok := filter(k8swatcher.WatchEventUpdate, copyPod(pod))
	c.Assert(ok, jc.IsTrue)
	podFirer([]string{evt})

	select {
	case v, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
		c.Assert(v, gc.DeepEquals, []string{"test-0"})
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for event")
	}

	// test second-container does not fire
	pod.Status = core.PodStatus{
		ContainerStatuses: []core.ContainerStatus{
			{Name: "first-container", State: core.ContainerState{Running: &core.ContainerStateRunning{}}},
			{Name: "second-container", State: core.ContainerState{Running: &core.ContainerStateRunning{}}},
			{Name: "third-container", State: core.ContainerState{Waiting: &core.ContainerStateWaiting{}}},
		},
		Phase: core.PodPending,
	}
	_, ok = filter(k8swatcher.WatchEventUpdate, copyPod(pod))
	c.Assert(ok, jc.IsFalse)

	select {
	case <-w.Changes():
		c.Fatal("unexpected event")
	case <-time.After(testing.ShortWait):
	}

	// test third-container fires
	pod.Status = core.PodStatus{
		ContainerStatuses: []core.ContainerStatus{
			{Name: "first-container", State: core.ContainerState{Running: &core.ContainerStateRunning{}}},
			{Name: "second-container", State: core.ContainerState{Running: &core.ContainerStateRunning{}}},
			{Name: "third-container", State: core.ContainerState{Running: &core.ContainerStateRunning{}}},
		},
		Phase: core.PodPending,
	}
	evt, ok = filter(k8swatcher.WatchEventUpdate, copyPod(pod))
	c.Assert(ok, jc.IsTrue)
	podFirer([]string{evt})

	select {
	case v, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
		c.Assert(v, gc.DeepEquals, []string{"test-0"})
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for event")
	}
}

func (s *K8sBrokerSuite) TestWatchContainerStartDefault(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	podWatcher, podFirer := k8swatchertest.NewKubernetesTestStringsWatcher()
	var filter k8swatcher.K8sStringsWatcherFilterFunc
	s.k8sStringsWatcherFn = func(_ cache.SharedIndexInformer,
		_ string,
		_ jujuclock.Clock,
		_ []string,
		ff k8swatcher.K8sStringsWatcherFilterFunc) (k8swatcher.KubernetesStringsWatcher, error) {
		filter = ff
		return podWatcher, nil
	}

	podList := &core.PodList{
		Items: []core.Pod{{
			ObjectMeta: v1.ObjectMeta{
				Name: "test-0",
				OwnerReferences: []v1.OwnerReference{
					{Kind: "StatefulSet"},
				},
				Annotations: map[string]string{
					"unit.juju.is/id": "test-0",
				},
			},
			Status: core.PodStatus{
				ContainerStatuses: []core.ContainerStatus{
					{Name: "first-container", State: core.ContainerState{Waiting: &core.ContainerStateWaiting{}}},
					{Name: "second-container", State: core.ContainerState{Waiting: &core.ContainerStateWaiting{}}},
				},
				Phase: core.PodPending,
			},
		}},
	}

	gomock.InOrder(
		s.mockPods.EXPECT().List(gomock.Any(),
			listOptionsLabelSelectorMatcher("app.kubernetes.io/name=test"),
		).Return(podList, nil),
	)

	w, err := s.broker.WatchContainerStart("test", "")
	c.Assert(err, jc.ErrorIsNil)

	// Send an event to one of the watchers; multi-watcher should fire.
	pod := &core.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name: "test-0",
			OwnerReferences: []v1.OwnerReference{
				{Kind: "StatefulSet"},
			},
			Annotations: map[string]string{
				"unit.juju.is/id": "test-0",
			},
		},
		Status: core.PodStatus{
			ContainerStatuses: []core.ContainerStatus{
				{Name: "first-container", State: core.ContainerState{Running: &core.ContainerStateRunning{}}},
				{Name: "second-container", State: core.ContainerState{Waiting: &core.ContainerStateWaiting{}}},
			},
			Phase: core.PodPending,
		},
	}

	select {
	case v, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
		c.Assert(v, gc.HasLen, 0)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for event")
	}

	evt, ok := filter(k8swatcher.WatchEventUpdate, pod)
	c.Assert(ok, jc.IsTrue)
	podFirer([]string{evt})

	select {
	case v, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
		c.Assert(v, gc.DeepEquals, []string{"test-0"})
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for event")
	}
}

func (s *K8sBrokerSuite) TestWatchContainerStartDefaultWaitForUnit(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	podWatcher, podFirer := k8swatchertest.NewKubernetesTestStringsWatcher()
	var filter k8swatcher.K8sStringsWatcherFilterFunc
	s.k8sStringsWatcherFn = func(_ cache.SharedIndexInformer,
		_ string,
		_ jujuclock.Clock,
		_ []string,
		ff k8swatcher.K8sStringsWatcherFilterFunc) (k8swatcher.KubernetesStringsWatcher, error) {
		filter = ff
		return podWatcher, nil
	}

	podList := &core.PodList{
		Items: []core.Pod{{
			ObjectMeta: v1.ObjectMeta{
				Name: "test-0",
				OwnerReferences: []v1.OwnerReference{
					{Kind: "StatefulSet"},
				},
			},
			Status: core.PodStatus{
				ContainerStatuses: []core.ContainerStatus{
					{Name: "first-container", State: core.ContainerState{Running: &core.ContainerStateRunning{}}},
				},
				Phase: core.PodPending,
			},
		}},
	}

	gomock.InOrder(
		s.mockPods.EXPECT().List(gomock.Any(),
			listOptionsLabelSelectorMatcher("app.kubernetes.io/name=test"),
		).Return(podList, nil),
	)

	w, err := s.broker.WatchContainerStart("test", "")
	c.Assert(err, jc.ErrorIsNil)

	select {
	case v, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
		c.Assert(v, gc.HasLen, 0)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for event")
	}

	pod := &core.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name: "test-0",
			OwnerReferences: []v1.OwnerReference{
				{Kind: "StatefulSet"},
			},
			Annotations: map[string]string{
				"unit.juju.is/id": "test-0",
			},
		},
		Status: core.PodStatus{
			ContainerStatuses: []core.ContainerStatus{
				{Name: "first-container", State: core.ContainerState{Running: &core.ContainerStateRunning{}}},
			},
			Phase: core.PodPending,
		},
	}
	evt, ok := filter(k8swatcher.WatchEventUpdate, pod)
	c.Assert(ok, jc.IsTrue)
	podFirer([]string{evt})

	select {
	case v, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
		c.Assert(v, gc.DeepEquals, []string{"test-0"})
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for event")
	}
}

func (s *K8sBrokerSuite) TestUpdateStrategyForDaemonSet(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	_, err := provider.UpdateStrategyForDaemonSet(specs.UpdateStrategy{})
	c.Assert(err, gc.ErrorMatches, `strategy type "" for daemonset not valid`)

	o, err := provider.UpdateStrategyForDaemonSet(specs.UpdateStrategy{
		Type: "RollingUpdate",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(o, jc.DeepEquals, appsv1.DaemonSetUpdateStrategy{
		Type: appsv1.RollingUpdateDaemonSetStrategyType,
	})

	_, err = provider.UpdateStrategyForDaemonSet(specs.UpdateStrategy{
		Type:          "RollingUpdate",
		RollingUpdate: &specs.RollingUpdateSpec{},
	})
	c.Assert(err, gc.ErrorMatches, `rolling update spec maxUnavailable is missing`)

	_, err = provider.UpdateStrategyForDaemonSet(specs.UpdateStrategy{
		Type: "RollingUpdate",
		RollingUpdate: &specs.RollingUpdateSpec{
			Partition: pointer.Int32Ptr(10),
		},
	})
	c.Assert(err, gc.ErrorMatches, `rolling update spec for daemonset not valid`)

	_, err = provider.UpdateStrategyForDaemonSet(specs.UpdateStrategy{
		Type: "RollingUpdate",
		RollingUpdate: &specs.RollingUpdateSpec{
			MaxSurge: &specs.IntOrString{IntVal: 10},
		},
	})
	c.Assert(err, gc.ErrorMatches, `rolling update spec for daemonset not valid`)

	o, err = provider.UpdateStrategyForDaemonSet(specs.UpdateStrategy{
		Type: "RollingUpdate",
		RollingUpdate: &specs.RollingUpdateSpec{
			MaxUnavailable: &specs.IntOrString{IntVal: 10},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(o, jc.DeepEquals, appsv1.DaemonSetUpdateStrategy{
		Type: appsv1.RollingUpdateDaemonSetStrategyType,
		RollingUpdate: &appsv1.RollingUpdateDaemonSet{
			MaxUnavailable: &intstr.IntOrString{IntVal: 10},
		},
	})

	o, err = provider.UpdateStrategyForDaemonSet(specs.UpdateStrategy{
		Type: "OnDelete",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(o, jc.DeepEquals, appsv1.DaemonSetUpdateStrategy{
		Type: appsv1.OnDeleteDaemonSetStrategyType,
	})

	_, err = provider.UpdateStrategyForDaemonSet(specs.UpdateStrategy{
		Type: "OnDelete",
		RollingUpdate: &specs.RollingUpdateSpec{
			MaxUnavailable: &specs.IntOrString{IntVal: 10},
		},
	})
	c.Assert(err, gc.ErrorMatches, `rolling update spec is not supported for "OnDelete"`)
}

func (s *K8sBrokerSuite) TestUpdateStrategyForDeployment(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	_, err := provider.UpdateStrategyForDeployment(specs.UpdateStrategy{})
	c.Assert(err, gc.ErrorMatches, `strategy type "" for deployment not valid`)

	o, err := provider.UpdateStrategyForDeployment(specs.UpdateStrategy{
		Type: "RollingUpdate",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(o, jc.DeepEquals, appsv1.DeploymentStrategy{
		Type: appsv1.RollingUpdateDeploymentStrategyType,
	})

	_, err = provider.UpdateStrategyForDeployment(specs.UpdateStrategy{
		Type:          "RollingUpdate",
		RollingUpdate: &specs.RollingUpdateSpec{},
	})
	c.Assert(err, gc.ErrorMatches, `empty rolling update spec`)

	_, err = provider.UpdateStrategyForDeployment(specs.UpdateStrategy{
		Type: "RollingUpdate",
		RollingUpdate: &specs.RollingUpdateSpec{
			Partition:      pointer.Int32Ptr(10),
			MaxUnavailable: &specs.IntOrString{IntVal: 10},
		},
	})
	c.Assert(err, gc.ErrorMatches, `rolling update spec for deployment not valid`)

	o, err = provider.UpdateStrategyForDeployment(specs.UpdateStrategy{
		Type: "Recreate",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(o, jc.DeepEquals, appsv1.DeploymentStrategy{
		Type: appsv1.RecreateDeploymentStrategyType,
	})

	_, err = provider.UpdateStrategyForDeployment(specs.UpdateStrategy{
		Type: "Recreate",
		RollingUpdate: &specs.RollingUpdateSpec{
			MaxUnavailable: &specs.IntOrString{IntVal: 10},
			MaxSurge:       &specs.IntOrString{IntVal: 20},
		},
	})
	c.Assert(err, gc.ErrorMatches, `rolling update spec is not supported for "Recreate"`)

	o, err = provider.UpdateStrategyForDeployment(specs.UpdateStrategy{
		Type: "RollingUpdate",
		RollingUpdate: &specs.RollingUpdateSpec{
			MaxUnavailable: &specs.IntOrString{IntVal: 10},
			MaxSurge:       &specs.IntOrString{IntVal: 20},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(o, jc.DeepEquals, appsv1.DeploymentStrategy{
		Type: appsv1.RollingUpdateDeploymentStrategyType,
		RollingUpdate: &appsv1.RollingUpdateDeployment{
			MaxUnavailable: &intstr.IntOrString{IntVal: 10},
			MaxSurge:       &intstr.IntOrString{IntVal: 20},
		},
	})
}

func (s *K8sBrokerSuite) TestUpdateStrategyForStatefulSet(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	_, err := provider.UpdateStrategyForStatefulSet(specs.UpdateStrategy{})
	c.Assert(err, gc.ErrorMatches, `strategy type "" for statefulset not valid`)

	o, err := provider.UpdateStrategyForStatefulSet(specs.UpdateStrategy{
		Type: "RollingUpdate",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(o, jc.DeepEquals, appsv1.StatefulSetUpdateStrategy{
		Type: appsv1.RollingUpdateStatefulSetStrategyType,
	})

	_, err = provider.UpdateStrategyForStatefulSet(specs.UpdateStrategy{
		Type:          "RollingUpdate",
		RollingUpdate: &specs.RollingUpdateSpec{},
	})
	c.Assert(err, gc.ErrorMatches, `rolling update spec partition is missing`)

	_, err = provider.UpdateStrategyForStatefulSet(specs.UpdateStrategy{
		Type: "RollingUpdate",
		RollingUpdate: &specs.RollingUpdateSpec{
			Partition: pointer.Int32Ptr(10),
			MaxSurge:  &specs.IntOrString{IntVal: 10},
		},
	})
	c.Assert(err, gc.ErrorMatches, `rolling update spec for statefulset not valid`)

	_, err = provider.UpdateStrategyForStatefulSet(specs.UpdateStrategy{
		Type: "RollingUpdate",
		RollingUpdate: &specs.RollingUpdateSpec{
			Partition:      pointer.Int32Ptr(10),
			MaxUnavailable: &specs.IntOrString{IntVal: 10},
		},
	})
	c.Assert(err, gc.ErrorMatches, `rolling update spec for statefulset not valid`)

	o, err = provider.UpdateStrategyForStatefulSet(specs.UpdateStrategy{
		Type: "OnDelete",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(o, jc.DeepEquals, appsv1.StatefulSetUpdateStrategy{
		Type: appsv1.OnDeleteStatefulSetStrategyType,
	})

	_, err = provider.UpdateStrategyForStatefulSet(specs.UpdateStrategy{
		Type: "OnDelete",
		RollingUpdate: &specs.RollingUpdateSpec{
			Partition: pointer.Int32Ptr(10),
		},
	})
	c.Assert(err, gc.ErrorMatches, `rolling update spec is not supported for "OnDelete"`)

	o, err = provider.UpdateStrategyForStatefulSet(specs.UpdateStrategy{
		Type: "RollingUpdate",
		RollingUpdate: &specs.RollingUpdateSpec{
			Partition: pointer.Int32Ptr(10),
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(o, jc.DeepEquals, appsv1.StatefulSetUpdateStrategy{
		Type: appsv1.RollingUpdateStatefulSetStrategyType,
		RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
			Partition: pointer.Int32Ptr(10),
		},
	})
}

func (s *K8sBrokerSuite) TestExposeServiceIngressClassProvided(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	svc1 := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      "gitlab",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "gitlab"},
			Annotations: map[string]string{
				"controller.juju.is/id": testing.ControllerTag.Id(),
			}},
		Spec: core.ServiceSpec{
			Selector: k8sutils.LabelForKeyValue("app", "gitlab"),
			Type:     core.ServiceTypeClusterIP,
			Ports: []core.ServicePort{
				{
					Protocol:   core.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.IntOrString{IntVal: 9376},
				},
			},
		},
	}
	pathType := networkingv1.PathTypePrefix
	ingress := &networkingv1.Ingress{
		ObjectMeta: v1.ObjectMeta{
			Name:   "gitlab",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "gitlab"},
			Annotations: map[string]string{
				"ingress.kubernetes.io/rewrite-target":  "",
				"ingress.kubernetes.io/ssl-redirect":    "false",
				"kubernetes.io/ingress.allow-http":      "false",
				"ingress.kubernetes.io/ssl-passthrough": "false",
				"kubernetes.io/ingress.class":           "foo",
			},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				Host: "172.0.0.1.xip.io",
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path:     "/",
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: "gitlab",
									Port: networkingv1.ServiceBackendPort{
										Number: int32(9376),
									},
								},
							},
						}}},
				}}},
		},
	}

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-gitlab", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get(gomock.Any(), "gitlab", v1.GetOptions{}).
			Return(svc1, nil),
		s.mockIngressV1.EXPECT().Create(gomock.Any(), ingress, v1.CreateOptions{}).Return(nil, nil),
	)

	err := s.broker.ExposeService("gitlab", nil, config.ConfigAttributes{
		"kubernetes-ingress-class": "foo",
		"juju-external-hostname":   "172.0.0.1.xip.io",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestExposeServiceGetDefaultIngressClassFromResource(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	svc1 := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      "gitlab",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "gitlab"},
			Annotations: map[string]string{
				"controller.juju.is/id": testing.ControllerTag.Id(),
			}},
		Spec: core.ServiceSpec{
			Selector: k8sutils.LabelForKeyValue("app", "gitlab"),
			Type:     core.ServiceTypeClusterIP,
			Ports: []core.ServicePort{
				{
					Protocol:   core.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.IntOrString{IntVal: 9376},
				},
			},
		},
	}

	pathType := networkingv1.PathTypeImplementationSpecific
	ingress := &networkingv1.Ingress{
		ObjectMeta: v1.ObjectMeta{
			Name:   "gitlab",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "gitlab"},
			Annotations: map[string]string{
				"ingress.kubernetes.io/rewrite-target":  "",
				"ingress.kubernetes.io/ssl-redirect":    "false",
				"kubernetes.io/ingress.allow-http":      "false",
				"ingress.kubernetes.io/ssl-passthrough": "false",
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: pointer.StringPtr("foo"),
			Rules: []networkingv1.IngressRule{{
				Host: "172.0.0.1.xip.io",
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path:     "/",
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: "gitlab",
									Port: networkingv1.ServiceBackendPort{
										Number: int32(9376),
									},
								},
							},
						}}},
				}}},
		},
	}

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-gitlab", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get(gomock.Any(), "gitlab", v1.GetOptions{}).
			Return(svc1, nil),
		s.mockIngressClasses.EXPECT().List(gomock.Any(), v1.ListOptions{}).
			Return(&networkingv1.IngressClassList{Items: []networkingv1.IngressClass{
				{
					ObjectMeta: v1.ObjectMeta{
						Name: "foo",
						Annotations: map[string]string{
							"ingressclass.kubernetes.io/is-default-class": "true",
						},
					},
				},
			}}, nil),
		s.mockIngressV1.EXPECT().Create(gomock.Any(), ingress, v1.CreateOptions{}).Return(nil, nil),
	)

	err := s.broker.ExposeService("gitlab", nil, config.ConfigAttributes{
		"juju-external-hostname": "172.0.0.1.xip.io",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestExposeServiceGetDefaultIngressClass(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	svc1 := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      "gitlab",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "gitlab"},
			Annotations: map[string]string{
				"controller.juju.is/id": testing.ControllerTag.Id(),
			}},
		Spec: core.ServiceSpec{
			Selector: k8sutils.LabelForKeyValue("app", "gitlab"),
			Type:     core.ServiceTypeClusterIP,
			Ports: []core.ServicePort{
				{
					Protocol:   core.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.IntOrString{IntVal: 9376},
				},
			},
		},
	}

	pathType := networkingv1.PathTypePrefix
	ingress := &networkingv1.Ingress{
		ObjectMeta: v1.ObjectMeta{
			Name:   "gitlab",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "gitlab"},
			Annotations: map[string]string{
				"ingress.kubernetes.io/rewrite-target":  "",
				"ingress.kubernetes.io/ssl-redirect":    "false",
				"kubernetes.io/ingress.allow-http":      "false",
				"ingress.kubernetes.io/ssl-passthrough": "false",
				"kubernetes.io/ingress.class":           "nginx",
			},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				Host: "172.0.0.1.xip.io",
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path:     "/",
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: "gitlab",
									Port: networkingv1.ServiceBackendPort{
										Number: int32(9376),
									},
								},
							},
						}}},
				}}},
		},
	}
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-gitlab", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get(gomock.Any(), "gitlab", v1.GetOptions{}).
			Return(svc1, nil),
		s.mockIngressClasses.EXPECT().List(gomock.Any(), v1.ListOptions{}).
			Return(&networkingv1.IngressClassList{Items: []networkingv1.IngressClass{}}, nil),
		s.mockIngressV1.EXPECT().Create(gomock.Any(), ingress, v1.CreateOptions{}).Return(nil, nil),
	)

	err := s.broker.ExposeService("gitlab", nil, config.ConfigAttributes{
		"juju-external-hostname": "172.0.0.1.xip.io",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func initContainers() []core.Container {
	jujudCmd := `
export JUJU_DATA_DIR=/var/lib/juju
export JUJU_TOOLS_DIR=$JUJU_DATA_DIR/tools

mkdir -p $JUJU_TOOLS_DIR
cp /opt/jujud $JUJU_TOOLS_DIR/jujud
`[1:]
	jujudCmd += `
initCmd=$($JUJU_TOOLS_DIR/jujud help commands | grep caas-unit-init)
if test -n "$initCmd"; then
exec $JUJU_TOOLS_DIR/jujud caas-unit-init --debug --wait;
else
exit 0
fi
`
	return []core.Container{{
		Name:            "juju-pod-init",
		Image:           "operator/image-path",
		Command:         []string{"/bin/sh"},
		Args:            []string{"-c", jujudCmd},
		WorkingDir:      "/var/lib/juju",
		VolumeMounts:    []core.VolumeMount{{Name: "juju-data-dir", MountPath: "/var/lib/juju"}},
		ImagePullPolicy: "IfNotPresent",
	}}
}

func dataVolumeMounts() []core.VolumeMount {
	return []core.VolumeMount{
		{
			Name:      "juju-data-dir",
			MountPath: "/var/lib/juju",
		},
		{
			Name:      "juju-data-dir",
			MountPath: "/usr/bin/juju-exec",
			SubPath:   "tools/jujud",
		},
	}
}

func dataVolumes() []core.Volume {
	return []core.Volume{
		{
			Name: "juju-data-dir",
			VolumeSource: core.VolumeSource{
				EmptyDir: &core.EmptyDirVolumeSource{},
			},
		},
	}
}
