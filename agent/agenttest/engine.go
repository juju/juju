// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttest

import (
	"github.com/juju/collections/set"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/dependency"
)

// AssertManifoldsDependencies asserts that given manifolds have expected dependencies.
func AssertManifoldsDependencies(c *tc.C, manifolds dependency.Manifolds, expected map[string][]string) {
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
	c.Assert(names.Difference(expectedNames), tc.DeepEquals, empty)
	// Missing names...
	c.Assert(expectedNames.Difference(names), tc.DeepEquals, empty)

	for _, n := range manifoldNames.SortedValues() {
		if !c.Check(dependencies[n], tc.SameContents, expected[n], tc.Commentf("mismatched dependencies for worker %q", n)) {
			// Make life easier when attempting to interpret the output.
			// We already know the answer, just tell us what to do!
			add := set.NewStrings(dependencies[n]...).Difference(set.NewStrings(expected[n]...)).SortedValues()
			remove := set.NewStrings(expected[n]...).Difference(set.NewStrings(dependencies[n]...)).SortedValues()
			if len(add) == 0 && len(remove) == 0 {
				c.Logf(" > changes required for %q:\n    - remove duplicate dependencies\n", n)
			} else {
				c.Logf(" > changes required for %q:\n    - add dependencies: %v\n    - remove dependencies %v\n", n, add, remove)
			}
		}
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
