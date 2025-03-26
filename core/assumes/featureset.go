// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package assumes

import (
	"github.com/juju/collections/set"

	chassumes "github.com/juju/juju/internal/charm/assumes"
	"github.com/juju/juju/internal/version"
)

// Feature identifies a particular piece of functionality provided by a Juju
// controller depending on the substrate associated with a particular model.
type Feature struct {
	// The name of the featureflag.
	Name string

	// A user-friendly description of what the feature provides.
	Description string

	// An optional semantic version for this featureflag. It can be left empty
	// to signify that a particular feature is available without explicitly
	// specifying a version
	Version *version.Number
}

// FeatureSet describes a set of features supported by a particular model.
type FeatureSet struct {
	features map[string]Feature
}

// Merge the features from other into this feature set.
func (fs *FeatureSet) Merge(other FeatureSet) {
	for _, feat := range other.features {
		fs.Add(feat)
	}
}

// AsList returns the contents of this set as a list sorted by feature name.
func (fs *FeatureSet) AsList() []Feature {
	if fs == nil {
		return nil
	}
	set := set.NewStrings()
	for featName := range fs.features {
		set.Add(featName)
	}

	var list []Feature
	for _, featName := range set.SortedValues() {
		list = append(list, fs.features[featName])
	}

	return list
}

// Add a list of Features to the feature set. Duplicate feature entries
// will be ignored.
func (fs *FeatureSet) Add(features ...Feature) {
	if fs.features == nil {
		fs.features = make(map[string]Feature)
	}

	for _, feat := range features {
		fs.features[feat.Name] = feat
	}
}

// Get a feature with the provide feature name. The method returns a boolean
// value to indicate if the feature was found.
func (fs FeatureSet) Get(featName string) (Feature, bool) {
	if fs.features == nil {
		return Feature{}, false
	}

	feat, found := fs.features[featName]
	return feat, found
}

// Satisfies checks whether the feature set contents satisfy the provided
// "assumes" expression tree and returns an error otherwise.
func (fs FeatureSet) Satisfies(assumesExprTree *chassumes.ExpressionTree) error {
	if assumesExprTree == nil || assumesExprTree.Expression == nil {
		return nil // empty expressions are implicitly satisfied
	}

	return satisfyExpr(fs, assumesExprTree.Expression, 0)
}
