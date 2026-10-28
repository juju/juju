// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"github.com/juju/names/v5"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/utils"
)

const (
	labelJujuSecretModelName = "secrets.juju.is/model-name"
	labelJujuSecretModelUUID = "secrets.juju.is/model-id"
	labelJujuSecretConsumer  = "secrets.juju.is/consumer"
	// labelJujuIssuedTokenID is the label key for the issued token UUID
	// stamped on every resource created for a secret access token.
	// It allows bulk deletion via DeleteCollection with an In selector.
	labelJujuIssuedTokenID = "secrets.juju.is/issued-token-id"
)

const (
	annotationJujuSecretExpireAt = "secrets.juju.is/expire-at"
)

func labelsForSecretRevision(modelName string, modelUUID string) labels.Set {
	secretLabels := map[string]string{
		constants.LabelJujuModelName: modelName,
		labelJujuSecretModelName:     modelName,
		labelJujuSecretModelUUID:     modelUUID,
	}
	return utils.LabelsMerge(utils.LabelsJuju, secretLabels)
}

func labelsForServiceAccount(
	modelName string, modelUUID string, consumer names.Tag,
) labels.Set {
	secretLabels := map[string]string{
		constants.LabelJujuModelName: modelName,
		labelJujuSecretModelName:     modelName,
		labelJujuSecretModelUUID:     modelUUID,
	}
	if consumer != nil {
		secretLabels[labelJujuSecretConsumer] = consumer.String()
	}
	return utils.LabelsMerge(utils.LabelsJuju, secretLabels)
}

func modelLabelSelector(modelName string) labels.Selector {
	return utils.LabelsToSelector(map[string]string{
		constants.LabelJujuModelName: modelName,
	})
}
