// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/juju/juju/internal/provider/caas/kubernetes/provider/utils"
)

// IsLegacyLabels indicates if this provider is operating on a legacy label schema
func (k *kubernetesClient) IsLegacyLabels() bool {
	return k.isLegacyLabels
}

func isK8sObjectOwnedByJuju(objMeta meta.ObjectMeta) bool {
	return utils.HasLabels(labels.Set(objMeta.Labels), utils.LabelsJuju)
}
