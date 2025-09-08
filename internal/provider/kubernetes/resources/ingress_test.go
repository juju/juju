// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"
	netv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	netv1client "k8s.io/client-go/kubernetes/typed/networking/v1"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/resources"
	providerutils "github.com/juju/juju/caas/kubernetes/provider/utils"
)

type ingressSuite struct {
	resourceSuite
	namespace     string
	ingressClient netv1client.IngressInterface
}

var _ = gc.Suite(&ingressSuite{})

func (s *ingressSuite) SetUpTest(c *gc.C) {
	s.resourceSuite.SetUpTest(c)
	s.namespace = "ns1"
	s.ingressClient = s.client.NetworkingV1().Ingresses(s.namespace)
}

func (s *ingressSuite) TestApply(c *gc.C) {
	ig := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ig1",
		},
	}

	// Create.
	res := resources.NewIngress(s.ingressClient, "ig1", ig)
	c.Assert(res.Apply(context.TODO()), jc.ErrorIsNil)

	// Get.
	result, err := s.ingressClient.Get(context.TODO(), "ig1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), gc.Equals, 0)

	// Apply
	ig.SetAnnotations(map[string]string{"a": "b"})
	res = resources.NewIngress(s.ingressClient, "ig1", ig)
	c.Assert(res.Apply(context.TODO()), jc.ErrorIsNil)

	// Get again to test apply successful.
	result, err = s.ingressClient.Get(context.TODO(), "ig1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `ig1`)
	c.Assert(result.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *ingressSuite) TestGet(c *gc.C) {
	template := netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ig1",
		},
	}
	ig := template

	// Create ig with annotations.
	ig.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.ingressClient.Create(context.TODO(), &ig, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Create new object that has no annotations.
	res := resources.NewIngress(s.ingressClient, "ig1", &template)
	c.Assert(len(res.GetAnnotations()), gc.Equals, 0)

	// Get actual resource that has annotations using k8s api.
	err = res.Get(context.TODO())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.GetName(), gc.Equals, `ig1`)
	c.Assert(res.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *ingressSuite) TestDelete(c *gc.C) {
	ig := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ig1",
		},
	}

	// Create ig1.
	_, err := s.ingressClient.Create(context.TODO(), ig, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Get ig1 to ensure it exists.
	result, err := s.ingressClient.Get(context.TODO(), "ig1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `ig1`)

	// Create new object for deletion.
	res := resources.NewIngress(s.ingressClient, "ig1", ig)

	// Delete ig1.
	err = res.Delete(context.TODO())
	c.Assert(err, jc.ErrorIsNil)

	// Delete ig1 again.
	err = res.Delete(context.TODO())
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	err = res.Get(context.TODO())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.ingressClient.Get(context.TODO(), "ig1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}

func (s *ingressSuite) TestListIngresses(c *gc.C) {
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

	// Create ig1
	ig1Name := "ig1"
	ig1 := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:   ig1Name,
			Labels: labelSet,
		},
	}
	_, err = s.ingressClient.Create(context.TODO(), ig1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Create ig2
	ig2Name := "ig2"
	ig2 := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:   ig2Name,
			Labels: labelSet,
		},
	}
	_, err = s.ingressClient.Create(context.TODO(), ig2, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// List resources with correct labels.
	igs, err := resources.ListIngresses(context.Background(), s.ingressClient, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(igs), gc.Equals, 2)
	c.Assert(igs[0].GetName(), gc.Equals, ig1Name)
	c.Assert(igs[1].GetName(), gc.Equals, ig2Name)

	// List resources with no labels.
	igs, err = resources.ListIngresses(context.Background(), s.ingressClient, metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(igs), gc.Equals, 2)

	// List resources with wrong labels.
	igs, err = resources.ListIngresses(context.Background(), s.ingressClient, metav1.ListOptions{
		LabelSelector: "foo=bar",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(igs), gc.Equals, 0)
}
