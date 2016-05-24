// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/worker/catacomb"
)

type instanceGetter interface {
	Instances(ids []instance.Id) ([]instance.Instance, error)
}

type aggregatorConfig struct {
	Clock   clock.Clock
	Delay   time.Duration
	Environ instanceGetter
}

func (c aggregatorConfig) validate() error {
	if c.Clock == nil {
		return errors.NotValidf("nil clock.Clock")
	}
	if c.Delay == 0 {
		return errors.NotValidf("zero Delay")
	}
	if c.Environ == nil {
		return errors.NotValidf("nil Environ")
	}
	return nil

}

type aggregator struct {
	config   aggregatorConfig
	catacomb catacomb.Catacomb
	reqc     chan instanceInfoReq
}

func newAggregator(config aggregatorConfig) (*aggregator, error) {
	if err := config.validate(); err != nil {
		return nil, errors.Trace(err)
	}
	a := &aggregator{
		config: config,
		reqc:   make(chan instanceInfoReq),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &a.catacomb,
		Work: a.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return a, nil
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
	reqc := a.reqc
	for {
		select {
		case <-a.catacomb.Dying():
			return instanceInfo{}, errors.New("instanceInfo call aborted")
		case reqc <- instanceInfoReq{id, reply}:
			reqc = nil
		case r := <-reply:
			return r.info, r.err
		}
	}
}

func (a *aggregator) loop() error {
	var (
		next time.Time
		reqs []instanceInfoReq
	)

	for {
		var ready <-chan time.Time
		if !next.IsZero() {
			when := next.Add(a.config.Delay)
			ready = clock.Alarm(a.config.Clock, when)
		}
		select {
		case <-a.catacomb.Dying():
			return a.catacomb.ErrDying()

		case req := <-a.reqc:
			reqs = append(reqs, req)

			if next.IsZero() {
				next = a.config.Clock.Now()
			}

		case <-ready:
			if err := a.doRequests(reqs); err != nil {
				return errors.Trace(err)
			}
			next = time.Time{}
			reqs = nil
		}
	}
}

func (a *aggregator) doRequests(reqs []instanceInfoReq) error {
	ids := make([]instance.Id, len(reqs))
	for i, req := range reqs {
		ids[i] = req.instId
	}
	insts, err := a.config.Environ.Instances(ids)
	for i, req := range reqs {
		var reply instanceInfoReply
		if err != nil && err != environs.ErrPartialInstances {
			reply.err = err
		} else {
			reply.info, reply.err = a.instInfo(req.instId, insts[i])
		}
		select {
		// Per review http://reviews.vapour.ws/r/4885/ it's dumb to block
		// the main goroutine on these responses.
		case <-a.catacomb.Dying():
			return a.catacomb.ErrDying()
		case req.reply <- reply:
		}
	}
	return nil
}

// instInfo returns the instance info for the given id
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

func (a *aggregator) Kill() {
	a.catacomb.Kill(nil)
}

func (a *aggregator) Wait() error {
	return a.catacomb.Wait()
}
