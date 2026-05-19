// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret

//go:generate go run github.com/canonical/gomock/mockgen -package secret -destination backend_mock_test.go github.com/juju/juju/domain/secret/service SecretBackendState
