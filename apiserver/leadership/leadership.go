// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"time"

	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/leadership"
	"github.com/juju/juju/lease"
	"github.com/juju/juju/state"
)

const (
	// FacadeName is the string-representation of this API used both
	// to register the service, and for the client to resolve the
	// service endpoint.
	FacadeName = "LeadershipService"
)

var (
	logger = loggo.GetLogger("juju.apiserver.leadership")
	// Begin injection-chain so we can instantiate leadership
	// services. Exposed as variables so we can change the
	// implementation for testing purposes.
	leaseMgr  = lease.Manager()
	leaderMgr = leadership.NewLeadershipManager(leaseMgr)
)

func init() {

	common.RegisterStandardFacade(
		FacadeName,
		0,
		NewLeadershipServiceFn(leaderMgr),
	)
}

// NewLeadershipServiceFn returns a function which can construct a
// LeadershipService when passed a state, resources, and authorizer.
// This function signature conforms to Juju's required API server
// signature.
func NewLeadershipServiceFn(
	leadershipMgr leadership.LeadershipManager,
) func(*state.State, *common.Resources, common.Authorizer) (LeadershipService, error) {
	return func(
		state *state.State,
		resources *common.Resources,
		authorizer common.Authorizer,
	) (LeadershipService, error) {
		return NewLeadershipService(state, resources, authorizer, leadershipMgr)
	}
}

// NewLeadershipService constructs a new LeadershipService.
func NewLeadershipService(
	state *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
	leadershipMgr leadership.LeadershipManager,
) (LeadershipService, error) {

	if !authorizer.AuthUnitAgent() {
		return nil, common.ErrPerm
	}

	return &leadershipService{
		state:             state,
		authorizer:        authorizer,
		LeadershipManager: leadershipMgr,
	}, nil
}

// LeadershipService implements the LeadershipManager interface and
// is the concrete implementation of the API endpoint.
type leadershipService struct {
	state      *state.State
	authorizer common.Authorizer
	leadership.LeadershipManager
}

// ClaimLeadership implements the LeadershipService interface.
func (m *leadershipService) ClaimLeadership(args params.ClaimLeadershipBulkParams) (params.ClaimLeadershipBulkResults, error) {

	var dur time.Duration
	claim := callWithIds(func(sid, uid string) (err error) {
		dur, err = m.LeadershipManager.ClaimLeadership(sid, uid)
		return err
	})

	results := make([]params.ClaimLeadershipResults, len(args.Params))
	for pIdx, p := range args.Params {

		result := &results[pIdx]
		svcTag, unitTag, err := parseServiceAndUnitTags(p.ServiceTag, p.UnitTag)
		if err != nil {
			result.Error = err
			continue
		}

		// In the future, situations may arise wherein units will make
		// leadership claims for other units. For now, units can only
		// claim leadership for themselves.
		if !m.authorizer.AuthUnitAgent() || !m.authorizer.AuthOwner(unitTag) {
			result.Error = common.ServerError(common.ErrPerm)
			continue
		} else if err := claim(svcTag, unitTag).Error; err != nil {
			result.Error = err
			continue
		}

		result.ClaimDurationInSec = dur.Seconds()
		result.ServiceTag = p.ServiceTag
	}

	return params.ClaimLeadershipBulkResults{results}, nil
}

// ReleaseLeadership implements the LeadershipService interface.
func (m *leadershipService) ReleaseLeadership(args params.ReleaseLeadershipBulkParams) (params.ReleaseLeadershipBulkResults, error) {

	release := callWithIds(m.LeadershipManager.ReleaseLeadership)
	results := make([]params.ErrorResult, len(args.Params))

	for paramIdx, p := range args.Params {

		result := &results[paramIdx]
		svcTag, unitTag, err := parseServiceAndUnitTags(p.ServiceTag, p.UnitTag)
		if err != nil {
			result.Error = err
			continue
		}

		// In the future, situations may arise wherein units will make
		// leadership claims for other units. For now, units can only
		// claim leadership for themselves.
		if !m.authorizer.AuthUnitAgent() || !m.authorizer.AuthOwner(unitTag) {
			result.Error = common.ServerError(common.ErrPerm)
			continue
		}

		result.Error = release(svcTag, unitTag).Error
	}

	return params.ReleaseLeadershipBulkResults{results}, nil
}

// BlockUntilLeadershipReleased implements the LeadershipService interface.
func (m *leadershipService) BlockUntilLeadershipReleased(serviceTag names.ServiceTag) (params.ErrorResult, error) {
	if !m.authorizer.AuthUnitAgent() {
		return params.ErrorResult{Error: common.ServerError(common.ErrPerm)}, nil
	}

	if err := m.LeadershipManager.BlockUntilLeadershipReleased(serviceTag.Id()); err != nil {
		return params.ErrorResult{Error: common.ServerError(err)}, nil
	}
	return params.ErrorResult{}, nil
}

// callWithIds transforms a common Leadership Election function
// signature into one that is conducive to use in the API Server.
func callWithIds(fn func(string, string) error) func(st, ut names.Tag) params.ErrorResult {
	return func(st, ut names.Tag) (result params.ErrorResult) {
		if err := fn(st.Id(), ut.Id()); err != nil {
			result.Error = common.ServerError(err)
		}
		return result
	}
}

// parseServiceAndUnitTags takes in string representations of service
// and unit tags and parses them into structs.
//
// NOTE: we return permissions errors when parsing fails to obfuscate
// the parse-failure. This is for security purposes.
func parseServiceAndUnitTags(
	serviceTagString, unitTagString string,
) (serviceTag names.ServiceTag, unitTag names.UnitTag, _ *params.Error) {
	serviceTag, err := names.ParseServiceTag(serviceTagString)
	if err != nil {
		return serviceTag, unitTag, common.ServerError(common.ErrPerm)
	}

	unitTag, err = names.ParseUnitTag(unitTagString)
	if err != nil {
		return serviceTag, unitTag, common.ServerError(common.ErrPerm)
	}

	return serviceTag, unitTag, nil
}
