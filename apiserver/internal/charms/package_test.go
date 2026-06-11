// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/mocks.go github.com/juju/juju/apiserver/internal/charms CharmService,ApplicationService
