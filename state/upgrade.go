// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
This file defines infrastructure for synchronising state server tools
upgrades. Synchronisation is handled via a mongo DB document in the
"upgradeInfo" collection.

The functionality here is intended to be used as follows:

1. When state servers come up running the new tools version, they call
EnsureUpgradeInfo before running upgrade steps.

2a. Any secondary state server watches the UpgradeInfo document and
waits for the status to change to UpgradeFinishing.

2b. The master state server watches the UpgradeInfo document and waits
for AllProvisionedStateServersReady to return true. This indicates
that all provisioned state servers have called EnsureUpgradeInfo and
are ready to upgrade.

3. The master state server calls SetStatus with UpgradeRunning and
runs its upgrade steps.

4. The master state server calls SetStatus with UpgradeFinishing and
then calls SetStateServerDone with it's own machine id.

5. Secondary state servers, seeing that the status has changed to
UpgradeFinishing, run their upgrade steps and then call
SetStateServerDone when complete.

6. Once the final state server calls SetStateServerDone, the status is
changed to UpgradeComplete and the upgradeInfo document is archived.
*/

package state

import (
	"time"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils/set"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/version"
)

// UpgradeStatus describes the states an upgrade operation may be in.
type UpgradeStatus string

const (
	// UpgradePending indicates that an upgrade is queued but not yet started.
	UpgradePending UpgradeStatus = "pending"

	// UpgradeRunning indicates that the master state server has started
	// running upgrade logic, and other state servers are waiting for it.
	UpgradeRunning UpgradeStatus = "running"

	// UpgradeFinishing indicates that the master state server has finished
	// running upgrade logic, and other state servers are catching up.
	UpgradeFinishing UpgradeStatus = "finishing"

	// UpgradeComplete indicates that all state servers have finished running
	// upgrade logic.
	UpgradeComplete UpgradeStatus = "complete"

	// UpgradeAborted indicates that the upgrade wasn't completed due
	// to some problem.
	UpgradeAborted UpgradeStatus = "aborted"

	// currentUpgradeId is the mongo _id of the current upgrade info document.
	currentUpgradeId = "current"
)

type upgradeInfoDoc struct {
	Id                string         `bson:"_id"`
	PreviousVersion   version.Number `bson:"previousVersion"`
	TargetVersion     version.Number `bson:"targetVersion"`
	Status            UpgradeStatus  `bson:"status"`
	Started           time.Time      `bson:"started"`
	StateServersReady []string       `bson:"stateServersReady"`
	StateServersDone  []string       `bson:"stateServersDone"`
}

// UpgradeInfo is used to synchronise state server upgrades.
type UpgradeInfo struct {
	st  *State
	doc upgradeInfoDoc
}

// PreviousVersion returns the version being upgraded from.
func (info *UpgradeInfo) PreviousVersion() version.Number {
	return info.doc.PreviousVersion
}

// TargetVersion returns the version being upgraded to.
func (info *UpgradeInfo) TargetVersion() version.Number {
	return info.doc.TargetVersion
}

// Status returns the status of the upgrade.
func (info *UpgradeInfo) Status() UpgradeStatus {
	return info.doc.Status
}

// Started returns the time at which the upgrade was started.
func (info *UpgradeInfo) Started() time.Time {
	return info.doc.Started
}

// StateServersReady returns the machine ids for state servers that
// have signalled that they are ready for upgrade.
func (info *UpgradeInfo) StateServersReady() []string {
	result := make([]string, len(info.doc.StateServersReady))
	copy(result, info.doc.StateServersReady)
	return result
}

// StateServersDone returns the machine ids for state servers that
// have completed their upgrades.
func (info *UpgradeInfo) StateServersDone() []string {
	result := make([]string, len(info.doc.StateServersDone))
	copy(result, info.doc.StateServersDone)
	return result
}

// Refresh updates the contents of the UpgradeInfo from underlying state.
func (info *UpgradeInfo) Refresh() error {
	doc, err := currentUpgradeInfoDoc(info.st)
	if err != nil {
		return errors.Trace(err)
	}
	info.doc = *doc
	return nil
}

// Watcher returns a watcher for the state underlying the current
// UpgradeInfo instance. This is provided purely for convenience.
func (info *UpgradeInfo) Watch() NotifyWatcher {
	return info.st.WatchUpgradeInfo()
}

