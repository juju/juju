// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Low-level functionality for interacting with the logs collection.

package state

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const logsDB = "logs"
const logsC = "logs"

// InitDbLogs sets up the indexes for the logs collection. It should
// be called as state is opened. It is idempotent.
func InitDbLogs(session *mgo.Session) error {
	logColl := session.DB(logsDB).C(logsC)
	for _, key := range [][]string{{"e", "t"}, {"e", "n"}} {
		err := logColl.EnsureIndex(mgo.Index{Key: key})
		if err != nil {
			return errors.Annotate(err, "cannot create index for logs collection")
		}
	}
	return nil
}

// logDoc describes log messages stored in MongoDB.
//
// Single character field names are used for serialisation to save
// space. These documents will be inserted 1000's of times and each
// document includes the field names.
type logDoc struct {
	Id       bson.ObjectId `bson:"_id"`
	Time     time.Time     `bson:"t"`
	EnvUUID  string        `bson:"e"`
	Entity   string        `bson:"n"` // e.g. "machine-0"
	Module   string        `bson:"m"` // e.g. "juju.worker.firewaller"
	Location string        `bson:"l"` // "filename:lineno"
	Level    loggo.Level   `bson:"v"`
	Message  string        `bson:"x"`
}

type dbLogger struct {
	logColl *mgo.Collection
	envUUID string
	entity  string
}

// NewDbLogger returns a dbLogger instance which is used to write logs
// to the database.
func NewDbLogger(st *State, entity names.Tag) *dbLogger {
	session := st.MongoSession().Copy()
	// To improve throughput, only wait for the logs to be written to
	// the primary. For some reason, this makes a huge difference even
	// when the replicaset only has one member (i.e. a single primary).
	session.SetSafe(&mgo.Safe{
		W: 1,
	})
	db := session.DB(logsDB)
	return &dbLogger{
		logColl: db.C(logsC).With(session),
		envUUID: st.EnvironUUID(),
		entity:  entity.String(),
	}
}

// Log writes a log message to the database.
func (logger *dbLogger) Log(t time.Time, module string, location string, level loggo.Level, msg string) error {
	return logger.logColl.Insert(&logDoc{
		Id:       bson.NewObjectId(),
		Time:     t,
		EnvUUID:  logger.envUUID,
		Entity:   logger.entity,
		Module:   module,
		Location: location,
		Level:    level,
		Message:  msg,
	})
}

// Close cleans up resources used by the dbLogger instance.
func (logger *dbLogger) Close() {
	if logger.logColl != nil {
		logger.logColl.Database.Session.Close()
	}
}
