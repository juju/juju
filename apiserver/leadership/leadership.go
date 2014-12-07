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
func (m *leadershipService) ClaimLeadership(args params.ClaimLeadershipBulkParams) (results params.ClaimLeadershipBulkResults, _ error) {

	var dur time.Duration
	claim := callWithIds(func(sid, uid string) (err error) {
		dur, err = m.LeadershipManager.ClaimLeadership(sid, uid)
		return err
	})

	for _, p := range args.Params {

		result := params.ClaimLeadershipResults{}

		if !m.authorizer.AuthOwner(p.ServiceTag) || !m.authorizer.AuthOwner(p.UnitTag) {
			result.Error = common.ServerError(common.ErrPerm)
		} else if result.Error = claim(p.ServiceTag, p.UnitTag); result.Error == nil {
			result.ClaimDurationInSec = dur.Seconds()
			result.ServiceTag = p.ServiceTag
		}

		results.Results = append(results.Results, result)
	}
	return results, nil
}

// ReleaseLeadership implements the LeadershipService interface.
func (m *leadershipService) ReleaseLeadership(args params.ReleaseLeadershipBulkParams) (results params.ReleaseLeadershipBulkResults, _ error) {
	release := callWithIds(m.LeadershipManager.ReleaseLeadership)
	for _, p := range args.Params {
		if !m.authorizer.AuthOwner(p.ServiceTag) || !m.authorizer.AuthOwner(p.UnitTag) {
			results.Errors = append(results.Errors, common.ServerError(common.ErrPerm))
			continue
		}

		if err := release(p.ServiceTag, p.UnitTag); err != nil {
			results.Errors = append(results.Errors, err)
		}
	}

	return results, nil
}

// BlockUntilLeadershipReleased implements the LeadershipService interface.
func (m *leadershipService) BlockUntilLeadershipReleased(serviceTag names.ServiceTag) error {
	if !m.authorizer.AuthOwner(serviceTag) {
		return common.ErrPerm
	}

	return m.LeadershipManager.BlockUntilLeadershipReleased(serviceTag.Id())
}

func callWithIds(fn func(string, string) error) func(st, ut names.Tag) *params.Error {
	return func(st, ut names.Tag) *params.Error {
		return common.ServerError(fn(st.Id(), ut.Id()))
	}
}
