// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

//go:generate go run github.com/canonical/gomock/mockgen -package providertracker -destination provider_mock_test.go github.com/juju/juju/core/providertracker ProviderFactory,Provider
