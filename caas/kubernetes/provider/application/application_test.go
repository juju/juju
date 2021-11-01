// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golang/mock/gomock"
	jujuclock "github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/pointer"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/application"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/resources"
	resourcesmocks "github.com/juju/juju/caas/kubernetes/provider/resources/mocks"
	k8sutils "github.com/juju/juju/caas/kubernetes/provider/utils"
	k8swatcher "github.com/juju/juju/caas/kubernetes/provider/watcher"
	k8swatchertest "github.com/juju/juju/caas/kubernetes/provider/watcher/test"
	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/paths"
	coreresources "github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type applicationSuite struct {
	testing.BaseSuite
	client *fake.Clientset

	namespace    string
	appName      string
	clock        *testclock.Clock
	k8sWatcherFn k8swatcher.NewK8sWatcherFunc
	watchers     []k8swatcher.KubernetesNotifyWatcher
	applier      *resourcesmocks.MockApplier
}

var _ = gc.Suite(&applicationSuite{})

func (s *applicationSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.namespace = "test"
	s.appName = "gitlab"
	s.client = fake.NewSimpleClientset()
	s.clock = testclock.NewClock(time.Time{})
}

func (s *applicationSuite) TearDownTest(c *gc.C) {
	s.client = nil
	s.clock = nil
	s.watchers = nil
	s.applier = nil

	s.BaseSuite.TearDownTest(c)
}

func (s *applicationSuite) getApp(c *gc.C, deploymentType caas.DeploymentType, mockApplier bool) (application.ApplicationInterfaceForTest, *gomock.Controller) {
	watcherFn := k8swatcher.NewK8sWatcherFunc(func(i cache.SharedIndexInformer, n string, c jujuclock.Clock) (k8swatcher.KubernetesNotifyWatcher, error) {
		if s.k8sWatcherFn == nil {
			return nil, errors.NewNotFound(nil, "undefined k8sWatcherFn for base test")
		}

		w, err := s.k8sWatcherFn(i, n, c)
		if err == nil {
			s.watchers = append(s.watchers, w)
		}
		return w, err
	})

	ctrl := gomock.NewController(c)
	s.applier = resourcesmocks.NewMockApplier(ctrl)

	return application.NewApplicationForTest(
		s.appName, s.namespace, "deadbeef", s.namespace, false,
		deploymentType,
		s.client,
		watcherFn,
		s.clock,
		func() (string, error) {
			return "appuuid", nil
		},
		func() resources.Applier {
			if mockApplier {
				return s.applier
			}
			return resources.NewApplier()
		},
	), ctrl
}

func (s *applicationSuite) assertEnsure(c *gc.C, app caas.Application, isPrivateImageRepo bool, cons constraints.Value, trust bool, checkMainResource func()) {
	appSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gitlab-application-config",
			Namespace: "test",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "gitlab",
				"app.kubernetes.io/managed-by": "juju",
			},
			Annotations: map[string]string{"juju.is/version": "1.1.1"},
		},
		Data: map[string][]byte{
			"JUJU_K8S_APPLICATION":          []byte("gitlab"),
			"JUJU_K8S_MODEL":                []byte("deadbeef"),
			"JUJU_K8S_APPLICATION_PASSWORD": []byte(""),
			"JUJU_K8S_CONTROLLER_ADDRESSES": []byte(""),
			"JUJU_K8S_CONTROLLER_CA_CERT":   []byte(""),
		},
	}
	appSvc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gitlab",
			Namespace: "test",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "gitlab",
				"app.kubernetes.io/managed-by": "juju",
			},
			Annotations: map[string]string{"juju.is/version": "1.1.1"},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app.kubernetes.io/name": "gitlab"},
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{{
				Name: "placeholder",
				Port: 65535,
			}},
		},
	}
	pullSecretConfig, _ := k8sutils.CreateDockerConfigJSON("username", "password", "nginx-image:latest")
	nginxPullSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gitlab-nginx-secret",
			Namespace: "test",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "gitlab",
				"app.kubernetes.io/managed-by": "juju",
			},
			Annotations: map[string]string{"juju.is/version": "1.1.1"},
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: pullSecretConfig,
		},
	}
	appSA := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gitlab",
			Namespace: "test",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "gitlab",
				"app.kubernetes.io/managed-by": "juju",
			},
			Annotations: map[string]string{"juju.is/version": "1.1.1"},
		},
		AutomountServiceAccountToken: pointer.BoolPtr(false),
	}
	appRole := rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gitlab",
			Namespace: "test",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "gitlab",
				"app.kubernetes.io/managed-by": "juju",
			},
			Annotations: map[string]string{"juju.is/version": "1.1.1"},
		},
	}
	if trust {
		appRole.Rules = []rbacv1.PolicyRule{{
			Verbs:     []string{"*"},
			APIGroups: []string{"*"},
			Resources: []string{"*"},
		}}
	} else {
		appRole.Rules = []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods", "services"},
				Verbs: []string{
					"get",
					"list",
					"patch",
				},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods/exec"},
				Verbs: []string{
					"create",
				},
			},
		}
	}
	appRoleBinding := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gitlab",
			Namespace: "test",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "gitlab",
				"app.kubernetes.io/managed-by": "juju",
			},
			Annotations: map[string]string{"juju.is/version": "1.1.1"},
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      "gitlab",
			Namespace: "test",
		}},
		RoleRef: rbacv1.RoleRef{
			Kind: "Role",
			Name: "gitlab",
		},
	}
	appClusterRole := rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-gitlab",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "gitlab",
				"app.kubernetes.io/managed-by": "juju",
			},
			Annotations: map[string]string{"juju.is/version": "1.1.1"},
		},
	}
	if trust {
		appClusterRole.Rules = []rbacv1.PolicyRule{{
			Verbs:     []string{"*"},
			APIGroups: []string{"*"},
			Resources: []string{"*"},
		}}
	}
	appClusterRoleBinding := rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-gitlab",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "gitlab",
				"app.kubernetes.io/managed-by": "juju",
			},
			Annotations: map[string]string{"juju.is/version": "1.1.1"},
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      "gitlab",
			Namespace: "test",
		}},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: "test-gitlab",
		},
	}

	c.Assert(app.Ensure(
		caas.ApplicationConfig{
			AgentVersion:         version.MustParse("1.1.1"),
			IsPrivateImageRepo:   isPrivateImageRepo,
			AgentImagePath:       "operator/image-path:1.1.1",
			CharmBaseImagePath:   "ubuntu:20.04",
			CharmModifiedVersion: 9001,
			Filesystems: []storage.KubernetesFilesystemParams{
				{
					StorageName: "database",
					Size:        100,
					Provider:    "kubernetes",
					Attributes:  map[string]interface{}{"storage-class": "workload-storage"},
					Attachment: &storage.KubernetesFilesystemAttachmentParams{
						Path: "path/to/here",
					},
					ResourceTags: map[string]string{"foo": "bar"},
				},
				// TODO(sidecar): fix here - all filesystems will not be mounted if it's not in `Containers[*].Mounts`
				// {
				// 	StorageName: "logs",
				// 	Size:        200,
				// 	Provider:    "tmpfs",
				// 	Attributes:  map[string]interface{}{"storage-medium": "Memory"},
				// 	Attachment: &storage.KubernetesFilesystemAttachmentParams{
				// 		Path: "path/to/there",
				// 	},
				// },
			},
			Containers: map[string]caas.ContainerConfig{
				"gitlab": {
					Name: "gitlab",
					Image: coreresources.DockerImageDetails{
						RegistryPath: "gitlab-image:latest",
					},
					Mounts: []caas.MountConfig{
						{
							StorageName: "database",
							Path:        "path/to/here",
						},
					},
				},
				"nginx": {
					Name: "nginx",
					Image: coreresources.DockerImageDetails{
						RegistryPath: "nginx-image:latest",
						ImageRepoDetails: docker.ImageRepoDetails{
							BasicAuthConfig: docker.BasicAuthConfig{
								Username: "username",
								Password: "password",
							},
						},
					},
				},
			},
			Constraints:  cons,
			InitialScale: 3,
			Trust:        trust,
		},
	), jc.ErrorIsNil)

	secret, err := s.client.CoreV1().Secrets("test").Get(context.TODO(), "gitlab-application-config", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(secret, gc.DeepEquals, &appSecret)

	secret, err = s.client.CoreV1().Secrets("test").Get(context.TODO(), "gitlab-nginx-secret", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(secret, gc.DeepEquals, &nginxPullSecret)

	svc, err := s.client.CoreV1().Services("test").Get(context.TODO(), "gitlab", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(svc, gc.DeepEquals, &appSvc)

	sa, err := s.client.CoreV1().ServiceAccounts(s.namespace).Get(context.TODO(), "gitlab", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sa, gc.DeepEquals, &appSA)

	r, err := s.client.RbacV1().Roles(s.namespace).Get(context.TODO(), "gitlab", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r, gc.DeepEquals, &appRole)

	cr, err := s.client.RbacV1().ClusterRoles().Get(context.TODO(), "test-gitlab", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cr, gc.DeepEquals, &appClusterRole)

	rb, err := s.client.RbacV1().RoleBindings(s.namespace).Get(context.TODO(), "gitlab", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rb, gc.DeepEquals, &appRoleBinding)

	crb, err := s.client.RbacV1().ClusterRoleBindings().Get(context.TODO(), "test-gitlab", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(crb, gc.DeepEquals, &appClusterRoleBinding)

	checkMainResource()
}

func (s *applicationSuite) assertDelete(c *gc.C, app caas.Application) {
	err := app.Delete()
	c.Assert(err, jc.ErrorIsNil)

	clusterRoles, err := s.client.RbacV1().ClusterRoles().List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clusterRoles.Items, gc.IsNil)

	clusterRoleBinding, err := s.client.RbacV1().ClusterRoleBindings().List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clusterRoleBinding.Items, gc.IsNil)

	daemonSets, err := s.client.AppsV1().DaemonSets(s.namespace).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(daemonSets.Items, gc.IsNil)

	deployments, err := s.client.AppsV1().Deployments(s.namespace).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(deployments.Items, gc.IsNil)

	roles, err := s.client.RbacV1().Roles(s.namespace).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(roles.Items, gc.IsNil)

	roleBindings, err := s.client.RbacV1().RoleBindings(s.namespace).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(roleBindings.Items, gc.IsNil)

	secrets, err := s.client.CoreV1().Secrets(s.namespace).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(secrets.Items, gc.IsNil)

	services, err := s.client.CoreV1().Services(s.namespace).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(services.Items, gc.IsNil)

	serviceAccounts, err := s.client.CoreV1().ServiceAccounts(s.namespace).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(serviceAccounts.Items, gc.IsNil)

	statefulSets, err := s.client.AppsV1().StatefulSets(s.namespace).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statefulSets.Items, gc.IsNil)
}