// AllProvisionedStateServersReady returns true if and only if all state servers
// that have been started by the provisioner have called EnsureUpgradeInfo with
// matching versions.
//
// When this returns true the master state state server can begin it's
// own upgrade.
func (info *UpgradeInfo) AllProvisionedStateServersReady() (bool, error) {
	provisioned, err := info.getProvisionedStateServers()
	if err != nil {
		return false, errors.Trace(err)
	}
	ready := set.NewStrings(info.doc.StateServersReady...)
	missing := set.NewStrings(provisioned...).Difference(ready)
	return missing.IsEmpty(), nil
}

func (info *UpgradeInfo) getProvisionedStateServers() ([]string, error) {
	var provisioned []string

	stateServerInfo, err := info.st.StateServerInfo()
	if err != nil {
		return provisioned, errors.Annotate(err, "cannot read state servers")
	}

	upgradeDone, err := info.isEnvUUIDUpgradeDone()
	if err != nil {
		return provisioned, errors.Trace(err)
	}

	// Extract current and provisioned state servers.
	instanceData, closer := info.st.getRawCollection(instanceDataC)
	defer closer()

	// If instanceData has the env UUID upgrade query using the
	// machineid field, otherwise check using _id.
	var sel bson.D
	var field string
	if upgradeDone {
		sel = bson.D{{"env-uuid", info.st.EnvironUUID()}}
		field = "machineid"
	} else {
		field = "_id"
	}
	sel = append(sel, bson.DocElem{field, bson.D{{"$in", stateServerInfo.MachineIds}}})
	iter := instanceData.Find(sel).Select(bson.D{{field, true}}).Iter()

	var doc bson.M
	for iter.Next(&doc) {
		provisioned = append(provisioned, doc[field].(string))
	}
	if err := iter.Close(); err != nil {
		return provisioned, errors.Annotate(err, "cannot read provisioned machines")
	}
	return provisioned, nil
}

func (info *UpgradeInfo) isEnvUUIDUpgradeDone() (bool, error) {
	instanceData, closer := info.st.getRawCollection(instanceDataC)
	defer closer()

	query := instanceData.Find(bson.D{{"env-uuid", bson.D{{"$exists", true}}}})
	n, err := query.Count()
	if err != nil {
		return false, errors.Annotatef(err, "couldn't query instance upgrade status")
	}
	return n > 0, nil
}

// SetStatus sets the status of the current upgrade. Checks are made
// to ensure that status changes are performed in the correct order.
func (info *UpgradeInfo) SetStatus(status UpgradeStatus) error {
	var assertSane bson.D
	switch status {
	case UpgradePending, UpgradeComplete, UpgradeAborted:
		return errors.Errorf("cannot explicitly set upgrade status to \"%s\"", status)
	case UpgradeRunning:
		assertSane = bson.D{{"status", bson.D{{"$in",
			[]UpgradeStatus{UpgradePending, UpgradeRunning},
		}}}}
	case UpgradeFinishing:
		assertSane = bson.D{{"status", bson.D{{"$in",
			[]UpgradeStatus{UpgradeRunning, UpgradeFinishing},
		}}}}
	default:
		return errors.Errorf("unknown upgrade status: %s", status)
	}
	if info.doc.Id != currentUpgradeId {
		return errors.New("cannot set status on non-current upgrade")
	}

	ops := []txn.Op{{
		C:  upgradeInfoC,
		Id: currentUpgradeId,
		Assert: append(bson.D{{
			"previousVersion", info.doc.PreviousVersion,
		}, {
			"targetVersion", info.doc.TargetVersion,
		}}, assertSane...),
		Update: bson.D{{"$set", bson.D{{"status", status}}}},
	}}
	err := info.st.runTransaction(ops)
	if err == txn.ErrAborted {
		return errors.Errorf("cannot set upgrade status to %q: Another "+
			"status change may have occurred concurrently", status)
	}
	return errors.Annotate(err, "cannot set upgrade status")
}

