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
	FacadeName = "LeadershipService"
)

var (
	logger = loggo.GetLogger("juju.apiserver.leadership")
	// Ensure the LeadershipService conforms to the interface at compile-time.
	_ LeadershipService = (*leadershipService)(nil)
	// Begin injection-chain so we can instantiate leadership
	// services. Exposed as variables so we can change the
	// implementation for testing purposes.
	leaseMgr  = lease.Manager()
	leaderMgr = leadership.NewLeadershipManager(leaseMgr)
)

func init() {

	// Worker that starts the lease manager.
	common.RegisterStandardFacade(
		FacadeName,
		0,
		NewLeadershipServiceFn(leaderMgr),
	)
}

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

func NewLeadershipService(
	state *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
	leadershipMgr leadership.LeadershipManager,
) (LeadershipService, error) {
	if !authorizer.AuthClient() {
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

func (m *leadershipService) ClaimLeadership(args ClaimLeadershipBulkParams) (results ClaimLeadershipBulkResults, _ error) {

	var dur time.Duration
	claim := closeOver(func(st, ut string) (err error) {
		dur, err = m.LeadershipManager.ClaimLeadership(st, ut)
		return err
	})
	for _, p := range args.Params {
		result := ClaimLeadershipResults{}
		results.Results = append(results.Results, result)
		result.Error = claim(p.ServiceTag, p.UnitTag)
		if result.Error != nil {
			continue
		}

		result.ClaimDurationInSec = dur.Seconds()
		result.ServiceTag = p.ServiceTag
	}
	return results, nil
}

func (m *leadershipService) ReleaseLeadership(args ReleaseLeadershipBulkParams) (results ReleaseLeadershipBulkResults, _ error) {
	release := closeOver(m.LeadershipManager.ReleaseLeadership)
	for _, p := range args.Params {
		if err := release(p.ServiceTag, p.UnitTag); err != nil {
			results.Errors = append(results.Errors, err)
		}
	}

	return results, nil
}

func (m *leadershipService) BlockUntilLeadershipReleased(serviceTag names.ServiceTag) error {
	return m.LeadershipManager.BlockUntilLeadershipReleased(serviceTag.Id())
}

func closeOver(fn func(string, string) error) func(st, ut names.Tag) *params.Error {
	return func(st, ut names.Tag) *params.Error {
		return common.ServerError(fn(st.Id(), ut.Id()))
	}
}
