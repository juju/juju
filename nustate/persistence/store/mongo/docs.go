// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/nustate/model"
	"github.com/juju/juju/nustate/persistence/transaction"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

var _ storeModel = (*machinePortRangesDoc)(nil)

type unitPortRangesDoc map[string][]network.PortRange

type machinePortRangesDoc struct {
	// Allows the model to be included in a model txn
	transaction.BuildingBlock

	doc struct {
		DocID          string                       `bson:"doc-id"`
		MachineID      string                       `bson:"machine-id"`
		UnitPortRanges map[string]unitPortRangesDoc `bson:"unit-port-ranges"`
		TxnRevno       int                          `bson:"txn-revno"`
	}

	docExists bool
	remove    bool
}

func (m *machinePortRangesDoc) MachineID() string {
	return m.doc.MachineID
}

func (m *machinePortRangesDoc) ForUnit(unitName string) *model.UnitPortRanges {
	return &model.UnitPortRanges{
		OpenPortsByEndpoint: m.doc.UnitPortRanges[unitName],
	}
}

func (m *machinePortRangesDoc) SetPortRanges(upr map[string]*model.UnitPortRanges) {
	m.doc.UnitPortRanges = make(map[string]unitPortRangesDoc)
	for unitName, ranges := range upr {
		m.doc.UnitPortRanges[unitName] = ranges.OpenPortsByEndpoint
	}
}

func (m *machinePortRangesDoc) Remove() {
	m.remove = true
}

func (m *machinePortRangesDoc) persistOp() []txn.Op {
	if m.remove {
		return []txn.Op{{
			C:      "openedPorts",
			Id:     m.doc.DocID,
			Assert: txn.DocExists,
			Remove: true,
		}}
	} else if !m.docExists {
		return []txn.Op{{
			C:      "openedPorts",
			Id:     m.doc.DocID,
			Assert: txn.DocMissing,
			Insert: m.doc,
		}}
	}

	return []txn.Op{{
		C:      "openedPorts",
		Id:     m.doc.DocID,
		Assert: bson.D{{"txn-revno", m.doc.TxnRevno}},
		Update: bson.D{{"$set", bson.D{{"unit-port-ranges", m.doc.UnitPortRanges}}}},
	}}
}
