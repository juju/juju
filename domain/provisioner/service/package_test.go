// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

//go:generate go run github.com/canonical/gomock/mockgen -package service -destination service_mock_test.go -source=./service.go
//go:generate go run github.com/canonical/gomock/mockgen -package service -destination imagemetadata_mock_test.go -source=./imagemetadatafetcher.go
