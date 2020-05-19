// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
This file defines infrastructure for synchronising controller tools
upgrades. Synchronisation is handled via a mongo DB document in the
"upgradeInfo" collection.

The functionality here is intended to be used as follows:

1. When controllers come up running the new tools version, they call
EnsureUpgradeInfo before running upgrade steps.

2a. Any secondary controller watches the UpgradeInfo document and
waits for the status to change to UpgradeFinishing.

2b. The master controller watches the UpgradeInfo document and waits
for AllProvisionedControllersReady to return true. This indicates
that all provisioned controllers have called EnsureUpgradeInfo and
are ready to upgrade.

3. The master controller calls SetStatus with UpgradeRunning and
runs its upgrade steps.

4. The master controller calls SetStatus with UpgradeFinishing and
then calls SetControllerDone with it's own machine id.

5. Secondary controllers, seeing that the status has changed to
UpgradeFinishing, run their upgrade steps and then call
SetControllerDone when complete.

6. Once the final controller calls SetControllerDone, the status is
changed to UpgradeComplete and the upgradeInfo document is archived.
*/

package state

import (
	"fmt"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"github.com/juju/version"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/status"
)

// UpgradeStatus describes the states an upgrade operation may be in.
type UpgradeStatus string

const (
	// UpgradePending indicates that an upgrade is queued but not yet started.
	UpgradePending UpgradeStatus = "pending"

	// UpgradeDBComplete indicates that the controller running the primary
	// MongoDB has completed running the database upgrade steps.
	UpgradeDBComplete UpgradeStatus = "db-complete"

	// UpgradeRunning indicates that the master controller has started
	// running non-DB upgrade logic, and other controllers are waiting for it.
	UpgradeRunning UpgradeStatus = "running"

	// UpgradeFinishing indicates that the master controller has finished
	// running all upgrade logic, and other controllers are catching up.
	UpgradeFinishing UpgradeStatus = "finishing"

	// UpgradeComplete indicates that all controllers have finished running
	// upgrade logic.
	UpgradeComplete UpgradeStatus = "complete"

	// UpgradeAborted indicates that the upgrade wasn't completed due
	// to some problem.
	UpgradeAborted UpgradeStatus = "aborted"

	// currentUpgradeId is the mongo _id of the current upgrade info document.
	currentUpgradeId = "current"
)

type upgradeInfoDoc struct {
	Id               string         `bson:"_id"`
	PreviousVersion  version.Number `bson:"previousVersion"`
	TargetVersion    version.Number `bson:"targetVersion"`
	Status           UpgradeStatus  `bson:"status"`
	Started          time.Time      `bson:"started"`
	ControllersReady []string       `bson:"controllersReady"`
	ControllersDone  []string       `bson:"controllersDone"`
}

// UpgradeInfo is used to synchronise controller upgrades.
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

// ControllersReady returns the machine ids for controllers that
// have signalled that they are ready for upgrade.
func (info *UpgradeInfo) ControllersReady() []string {
	result := make([]string, len(info.doc.ControllersReady))
	copy(result, info.doc.ControllersReady)
	return result
}

