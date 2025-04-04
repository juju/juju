// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
)

// LabelVersion returns the detected label version for k8s resources created
// for this model.
func (k *kubernetesClient) LabelVersion() constants.LabelVersion {
	return k.labelVersion
}

func isK8sObjectOwnedByJuju(objMeta meta.ObjectMeta) bool {
	return utils.HasLabels(labels.Set(objMeta.Labels), utils.LabelsJuju)
}
