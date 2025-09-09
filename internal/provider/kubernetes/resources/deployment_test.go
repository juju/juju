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

type deploymentSuite struct {
	resourceSuite
	namespace        string
	deploymentClient v1.DeploymentInterface
}

func TestDeploymentSuite(t *testing.T) {
	tc.Run(t, &deploymentSuite{})
}

func (s *deploymentSuite) SetUpTest(c *tc.C) {
	s.resourceSuite.SetUpTest(c)
	s.namespace = "ns1"
	s.deploymentClient = s.client.AppsV1().Deployments(s.namespace)
}

func (s *deploymentSuite) TestApply(c *tc.C) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deployment1",
			Namespace: "test",
		},
	}
	// Create.
	deploymentResource := resources.NewDeployment(s.client.AppsV1().Deployments(deployment.Namespace), "test", "deployment1", deployment)
	c.Assert(deploymentResource.Apply(c.Context()), tc.ErrorIsNil)
	result, err := s.client.AppsV1().Deployments("test").Get(c.Context(), "deployment1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), tc.Equals, 0)

	// Update.
	deployment.SetAnnotations(map[string]string{"a": "b"})
	deploymentResource = resources.NewDeployment(s.client.AppsV1().Deployments(deployment.Namespace), "test", "deployment1", deployment)
	c.Assert(deploymentResource.Apply(c.Context()), tc.ErrorIsNil)

	result, err = s.client.AppsV1().Deployments("test").Get(c.Context(), "deployment1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `deployment1`)
	c.Assert(result.GetNamespace(), tc.Equals, `test`)
	c.Assert(result.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *deploymentSuite) TestGet(c *tc.C) {
	template := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deployment1",
			Namespace: "test",
		},
	}
	deployment1 := template
	deployment1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.AppsV1().Deployments("test").Create(c.Context(), &deployment1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	deploymentResource := resources.NewDeployment(s.client.AppsV1().Deployments(deployment1.Namespace), "test", "deployment1", &template)
	c.Assert(len(deploymentResource.GetAnnotations()), tc.Equals, 0)
	err = deploymentResource.Get(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(deploymentResource.GetName(), tc.Equals, `deployment1`)
	c.Assert(deploymentResource.GetNamespace(), tc.Equals, `test`)
	c.Assert(deploymentResource.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *deploymentSuite) TestDelete(c *tc.C) {
	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deployment1",
			Namespace: "test",
		},
	}
	_, err := s.client.AppsV1().Deployments("test").Create(c.Context(), &deployment, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.client.AppsV1().Deployments("test").Get(c.Context(), "deployment1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `deployment1`)

	deploymentResource := resources.NewDeployment(s.client.AppsV1().Deployments(deployment.Namespace), "test", "deployment1", &deployment)
	err = deploymentResource.Delete(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	err = deploymentResource.Delete(c.Context())
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	err = deploymentResource.Get(c.Context())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	_, err = s.client.AppsV1().Deployments("test").Get(c.Context(), "deployment1", metav1.GetOptions{})
	c.Assert(err, tc.Satisfies, k8serrors.IsNotFound)
}

func (s *daemonsetSuite) TestListDeployments(c *tc.C) {
	// Set up labels for model and app to list resource.
	controllerUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	modelUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

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
	_, err = s.daemonsetClient.Create(c.Context(), deployment1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// Create deployment2.
	deployment2Name := "deployment2"
	deployment2 := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   deployment2Name,
			Labels: labelSet,
		},
	}
	_, err = s.daemonsetClient.Create(c.Context(), deployment2, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// List resources with correct labels.
	cms, err := resources.ListDaemonSets(context.Background(), s.daemonsetClient, s.namespace, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(cms), tc.Equals, 2)
	c.Assert(cms[0].GetName(), tc.Equals, deployment1Name)
	c.Assert(cms[1].GetName(), tc.Equals, deployment2Name)

	// List resources with no labels.
	cms, err = resources.ListDaemonSets(context.Background(), s.daemonsetClient, s.namespace, metav1.ListOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(cms), tc.Equals, 2)

	// List resources with wrong labels.
	cms, err = resources.ListDaemonSets(context.Background(), s.daemonsetClient, s.namespace, metav1.ListOptions{
		LabelSelector: "foo=bar",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(cms), tc.Equals, 0)
}
