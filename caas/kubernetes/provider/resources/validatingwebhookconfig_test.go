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

type validatingWebhookConfigSuite struct {
	resourceSuite
	validatingWebhookClient admissionclient.ValidatingWebhookConfigurationInterface
}

var _ = gc.Suite(&validatingWebhookConfigSuite{})

func (s *validatingWebhookConfigSuite) SetUpTest(c *gc.C) {
	s.resourceSuite.SetUpTest(c)
	s.validatingWebhookClient = s.client.AdmissionregistrationV1().ValidatingWebhookConfigurations()
}

func (s *validatingWebhookConfigSuite) TestApply(c *gc.C) {
	vw := &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vw1",
		},
	}

	// Create.
	res := resources.NewValidatingWebhookConfig(s.validatingWebhookClient, "vw1", vw)
	c.Assert(res.Apply(context.TODO()), jc.ErrorIsNil)

	// Get.
	result, err := s.validatingWebhookClient.Get(context.TODO(), "vw1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), gc.Equals, 0)

	// Apply.
	vw.SetAnnotations(map[string]string{"a": "b"})
	res = resources.NewValidatingWebhookConfig(s.validatingWebhookClient, "vw1", vw)
	c.Assert(res.Apply(context.TODO()), jc.ErrorIsNil)

	// Get again to test apply successful.
	result, err = s.validatingWebhookClient.Get(context.TODO(), "vw1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `vw1`)
	c.Assert(result.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *validatingWebhookConfigSuite) TestGet(c *gc.C) {
	template := admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vw1",
		},
	}
	vw := template

	// Create vw with annotations.
	vw.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.validatingWebhookClient.Create(context.TODO(), &vw, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Create new object that has no annotations.
	res := resources.NewValidatingWebhookConfig(s.validatingWebhookClient, "vw1", &template)
	c.Assert(len(res.GetAnnotations()), gc.Equals, 0)

	// Get actual resource that has annotations using k8s api.
	err = res.Get(context.TODO())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.GetName(), gc.Equals, `vw1`)
	c.Assert(res.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *validatingWebhookConfigSuite) TestDelete(c *gc.C) {
	vw := &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vw1",
		},
	}

	// Create vw1.
	_, err := s.validatingWebhookClient.Create(context.TODO(), vw, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Get vw1 to ensure it exists.
	result, err := s.validatingWebhookClient.Get(context.TODO(), "vw1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `vw1`)

	// Create new object for deletion.
	res := resources.NewValidatingWebhookConfig(s.validatingWebhookClient, "vw1", vw)

	// Delete vw1
	err = res.Delete(context.TODO())
	c.Assert(err, jc.ErrorIsNil)

	// Delete vw1 again.
	err = res.Delete(context.TODO())
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	err = res.Get(context.TODO())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.validatingWebhookClient.Get(context.TODO(), "vw1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}

func (s *validatingWebhookConfigSuite) TestListValidatingWebhookConfiguration(c *gc.C) {
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

	// Create vw1
	vw1Name := "vw1"
	vw1 := &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:   vw1Name,
			Labels: labelSet,
		},
	}
	_, err = s.validatingWebhookClient.Create(context.TODO(), vw1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Create vw2
	vw2Name := "vw2"
	vw2 := &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:   vw2Name,
			Labels: labelSet,
		},
	}
	_, err = s.validatingWebhookClient.Create(context.TODO(), vw2, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// List resources
	crds, err := resources.ListValidatingWebhookConfigs(context.Background(), s.validatingWebhookClient, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(crds), gc.Equals, 2)
	c.Assert(crds[0].GetName(), gc.Equals, vw1Name)
	c.Assert(crds[1].GetName(), gc.Equals, vw2Name)
}
