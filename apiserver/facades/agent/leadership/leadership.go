// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/rpc/params"
)

const (
	// minLeaseRequest is the shortest duration for which we will accept
	// a leadership claim.
	minLeaseRequest = 5 * time.Second

	// maxLeaseRequest is the longest duration for which we will accept
	// a leadership claim.
	maxLeaseRequest = 5 * time.Minute
)

// NewLeadershipService constructs a new LeadershipService.
func NewLeadershipService(
	claimer leadership.Claimer, authorizer facade.Authorizer,
) (LeadershipService, error) {

	if !authorizer.AuthUnitAgent() && !authorizer.AuthApplicationAgent() {
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
	authorizer facade.Authorizer
}

// ClaimLeadership is part of the LeadershipService interface.
func (m *leadershipService) ClaimLeadership(args params.ClaimLeadershipBulkParams) (params.ClaimLeadershipBulkResults, error) {

	results := make([]params.ErrorResult, len(args.Params))
	for pIdx, p := range args.Params {

		result := &results[pIdx]
		applicationTag, unitTag, err := parseApplicationAndUnitTags(p.ApplicationTag, p.UnitTag)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
			continue
		}
		duration := time.Duration(p.DurationSeconds * float64(time.Second))
		if duration > maxLeaseRequest || duration < minLeaseRequest {
			result.Error = apiservererrors.ServerError(errors.New("invalid duration"))
			continue
		}

		// In the future, situations may arise wherein units will make
		// leadership claims for other units. For now, units can only
		// claim leadership for themselves, for their own service.
		authTag := m.authorizer.GetAuthTag()
		canClaim := false
		switch authTag.(type) {
		case names.UnitTag:
			canClaim = m.authorizer.AuthOwner(unitTag) && m.authMember(applicationTag)
		case names.ApplicationTag:
			canClaim = m.authorizer.AuthOwner(applicationTag)
		}
		if !canClaim {
			result.Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if err = m.claimer.ClaimLeadership(applicationTag.Id(), unitTag.Id(), duration); err != nil {
			result.Error = apiservererrors.ServerError(err)
		}
	}

	return params.ClaimLeadershipBulkResults{Results: results}, nil
}

// BlockUntilLeadershipReleased implements the LeadershipService interface.
func (m *leadershipService) BlockUntilLeadershipReleased(ctx context.Context, applicationTag names.ApplicationTag) (params.ErrorResult, error) {
	authTag := m.authorizer.GetAuthTag()
	hasPerm := false
	switch authTag.(type) {
	case names.UnitTag:
		hasPerm = m.authMember(applicationTag)
	case names.ApplicationTag:
		hasPerm = m.authorizer.AuthOwner(applicationTag)
	}

	if !hasPerm {
		return params.ErrorResult{Error: apiservererrors.ServerError(apiservererrors.ErrPerm)}, nil
	}

	if err := m.claimer.BlockUntilLeadershipReleased(applicationTag.Id(), ctx.Done()); err != nil {
		return params.ErrorResult{Error: apiservererrors.ServerError(err)}, nil
	}
	return params.ErrorResult{}, nil
}

func (m *leadershipService) authMember(applicationTag names.ApplicationTag) bool {
	ownerTag := m.authorizer.GetAuthTag()
	unitTag, ok := ownerTag.(names.UnitTag)
	if !ok {
		return false
	}
	unitId := unitTag.Id()
	requireAppName, err := names.UnitApplication(unitId)
	if err != nil {
		return false
	}
	return applicationTag.Id() == requireAppName
}

// parseApplicationAndUnitTags takes in string representations of application
// and unit tags and returns their corresponding tags.
func parseApplicationAndUnitTags(
	applicationTagString, unitTagString string,
) (
	names.ApplicationTag, names.UnitTag, error,
) {
	// TODO(fwereade) 2015-02-25 bug #1425506
	// These permissions errors are not appropriate -- there's no permission or
	// security issue in play here, because our tag format is public, and the
	// error only triggers when the strings fail to match that format.
	applicationTag, err := names.ParseApplicationTag(applicationTagString)
	if err != nil {
		return names.ApplicationTag{}, names.UnitTag{}, apiservererrors.ErrPerm
	}

	unitTag, err := names.ParseUnitTag(unitTagString)
	if err != nil {
		return names.ApplicationTag{}, names.UnitTag{}, apiservererrors.ErrPerm
	}

	return applicationTag, unitTag, nil
}
