// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdprofile

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/juju/juju/names"
)

// Prefix is used to prefix all the lxd profile programmable profiles. If a
// profile doesn't have the prefix, then it will be removed when ensuring the
// the validity of the names (see LXDProfileNames)
var Prefix = fmt.Sprintf("%s-", names.Juju)

// Name returns a serialisable name that we can use to identify profiles
// juju-<model>-<application>-<charm-revision>
func Name(modelName, appName string, revision int) string {
	return fmt.Sprintf("%s%s-%s-%d", Prefix, modelName, appName, revision)
}

// LXDProfileNames ensures that the LXD profile names are unique yet preserve
// the same order as the input. It removes certain profile names from the list,
// for example "default" profile name will be removed.
func LXDProfileNames(names []string) []string {
	// ensure that the ones we have are unique
	unique := make(map[string]int)
	for k, v := range names {
		if !IsValidName(v) {
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

// IsValidName returns if the name of the lxd profile looks valid.
func IsValidName(name string) bool {
	// doesn't contain the prefix
	if !strings.HasPrefix(name, Prefix) {
		return false
	}
	// it's required to have at least the following chars `x-x-0`
	suffix := name[len(Prefix):]
	if len(suffix) < 5 {
		return false
	}
	// lastly check the last part is a number
	lastHyphen := strings.LastIndex(suffix, "-")
	revision := suffix[lastHyphen+1:]
	_, err := strconv.Atoi(revision)
	return err == nil
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