func getPodSpec(c *gc.C) corev1.PodSpec {
	jujuDataDir := paths.DataDir(paths.OSUnixLike)
	return corev1.PodSpec{
		ServiceAccountName:           "gitlab",
		AutomountServiceAccountToken: pointer.BoolPtr(true),
		ImagePullSecrets:             []corev1.LocalObjectReference{{Name: "gitlab-nginx-secret"}},
		InitContainers: []corev1.Container{{
			Name:            "charm-init",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Image:           "operator/image-path:1.1.1",
			WorkingDir:      jujuDataDir,
			Command:         []string{"/opt/containeragent"},
			Args:            []string{"init", "--data-dir", "/var/lib/juju", "--bin-dir", "/charm/bin"},
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
			},
		}},
		Containers: []corev1.Container{{
			Name:            "charm",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Image:           "ubuntu:20.04",
			WorkingDir:      jujuDataDir,
			Command:         []string{"/charm/bin/containeragent"},
			Args:            []string{"unit", "--data-dir", jujuDataDir, "--charm-modified-version", "9001", "--append-env", "PATH=$PATH:/charm/bin"},
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
			SecurityContext: &corev1.SecurityContext{
				RunAsUser:  int64Ptr(0),
				RunAsGroup: int64Ptr(0),
			},
			LivenessProbe: &corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: constants.AgentHTTPPathLiveness,
						Port: intstr.Parse(constants.AgentHTTPProbePort),
					},
				},
				InitialDelaySeconds: 30,
				PeriodSeconds:       10,
				SuccessThreshold:    1,
				FailureThreshold:    2,
			},
			ReadinessProbe: &corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: constants.AgentHTTPPathReadiness,
						Port: intstr.Parse(constants.AgentHTTPProbePort),
					},
				},
				InitialDelaySeconds: 30,
				PeriodSeconds:       10,
				SuccessThreshold:    1,
				FailureThreshold:    2,
			},
			StartupProbe: &corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: constants.AgentHTTPPathStartup,
						Port: intstr.Parse(constants.AgentHTTPProbePort),
					},
				},
				InitialDelaySeconds: 30,
				PeriodSeconds:       10,
				SuccessThreshold:    1,
				FailureThreshold:    2,
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
					Name:      "gitlab-database-appuuid",
					MountPath: "path/to/here",
				},
			},
		}, {
			Name:            "gitlab",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Image:           "gitlab-image:latest",
			Command:         []string{"/charm/bin/pebble"},
			Args:            []string{"run", "--create-dirs", "--hold", "--verbose"},
			Env: []corev1.EnvVar{
				{
					Name:  "JUJU_CONTAINER_NAME",
					Value: "gitlab",
				},
				{
					Name:  "PEBBLE_SOCKET",
					Value: "/charm/container/pebble.socket",
				},
			},
			SecurityContext: &corev1.SecurityContext{
				RunAsUser:  int64Ptr(0),
				RunAsGroup: int64Ptr(0),
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
		}, {
			Name:            "nginx",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Image:           "nginx-image:latest",
			Command:         []string{"/charm/bin/pebble"},
			Args:            []string{"run", "--create-dirs", "--hold", "--verbose"},
			Env: []corev1.EnvVar{
				{
					Name:  "JUJU_CONTAINER_NAME",
					Value: "nginx",
				},
				{
					Name:  "PEBBLE_SOCKET",
					Value: "/charm/container/pebble.socket",
				},
			},
			SecurityContext: &corev1.SecurityContext{
				RunAsUser:  int64Ptr(0),
				RunAsGroup: int64Ptr(0),
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

func (s *applicationSuite) TestEnsureStateful(c *gc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	s.assertEnsure(
		c, app, false, constraints.Value{}, true, func() {
			svc, err := s.client.CoreV1().Services("test").Get(context.TODO(), "gitlab-endpoints", metav1.GetOptions{})
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(svc, gc.DeepEquals, &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gitlab-endpoints",
					Namespace: "test",
					Labels: map[string]string{
						"app.kubernetes.io/name":       "gitlab",
						"app.kubernetes.io/managed-by": "juju",
					},
					Annotations: map[string]string{
						"juju.is/version": "1.1.1",
						"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
					},
				},
				Spec: corev1.ServiceSpec{
					Selector:                 map[string]string{"app.kubernetes.io/name": "gitlab"},
					Type:                     corev1.ServiceTypeClusterIP,
					ClusterIP:                "None",
					PublishNotReadyAddresses: true,
				},
			})

			ss, err := s.client.AppsV1().StatefulSets("test").Get(context.TODO(), "gitlab", metav1.GetOptions{})
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(ss, gc.DeepEquals, &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gitlab",
					Namespace: "test",
					Labels: map[string]string{
						"app.kubernetes.io/name":       "gitlab",
						"app.kubernetes.io/managed-by": "juju",
					},
					Annotations: map[string]string{
						"juju.is/version":  "1.1.1",
						"app.juju.is/uuid": "appuuid",
					},
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: pointer.Int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name": "gitlab",
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels:      map[string]string{"app.kubernetes.io/name": "gitlab"},
							Annotations: map[string]string{"juju.is/version": "1.1.1"},
						},
						Spec: getPodSpec(c),
					},
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "gitlab-database-appuuid",
								Labels: map[string]string{
									"storage.juju.is/name":         "database",
									"app.kubernetes.io/managed-by": "juju",
								},
								Annotations: map[string]string{
									"foo":                  "bar",
									"storage.juju.is/name": "database",
								}},
							Spec: corev1.PersistentVolumeClaimSpec{
								StorageClassName: pointer.StringPtr("test-workload-storage"),
								AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceStorage: k8sresource.MustParse("100Mi"),
									},
								},
							},
						},
					},
					PodManagementPolicy: appsv1.ParallelPodManagement,
					ServiceName:         "gitlab-endpoints",
				},
			})
		},
	)
	s.assertDelete(c, app)
}

