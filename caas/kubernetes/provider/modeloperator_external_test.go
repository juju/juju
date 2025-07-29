// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/caas/kubernetes/provider/mocks"
	k8swatcher "github.com/juju/juju/caas/kubernetes/provider/watcher"
	"github.com/juju/juju/testing"
)

type ModelOperatorExternalSuite struct {
	BaseSuite
}

var _ = gc.Suite(&ModelOperatorExternalSuite{})

func (m *ModelOperatorExternalSuite) SetUpTest(c *gc.C) {
	m.BaseSuite.SetUpTest(c)
}

func (m *ModelOperatorExternalSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	m.BaseSuite.mockDeployments = mocks.NewMockDeploymentInterface(ctrl)
	return ctrl
}

func (m *ModelOperatorExternalSuite) setupBroker(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	m.clock = testclock.NewClock(time.Time{})

	newK8sClientFunc, newK8sRestFunc := m.setupK8sRestClient(c, ctrl, "")
	randomPrefixFunc := func() (string, error) {
		return "", nil
	}

	watcherFn := k8swatcher.NewK8sWatcherFunc(func(i cache.SharedIndexInformer, n string, c jujuclock.Clock) (k8swatcher.KubernetesNotifyWatcher, error) {
		return nil, errors.NewNotFound(nil, "undefined k8sWatcherFn")
	})
	stringsWatcherFn := k8swatcher.NewK8sStringsWatcherFunc(func(i cache.SharedIndexInformer, n string, c jujuclock.Clock, e []string,
		f k8swatcher.K8sStringsWatcherFilterFunc) (k8swatcher.KubernetesStringsWatcher, error) {
		return nil, errors.NewNotFound(nil, "undefined k8sStringsWatcherFn")
	})

	var err error
	m.broker, err = provider.NewK8sBroker(testing.ControllerTag.Id(), m.k8sRestConfig, m.cfg, "", newK8sClientFunc, newK8sRestFunc,
		watcherFn, stringsWatcherFn, randomPrefixFunc, m.clock)

	c.Assert(err, jc.ErrorIsNil)
	return ctrl
}

func (m *ModelOperatorExternalSuite) TestGetModelOperatorDeploymentImage(c *gc.C) {
	defer m.setupMocks(c).Finish()
	defer m.setupBroker(c).Finish()

	modelOperatorName := "modeloperator"
	imageName := "ghcr.io/juju/jujud-operator:3.6.9"

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: modelOperatorName,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Image: imageName},
					},
				},
			},
		},
	}

	m.BaseSuite.mockDeployments.EXPECT().Get(gomock.Any(), modelOperatorName, metav1.GetOptions{}).Return(deployment, nil)
	deploymentImage, err := m.BaseSuite.broker.GetModelOperatorDeploymentImage()
	c.Assert(err, gc.IsNil)
	c.Assert(deploymentImage, gc.Equals, imageName)
}
