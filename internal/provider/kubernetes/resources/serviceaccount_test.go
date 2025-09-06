// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	providerutils "github.com/juju/juju/internal/provider/kubernetes/utils"
)

type serviceAccountSuite struct {
	resourceSuite
	namespace            string
	serviceAccountClient v1.ServiceAccountInterface
}

var _ = gc.Suite(&serviceAccountSuite{})

func (s *serviceAccountSuite) SetUpTest(c *gc.C) {
	s.resourceSuite.SetUpTest(c)
	s.namespace = "ns1"
	s.serviceAccountClient = s.client.CoreV1().ServiceAccounts(s.namespace)
}

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

	err = saResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIs, errors.NotFound)

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

func (s *serviceAccountSuite) TestListServiceAccounts(c *gc.C) {
	// Set up labels for model and app to list resource
	controllerUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	modelUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	modelName := "testmodel"

	appName := "app1"
	appLabel := providerutils.SelectorLabelsForApp(appName, constants.LabelVersion2)

	modelLabel := providerutils.LabelsForModel(modelName, modelUUID.String(), controllerUUID.String(), constants.LabelVersion2)
	labelSet := providerutils.LabelsMerge(appLabel, modelLabel)

	// Create service1
	service1Name := "service1"
	service1 := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:   service1Name,
			Labels: labelSet,
		},
	}
	_, err = s.serviceAccountClient.Create(context.TODO(), service1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Create service2
	service2Name := "service2"
	service2 := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:   service2Name,
			Labels: labelSet,
		},
	}
	_, err = s.serviceAccountClient.Create(context.TODO(), service2, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// List resources with correct labels.
	services, err := resources.ListServiceAccounts(context.Background(), s.serviceAccountClient, s.namespace, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(services), gc.Equals, 2)
	c.Assert(services[0].GetName(), gc.Equals, service1Name)
	c.Assert(services[1].GetName(), gc.Equals, service2Name)

	// List resources with no labels.
	services, err = resources.ListServiceAccounts(context.Background(), s.serviceAccountClient, s.namespace, metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(services), gc.Equals, 2)

	// List resources with wrong labels.
	services, err = resources.ListServiceAccounts(context.Background(), s.serviceAccountClient, s.namespace, metav1.ListOptions{
		LabelSelector: "foo=bar",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(services), gc.Equals, 0)
}
