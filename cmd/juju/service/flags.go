// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/storage"
)

type storageFlag struct {
	stores       *map[string]storage.Constraints
	bundleStores *map[string]map[string]storage.Constraints
}

// Set implements gnuflag.Value.Set.
func (f storageFlag) Set(s string) error {
	fields := strings.SplitN(s, "=", 2)
	if len(fields) < 2 {
		return errors.New("expected [<service>:]<store>=<constraints>")
	}
	var serviceName, storageName string
	if colon := strings.IndexRune(fields[0], ':'); colon >= 0 {
		serviceName = fields[0][:colon]
		storageName = fields[0][colon+1:]
	} else {
		storageName = fields[0]
	}
	cons, err := storage.ParseConstraints(fields[1])
	if err != nil {
		return errors.Annotate(err, "cannot parse disk constraints")
	}
	var stores map[string]storage.Constraints
	if serviceName != "" {
		if *f.bundleStores == nil {
			*f.bundleStores = make(map[string]map[string]storage.Constraints)
		}
		stores = (*f.bundleStores)[serviceName]
		if stores == nil {
			stores = make(map[string]storage.Constraints)
			(*f.bundleStores)[serviceName] = stores
		}
	} else {
		if *f.stores == nil {
			*f.stores = make(map[string]storage.Constraints)
		}
		stores = *f.stores
	}
	stores[storageName] = cons
	return nil
}

// String implements gnuflag.Value.String.
func (f storageFlag) String() string {
	strs := make([]string, 0, len(*f.stores)+len(*f.bundleStores))
	for store, cons := range *f.stores {
		strs = append(strs, fmt.Sprintf("%s=%v", store, cons))
	}
	for service, stores := range *f.bundleStores {
		for store, cons := range stores {
			strs = append(strs, fmt.Sprintf("%s:%s=%v", service, store, cons))
		}
	}
	return strings.Join(strs, " ")
}

// stringMap is a type that deserializes a CLI string using gnuflag's Value
// semantics.  It expects a name=value pair, and supports multiple copies of the
// flag adding more pairs, though the names must be unique.
type stringMap struct {
	mapping *map[string]string
}

// Set implements gnuflag.Value's Set method.
func (m stringMap) Set(s string) error {
	if *m.mapping == nil {
		*m.mapping = map[string]string{}
	}
	// make a copy so the following code is less ugly with dereferencing.
	mapping := *m.mapping

	vals := strings.SplitN(s, "=", 2)
	if len(vals) != 2 {
		return errors.NewNotValid(nil, "badly formatted name value pair: "+s)
	}
	name, value := vals[0], vals[1]
	if _, ok := mapping[name]; ok {
		return errors.Errorf("duplicate name specified: %q", name)
	}
	mapping[name] = value
	return nil
}

// String implements gnuflag.Value's String method
func (m stringMap) String() string {
	pairs := make([]string, 0, len(*m.mapping))
	for name, value := range *m.mapping {
		pairs = append(pairs, name+"="+value)
	}
	return strings.Join(pairs, ";")
}
