// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"fmt"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/storage"
)

type disksFlag struct {
	disks *[]storage.Constraints
}

// Set implements gnuflag.Value.Set.
func (f disksFlag) Set(s string) error {
	for _, field := range strings.Fields(s) {
		cons, err := storage.ParseConstraints(field)
		if err != nil {
			return errors.Annotate(err, "cannot parse disk constraints")
		}
		*f.disks = append(*f.disks, cons)
	}
	return nil
}

// Set implements gnuflag.Value.String.
func (f disksFlag) String() string {
	strs := make([]string, len(*f.disks))
	for i, cons := range *f.disks {
		strs[i] = fmt.Sprint(cons)
	}
	return strings.Join(strs, " ")
}
