// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	netv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	netv1client "k8s.io/client-go/kubernetes/typed/networking/v1"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	providerutils "github.com/juju/juju/internal/provider/kubernetes/utils"
	"github.com/juju/juju/internal/uuid"
)

type ingressSuite struct {
	resourceSuite
	namespace     string
	ingressClient netv1client.IngressInterface
}

func TestIngressSuite(t *testing.T) {
	tc.Run(t, &ingressSuite{})
}

func (s *ingressSuite) SetUpTest(c *tc.C) {
	s.resourceSuite.SetUpTest(c)
	s.namespace = "ns1"
	s.ingressClient = s.client.NetworkingV1().Ingresses(s.namespace)
}

func (s *ingressSuite) TestApply(c *tc.C) {
	ig := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ig1",
		},
	}

	// Create.
	res := resources.NewIngress(s.ingressClient, "ig1", ig)
	c.Assert(res.Apply(c.Context()), tc.ErrorIsNil)

	// Get.
	result, err := s.ingressClient.Get(c.Context(), "ig1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), tc.Equals, 0)

	// Apply
	ig.SetAnnotations(map[string]string{"a": "b"})
	res = resources.NewIngress(s.ingressClient, "ig1", ig)
	c.Assert(res.Apply(c.Context()), tc.ErrorIsNil)

	// Get again to test apply successful.
	result, err = s.ingressClient.Get(c.Context(), "ig1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `ig1`)
	c.Assert(result.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *ingressSuite) TestGet(c *tc.C) {
	template := netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ig1",
		},
	}
	ig := template

	// Create ig with annotations.
	ig.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.ingressClient.Create(c.Context(), &ig, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// Create new object that has no annotations.
	res := resources.NewIngress(s.ingressClient, "ig1", &template)
	c.Assert(len(res.GetAnnotations()), tc.Equals, 0)

	// Get actual resource that has annotations using k8s api.
	err = res.Get(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.GetName(), tc.Equals, `ig1`)
	c.Assert(res.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *ingressSuite) TestDelete(c *tc.C) {
	ig := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ig1",
		},
	}

	// Create ig1.
	_, err := s.ingressClient.Create(c.Context(), ig, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// Get ig1 to ensure it exists.
	result, err := s.ingressClient.Get(c.Context(), "ig1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `ig1`)

	// Create new object for deletion.
	res := resources.NewIngress(s.ingressClient, "ig1", ig)

	// Delete ig1.
	err = res.Delete(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// Delete ig1 again.
	err = res.Delete(c.Context())
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	err = res.Get(c.Context())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	_, err = s.ingressClient.Get(c.Context(), "ig1", metav1.GetOptions{})
	c.Assert(err, tc.Satisfies, k8serrors.IsNotFound)
}

func (s *ingressSuite) TestListIngresses(c *tc.C) {
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

	// Create ig1
	ig1Name := "ig1"
	ig1 := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:   ig1Name,
			Labels: labelSet,
		},
	}
	_, err = s.ingressClient.Create(c.Context(), ig1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// Create ig2
	ig2Name := "ig2"
	ig2 := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:   ig2Name,
			Labels: labelSet,
		},
	}
	_, err = s.ingressClient.Create(c.Context(), ig2, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// List resources with correct labels.
	igs, err := resources.ListIngresses(context.Background(), s.ingressClient, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(igs), tc.Equals, 2)
	c.Assert(igs[0].GetName(), tc.Equals, ig1Name)
	c.Assert(igs[1].GetName(), tc.Equals, ig2Name)

	// List resources with no labels.
	igs, err = resources.ListIngresses(context.Background(), s.ingressClient, metav1.ListOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(igs), tc.Equals, 2)

	// List resources with wrong labels.
	igs, err = resources.ListIngresses(context.Background(), s.ingressClient, metav1.ListOptions{
		LabelSelector: "foo=bar",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(igs), tc.Equals, 0)
}
