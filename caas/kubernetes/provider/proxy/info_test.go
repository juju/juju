// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy_test

import (
	"context"
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

type infoSuite struct {
	client *fake.Clientset
	clock  *testclock.Clock
}

var _ = gc.Suite(&infoSuite{})

func (i *infoSuite) SetUpTest(c *gc.C) {
	i.clock = testclock.NewClock(time.Time{})

	i.client = fake.NewSimpleClientset()
	_, err := i.client.CoreV1().Namespaces().Create(context.Background(),
		&core.Namespace{
			ObjectMeta: meta.ObjectMeta{
				Name: testNamespace,
			},
		},
		meta.CreateOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (i *infoSuite) TestHasControllerProxyFalse(c *gc.C) {
	has, err := proxy.HasControllerProxy("test",
		i.client.CoreV1().ConfigMaps(testNamespace),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(has, jc.IsFalse)
}

func (i *infoSuite) TestHasControllerProxy(c *gc.C) {
	config := proxy.ControllerProxyConfig{
		Name:          "controller-proxy",
		Namespace:     testNamespace,
		RemotePort:    "17707",
		TargetService: "controller-service",
	}

	// fake k8s client does not populate the token for secret, so we have to do it manually.
	_, err := i.client.CoreV1().Secrets(testNamespace).Create(context.Background(), &core.Secret{
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
		i.clock,
		i.client.CoreV1().ConfigMaps(testNamespace),
		i.client.RbacV1().Roles(testNamespace),
		i.client.RbacV1().RoleBindings(testNamespace),
		i.client.CoreV1().ServiceAccounts(testNamespace),
		i.client.CoreV1().Secrets(testNamespace),
	)
	c.Assert(err, jc.ErrorIsNil)

	has, err := proxy.HasControllerProxy(config.Name,
		i.client.CoreV1().ConfigMaps(testNamespace),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(has, jc.IsTrue)
}

func (i *infoSuite) TestGetControllerProxier(c *gc.C) {
	config := proxy.ControllerProxyConfig{
		Name:          "controller-proxy",
		Namespace:     testNamespace,
		RemotePort:    "17707",
		TargetService: "controller-service",
	}

	// fake k8s client does not populate the token for secret, so we have to do it manually.
	_, err := i.client.CoreV1().Secrets(testNamespace).Create(context.Background(), &core.Secret{
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
		i.clock,
		i.client.CoreV1().ConfigMaps(testNamespace),
		i.client.RbacV1().Roles(testNamespace),
		i.client.RbacV1().RoleBindings(testNamespace),
		i.client.CoreV1().ServiceAccounts(testNamespace),
		i.client.CoreV1().Secrets(testNamespace),
	)
	c.Assert(err, jc.ErrorIsNil)

	_, err = proxy.GetControllerProxy(
		config.Name,
		"https://localhost:8123",
		i.client.CoreV1().ConfigMaps(testNamespace),
		i.client.CoreV1().ServiceAccounts(testNamespace),
		i.client.CoreV1().Secrets(testNamespace),
	)
	c.Assert(err, jc.ErrorIsNil)
}
