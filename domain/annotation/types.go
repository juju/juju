// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotation

import "github.com/juju/errors"

// GetCharmArgs holds the arguments for the GetCharm method.
type GetCharmArgs struct {
	Source   string
	Name     string
	Revision int
}

// Validate checks if the GetCharmArgs are valid or not.
func (a GetCharmArgs) Validate() error {
	if a.Source == "" {
		return errors.NotValidf("source")
	}
	if a.Name == "" {
		return errors.NotValidf("name")
	}
	if a.Revision < 0 {
		return errors.NotValidf("negative revision")
	}
	return nil
}
