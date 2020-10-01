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

// IsLegacyModelLabels checks to see if the provided model is running on an older
// labeling scheme or a newer one.
func IsLegacyModelLabels(model string, namespaceI core.NamespaceInterface) (bool, error) {
	ns, err := namespaceI.Get(context.TODO(), model, meta.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return true, errors.Annotatef(err, "unable to determine legacy status for namespace %s", model)
	}

	return !HasLabels(ns.Labels, LabelsForModel(model, false)), nil
}

// LabelSetToSelector converts a set of Kubernetes labels.
func LabelSetToSelector(l labels.Set) labels.Selector {
	return labels.SelectorFromValidatedSet(l)
}

// LabelsForApp returns the labels that should be on a k8s object for a given
// application name.
func LabelsForApp(name string, legacy bool) labels.Set {
	result := SelectorLabelsForApp(name, legacy)
	if legacy {
		return result
	}
	return LabelsMerge(result, LabelsJuju)
}

// SelectorLabelsForApp returns the pod selector labels that should
// be used to select pods belonging to an application.
func SelectorLabelsForApp(name string, legacy bool) labels.Set {
	if legacy {
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
// model name.
func LabelsForModel(name string, legacy bool) labels.Set {
	if legacy {
		return map[string]string{
			constants.LegacyLabelModelName: name,
		}
	}
	return map[string]string{
		constants.LabelJujuModelName: name,
	}
}

// LabelsForOperator returns the labels that should be placed on a juju operator
// Takes the operator name, type and a legacy flag to indicate these labels are
// being used on a model that is operating in "legacy" label mode.
func LabelsForOperator(name, target string, legacy bool) labels.Set {
	if legacy {
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
func LabelsForStorage(name string, legacy bool) labels.Set {
	if legacy {
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

// StorageNameFromLabels returns the name of the Juju storage
// from the supplied labels.
func StorageNameFromLabels(labels labels.Set) string {
	if labels[constants.LabelJujuStorageName] != "" {
		return labels[constants.LabelJujuStorageName]
	}
	return labels[constants.LegacyLabelStorageName]
}
