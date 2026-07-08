// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package legacy

//go:generate go run github.com/canonical/gomock/mockgen -package legacy -destination services_mock_test.go github.com/juju/juju/internal/migration/legacy ProviderConfigServicesGetter,ProviderConfigServices,CloudService
