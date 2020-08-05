// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/juju/caas/kubernetes/provider/utils"
)

func (k *kubernetesClient) getlabelsForApp(appName string, isNamespaced bool) map[string]string {
	labels := k8sutils.LabelsForApp(appName)
	if !isNamespaced {
		labels = k8sutils.AppendLabels(labels, k8sutils.LabelsForModel(k.CurrentModel()))
	}
	return labels
}
