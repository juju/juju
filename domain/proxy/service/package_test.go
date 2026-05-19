// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

//go:generate go run github.com/canonical/gomock/mockgen -package service -destination proxy_mock_test.go github.com/juju/juju/internal/proxy Proxier
//go:generate go run github.com/canonical/gomock/mockgen -package service -destination provider_mock_test.go github.com/juju/juju/domain/proxy/service Provider
