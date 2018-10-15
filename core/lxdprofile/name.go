package lxdprofile

import (
	"fmt"
	"sort"

	"github.com/juju/juju/juju/names"
)

// Name returns a serialisable name that we can use to identify profiles
// juju-<model>-<application>-<charm-revision>
func Name(modelName, appName string, revision int) string {
	return fmt.Sprintf("%s-%s-%s-%d", names.Juju, modelName, appName, revision)
}

var defaultLXDProfileNames = []string{
	"default",
}

// LXDProfileNames ensures that the LXD profile names are unique and are sorted
// correctly. It removes certain profile names from the list, for example
// "default" profile name will be removed.
//
// The function aims to preserve the order from which it got the results from.
func LXDProfileNames(names []string) []string {
	// ensure that the ones we have are unique
	unique := make(map[string]int)
	for k, v := range names {
		if contains(defaultLXDProfileNames, v) {
			continue
		}
		unique[v] = k
	}
	i := 0
	unordered := make([]nameIndex, len(unique))
	for k, v := range unique {
		unordered[i] = nameIndex{
			Name:  k,
			Index: v,
		}
		i++
	}
	sort.Slice(unordered, func(i, j int) bool {
		return unordered[i].Index < unordered[j].Index
	})
	ordered := make([]string, len(unordered))
	for k, v := range unordered {
		ordered[k] = v.Name
	}
	return ordered
}

type nameIndex struct {
	Name  string
	Index int
}

func contains(a []string, b string) bool {
	for _, v := range a {
		if v == b {
			return true
		}
	}
	return false
}