// EnsureUpgradeInfo returns an UpgradeInfo describing a current upgrade between the
// supplied versions. If a matching upgrade is in progress, that upgrade is returned;
// if there's a mismatch, an error is returned. The supplied machine id must correspond
// to a current state server.
func (st *State) EnsureUpgradeInfo(machineId string, previousVersion, targetVersion version.Number) (*UpgradeInfo, error) {

	assertSanity, err := checkUpgradeInfoSanity(st, machineId, previousVersion, targetVersion)
	if err != nil {
		return nil, errors.Trace(err)
	}

	doc := upgradeInfoDoc{
		Id:                currentUpgradeId,
		PreviousVersion:   previousVersion,
		TargetVersion:     targetVersion,
		Status:            UpgradePending,
		Started:           time.Now().UTC(),
		StateServersReady: []string{machineId},
	}

	machine, err := st.Machine(machineId)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ops := []txn.Op{{
		C:      upgradeInfoC,
		Id:     currentUpgradeId,
		Assert: txn.DocMissing,
		Insert: doc,
	}, {
		C:      instanceDataC,
		Id:     machine.doc.DocID,
		Assert: txn.DocExists,
	}}
	if err := st.runRawTransaction(ops); err == nil {
		return &UpgradeInfo{st: st, doc: doc}, nil
	} else if err != txn.ErrAborted {
		return nil, errors.Annotate(err, "cannot create upgrade info")
	}

	if provisioned, err := st.isMachineProvisioned(machineId); err != nil {
		return nil, errors.Trace(err)
	} else if !provisioned {
		return nil, errors.Errorf(
			"machine %s is not provisioned and should not be participating in upgrades",
			machineId)
	}

	if info, err := ensureUpgradeInfoUpdated(st, machineId, previousVersion, targetVersion); err == nil {
		return info, nil
	} else if errors.Cause(err) != errUpgradeInfoNotUpdated {
		return nil, errors.Trace(err)
	}

	ops = []txn.Op{{
		C:      upgradeInfoC,
		Id:     currentUpgradeId,
		Assert: assertSanity,
		Update: bson.D{{
			"$addToSet", bson.D{{"stateServersReady", machineId}},
		}},
	}}
	switch err := st.runTransaction(ops); err {
	case nil:
		return ensureUpgradeInfoUpdated(st, machineId, previousVersion, targetVersion)
	case txn.ErrAborted:
		return nil, errors.New("upgrade info changed during update")
	}
	return nil, errors.Annotate(err, "cannot update upgrade info")
}

