// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/deployment"
)

// Application represents the status of an application.
type Application struct {
	Life         life.Value
	Status       status.StatusInfo
	Relations    []relation.UUID
	Subordinate  bool
	CharmLocator charm.CharmLocator
	CharmVersion string
	Platform     deployment.Platform
	Channel      *deployment.Channel
}
