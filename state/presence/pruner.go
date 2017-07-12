// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package presence

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// beingRemover tracks what records we've decided we wanted to remove.
type beingRemover struct {
	queue []string
}

// Pruner tracks the state of removing unworthy beings from the
// presence.beings and presence.pings collections. Being sequences are unworthy
// once their sequence has been superseded, and pings older than 2 slots are
// no longer referenced.
type Pruner struct {
	modelUUID    string
	beingsC      *mgo.Collection
	pingsC       *mgo.Collection
	toRemove     []string
	maxQueue     int
	removedCount uint64
	delta        time.Duration
}

// iterKeys is returns an iterator of Keys from this modelUUID and which Sequences
// are used to represent them.
// It only returns sequences that have more than one sequence associated with the same
// being (as beings with a single sequence will never be pruned).
func (p *Pruner) iterKeys() *mgo.Iter {
	thisModelRegex := bson.M{"_id": bson.M{"$regex": bson.RegEx{"^" + p.modelUUID, ""}}}
	pipe := p.beingsC.Pipe([]bson.M{
		// Grab all sequences for this model
		{"$match": thisModelRegex},
		// We don't need the _id
		{"$project": bson.M{"_id": 0, "seq": 1, "key": 1}},
		// Group all the sequences by their key.
		{"$group": bson.M{
			"_id":  "$key",
			"seqs": bson.M{"$push": "$seq"},
		}},
		// Filter out any keys that have only a single sequence
		// representing them
		// Note: indexing is from 0, you can set this to 2 if you wanted
		// to only bother pruning sequences that have >2 entries.
		// This mostly helps the 'nothing to do' case, dropping the time
		// to realize there are no sequences to be removed from 36ms,
		// down to 15ms with 3500 keys.
		{"$match": bson.M{"seqs.1": bson.M{"$exists": 1}}},
	})
	pipe.Batch(1600)
	return pipe.Iter()
}

// queueRemoval includes this sequence as one that has been superseded
func (p *Pruner) queueRemoval(seq int64) {
	p.toRemove = append(p.toRemove, docIDInt64(p.modelUUID, seq))
}

// flushRemovals makes sure that we've applied all desired removals
func (p *Pruner) flushRemovals() error {
	if len(p.toRemove) == 0 {
		return nil
	}
	matched, err := p.beingsC.RemoveAll(bson.M{"_id": bson.M{"$in": p.toRemove}})
	if err != nil {
		return err
	}
	p.toRemove = p.toRemove[:0]
	if matched.Removed > 0 {
		p.removedCount += uint64(matched.Removed)
	}
	return err
}

func (p *Pruner) removeOldPings() error {
	// now and now-period are both considered active slots, so we don't
	// touch those. We also leave 2 more slots around
	startTime := time.Now()
	logger.Tracef("pruning %q for %q", p.pingsC.Name, p.modelUUID)
	s := timeSlot(time.Now(), p.delta)
	oldSlot := s - 3*period
	res, err := p.pingsC.RemoveAll(bson.D{{"_id", bson.RegEx{"^" + p.modelUUID, ""}},
		{"slot", bson.M{"$lt": oldSlot}}})
	if err != nil && err != mgo.ErrNotFound {
		logger.Errorf("error removing old entries from %q: %v", p.pingsC.Name, err)
		return err
	}
	logger.Debugf("pruned %q for %q of %d old pings in %v",
		p.pingsC.Name, p.modelUUID, res.Removed, time.Since(startTime))
	return nil
}

func (p *Pruner) removeUnusedBeings() error {
	var keyInfo collapsedBeingsInfo
	seqSet, err := p.findActiveSeqs()
	if err != nil {
		return err
	}
	logger.Tracef("pruning %q for %q starting", p.beingsC.Name, p.modelUUID)
	startTime := time.Now()
	keyCount := 0
	seqCount := 0
	iter := p.iterKeys()
	for iter.Next(&keyInfo) {
		keyCount += 1
		// Find the max
		maxSeq := int64(-1)
		for _, seq := range keyInfo.Seqs {
			if seq > maxSeq {
				maxSeq = seq
			}
		}
		// Queue everything < max to be deleted
		for _, seq := range keyInfo.Seqs {
			seqCount++
			_, isActive := seqSet[seq]
			if seq >= maxSeq || isActive {
				// It shouldn't be possible to be > at this point
				continue
			}
			p.queueRemoval(seq)
			if len(p.toRemove) > p.maxQueue {
				if err := p.flushRemovals(); err != nil {
					return err
				}
			}
		}
	}
	if err := p.flushRemovals(); err != nil {
		return err
	}
	if err := iter.Close(); err != nil {
		return err
	}
	logger.Debugf("pruned %q for %q of %d sequence keys (evaluated %d) from %d keys in %v",
		p.beingsC.Name, p.modelUUID, p.removedCount, seqCount, keyCount, time.Since(startTime))
	return nil
}

func (p *Pruner) findActiveSeqs() (map[int64]struct{}, error) {
	// After pruning old pings, we now track all sequences which are still alive.
	var infos []pingInfo
	err := p.pingsC.Find(nil).All(&infos)
	if err != nil {
		return nil, err
	}
	maps := make([]map[string]int64, 0, len(infos)*2)
	for _, ping := range infos {
		maps = append(maps, ping.Alive)
		maps = append(maps, ping.Dead)
	}
	seqs, err := decompressPings(maps)
	if err != nil {
		return nil, err
	}
	seqSet := make(map[int64]struct{})
	for _, seq := range seqs {
		seqSet[seq] = struct{}{}
	}
	return seqSet, nil
}

// Prune removes beings from the beings collection that have been superseded by
// another entry with a higher sequence.
// It also removes pings that are outside of the 'active' range
// (the last few slots)
func (p *Pruner) Prune() error {
	err := p.removeOldPings()
	if err != nil {
		return errors.Trace(err)
	}
	err = p.removeUnusedBeings()
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// NewPruner returns an object that is ready to prune the Beings collection
// of old beings sequence entries that we no longer need.
func NewPruner(modelUUID string, beings *mgo.Collection, pings *mgo.Collection, delta time.Duration) *Pruner {
	return &Pruner{
		modelUUID: modelUUID,
		beingsC:   beings,
		maxQueue:  1000,
		pingsC:    pings,
		delta:     delta,
	}
}
