// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdprofile

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// AppName here is used as the application prefix name. We can't use names.Juju
// as that changes depending on platform.
const AppName = "juju"

// Prefix is used to prefix all the lxd profile programmable profiles. If a
// profile doesn't have the prefix, then it will be removed when ensuring the
// the validity of the names (see FilterLXDProfileNames)
var Prefix = fmt.Sprintf("%s-", AppName)

// Name returns a serialisable name that we can use to identify profiles
// juju-<model>-<application>-<charm-revision>
func Name(modelName, appName string, revision int) string {
	return fmt.Sprintf("%s%s-%s-%d", Prefix, modelName, appName, revision)
}

// FilterLXDProfileNames ensures that the LXD profile names are unique yet preserve
// the same order as the input. It removes certain profile names from the list,
// for example "default" profile name will be removed.
func FilterLXDProfileNames(names []string) []string {
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

// ProfileRevision returns an int which is the charm revision of the given
// profile name.
func ProfileRevision(profile string) (int, error) {
	if !IsValidName(profile) {
		return 0, errors.Errorf("not a juju profile name: %q", profile).Add(coreerrors.BadRequest)
	}
	split := strings.Split(profile, "-")
	rev := split[len(split)-1:]
	return strconv.Atoi(rev[0])
}

// ProfileReplaceRevision replaces the old revision with a new revision
// in the profile.
func ProfileReplaceRevision(profile string, rev int) (string, error) {
	if !IsValidName(profile) {
		return "", errors.Errorf("not a juju profile name: %q", profile).Add(coreerrors.BadRequest)
	}
	split := strings.Split(profile, "-")
	notRev := split[:len(split)-1]
	return strings.Join(append(notRev, strconv.Itoa(rev)), "-"), nil
}

// MatchProfileNameByApp returns the first profile which matches the provided
// appName.  No match returns an empty string.
// Assumes there is not more than one profile for the same application.
func MatchProfileNameByAppName(names []string, appName string) (string, error) {
	if appName == "" {
		return "", errors.Errorf("no application name specified").Add(coreerrors.BadRequest)
	}
	var foundProfile string
	for _, p := range FilterLXDProfileNames(names) {
		rev, err := ProfileRevision(p)
		if err != nil {
			// "Shouldn't" happen since we used FilterLXDProfileNames...
			if errors.Is(err, coreerrors.BadRequest) {
				continue
			}
			return "", err
		}
		if strings.HasSuffix(p, fmt.Sprintf("-%s-%d", appName, rev)) {
			foundProfile = p
			break
		}
	}
	return foundProfile, nil
}
