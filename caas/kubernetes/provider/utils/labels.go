// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"context"

	"github.com/juju/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	core "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
)

var (
	// LabelsJuju is a common set
	LabelsJuju = map[string]string{
		constants.LabelKubernetesAppManaged: "juju",
	}

	// LabelsJujuModelOperatorDisableWebhook is a set of labels needed on a
	// given object to disable admission webhook validation.
	LabelsJujuModelOperatorDisableWebhook = map[string]string{
		constants.LabelJujuModelOperatorDisableWebhook: "true",
	}
)

// HasLabels returns true if the src contains the labels in has
func HasLabels(src, has labels.Set) bool {
	for k, v := range has {
		if src[k] != v {
			return false
		}
	}
	return true
}

// ErrUnexpectedModelLabels is returned when the namespace does not have
// any of the expected variants of labels.
const ErrUnexpectedModelLabels errors.ConstError = "unexpected model labels"

// DetectModelLabelVersion checks to see if the provided model is running on an older
// labeling scheme or a newer one.
func DetectModelLabelVersion(ctx context.Context, namespace, modelName, modelUUID, controllerUUID string, namespaceI core.NamespaceInterface) (constants.LabelVersion, error) {
	ns, err := namespaceI.Get(ctx, namespace, meta.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return constants.LabelVersion2, nil
	}
	if err != nil {
		return -1, errors.Annotatef(err, "unable to determine model label version for namespace %q", namespace)
	}
	for i := constants.LastLabelVersion; i >= constants.FirstLabelVersion; i-- {
		if HasLabels(ns.Labels, LabelsForModel(modelName, modelUUID, controllerUUID, i)) {
			return i, nil
		}
	}
	return -1, ErrUnexpectedModelLabels
}

// LabelsForApp returns the labels that should be on a k8s object for a given
// application name
func LabelsForApp(name string, labelVersion constants.LabelVersion) labels.Set {
	result := SelectorLabelsForApp(name, labelVersion)
	if labelVersion == constants.LegacyLabelVersion {
		return result
	}
	return LabelsMerge(result, LabelsJuju)
}

// SelectorLabelsForApp returns the pod selector labels that should be on
// a k8s object for a given application name
func SelectorLabelsForApp(name string, labelVersion constants.LabelVersion) labels.Set {
	if labelVersion == constants.LegacyLabelVersion {
		return labels.Set{
			constants.LegacyLabelKubernetesAppName: name,
		}
	}
	return labels.Set{
		constants.LabelKubernetesAppName: name,
	}
}

// LabelForKeyValue returns a Kubernetes label set for the supplied key value.
func LabelForKeyValue(key, value string) labels.Set {
	return labels.Set{
		key: value,
	}
}

// LabelsMerge merges one or more sets of labels together into a new set. For
// duplicate keys the last key found is used.
func LabelsMerge(a labels.Set, merges ...labels.Set) labels.Set {
	for _, merge := range merges {
		a = labels.Merge(a, merge)
	}
	return a
}

// LabelsForModel returns the labels that should be on a k8s object for a given
// model name
func LabelsForModel(modelName string, modelUUID string, controllerUUID string, labelVersion constants.LabelVersion) labels.Set {
	switch labelVersion {
	case constants.LegacyLabelVersion:
		return map[string]string{
			constants.LegacyLabelModelName: modelName,
		}
	case constants.LabelVersion1:
		return map[string]string{
			constants.LabelJujuModelName: modelName,
		}
	case constants.LabelVersion2:
		fallthrough
	default:
		if modelName == constants.JujuControllerModelName {
			return map[string]string{
				constants.LabelJujuModelName:      modelName,
				constants.LabelJujuControllerUUID: controllerUUID,
			}
		}
		return map[string]string{
			constants.LabelJujuModelName: modelName,
			constants.LabelJujuModelUUID: modelUUID,
		}
	}
}

// LabelsForOperator returns the labels that should be placed on a juju operator
// Takes the operator name, type and a legacy flag to indicate these labels are
// being used on a model that is operating in "legacy" label mode
func LabelsForOperator(name, target string, labelVersion constants.LabelVersion) labels.Set {
	if labelVersion == constants.LegacyLabelVersion {
		return map[string]string{
			constants.LegacyLabelKubernetesOperatorName: name,
		}
	}
	return map[string]string{
		constants.LabelJujuOperatorName:   name,
		constants.LabelJujuOperatorTarget: target,
	}
}

// LabelsForStorage return the labels that should be placed on a k8s storage
// object. Takes the storage name and a legacy flat.
func LabelsForStorage(name string, labelVersion constants.LabelVersion) labels.Set {
	if labelVersion == constants.LegacyLabelVersion {
		return map[string]string{
			constants.LegacyLabelStorageName: name,
		}
	}
	return map[string]string{
		constants.LabelJujuStorageName: name,
	}
}

// LabelsToSelector transforms the supplied label set to a valid Kubernetes
// label selector
func LabelsToSelector(ls labels.Set) labels.Selector {
	return labels.SelectorFromValidatedSet(ls)
}

// StorageNameFromLabels returns the juju storage name used in the provided
// label set. First checks for the key LabelJujuStorageName and then defaults
// over to the key LegacyLabelStorageName. If neither key exists an empty string
// is returned.
func StorageNameFromLabels(labels labels.Set) string {
	if labels[constants.LabelJujuStorageName] != "" {
		return labels[constants.LabelJujuStorageName]
	}
	return labels[constants.LegacyLabelStorageName]
}