// ControllersDone returns the machine ids for controllers that
// have completed their upgrades.
func (info *UpgradeInfo) ControllersDone() []string {
	result := make([]string, len(info.doc.ControllersDone))
	copy(result, info.doc.ControllersDone)
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

// Watch returns a watcher for the state underlying the current
// UpgradeInfo instance. This is provided purely for convenience.
func (info *UpgradeInfo) Watch() NotifyWatcher {
	return info.st.WatchUpgradeInfo()
}

// AllProvisionedControllersReady returns true if and only if all controllers
// that have been started by the provisioner have called EnsureUpgradeInfo with
// matching versions.
//
// When this returns true the master state controller can begin it's
// own upgrade.
func (info *UpgradeInfo) AllProvisionedControllersReady() (bool, error) {
	provisioned, err := info.getProvisionedControllers()
	if err != nil {
		return false, errors.Trace(err)
	}
	ready := set.NewStrings(info.doc.ControllersReady...)
	missing := set.NewStrings(provisioned...).Difference(ready)
	return missing.IsEmpty(), nil
}

func (info *UpgradeInfo) getProvisionedControllers() ([]string, error) {
	var provisioned []string

	controllerIds, err := info.st.ControllerIds()
	if err != nil {
		return provisioned, errors.Annotate(err, "cannot read controllers")
	}

	// Extract current and provisioned controllers.
	instanceData, closer := info.st.db().GetRawCollection(instanceDataC)
	defer closer()

	query := bson.D{
		{"model-uuid", info.st.ModelUUID()},
		{"machineid", bson.D{{"$in", controllerIds}}},
	}
	iter := instanceData.Find(query).Select(bson.D{{"machineid", true}}).Iter()

	var doc bson.M
	for iter.Next(&doc) {
		provisioned = append(provisioned, doc["machineid"].(string))
	}
	if err := iter.Close(); err != nil {
		return provisioned, errors.Annotate(err, "cannot read provisioned machines")
	}
	return provisioned, nil
}

// upgradeStatusHistoryAndOps sets the model's status history and returns ops for
// setting model status according to the UpgradeStatus.
func upgradeStatusHistoryAndOps(mb modelBackend, upgradeStatus UpgradeStatus, now time.Time) ([]txn.Op, error) {
	var modelStatus status.Status
	var msg string
	switch upgradeStatus {
	case UpgradeComplete:
		modelStatus = status.Available
		msg = fmt.Sprintf("upgraded on %q", now.UTC().Format(time.RFC3339))
	case UpgradeRunning:
		modelStatus = status.Busy
		msg = fmt.Sprintf("upgrade in progress since %q", now.UTC().Format(time.RFC3339))
	case UpgradeAborted:
		modelStatus = status.Available
		msg = fmt.Sprintf("last upgrade aborted on %q", now.UTC().Format(time.RFC3339))
	default:
		return []txn.Op{}, nil
	}
	doc := statusDoc{
		Status:     modelStatus,
		StatusInfo: msg,
		Updated:    now.UnixNano(),
	}
	ops, err := statusSetOps(mb.db(), doc, modelGlobalKey)
	if err != nil {
		return nil, errors.Trace(err)
	}
	_, _ = probablyUpdateStatusHistory(mb.db(), modelGlobalKey, doc)
	return ops, nil
}

// SetStatus sets the status of the current upgrade. Checks are made
// to ensure that status changes are performed in the correct order.
func (info *UpgradeInfo) SetStatus(status UpgradeStatus) error {
	var assertSane bson.D
	switch status {
	case UpgradePending, UpgradeComplete, UpgradeAborted:
		return errors.Errorf("cannot explicitly set upgrade status to \"%s\"", status)
	case UpgradeDBComplete:
		assertSane = bson.D{{"status", bson.D{{"$in",
			[]UpgradeStatus{UpgradePending, UpgradeDBComplete},
		}}}}
	case UpgradeRunning:
		assertSane = bson.D{{"status", bson.D{{"$in",
			[]UpgradeStatus{UpgradeDBComplete, UpgradeRunning},
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

	extraOps, err := upgradeStatusHistoryAndOps(info.st, status, info.st.clock().Now())
	if err != nil {
		return errors.Trace(err)
	}
	if len(extraOps) > 0 {
		ops = append(ops, extraOps...)
	}
	err = info.st.db().RunTransaction(ops)
	if err == txn.ErrAborted {
		return errors.Errorf("cannot set upgrade status to %q: Another "+
			"status change may have occurred concurrently", status)
	}
	return errors.Annotate(err, "cannot set upgrade status")
}

// EnsureUpgradeInfo returns an UpgradeInfo describing a current upgrade between the
// supplied versions. If a matching upgrade is in progress, that upgrade is returned;
// if there's a mismatch, an error is returned.
func (st *State) EnsureUpgradeInfo(controllerId string, previousVersion, targetVersion version.Number) (*UpgradeInfo, error) {

	assertSanity, err := checkUpgradeInfoSanity(st, controllerId, previousVersion, targetVersion)
	if err != nil {
		return nil, errors.Trace(err)
	}

	doc := upgradeInfoDoc{
		Id:               currentUpgradeId,
		PreviousVersion:  previousVersion,
		TargetVersion:    targetVersion,
		Status:           UpgradePending,
		Started:          st.clock().Now().UTC(),
		ControllersReady: []string{controllerId},
	}

	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	hasMachine := m.Type() == ModelTypeIAAS

	ops := []txn.Op{{
		C:      upgradeInfoC,
		Id:     currentUpgradeId,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
	if hasMachine {
		machine, err := st.Machine(controllerId)
		if err != nil {
			return nil, errors.Trace(err)
		}

		ops = append(ops, txn.Op{
			C:      instanceDataC,
			Id:     machine.doc.DocID,
			Assert: txn.DocExists,
		})
	}
	if err := st.runRawTransaction(ops); err == nil {
		return &UpgradeInfo{st: st, doc: doc}, nil
	} else if err != txn.ErrAborted {
		return nil, errors.Annotate(err, "cannot create upgrade info")
	}

	if hasMachine {
		if provisioned, err := st.isMachineProvisioned(controllerId); err != nil {
			return nil, errors.Trace(err)
		} else if !provisioned {
			return nil, errors.Errorf(
				"machine %s is not provisioned and should not be participating in upgrades",
				controllerId)
		}
	}

	if info, err := ensureUpgradeInfoUpdated(st, controllerId, previousVersion, targetVersion); err == nil {
		return info, nil
	} else if errors.Cause(err) != errUpgradeInfoNotUpdated {
		return nil, errors.Trace(err)
	}

	ops = []txn.Op{{
		C:      upgradeInfoC,
		Id:     currentUpgradeId,
		Assert: assertSanity,
		Update: bson.D{{
			"$addToSet", bson.D{{"controllersReady", controllerId}},
		}},
	}}
	switch err := st.db().RunTransaction(ops); err {
	case nil:
		return ensureUpgradeInfoUpdated(st, controllerId, previousVersion, targetVersion)
	case txn.ErrAborted:
		return nil, errors.New("upgrade info changed during update")
	}
	return nil, errors.Annotate(err, "cannot update upgrade info")
}

func (st *State) isMachineProvisioned(machineId string) (bool, error) {
	instanceData, closer := st.db().GetRawCollection(instanceDataC)
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

func ensureUpgradeInfoUpdated(st *State, controllerId string, previousVersion, targetVersion version.Number) (*UpgradeInfo, error) {
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

	controllersReady := set.NewStrings(doc.ControllersReady...)
	if !controllersReady.Contains(controllerId) {
		return nil, errors.Trace(errUpgradeInfoNotUpdated)
	}
	return &UpgradeInfo{st: st, doc: doc}, nil
}

// SetControllerDone marks the supplied state controllerId as having
// completed its upgrades. When SetControllerDone is called by the
// last provisioned controller, the current upgrade info document
// will be archived with a status of UpgradeComplete.
func (info *UpgradeInfo) SetControllerDone(controllerId string) error {
	assertSanity, err := checkUpgradeInfoSanity(info.st, controllerId,
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

		controllersDone := set.NewStrings(doc.ControllersDone...)
		if controllersDone.Contains(controllerId) {
			return nil, jujutxn.ErrNoOperations
		}
		controllersDone.Add(controllerId)

		controllersReady := set.NewStrings(doc.ControllersReady...)
		controllersNotDone := controllersReady.Difference(controllersDone)
		if controllersNotDone.IsEmpty() {
			// This is the last controller. Archive the current
			// upgradeInfo document.
			doc.ControllersDone = controllersDone.SortedValues()

			ops := info.makeArchiveOps(doc, UpgradeComplete)
			extraOps, err := upgradeStatusHistoryAndOps(info.st, UpgradeComplete, info.st.clock().Now())
			if err != nil {
				return nil, errors.Trace(err)
			}
			if len(extraOps) > 0 {
				ops = append(ops, extraOps...)
			}

			return ops, nil
		}

		return []txn.Op{{
			C:  upgradeInfoC,
			Id: currentUpgradeId,
			// This is not the last controller, but we need to be
			// sure it still isn't when we run this.
			Assert: append(assertSanity, bson.D{{
				"controllersDone", bson.D{{"$nin", controllersNotDone.Values()}},
			}}...),
			Update: bson.D{{"$addToSet", bson.D{{"controllersDone", controllerId}}}},
		}}, nil
	}
	err = info.st.db().Run(buildTxn)
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
		ops := info.makeArchiveOps(doc, UpgradeAborted)
		extraOps, err := upgradeStatusHistoryAndOps(info.st, UpgradeAborted, info.st.clock().Now())
		if err != nil {
			return nil, errors.Trace(err)
		}
		if len(extraOps) > 0 {
			ops = append(ops, extraOps...)
		}

		return ops, nil
	}
	err := info.st.db().Run(buildTxn)
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
	upgradeInfo, closer := st.db().GetCollection(upgradeInfoC)
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
	controllerIds, err := st.SafeControllerIds()
	if err != nil {
		return nil, errors.Annotate(err, "cannot read controller ids")
	}
	validIds := set.NewStrings(controllerIds...)
	if !validIds.Contains(machineId) {
		return nil, errors.Errorf("machine %q is not a controller", machineId)
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
	err := st.db().RunTransaction(ops)
	return errors.Annotate(err, "cannot clear upgrade info")
}
