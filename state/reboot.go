package state

import (
	"fmt"

	"github.com/juju/utils"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

var _ RebootFlagSetter = (*Machine)(nil)
var _ RebootActionGetter = (*Machine)(nil)

type rebootAction string

const (
	// ShouldDoNothing instructs a machine agent that no action
	// is required on its part
	ShouldDoNothing rebootAction = "noop"
	// ShouldReboot instructs a machine to reboot
	// this happens when a hook running on a machine, requests
	// a reboot
	ShouldReboot rebootAction = "reboot"
	// ShouldShutdown instructs a machine to shut down. This usually
	// happens when running inside a container, and a hook on the parent
	// machine requests a reboot
	ShouldShutdown rebootAction = "shutdown"
)

// RebootAction defines the action a machine should
// take when a hook needs to reboot. The UUID is used to
// keep track of reboot actions on machines
type RebootAction struct {
	Action rebootAction
	UUID   string
}

// rebootDoc will hold the reboot flag for a machine.
type rebootDoc struct {
	Id   string `bson:"_id"`
	UUID string `bson:"UUID"`
}

func addRebootDocOps(machineId string, uuid string) []txn.Op {
	ops := []txn.Op{{
		C:      machinesC,
		Id:     machineId,
		Assert: notDeadDoc,
	}, {
		C:      rebootC,
		Id:     machineId,
		Assert: txn.DocMissing,
		Insert: rebootDoc{Id: machineId, UUID: uuid},
	}}
	return ops
}

func removeRebootDocOps(machineId string) txn.Op {
	ops := txn.Op{
		C:      rebootC,
		Id:     machineId,
		Remove: true,
	}
	return ops
}

func (m *Machine) setFlag() error {
	if m.Life() == Dead {
		return mgo.ErrNotFound
	}
	uuid, err := utils.NewUUID()
	if err != nil {
		return err
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if m.Life() != Dead {
				// Previous error caused by reboot document existance
				return nil, nil
			} else {
				return nil, mgo.ErrNotFound
			}
		}
		return addRebootDocOps(m.Id(), uuid.String()), nil
	}
	if err = m.st.run(buildTxn); err == txn.ErrAborted {
		return mgo.ErrNotFound
	} else if err != nil {
		return err
	}

	return nil
}

func (m *Machine) clearFlag() error {
	reboot, closer := m.st.getCollection(rebootC)
	defer closer()

	t := []txn.Op{
		removeRebootDocOps(m.Id()),
	}
	count, err := reboot.FindId(m.Id()).Count()
	if count == 0 {
		return nil
	}

	err = m.st.runTransaction(t)
	if err != nil {
		return errors.Errorf("failed to clear reboot flag: %v", err)
	}
	return nil
}

// SetRebootFlag sets the reboot flag of a machine to a boolean value. It will also
// do a lazy create of a reboot document if needed; i.e. If a document
// does not exist yet for this machine, it will create it.
func (m *Machine) SetRebootFlag(flag bool) error {
	if flag {
		return m.setFlag()
	}
	return m.clearFlag()
}

// GetRebootFlag returns the reboot flag for this machine.
func (m *Machine) GetRebootFlag() (bool, error) {
	rebootCol, closer := m.st.getCollection(rebootC)
	defer closer()

	count, err := rebootCol.FindId(m.Id()).Count()
	if err != nil {
		return false, fmt.Errorf("failed to get reboot flag: %v", err)
	}
	if count == 0 {
		return false, nil
	}
	return true, nil
}

func (m *Machine) machinesToCareAboutRebootsFor() []string {
	var possibleIds []string
	for currentId := m.Id(); currentId != ""; {
		possibleIds = append(possibleIds, currentId)
		currentId = ParentId(currentId)
	}
	return possibleIds
}

// ShouldRebootOrShutdown check if the current node should reboot or shutdown
// If we are a container, and our parent needs to reboot, this should return:
// ShouldShutdown
func (m *Machine) ShouldRebootOrShutdown() (RebootAction, error) {
	rebootCol, closer := m.st.getCollection(rebootC)
	defer closer()

	machines := m.machinesToCareAboutRebootsFor()

	docs := []rebootDoc{}
	sel := bson.D{{"_id", bson.D{{"$in", machines}}}}
	if err := rebootCol.Find(sel).All(&docs); err != nil {
		return RebootAction{Action: ShouldDoNothing}, errors.Trace(err)
	}

	iNeedReboot := false
	var uuid string
	for _, val := range docs {
		if val.Id != m.doc.Id {
			return RebootAction{Action: ShouldShutdown, UUID: val.UUID}, nil
		}
		uuid = val.UUID
		iNeedReboot = true
	}
	if iNeedReboot {
		return RebootAction{Action: ShouldReboot, UUID: uuid}, nil
	}
	return RebootAction{Action: ShouldDoNothing}, nil
}

type RebootFlagSetter interface {
	SetRebootFlag(flag bool) error
}

type RebootActionGetter interface {
	ShouldRebootOrShutdown() (RebootAction, error)
}
