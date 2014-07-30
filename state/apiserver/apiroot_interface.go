package apiserver

import (
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/apiserver/common"
)

// apiRoot describes an API root after login.
type apiRoot interface {
	getResources() *common.Resources
	getRpcConn() *rpc.Conn
	DescribeFacades() []params.FacadeVersions
	rpc.Killer
	rpc.MethodFinder
	common.Authorizer
}
