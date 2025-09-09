// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	providerutils "github.com/juju/juju/internal/provider/kubernetes/utils"
	"github.com/juju/juju/internal/uuid"
)

type serviceAccountSuite struct {
	resourceSuite
	namespace            string
	serviceAccountClient v1.ServiceAccountInterface
}

func TestServiceAccountSuite(t *testing.T) {
	tc.Run(t, &serviceAccountSuite{})
}

func (s *serviceAccountSuite) SetUpTest(c *tc.C) {
	s.resourceSuite.SetUpTest(c)
	s.namespace = "ns1"
	s.serviceAccountClient = s.client.CoreV1().ServiceAccounts(s.namespace)
}

func (s *serviceAccountSuite) TestApply(c *tc.C) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sa1",
			Namespace: "test",
		},
	}
	// Create.
	saResource := resources.NewServiceAccount(s.client.CoreV1().ServiceAccounts(sa.Namespace), "test", "sa1", sa)
	c.Assert(saResource.Apply(c.Context()), tc.ErrorIsNil)
	result, err := s.client.CoreV1().ServiceAccounts("test").Get(c.Context(), "sa1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), tc.Equals, 0)

	// Update.
	sa.SetAnnotations(map[string]string{"a": "b"})
	saResource = resources.NewServiceAccount(s.client.CoreV1().ServiceAccounts(sa.Namespace), "test", "sa1", sa)
	c.Assert(saResource.Apply(c.Context()), tc.ErrorIsNil)

	result, err = s.client.CoreV1().ServiceAccounts("test").Get(c.Context(), "sa1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `sa1`)
	c.Assert(result.GetNamespace(), tc.Equals, `test`)
	c.Assert(result.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *serviceAccountSuite) TestGet(c *tc.C) {
	template := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sa1",
			Namespace: "test",
		},
	}
	sa1 := template
	sa1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.CoreV1().ServiceAccounts("test").Create(c.Context(), &sa1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	saResource := resources.NewServiceAccount(s.client.CoreV1().ServiceAccounts(sa1.Namespace), "test", "sa1", &template)
	c.Assert(len(saResource.GetAnnotations()), tc.Equals, 0)
	err = saResource.Get(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(saResource.GetName(), tc.Equals, `sa1`)
	c.Assert(saResource.GetNamespace(), tc.Equals, `test`)
	c.Assert(saResource.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *serviceAccountSuite) TestDelete(c *tc.C) {
	sa := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sa1",
			Namespace: "test",
		},
	}
	_, err := s.client.CoreV1().ServiceAccounts("test").Create(c.Context(), &sa, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.client.CoreV1().ServiceAccounts("test").Get(c.Context(), "sa1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `sa1`)

	saResource := resources.NewServiceAccount(s.client.CoreV1().ServiceAccounts(sa.Namespace), "test", "sa1", &sa)
	err = saResource.Delete(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	err = saResource.Delete(c.Context())
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	err = saResource.Get(c.Context())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	_, err = s.client.CoreV1().ServiceAccounts("test").Get(c.Context(), "sa1", metav1.GetOptions{})
	c.Assert(err, tc.Satisfies, k8serrors.IsNotFound)
}

func (s *serviceAccountSuite) TestUpdate(c *tc.C) {
	sa := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sa1",
			Namespace: "test",
		},
	}
	_, err := s.client.CoreV1().ServiceAccounts("test").Create(
		c.Context(),
		&sa,
		metav1.CreateOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)

	sa.ObjectMeta.Labels = map[string]string{
		"test": "label",
	}

	saResource := resources.NewServiceAccount(s.client.CoreV1().ServiceAccounts(sa.Namespace), "test", "sa1", &sa)
	err = saResource.Update(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	rsa, err := s.client.CoreV1().ServiceAccounts("test").Get(
		c.Context(),
		"sa1",
		metav1.GetOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rsa, tc.DeepEquals, &saResource.ServiceAccount)
}

func (s *serviceAccountSuite) TestEnsureCreatesNew(c *tc.C) {
	sa := resources.NewServiceAccount(s.client.CoreV1().ServiceAccounts("test"), "test", "sa1", &corev1.ServiceAccount{})
	cleanups, err := sa.Ensure(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	obj, err := s.client.CoreV1().ServiceAccounts("test").Get(
		c.Context(), "sa1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(&sa.ServiceAccount, tc.DeepEquals, obj)

	for _, v := range cleanups {
		v()
	}

	// Test cleanup removes service account
	_, err = s.client.CoreV1().ServiceAccounts("test").Get(
		c.Context(), "sa1", metav1.GetOptions{})
	c.Assert(k8serrors.IsNotFound(err), tc.IsTrue)
}

func (s *serviceAccountSuite) TestEnsureUpdates(c *tc.C) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sa2",
			Namespace: "testing",
		},
	}

	_, err := s.client.CoreV1().ServiceAccounts("testing").Create(
		c.Context(), sa, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	sa.ObjectMeta.Labels = map[string]string{
		"test": "case",
	}

	resource := resources.NewServiceAccount(s.client.CoreV1().ServiceAccounts("testing"), "testing", "sa2", sa)
	_, err = resource.Ensure(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	obj, err := s.client.CoreV1().ServiceAccounts("testing").Get(
		c.Context(), sa.Name, metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(obj, tc.DeepEquals, &resource.ServiceAccount)
}

func (s *serviceAccountSuite) TestListServiceAccounts(c *tc.C) {
	// Set up labels for model and app to list resource
	controllerUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	modelUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

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
	_, err = s.serviceAccountClient.Create(c.Context(), service1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// Create service2
	service2Name := "service2"
	service2 := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:   service2Name,
			Labels: labelSet,
		},
	}
	_, err = s.serviceAccountClient.Create(c.Context(), service2, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// List resources with correct labels.
	services, err := resources.ListServiceAccounts(context.Background(), s.serviceAccountClient, s.namespace, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(services), tc.Equals, 2)
	c.Assert(services[0].GetName(), tc.Equals, service1Name)
	c.Assert(services[1].GetName(), tc.Equals, service2Name)

	// List resources with no labels.
	services, err = resources.ListServiceAccounts(context.Background(), s.serviceAccountClient, s.namespace, metav1.ListOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(services), tc.Equals, 2)

	// List resources with wrong labels.
	services, err = resources.ListServiceAccounts(context.Background(), s.serviceAccountClient, s.namespace, metav1.ListOptions{
		LabelSelector: "foo=bar",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(services), tc.Equals, 0)
}
