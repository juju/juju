// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
	stdtesting "testing"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/juju/juju/internal/provider/kubernetes"
	"github.com/juju/juju/internal/provider/kubernetes/mocks"
	k8swatcher "github.com/juju/juju/internal/provider/kubernetes/watcher"
	"github.com/juju/juju/internal/testing"
)

func TestModelOperatorExternalSuite(t *stdtesting.T) {
	tc.Run(t, &ModelOperatorExternalSuite{})
}

type ModelOperatorExternalSuite struct {
	BaseSuite
}

func (m *ModelOperatorExternalSuite) SetUpTest(c *tc.C) {
	m.BaseSuite.SetUpTest(c)
}

func (m *ModelOperatorExternalSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	m.BaseSuite.mockDeployments = mocks.NewMockDeploymentInterface(ctrl)
	return ctrl
}

func (m *ModelOperatorExternalSuite) setupBroker(c *tc.C) *gomock.Controller {
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
	m.broker, err = kubernetes.NewK8sBroker(c.Context(), testing.ControllerTag.Id(), m.k8sRestConfig, m.cfg, "", newK8sClientFunc, newK8sRestFunc,
		watcherFn, stringsWatcherFn, randomPrefixFunc, m.clock)

	c.Assert(err, tc.ErrorIsNil)
	return ctrl
}

func (m *ModelOperatorExternalSuite) TestGetModelOperatorDeploymentImage(c *tc.C) {
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
	deploymentImage, err := m.BaseSuite.broker.GetModelOperatorDeploymentImage(c.Context())
	c.Assert(err, tc.IsNil)
	c.Assert(deploymentImage, tc.Equals, imageName)
}