func (s *applicationSuite) TestEnsureTrusted(c *gc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	s.assertEnsure(
		c, app, false, constraints.Value{}, true, func() {},
	)
	s.assertDelete(c, app)
}

func (s *applicationSuite) TestEnsureUntrusted(c *gc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	s.assertEnsure(
		c, app, false, constraints.Value{}, false, func() {},
	)
	s.assertDelete(c, app)
}

func (s *applicationSuite) TestEnsureStatefulPrivateImageRepo(c *gc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateful, false)

	podSpec := getPodSpec(c)
	podSpec.ImagePullSecrets = append(
		[]corev1.LocalObjectReference{
			{Name: constants.CAASImageRepoSecretName},
		},
		podSpec.ImagePullSecrets...,
	)
	s.assertEnsure(
		c, app, true, constraints.Value{}, true, func() {
			svc, err := s.client.CoreV1().Services("test").Get(context.TODO(), "gitlab-endpoints", metav1.GetOptions{})
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(svc, gc.DeepEquals, &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gitlab-endpoints",
					Namespace: "test",
					Labels: map[string]string{
						"app.kubernetes.io/name":       "gitlab",
						"app.kubernetes.io/managed-by": "juju",
					},
					Annotations: map[string]string{
						"juju.is/version": "1.1.1",
						"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
					},
				},
				Spec: corev1.ServiceSpec{
					Selector:                 map[string]string{"app.kubernetes.io/name": "gitlab"},
					Type:                     corev1.ServiceTypeClusterIP,
					ClusterIP:                "None",
					PublishNotReadyAddresses: true,
				},
			})

			ss, err := s.client.AppsV1().StatefulSets("test").Get(context.TODO(), "gitlab", metav1.GetOptions{})
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(ss, gc.DeepEquals, &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gitlab",
					Namespace: "test",
					Labels: map[string]string{
						"app.kubernetes.io/name":       "gitlab",
						"app.kubernetes.io/managed-by": "juju",
					},
					Annotations: map[string]string{
						"juju.is/version":  "1.1.1",
						"app.juju.is/uuid": "appuuid",
					},
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: pointer.Int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name": "gitlab",
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels:      map[string]string{"app.kubernetes.io/name": "gitlab"},
							Annotations: map[string]string{"juju.is/version": "1.1.1"},
						},
						Spec: podSpec,
					},
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "gitlab-database-appuuid",
								Labels: map[string]string{
									"storage.juju.is/name":         "database",
									"app.kubernetes.io/managed-by": "juju",
								},
								Annotations: map[string]string{
									"foo":                  "bar",
									"storage.juju.is/name": "database",
								}},
							Spec: corev1.PersistentVolumeClaimSpec{
								StorageClassName: pointer.StringPtr("test-workload-storage"),
								AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceStorage: k8sresource.MustParse("100Mi"),
									},
								},
							},
						},
					},
					PodManagementPolicy: appsv1.ParallelPodManagement,
					ServiceName:         "gitlab-endpoints",
				},
			})
		},
	)
	s.assertDelete(c, app)
}

func (s *applicationSuite) TestEnsureStateless(c *gc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateless, false)
	s.assertEnsure(
		c, app, false, constraints.Value{}, true, func() {
			ss, err := s.client.AppsV1().Deployments("test").Get(context.TODO(), "gitlab", metav1.GetOptions{})
			c.Assert(err, jc.ErrorIsNil)

			pvc, err := s.client.CoreV1().PersistentVolumeClaims("test").Get(context.TODO(), "gitlab-database-appuuid", metav1.GetOptions{})
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(pvc, gc.DeepEquals, &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gitlab-database-appuuid",
					Namespace: "test",
					Labels: map[string]string{
						"storage.juju.is/name":         "database",
						"app.kubernetes.io/managed-by": "juju",
					},
					Annotations: map[string]string{
						"foo":                  "bar",
						"storage.juju.is/name": "database",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: pointer.StringPtr("test-workload-storage"),
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: k8sresource.MustParse("100Mi"),
						},
					},
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				},
			})

			podSpec := getPodSpec(c)
			podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
				Name: "gitlab-database-appuuid",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "gitlab-database-appuuid",
					}},
			})
			c.Assert(ss, gc.DeepEquals, &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gitlab",
					Namespace: "test",
					Labels: map[string]string{
						"app.kubernetes.io/name":       "gitlab",
						"app.kubernetes.io/managed-by": "juju",
					},
					Annotations: map[string]string{
						"juju.is/version":  "1.1.1",
						"app.juju.is/uuid": "appuuid",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: pointer.Int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app.kubernetes.io/name": "gitlab"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels:      map[string]string{"app.kubernetes.io/name": "gitlab"},
							Annotations: map[string]string{"juju.is/version": "1.1.1"},
						},
						Spec: podSpec,
					},
				},
			})
		},
	)
	s.assertDelete(c, app)
}

func (s *applicationSuite) TestEnsureDaemon(c *gc.C) {
	app, _ := s.getApp(c, caas.DeploymentDaemon, false)
	s.assertEnsure(
		c, app, false, constraints.Value{}, true, func() {
			ss, err := s.client.AppsV1().DaemonSets("test").Get(context.TODO(), "gitlab", metav1.GetOptions{})
			c.Assert(err, jc.ErrorIsNil)

			pvc, err := s.client.CoreV1().PersistentVolumeClaims("test").Get(context.TODO(), "gitlab-database-appuuid", metav1.GetOptions{})
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(pvc, gc.DeepEquals, &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gitlab-database-appuuid",
					Namespace: "test",
					Labels: map[string]string{
						"storage.juju.is/name":         "database",
						"app.kubernetes.io/managed-by": "juju",
					},
					Annotations: map[string]string{
						"foo":                  "bar",
						"storage.juju.is/name": "database",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: pointer.StringPtr("test-workload-storage"),
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: k8sresource.MustParse("100Mi"),
						},
					},
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				},
			})

			podSpec := getPodSpec(c)
			podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
				Name: "gitlab-database-appuuid",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "gitlab-database-appuuid",
					}},
			})
			c.Assert(ss, gc.DeepEquals, &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gitlab",
					Namespace: "test",
					Labels: map[string]string{
						"app.kubernetes.io/name":       "gitlab",
						"app.kubernetes.io/managed-by": "juju",
					},
					Annotations: map[string]string{
						"juju.is/version":  "1.1.1",
						"app.juju.is/uuid": "appuuid",
					},
				},
				Spec: appsv1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app.kubernetes.io/name": "gitlab"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels:      map[string]string{"app.kubernetes.io/name": "gitlab"},
							Annotations: map[string]string{"juju.is/version": "1.1.1"},
						},
						Spec: podSpec,
					},
				},
			})
		},
	)
	s.assertDelete(c, app)
}

func (s *applicationSuite) TestExistsNotSupported(c *gc.C) {
	app, _ := s.getApp(c, "notsupported", false)
	_, err := app.Exists()
	c.Assert(err, gc.ErrorMatches, `unknown deployment type not supported`)
}

func (s *applicationSuite) TestExistsDeployment(c *gc.C) {
	now := metav1.Now()

	app, _ := s.getApp(c, caas.DeploymentStateless, false)
	// Deployment does not exists.
	result, err := app.Exists()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, caas.DeploymentState{})

	// ensure a terminating Deployment.
	dr := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gitlab",
			Namespace: "test",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "gitlab",
				"app.kubernetes.io/managed-by": "juju",
			},
			Annotations: map[string]string{"juju.is/version": "1.1.1"},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "gitlab"},
			},
		},
	}
	dr.SetDeletionTimestamp(&now)
	_, err = s.client.AppsV1().Deployments("test").Create(context.TODO(),
		dr, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Deployment exists and is terminating.
	result, err = app.Exists()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, caas.DeploymentState{
		Exists: true, Terminating: true,
	})
}

func (s *applicationSuite) TestExistsStatefulSet(c *gc.C) {
	now := metav1.Now()

	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	// Statefulset does not exists.
	result, err := app.Exists()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, caas.DeploymentState{})

	// ensure a terminating Statefulset.
	sr := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gitlab",
			Namespace: "test",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "gitlab",
				"app.kubernetes.io/managed-by": "juju",
			},
			Annotations: map[string]string{"juju.is/version": "1.1.1"},
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "gitlab"},
			},
		},
	}
	sr.SetDeletionTimestamp(&now)
	_, err = s.client.AppsV1().StatefulSets("test").Create(context.TODO(),
		sr, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Statefulset exists and is terminating.
	result, err = app.Exists()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, caas.DeploymentState{
		Exists: true, Terminating: true,
	})

}

