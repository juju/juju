// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy_test

import (
	"encoding/json"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/juju/juju/caas/kubernetes/provider/proxy"
)

type setupSuite struct {
	client *fake.Clientset
	clock  *testclock.Clock
}

var (
	_             = tc.Suite(&setupSuite{})
	testNamespace = "test"
)

func (s *setupSuite) SetUpTest(c *tc.C) {
	s.clock = testclock.NewClock(time.Time{})
	s.client = fake.NewSimpleClientset()
	_, err := s.client.CoreV1().Namespaces().Create(c.Context(),
		&core.Namespace{
			ObjectMeta: meta.ObjectMeta{
				Name: testNamespace,
			},
		},
		meta.CreateOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *setupSuite) TestProxyObjCreation(c *tc.C) {
	config := proxy.ControllerProxyConfig{
		Name:          "controller-proxy",
		Namespace:     testNamespace,
		RemotePort:    "17707",
		TargetService: "controller-service",
	}

	// fake k8s client does not populate the token for secret, so we have to do it manually.
	_, err := s.client.CoreV1().Secrets(testNamespace).Create(c.Context(), &core.Secret{
		ObjectMeta: meta.ObjectMeta{
			Labels: labels.Set{},
			Name:   config.Name,
			Annotations: map[string]string{
				core.ServiceAccountNameKey: config.Name,
			},
		},
		Type: core.SecretTypeServiceAccountToken,
		Data: map[string][]byte{
			core.ServiceAccountTokenKey: []byte("token"),
		},
	}, meta.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)
	err = proxy.CreateControllerProxy(
		c.Context(),
		config,
		labels.Set{},
		s.clock,
		s.client.CoreV1().ConfigMaps(testNamespace),
		s.client.RbacV1().Roles(testNamespace),
		s.client.RbacV1().RoleBindings(testNamespace),
		s.client.CoreV1().ServiceAccounts(testNamespace),
		s.client.CoreV1().Secrets(testNamespace),
	)
	c.Assert(err, tc.ErrorIsNil)

	role, err := s.client.RbacV1().Roles(testNamespace).Get(
		c.Context(),
		config.Name,
		meta.GetOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(role.Name, tc.Equals, config.Name)
	c.Assert(role.Rules[0].Resources, tc.DeepEquals, []string{"pods"})
	c.Assert(role.Rules[0].Verbs, tc.DeepEquals, []string{"list", "get", "watch"})
	c.Assert(role.Rules[1].Resources, tc.DeepEquals, []string{"services"})
	c.Assert(role.Rules[1].Verbs, tc.DeepEquals, []string{"get"})
	c.Assert(role.Rules[2].Resources, tc.DeepEquals, []string{"pods/portforward"})
	c.Assert(role.Rules[2].Verbs, tc.DeepEquals, []string{"create", "get"})

	sa, err := s.client.CoreV1().ServiceAccounts(testNamespace).Get(
		c.Context(),
		config.Name,
		meta.GetOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sa.Name, tc.Equals, config.Name)
	c.Assert(len(sa.Secrets), tc.Equals, 1)
	c.Assert(sa.Secrets[0].Name, tc.Equals, config.Name)

	secret, err := s.client.CoreV1().ServiceAccounts(testNamespace).Get(
		c.Context(),
		config.Name,
		meta.GetOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(secret.Name, tc.Equals, config.Name)

	roleBinding, err := s.client.RbacV1().RoleBindings(testNamespace).Get(
		c.Context(),
		config.Name,
		meta.GetOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(roleBinding.Name, tc.Equals, config.Name)

	cm, err := s.client.CoreV1().ConfigMaps(testNamespace).Get(
		c.Context(),
		config.Name,
		meta.GetOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cm.Name, tc.Equals, config.Name)
}

func (s *setupSuite) TestProxyConfigMap(c *tc.C) {
	config := proxy.ControllerProxyConfig{
		Name:          "controller-proxy",
		Namespace:     testNamespace,
		RemotePort:    "17707",
		TargetService: "controller-service",
	}

	// fake k8sclient does not populate the token for secret, so we have to do it manually.
	_, err := s.client.CoreV1().Secrets(testNamespace).Create(c.Context(), &core.Secret{
		ObjectMeta: meta.ObjectMeta{
			Labels: labels.Set{},
			Name:   config.Name,
			Annotations: map[string]string{
				core.ServiceAccountNameKey: config.Name,
			},
		},
		Type: core.SecretTypeServiceAccountToken,
		Data: map[string][]byte{
			core.ServiceAccountTokenKey: []byte("token"),
		},
	}, meta.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)
	err = proxy.CreateControllerProxy(
		c.Context(),
		config,
		labels.Set{},
		s.clock,
		s.client.CoreV1().ConfigMaps(testNamespace),
		s.client.RbacV1().Roles(testNamespace),
		s.client.RbacV1().RoleBindings(testNamespace),
		s.client.CoreV1().ServiceAccounts(testNamespace),
		s.client.CoreV1().Secrets(testNamespace),
	)
	c.Assert(err, tc.ErrorIsNil)

	cm, err := s.client.CoreV1().ConfigMaps(testNamespace).Get(
		c.Context(),
		config.Name,
		meta.GetOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)

	fetchedConfig := proxy.ControllerProxyConfig{}
	configJson := cm.Data[proxy.ProxyConfigMapKey]
	err = json.Unmarshal([]byte(configJson), &fetchedConfig)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(fetchedConfig, tc.DeepEquals, config)
}
