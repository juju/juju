// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"k8s.io/apimachinery/pkg/labels"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/utils"
)

const (
	labelJujuSecretModelName = "secrets.juju.is/model-name"
	labelJujuSecretModelUUID = "secrets.juju.is/model-id"
)

func labelsForSecretRevision(modelName string, modelUUID string) labels.Set {
	secretLabels := map[string]string{
		constants.LabelJujuModelName: modelName,
		labelJujuSecretModelName:     modelName,
		labelJujuSecretModelUUID:     modelUUID,
	}
	return utils.LabelsMerge(utils.LabelsJuju, secretLabels)
}

func labelsForServiceAccount(modelName string, modelUUID string) labels.Set {
	secretLabels := map[string]string{
		constants.LabelJujuModelName: modelName,
		labelJujuSecretModelName:     modelName,
		labelJujuSecretModelUUID:     modelUUID,
	}
	return utils.LabelsMerge(utils.LabelsJuju, secretLabels)
}

func modelLabelSelector(modelName string) labels.Selector {
	return utils.LabelsToSelector(map[string]string{
		constants.LabelJujuModelName: modelName,
	})
}
