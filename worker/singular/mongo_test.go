package singular_test

import (
	"fmt"
	"time"

	"github.com/juju/loggo"
	"labix.org/v2/mgo"

	gc "launchpad.net/gocheck"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/replicaset"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/worker/singular"
)

var logger = loggo.GetLogger("juju.singular-test")

type mongoSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&mongoSuite{})

// start replica set with three mongods
// start singular worker on each one.
// change worker priorities so the master changes.
// check that
// a) there is never more than one running at a time
// b) the running worker changes when the master changes.

func (*mongoSuite) TestMongoMastership(c *gc.C) {
	insts, err := startReplicaSet(3)
	c.Assert(err, gc.IsNil)

	agentFinished := make(chan error)

	notifyCh := make(chan string)
	agents := make([]*agent, len(insts))
	stats 
	for i, inst := range insts {
		a := &agent{
			notify: newNotifier(i, notifyCh),
			Runner: newRunner(),
			hostPort: inst.Addr(),
		}
		go func() {
			err := a.run()
			agentFinished <- fmt.Errorf("agent %s finished: %v", agent.hostPort, err)
		}()
	}

	wait on notify channel
	expect "0 start"
	expect "0 operation"*

	adjust priorities so that 1 will be elected
	expect 
		"1 start" then "0 stop"
	or
		"0 stop" then "1 start"

	kill
	expect all agents finished
}

type stats struct {
	mu sync.Mutex
	nrunning int
	maxRunning int
}

type notifier struct {
	index int
	ch chan string
}

func (n *notifier) workerStarted() {
	ch <- fmt.Sprintf("%d start", n.index)
}

func (n *notifier) workerStopped() {
	ch <- fmt.Sprintf("%d stop", n.index)
}

type agent struct {
	notify *notifier
	worker.Runner
	hostPort string
}

type notifier struct {
}

func newAgent(hostPort, 

func (a *agent) run() error {
	runner.StartWorker("mongo-"+a.hostPort, a.mongoWorker)
}

func (a *agent) mongoWorker() (worker.Worker, error) {
	session, err := a.inst.Dial()
	if err != nil {
		return nil, err
	}
	mc := &mongoConn{
		localHostPort: a.hostPort,
		session: session,
	}
	runner := worker.NewRunner(
		connectionIsFatal(mc),
		func(err0, err1 error) bool { return true },
	}
	runner.StartWorker("worker-"+a.hostPort, func() (worker.Worker, error) {
		return worker.NewSimpleWorker(func(stop <-chan struct{}) error {
			return a.worker(session, stop)
		})
	})
	return runner
}

func (a *agent) worker(session *mgo.Session, stop <-chan struct{}) error {
	a.notify.workerStarted()
	defer a.notify.workerStopped()
	coll := session.DB("foo").C("bar")
	for {
		select {
		case <-stop:
			return nil
		case <-time.After(250 * time.Millisecond):
		}
		if err := coll.Insert(struct{}{}); err != nil {
			return fmt.Errorf("insert error: %v", err)
		}
	}
}
		

type mongoConn struct {
	localHostPort string
	session       *mgo.Session
}

func (c *mongoConn) Ping() error {
	return c.session.Ping()
}

func (c *mongoConn) IsMaster() (bool, error) {
	hostPort, err := replicaset.MasterHostPort(c.session)
	if err != nil {
		return false, err
	}
	return hostPort == c.localHostPort, nil
}

const replicaSetName = "juju"

// startReplicaSet starts up a replica set with n mongo instances.
func startReplicaSet(n int) (_ []testing.MgoInstance, err error) {
	insts := make([]*testing.MgoInstance, 0, n)
	root, err := newMongoInstance()
	if err != nil {
		return nil, err
	}
	insts = append(insts, root)
	defer func() {
		if err == nil {
			return
		}
		for _, inst := range insts {
			inst.Destroy()
		}
	}()

	dialInfo := root.DialInfo()
	dialInfo.Direct = true
	dialInfo.Timeout = 60 * time.Second

	session, err := root.DialDirect()
	if err != nil {
		return nil, fmt.Errorf("cannot dial root instance: %v", err)
	}
	defer session.Close()

	logger.Infof("dialled root instance")

	if err := replicaset.Initiate(session, root.Addr(), replicaSetName); err != nil {
		return nil, fmt.Errorf("cannot initiate replica set: %v", err)
	}
	var members []replicaset.Member
	for i := 0; i < n; i++ {
		inst, err := newMongoInstance()
		if err != nil {
			return nil, err
		}
		insts = append(insts, inst)
		members = append(members, replicaset.Member{
			Address:  inst.Addr(),
			Priority: newFloat64(0.5),
		})
	}
	attempt := utils.AttemptStrategy{
		Total: 60 * time.Second,
		Delay: 1 * time.Second,
	}
	for a := attempt.Start(); a.Next(); {
		err := replicaset.Add(session, members...)
		if err == nil {
			break
		}
		logger.Errorf("cannot add members: %v", err)
		if !a.HasNext() {
			return nil, fmt.Errorf("timed out trying to add members")
		}
		logger.Errorf("retrying")
	}
	return insts, err
}

func newMongoInstance() (*testing.MgoInstance, error) {
	inst := &testing.MgoInstance{Params: []string{"--replSet", replicaSetName}}
	if err := inst.Start(true); err != nil {
		return nil, fmt.Errorf("cannot start mongo server: %s", err.Error())
	}
	return inst, nil
}

func newFloat64(f float64) *float64 {
	return &f
}

// connectionIsFatal returns a function suitable for passing
// as the isFatal argument to worker.NewRunner,
// that diagnoses an error as fatal if the connection
// has failed or if the error is otherwise fatal.
// Copied from jujud.
func connectionIsFatal(conn singular.Conn) func(err error) bool {
	return func(err error) bool {
		if err := conn.Ping(); err != nil {
			logger.Infof("error pinging %T: %v", conn, err)
			return true
		}
		logger.Infof("error %q is not fatal", err)
		return false
	}
}