func (s *applicationSuite) TestExistsDaemonSet(c *gc.C) {
	now := metav1.Now()

	app, _ := s.getApp(c, caas.DeploymentDaemon, false)
	// Daemonset does not exists.
	result, err := app.Exists()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, caas.DeploymentState{})

	// ensure a terminating Daemonset.
	dmr := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gitlab",
			Namespace: "test",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "gitlab",
				"app.kubernetes.io/managed-by": "juju",
			},
			Annotations: map[string]string{"juju.is/version": "1.1.1"},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "gitlab"},
			},
		},
	}
	dmr.SetDeletionTimestamp(&now)
	_, err = s.client.AppsV1().DaemonSets("test").Create(context.TODO(),
		dmr, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Daemonset exists and is terminating.
	result, err = app.Exists()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, caas.DeploymentState{
		Exists: true, Terminating: true,
	})
}

func (s *applicationSuite) TestUpgradeStateful(c *gc.C) {
	// Not mock applier and ensure the resources created in the s.client.
	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	s.assertEnsure(c, app, false, constraints.Value{}, true, func() {})

	app, ctrl := s.getApp(c, caas.DeploymentStateful, true)
	defer ctrl.Finish()

	targetVersion := version.MustParse("2.9.1")
	versionKey := k8sutils.AnnotationVersionKey(app.IsLegacyLabels())

	assertExpectedVersion := func(r application.AnnotationUpdater) {
		c.Assert(r.Get(context.Background(), s.client), jc.ErrorIsNil)
		c.Assert(r.GetAnnotations()[versionKey], gc.Equals, "1.1.1")
		r.SetAnnotations(annotations.New(r.GetAnnotations()).Add(
			versionKey, targetVersion.String(),
		))
	}

	headlessSvc := resources.NewService("gitlab-endpoints", "test", nil)
	assertExpectedVersion(headlessSvc)

	ss := resources.NewStatefulSet("gitlab", "test", nil)
	assertExpectedVersion(ss)
	ss.Spec.Template.SetAnnotations(annotations.New(ss.Spec.Template.GetAnnotations()).Add(
		versionKey, targetVersion.String(),
	))
	initContainer := ss.Spec.Template.Spec.InitContainers[0]
	c.Assert(initContainer.Image, gc.Equals, `operator/image-path:1.1.1`)
	initContainer.Image = `operator/image-path:2.9.1`
	ss.Spec.Template.Spec.InitContainers = []corev1.Container{initContainer}

	secret := resources.NewSecret("gitlab-application-config", "test", nil)
	assertExpectedVersion(secret)

	sa := resources.NewServiceAccount("gitlab", "test", nil)
	assertExpectedVersion(sa)

	role := resources.NewRole("gitlab", "test", nil)
	assertExpectedVersion(role)

	rolebinding := resources.NewRoleBinding("gitlab", "test", nil)
	assertExpectedVersion(rolebinding)

	clusterrole := resources.NewClusterRole("test-gitlab", nil)
	assertExpectedVersion(clusterrole)

	clusterrolebinding := resources.NewClusterRoleBinding("test-gitlab", nil)
	assertExpectedVersion(clusterrolebinding)

	svc := resources.NewService("gitlab", "test", nil)
	assertExpectedVersion(svc)

	gomock.InOrder(
		s.applier.EXPECT().Apply(headlessSvc),
		s.applier.EXPECT().Apply(ss),
		s.applier.EXPECT().Apply(secret),
		s.applier.EXPECT().Apply(sa),
		s.applier.EXPECT().Apply(role),
		s.applier.EXPECT().Apply(rolebinding),
		s.applier.EXPECT().Apply(clusterrole),
		s.applier.EXPECT().Apply(clusterrolebinding),
		s.applier.EXPECT().Apply(svc),
		s.applier.EXPECT().Run(context.Background(), s.client, false).Return(nil),
	)
	c.Assert(app.Upgrade(targetVersion), jc.ErrorIsNil)
}

func (s *applicationSuite) TestUpgradeStateless(c *gc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentStateless, true)
	defer ctrl.Finish()
	err := app.Upgrade(version.MustParse("2.9.1"))
	c.Assert(err, gc.ErrorMatches, `upgrade for deployment type "stateless" not supported`)
}

func (s *applicationSuite) TestUpgradeDaemon(c *gc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentDaemon, true)
	defer ctrl.Finish()
	err := app.Upgrade(version.MustParse("2.9.1"))
	c.Assert(err, gc.ErrorMatches, `upgrade for deployment type "daemon" not supported`)
}

func (s *applicationSuite) TestUpgradeNotsupported(c *gc.C) {
	app, ctrl := s.getApp(c, "bad-deployment-type", true)
	defer ctrl.Finish()
	err := app.Upgrade(version.MustParse("2.9.1"))
	c.Assert(err, gc.ErrorMatches, `unknown deployment type "bad-deployment-type" not supported`)
}

func (s *applicationSuite) TestDeleteStateful(c *gc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentStateful, true)
	defer ctrl.Finish()

	gomock.InOrder(
		s.applier.EXPECT().Delete(resources.NewStatefulSet("gitlab", "test", nil)),
		s.applier.EXPECT().Delete(resources.NewService("gitlab-endpoints", "test", nil)),
		s.applier.EXPECT().Delete(resources.NewService("gitlab", "test", nil)),
		s.applier.EXPECT().Delete(resources.NewSecret("gitlab-application-config", "test", nil)),
		s.applier.EXPECT().Delete(resources.NewRoleBinding("gitlab", "test", nil)),
		s.applier.EXPECT().Delete(resources.NewRole("gitlab", "test", nil)),
		s.applier.EXPECT().Delete(resources.NewClusterRoleBinding("test-gitlab", nil)),
		s.applier.EXPECT().Delete(resources.NewClusterRole("test-gitlab", nil)),
		s.applier.EXPECT().Delete(resources.NewServiceAccount("gitlab", "test", nil)),
		s.applier.EXPECT().Run(context.Background(), s.client, false).Return(nil),
	)
	c.Assert(app.Delete(), jc.ErrorIsNil)
}

func (s *applicationSuite) TestDeleteStateless(c *gc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentStateless, true)
	defer ctrl.Finish()

	gomock.InOrder(
		s.applier.EXPECT().Delete(resources.NewDeployment("gitlab", "test", nil)),
		s.applier.EXPECT().Delete(resources.NewService("gitlab", "test", nil)),
		s.applier.EXPECT().Delete(resources.NewSecret("gitlab-application-config", "test", nil)),
		s.applier.EXPECT().Delete(resources.NewRoleBinding("gitlab", "test", nil)),
		s.applier.EXPECT().Delete(resources.NewRole("gitlab", "test", nil)),
		s.applier.EXPECT().Delete(resources.NewClusterRoleBinding("test-gitlab", nil)),
		s.applier.EXPECT().Delete(resources.NewClusterRole("test-gitlab", nil)),
		s.applier.EXPECT().Delete(resources.NewServiceAccount("gitlab", "test", nil)),
		s.applier.EXPECT().Run(context.Background(), s.client, false).Return(nil),
	)
	c.Assert(app.Delete(), jc.ErrorIsNil)
}

func (s *applicationSuite) TestDeleteDaemon(c *gc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentDaemon, true)
	defer ctrl.Finish()

	gomock.InOrder(
		s.applier.EXPECT().Delete(resources.NewDaemonSet("gitlab", "test", nil)),
		s.applier.EXPECT().Delete(resources.NewService("gitlab", "test", nil)),
		s.applier.EXPECT().Delete(resources.NewSecret("gitlab-application-config", "test", nil)),
		s.applier.EXPECT().Delete(resources.NewRoleBinding("gitlab", "test", nil)),
		s.applier.EXPECT().Delete(resources.NewRole("gitlab", "test", nil)),
		s.applier.EXPECT().Delete(resources.NewClusterRoleBinding("test-gitlab", nil)),
		s.applier.EXPECT().Delete(resources.NewClusterRole("test-gitlab", nil)),
		s.applier.EXPECT().Delete(resources.NewServiceAccount("gitlab", "test", nil)),
		s.applier.EXPECT().Run(context.Background(), s.client, false).Return(nil),
	)
	c.Assert(app.Delete(), jc.ErrorIsNil)
}

