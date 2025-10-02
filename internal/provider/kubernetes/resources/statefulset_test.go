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

type statefulSetSuite struct {
	resourceSuite
	namespace         string
	statefulSetClient v1.StatefulSetInterface
}

var _ = gc.Suite(&statefulSetSuite{})

func (s *statefulSetSuite) SetUpTest(c *gc.C) {
	s.resourceSuite.SetUpTest(c)
	s.namespace = "ns1"
	s.statefulSetClient = s.client.AppsV1().StatefulSets(s.namespace)
}

func (s *statefulSetSuite) TestApply(c *gc.C) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sts1",
			Namespace: "test",
		},
	}
	// Create.
	stsResource := resources.NewStatefulSet(s.client.AppsV1().StatefulSets(sts.Namespace), "test", "sts1", sts)
	c.Assert(stsResource.Apply(context.TODO()), jc.ErrorIsNil)
	result, err := s.client.AppsV1().StatefulSets("test").Get(context.TODO(), "sts1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), gc.Equals, 0)

	// Update.
	sts.SetAnnotations(map[string]string{"a": "b"})
	stsResource = resources.NewStatefulSet(s.client.AppsV1().StatefulSets(sts.Namespace), "test", "sts1", sts)
	c.Assert(stsResource.Apply(context.TODO()), jc.ErrorIsNil)

	result, err = s.client.AppsV1().StatefulSets("test").Get(context.TODO(), "sts1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `sts1`)
	c.Assert(result.GetNamespace(), gc.Equals, `test`)
	c.Assert(result.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *statefulSetSuite) TestGet(c *gc.C) {
	template := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sts1",
			Namespace: "test",
		},
	}
	sts1 := template
	sts1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.AppsV1().StatefulSets("test").Create(context.TODO(), &sts1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	stsResource := resources.NewStatefulSet(s.client.AppsV1().StatefulSets(sts1.Namespace), "test", "sts1", &template)
	c.Assert(len(stsResource.GetAnnotations()), gc.Equals, 0)
	err = stsResource.Get(context.TODO())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stsResource.GetName(), gc.Equals, `sts1`)
	c.Assert(stsResource.GetNamespace(), gc.Equals, `test`)
	c.Assert(stsResource.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *statefulSetSuite) TestDelete(c *gc.C) {
	sts := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sts1",
			Namespace: "test",
		},
	}
	_, err := s.client.AppsV1().StatefulSets("test").Create(context.TODO(), &sts, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.client.AppsV1().StatefulSets("test").Get(context.TODO(), "sts1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `sts1`)

	stsResource := resources.NewStatefulSet(s.client.AppsV1().StatefulSets(sts.Namespace), "test", "sts1", &sts)
	err = stsResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIsNil)

	err = stsResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	err = stsResource.Get(context.TODO())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.client.AppsV1().StatefulSets("test").Get(context.TODO(), "sts1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}

func (s *statefulSetSuite) TestDeleteWithOrphan(c *gc.C) {
	sts := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sts1",
			Namespace: "test",
		},
	}
	_, err := s.client.AppsV1().StatefulSets("test").Create(context.TODO(), &sts, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.client.AppsV1().StatefulSets("test").Get(context.TODO(), "sts1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `sts1`)

	stsResource := resources.NewStatefulSet(s.client.AppsV1().StatefulSets(sts.Namespace), "test", "sts1", &sts)
	stsWithOrphanDeleteResource := resources.NewStatefulSetWithOrphanDelete(stsResource)
	err = stsWithOrphanDeleteResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIsNil)

	err = stsWithOrphanDeleteResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	err = stsWithOrphanDeleteResource.Get(context.TODO())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.client.AppsV1().StatefulSets("test").Get(context.TODO(), "sts1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}

func (s *statefulSetSuite) TestListStatefulSets(c *gc.C) {
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

	// Create sts1
	sts1Name := "sts1"
	sts1 := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   sts1Name,
			Labels: labelSet,
		},
	}
	_, err = s.statefulSetClient.Create(context.TODO(), sts1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Create sts2
	sts2Name := "sts2"
	sts2 := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   sts2Name,
			Labels: labelSet,
		},
	}
	_, err = s.statefulSetClient.Create(context.TODO(), sts2, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// List resources with correct labels.
	stses, err := resources.ListStatefulSets(context.Background(), s.statefulSetClient, s.namespace, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(stses), gc.Equals, 2)
	c.Assert(stses[0].GetName(), gc.Equals, sts1Name)
	c.Assert(stses[1].GetName(), gc.Equals, sts2Name)

	// List resources with no labels.
	stses, err = resources.ListStatefulSets(context.Background(), s.statefulSetClient, s.namespace, metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(stses), gc.Equals, 2)

	// List resources with wrong labels.
	stses, err = resources.ListStatefulSets(context.Background(), s.statefulSetClient, s.namespace, metav1.ListOptions{
		LabelSelector: "foo=bar",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(stses), gc.Equals, 0)
}
