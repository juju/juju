// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/clientfacade_mock.go github.com/juju/juju/api/base APICallCloser,ClientFacade
//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/caller_mock.go github.com/juju/juju/api/base APICaller,FacadeCaller