func (s *applicationSuite) TestWatchNotsupported(c *gc.C) {
	app, ctrl := s.getApp(c, "notsupported", true)
	defer ctrl.Finish()

	s.k8sWatcherFn = func(_ cache.SharedIndexInformer, _ string, _ jujuclock.Clock) (k8swatcher.KubernetesNotifyWatcher, error) {
		w, _ := k8swatchertest.NewKubernetesTestWatcher()
		return w, nil
	}

	_, err := app.Watch()
	c.Assert(err, gc.ErrorMatches, `unknown deployment type not supported`)
}

func (s *applicationSuite) TestWatch(c *gc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentDaemon, true)
	defer ctrl.Finish()

	s.k8sWatcherFn = func(_ cache.SharedIndexInformer, _ string, _ jujuclock.Clock) (k8swatcher.KubernetesNotifyWatcher, error) {
		w, _ := k8swatchertest.NewKubernetesTestWatcher()
		return w, nil
	}

	w, err := app.Watch()
	c.Assert(err, jc.ErrorIsNil)

	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for event")
	}
}

func (s *applicationSuite) TestWatchReplicas(c *gc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentDaemon, true)
	defer ctrl.Finish()

	s.k8sWatcherFn = func(_ cache.SharedIndexInformer, _ string, _ jujuclock.Clock) (k8swatcher.KubernetesNotifyWatcher, error) {
		w, _ := k8swatchertest.NewKubernetesTestWatcher()
		return w, nil
	}

	w, err := app.WatchReplicas()
	c.Assert(err, jc.ErrorIsNil)

	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for event")
	}
}

func (s *applicationSuite) TestStateNotSupported(c *gc.C) {
	app, _ := s.getApp(c, "notsupported", false)
	_, err := app.State()
	c.Assert(err, gc.ErrorMatches, `unknown deployment type not supported`)
}

func (s *applicationSuite) assertState(c *gc.C, deploymentType caas.DeploymentType, createMainResource func() int) {
	app, ctrl := s.getApp(c, deploymentType, false)
	defer ctrl.Finish()

	desiredReplicas := createMainResource()

	pod1 := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "pod1",
			Namespace:   "test",
			Labels:      map[string]string{"app.kubernetes.io/name": "gitlab"},
			Annotations: map[string]string{"juju.is/version": "1.1.1"},
		},
	}
	_, err := s.client.CoreV1().Pods("test").Create(context.TODO(),
		pod1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	pod2 := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "pod2",
			Namespace:   "test",
			Labels:      map[string]string{"app.kubernetes.io/name": "gitlab"},
			Annotations: map[string]string{"juju.is/version": "1.1.1"},
		},
	}
	_, err = s.client.CoreV1().Pods("test").Create(context.TODO(),
		pod2, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	appState, err := app.State()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appState, gc.DeepEquals, caas.ApplicationState{
		DesiredReplicas: desiredReplicas,
		Replicas:        []string{"pod1", "pod2"},
	})
}

func (s *applicationSuite) TestStateStateful(c *gc.C) {
	s.assertState(c, caas.DeploymentStateful, func() int {
		desiredReplicas := 10

		dmr := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gitlab",
				Namespace: "test",
				Labels: map[string]string{
					"app.kubernetes.io/name":       "gitlab",
					"app.kubernetes.io/managed-by": "juju",
				},
				Annotations: map[string]string{"juju.is/version": "1.1.1"},
			},
			Spec: appsv1.StatefulSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app.kubernetes.io/name": "gitlab"},
				},
				Replicas: pointer.Int32Ptr(int32(desiredReplicas)),
			},
		}
		_, err := s.client.AppsV1().StatefulSets("test").Create(context.TODO(),
			dmr, metav1.CreateOptions{})
		c.Assert(err, jc.ErrorIsNil)
		return desiredReplicas
	})
}

func (s *applicationSuite) TestStateStateless(c *gc.C) {
	s.assertState(c, caas.DeploymentStateless, func() int {
		desiredReplicas := 10

		dmr := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gitlab",
				Namespace: "test",
				Labels: map[string]string{
					"app.kubernetes.io/name":       "gitlab",
					"app.kubernetes.io/managed-by": "juju",
				},
				Annotations: map[string]string{"juju.is/version": "1.1.1"},
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app.kubernetes.io/name": "gitlab"},
				},
				Replicas: pointer.Int32Ptr(int32(desiredReplicas)),
			},
		}
		_, err := s.client.AppsV1().Deployments("test").Create(context.TODO(),
			dmr, metav1.CreateOptions{})
		c.Assert(err, jc.ErrorIsNil)
		return desiredReplicas
	})
}

func (s *applicationSuite) TestStateDaemon(c *gc.C) {
	s.assertState(c, caas.DeploymentDaemon, func() int {
		desiredReplicas := 10

		dmr := &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gitlab",
				Namespace: "test",
				Labels: map[string]string{
					"app.kubernetes.io/name":       "gitlab",
					"app.kubernetes.io/managed-by": "juju",
				},
				Annotations: map[string]string{"juju.is/version": "1.1.1"},
			},
			Spec: appsv1.DaemonSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app.kubernetes.io/name": "gitlab"},
				},
			},
			Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: int32(desiredReplicas),
			},
		}
		_, err := s.client.AppsV1().DaemonSets("test").Create(context.TODO(),
			dmr, metav1.CreateOptions{})
		c.Assert(err, jc.ErrorIsNil)
		return desiredReplicas
	})
}

func getDefaultSvc() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gitlab",
			Namespace: "test",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "gitlab",
				"app.kubernetes.io/managed-by": "juju",
			},
			Annotations: map[string]string{"juju.is/version": "1.1.1"},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app.kubernetes.io/name": "gitlab"},

			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

func (s *applicationSuite) TestUpdatePortsStatelessUpdateContainerPorts(c *gc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentStateless, true)
	defer ctrl.Finish()

	_, err := s.client.CoreV1().Services("test").Create(context.TODO(), getDefaultSvc(), metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	getMainResourceSpec := func() *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gitlab",
				Namespace: "test",
				Labels: map[string]string{
					"app.kubernetes.io/name":       "gitlab",
					"app.kubernetes.io/managed-by": "juju",
				},
				Annotations: map[string]string{
					"juju.is/version":  "1.1.1",
					"app.juju.is/uuid": "appuuid",
				},
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app.kubernetes.io/name": "gitlab"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels:      map[string]string{"app.kubernetes.io/name": "gitlab"},
						Annotations: map[string]string{"juju.is/version": "1.1.1"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:            "charm",
							ImagePullPolicy: corev1.PullIfNotPresent,
							Image:           "operator/image-path",
							WorkingDir:      "/var/lib/juju",
							Command:         []string{"/charm/bin/containeragent"},
							Args:            []string{"unit", "--data-dir", "/var/lib/juju", "--append-env", "PATH=$PATH:/charm/bin"},
							Env: []corev1.EnvVar{{
								Name:  "HTTP_PROBE_PORT",
								Value: "3856",
							}},
							SecurityContext: &corev1.SecurityContext{
								RunAsUser:  int64Ptr(0),
								RunAsGroup: int64Ptr(0),
							},
							LivenessProbe: &corev1.Probe{
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/liveness",
										Port: intstr.FromString("3856"),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    2,
							},
							ReadinessProbe: &corev1.Probe{
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/readiness",
										Port: intstr.FromString("3856"),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    2,
							},
							StartupProbe: &corev1.Probe{
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/startup",
										Port: intstr.FromString("3856"),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    2,
							},
						}, {
							Name:            "gitlab",
							ImagePullPolicy: corev1.PullIfNotPresent,
							Image:           "test-image",
							Command:         []string{"/charm/bin/pebble"},
							Args:            []string{"listen", "--socket", "/charm/container/pebble.sock", "--append-env", "PATH=$PATH:/charm/bin"},
						}},
					},
				},
			},
		}
	}
	_, err = s.client.AppsV1().Deployments("test").Create(context.TODO(), getMainResourceSpec(), metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	updatedSvc := getDefaultSvc()
	updatedSvc.Spec.Ports = []corev1.ServicePort{
		{
			Name:       "port1",
			Port:       int32(80),
			TargetPort: intstr.FromInt(8080),
			Protocol:   corev1.ProtocolTCP,
		},
	}

	updatedMainResource := getMainResourceSpec()
	updatedMainResource.Spec.Template.Spec.Containers[1].Ports = []corev1.ContainerPort{
		{
			Name:          "port1",
			ContainerPort: int32(8080),
			Protocol:      corev1.ProtocolTCP,
		},
	}
	gomock.InOrder(
		s.applier.EXPECT().Apply(resources.NewService("gitlab", "test", updatedSvc)),
		s.applier.EXPECT().Apply(resources.NewDeployment("gitlab", "test", updatedMainResource)),
		s.applier.EXPECT().Run(context.Background(), s.client, false).Return(nil),
	)
	c.Assert(app.UpdatePorts([]caas.ServicePort{
		{
			Name:       "port1",
			Port:       80,
			TargetPort: 8080,
			Protocol:   "TCP",
		},
	}, true), jc.ErrorIsNil)
}

