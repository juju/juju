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

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
)

type deployment struct {
	appsv1.Deployment
}

// NewDeployment creates a new deployment resource.
func NewDeployment(name string, namespace string, in *appsv1.Deployment) Resource {
	if in == nil {
		in = &appsv1.Deployment{}
	}
	in.SetName(name)
	in.SetNamespace(namespace)
	return &deployment{*in}
}

func (d *deployment) Clone() Resource {
	clone := *d
	return &clone
}

func (d *deployment) Apply(ctx context.Context, client kubernetes.Interface) error {
	api := client.AppsV1().Deployments(d.Namespace)
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

func (d *deployment) Get(ctx context.Context, client kubernetes.Interface) error {
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

func (d *deployment) Delete(ctx context.Context, client kubernetes.Interface) error {
	api := client.AppsV1().Deployments(d.Namespace)
	err := api.Delete(ctx, d.Name, metav1.DeleteOptions{
		PropagationPolicy: &k8sconstants.DefaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}
