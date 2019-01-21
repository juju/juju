// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package clientconfig_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/juju/caas/kubernetes/provider/mocks"
	"github.com/juju/juju/testing"
	gc "gopkg.in/check.v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type BaseSuite struct {
	testing.BaseSuite

	namespace string

	ctrl                   *gomock.Controller
	k8sClient              *mocks.MockInterface
	mockRbacV1             *mocks.MockRbacV1Interface
	mockClusterRoles       *mocks.MockClusterRoleInterface
	mockClusterRoleBinding *mocks.MockClusterRoleBindingInterface
	mockServiceAccounts    *mocks.MockServiceAccountInterface
}

func (s *BaseSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.namespace = "test"
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.ctrl = s.setupBroker(c)
}

func (s *BaseSuite) TearDownTest(c *gc.C) {
	s.ctrl.Finish()
	s.BaseSuite.TearDownTest(c)
}

func (s *BaseSuite) setupBroker(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.k8sClient = mocks.NewMockInterface(ctrl)

	mockCoreV1 := mocks.NewMockCoreV1Interface(ctrl)
	s.k8sClient.EXPECT().CoreV1().AnyTimes().Return(mockCoreV1)

	s.mockServiceAccounts = mocks.NewMockServiceAccountInterface(ctrl)
	mockCoreV1.EXPECT().ServiceAccounts(s.namespace).AnyTimes().Return(s.mockServiceAccounts)

	s.mockRbacV1 = mocks.NewMockRbacV1Interface(ctrl)
	s.k8sClient.EXPECT().RbacV1().AnyTimes().Return(s.mockRbacV1)

	s.mockClusterRoles = mocks.NewMockClusterRoleInterface(ctrl)
	s.mockRbacV1.EXPECT().ClusterRoles().AnyTimes().Return(s.mockClusterRoles)

	s.mockClusterRoleBinding = mocks.NewMockClusterRoleBindingInterface(ctrl)
	s.mockRbacV1.EXPECT().ClusterRoleBindings().AnyTimes().Return(s.mockClusterRoleBinding)

	return ctrl
}

func (s *BaseSuite) k8sNotFoundError() *k8serrors.StatusError {
	return k8serrors.NewNotFound(schema.GroupResource{}, "test")
}

func (s *BaseSuite) k8sAlreadyExistsError() *k8serrors.StatusError {
	return k8serrors.NewAlreadyExists(schema.GroupResource{}, "test")
}