func (s *applicationSuite) TestUpdatePortsStatefulUpdateContainerPorts(c *gc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentStateful, true)
	defer ctrl.Finish()

	_, err := s.client.CoreV1().Services("test").Create(context.TODO(), getDefaultSvc(), metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	getMainResourceSpec := func() *appsv1.StatefulSet {
		return &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gitlab",
				Namespace: "test",
				Labels: map[string]string{
					"app.kubernetes.io/name":       "gitlab",
					"app.kubernetes.io/managed-by": "juju",
				},
				Annotations: map[string]string{
					"juju.is/version":  "1.1.1",
					"app.juju.is/uuid": "appuuid",
				},
			},
			Spec: appsv1.StatefulSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app.kubernetes.io/name": "gitlab"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels:      map[string]string{"app.kubernetes.io/name": "gitlab"},
						Annotations: map[string]string{"juju.is/version": "1.1.1"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:            "charm",
							ImagePullPolicy: corev1.PullIfNotPresent,
							Image:           "operator/image-path",
							WorkingDir:      "/var/lib/juju",
							Command:         []string{"/charm/bin/containeragent"},
							Args:            []string{"unit", "--data-dir", "/var/lib/juju", "--append-env", "PATH=$PATH:/charm/bin"},
							Env: []corev1.EnvVar{{
								Name:  "HTTP_PROBE_PORT",
								Value: "3856",
							}},
							SecurityContext: &corev1.SecurityContext{
								RunAsUser:  int64Ptr(0),
								RunAsGroup: int64Ptr(0),
							},
							LivenessProbe: &corev1.Probe{
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/liveness",
										Port: intstr.FromString("3856"),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    2,
							},
							ReadinessProbe: &corev1.Probe{
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/readiness",
										Port: intstr.FromString("3856"),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    2,
							},
							StartupProbe: &corev1.Probe{
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/startup",
										Port: intstr.FromString("3856"),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    2,
							},
						}, {
							Name:            "gitlab",
							ImagePullPolicy: corev1.PullIfNotPresent,
							Image:           "test-image",
							Command:         []string{"/charm/bin/pebble"},
							Args:            []string{"listen", "--socket", "/charm/container/pebble.sock", "--append-env", "PATH=$PATH:/charm/bin"},
						}},
					},
				},
			},
		}
	}
	_, err = s.client.AppsV1().StatefulSets("test").Create(context.TODO(), getMainResourceSpec(), metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	updatedSvc := getDefaultSvc()
	updatedSvc.Spec.Ports = []corev1.ServicePort{
		{
			Name:       "port1",
			Port:       int32(80),
			TargetPort: intstr.FromInt(8080),
			Protocol:   corev1.ProtocolTCP,
		},
	}

	updatedMainResource := getMainResourceSpec()
	updatedMainResource.Spec.Template.Spec.Containers[1].Ports = []corev1.ContainerPort{
		{
			Name:          "port1",
			ContainerPort: int32(8080),
			Protocol:      corev1.ProtocolTCP,
		},
	}
	gomock.InOrder(
		s.applier.EXPECT().Apply(resources.NewService("gitlab", "test", updatedSvc)),
		s.applier.EXPECT().Apply(resources.NewStatefulSet("gitlab", "test", updatedMainResource)),
		s.applier.EXPECT().Run(context.Background(), s.client, false).Return(nil),
	)
	c.Assert(app.UpdatePorts([]caas.ServicePort{
		{
			Name:       "port1",
			Port:       80,
			TargetPort: 8080,
			Protocol:   "TCP",
		},
	}, true), jc.ErrorIsNil)
}

func (s *applicationSuite) TestUpdatePortsDaemonUpdateContainerPorts(c *gc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentDaemon, true)
	defer ctrl.Finish()

	_, err := s.client.CoreV1().Services("test").Create(context.TODO(), getDefaultSvc(), metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	getMainResourceSpec := func() *appsv1.DaemonSet {
		return &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gitlab",
				Namespace: "test",
				Labels: map[string]string{
					"app.kubernetes.io/name":       "gitlab",
					"app.kubernetes.io/managed-by": "juju",
				},
				Annotations: map[string]string{
					"juju.is/version":  "1.1.1",
					"app.juju.is/uuid": "appuuid",
				},
			},
			Spec: appsv1.DaemonSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app.kubernetes.io/name": "gitlab"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels:      map[string]string{"app.kubernetes.io/name": "gitlab"},
						Annotations: map[string]string{"juju.is/version": "1.1.1"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:            "charm",
							ImagePullPolicy: corev1.PullIfNotPresent,
							Image:           "operator/image-path",
							WorkingDir:      "/var/lib/juju",
							Command:         []string{"/charm/bin/containeragent"},
							Args:            []string{"unit", "--data-dir", "/var/lib/juju", "--append-env", "PATH=$PATH:/charm/bin"},
							SecurityContext: &corev1.SecurityContext{
								RunAsUser:  int64Ptr(0),
								RunAsGroup: int64Ptr(0),
							},
						}, {
							Name:            "gitlab",
							ImagePullPolicy: corev1.PullIfNotPresent,
							Image:           "test-image",
							Command:         []string{"/charm/bin/pebble"},
							Args:            []string{"listen", "--socket", "/charm/container/pebble.sock", "--append-env", "PATH=$PATH:/charm/bin"},
						}},
					},
				},
			},
		}
	}
	_, err = s.client.AppsV1().DaemonSets("test").Create(context.TODO(), getMainResourceSpec(), metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	updatedSvc := getDefaultSvc()
	updatedSvc.Spec.Ports = []corev1.ServicePort{
		{
			Name:       "port1",
			Port:       int32(80),
			TargetPort: intstr.FromInt(8080),
			Protocol:   corev1.ProtocolTCP,
		},
	}

	updatedMainResource := getMainResourceSpec()
	updatedMainResource.Spec.Template.Spec.Containers[1].Ports = []corev1.ContainerPort{
		{
			Name:          "port1",
			ContainerPort: int32(8080),
			Protocol:      corev1.ProtocolTCP,
		},
	}
	gomock.InOrder(
		s.applier.EXPECT().Apply(resources.NewService("gitlab", "test", updatedSvc)),
		s.applier.EXPECT().Apply(resources.NewDaemonSet("gitlab", "test", updatedMainResource)),
		s.applier.EXPECT().Run(context.Background(), s.client, false).Return(nil),
	)
	c.Assert(app.UpdatePorts([]caas.ServicePort{
		{
			Name:       "port1",
			Port:       80,
			TargetPort: 8080,
			Protocol:   "TCP",
		},
	}, true), jc.ErrorIsNil)
}

func (s *applicationSuite) TestUpdatePortsStateless(c *gc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentStateless, true)
	defer ctrl.Finish()

	_, err := s.client.CoreV1().Services("test").Create(context.TODO(), getDefaultSvc(), metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	updatedSvc := getDefaultSvc()
	updatedSvc.Spec.Ports = []corev1.ServicePort{
		{
			Name:       "port1",
			Port:       int32(80),
			TargetPort: intstr.FromInt(8080),
			Protocol:   corev1.ProtocolTCP,
		},
	}

	gomock.InOrder(
		s.applier.EXPECT().Apply(resources.NewService("gitlab", "test", updatedSvc)),
		s.applier.EXPECT().Run(context.Background(), s.client, false).Return(nil),
	)
	c.Assert(app.UpdatePorts([]caas.ServicePort{
		{
			Name:       "port1",
			Port:       80,
			TargetPort: 8080,
			Protocol:   "TCP",
		},
	}, false), jc.ErrorIsNil)
}

