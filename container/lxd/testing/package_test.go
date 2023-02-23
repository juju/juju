// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

//go:generate go run github.com/golang/mock/mockgen -package testing -destination lxd_mock.go -write_package_comment=false github.com/lxc/lxd/client Operation,RemoteOperation,Server,ImageServer,InstanceServer
