// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
)

type singularSecretary struct {
	uuid string
}

func (s singularSecretary) CheckLease(name string) error {
	if name != s.uuid {
		return errors.New("expected environ UUID")
	}
	return nil
}

func (s singularSecretary) CheckHolder(name string) error {
	if _, err := names.ParseMachineTag(name); err != nil {
		return errors.New("expected machine tag")
	}
	return nil
}

func (s singularSecretary) CheckDuration(duration time.Duration) error {
	return nil
}
