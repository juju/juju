// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"context"

	corebackups "github.com/juju/juju/core/backups"
)

type backupCreatorFunc func(context.Context, string) (*corebackups.Metadata, string, func(), error)

func (f backupCreatorFunc) Create(ctx context.Context, notes string) (*corebackups.Metadata, string, func(), error) {
	return f(ctx, notes)
}

//go:generate go run github.com/canonical/gomock/mockgen -package apiserver_test -destination controllerconfig_mock_test.go github.com/juju/juju/internal/worker/apiserver ControllerConfigService,ModelService
//go:generate go run github.com/canonical/gomock/mockgen -package apiserver_test -destination service_mock_test.go github.com/juju/juju/internal/services DomainServicesGetter
//go:generate go run github.com/canonical/gomock/mockgen -package apiserver_test -destination provider_mock_test.go github.com/juju/juju/core/providertracker ProviderFactory,Provider
