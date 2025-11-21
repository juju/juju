// Copyright 2025 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	"github.com/juju/juju/state"
	"github.com/juju/juju/upgrades"
)

type backendSuite struct{}

var _ = gc.Suite(&backendSuite{})

func newK8sClient(c *gc.C) func(model *state.Model) (kubernetes.Interface, *rest.Config, error) {
	return func(model *state.Model) (kubernetes.Interface, *rest.Config, error) {
		k8sClient := fake.NewSimpleClientset()

		// Seed some data.
		_, err := k8sClient.AppsV1().StatefulSets(model.Name()).
			Create(context.Background(), &v1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "app1",
					Annotations: map[string]string{
						"app.juju.is/uuid": "uniqueid1",
					},
				},
			}, metav1.CreateOptions{})
		c.Assert(err, jc.ErrorIsNil)

		_, err = k8sClient.AppsV1().StatefulSets(model.Name()).
			Create(context.Background(), &v1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "app2",
					Annotations: map[string]string{
						"app.juju.is/uuid": "uniqueid2",
					},
				},
			}, metav1.CreateOptions{})
		c.Assert(err, jc.ErrorIsNil)

		// Controller app doesn't have a storage id saved in the annotation.
		// We want to generate one to keep it consistent with all caas apps.
		_, err = k8sClient.AppsV1().StatefulSets(model.Name()).
			Create(context.Background(), &v1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "controller",
					Annotations: map[string]string{},
				},
			}, metav1.CreateOptions{})
		c.Assert(err, jc.ErrorIsNil)

		return k8sClient, &rest.Config{}, nil
	}
}

func namespaceForModel(modelName string, _ string, _ *rest.Config) (string, error) {
	return modelName, nil
}

// TestGetStorageUniqueIDs tests getting storage ids from k8s statefulset annotation
// for the supplied applications.
func (b *backendSuite) TestGetStorageUniqueIDs(c *gc.C) {
	apps := []state.AppAndStorageID{
		{
			Id:   "1",
			Name: "app1",
		},
		{
			Id:   "2",
			Name: "app2",
		},
	}
	appsAndStorageIDs, err := upgrades.GetStorageUniqueIDs(
		newK8sClient(c), namespaceForModel)(
		context.Background(),
		apps,
		&state.Model{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appsAndStorageIDs, jc.SameContents, []state.AppAndStorageID{
		{
			Id:              "1",
			Name:            "app1",
			StorageUniqueID: "uniqueid1",
		},
		{
			Id:              "2",
			Name:            "app2",
			StorageUniqueID: "uniqueid2",
		},
	})

	// Test for controller app.
	controllerApps := []state.AppAndStorageID{
		{
			Id:   "1",
			Name: "controller",
		},
	}
	appsAndStorageIDs, err = upgrades.GetStorageUniqueIDs(
		newK8sClient(c), namespaceForModel)(
		context.Background(),
		controllerApps,
		&state.Model{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appsAndStorageIDs, gc.HasLen, 1)
	c.Assert(appsAndStorageIDs[0].StorageUniqueID, gc.Not(gc.Equals), "")
}
