// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"
	"time"

	"github.com/juju/charm/v7"
	jujuclock "github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	// apicommoncharms "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/application"
	// "github.com/juju/juju/caas/kubernetes/provider/storage"
	k8swatcher "github.com/juju/juju/caas/kubernetes/provider/watcher"
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

	s.BaseSuite.TearDownTest(c)
}

func (s *applicationSuite) getApp(deploymentType caas.DeploymentType) caas.Application {
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

	return application.NewApplication(
		s.appName, s.namespace, "test",
		deploymentType,
		s.client,
		watcherFn,
		s.clock,
	)
}

func (s *applicationSuite) getCharm(deployment *charm.Deployment) charm.Charm {
	return &fakeCharm{deployment}
}

func (s *applicationSuite) TestEnsureFailed(c *gc.C) {
	app := s.getApp(caas.DeploymentType("notsupported"))
	c.Assert(app.Ensure(
		caas.ApplicationConfig{
			Charm: s.getCharm(&charm.Deployment{
				DeploymentType:     charm.DeploymentType("notsupported"),
				ContainerImageName: "gitlab:latest",
			}),
		},
	), gc.ErrorMatches, `unknown deployment type not supported`)

	app = s.getApp(caas.DeploymentStateless)
	c.Assert(app.Ensure(
		caas.ApplicationConfig{},
	), gc.ErrorMatches, `charm was missing for gitlab application not valid`)

	c.Assert(app.Ensure(
		caas.ApplicationConfig{
			Charm: s.getCharm(&charm.Deployment{
				DeploymentType: charm.DeploymentStateful,
			}),
		},
	), gc.ErrorMatches, `charm deployment type mismatch with application not valid`)

	c.Assert(app.Ensure(
		caas.ApplicationConfig{
			Charm: s.getCharm(&charm.Deployment{
				DeploymentType: charm.DeploymentStateless,
			}),
		},
	), gc.ErrorMatches, `generating application podspec: charm missing container-image-name not valid`)

}

func (s *applicationSuite) TestEnsure(c *gc.C) {
	app := s.getApp(caas.DeploymentStateful)
	c.Assert(app.Ensure(
		caas.ApplicationConfig{},
	), gc.ErrorMatches, `charm was missing for gitlab application not valid`)

	c.Assert(app.Ensure(
		caas.ApplicationConfig{
			Charm: s.getCharm(&charm.Deployment{
				DeploymentType:     charm.DeploymentStateful,
				ContainerImageName: "gitlab:latest",
				ServicePorts: []charm.ServicePort{
					{
						Name:       "tcp",
						Port:       8080,
						TargetPort: 8080,
						Protocol:   "TCP",
					},
				},
			}),
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
		},
	), jc.ErrorIsNil)

	secret, err := s.client.CoreV1().Secrets("test").Get(context.TODO(), "gitlab-application-config", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(secret, gc.DeepEquals, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "gitlab-application-config",
			Namespace:   "test",
			Labels:      map[string]string{"juju-app": "gitlab"},
			Annotations: map[string]string{"juju-version": "0.0.0"},
		},
		Data: map[string][]byte{
			"JUJU_K8S_APPLICATION":          []byte("gitlab"),
			"JUJU_K8S_MODEL":                []byte("test"),
			"JUJU_K8S_APPLICATION_PASSWORD": []byte(""),
			"JUJU_K8S_CONTROLLER_ADDRESSES": []byte(""),
			"JUJU_K8S_CONTROLLER_CA_CERT":   []byte(""),
		},
	})

	svc, err := s.client.CoreV1().Services("test").Get(context.TODO(), "gitlab", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(svc, gc.DeepEquals, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "gitlab",
			Namespace:   "test",
			Labels:      map[string]string{"juju-app": "gitlab"},
			Annotations: map[string]string{"juju-version": "0.0.0"},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"juju-app": "gitlab"},
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "tcp",
					Port:       8080,
					TargetPort: intstr.FromInt(8080),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	})

	ss, err := s.client.AppsV1().StatefulSets("test").Get(context.TODO(), "gitlab", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ss, gc.DeepEquals, &appsv1.StatefulSet{})
}

type fakeCharm struct {
	// TODO: remove this once `api/common/charms.CharmInfo` has upgraded to use the new charm.Charm.
	deployment *charm.Deployment
}

func (c *fakeCharm) Meta() *charm.Meta {
	return &charm.Meta{Deployment: c.deployment}
}

func (c *fakeCharm) Config() *charm.Config {
	return nil
}

func (c *fakeCharm) Metrics() *charm.Metrics {
	return nil
}

func (c *fakeCharm) Actions() *charm.Actions {
	return nil
}

func (c *fakeCharm) Revision() int {
	return 0
}
