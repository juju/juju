// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater

//go:generate go run github.com/canonical/gomock/mockgen -package certupdater -destination package_mock_test.go github.com/juju/juju/internal/worker/certupdater ControllerNodeService,ControllerDomainServices
//go:generate go run github.com/canonical/gomock/mockgen -package certupdater -destination pki_mock_test.go github.com/juju/juju/internal/pki Authority,LeafRequest
