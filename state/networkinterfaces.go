// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"gopkg.in/mgo.v2/bson"
)

// TODO(dimitern): Drop networkinterfacesC collection and the related code as
// we're using interfacesC instead now.

// networkInterfaceDoc represents a network interface for a machine on
// a given network.
//
// TODO(dimitern): Drop this, no longer used.
type networkInterfaceDoc struct {
	Id            bson.ObjectId `bson:"_id"`
	EnvUUID       string        `bson:"env-uuid"`
	MACAddress    string        `bson:"macaddress"`
	InterfaceName string        `bson:"interfacename"`
	NetworkName   string        `bson:"networkname"`
	MachineId     string        `bson:"machineid"`
	IsVirtual     bool          `bson:"isvirtual"`
	IsDisabled    bool          `bson:"isdisabled"`
}
