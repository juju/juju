package singular

import (
	"fmt"
	"time"

	"launchpad.net/tomb"
	"github.com/juju/loggo"

	"launchpad.net/juju-core/worker"
)

var logger = loggo.GetLogger("juju.worker")

var pingInterval = 10 * time.Second

type runner struct {
	tomb     tomb.Tomb
	isMaster bool
	worker.Runner
}

// Conn represents a connection to some resource.
type Conn interface {
	// IsMaster reports whether this connection is currently held by
	// the (singular) master of the resource.
	IsMaster() (bool, error)

	// Ping probes the resource and returns an error if the the
	// connection has failed. If the master changes, this method
	// must return an error.
	Ping() error
}

// New returns a Runner that can be used to start workers that will only
// run a single instance. The conn value is used to determine whether to
// run the workers or not.
//
// Any workers started will be started on the underlying runner, but
// will do nothing if the connection is not currently held by the master
// of the resource.
func New(underlying worker.Runner, conn Conn) (worker.Runner, error) {
	isMaster, err := conn.IsMaster()
	if err != nil {
		return nil, fmt.Errorf("cannot get master status: %v", err)
	}
	r := &runner{
		isMaster: isMaster,
		Runner:   underlying,
	}
	go func() {
		defer r.tomb.Done()
		if !isMaster {
			// We're not master, so start a pinger so that we know when
			// the connection master changes.
			r.pinger(conn)
		} else {
			// We *are* master; the started workers should
			// encounter an error as they do what they're supposed
			// to do - no need for an extra pinger to tell us.
			// We just wait to be killed.
			<-r.tomb.Dying()
		}
	}()
	return r, nil
}

func (r *runner) pinger(conn Conn) {
	timer := time.NewTimer(0)
	for {
		err := conn.Ping()
		if err != nil {
			logger.Infof("ping error: %v", err)
			return
		}
		timer.Reset(pingInterval)
		select {
		case <-timer.C:
		case <-r.tomb.Dying():
			return
		}
	}
}

func (r *runner) StartWorker(id string, startFunc func() (worker.Worker, error)) error {
	if r.isMaster {
		return r.Runner.StartWorker(id, startFunc)
	}
	return r.Runner.StartWorker(id, func() (worker.Worker, error) {
		return worker.NewSimpleWorker(func(stop <-chan struct{}) error {
			logger.Infof("not starting %q because we are not master", id)
			select {
			case <-stop:
			case <-r.tomb.Dying():
			}
			return nil
		}), nil
	})
}
