// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

const (
	// FacadeName is the string-representation of this API used both
	// to register the service, and for the client to resolve the
	// service endpoint.
	FacadeName = "LeadershipService"

	// MinLeaseRequest is the shortest duration for which we will accept
	// a leadership claim.
	MinLeaseRequest = 5 * time.Second

	// MaxLeaseRequest is the longest duration for which we will accept
	// a leadership claim.
	MaxLeaseRequest = 5 * time.Minute
)

var (
	logger = loggo.GetLogger("juju.apiserver.leadership")
)

func init() {
	common.RegisterStandardFacade(
		FacadeName,
		1,
		NewLeadershipService,
	)
}

// NewLeadershipService constructs a new LeadershipService.
func NewLeadershipService(
	state *state.State, resources *common.Resources, authorizer common.Authorizer,
) (LeadershipService, error) {
	if !authorizer.AuthUnitAgent() {
		return nil, common.ErrPerm
	}
	return &leadershipService{
		state:      state,
		authorizer: authorizer,
	}, nil
}

// LeadershipService implements the LeadershipService interface and
// is the concrete implementation of the API endpoint.
type leadershipService struct {
	state      *state.State
	authorizer common.Authorizer
}

// ClaimLeadership implements the LeadershipService interface.
func (m *leadershipService) ClaimLeadership(args params.ClaimLeadershipBulkParams) (params.ClaimLeadershipBulkResults, error) {

	claimer := m.state.LeadershipClaimer()
	results := make([]params.ErrorResult, len(args.Params))
	for pIdx, p := range args.Params {

		result := &results[pIdx]
		serviceTag, unitTag, err := parseServiceAndUnitTags(p.ServiceTag, p.UnitTag)
		if err != nil {
			result.Error = common.ServerError(err)
			continue
		}
		duration := time.Duration(p.DurationSeconds * float64(time.Second))
		if duration > MaxLeaseRequest || duration < MinLeaseRequest {
			result.Error = common.ServerError(errors.New("invalid duration"))
			continue
		}

		// In the future, situations may arise wherein units will make
		// leadership claims for other units. For now, units can only
		// claim leadership for themselves.
		if !m.authorizer.AuthUnitAgent() || !m.authorizer.AuthOwner(unitTag) {
			result.Error = common.ServerError(common.ErrPerm)
			continue
		}

		err = claimer.ClaimLeadership(serviceTag.Id(), unitTag.Id(), duration)
		if err != nil {
			result.Error = common.ServerError(err)
		}
	}

	return params.ClaimLeadershipBulkResults{results}, nil
}

// BlockUntilLeadershipReleased implements the LeadershipService interface.
func (m *leadershipService) BlockUntilLeadershipReleased(serviceTag names.ServiceTag) (params.ErrorResult, error) {
	if !m.authorizer.AuthUnitAgent() {
		return params.ErrorResult{Error: common.ServerError(common.ErrPerm)}, nil
	}

	claimer := m.state.LeadershipClaimer()
	if err := claimer.BlockUntilLeadershipReleased(serviceTag.Id()); err != nil {
		return params.ErrorResult{Error: common.ServerError(err)}, nil
	}
	return params.ErrorResult{}, nil
}

// parseServiceAndUnitTags takes in string representations of service
// and unit tags and returns their corresponding tags.
func parseServiceAndUnitTags(
	serviceTagString, unitTagString string,
) (
	names.ServiceTag, names.UnitTag, error,
) {
	// TODO(fwereade) 2015-02-25 bug #1425506
	// These permissions errors are not appropriate -- there's no permission or
	// security issue in play here, because our tag format is public, and the
	// error only triggers when the strings fail to match that format.
	serviceTag, err := names.ParseServiceTag(serviceTagString)
	if err != nil {
		return names.ServiceTag{}, names.UnitTag{}, common.ErrPerm
	}

	unitTag, err := names.ParseUnitTag(unitTagString)
	if err != nil {
		return names.ServiceTag{}, names.UnitTag{}, common.ErrPerm
	}

	return serviceTag, unitTag, nil
}
