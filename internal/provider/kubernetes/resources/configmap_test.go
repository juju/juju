// Copyright 2025 Canonical Ltd.
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
	core "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	providerutils "github.com/juju/juju/internal/provider/kubernetes/utils"
)

type configmapSuite struct {
	resourceSuite
	namespace       string
	configmapClient core.ConfigMapInterface
}

var _ = gc.Suite(&configmapSuite{})

func (s *configmapSuite) SetUpTest(c *gc.C) {
	s.resourceSuite.SetUpTest(c)
	s.namespace = "ns1"
	s.configmapClient = s.client.CoreV1().ConfigMaps(s.namespace)
}

func (s *configmapSuite) TestApply(c *gc.C) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cm1",
		},
	}

	// Create.
	configMapResource := resources.NewConfigMap(s.configmapClient, "cm1", cm)
	c.Assert(configMapResource.Apply(context.TODO()), jc.ErrorIsNil)

	// Get.
	result, err := s.configmapClient.Get(context.TODO(), "cm1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), gc.Equals, 0)

	// Apply.
	cm.SetAnnotations(map[string]string{"a": "b"})
	configMapResource = resources.NewConfigMap(s.configmapClient, "cm1", cm)
	c.Assert(configMapResource.Apply(context.TODO()), jc.ErrorIsNil)

	// Get again to test apply successful.
	result, err = s.configmapClient.Get(context.TODO(), "cm1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `cm1`)
	c.Assert(result.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *configmapSuite) TestGet(c *gc.C) {
	template := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cm1",
		},
	}
	cm := template

	// Create cm with annotations.
	cm.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.configmapClient.Create(context.TODO(), &cm, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Create new cm1 configmap object that has no annotations.
	configMapResource := resources.NewConfigMap(s.configmapClient, "cm1", &template)
	c.Assert(len(configMapResource.GetAnnotations()), gc.Equals, 0)

	// Get actual resource that has annotations using k8s api.
	err = configMapResource.Get(context.TODO())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(configMapResource.GetName(), gc.Equals, `cm1`)
	c.Assert(configMapResource.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *configmapSuite) TestDelete(c *gc.C) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cm1",
		},
	}

	// Create cm1.
	_, err := s.configmapClient.Create(context.TODO(), cm, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Get cm1 to ensure it exists.
	result, err := s.configmapClient.Get(context.TODO(), "cm1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `cm1`)

	// Create new cm1 configmap object for deletion.
	configMapResource := resources.NewConfigMap(s.configmapClient, "cm1", cm)

	// Delete cm1.
	err = configMapResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIsNil)

	// Delete cm1 again.
	err = configMapResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	err = configMapResource.Get(context.TODO())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.configmapClient.Get(context.TODO(), "cm1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}

func (s *configmapSuite) TestListConfigMaps(c *gc.C) {
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

	// Create cm1.
	cm1Name := "cm1"
	cm1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:   cm1Name,
			Labels: labelSet,
		},
	}
	_, err = s.configmapClient.Create(context.TODO(), cm1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Create cm2.
	cm2Name := "cm2"
	cm2 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:   cm2Name,
			Labels: labelSet,
		},
	}
	_, err = s.configmapClient.Create(context.TODO(), cm2, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// List resources with correct labels.
	cms, err := resources.ListConfigMaps(context.Background(), s.configmapClient, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(cms), gc.Equals, 2)
	c.Assert(cms[0].GetName(), gc.Equals, cm1Name)
	c.Assert(cms[1].GetName(), gc.Equals, cm2Name)

	// List resources with no labels.
	cms, err = resources.ListConfigMaps(context.Background(), s.configmapClient, metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(cms), gc.Equals, 2)

	// List resources with wrong labels.
	cms, err = resources.ListConfigMaps(context.Background(), s.configmapClient, metav1.ListOptions{
		LabelSelector: "foo=bar",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(cms), gc.Equals, 0)
}
