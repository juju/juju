// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy_test

import (
	"context"
	"encoding/json"
	"time"

	"github.com/juju/clock/testclock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
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
	_             = gc.Suite(&setupSuite{})
	testNamespace = "test"
)

func (s *setupSuite) SetUpTest(c *gc.C) {
	s.clock = testclock.NewClock(time.Time{})
	s.client = fake.NewSimpleClientset()
	_, err := s.client.CoreV1().Namespaces().Create(context.TODO(),
		&core.Namespace{
			ObjectMeta: meta.ObjectMeta{
				Name: testNamespace,
			},
		},
		meta.CreateOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *setupSuite) TestProxyObjCreation(c *gc.C) {
	config := proxy.ControllerProxyConfig{
		Name:          "controller-proxy",
		Namespace:     testNamespace,
		RemotePort:    "17707",
		TargetService: "controller-service",
	}

	// fake k8s client does not populate the token for secret, so we have to do it manually.
	_, err := s.client.CoreV1().Secrets(testNamespace).Create(context.TODO(), &core.Secret{
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
	c.Assert(err, jc.ErrorIsNil)
	err = proxy.CreateControllerProxy(
		context.Background(),
		config,
		labels.Set{},
		s.clock,
		s.client.CoreV1().ConfigMaps(testNamespace),
		s.client.RbacV1().Roles(testNamespace),
		s.client.RbacV1().RoleBindings(testNamespace),
		s.client.CoreV1().ServiceAccounts(testNamespace),
		s.client.CoreV1().Secrets(testNamespace),
	)
	c.Assert(err, jc.ErrorIsNil)

	role, err := s.client.RbacV1().Roles(testNamespace).Get(
		context.TODO(),
		config.Name,
		meta.GetOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(role.Name, gc.Equals, config.Name)
	c.Assert(role.Rules[0].Resources, jc.DeepEquals, []string{"pods"})
	c.Assert(role.Rules[0].Verbs, jc.DeepEquals, []string{"list", "get", "watch"})
	c.Assert(role.Rules[1].Resources, jc.DeepEquals, []string{"services"})
	c.Assert(role.Rules[1].Verbs, jc.DeepEquals, []string{"get"})
	c.Assert(role.Rules[2].Resources, jc.DeepEquals, []string{"pods/portforward"})
	c.Assert(role.Rules[2].Verbs, jc.DeepEquals, []string{"create", "get"})

	sa, err := s.client.CoreV1().ServiceAccounts(testNamespace).Get(
		context.TODO(),
		config.Name,
		meta.GetOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sa.Name, gc.Equals, config.Name)
	c.Assert(len(sa.Secrets), gc.Equals, 1)
	c.Assert(sa.Secrets[0].Name, gc.Equals, config.Name)

	secret, err := s.client.CoreV1().ServiceAccounts(testNamespace).Get(
		context.TODO(),
		config.Name,
		meta.GetOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(secret.Name, gc.Equals, config.Name)

	roleBinding, err := s.client.RbacV1().RoleBindings(testNamespace).Get(
		context.TODO(),
		config.Name,
		meta.GetOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(roleBinding.Name, gc.Equals, config.Name)

	cm, err := s.client.CoreV1().ConfigMaps(testNamespace).Get(
		context.TODO(),
		config.Name,
		meta.GetOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cm.Name, gc.Equals, config.Name)
}

func (s *setupSuite) TestProxyConfigMap(c *gc.C) {
	config := proxy.ControllerProxyConfig{
		Name:          "controller-proxy",
		Namespace:     testNamespace,
		RemotePort:    "17707",
		TargetService: "controller-service",
	}

	// fake k8sclient does not populate the token for secret, so we have to do it manually.
	_, err := s.client.CoreV1().Secrets(testNamespace).Create(context.TODO(), &core.Secret{
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
	c.Assert(err, jc.ErrorIsNil)
	err = proxy.CreateControllerProxy(
		context.Background(),
		config,
		labels.Set{},
		s.clock,
		s.client.CoreV1().ConfigMaps(testNamespace),
		s.client.RbacV1().Roles(testNamespace),
		s.client.RbacV1().RoleBindings(testNamespace),
		s.client.CoreV1().ServiceAccounts(testNamespace),
		s.client.CoreV1().Secrets(testNamespace),
	)
	c.Assert(err, jc.ErrorIsNil)

	cm, err := s.client.CoreV1().ConfigMaps(testNamespace).Get(
		context.TODO(),
		config.Name,
		meta.GetOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)

	fetchedConfig := proxy.ControllerProxyConfig{}
	configJson := cm.Data[proxy.ProxyConfigMapKey]
	err = json.Unmarshal([]byte(configJson), &fetchedConfig)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(fetchedConfig, jc.DeepEquals, config)
}
