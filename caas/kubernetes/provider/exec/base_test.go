// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package exec_test

import (
	"github.com/golang/mock/gomock"
	// jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	// clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/caas/kubernetes/provider/mocks"
	"github.com/juju/juju/testing"
)

type BaseSuite struct {
	testing.BaseSuite

	namespace     string
	k8sClient     *mocks.MockInterface
	restClient    *mocks.MockRestClientInterface
	execClient    exec.Executer
	mockPodGetter *mocks.MockPodInterface
}

func (s *BaseSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.namespace = "test"
}

func (s *BaseSuite) setupBroker(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.k8sClient = mocks.NewMockInterface(ctrl)

	mockCoreV1 := mocks.NewMockCoreV1Interface(ctrl)
	s.k8sClient.EXPECT().CoreV1().AnyTimes().Return(mockCoreV1)

	s.restClient = mocks.NewMockRestClientInterface(ctrl)
	mockCoreV1.EXPECT().RESTClient().AnyTimes().Return(s.restClient)

	s.mockPodGetter = mocks.NewMockPodInterface(ctrl)
	mockCoreV1.EXPECT().Pods(s.namespace).AnyTimes().Return(s.mockPodGetter)

	s.execClient = exec.New(s.namespace, s.k8sClient, &rest.Config{})
	return ctrl
}

func (s *BaseSuite) k8sNotFoundError() *k8serrors.StatusError {
	return k8serrors.NewNotFound(schema.GroupResource{}, "test")
}
