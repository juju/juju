package instancepoller

import (
        "fmt"
	"time"

        "github.com/juju/ratelimit"
        "launchpad.net/tomb"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
)

type aggregator struct {
	environ environs.Environ
	reqc    chan instanceInfoReq
        tomb    tomb.Tomb
}

type instanceGetter interface {

}

func newAggregator(env environs.Environ) *aggregator {
	a := &aggregator{
		environ: env,
		reqc:    make(chan instanceInfoReq),
	}
        go func() {
                defer a.tomb.Done()
                a.tomb.Kill(a.loop())
        }()
	return a
}

type instanceInfoReq struct {
	instId instance.Id
	reply  chan<- instanceInfoReply
}

type instanceInfoReply struct {
	info instanceInfo
	err  error
}

func (a *aggregator) instanceInfo(id instance.Id) (instanceInfo, error) {
	reply := make(chan instanceInfoReply)
	a.reqc <- instanceInfoReq{
		instId: id,
		reply:  reply,
	}
	r := <-reply
	return r.info, r.err
}

var GatherTime = 3 * time.Second
var Capacity int64 = 1

func (a *aggregator) loop() error {
	timer := time.NewTimer(0)
	timer.Stop()
	var reqs []instanceInfoReq
        bucket := ratelimit.New(GatherTime, Capacity)
	for {
		select {
                case <-a.tomb.Dying():
                    return tomb.ErrDying
		case req, ok := <-a.reqc:
			if !ok {
				return fmt.Errorf("request channel closed")
			}
			if len(reqs) == 0 {
                                waitTime := bucket.Take(1)
				timer.Reset(waitTime)
			}
			reqs = append(reqs, req)
		case <-timer.C:
			ids := make([]instance.Id, len(reqs))
			for i, req := range reqs {
				ids[i] = req.instId
			}
			insts, err := a.environ.Instances(ids)
			for i, req := range reqs {
				var reply instanceInfoReply
				if err != nil && err != environs.ErrPartialInstances {
					reply.err = err
				} else {
					reply.info, reply.err = a.instInfo(req.instId, insts[i])
				}
				req.reply <- reply
			}
			reqs = nil
		}
	}
}

func (a *aggregator) Kill() {
        a.tomb.Kill(nil)
}

func (a *aggregator) Wait() error {
        return a.tomb.Wait()
}

// instanceInfo returns the instance info for the given id
// and instance. If inst is nil, it returns a not-found error.
func (*aggregator) instInfo(id instance.Id, inst instance.Instance) (instanceInfo, error) {
	if inst == nil {
		return instanceInfo{}, errors.NotFoundf("instance %v", id)
	}
	addr, err := inst.Addresses()
	if err != nil {
		return instanceInfo{}, err
	}
	return instanceInfo{
		addr,
		inst.Status(),
	}, nil
}
