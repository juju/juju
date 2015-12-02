// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

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

// Set implements gnuflag.Value.String.
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
