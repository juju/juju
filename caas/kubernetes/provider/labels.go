// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/juju/caas/kubernetes/provider/utils"
)

func (k *kubernetesClient) getlabelsForApp(appName string, isNamespaced bool) map[string]string {
	labels := utils.LabelsForApp(appName)
	if !isNamespaced {
		labels = utils.AppendLabels(labels, utils.LabelsForModel(k.CurrentModel()))
	}
	return labels
}
