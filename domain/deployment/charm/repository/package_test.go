// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package repository

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/charmhub_client_mock.go github.com/juju/juju/domain/deployment/charm/repository CharmHubClient
