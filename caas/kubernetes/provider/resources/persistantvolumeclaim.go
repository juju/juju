// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8sresources

import (
	"context"

	"github.com/juju/errors"
	"github.com/kr/pretty"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
)

type PersistentVolumeClaim struct {
	corev1.PersistentVolumeClaim
}

func NewPersistentVolumeClaim(name string, namespace string) *PersistentVolumeClaim {
	return &PersistentVolumeClaim{
		PersistentVolumeClaim: corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		},
	}
}

func (pvc *PersistentVolumeClaim) Clone() Resource {
	clone := *pvc
	return &clone
}

func (pvc *PersistentVolumeClaim) Apply(ctx context.Context, client kubernetes.Interface) error {
	logger.Errorf("PersistentVolumeClaim.Apply %s", pretty.Sprint(pvc))
	api := client.CoreV1().PersistentVolumeClaims(pvc.Namespace)
	pvc.TypeMeta.Kind = "PersistentVolumeClaim"
	pvc.TypeMeta.APIVersion = corev1.SchemeGroupVersion.String()
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &pvc.PersistentVolumeClaim)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := api.Patch(ctx, pvc.Name, types.ApplyPatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if err != nil {
		return errors.Trace(err)
	}
	pvc.PersistentVolumeClaim = *res
	return nil
}

func (pvc *PersistentVolumeClaim) Get(ctx context.Context, client kubernetes.Interface) error {
	api := client.CoreV1().PersistentVolumeClaims(pvc.Namespace)
	res, err := api.Get(ctx, pvc.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	pvc.PersistentVolumeClaim = *res
	return nil
}

func (pvc *PersistentVolumeClaim) Delete(ctx context.Context, client kubernetes.Interface) error {
	api := client.CoreV1().PersistentVolumeClaims(pvc.Namespace)
	err := api.Delete(ctx, pvc.Name, metav1.DeleteOptions{
		PropagationPolicy: k8sconstants.DefaultPropagationPolicy(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (pvc *PersistentVolumeClaim) Events(ctx context.Context, client kubernetes.Interface) ([]corev1.Event, error) {
	selector := fields.AndSelectors(
		fields.OneTermEqualSelector("involvedObject.name", pvc.Name),
		fields.OneTermEqualSelector("involvedObject.kind", "PersistentVolumeClaim"),
	).String()
	eventList, err := client.CoreV1().Events(pvc.Namespace).List(ctx, metav1.ListOptions{
		FieldSelector: selector,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return eventList.Items, nil
}
