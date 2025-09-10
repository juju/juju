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

type statefulSetSuite struct {
	resourceSuite
	namespace         string
	statefulSetClient v1.StatefulSetInterface
}

func TestStatefulSetSuite(t *testing.T) {
	tc.Run(t, &statefulSetSuite{})
}

func (s *statefulSetSuite) SetUpTest(c *tc.C) {
	s.resourceSuite.SetUpTest(c)
	s.namespace = "ns1"
	s.statefulSetClient = s.client.AppsV1().StatefulSets(s.namespace)
}

func (s *statefulSetSuite) TestApply(c *tc.C) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sts1",
			Namespace: "test",
		},
	}
	// Create.
	stsResource := resources.NewStatefulSet(s.client.AppsV1().StatefulSets(sts.Namespace), "test", "sts1", sts)
	c.Assert(stsResource.Apply(c.Context()), tc.ErrorIsNil)
	result, err := s.client.AppsV1().StatefulSets("test").Get(c.Context(), "sts1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), tc.Equals, 0)

	// Update.
	sts.SetAnnotations(map[string]string{"a": "b"})
	stsResource = resources.NewStatefulSet(s.client.AppsV1().StatefulSets(sts.Namespace), "test", "sts1", sts)
	c.Assert(stsResource.Apply(c.Context()), tc.ErrorIsNil)

	result, err = s.client.AppsV1().StatefulSets("test").Get(c.Context(), "sts1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `sts1`)
	c.Assert(result.GetNamespace(), tc.Equals, `test`)
	c.Assert(result.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *statefulSetSuite) TestGet(c *tc.C) {
	template := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sts1",
			Namespace: "test",
		},
	}
	sts1 := template
	sts1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.AppsV1().StatefulSets("test").Create(c.Context(), &sts1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	stsResource := resources.NewStatefulSet(s.client.AppsV1().StatefulSets(sts1.Namespace), "test", "sts1", &template)
	c.Assert(len(stsResource.GetAnnotations()), tc.Equals, 0)
	err = stsResource.Get(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(stsResource.GetName(), tc.Equals, `sts1`)
	c.Assert(stsResource.GetNamespace(), tc.Equals, `test`)
	c.Assert(stsResource.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *statefulSetSuite) TestDelete(c *tc.C) {
	sts := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sts1",
			Namespace: "test",
		},
	}
	_, err := s.client.AppsV1().StatefulSets("test").Create(c.Context(), &sts, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.client.AppsV1().StatefulSets("test").Get(c.Context(), "sts1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `sts1`)

	stsResource := resources.NewStatefulSet(s.client.AppsV1().StatefulSets(sts.Namespace), "test", "sts1", &sts)
	err = stsResource.Delete(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	err = stsResource.Delete(c.Context())
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	err = stsResource.Get(c.Context())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	_, err = s.client.AppsV1().StatefulSets("test").Get(c.Context(), "sts1", metav1.GetOptions{})
	c.Assert(err, tc.Satisfies, k8serrors.IsNotFound)
}

func (s *statefulSetSuite) TestListStatefulSets(c *tc.C) {
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

	// Create sts1
	sts1Name := "sts1"
	sts1 := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   sts1Name,
			Labels: labelSet,
		},
	}
	_, err = s.statefulSetClient.Create(c.Context(), sts1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// Create sts2
	sts2Name := "sts2"
	sts2 := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   sts2Name,
			Labels: labelSet,
		},
	}
	_, err = s.statefulSetClient.Create(c.Context(), sts2, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// List resources with correct labels.
	stses, err := resources.ListStatefulSets(context.Background(), s.statefulSetClient, s.namespace, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(stses), tc.Equals, 2)
	c.Assert(stses[0].GetName(), tc.Equals, sts1Name)
	c.Assert(stses[1].GetName(), tc.Equals, sts2Name)

	// List resources with no labels.
	stses, err = resources.ListStatefulSets(context.Background(), s.statefulSetClient, s.namespace, metav1.ListOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(stses), tc.Equals, 2)

	// List resources with wrong labels.
	stses, err = resources.ListStatefulSets(context.Background(), s.statefulSetClient, s.namespace, metav1.ListOptions{
		LabelSelector: "foo=bar",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(stses), tc.Equals, 0)
}
