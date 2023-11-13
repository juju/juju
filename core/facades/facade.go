// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facades

import "github.com/juju/collections/set"

// FacadeVersion is a list of version numbers for a single facade.
type FacadeVersion []int

// NamedFacadeVersion is a map of facade name to version numbers.
type NamedFacadeVersion struct {
	Name     string
	Versions FacadeVersion
}

// FacadeVersions is a map of facade name to version numbers. The facade version
// numbers contain each version of the facade that the API server is capable of
// supporting. This supports having holes in the version numbers, so that we can
// depreciate broken versions of the facade.
type FacadeVersions map[string]FacadeVersion

// Merge adds the other facade versions to the current facade versions.
func (f FacadeVersions) Add(others ...NamedFacadeVersion) FacadeVersions {
	for _, other := range others {
		f[other.Name] = set.NewInts(f[other.Name]...).Union(set.NewInts(other.Versions...)).SortedValues()
	}
	return f
}

// BestVersion finds the newest version in the version list that we can
// use.
func BestVersion(desired FacadeVersion, versions FacadeVersion) int {
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