func (s *applicationSuite) TestUpdatePortsStateful(c *gc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentStateful, true)
	defer ctrl.Finish()

	_, err := s.client.CoreV1().Services("test").Create(context.TODO(), getDefaultSvc(), metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	updatedSvc := getDefaultSvc()
	updatedSvc.Spec.Ports = []corev1.ServicePort{
		{
			Name:       "port1",
			Port:       int32(80),
			TargetPort: intstr.FromInt(8080),
			Protocol:   corev1.ProtocolTCP,
		},
	}

	gomock.InOrder(
		s.applier.EXPECT().Apply(resources.NewService("gitlab", "test", updatedSvc)),
		s.applier.EXPECT().Run(context.Background(), s.client, false).Return(nil),
	)
	c.Assert(app.UpdatePorts([]caas.ServicePort{
		{
			Name:       "port1",
			Port:       80,
			TargetPort: 8080,
			Protocol:   "TCP",
		},
	}, false), jc.ErrorIsNil)
}

func (s *applicationSuite) TestUpdatePortsDaemonUpdate(c *gc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentDaemon, true)
	defer ctrl.Finish()

	_, err := s.client.CoreV1().Services("test").Create(context.TODO(), getDefaultSvc(), metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	updatedSvc := getDefaultSvc()
	updatedSvc.Spec.Ports = []corev1.ServicePort{
		{
			Name:       "port1",
			Port:       int32(80),
			TargetPort: intstr.FromInt(8080),
			Protocol:   corev1.ProtocolTCP,
		},
	}

	gomock.InOrder(
		s.applier.EXPECT().Apply(resources.NewService("gitlab", "test", updatedSvc)),
		s.applier.EXPECT().Run(context.Background(), s.client, false).Return(nil),
	)
	c.Assert(app.UpdatePorts([]caas.ServicePort{
		{
			Name:       "port1",
			Port:       80,
			TargetPort: 8080,
			Protocol:   "TCP",
		},
	}, false), jc.ErrorIsNil)
}

func (s *applicationSuite) TestUnits(c *gc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateful, false)

	for i := 0; i < 4; i++ {
		podSpec := getPodSpec(c)
		podSpec.Volumes = append(podSpec.Volumes,
			corev1.Volume{
				Name: "gitlab-database-appuuid",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: fmt.Sprintf("gitlab-database-appuuid-gitlab-%d", i),
					},
				},
			},
		)
		// Ensure these volume sources are ignored
		podSpec.Volumes = append(podSpec.Volumes,
			corev1.Volume{
				Name: "vol-secret",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
					Secret: &corev1.SecretVolumeSource{
						// secret name must have "-token" suffix to be ignored (see lp:1925721)
						SecretName: "charm-data-token",
					},
				},
			},
			corev1.Volume{
				Name: "vol-projected",
				VolumeSource: corev1.VolumeSource{
					Projected: &corev1.ProjectedVolumeSource{},
				},
			},
			corev1.Volume{
				Name: "vol-configmap",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{},
				},
			},
			corev1.Volume{
				Name: "vol-hostpath",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{},
				},
			},
			corev1.Volume{
				Name: "vol-emptydir",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		)
		podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts,
			corev1.VolumeMount{Name: "vol-secret", MountPath: "path/secret"},
			corev1.VolumeMount{Name: "vol-projected", MountPath: "path/projected"},
			corev1.VolumeMount{Name: "vol-configmap", MountPath: "path/configmap"},
			corev1.VolumeMount{Name: "vol-hostpath", MountPath: "path/hostpath"},
			corev1.VolumeMount{Name: "vol-emptydir", MountPath: "path/emptydir"},
		)
		pod := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   s.namespace,
				Name:        fmt.Sprintf("%s-%d", s.appName, i),
				Labels:      map[string]string{"app.kubernetes.io/name": "gitlab"},
				Annotations: map[string]string{"juju.is/version": "1.1.1"},
			},
			Spec: podSpec,
			Status: corev1.PodStatus{
				PodIP: fmt.Sprintf("10.10.10.%d", i),
			},
		}
		switch i {
		case 0:
			pod.Status.Phase = corev1.PodRunning
		case 1:
			pod.Status.Phase = corev1.PodPending
		case 2:
			pod.DeletionTimestamp = &metav1.Time{
				Time: time.Now(),
			}
		case 3:
			pod.Status.Phase = corev1.PodFailed
		}
		_, err := s.client.CoreV1().Pods(s.namespace).Create(context.TODO(), &pod, metav1.CreateOptions{})
		c.Assert(err, jc.ErrorIsNil)

		pvc := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: s.namespace,
				Name:      fmt.Sprintf("gitlab-database-appuuid-gitlab-%d", i),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						"storage": k8sresource.MustParse("1Gi"),
					},
				},
				VolumeName: fmt.Sprintf("pv-%d", i),
			},
			Status: corev1.PersistentVolumeClaimStatus{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Capacity: corev1.ResourceList{
					"storage": k8sresource.MustParse("1Gi"),
				},
				Phase: corev1.ClaimBound,
			},
		}
		_, err = s.client.CoreV1().PersistentVolumeClaims(s.namespace).Create(context.TODO(), &pvc, metav1.CreateOptions{})
		c.Assert(err, jc.ErrorIsNil)

		pv := corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("pv-%d", i),
			},
			Spec: corev1.PersistentVolumeSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Capacity: corev1.ResourceList{
					"storage": k8sresource.MustParse("1Gi"),
				},
			},
			Status: corev1.PersistentVolumeStatus{
				Phase:   corev1.VolumeBound,
				Message: "volume bound",
			},
		}
		_, err = s.client.CoreV1().PersistentVolumes().Create(context.TODO(), &pv, metav1.CreateOptions{})
		c.Assert(err, jc.ErrorIsNil)
	}

	units, err := app.Units()
	c.Assert(err, jc.ErrorIsNil)

	mc := jc.NewMultiChecker()
	mc.AddExpr(`_[_].Status.Since`, jc.Ignore)
	mc.AddExpr(`_[_].FilesystemInfo[_].Status.Since`, jc.Ignore)
	mc.AddExpr(`_[_].FilesystemInfo[_].Volume.Status.Since`, jc.Ignore)

	c.Assert(units, mc, []caas.Unit{
		{
			Id:       "gitlab-0",
			Address:  "10.10.10.0",
			Ports:    []string(nil),
			Dying:    false,
			Stateful: true,
			Status: status.StatusInfo{
				Status: "running",
			},
			FilesystemInfo: []caas.FilesystemInfo{
				{
					StorageName:  "gitlab-database",
					FilesystemId: "",
					Size:         1024,
					MountPoint:   "path/to/here",
					ReadOnly:     false,
					Status: status.StatusInfo{
						Status: "attached",
					},
					Volume: caas.VolumeInfo{
						VolumeId:   "pv-0",
						Size:       1024,
						Persistent: false,
						Status: status.StatusInfo{
							Status:  "attached",
							Message: "volume bound",
						},
					},
				},
			},
		},
		{
			Id:       "gitlab-1",
			Address:  "10.10.10.1",
			Ports:    []string(nil),
			Dying:    false,
			Stateful: true,
			Status: status.StatusInfo{
				Status: "allocating",
			},
			FilesystemInfo: []caas.FilesystemInfo{
				{
					StorageName:  "gitlab-database",
					FilesystemId: "",
					Size:         1024,
					MountPoint:   "path/to/here",
					ReadOnly:     false,
					Status: status.StatusInfo{
						Status: "attached",
					},
					Volume: caas.VolumeInfo{
						VolumeId:   "pv-1",
						Size:       1024,
						Persistent: false,
						Status: status.StatusInfo{
							Status:  "attached",
							Message: "volume bound",
						},
					},
				},
			},
		},
		{
			Id:       "gitlab-2",
			Address:  "10.10.10.2",
			Ports:    []string(nil),
			Dying:    true,
			Stateful: true,
			Status: status.StatusInfo{
				Status: "terminated",
			},
			FilesystemInfo: []caas.FilesystemInfo{
				{
					StorageName:  "gitlab-database",
					FilesystemId: "",
					Size:         1024,
					MountPoint:   "path/to/here",
					ReadOnly:     false,
					Status: status.StatusInfo{
						Status: "attached",
					},
					Volume: caas.VolumeInfo{
						VolumeId:   "pv-2",
						Size:       1024,
						Persistent: false,
						Status: status.StatusInfo{
							Status:  "attached",
							Message: "volume bound",
						},
					},
				},
			},
		},
		{
			Id:       "gitlab-3",
			Address:  "10.10.10.3",
			Ports:    []string(nil),
			Dying:    false,
			Stateful: true,
			Status: status.StatusInfo{
				Status: "error",
			},
			FilesystemInfo: []caas.FilesystemInfo{
				{
					StorageName:  "gitlab-database",
					FilesystemId: "",
					Size:         1024,
					MountPoint:   "path/to/here",
					ReadOnly:     false,
					Status: status.StatusInfo{
						Status: "attached",
					},
					Volume: caas.VolumeInfo{
						VolumeId:   "pv-3",
						Size:       1024,
						Persistent: false,
						Status: status.StatusInfo{
							Status:  "attached",
							Message: "volume bound",
						},
					},
				},
			},
		},
	})
}

