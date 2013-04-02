package state

import (
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"
	"launchpad.net/juju-core/constraints"
)

// constraintsDoc is the mongodb representation of a constraints.Value.
type constraintsDoc struct {
	Arch     *string
	CpuCores *uint64
	CpuPower *uint64
	Mem      *uint64
}

func newConstraintsDoc(cons constraints.Value) constraintsDoc {
	return constraintsDoc{
		Arch:     cons.Arch,
		CpuCores: cons.CpuCores,
		CpuPower: cons.CpuPower,
		Mem:      cons.Mem,
	}
}

func createConstraintsOp(st *State, id string, cons constraints.Value) txn.Op {
	return txn.Op{
		C:      st.constraints.Name,
		Id:     id,
		Assert: txn.DocMissing,
		Insert: newConstraintsDoc(cons),
	}
}

func setConstraintsOp(st *State, id string, cons constraints.Value) txn.Op {
	return txn.Op{
		C:      st.constraints.Name,
		Id:     id,
		Assert: txn.DocExists,
		Update: D{{"$set", newConstraintsDoc(cons)}},
	}
}

func removeConstraintsOp(st *State, id string) txn.Op {
	return txn.Op{
		C:      st.constraints.Name,
		Id:     id,
		Assert: txn.DocExists,
		Remove: true,
	}
}

func readConstraints(st *State, id string) (constraints.Value, error) {
	doc := constraintsDoc{}
	if err := st.constraints.FindId(id).One(&doc); err == mgo.ErrNotFound {
		return constraints.Value{}, NotFoundf("constraints")
	} else if err != nil {
		return constraints.Value{}, err
	}
	return constraints.Value{
		Arch:     doc.Arch,
		CpuCores: doc.CpuCores,
		CpuPower: doc.CpuPower,
		Mem:      doc.Mem,
	}, nil
}

func writeConstraints(st *State, id string, cons constraints.Value) error {
	ops := []txn.Op{setConstraintsOp(st, id, cons)}
	if err := st.runner.Run(ops, "", nil); err != nil {
		return fmt.Errorf("cannot set constraints: %v", err)
	}
	return nil
}
