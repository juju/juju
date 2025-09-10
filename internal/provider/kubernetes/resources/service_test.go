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

type serviceSuite struct {
	resourceSuite
	namespace     string
	serviceClient v1.ServiceInterface
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) SetUpTest(c *tc.C) {
	s.resourceSuite.SetUpTest(c)
	s.namespace = "ns1"
	s.serviceClient = s.client.CoreV1().Services(s.namespace)
}

func (s *serviceSuite) TestApply(c *tc.C) {
	ds := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	// Create.
	dsResource := resources.NewService(s.client.CoreV1().Services(ds.Namespace), "test", "ds1", ds)
	c.Assert(dsResource.Apply(c.Context()), tc.ErrorIsNil)
	result, err := s.client.CoreV1().Services("test").Get(c.Context(), "ds1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), tc.Equals, 0)

	// Update.
	ds.SetAnnotations(map[string]string{"a": "b"})
	dsResource = resources.NewService(s.client.CoreV1().Services(ds.Namespace), "test", "ds1", ds)
	c.Assert(dsResource.Apply(c.Context()), tc.ErrorIsNil)

	result, err = s.client.CoreV1().Services("test").Get(c.Context(), "ds1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `ds1`)
	c.Assert(result.GetNamespace(), tc.Equals, `test`)
	c.Assert(result.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *serviceSuite) TestGet(c *tc.C) {
	template := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	ds1 := template
	ds1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.CoreV1().Services("test").Create(c.Context(), &ds1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	dsResource := resources.NewService(s.client.CoreV1().Services(ds1.Namespace), "test", "ds1", &template)
	c.Assert(len(dsResource.GetAnnotations()), tc.Equals, 0)
	err = dsResource.Get(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dsResource.GetName(), tc.Equals, `ds1`)
	c.Assert(dsResource.GetNamespace(), tc.Equals, `test`)
	c.Assert(dsResource.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *serviceSuite) TestDelete(c *tc.C) {
	ds := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	_, err := s.client.CoreV1().Services("test").Create(c.Context(), &ds, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.client.CoreV1().Services("test").Get(c.Context(), "ds1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `ds1`)

	dsResource := resources.NewService(s.client.CoreV1().Services(ds.Namespace), "test", "ds1", &ds)
	err = dsResource.Delete(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	err = dsResource.Delete(c.Context())
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	err = dsResource.Get(c.Context())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	_, err = s.client.CoreV1().Services("test").Get(c.Context(), "ds1", metav1.GetOptions{})
	c.Assert(err, tc.Satisfies, k8serrors.IsNotFound)
}

func (s *serviceSuite) TestListServices(c *tc.C) {
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
	service1 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   service1Name,
			Labels: labelSet,
		},
	}
	_, err = s.serviceClient.Create(c.Context(), service1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// Create service2
	service2Name := "service2"
	service2 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   service2Name,
			Labels: labelSet,
		},
	}
	_, err = s.serviceClient.Create(c.Context(), service2, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// List resources with correct labels.
	services, err := resources.ListServices(context.Background(), s.serviceClient, s.namespace, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(services), tc.Equals, 2)
	c.Assert(services[0].GetName(), tc.Equals, service1Name)
	c.Assert(services[1].GetName(), tc.Equals, service2Name)

	// List resources with no labels.
	services, err = resources.ListServices(context.Background(), s.serviceClient, s.namespace, metav1.ListOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(services), tc.Equals, 2)

	// List resources with wrong labels.
	services, err = resources.ListServices(context.Background(), s.serviceClient, s.namespace, metav1.ListOptions{
		LabelSelector: "foo=bar",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(services), tc.Equals, 0)
}
