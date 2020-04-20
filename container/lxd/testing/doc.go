// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// This package contains the testing infrastructure to mock out the lxd API.
// run 'go generate' to regenerate the mock interfaces

package testing

//go:generate go run github.com/golang/mock/mockgen -package testing -destination lxd_mock.go github.com/lxc/lxd/client Operation,RemoteOperation,Server,ImageServer,ContainerServer
