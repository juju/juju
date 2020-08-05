// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8sresources

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

	"github.com/juju/juju/caas/kubernetes/provider/constants"
)

type Secret struct {
	corev1.Secret
}

func NewSecret(name string, namespace string) *Secret {
	return &Secret{
		Secret: corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		},
	}
}

func (s *Secret) Clone() Resource {
	clone := *s
	return &clone
}

func (s *Secret) Apply(ctx context.Context, client kubernetes.Interface) error {
	api := client.CoreV1().Secrets(s.Namespace)
	s.TypeMeta.Kind = "Secret"
	s.TypeMeta.APIVersion = corev1.SchemeGroupVersion.String()
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

func (s *Secret) Get(ctx context.Context, client kubernetes.Interface) error {
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

func (s *Secret) Delete(ctx context.Context, client kubernetes.Interface) error {
	api := client.CoreV1().Secrets(s.Namespace)
	err := api.Delete(ctx, s.Name, metav1.DeleteOptions{
		PropagationPolicy: k8sconstants.DefaultPropagationPolicy(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}
