// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret

//go:generate go run go.uber.org/mock/mockgen -typed -package secret -destination backend_mock_test.go github.com/juju/juju/domain/secret/service SecretBackendState
