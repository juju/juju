// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package presence

import (
	"math/rand"
	"time"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/tomb.v1"
)

const maxBatch = 1000

type slot struct {
	Slot  int64
	Alive map[string]uint64
}

type singlePing struct {
	Slot      int64
	ModelUUID string
	FieldKey  string
	FieldBit  uint64
}

func NewPingBatcher(pings *mgo.Collection, flushInterval time.Duration) *PingBatcher {
	return &PingBatcher{
		pings:         pings,
		pending:       make(map[string]slot),
		flushInterval: flushInterval,
		pingChan:      make(chan singlePing),
		flushChan:     make(chan chan struct{}),
	}
}

// PingBatcher aggregates several pingers to update the database on a fixed schedule.
type PingBatcher struct {
	pings         *mgo.Collection
	modelUUID     string
	pending       map[string]slot
	pingCount     uint64
	flushInterval time.Duration
	tomb          tomb.Tomb
	pingChan      chan singlePing
	flushChan     chan chan struct{}
}

func (p *PingBatcher) Start() error {
	rand.Seed(time.Now().UnixNano())
	go func() {
		err := p.loop()
		cause := errors.Cause(err)
		// tomb expects ErrDying or ErrStillAlive as
		// exact values, so we need to log and unwrap
		// the error first.
		if err != nil && cause != tomb.ErrDying {
			logger.Infof("ping batching loop failed: %v", err)
		}
		p.tomb.Kill(cause)
		p.tomb.Done()
	}()
	return nil
}

// Kill is part of the worker.Worker interface.
func (pb *PingBatcher) Kill() {
	pb.tomb.Kill(nil)
}

// Wait returns when the Pinger has stopped, and returns the first error
// it encountered.
func (pb *PingBatcher) Wait() error {
	return pb.tomb.Wait()
}

// Stop this PingBatcher
func (pb *PingBatcher) Stop() error {
	pb.tomb.Kill(nil)
	err := pb.tomb.Wait()
	return errors.Trace(err)
}

func (pb *PingBatcher) nextSleep() time.Duration {
	sleepMin := float64(pb.flushInterval) * 0.8
	sleepRange := float64(pb.flushInterval) * 0.4
	offset := rand.Int63n(int64(sleepRange))
	return time.Duration(int64(sleepMin) + offset)
}

func (pb *PingBatcher) loop() error {
	// flushDone and flushRequest exist to make a flushRequest synchronous
	// with all other pings that have been requested. The logic is:
	for {
		select {
		case <-pb.tomb.Dying():
			return errors.Trace(tomb.ErrDying)
		case <-time.After(pb.nextSleep()):
			if err := pb.flush(); err != nil {
				return errors.Trace(err)
			}
		case singlePing := <-pb.pingChan:
			pb.handlePing(singlePing)
		case flushReq := <-pb.flushChan:
			// Flush is requested synchronously.
			// This way we know all pings have been handled. We will
			// close the channel that was passed to us once we have
			// finished flushing.
			if err := pb.flush(); err != nil {
				return err
			}
			close(flushReq)
		}
	}
}

func (pb *PingBatcher) Ping(modelUUID string, slot int64, fieldKey string, fieldBit uint64) error {
	ping := singlePing{
		Slot:      slot,
		ModelUUID: modelUUID,
		FieldKey:  fieldKey,
		FieldBit:  fieldBit,
	}
	select {
	case pb.pingChan <- ping:
		return nil
	case <-pb.tomb.Dying():
		return errors.Trace(pb.tomb.Err())
	}
}

func (pb *PingBatcher) handlePing(ping singlePing) {
	docId := docIDInt64(ping.ModelUUID, ping.Slot)
	cur, slotExists := pb.pending[docId]
	if !slotExists {
		cur.Alive = make(map[string]uint64)
		cur.Slot = ping.Slot
		pb.pending[docId] = cur
	}
	alive := cur.Alive
	alive[ping.FieldKey] |= ping.FieldBit
	pb.pingCount++
}

func (pb *PingBatcher) flush() error {
	// We treat all of these as 'consumed'. Even if the query fails, it is
	// not safe to ever $inc the same fields a second time, so we just move on.
	next := pb.pending
	pingCount := pb.pingCount
	pb.pending = make(map[string]slot)
	pb.pingCount = 0
	session := pb.pings.Database.Session.Copy()
	defer session.Close()
	pings := pb.pings.With(session)
	bulk := pings.Bulk()
	docCount := 0
	fieldCount := 0
	t := time.Now()
	bulkCount := 0
	for docId, slot := range next {
		docCount++
		var incFields bson.D
		for fieldKey, value := range slot.Alive {
			incFields = append(incFields, bson.DocElem{Name: "alive." + fieldKey, Value: value})
			fieldCount++
		}
		bulk.Upsert(
			bson.D{{"_id", docId}},
			bson.D{
				{"$set", bson.D{{"slot", slot.Slot}}},
				{"$inc", incFields},
			},
		)
		bulkCount++
		if bulkCount >= maxBatch {
			if _, err := bulk.Run(); err != nil {
				return errors.Trace(err)
			}
			bulkCount = 0
			bulk = pings.Bulk()
		}
	}
	if bulkCount > 0 {
		if _, err := bulk.Run(); err != nil {
			return errors.Trace(err)
		}
	}
	// usually we should only be processing 1 slot
	logger.Debugf("recorded %d pings for %d ping slot(s) and %d fields in %v", pingCount, docCount, fieldCount, time.Since(t))
	return nil
}
