package instancepoller

import (
	"time"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
)

type aggregator struct {
	environ environs.Environ
	reqc    chan instanceInfoReq
}

func newAggregator(env environs.Environ) *aggregator {
	a := &aggregator{
		environ: env,
		reqc:    make(chan instanceInfoReq),
	}
	go a.loop()
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

func (a *aggregator) loop() {
	timer := time.NewTimer(0)
	timer.Stop()
	var reqs []instanceInfoReq
	for {
		select {
		case req, ok := <-a.reqc:
			if !ok {
				return
			}
			if len(reqs) == 0 {
				timer.Reset(GatherTime)
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
