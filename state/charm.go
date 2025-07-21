// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	applicationcharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/internal/charm"
)

// CharmService represents a service for retrieving charms.
type CharmService interface {
	// GetCharm returns the charm by name, source and revision. Calling this method
	// will return all the data associated with the charm. It is not expected to
	// call this method for all calls, instead use the move focused and specific
	// methods. That's because this method is very expensive to call. This is
	// implemented for the cases where all the charm data is needed; model
	// migration, charm export, etc.
	GetCharm(ctx context.Context, locator applicationcharm.CharmLocator) (charm.Charm, applicationcharm.CharmLocator, bool, error)
}

// Channel identifies and describes completely a store channel.
type Channel struct {
	Track  string `bson:"track,omitempty"`
	Risk   string `bson:"risk"`
	Branch string `bson:"branch,omitempty"`
}

// Base identifies the base os the charm was installed on.
type Base struct {
	OS      string `bson:"os"`
	Channel string `bson:"channel"`
}

func (b Base) String() string {
	if b.OS == "" || b.Channel == "" {
		return ""
	}
	return fmt.Sprintf("%s@%s", b.OS, b.Channel)
}
