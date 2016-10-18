// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package resource

import (
	"sort"
)

// Sort sorts the provided resources.
func Sort(resources []Resource) {
	sort.Sort(byName(resources))
}

type byName []Resource

func (sorted byName) Len() int           { return len(sorted) }
func (sorted byName) Swap(i, j int)      { sorted[i], sorted[j] = sorted[j], sorted[i] }
func (sorted byName) Less(i, j int) bool { return sorted[i].Name < sorted[j].Name }
