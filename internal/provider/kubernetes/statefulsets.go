// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	apps "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/specs"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
)

// https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/#update-strategies
func updateStrategyForStatefulSet(strategy specs.UpdateStrategy) (o apps.StatefulSetUpdateStrategy, err error) {
	strategyType := apps.StatefulSetUpdateStrategyType(strategy.Type)

	o = apps.StatefulSetUpdateStrategy{Type: strategyType}
	switch strategyType {
	case apps.OnDeleteStatefulSetStrategyType:
		if strategy.RollingUpdate != nil {
			return o, errors.NewNotValid(nil, fmt.Sprintf("rolling update spec is not supported for %q", strategyType))
		}
	case apps.RollingUpdateStatefulSetStrategyType:
		if strategy.RollingUpdate != nil {
			if strategy.RollingUpdate.MaxSurge != nil || strategy.RollingUpdate.MaxUnavailable != nil {
				return o, errors.NotValidf("rolling update spec for statefulset")
			}
			if strategy.RollingUpdate.Partition == nil {
				return o, errors.New("rolling update spec partition is missing")
			}
			o.RollingUpdate = &apps.RollingUpdateStatefulSetStrategy{
				Partition: strategy.RollingUpdate.Partition,
			}
		}
	default:
		return o, errors.NotValidf("strategy type %q for statefulset", strategyType)
	}
	return o, nil
}

func (k *kubernetesClient) createStatefulSet(ctx context.Context, spec *apps.StatefulSet) (*apps.StatefulSet, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client().AppsV1().StatefulSets(k.namespace).Create(ctx, spec, v1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("stateful set %q", spec.GetName())
	}
	if k8serrors.IsInvalid(err) {
		return nil, errors.NotValidf("stateful set %q", spec.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) getStatefulSet(ctx context.Context, name string) (*apps.StatefulSet, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client().AppsV1().StatefulSets(k.namespace).Get(ctx, name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("stateful set %q", name)
	}
	return out, errors.Trace(err)
}

// deleteStatefulSet deletes a statefulset resource.
func (k *kubernetesClient) deleteStatefulSet(ctx context.Context, name string) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	err := k.client().AppsV1().StatefulSets(k.namespace).Delete(ctx, name, v1.DeleteOptions{
		PropagationPolicy: constants.DefaultPropagationPolicy(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}
