package state

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/leadership"
	stateleadership "github.com/juju/juju/state/leadership"
	"github.com/juju/juju/state/lease"
)

const (
	leadershipWorkerName = "leadership"

	settingsKey = "s#%s#leader"
)

func addLeadershipSettingsOp(serviceId string) txn.Op {
	return txn.Op{
		C:      settingsC,
		Id:     leadershipSettingsDocId(serviceId),
		Insert: bson.D{},
		Assert: txn.DocMissing,
	}
}

func removeLeadershipSettingsOp(serviceId string) txn.Op {
	return txn.Op{
		C:      settingsC,
		Id:     leadershipSettingsDocId(serviceId),
		Remove: true,
	}
}

func leadershipSettingsDocId(serviceId string) string {
	return fmt.Sprintf(settingsKey, serviceId)
}

// LeadershipClaimer returns a leadership.Claimer for units and services in the
// state's environment.
func (st *State) LeadershipClaimer() leadership.Claimer {
	return lazyLeadershipManager{st.leadershipWorker}
}

// LeadershipChecker returns a leadership.Checker for units and services in the
// state's environment.
func (st *State) LeadershipChecker() leadership.Checker {
	return lazyLeadershipManager{st.leadershipWorker}
}

// HackLeadership stops the state's internal leadership manager to prevent it
// from interfering with apiserver shutdown.
func (st *State) HackLeadership() {
	// TODO(fwereade): 2015-08-07 lp:1482634
	// obviously, this should not exist: it's a quick hack to address lp:1481368 in
	// 1.24.4, and should be quickly replaced with something that isn't so heinous.
	//
	// But.
	//
	// I *believe* that what it'll take to fix this is to extract the mongo-session-
	// opening from state.Open, so we can create a mongosessioner Manifold on which
	// state, leadership, watching, tools storage, etc etc etc can all independently
	// depend. (Each dependency would/should have a separate session so they can
	// close them all on their own schedule, without panics -- but the failure of
	// the shared component should successfully goose them all into shutting down,
	// in parallel, of their own accord.)
	st.leadershipWorker.Kill()
}

// buildTxnWithLeadership returns a transaction source that combines the supplied source
// with checks and asserts on the supplied token.
func buildTxnWithLeadership(buildTxn jujutxn.TransactionSource, token leadership.Token) jujutxn.TransactionSource {
	return func(attempt int) ([]txn.Op, error) {
		var prereqs []txn.Op
		if err := token.Check(&prereqs); err != nil {
			return nil, errors.Annotatef(err, "prerequisites failed")
		}
		ops, err := buildTxn(attempt)
		if err == jujutxn.ErrNoOperations {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		return append(prereqs, ops...), nil
	}
}

type lazyLeadershipManager struct {
	w *leadershipWorker
}

func (l lazyLeadershipManager) ClaimLeadership(serviceId, unitId string, duration time.Duration) error {
	return l.w.manager().ClaimLeadership(serviceId, unitId, duration)
}

func (l lazyLeadershipManager) BlockUntilLeadershipReleased(serviceId string) error {
	return l.w.manager().BlockUntilLeadershipReleased(serviceId)
}

func (l lazyLeadershipManager) LeadershipCheck(serviceName, unitName string) leadership.Token {
	return l.w.manager().LeadershipCheck(serviceName, unitName)
}

type leadershipWorker struct {
	*worker.Runner
}

func newLeadershipWorker(st *State) *leadershipWorker {
	w := &leadershipWorker{
		Runner: worker.NewRunner(worker.RunnerParams{
			IsFatal: func(error) bool { return false },
		}),
	}
	w.startWorker(st)
	return w
}

func (w *leadershipWorker) startWorker(st *State) {
	w.StartWorker(leadershipWorkerName, func() (worker.Worker, error) {
		var clientId string
		if identity := st.mongoInfo.Tag; identity != nil {
			// TODO(fwereade): it feels a bit wrong to take this from MongoInfo -- I
			// think it's just coincidental that the mongodb user happens to map to
			// the machine that's executing the code -- but there doesn't seem to be
			// an accessible alternative.
			clientId = identity.String()
		} else {
			// If we're running state anonymously, we can still use the lease
			// manager; but we need to make sure we use a unique client ID, and
			// will thus not be very performant.
			logger.Infof("running state anonymously; using unique client id")
			uuid, err := utils.NewUUID()
			if err != nil {
				return nil, errors.Trace(err)
			}
			clientId = fmt.Sprintf("anon-%s", uuid.String())
		}

		logger.Infof("creating lease client as %s", clientId)
		clock := GetClock()
		datastore := &environMongo{st}
		leaseClient, err := lease.NewClient(lease.ClientConfig{
			Id:         clientId,
			Namespace:  serviceLeadershipNamespace,
			Collection: leasesC,
			Mongo:      datastore,
			Clock:      clock,
		})
		if err != nil {
			return nil, errors.Annotatef(err, "cannot create lease client")
		}
		logger.Infof("starting leadership manager")

		leadershipManager, err := stateleadership.NewManager(stateleadership.ManagerConfig{
			Client:   leaseClient,
			Clock:    clock,
			MaxSleep: time.Minute,
		})
		if err != nil {
			return nil, errors.Annotatef(err, "cannot create leadership manager")
		}
		return leadershipManager, nil
	})
}

func (lw *leadershipWorker) manager() stateleadership.ManagerWorker {
	w, err := lw.Worker(leadershipWorkerName, nil)
	if err != nil {
		return stateleadership.NewDeadManager(errors.Trace(err))
	}
	return w.(stateleadership.ManagerWorker)
}
