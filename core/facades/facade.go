// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facades

import "github.com/juju/collections/set"

// FacadeVersions is a map of facade name to version numbers. The facade version
// numbers contain each version of the facade that the API server is capable of
// supporting. This supports having holes in the version numbers, so that we can
// depreciate broken versions of the facade.
type FacadeVersions map[string][]int

// Merge adds the other facade versions to the current facade versions.
func (f FacadeVersions) Merge(others ...FacadeVersions) FacadeVersions {
	for _, other := range others {
		for name, versions := range other {
			f[name] = set.NewInts(f[name]...).Union(set.NewInts(versions...)).SortedValues()
		}
	}
	return f
}

// BestVersion finds the newest version in the version list that we can
// use.
func BestVersion(desired []int, versions []int) int {
	intersection := set.NewInts(desired...).Intersection(set.NewInts(versions...))
	if intersection.Size() == 0 {
		return 0
	}
	sorted := intersection.SortedValues()
	return sorted[len(sorted)-1]
}

// CompleteIntersection returns true if the src and dest facades have a
// complete intersection. This means that the dest facades support all of
// the src facades.
func CompleteIntersection(src, dest FacadeVersions) bool {
	for name, versions := range src {
		if _, ok := dest[name]; !ok {
			return false
		}
		if BestVersion(versions, dest[name]) == 0 {
			return false
		}
	}
	return true
}
