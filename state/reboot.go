package state

import (
	"fmt"

	"github.com/juju/errors"
	// "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/apiserver/params"
)

var _ RebootFlagSetter = (*Machine)(nil)
var _ RebootActionGetter = (*Machine)(nil)

// rebootDoc will hold the reboot flag for a machine.
type rebootDoc struct {
	Id string `bson:"_id"`
}

func addRebootDocOps(machineId string) []txn.Op {
	ops := []txn.Op{{
		C:      machinesC,
		Id:     machineId,
		Assert: notDeadDoc,
	}, {
		C:      rebootC,
		Id:     machineId,
		Insert: rebootDoc{Id: machineId},
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

// SetRebootFlag sets the reboot flag of a machine to a boolean value. It will also
// do a lazy create of a reboot document if needed; i.e. If a document
// does not exist yet for this machine, it will create it.
func (m *Machine) SetRebootFlag(flag bool) error {
	reboot, closer := m.st.getCollection(rebootC)
	defer closer()
	t := addRebootDocOps(m.Id())

	count, err := reboot.FindId(m.Id()).Count()
	if flag == false {
		if count == 0 {
			return nil
		} else {
			t = []txn.Op{
				removeRebootDocOps(m.Id()),
			}
		}
	}

	err = m.st.runTransaction(t)
	if err != nil {
		return errors.Errorf("Failed to set reboot flag: %v", err)
	}
	return nil
}

// GetRebootFlag returns the reboot flag for this machine.
func (m *Machine) GetRebootFlag() (bool, error) {
	rebootCol, closer := m.st.getCollection(rebootC)
	defer closer()

	count, err := rebootCol.FindId(m.Id()).Count()
	if err != nil {
		return false, fmt.Errorf("Failed to get reboot flag: %v", err)
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
func (m *Machine) ShouldRebootOrShutdown() (params.RebootAction, error) {
	rebootCol, closer := m.st.getCollection(rebootC)
	defer closer()

	machines := m.machinesToCareAboutRebootsFor()

	docs := []rebootDoc{}
	sel := bson.D{{"_id", bson.D{{"$in", machines}}}}
	if err := rebootCol.Find(sel).All(&docs); err != nil {
		return params.ShouldDoNothing, errors.Trace(err)
	}

	iNeedReboot := false
	for _, val := range docs {
		if val.Id != m.doc.Id {
			return params.ShouldShutdown, nil
		}
		iNeedReboot = true
	}
	if iNeedReboot {
		return params.ShouldReboot, nil
	}
	return params.ShouldDoNothing, nil
}

type RebootFlagSetter interface {
	SetRebootFlag(flag bool) error
}

type RebootActionGetter interface {
	ShouldRebootOrShutdown() (params.RebootAction, error)
}
