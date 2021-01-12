// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy_test

import (
	"context"

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
}

var _ = gc.Suite(&infoSuite{})

func (i *infoSuite) SetUpTest(c *gc.C) {
	i.client = fake.NewSimpleClientset()
	_, err := i.client.CoreV1().Namespaces().Create(context.TODO(),
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

	err := proxy.CreateControllerProxy(
		config,
		labels.Set{},
		i.client.CoreV1().ConfigMaps(testNamespace),
		i.client.RbacV1().Roles(testNamespace),
		i.client.RbacV1().RoleBindings(testNamespace),
		i.client.CoreV1().ServiceAccounts(testNamespace),
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

	err := proxy.CreateControllerProxy(
		config,
		labels.Set{},
		i.client.CoreV1().ConfigMaps(testNamespace),
		i.client.RbacV1().Roles(testNamespace),
		i.client.RbacV1().RoleBindings(testNamespace),
		i.client.CoreV1().ServiceAccounts(testNamespace),
	)
	c.Assert(err, jc.ErrorIsNil)

	_, err = i.client.CoreV1().Secrets(testNamespace).Create(
		context.TODO(),
		&core.Secret{
			ObjectMeta: meta.ObjectMeta{
				Name: "test-1234",
			},
			Data: map[string][]byte{
				"token":     []byte("iouwefbnuwefpo193923"),
				"namespace": []byte(testNamespace),
			},
			Type: core.SecretType("kubernetes.io/service-account-token"),
		},
		meta.CreateOptions{},
	)

	sa, err := i.client.CoreV1().ServiceAccounts(testNamespace).Get(
		context.TODO(),
		config.Name,
		meta.GetOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)

	sa.Secrets = append(sa.Secrets, core.ObjectReference{
		Name:      "test-1234",
		Namespace: testNamespace,
	})

	_, err = i.client.CoreV1().ServiceAccounts(testNamespace).Update(
		context.TODO(),
		sa,
		meta.UpdateOptions{},
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
