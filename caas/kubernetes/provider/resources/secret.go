// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"

	"github.com/juju/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
)

type secret struct {
	corev1.Secret
}

// NewSecret creates a new secret resource.
func NewSecret(name string, namespace string, in *corev1.Secret) Resource {
	if in == nil {
		in = &corev1.Secret{}
	}
	in.SetName(name)
	in.SetNamespace(namespace)
	return &secret{*in}
}

func (s *secret) Clone() Resource {
	clone := *s
	return &clone
}

func (s *secret) Apply(ctx context.Context, client kubernetes.Interface) error {
	api := client.CoreV1().Secrets(s.Namespace)
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &s.Secret)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := api.Patch(ctx, s.Name, types.ApplyPatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if err != nil {
		return errors.Trace(err)
	}
	s.Secret = *res
	return nil
}

func (s *secret) Get(ctx context.Context, client kubernetes.Interface) error {
	api := client.CoreV1().Secrets(s.Namespace)
	res, err := api.Get(ctx, s.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	s.Secret = *res
	return nil
}

func (s *secret) Delete(ctx context.Context, client kubernetes.Interface) error {
	api := client.CoreV1().Secrets(s.Namespace)
	err := api.Delete(ctx, s.Name, metav1.DeleteOptions{
		PropagationPolicy: &k8sconstants.DefaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}
