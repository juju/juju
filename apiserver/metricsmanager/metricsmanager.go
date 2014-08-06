// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsmanager

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/apiserver/common"
)

var logger = loggo.GetLogger("juju.state.apiserver.metricsmanager")

func init() {
	common.RegisterStandardFacade("MetricsManager", 0, NewMetricsManagerAPI)
}

// MetricsManager defines the methods on the metricsmanager API end point.
type MetricsManager interface {
	CleanupOldMetrics() (params.ErrorResult, error)
}

// MetricsManagerAPI implements the metrics manager interface and is the concrete
// implementation of the api end point.
type MetricsManagerAPI struct {
	state      *state.State
	authorizer common.Authorizer
}

var _ MetricsManager = (*MetricsManagerAPI)(nil)

func NewMetricsManagerAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*MetricsManagerAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &MetricsManagerAPI{
		state:      st,
		authorizer: authorizer,
	}, nil
}

// CleanupOldMetrics removes old metrics from the collection.
// TODO (mattyw) Returns result with all the delete metrics
func (api *MetricsManagerAPI) CleanupOldMetrics() (params.ErrorResult, error) {
	result := params.ErrorResult{}

	err := api.state.CleanupOldMetrics()
	if err != nil {
		err = errors.Annotate(err, "failed to cleanup old metrics")
		result.Error = common.ServerError(err)
		return result, err
	}
	return result, nil
}
