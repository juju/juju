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
	stores *map[string]storage.Constraints
}

// Set implements gnuflag.Value.Set.
func (f storageFlag) Set(s string) error {
	fields := strings.SplitN(s, "=", 2)
	if len(fields) < 2 {
		return errors.New("expected <store>=<constraints>")
	}
	cons, err := storage.ParseConstraints(fields[1])
	if err != nil {
		return errors.Annotate(err, "cannot parse disk constraints")
	}
	if *f.stores == nil {
		*f.stores = make(map[string]storage.Constraints)
	}
	(*f.stores)[fields[0]] = cons
	return nil
}

// Set implements gnuflag.Value.String.
func (f storageFlag) String() string {
	strs := make([]string, 0, len(*f.stores))
	for store, cons := range *f.stores {
		strs = append(strs, fmt.Sprintf("%s=%v", store, cons))
	}
	return strings.Join(strs, " ")
}
