// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"

	"github.com/juju/errors"
	appsv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
)

type Deployment struct {
	appsv1.Deployment
}

func NewDeployment(name string, namespace string) *Deployment {
	return &Deployment{
		Deployment: appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		},
	}
}

func (d *Deployment) Clone() Resource {
	clone := *d
	return &clone
}

func (d *Deployment) Apply(ctx context.Context, client kubernetes.Interface) error {
	api := client.AppsV1().Deployments(d.Namespace)
	d.TypeMeta.Kind = "Deployment"
	d.TypeMeta.APIVersion = appsv1.SchemeGroupVersion.String()
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &d.Deployment)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := api.Patch(ctx, d.Name, types.ApplyPatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if err != nil {
		return errors.Trace(err)
	}
	d.Deployment = *res
	return nil
}

func (d *Deployment) Get(ctx context.Context, client kubernetes.Interface) error {
	api := client.AppsV1().Deployments(d.Namespace)
	res, err := api.Get(ctx, d.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	d.Deployment = *res
	return nil
}

func (d *Deployment) Delete(ctx context.Context, client kubernetes.Interface) error {
	api := client.AppsV1().Deployments(d.Namespace)
	err := api.Delete(ctx, d.Name, metav1.DeleteOptions{
		PropagationPolicy: &constants.DefaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}
