// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8sutils

import (
	k8slabels "k8s.io/apimachinery/pkg/labels"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
)

func LabelSetToSelector(labels k8slabels.Set) k8slabels.Selector {
	return k8slabels.SelectorFromValidatedSet(labels)
}

// AppendLabels adds the labels defined in src to dest returning the result.
// Overlapping keys in sources maps are overwritten by the very last defined
// value for a duplicate key.
func AppendLabels(dest map[string]string, sources ...map[string]string) map[string]string {
	if dest == nil {
		dest = map[string]string{}
	}
	if sources == nil {
		return dest
	}
	for _, s := range sources {
		for k, v := range s {
			dest[k] = v
		}
	}
	return dest
}

// LabelsForApp returns the labels that should be on a k8s object for a given
// application name
func LabelsForApp(name string) map[string]string {
	return map[string]string{
		k8sconstants.LabelApplication: name,
	}
}

// LabelsForModel returns the labels that should be on a k8s object for a given
// model name
func LabelsForModel(name string) map[string]string {
	return map[string]string{
		k8sconstants.LabelModel: name,
	}
}
