// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package clientconfig_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/juju/juju/caas/kubernetes/clientconfig/mocks"
	"github.com/juju/juju/internal/testing"
)

type BaseSuite struct {
	testing.BaseSuite

	namespace string
	clock     *testclock.Clock

	k8sClient               *mocks.MockInterface
	mockRbacV1              *mocks.MockRbacV1Interface
	mockClusterRoles        *mocks.MockClusterRoleInterface
	mockClusterRoleBindings *mocks.MockClusterRoleBindingInterface
	mockServiceAccounts     *mocks.MockServiceAccountInterface
	mockSecrets             *mocks.MockSecretInterface
}

func (s *BaseSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.namespace = "test"
}

func (s *BaseSuite) setupBroker(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.k8sClient = mocks.NewMockInterface(ctrl)

	s.clock = testclock.NewClock(time.Time{})

	mockCoreV1 := mocks.NewMockCoreV1Interface(ctrl)
	s.k8sClient.EXPECT().CoreV1().AnyTimes().Return(mockCoreV1)

	s.mockServiceAccounts = mocks.NewMockServiceAccountInterface(ctrl)
	mockCoreV1.EXPECT().ServiceAccounts(s.namespace).AnyTimes().Return(s.mockServiceAccounts)

	s.mockSecrets = mocks.NewMockSecretInterface(ctrl)
	mockCoreV1.EXPECT().Secrets(s.namespace).AnyTimes().Return(s.mockSecrets)

	s.mockRbacV1 = mocks.NewMockRbacV1Interface(ctrl)
	s.k8sClient.EXPECT().RbacV1().AnyTimes().Return(s.mockRbacV1)

	s.mockClusterRoles = mocks.NewMockClusterRoleInterface(ctrl)
	s.mockRbacV1.EXPECT().ClusterRoles().AnyTimes().Return(s.mockClusterRoles)

	s.mockClusterRoleBindings = mocks.NewMockClusterRoleBindingInterface(ctrl)
	s.mockRbacV1.EXPECT().ClusterRoleBindings().AnyTimes().Return(s.mockClusterRoleBindings)

	return ctrl
}

func (s *BaseSuite) k8sNotFoundError() *k8serrors.StatusError {
	return k8serrors.NewNotFound(schema.GroupResource{}, "test")
}

func (s *BaseSuite) k8sAlreadyExistsError() *k8serrors.StatusError {
	return k8serrors.NewAlreadyExists(schema.GroupResource{}, "test")
}

func newClientConfig() *clientcmdapi.Config {
	cfg := clientcmdapi.NewConfig()
	cfg.Preferences.Colors = true
	cfg.Clusters["alfa"] = &clientcmdapi.Cluster{
		Server:                "https://alfa.org:8080",
		InsecureSkipTLSVerify: true,
		CertificateAuthority:  "path/to/my/cert-ca-filename",
	}
	cfg.Clusters["bravo"] = &clientcmdapi.Cluster{
		Server:                "https://bravo.org:8080",
		InsecureSkipTLSVerify: false,
	}
	cfg.AuthInfos["white-mage-via-cert"] = &clientcmdapi.AuthInfo{
		ClientCertificate: "path/to/my/client-cert-filename",
		ClientKey:         "path/to/my/client-key-filename",
	}
	cfg.AuthInfos["red-mage-via-token"] = &clientcmdapi.AuthInfo{
		Token: "my-secret-token",
	}
	cfg.AuthInfos["black-mage-via-auth-provider"] = &clientcmdapi.AuthInfo{
		AuthProvider: &clientcmdapi.AuthProviderConfig{
			Name: "gcp",
			Config: map[string]string{
				"foo":   "bar",
				"token": "s3cr3t-t0k3n",
			},
		},
	}
	cfg.Contexts["bravo-as-black-mage"] = &clientcmdapi.Context{
		Cluster:   "bravo",
		AuthInfo:  "black-mage-via-auth-provider",
		Namespace: "yankee",
	}
	cfg.Contexts["alfa-as-black-mage"] = &clientcmdapi.Context{
		Cluster:   "alfa",
		AuthInfo:  "black-mage-via-auth-provider",
		Namespace: "zulu",
	}
	cfg.Contexts["alfa-as-white-mage"] = &clientcmdapi.Context{
		Cluster:  "alfa",
		AuthInfo: "white-mage-via-cert",
	}
	cfg.CurrentContext = "alfa-as-white-mage"
	return cfg
}
