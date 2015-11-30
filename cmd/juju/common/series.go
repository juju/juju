// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
)

// SeriesValue may be used with gnuflag.FlagSet.Var() for a series option.
type SeriesValue struct {
	*cmd.StringsValue
}

// NewSeriesValue is used to create the type passed into the gnuflag.FlagSet Var function.
func NewSeriesValue(defaultValue []string, target *[]string) *SeriesValue {
	v := SeriesValue{(*cmd.StringsValue)(target)}
	*(v.StringsValue) = defaultValue
	return &v
}

// Set implements gnuflag.Value.
func (v *SeriesValue) Set(s string) error {
	if err := v.StringsValue.Set(s); err != nil {
		return err
	}
	for _, name := range *(v.StringsValue) {
		if !charm.IsValidSeries(name) {
			v.StringsValue = nil
			return errors.Errorf("invalid series name %q", name)
		}
	}
	return nil
}
