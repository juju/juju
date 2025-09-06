// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	providerutils "github.com/juju/juju/internal/provider/kubernetes/utils"
)

type secretSuite struct {
	resourceSuite
	namespace    string
	secretClient v1.SecretInterface
}

var _ = gc.Suite(&secretSuite{})

func (s *secretSuite) SetUpTest(c *gc.C) {
	s.resourceSuite.SetUpTest(c)
	s.namespace = "ns1"
	s.secretClient = s.client.CoreV1().Secrets(s.namespace)
}

func (s *secretSuite) TestApply(c *gc.C) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret1",
			Namespace: "test",
		},
	}
	// Create.
	secretResource := resources.NewSecret(s.client.CoreV1().Secrets(secret.Namespace), "test", "secret1", secret)
	c.Assert(secretResource.Apply(context.TODO()), jc.ErrorIsNil)
	result, err := s.client.CoreV1().Secrets("test").Get(context.TODO(), "secret1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), gc.Equals, 0)

	// Update.
	secret.SetAnnotations(map[string]string{"a": "b"})
	secretResource = resources.NewSecret(s.client.CoreV1().Secrets(secret.Namespace), "test", "secret1", secret)
	c.Assert(secretResource.Apply(context.TODO()), jc.ErrorIsNil)

	result, err = s.client.CoreV1().Secrets("test").Get(context.TODO(), "secret1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `secret1`)
	c.Assert(result.GetNamespace(), gc.Equals, `test`)
	c.Assert(result.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *secretSuite) TestGet(c *gc.C) {
	template := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret1",
			Namespace: "test",
		},
	}
	secret1 := template
	secret1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.CoreV1().Secrets("test").Create(context.TODO(), &secret1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	secretResource := resources.NewSecret(s.client.CoreV1().Secrets(secret1.Namespace), "test", "secret1", &template)
	c.Assert(len(secretResource.GetAnnotations()), gc.Equals, 0)
	err = secretResource.Get(context.TODO())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(secretResource.GetName(), gc.Equals, `secret1`)
	c.Assert(secretResource.GetNamespace(), gc.Equals, `test`)
	c.Assert(secretResource.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *secretSuite) TestDelete(c *gc.C) {
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret1",
			Namespace: "test",
		},
	}
	_, err := s.client.CoreV1().Secrets("test").Create(context.TODO(), &secret, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.client.CoreV1().Secrets("test").Get(context.TODO(), "secret1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `secret1`)

	secretResource := resources.NewSecret(s.client.CoreV1().Secrets(secret.Namespace), "test", "secret1", &secret)
	err = secretResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIsNil)

	err = secretResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	err = secretResource.Get(context.TODO())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.client.CoreV1().Secrets("test").Get(context.TODO(), "secret1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}

func (s *secretSuite) TestListSecrets(c *gc.C) {
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

	// Create secret1
	secret1Name := "secret1"
	secret1 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:   secret1Name,
			Labels: labelSet,
		},
	}
	_, err = s.secretClient.Create(context.TODO(), secret1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Create secret2
	secret2Name := "secret2"
	secret2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:   secret2Name,
			Labels: labelSet,
		},
	}
	_, err = s.secretClient.Create(context.TODO(), secret2, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// List resources with correct labels.
	secrets, err := resources.ListSecrets(context.Background(), s.secretClient, s.namespace, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(secrets), gc.Equals, 2)
	c.Assert(secrets[0].GetName(), gc.Equals, secret1Name)
	c.Assert(secrets[1].GetName(), gc.Equals, secret2Name)

	// List resources with no labels.
	secrets, err = resources.ListSecrets(context.Background(), s.secretClient, s.namespace, metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(secrets), gc.Equals, 2)

	// List resources with wrong labels.
	secrets, err = resources.ListSecrets(context.Background(), s.secretClient, s.namespace, metav1.ListOptions{
		LabelSelector: "foo=bar",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(secrets), gc.Equals, 0)
}
