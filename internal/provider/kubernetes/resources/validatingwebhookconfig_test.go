// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	admissionclient "k8s.io/client-go/kubernetes/typed/admissionregistration/v1"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	providerutils "github.com/juju/juju/internal/provider/kubernetes/utils"
	"github.com/juju/juju/internal/uuid"
)

type validatingWebhookConfigSuite struct {
	resourceSuite
	validatingWebhookClient admissionclient.ValidatingWebhookConfigurationInterface
}

func TestValidatingWebhookConfigSuite(t *testing.T) {
	tc.Run(t, &validatingWebhookConfigSuite{})
}

func (s *validatingWebhookConfigSuite) SetUpTest(c *tc.C) {
	s.resourceSuite.SetUpTest(c)
	s.validatingWebhookClient = s.client.AdmissionregistrationV1().ValidatingWebhookConfigurations()
}

func (s *validatingWebhookConfigSuite) TestApply(c *tc.C) {
	vw := &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vw1",
		},
	}

	// Create.
	res := resources.NewValidatingWebhookConfig(s.validatingWebhookClient, "vw1", vw)
	c.Assert(res.Apply(c.Context()), tc.ErrorIsNil)

	// Get.
	result, err := s.validatingWebhookClient.Get(c.Context(), "vw1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), tc.Equals, 0)

	// Apply.
	vw.SetAnnotations(map[string]string{"a": "b"})
	res = resources.NewValidatingWebhookConfig(s.validatingWebhookClient, "vw1", vw)
	c.Assert(res.Apply(c.Context()), tc.ErrorIsNil)

	// Get again to test apply successful.
	result, err = s.validatingWebhookClient.Get(c.Context(), "vw1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `vw1`)
	c.Assert(result.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *validatingWebhookConfigSuite) TestGet(c *tc.C) {
	template := admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vw1",
		},
	}
	vw := template

	// Create vw with annotations.
	vw.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.validatingWebhookClient.Create(c.Context(), &vw, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// Create new object that has no annotations.
	res := resources.NewValidatingWebhookConfig(s.validatingWebhookClient, "vw1", &template)
	c.Assert(len(res.GetAnnotations()), tc.Equals, 0)

	// Get actual resource that has annotations using k8s api.
	err = res.Get(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.GetName(), tc.Equals, `vw1`)
	c.Assert(res.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *validatingWebhookConfigSuite) TestDelete(c *tc.C) {
	vw := &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vw1",
		},
	}

	// Create vw1.
	_, err := s.validatingWebhookClient.Create(c.Context(), vw, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// Get vw1 to ensure it exists.
	result, err := s.validatingWebhookClient.Get(c.Context(), "vw1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `vw1`)

	// Create new object for deletion.
	res := resources.NewValidatingWebhookConfig(s.validatingWebhookClient, "vw1", vw)

	// Delete vw1
	err = res.Delete(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// Delete vw1 again.
	err = res.Delete(c.Context())
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	err = res.Get(c.Context())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	_, err = s.validatingWebhookClient.Get(c.Context(), "vw1", metav1.GetOptions{})
	c.Assert(err, tc.Satisfies, k8serrors.IsNotFound)
}

func (s *validatingWebhookConfigSuite) TestListValidatingWebhookConfiguration(c *tc.C) {
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

	// Create vw1
	vw1Name := "vw1"
	vw1 := &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:   vw1Name,
			Labels: labelSet,
		},
	}
	_, err = s.validatingWebhookClient.Create(c.Context(), vw1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// Create vw2
	vw2Name := "vw2"
	vw2 := &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:   vw2Name,
			Labels: labelSet,
		},
	}
	_, err = s.validatingWebhookClient.Create(c.Context(), vw2, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// List resources
	crds, err := resources.ListValidatingWebhookConfigs(context.Background(), s.validatingWebhookClient, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(crds), tc.Equals, 2)
	c.Assert(crds[0].GetName(), tc.Equals, vw1Name)
	c.Assert(crds[1].GetName(), tc.Equals, vw2Name)
}
