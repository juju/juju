// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

func newLabelRequirements(rs ...requirement) k8slabels.Selector {
	s := k8slabels.NewSelector()
	for _, r := range rs {
		l, err := k8slabels.NewRequirement(r.key, r.operator, r.strValues)
		if err != nil {
			panic(errors.Annotatef(err, "incorrect requirement config %v", r))
		}
		s = s.Add(*l)
	}
	return s
}

type requirement struct {
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
