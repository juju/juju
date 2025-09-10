// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	appsv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/kubernetes/typed/apps/v1"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	providerutils "github.com/juju/juju/internal/provider/kubernetes/utils"
	"github.com/juju/juju/internal/uuid"
)

type daemonsetSuite struct {
	resourceSuite
	namespace       string
	daemonsetClient v1.DaemonSetInterface
}

func TestDaemonsetSuite(t *testing.T) {
	tc.Run(t, &daemonsetSuite{})
}

func (s *daemonsetSuite) SetUpTest(c *tc.C) {
	s.resourceSuite.SetUpTest(c)
	s.namespace = "ns1"
	s.daemonsetClient = s.client.AppsV1().DaemonSets(s.namespace)
}

func (s *daemonsetSuite) TestApply(c *tc.C) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	// Create.
	dsResource := resources.NewDaemonSet(s.client.AppsV1().DaemonSets(ds.Namespace), "test", "ds1", ds)
	c.Assert(dsResource.Apply(c.Context()), tc.ErrorIsNil)
	result, err := s.client.AppsV1().DaemonSets("test").Get(c.Context(), "ds1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), tc.Equals, 0)

	// Update.
	ds.SetAnnotations(map[string]string{"a": "b"})
	dsResource = resources.NewDaemonSet(s.client.AppsV1().DaemonSets(ds.Namespace), "test", "ds1", ds)
	c.Assert(dsResource.Apply(c.Context()), tc.ErrorIsNil)

	result, err = s.client.AppsV1().DaemonSets("test").Get(c.Context(), "ds1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `ds1`)
	c.Assert(result.GetNamespace(), tc.Equals, `test`)
	c.Assert(result.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *daemonsetSuite) TestGet(c *tc.C) {
	template := appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	ds1 := template
	ds1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.AppsV1().DaemonSets("test").Create(c.Context(), &ds1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	dsResource := resources.NewDaemonSet(s.client.AppsV1().DaemonSets(ds1.Namespace), "test", "ds1", &template)
	c.Assert(len(dsResource.GetAnnotations()), tc.Equals, 0)
	err = dsResource.Get(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dsResource.GetName(), tc.Equals, `ds1`)
	c.Assert(dsResource.GetNamespace(), tc.Equals, `test`)
	c.Assert(dsResource.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *daemonsetSuite) TestDelete(c *tc.C) {
	ds := appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	_, err := s.client.AppsV1().DaemonSets("test").Create(c.Context(), &ds, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.client.AppsV1().DaemonSets("test").Get(c.Context(), "ds1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `ds1`)

	dsResource := resources.NewDaemonSet(s.client.AppsV1().DaemonSets(ds.Namespace), "test", "ds1", &ds)
	err = dsResource.Delete(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	err = dsResource.Delete(c.Context())
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	err = dsResource.Get(c.Context())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	_, err = s.client.AppsV1().DaemonSets("test").Get(c.Context(), "ds1", metav1.GetOptions{})
	c.Assert(err, tc.Satisfies, k8serrors.IsNotFound)
}

func (s *daemonsetSuite) TestListDaemonSets(c *tc.C) {
	// Set up labels for model and app to list resource.
	controllerUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	modelUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	modelName := "testmodel"

	appName := "app1"
	appLabel := providerutils.SelectorLabelsForApp(appName, constants.LabelVersion2)

	modelLabel := providerutils.LabelsForModel(modelName, modelUUID.String(), controllerUUID.String(), constants.LabelVersion2)
	labelSet := providerutils.LabelsMerge(appLabel, modelLabel)

	// Create ds1.
	ds1Name := "ds1"
	ds1 := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   ds1Name,
			Labels: labelSet,
		},
	}
	_, err = s.daemonsetClient.Create(c.Context(), ds1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// Create ds2.
	ds2Name := "ds2"
	ds2 := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   ds2Name,
			Labels: labelSet,
		},
	}
	_, err = s.daemonsetClient.Create(c.Context(), ds2, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// List resources with correct labels.
	daemonsets, err := resources.ListDaemonSets(context.Background(), s.daemonsetClient, s.namespace, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(daemonsets), tc.Equals, 2)
	c.Assert(daemonsets[0].GetName(), tc.Equals, ds1Name)
	c.Assert(daemonsets[1].GetName(), tc.Equals, ds2Name)

	// List resources with no labels.
	daemonsets, err = resources.ListDaemonSets(context.Background(), s.daemonsetClient, s.namespace, metav1.ListOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(daemonsets), tc.Equals, 2)

	// List resources with wrong labels.
	daemonsets, err = resources.ListDaemonSets(context.Background(), s.daemonsetClient, s.namespace, metav1.ListOptions{
		LabelSelector: "foo=bar",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(daemonsets), tc.Equals, 0)
}
