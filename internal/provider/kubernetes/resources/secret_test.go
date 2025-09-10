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

type secretSuite struct {
	resourceSuite
	namespace    string
	secretClient v1.SecretInterface
}

func TestSecretSuite(t *testing.T) {
	tc.Run(t, &secretSuite{})
}

func (s *secretSuite) SetUpTest(c *tc.C) {
	s.resourceSuite.SetUpTest(c)
	s.namespace = "ns1"
	s.secretClient = s.client.CoreV1().Secrets(s.namespace)
}

func (s *secretSuite) TestApply(c *tc.C) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret1",
			Namespace: "test",
		},
	}
	// Create.
	secretResource := resources.NewSecret(s.client.CoreV1().Secrets(secret.Namespace), "test", "secret1", secret)
	c.Assert(secretResource.Apply(c.Context()), tc.ErrorIsNil)
	result, err := s.client.CoreV1().Secrets("test").Get(c.Context(), "secret1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), tc.Equals, 0)

	// Update.
	secret.SetAnnotations(map[string]string{"a": "b"})
	secretResource = resources.NewSecret(s.client.CoreV1().Secrets(secret.Namespace), "test", "secret1", secret)
	c.Assert(secretResource.Apply(c.Context()), tc.ErrorIsNil)

	result, err = s.client.CoreV1().Secrets("test").Get(c.Context(), "secret1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `secret1`)
	c.Assert(result.GetNamespace(), tc.Equals, `test`)
	c.Assert(result.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *secretSuite) TestGet(c *tc.C) {
	template := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret1",
			Namespace: "test",
		},
	}
	secret1 := template
	secret1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.CoreV1().Secrets("test").Create(c.Context(), &secret1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	secretResource := resources.NewSecret(s.client.CoreV1().Secrets(secret1.Namespace), "test", "secret1", &template)
	c.Assert(len(secretResource.GetAnnotations()), tc.Equals, 0)
	err = secretResource.Get(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(secretResource.GetName(), tc.Equals, `secret1`)
	c.Assert(secretResource.GetNamespace(), tc.Equals, `test`)
	c.Assert(secretResource.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *secretSuite) TestDelete(c *tc.C) {
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret1",
			Namespace: "test",
		},
	}
	_, err := s.client.CoreV1().Secrets("test").Create(c.Context(), &secret, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.client.CoreV1().Secrets("test").Get(c.Context(), "secret1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `secret1`)

	secretResource := resources.NewSecret(s.client.CoreV1().Secrets(secret.Namespace), "test", "secret1", &secret)
	err = secretResource.Delete(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	err = secretResource.Delete(c.Context())
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	err = secretResource.Get(c.Context())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	_, err = s.client.CoreV1().Secrets("test").Get(c.Context(), "secret1", metav1.GetOptions{})
	c.Assert(err, tc.Satisfies, k8serrors.IsNotFound)
}

func (s *secretSuite) TestListSecrets(c *tc.C) {
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

	// Create secret1
	secret1Name := "secret1"
	secret1 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:   secret1Name,
			Labels: labelSet,
		},
	}
	_, err = s.secretClient.Create(c.Context(), secret1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// Create secret2
	secret2Name := "secret2"
	secret2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:   secret2Name,
			Labels: labelSet,
		},
	}
	_, err = s.secretClient.Create(c.Context(), secret2, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// List resources with correct labels.
	secrets, err := resources.ListSecrets(context.Background(), s.secretClient, s.namespace, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(secrets), tc.Equals, 2)
	c.Assert(secrets[0].GetName(), tc.Equals, secret1Name)
	c.Assert(secrets[1].GetName(), tc.Equals, secret2Name)

	// List resources with no labels.
	secrets, err = resources.ListSecrets(context.Background(), s.secretClient, s.namespace, metav1.ListOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(secrets), tc.Equals, 2)

	// List resources with wrong labels.
	secrets, err = resources.ListSecrets(context.Background(), s.secretClient, s.namespace, metav1.ListOptions{
		LabelSelector: "foo=bar",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(secrets), tc.Equals, 0)
}
