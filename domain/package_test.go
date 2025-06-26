// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

//go:generate go run go.uber.org/mock/mockgen -typed -package domain -destination changestream_mock_test.go github.com/juju/juju/core/changestream Subscription,EventSource
//go:generate go run go.uber.org/mock/mockgen -typed -package domain -destination lease_mock_test.go github.com/juju/juju/core/lease Token,Checker,Revoker,LeaseManager,ModelLeaseManagerGetter
