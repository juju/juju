// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
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

func init() {
	common.RegisterStandardFacade(
		FacadeName,
		2,
		NewLeadershipServiceFacade,
	)
}

// NewLeadershipServiceFacade constructs a new LeadershipService and presents
// a signature that can be used with RegisterStandardFacade.
func NewLeadershipServiceFacade(
	state *state.State, resources *common.Resources, authorizer common.Authorizer,
) (LeadershipService, error) {
	return NewLeadershipService(state.LeadershipClaimer(), authorizer)
}

// NewLeadershipService constructs a new LeadershipService.
func NewLeadershipService(
	claimer leadership.Claimer, authorizer common.Authorizer,
) (LeadershipService, error) {

	if !authorizer.AuthUnitAgent() {
		return nil, errors.Unauthorizedf("permission denied")
	}

	return &leadershipService{
		claimer:    claimer,
		authorizer: authorizer,
	}, nil
}

// leadershipService implements the LeadershipService interface and
// is the concrete implementation of the API endpoint.
type leadershipService struct {
	claimer    leadership.Claimer
	authorizer common.Authorizer
}

// ClaimLeadership is part of the LeadershipService interface.
func (m *leadershipService) ClaimLeadership(args params.ClaimLeadershipBulkParams) (params.ClaimLeadershipBulkResults, error) {

	results := make([]params.ErrorResult, len(args.Params))
	for pIdx, p := range args.Params {

		result := &results[pIdx]
		ApplicationTag, unitTag, err := parseServiceAndUnitTags(p.ApplicationTag, p.UnitTag)
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
		// claim leadership for themselves, for their own service.
		if !m.authorizer.AuthOwner(unitTag) || !m.authMember(ApplicationTag) {
			result.Error = common.ServerError(common.ErrPerm)
			continue
		}

		err = m.claimer.ClaimLeadership(ApplicationTag.Id(), unitTag.Id(), duration)
		if err != nil {
			result.Error = common.ServerError(err)
		}
	}

	return params.ClaimLeadershipBulkResults{results}, nil
}

// BlockUntilLeadershipReleased implements the LeadershipService interface.
func (m *leadershipService) BlockUntilLeadershipReleased(ApplicationTag names.ApplicationTag) (params.ErrorResult, error) {
	if !m.authMember(ApplicationTag) {
		return params.ErrorResult{Error: common.ServerError(common.ErrPerm)}, nil
	}

	if err := m.claimer.BlockUntilLeadershipReleased(ApplicationTag.Id()); err != nil {
		return params.ErrorResult{Error: common.ServerError(err)}, nil
	}
	return params.ErrorResult{}, nil
}

func (m *leadershipService) authMember(ApplicationTag names.ApplicationTag) bool {
	ownerTag := m.authorizer.GetAuthTag()
	unitTag, ok := ownerTag.(names.UnitTag)
	if !ok {
		return false
	}
	unitId := unitTag.Id()
	requireServiceId, err := names.UnitApplication(unitId)
	if err != nil {
		return false
	}
	return ApplicationTag.Id() == requireServiceId
}

// parseServiceAndUnitTags takes in string representations of service
// and unit tags and returns their corresponding tags.
func parseServiceAndUnitTags(
	ApplicationTagString, unitTagString string,
) (
	names.ApplicationTag, names.UnitTag, error,
) {
	// TODO(fwereade) 2015-02-25 bug #1425506
	// These permissions errors are not appropriate -- there's no permission or
	// security issue in play here, because our tag format is public, and the
	// error only triggers when the strings fail to match that format.
	ApplicationTag, err := names.ParseApplicationTag(ApplicationTagString)
	if err != nil {
		return names.ApplicationTag{}, names.UnitTag{}, common.ErrPerm
	}

	unitTag, err := names.ParseUnitTag(unitTagString)
	if err != nil {
		return names.ApplicationTag{}, names.UnitTag{}, common.ErrPerm
	}

	return ApplicationTag, unitTag, nil
}
