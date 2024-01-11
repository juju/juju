// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttest

import (
	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/dependency"
	gc "gopkg.in/check.v1"
)

// AssertManifoldsDependencies asserts that given manifolds have expected dependencies.
func AssertManifoldsDependencies(c *gc.C, manifolds dependency.Manifolds, expected map[string][]string) {
	dependencies := make(map[string][]string, len(manifolds))
	manifoldNames := set.NewStrings()

	for name, manifold := range manifolds {
		manifoldNames.Add(name)
		dependencies[name] = ManifoldDependencies(manifolds, manifold).SortedValues()
	}

	empty := set.NewStrings()
	names := set.NewStrings(keys(dependencies)...)
	expectedNames := set.NewStrings(keys(expected)...)
	// Unexpected names...
	c.Assert(names.Difference(expectedNames), gc.DeepEquals, empty)
	// Missing names...
	c.Assert(expectedNames.Difference(names), gc.DeepEquals, empty)

	for _, n := range manifoldNames.SortedValues() {
		c.Check(dependencies[n], jc.SameContents, expected[n], gc.Commentf("mismatched dependencies for worker %q", n))
	}
}

// ManifoldDependencies returns all - direct and indirect - manifold dependencies.
func ManifoldDependencies(all dependency.Manifolds, manifold dependency.Manifold) set.Strings {
	result := set.NewStrings()
	for _, input := range manifold.Inputs {
		result.Add(input)
		result = result.Union(ManifoldDependencies(all, all[input]))
	}
	return result
}

func keys(items map[string][]string) []string {
	result := make([]string, 0, len(items))
	for key := range items {
		result = append(result, key)
	}
	return result
}
