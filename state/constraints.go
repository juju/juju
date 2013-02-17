package state

import (
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"
	"strings"
)

// Constraints describes a user's requirements of the hardware on which units
// of a service will run. Constraints are used to choose an existing machine
// onto which a unit will be deployed, or to provision a new machine if no
// existing one satisfies the requirements.
type Constraints struct {

	// CpuCores, if not nil, indicates that a machine must have at least that
	// number of effective cores available.
	CpuCores *uint64

	// CpuPower, if not nil, indicates that a machine must have at least that
	// amount of CPU power available, where 100 CpuPower is considered to be
	// equivalent to 1 Amazon ECU (or, roughly, a single 2007-era Xeon).
	CpuPower *uint64

	// Mem, if not nil, indicates that a machine must have at least that many
	// megabytes of RAM.
	Mem *uint64
}

// String expresses a Constraints in the language in which it was specified.
func (c Constraints) String() string {
	var strs []string
	if c.CpuCores != nil {
		strs = append(strs, "cpu-cores="+uintStr(*c.CpuCores))
	}
	if c.CpuPower != nil {
		strs = append(strs, "cpu-power="+uintStr(*c.CpuPower))
	}
	if c.Mem != nil {
		s := uintStr(*c.Mem)
		if s != "" {
			s += "M"
		}
		strs = append(strs, "mem="+s)
	}
	return strings.Join(strs, " ")
}

func uintStr(i uint64) string {
	if i == 0 {
		return ""
	}
	return fmt.Sprintf("%d", i)
}

type constraintsDoc struct {
	CpuCores *uint64
	CpuPower *uint64
	Mem      *uint64
}

func newConstraintsDoc(cons Constraints) constraintsDoc {
	return constraintsDoc{
		CpuCores: cons.CpuCores,
		CpuPower: cons.CpuPower,
		Mem:      cons.Mem,
	}
}

func createConstraintsOp(st *State, id string, cons Constraints) txn.Op {
	return txn.Op{
		C:      st.constraints.Name,
		Id:     id,
		Assert: txn.DocMissing,
		Insert: newConstraintsDoc(cons),
	}
}

func readConstraints(st *State, id string) (Constraints, error) {
	doc := constraintsDoc{}
	if err := st.constraints.FindId(id).One(&doc); err == mgo.ErrNotFound {
		return Constraints{}, notFoundf("constraints")
	} else if err != nil {
		return Constraints{}, err
	}
	return Constraints{
		CpuCores: doc.CpuCores,
		CpuPower: doc.CpuPower,
		Mem:      doc.Mem,
	}, nil
}

func writeConstraints(st *State, id string, cons Constraints) error {
	ops := []txn.Op{{
		C:      st.constraints.Name,
		Id:     id,
		Assert: txn.DocExists,
		Update: D{{"$set", newConstraintsDoc(cons)}},
	}}
	if err := st.runner.Run(ops, "", nil); err != nil {
		return fmt.Errorf("cannot set constraints: %v", err)
	}
	return nil
}
