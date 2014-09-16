package apiserver

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/rpc"
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
