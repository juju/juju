// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

//go:generate go run github.com/canonical/gomock/mockgen -package crossmodel -destination service_mock_test.go github.com/juju/juju/apiserver/internal/handlers/crossmodel CrossModelAuthContextProvider
