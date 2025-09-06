// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"
	appsv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/kubernetes/typed/apps/v1"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	providerutils "github.com/juju/juju/internal/provider/kubernetes/utils"
)

type daemonsetSuite struct {
	resourceSuite
	namespace       string
	daemonsetClient v1.DaemonSetInterface
}

var _ = gc.Suite(&daemonsetSuite{})

func (s *daemonsetSuite) SetUpTest(c *gc.C) {
	s.resourceSuite.SetUpTest(c)
	s.namespace = "ns1"
	s.daemonsetClient = s.client.AppsV1().DaemonSets(s.namespace)
}

func (s *daemonsetSuite) TestApply(c *gc.C) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	// Create.
	dsResource := resources.NewDaemonSet(s.client.AppsV1().DaemonSets(ds.Namespace), "test", "ds1", ds)
	c.Assert(dsResource.Apply(context.TODO()), jc.ErrorIsNil)
	result, err := s.client.AppsV1().DaemonSets("test").Get(context.TODO(), "ds1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), gc.Equals, 0)

	// Update.
	ds.SetAnnotations(map[string]string{"a": "b"})
	dsResource = resources.NewDaemonSet(s.client.AppsV1().DaemonSets(ds.Namespace), "test", "ds1", ds)
	c.Assert(dsResource.Apply(context.TODO()), jc.ErrorIsNil)

	result, err = s.client.AppsV1().DaemonSets("test").Get(context.TODO(), "ds1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `ds1`)
	c.Assert(result.GetNamespace(), gc.Equals, `test`)
	c.Assert(result.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *daemonsetSuite) TestGet(c *gc.C) {
	template := appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	ds1 := template
	ds1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.AppsV1().DaemonSets("test").Create(context.TODO(), &ds1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	dsResource := resources.NewDaemonSet(s.client.AppsV1().DaemonSets(ds1.Namespace), "test", "ds1", &template)
	c.Assert(len(dsResource.GetAnnotations()), gc.Equals, 0)
	err = dsResource.Get(context.TODO())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dsResource.GetName(), gc.Equals, `ds1`)
	c.Assert(dsResource.GetNamespace(), gc.Equals, `test`)
	c.Assert(dsResource.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *daemonsetSuite) TestDelete(c *gc.C) {
	ds := appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	_, err := s.client.AppsV1().DaemonSets("test").Create(context.TODO(), &ds, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.client.AppsV1().DaemonSets("test").Get(context.TODO(), "ds1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `ds1`)

	dsResource := resources.NewDaemonSet(s.client.AppsV1().DaemonSets(ds.Namespace), "test", "ds1", &ds)
	err = dsResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIsNil)

	err = dsResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	err = dsResource.Get(context.TODO())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.client.AppsV1().DaemonSets("test").Get(context.TODO(), "ds1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}

func (s *daemonsetSuite) TestListDaemonSets(c *gc.C) {
	// Set up labels for model and app to list resource.
	controllerUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	modelUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

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
	_, err = s.daemonsetClient.Create(context.TODO(), ds1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Create ds2.
	ds2Name := "ds2"
	ds2 := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   ds2Name,
			Labels: labelSet,
		},
	}
	_, err = s.daemonsetClient.Create(context.TODO(), ds2, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// List resources with correct labels.
	daemonsets, err := resources.ListDaemonSets(context.Background(), s.daemonsetClient, s.namespace, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(daemonsets), gc.Equals, 2)
	c.Assert(daemonsets[0].GetName(), gc.Equals, ds1Name)
	c.Assert(daemonsets[1].GetName(), gc.Equals, ds2Name)

	// List resources with no labels.
	daemonsets, err = resources.ListDaemonSets(context.Background(), s.daemonsetClient, s.namespace, metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(daemonsets), gc.Equals, 2)

	// List resources with wrong labels.
	daemonsets, err = resources.ListDaemonSets(context.Background(), s.daemonsetClient, s.namespace, metav1.ListOptions{
		LabelSelector: "foo=bar",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(daemonsets), gc.Equals, 0)
}