func (s *applicationSuite) TestService(c *gc.C) {
	testSvc := getDefaultSvc()
	testSvc.UID = "deadbeaf"
	testSvc.Spec.ClusterIP = "10.6.6.6"
	_, err := s.client.CoreV1().Services("test").Create(context.TODO(), testSvc, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	svc, err := app.Service()
	c.Assert(err, jc.ErrorIsNil)

	since := time.Time{}
	c.Assert(svc, jc.DeepEquals, &caas.Service{
		Id: "deadbeaf",
		Addresses: network.ProviderAddresses{{
			MachineAddress: network.MachineAddress{
				Value: "10.6.6.6",
				Type:  "ipv4",
				Scope: "local-cloud",
			},
		}},
		Status: status.StatusInfo{
			Status: "active",
			Since:  &since,
		},
	})
}

func (s *applicationSuite) TestEnsureConstraints(c *gc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	s.assertEnsure(
		c, app, false, constraints.MustParse("mem=1G cpu-power=1000 arch=arm64"), true, func() {
			svc, err := s.client.CoreV1().Services("test").Get(context.TODO(), "gitlab-endpoints", metav1.GetOptions{})
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(svc, gc.DeepEquals, &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gitlab-endpoints",
					Namespace: "test",
					Labels: map[string]string{
						"app.kubernetes.io/name":       "gitlab",
						"app.kubernetes.io/managed-by": "juju",
					},
					Annotations: map[string]string{
						"juju.is/version": "1.1.1",
						"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
					},
				},
				Spec: corev1.ServiceSpec{
					Selector:                 map[string]string{"app.kubernetes.io/name": "gitlab"},
					Type:                     corev1.ServiceTypeClusterIP,
					ClusterIP:                "None",
					PublishNotReadyAddresses: true,
				},
			})

			ps := getPodSpec(c)
			ps.NodeSelector = map[string]string{
				"kubernetes.io/arch": "arm64",
			}
			ps.Containers[0].Resources.Requests = corev1.ResourceList{
				corev1.ResourceCPU:    k8sresource.MustParse("1000m"),
				corev1.ResourceMemory: k8sresource.MustParse("1024Mi"),
			}

			ss, err := s.client.AppsV1().StatefulSets("test").Get(context.TODO(), "gitlab", metav1.GetOptions{})
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(ss, gc.DeepEquals, &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gitlab",
					Namespace: "test",
					Labels: map[string]string{
						"app.kubernetes.io/name":       "gitlab",
						"app.kubernetes.io/managed-by": "juju",
					},
					Annotations: map[string]string{
						"juju.is/version":  "1.1.1",
						"app.juju.is/uuid": "appuuid",
					},
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: pointer.Int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name": "gitlab",
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels:      map[string]string{"app.kubernetes.io/name": "gitlab"},
							Annotations: map[string]string{"juju.is/version": "1.1.1"},
						},
						Spec: ps,
					},
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "gitlab-database-appuuid",
								Labels: map[string]string{
									"storage.juju.is/name":         "database",
									"app.kubernetes.io/managed-by": "juju",
								},
								Annotations: map[string]string{
									"foo":                  "bar",
									"storage.juju.is/name": "database",
								}},
							Spec: corev1.PersistentVolumeClaimSpec{
								StorageClassName: pointer.StringPtr("test-workload-storage"),
								AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceStorage: k8sresource.MustParse("100Mi"),
									},
								},
							},
						},
					},
					PodManagementPolicy: appsv1.ParallelPodManagement,
					ServiceName:         "gitlab-endpoints",
				},
			})
		},
	)
}

func (s *applicationSuite) TestPullSecretUpdate(c *gc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateful, false)

	unusedPullSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gitlab-oldcontainer-secret",
			Namespace: "test",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "gitlab",
				"app.kubernetes.io/managed-by": "juju",
			},
			Annotations: map[string]string{"juju.is/version": "1.1.1"},
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: []byte("wow"),
		},
	}

	_, err := s.client.CoreV1().Secrets(s.namespace).Create(context.TODO(), &unusedPullSecret,
		metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	pullSecretConfig, _ := k8sutils.CreateDockerConfigJSON("username-old", "password-old", "nginx-image:latest")
	nginxPullSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gitlab-nginx-secret",
			Namespace: "test",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "gitlab",
				"app.kubernetes.io/managed-by": "juju",
			},
			Annotations: map[string]string{"juju.is/version": "1.1.1"},
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: pullSecretConfig,
		},
	}
	_, err = s.client.CoreV1().Secrets(s.namespace).Create(context.TODO(), &nginxPullSecret,
		metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	s.assertEnsure(c, app, false, constraints.Value{}, true, func() {})

	_, err = s.client.CoreV1().Secrets(s.namespace).Get(context.TODO(), "gitlab-oldcontainer-secret", metav1.GetOptions{})
	c.Assert(err, gc.ErrorMatches, `secrets "gitlab-oldcontainer-secret" not found`)

	secret, err := s.client.CoreV1().Secrets(s.namespace).Get(context.TODO(), "gitlab-nginx-secret", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(secret, gc.NotNil)
	newPullSecretConfig, _ := k8sutils.CreateDockerConfigJSON("username", "password", "nginx-image:latest")
	newNginxPullSecret := nginxPullSecret
	newNginxPullSecret.Data = map[string][]byte{
		corev1.DockerConfigJsonKey: newPullSecretConfig,
	}
	c.Assert(*secret, jc.DeepEquals, newNginxPullSecret)
}

func (s *applicationSuite) TestPVCNames(c *gc.C) {
	claims := []*corev1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "storage_a-abcd1234-gitlab-0",
				Namespace: "test",
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "juju",
					"app.kubernetes.io/name":       "gitlab",
					"storage.juju.is/name":         "storage_a",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gitlab-storage_b-abcd1235-gitlab-0",
				Namespace: "test",
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "juju",
					"app.kubernetes.io/name":       "gitlab",
					"storage.juju.is/name":         "storage_b",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "juju-storage_c-42",
				Namespace: "test",
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "juju",
					"app.kubernetes.io/name":       "gitlab",
					"storage.juju.is/name":         "storage_c",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "storage_d-abcd1234-gitlab-0",
				Namespace: "test",
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "juju",
					"app.kubernetes.io/name":       "another-app",
					"storage.juju.is/name":         "storage_d",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "storage_e-abcd1236-gitlab-0",
				Namespace: "test",
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "juju",
					"app.kubernetes.io/name":       "gitlab",
					// no "storage.juju.is/name" label -- will be ignored
				},
			},
		},
	}
	for _, claim := range claims {
		_, err := s.client.CoreV1().PersistentVolumeClaims("test").Create(context.Background(), claim, metav1.CreateOptions{})
		c.Assert(err, jc.ErrorIsNil)
	}

	names, err := application.PVCNames(s.client, "test", "gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(names, gc.DeepEquals, map[string]string{
		"gitlab-storage_a": "storage_a-abcd1234",
		"gitlab-storage_b": "gitlab-storage_b-abcd1235",
		"gitlab-storage_c": "juju-storage_c-42",
	})
}

func int64Ptr(a int64) *int64 {
	return &a
}
