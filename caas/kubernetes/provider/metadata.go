// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

// newLabelRequirements creates a list of k8s node label requirements.
// This should be called inside package init function to panic earlier
// if there is a invalid requirement definition.
func newLabelRequirements(rs ...requirementParams) k8slabels.Selector {
	s := k8slabels.NewSelector()
	for _, r := range rs {
		l, err := k8slabels.NewRequirement(r.key, r.operator, r.strValues)
		if err != nil {
			// this panic only happens if the compiled code is wrong.
			panic(errors.Annotatef(err, "incorrect requirement config %v", r))
		}
		s = s.Add(*l)
	}
	return s
}

// requirementParams defines parameters used to create a k8s label requirement.
type requirementParams struct {
	key       string
	operator  selection.Operator
	strValues []string
}

func getCloudProviderFromNodeMeta(node core.Node) string {
	for k, checker := range k8sCloudCheckers {
		if checker.Matches(k8slabels.Set(node.GetLabels())) {
			return k
		}
	}
	return ""
}