func (st *State) isMachineProvisioned(machineId string) (bool, error) {
	instanceData, closer := st.getRawCollection(instanceDataC)
	defer closer()

	for _, id := range []string{st.docID(machineId), machineId} {
		count, err := instanceData.FindId(id).Count()
		if err != nil {
			return false, errors.Annotate(err, "cannot read instance data")
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}

var errUpgradeInfoNotUpdated = errors.New("upgrade info not updated")

func ensureUpgradeInfoUpdated(st *State, machineId string, previousVersion, targetVersion version.Number) (*UpgradeInfo, error) {
	var doc upgradeInfoDoc
	if pdoc, err := currentUpgradeInfoDoc(st); err != nil {
		return nil, errors.Trace(err)
	} else {
		doc = *pdoc
	}

	if doc.PreviousVersion != previousVersion {
		return nil, errors.Errorf(
			"current upgrade info mismatch: expected previous version %s, got %s",
			previousVersion, doc.PreviousVersion)
	}
	if doc.TargetVersion != targetVersion {
		return nil, errors.Errorf(
			"current upgrade info mismatch: expected target version %s, got %s",
			targetVersion, doc.TargetVersion)
	}

	stateServersReady := set.NewStrings(doc.StateServersReady...)
	if !stateServersReady.Contains(machineId) {
		return nil, errors.Trace(errUpgradeInfoNotUpdated)
	}
	return &UpgradeInfo{st: st, doc: doc}, nil
}

// SetStateServerDone marks the supplied state machineId as having
// completed its upgrades. When SetStateServerDone is called by the
// last provisioned state server, the current upgrade info document
// will be archived with a status of UpgradeComplete.
func (info *UpgradeInfo) SetStateServerDone(machineId string) error {
	assertSanity, err := checkUpgradeInfoSanity(info.st, machineId,
		info.doc.PreviousVersion, info.doc.TargetVersion)
	if err != nil {
		return errors.Trace(err)
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		doc, err := currentUpgradeInfoDoc(info.st)
		if errors.IsNotFound(err) {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		switch doc.Status {
		case UpgradePending, UpgradeRunning:
			return nil, errors.New("upgrade has not yet run")
		}

		stateServersDone := set.NewStrings(doc.StateServersDone...)
		if stateServersDone.Contains(machineId) {
			return nil, jujutxn.ErrNoOperations
		}
		stateServersDone.Add(machineId)

		stateServersReady := set.NewStrings(doc.StateServersReady...)
		stateServersNotDone := stateServersReady.Difference(stateServersDone)
		if stateServersNotDone.IsEmpty() {
			// This is the last state server. Archive the current
			// upgradeInfo document.
			doc.StateServersDone = stateServersDone.SortedValues()
			return info.makeArchiveOps(doc, UpgradeComplete), nil
		}

		return []txn.Op{{
			C:  upgradeInfoC,
			Id: currentUpgradeId,
			// This is not the last state server, but we need to be
			// sure it still isn't when we run this.
			Assert: append(assertSanity, bson.D{{
				"stateServersDone", bson.D{{"$nin", stateServersNotDone.Values()}},
			}}...),
			Update: bson.D{{"$addToSet", bson.D{{"stateServersDone", machineId}}}},
		}}, nil
	}
	err = info.st.run(buildTxn)
	return errors.Annotate(err, "cannot complete upgrade")
}

// Abort marks the current upgrade as aborted. It should be called if
// the upgrade can't be completed for some reason.
func (info *UpgradeInfo) Abort() error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		doc, err := currentUpgradeInfoDoc(info.st)
		if errors.IsNotFound(err) {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		return info.makeArchiveOps(doc, UpgradeAborted), nil
	}
	err := info.st.run(buildTxn)
	return errors.Annotate(err, "cannot abort upgrade")
}

func (info *UpgradeInfo) makeArchiveOps(doc *upgradeInfoDoc, status UpgradeStatus) []txn.Op {
	doc.Status = status
	doc.Id = bson.NewObjectId().String() // change id to archive value
	return []txn.Op{{
		C:      upgradeInfoC,
		Id:     currentUpgradeId,
		Assert: assertExpectedVersions(doc.PreviousVersion, doc.TargetVersion),
		Remove: true,
	}, {
		C:      upgradeInfoC,
		Id:     doc.Id,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
}

// IsUpgrading returns true if an upgrade is currently in progress.
func (st *State) IsUpgrading() (bool, error) {
	doc, err := currentUpgradeInfoDoc(st)
	if doc != nil && err == nil {
		return true, nil
	} else if errors.IsNotFound(err) {
		return false, nil
	} else {
		return false, errors.Trace(err)
	}
}

// AbortCurrentUpgrade archives any current UpgradeInfo and sets its
// status to UpgradeAborted. Nothing happens if there's no current
// UpgradeInfo.
func (st *State) AbortCurrentUpgrade() error {
	doc, err := currentUpgradeInfoDoc(st)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return errors.Trace(err)
	}
	info := &UpgradeInfo{st: st, doc: *doc}
	return errors.Trace(info.Abort())

}

func currentUpgradeInfoDoc(st *State) (*upgradeInfoDoc, error) {
	var doc upgradeInfoDoc
	upgradeInfo, closer := st.getCollection(upgradeInfoC)
	defer closer()
	if err := upgradeInfo.FindId(currentUpgradeId).One(&doc); err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("current upgrade info")
	} else if err != nil {
		return nil, errors.Annotate(err, "cannot read upgrade info")
	}
	return &doc, nil
}

func checkUpgradeInfoSanity(st *State, machineId string, previousVersion, targetVersion version.Number) (bson.D, error) {
	if previousVersion.Compare(targetVersion) != -1 {
		return nil, errors.Errorf("cannot sanely upgrade from %s to %s", previousVersion, targetVersion)
	}
	stateServerInfo, err := st.StateServerInfo()
	if err != nil {
		return nil, errors.Annotate(err, "cannot read state servers")
	}
	validIds := set.NewStrings(stateServerInfo.MachineIds...)
	if !validIds.Contains(machineId) {
		return nil, errors.Errorf("machine %q is not a state server", machineId)
	}
	return assertExpectedVersions(previousVersion, targetVersion), nil
}

func assertExpectedVersions(previousVersion, targetVersion version.Number) bson.D {
	return bson.D{{
		"previousVersion", previousVersion,
	}, {
		"targetVersion", targetVersion,
	}}
}

// ClearUpgradeInfo clears information about an upgrade in progress. It returns
// an error if no upgrade is current.
func (st *State) ClearUpgradeInfo() error {
	ops := []txn.Op{{
		C:      upgradeInfoC,
		Id:     currentUpgradeId,
		Assert: txn.DocExists,
		Remove: true,
	}}
	err := st.runTransaction(ops)
	return errors.Annotate(err, "cannot clear upgrade info")
}
