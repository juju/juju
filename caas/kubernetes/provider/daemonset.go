// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"

	"github.com/juju/errors"
	apps "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
)

func (k *kubernetesClient) getDaemonSetLabels(appName string) map[string]string {
	return utils.LabelsForApp(appName, k.IsLegacyLabels())
}

func (k *kubernetesClient) ensureDaemonSet(spec *apps.DaemonSet) (func(), error) {
	cleanUp := func() {}
	out, err := k.createDaemonSet(spec)
	if err == nil {
		logger.Debugf("daemon set %q created", out.GetName())
		cleanUp = func() { _ = k.deleteDaemonSet(out.GetName(), out.GetUID()) }
		return cleanUp, nil
	}
	if !errors.IsAlreadyExists(err) {
		return cleanUp, errors.Trace(err)
	}
	_, err = k.listDaemonSets(spec.GetLabels())
	if err != nil {
		if errors.IsNotFound(err) {
			// spec.Name is already used for an existing daemon set.
			return cleanUp, errors.AlreadyExistsf("daemon set %q", spec.GetName())
		}
		return cleanUp, errors.Trace(err)
	}
	_, err = k.updateDaemonSet(spec)
	logger.Debugf("updating daemon set %q", spec.GetName())
	return cleanUp, errors.Trace(err)
}

func (k *kubernetesClient) createDaemonSet(spec *apps.DaemonSet) (*apps.DaemonSet, error) {
	utils.PurifyResource(spec)
	out, err := k.client().AppsV1().DaemonSets(k.namespace).Create(context.TODO(), spec, v1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("daemon set %q", spec.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) getDaemonSet(name string) (*apps.DaemonSet, error) {
	out, err := k.client().AppsV1().DaemonSets(k.namespace).Get(context.TODO(), name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("daemon set %q", name)
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) updateDaemonSet(spec *apps.DaemonSet) (*apps.DaemonSet, error) {
	out, err := k.client().AppsV1().DaemonSets(k.namespace).Update(context.TODO(), spec, v1.UpdateOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("daemon set %q", spec.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) deleteDaemonSet(name string, uid types.UID) error {
	err := k.client().AppsV1().DaemonSets(k.namespace).Delete(context.TODO(), name, utils.NewPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) listDaemonSets(labels map[string]string) ([]apps.DaemonSet, error) {
	listOps := v1.ListOptions{
		LabelSelector: utils.LabelSetToSelector(labels).String(),
	}
	out, err := k.client().AppsV1().DaemonSets(k.namespace).List(context.TODO(), listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(out.Items) == 0 {
		return nil, errors.NotFoundf("daemon set with labels %v", labels)
	}
	return out.Items, nil
}

func (k *kubernetesClient) deleteDaemonSets(appName string) error {
	labels := k.getDaemonSetLabels(appName)
	if !k.IsLegacyLabels() {
		labels = utils.LabelsMerge(labels, utils.LabelsJuju)
	}

	err := k.client().AppsV1().DaemonSets(k.namespace).DeleteCollection(context.TODO(), v1.DeleteOptions{
		PropagationPolicy: &constants.DefaultPropagationPolicy,
	}, v1.ListOptions{
		LabelSelector: utils.LabelSetToSelector(labels).String(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}
