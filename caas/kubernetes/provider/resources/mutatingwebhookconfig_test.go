// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	admissionclient "k8s.io/client-go/kubernetes/typed/admissionregistration/v1"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/resources"
	providerutils "github.com/juju/juju/caas/kubernetes/provider/utils"
)

type mutatingWebhookConfigSuite struct {
	resourceSuite
	namespace             string
	mutatingWebhookClient admissionclient.MutatingWebhookConfigurationInterface
}

var _ = gc.Suite(&mutatingWebhookConfigSuite{})

func (s *mutatingWebhookConfigSuite) SetUpTest(c *gc.C) {
	s.resourceSuite.SetUpTest(c)
	s.namespace = "ns1"
	s.mutatingWebhookClient = s.client.AdmissionregistrationV1().MutatingWebhookConfigurations()
}

func (s *mutatingWebhookConfigSuite) TestApply(c *gc.C) {
	mw := &admissionv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mw1",
		},
	}

	// Create.
	res := resources.NewMutatingWebhookConfig(s.mutatingWebhookClient, "mw1", mw)
	c.Assert(res.Apply(context.TODO()), jc.ErrorIsNil)

	// Get.
	result, err := s.mutatingWebhookClient.Get(context.TODO(), "mw1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), gc.Equals, 0)

	// Apply.
	mw.SetAnnotations(map[string]string{"a": "b"})
	res = resources.NewMutatingWebhookConfig(s.mutatingWebhookClient, "mw1", mw)
	c.Assert(res.Apply(context.TODO()), jc.ErrorIsNil)

	// Get again to test apply successful.
	result, err = s.mutatingWebhookClient.Get(context.TODO(), "mw1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `mw1`)
	c.Assert(result.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *mutatingWebhookConfigSuite) TestGet(c *gc.C) {
	template := admissionv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mw1",
		},
	}
	mw := template

	// Create mw with annotations
	mw.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.mutatingWebhookClient.Create(context.TODO(), &mw, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Create new object that has no annotations
	res := resources.NewMutatingWebhookConfig(s.mutatingWebhookClient, "mw1", &template)
	c.Assert(len(res.GetAnnotations()), gc.Equals, 0)

	// Get actual resource that has annotations using k8s api
	err = res.Get(context.TODO())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.GetName(), gc.Equals, `mw1`)
	c.Assert(res.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *mutatingWebhookConfigSuite) TestDelete(c *gc.C) {
	mw := &admissionv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mw1",
		},
	}

	// Create mw1.
	_, err := s.mutatingWebhookClient.Create(context.TODO(), mw, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Get mw1 to ensure it exists.
	result, err := s.mutatingWebhookClient.Get(context.TODO(), "mw1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `mw1`)

	// Create new object for deletion.
	res := resources.NewMutatingWebhookConfig(s.mutatingWebhookClient, "mw1", mw)

	// Delete mw1.
	err = res.Delete(context.TODO())
	c.Assert(err, jc.ErrorIsNil)

	// Delete mw1 again.
	err = res.Delete(context.TODO())
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	err = res.Get(context.TODO())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.mutatingWebhookClient.Get(context.TODO(), "mw1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}

func (s *mutatingWebhookConfigSuite) TestListMutatingWebhookConfigs(c *gc.C) {
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

	// Create mw1
	mw1Name := "mw1"
	mw1 := &admissionv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:   mw1Name,
			Labels: labelSet,
		},
	}
	_, err = s.mutatingWebhookClient.Create(context.TODO(), mw1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Create mw2
	mw2Name := "mw2"
	mw2 := &admissionv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:   mw2Name,
			Labels: labelSet,
		},
	}
	_, err = s.mutatingWebhookClient.Create(context.TODO(), mw2, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// List resources with correct labels.
	mws, err := resources.ListMutatingWebhookConfigs(context.Background(), s.mutatingWebhookClient, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(mws), gc.Equals, 2)
	c.Assert(mws[0].GetName(), gc.Equals, mw1Name)
	c.Assert(mws[1].GetName(), gc.Equals, mw2Name)

	// List resources with no labels.
	mws, err = resources.ListMutatingWebhookConfigs(context.Background(), s.mutatingWebhookClient, metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(mws), gc.Equals, 2)

	// List resources with wrong labels.
	mws, err = resources.ListMutatingWebhookConfigs(context.Background(), s.mutatingWebhookClient, metav1.ListOptions{
		LabelSelector: "foo=bar",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(mws), gc.Equals, 0)
}
