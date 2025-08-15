// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/provider/resources"
)

type serviceAccountSuite struct {
	resourceSuite
}

var _ = gc.Suite(&serviceAccountSuite{})

func (s *serviceAccountSuite) TestApply(c *gc.C) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sa1",
			Namespace: "test",
		},
	}
	// Create.
	saResource := resources.NewServiceAccount(s.client.CoreV1().ServiceAccounts(sa.Namespace), "test", "sa1", sa)
	c.Assert(saResource.Apply(context.TODO()), jc.ErrorIsNil)
	result, err := s.client.CoreV1().ServiceAccounts("test").Get(context.TODO(), "sa1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), gc.Equals, 0)

	// Update.
	sa.SetAnnotations(map[string]string{"a": "b"})
	saResource = resources.NewServiceAccount(s.client.CoreV1().ServiceAccounts(sa.Namespace), "test", "sa1", sa)
	c.Assert(saResource.Apply(context.TODO()), jc.ErrorIsNil)

	result, err = s.client.CoreV1().ServiceAccounts("test").Get(context.TODO(), "sa1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `sa1`)
	c.Assert(result.GetNamespace(), gc.Equals, `test`)
	c.Assert(result.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *serviceAccountSuite) TestGet(c *gc.C) {
	template := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sa1",
			Namespace: "test",
		},
	}
	sa1 := template
	sa1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.CoreV1().ServiceAccounts("test").Create(context.TODO(), &sa1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	saResource := resources.NewServiceAccount(s.client.CoreV1().ServiceAccounts(sa1.Namespace), "test", "sa1", &template)
	c.Assert(len(saResource.GetAnnotations()), gc.Equals, 0)
	err = saResource.Get(context.TODO())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(saResource.GetName(), gc.Equals, `sa1`)
	c.Assert(saResource.GetNamespace(), gc.Equals, `test`)
	c.Assert(saResource.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *serviceAccountSuite) TestDelete(c *gc.C) {
	sa := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sa1",
			Namespace: "test",
		},
	}
	_, err := s.client.CoreV1().ServiceAccounts("test").Create(context.TODO(), &sa, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.client.CoreV1().ServiceAccounts("test").Get(context.TODO(), "sa1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `sa1`)

	saResource := resources.NewServiceAccount(s.client.CoreV1().ServiceAccounts(sa.Namespace), "test", "sa1", &sa)
	err = saResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIsNil)

	err = saResource.Get(context.TODO())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.client.CoreV1().ServiceAccounts("test").Get(context.TODO(), "sa1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}

func (s *serviceAccountSuite) TestUpdate(c *gc.C) {
	sa := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sa1",
			Namespace: "test",
		},
	}
	_, err := s.client.CoreV1().ServiceAccounts("test").Create(
		context.TODO(),
		&sa,
		metav1.CreateOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)

	sa.ObjectMeta.Labels = map[string]string{
		"test": "label",
	}

	saResource := resources.NewServiceAccount(s.client.CoreV1().ServiceAccounts(sa.Namespace), "test", "sa1", &sa)
	err = saResource.Update(context.TODO())
	c.Assert(err, jc.ErrorIsNil)

	rsa, err := s.client.CoreV1().ServiceAccounts("test").Get(
		context.TODO(),
		"sa1",
		metav1.GetOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rsa, jc.DeepEquals, &saResource.ServiceAccount)
}

func (s *serviceAccountSuite) TestEnsureCreatesNew(c *gc.C) {
	sa := resources.NewServiceAccount(s.client.CoreV1().ServiceAccounts("test"), "test", "sa1", &corev1.ServiceAccount{})
	cleanups, err := sa.Ensure(context.TODO())
	c.Assert(err, jc.ErrorIsNil)

	obj, err := s.client.CoreV1().ServiceAccounts("test").Get(
		context.TODO(), "sa1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&sa.ServiceAccount, jc.DeepEquals, obj)

	for _, v := range cleanups {
		v()
	}

	// Test cleanup removes service account
	_, err = s.client.CoreV1().ServiceAccounts("test").Get(
		context.TODO(), "sa1", metav1.GetOptions{})
	c.Assert(k8serrors.IsNotFound(err), jc.IsTrue)
}

func (s *serviceAccountSuite) TestEnsureUpdates(c *gc.C) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sa2",
			Namespace: "testing",
		},
	}

	_, err := s.client.CoreV1().ServiceAccounts("testing").Create(
		context.TODO(), sa, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	sa.ObjectMeta.Labels = map[string]string{
		"test": "case",
	}

	resource := resources.NewServiceAccount(s.client.CoreV1().ServiceAccounts("testing"), "testing", "sa2", sa)
	_, err = resource.Ensure(context.TODO())
	c.Assert(err, jc.ErrorIsNil)

	obj, err := s.client.CoreV1().ServiceAccounts("testing").Get(
		context.TODO(), sa.Name, metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(obj, jc.DeepEquals, &resource.ServiceAccount)
}
