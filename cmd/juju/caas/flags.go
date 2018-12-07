// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
)

type regionsFlag struct {
	cloudName *string
	regions   *set.Strings
}

// Set implements gnuflag.Value.Set.
func (f regionsFlag) Set(str string) error {
	setOne := func(s string) error {
		fields := strings.SplitN(s, "/", 2)
		if len(fields) != 2 || fields[0] == "" || fields[1] == "" {
			return errors.New("expected <cloud>/<region>")
		}
		if *f.cloudName != "" && *f.cloudName != fields[0] {
			return errors.New("regions are expected in same cloud")
		}
		*f.cloudName = fields[0]
		if *f.regions == nil {
			*f.regions = set.NewStrings()
		}
		(*f.regions).Add(fields[1])
		return nil
	}
	for _, v := range strings.SplitN(str, ",", -1) {
		if err := setOne(v); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// String implements gnuflag.Value.String.
func (f regionsFlag) String() string {
	var items []string
	for v := range *f.regions {
		items = append(items, fmt.Sprintf("%s/%s", f.cloudName, v))
	}
	return strings.Join(items, " ")
}
