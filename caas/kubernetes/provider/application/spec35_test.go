// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/paths"
)

func getPodSpec35() corev1.PodSpec {
	jujuDataDir := paths.DataDir(paths.OSUnixLike)
	return corev1.PodSpec{
		ServiceAccountName:            "gitlab",
		AutomountServiceAccountToken:  pointer.BoolPtr(true),
		ImagePullSecrets:              []corev1.LocalObjectReference{{Name: "gitlab-nginx-secret"}},
		TerminationGracePeriodSeconds: pointer.Int64Ptr(30),
		SecurityContext: &corev1.PodSecurityContext{
			FSGroup:            int64Ptr(170),
			SupplementalGroups: []int64{170},
		},
		InitContainers: []corev1.Container{{
			Name:            "charm-init",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Image:           "operator/image-path:1.1.1",
			WorkingDir:      jujuDataDir,
			SecurityContext: &corev1.SecurityContext{
				RunAsUser:  int64Ptr(170),
				RunAsGroup: int64Ptr(170),
			},
			Command: []string{"/opt/containeragent"},
			Args: []string{
				"init",
				"--containeragent-pebble-dir", "/containeragent/pebble",
				"--charm-modified-version", "9001",
				"--data-dir", "/var/lib/juju",
				"--bin-dir", "/charm/bin",
				"--profile-dir", "/containeragent/etc/profile.d",
			},
			Env: []corev1.EnvVar{
				{
					Name:  "JUJU_CONTAINER_NAMES",
					Value: "gitlab,nginx",
				},
				{
					Name: "JUJU_K8S_POD_NAME",
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: "metadata.name",
						},
					},
				},
				{
					Name: "JUJU_K8S_POD_UUID",
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: "metadata.uid",
						},
					},
				},
			},
			EnvFrom: []corev1.EnvFromSource{
				{
					SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "gitlab-application-config",
						},
					},
				},
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "charm-data",
					MountPath: jujuDataDir,
					SubPath:   strings.TrimPrefix(jujuDataDir, "/"),
				},
				{
					Name:      "charm-data",
					MountPath: "/charm/bin",
					SubPath:   "charm/bin",
				},
				{
					Name:      "charm-data",
					MountPath: "/charm/containers",
					SubPath:   "charm/containers",
				},
				{
					Name:      "charm-data",
					MountPath: "/containeragent/pebble",
					SubPath:   "containeragent/pebble",
				},
				{
					Name:      "charm-data",
					MountPath: "/containeragent/etc/profile.d",
					SubPath:   "containeragent/etc/profile.d",
				},
			},
		}},
		Containers: []corev1.Container{{
			Name:            "charm",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Image:           "ubuntu@22.04",
			WorkingDir:      jujuDataDir,
			Command:         []string{"/charm/bin/pebble"},
			Args: []string{
				"run",
				"--http", ":38812",
				"--verbose",
			},
			Env: []corev1.EnvVar{
				{
					Name:  "JUJU_CONTAINER_NAMES",
					Value: "gitlab,nginx",
				},
				{
					Name:  constants.EnvAgentHTTPProbePort,
					Value: constants.AgentHTTPProbePort,
				},
			},
			LivenessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/v1/health?level=alive",
						Port: intstr.Parse("38812"),
					},
				},
				InitialDelaySeconds: 30,
				TimeoutSeconds:      1,
				PeriodSeconds:       5,
				SuccessThreshold:    1,
				FailureThreshold:    3,
			},
			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/v1/health?level=ready",
						Port: intstr.Parse("38812"),
					},
				},
				InitialDelaySeconds: 30,
				TimeoutSeconds:      1,
				PeriodSeconds:       5,
				SuccessThreshold:    1,
				FailureThreshold:    1,
			},
			StartupProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/v1/health?level=alive",
						Port: intstr.Parse("38812"),
					},
				},
				InitialDelaySeconds: 30,
				TimeoutSeconds:      1,
				PeriodSeconds:       5,
				SuccessThreshold:    1,
				FailureThreshold:    1,
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "charm-data",
					MountPath: "/charm/bin",
					SubPath:   "charm/bin",
					ReadOnly:  true,
				},
				{
					Name:      "charm-data",
					MountPath: jujuDataDir,
					SubPath:   strings.TrimPrefix(jujuDataDir, "/"),
				},
				{
					Name:      "charm-data",
					MountPath: "/charm/containers",
					SubPath:   "charm/containers",
				},
				{
					Name:      "charm-data",
					MountPath: "/var/lib/pebble/default",
					SubPath:   "containeragent/pebble",
				},
				{
					Name:      "charm-data",
					MountPath: "/var/log/juju",
					SubPath:   "containeragent/var/log/juju",
				},
				{
					Name:      "charm-data",
					MountPath: paths.JujuIntrospect(paths.OSUnixLike),
					SubPath:   "charm/bin/containeragent",
					ReadOnly:  true,
				},
				{
					Name:      "charm-data",
					MountPath: paths.JujuExec(paths.OSUnixLike),
					SubPath:   "charm/bin/containeragent",
					ReadOnly:  true,
				},
				{
					Name:      "charm-data",
					MountPath: "/etc/profile.d/juju-introspection.sh",
					SubPath:   "containeragent/etc/profile.d/juju-introspection.sh",
					ReadOnly:  true,
				},
				{
					Name:      "gitlab-database-appuuid",
					MountPath: "path/to/here",
				},
			},
			SecurityContext: &corev1.SecurityContext{
				RunAsUser:  int64Ptr(170),
				RunAsGroup: int64Ptr(170),
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: k8sresource.MustParse(fmt.Sprintf("%dMi", caas.CharmMemRequestMiB))},
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: k8sresource.MustParse(fmt.Sprintf("%dMi", caas.CharmMemLimitMiB))},
			},
		}, {
			Name:            "gitlab",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Image:           "docker.io/library/gitlab:latest",
			Command:         []string{"/charm/bin/pebble"},
			Args:            []string{"run", "--create-dirs", "--hold", "--http", ":38813", "--verbose"},
			Env: []corev1.EnvVar{
				{
					Name:  "JUJU_CONTAINER_NAME",
					Value: "gitlab",
				},
				{
					Name:  "PEBBLE_SOCKET",
					Value: "/charm/container/pebble.socket",
				},
				{
					Name:  "PEBBLE",
					Value: "/charm/container/pebble",
				},
				{
					Name:  "PEBBLE_COPY_ONCE",
					Value: "/var/lib/pebble/default",
				},
			},
			LivenessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/v1/health?level=alive",
						Port: intstr.FromInt(38813),
					},
				},
				InitialDelaySeconds: 30,
				TimeoutSeconds:      1,
				PeriodSeconds:       5,
				SuccessThreshold:    1,
				FailureThreshold:    3,
			},
			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/v1/health?level=ready",
						Port: intstr.FromInt(38813),
					},
				},
				InitialDelaySeconds: 30,
				TimeoutSeconds:      1,
				PeriodSeconds:       5,
				SuccessThreshold:    1,
				FailureThreshold:    1,
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "charm-data",
					MountPath: "/charm/bin/pebble",
					SubPath:   "charm/bin/pebble",
					ReadOnly:  true,
				},
				{
					Name:      "charm-data",
					MountPath: "/charm/container",
					SubPath:   "charm/containers/gitlab",
				},
				{
					Name:      "gitlab-database-appuuid",
					MountPath: "path/to/here",
				},
			},
			SecurityContext: &corev1.SecurityContext{},
		}, {
			Name:            "nginx",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Image:           "docker.io/library/nginx:latest",
			Command:         []string{"/charm/bin/pebble"},
			Args:            []string{"run", "--create-dirs", "--hold", "--http", ":38814", "--verbose"},
			Env: []corev1.EnvVar{
				{
					Name:  "JUJU_CONTAINER_NAME",
					Value: "nginx",
				},
				{
					Name:  "PEBBLE_SOCKET",
					Value: "/charm/container/pebble.socket",
				},
				{
					Name:  "PEBBLE",
					Value: "/charm/container/pebble",
				},
				{
					Name:  "PEBBLE_COPY_ONCE",
					Value: "/var/lib/pebble/default",
				},
			},
			LivenessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/v1/health?level=alive",
						Port: intstr.FromInt(38814),
					},
				},
				InitialDelaySeconds: 30,
				TimeoutSeconds:      1,
				PeriodSeconds:       5,
				SuccessThreshold:    1,
				FailureThreshold:    3,
			},
			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/v1/health?level=ready",
						Port: intstr.FromInt(38814),
					},
				},
				InitialDelaySeconds: 30,
				TimeoutSeconds:      1,
				PeriodSeconds:       5,
				SuccessThreshold:    1,
				FailureThreshold:    1,
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "charm-data",
					MountPath: "/charm/bin/pebble",
					SubPath:   "charm/bin/pebble",
					ReadOnly:  true,
				},
				{
					Name:      "charm-data",
					MountPath: "/charm/container",
					SubPath:   "charm/containers/nginx",
				},
			},
			SecurityContext: &corev1.SecurityContext{
				RunAsUser:  int64Ptr(1234),
				RunAsGroup: int64Ptr(4321),
			},
		}},
		Volumes: []corev1.Volume{
			{
				Name: "charm-data",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		},
	}
}
