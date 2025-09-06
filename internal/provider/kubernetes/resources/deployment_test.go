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

type deploymentSuite struct {
	resourceSuite
	namespace        string
	deploymentClient v1.DeploymentInterface
}

var _ = gc.Suite(&deploymentSuite{})

func (s *deploymentSuite) SetUpTest(c *gc.C) {
	s.resourceSuite.SetUpTest(c)
	s.namespace = "ns1"
	s.deploymentClient = s.client.AppsV1().Deployments(s.namespace)
}

func (s *deploymentSuite) TestApply(c *gc.C) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deployment1",
			Namespace: "test",
		},
	}
	// Create.
	deploymentResource := resources.NewDeployment(s.client.AppsV1().Deployments(deployment.Namespace), "test", "deployment1", deployment)
	c.Assert(deploymentResource.Apply(context.TODO()), jc.ErrorIsNil)
	result, err := s.client.AppsV1().Deployments("test").Get(context.TODO(), "deployment1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), gc.Equals, 0)

	// Update.
	deployment.SetAnnotations(map[string]string{"a": "b"})
	deploymentResource = resources.NewDeployment(s.client.AppsV1().Deployments(deployment.Namespace), "test", "deployment1", deployment)
	c.Assert(deploymentResource.Apply(context.TODO()), jc.ErrorIsNil)

	result, err = s.client.AppsV1().Deployments("test").Get(context.TODO(), "deployment1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `deployment1`)
	c.Assert(result.GetNamespace(), gc.Equals, `test`)
	c.Assert(result.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *deploymentSuite) TestGet(c *gc.C) {
	template := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deployment1",
			Namespace: "test",
		},
	}
	deployment1 := template
	deployment1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.AppsV1().Deployments("test").Create(context.TODO(), &deployment1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	deploymentResource := resources.NewDeployment(s.client.AppsV1().Deployments(deployment1.Namespace), "test", "deployment1", &template)
	c.Assert(len(deploymentResource.GetAnnotations()), gc.Equals, 0)
	err = deploymentResource.Get(context.TODO())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(deploymentResource.GetName(), gc.Equals, `deployment1`)
	c.Assert(deploymentResource.GetNamespace(), gc.Equals, `test`)
	c.Assert(deploymentResource.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *deploymentSuite) TestDelete(c *gc.C) {
	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deployment1",
			Namespace: "test",
		},
	}
	_, err := s.client.AppsV1().Deployments("test").Create(context.TODO(), &deployment, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.client.AppsV1().Deployments("test").Get(context.TODO(), "deployment1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `deployment1`)

	deploymentResource := resources.NewDeployment(s.client.AppsV1().Deployments(deployment.Namespace), "test", "deployment1", &deployment)
	err = deploymentResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIsNil)

	err = deploymentResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	err = deploymentResource.Get(context.TODO())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.client.AppsV1().Deployments("test").Get(context.TODO(), "deployment1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}

func (s *daemonsetSuite) TestListDeployments(c *gc.C) {
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

	// Create deployment1.
	deployment1Name := "deployment1"
	deployment1 := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   deployment1Name,
			Labels: labelSet,
		},
	}
	_, err = s.daemonsetClient.Create(context.TODO(), deployment1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Create deployment2.
	deployment2Name := "deployment2"
	deployment2 := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   deployment2Name,
			Labels: labelSet,
		},
	}
	_, err = s.daemonsetClient.Create(context.TODO(), deployment2, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// List resources with correct labels.
	cms, err := resources.ListDaemonSets(context.Background(), s.daemonsetClient, s.namespace, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(cms), gc.Equals, 2)
	c.Assert(cms[0].GetName(), gc.Equals, deployment1Name)
	c.Assert(cms[1].GetName(), gc.Equals, deployment2Name)

	// List resources with no labels.
	cms, err = resources.ListDaemonSets(context.Background(), s.daemonsetClient, s.namespace, metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(cms), gc.Equals, 2)

	// List resources with wrong labels.
	cms, err = resources.ListDaemonSets(context.Background(), s.daemonsetClient, s.namespace, metav1.ListOptions{
		LabelSelector: "foo=bar",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(cms), gc.Equals, 0)
}
