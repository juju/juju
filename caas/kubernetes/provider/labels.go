// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	core "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/juju/errors"
)

const (
	// LabelKubernetesAppName is the common meta key for kubernetes app names.
	// See https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/#labels
	LabelKubernetesAppName = "app.kubernetes.io/name"

	// LabelKubernetesAppManaged is the common meta key for kubernetes apps
	// that are managed by a non k8s process (such as Juju).
	// See https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/#labels
	LabelKubernetesAppManaged = "app.kubernetes.io/managed-by"

	// LabelJujuAppCreatedBy is a Juju application label to apply to objects
	// created by applications managed by Juju. Think istio, kubeflow etc
	// See https://bugs.launchpad.net/juju/+bug/1892285
	LabelJujuAppCreatedBy = "app.juju.is/created-by"

	// LabelJujuModelName is the juju label applied for juju models.
	LabelJujuModelName = "model.juju.is/name"

	// LabelJujuOperatorName is the juju label applied to Juju operators to
	// identify their name. Operator names are generally named after the thing
	// the operator is controlling. i.e an operator name for a model test would be
	// "test"
	LabelJujuOperatorName = "operator.juju.is/name"

	// LabelJujuOperatorTarget is the juju label applied to Juju operators to
	// describe the modeling paradigm they target. For example model,
	// application
	LabelJujuOperatorTarget = "operator.juju.is/target"

	// LabelJujuStorageName is the juju label applied to Juju storage objects to
	// describe their name.
	LabelJujuStorageName = "storage.juju.is/name"

	// legacyLabelKubernetesAppName is the legacy label key used for juju app
	// identification. This purely exists to maintain backwards functionality.
	// See https://bugs.launchpad.net/juju/+bug/1888513
	legacyLabelKubernetesAppName = "juju-app"

	// legacyLabelModelName is the legacy label key used for juju models. This
	// purely exists to maintain backwards functionality.
	// See https://bugs.launchpad.net/juju/+bug/1888513
	legacyLabelModelName = "juju-model"

	// legacyLabelKubernetesOperatorName is the legacy label key used for juju
	// operators. This purely exists to maintain backwards functionality.
	// See https://bugs.launchpad.net/juju/+bug/1888513
	legacyLabelKubernetesOperatorName = "juju-operator"

	// legacyLabelJujuStorageName is the legacy label key used for juju storage
	// pvc. This purely exists to maintain backwards functionality.
	// See https://bugs.launchpad.net/juju/+bug/1888513
	legacyLabelStorageName = "juju-storage"
)

var (
	// LabelsJuju is a common set
	LabelsJuju = map[string]string{
		LabelKubernetesAppManaged: "juju",
	}
)

func AppLabelSelector(appName string) (labels.Selector, error) {
	return labels.Parse(fmt.Sprintf("%s=%s", LabelKubernetesAppName, appName))
}

func (k *kubernetesClient) getlabelsForApp(appName string, isNamespaced bool) map[string]string {
	labels := LabelsForApp(appName, k.IsLegacyLabels())
	if !isNamespaced {
		labels = LabelsMerge(labels, LabelsForModel(k.CurrentModel(), k.IsLegacyLabels()))
	}
	return labels
}

// HasLabels returns true if the src contains the labels in has
func HasLabels(src, has labels.Set) bool {
	for k, v := range has {
		if src[k] != v {
			return false
		}
	}
	return true
}

// LabelSetToSelector converts a set of Kubernetes labels
func LabelSetToSelector(l labels.Set) labels.Selector {
	return labels.SelectorFromValidatedSet(l)
}

// LabelsForApp returns the labels that should be on a k8s object for a given
// application name
func LabelsForApp(name string, legacy bool) labels.Set {
	if legacy {
		return labels.Set{
			legacyLabelKubernetesAppName: name,
		}
	}
	return labels.Set{
		LabelKubernetesAppName: name,
	}
}

// LabelsForModel returns the labels that should be on a k8s object for a given
// model name
func LabelsForModel(name string, legacy bool) labels.Set {
	if legacy {
		return map[string]string{
			legacyLabelModelName: name,
		}
	}
	return map[string]string{
		LabelJujuModelName: name,
	}
}

// LabelsForOperator returns the labels that should be placed on a juju operator
// Takes the operator name, type and a legacy flag to indicate these labels are
// being used on a model that is operating in "legacy" label mode
func LabelsForOperator(name, target string, legacy bool) labels.Set {
	if legacy {
		return map[string]string{
			legacyLabelKubernetesOperatorName: name,
		}
	}
	return map[string]string{
		LabelJujuOperatorName:   name,
		LabelJujuOperatorTarget: target,
	}
}

// LabelsForStorage return the labels that should be placed on a k8s storage
// object. Takes the storage name and a legacy flat.
func LabelsForStorage(name string, legacy bool) labels.Set {
	if legacy {
		return map[string]string{
			legacyLabelStorageName: name,
		}
	}
	return map[string]string{
		LabelJujuStorageName: name,
	}
}

func LabelForKeyValue(key, value string) labels.Set {
	return labels.Set{
		key: value,
	}
}

// LabelsMerge
func LabelsMerge(a labels.Set, merges ...labels.Set) labels.Set {
	for _, merge := range merges {
		a = labels.Merge(a, merge)
	}
	return a
}

// LabelsToSelector transforms the supplied label set to a valid Kubernetes
// label selector
func LabelsToSelector(ls labels.Set) labels.Selector {
	return labels.SelectorFromValidatedSet(ls)
}

// IsLegacyLabels indicates if this provider is operating on a legacy label schema
func (k *kubernetesClient) IsLegacyLabels() bool {
	return k.isLegacyLabels
}

// IsLegacyModelLabels checks to see if the provided model is running on an older
// labeling scheme or a newer one.
func IsLegacyModelLabels(model string, namespaceI core.NamespaceInterface) (bool, error) {
	ns, err := namespaceI.Get(model, meta.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return true, errors.Annotatef(err, "unable to determine legacy status for namespace %s", model)
	}

	return !HasLabels(ns.Labels, LabelsForModel(model, false)), nil
}

func StorageNameFromLabels(labels labels.Set) string {
	if labels[LabelJujuStorageName] != "" {
		return labels[LabelJujuStorageName]
	}
	return labels[legacyLabelStorageName]
}
