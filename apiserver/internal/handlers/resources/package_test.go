// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

//go:generate go run github.com/canonical/gomock/mockgen -package resources -destination resource_opener_mock_test.go github.com/juju/juju/core/resource Opener
//go:generate go run github.com/canonical/gomock/mockgen -package resources -destination service_mock_test.go github.com/juju/juju/apiserver/internal/handlers/resources ResourceServiceGetter,ResourceService,ApplicationServiceGetter,ApplicationService,ModelServiceGetter,ModelService,ResourceOpenerGetter,Downloader
