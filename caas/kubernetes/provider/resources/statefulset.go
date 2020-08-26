// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8sresources

import (
	"context"

	"github.com/juju/errors"
	"github.com/kr/pretty"
	appsv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
)

type StatefulSet struct {
	appsv1.StatefulSet
}

func NewStatefulSet(name string, namespace string) *StatefulSet {
	return &StatefulSet{
		StatefulSet: appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		},
	}
}

func (ss *StatefulSet) Clone() Resource {
	clone := *ss
	return &clone
}

func (ss *StatefulSet) Apply(ctx context.Context, client kubernetes.Interface) error {
	logger.Errorf("StatefulSet.Apply %s", pretty.Sprint(ss))
	api := client.AppsV1().StatefulSets(ss.Namespace)
	ss.TypeMeta.Kind = "StatefulSet"
	ss.TypeMeta.APIVersion = appsv1.SchemeGroupVersion.String()
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &ss.StatefulSet)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := api.Patch(ctx, ss.Name, types.ApplyPatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if err != nil {
		return errors.Trace(err)
	}
	ss.StatefulSet = *res
	return nil
}

func (ss *StatefulSet) Get(ctx context.Context, client kubernetes.Interface) error {
	api := client.AppsV1().StatefulSets(ss.Namespace)
	res, err := api.Get(ctx, ss.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	ss.StatefulSet = *res
	return nil
}

func (ss *StatefulSet) Delete(ctx context.Context, client kubernetes.Interface) error {
	api := client.AppsV1().StatefulSets(ss.Namespace)
	err := api.Delete(ctx, ss.Name, metav1.DeleteOptions{
		PropagationPolicy: k8sconstants.DefaultPropagationPolicy(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}
