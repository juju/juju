// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/internal/provider/lxd/lxdnames"
)

// WhiteList contains a cloud compatibility matrix:
// if controller was bootstrapped on a particular cloud type,
// what other cloud types can be added to it.
type WhiteList struct {
	matrix map[string]set.Strings
}

// String constructs user friendly white list representation.
func (w *WhiteList) String() string {
	if len(w.matrix) == 0 {
		return "empty whitelist"
	}
	sorted := []string{}
	for one := range w.matrix {
		sorted = append(sorted, one)
	}
	sort.Strings(sorted)
	result := []string{}
	for _, one := range sorted {
		result = append(result, fmt.Sprintf(" - controller cloud type %q supports %v", one, w.matrix[one].SortedValues()))
	}
	return strings.Join(result, "\n")
}

// Check will err out if 'existing' controller cloud type is
// not compatible with a 'new' cloud type according to this white list.
func (w *WhiteList) Check(existing, new string) error {
	if list, ok := w.matrix[existing]; ok {
		if !list.Contains(new) {
			return errors.Errorf("cloud type %q is not whitelisted for controller cloud type %q, current whitelist: %v", new, existing, list.SortedValues())
		}
		return nil
	}
	return errors.Errorf("controller cloud type %q is not whitelisted, current whitelist: \n%v", existing, w)
}

// CurrentWhiteList returns current clouds whitelist supported by Juju.
func CurrentWhiteList() *WhiteList {
	return &WhiteList{map[string]set.Strings{
		"kubernetes":          set.NewStrings(lxdnames.ProviderType, "maas", "openstack"),
		lxdnames.ProviderType: set.NewStrings(lxdnames.ProviderType, "maas", "openstack"),
		"maas":                set.NewStrings("maas", "openstack"),
		"openstack":           set.NewStrings("openstack"),
	}}
}
