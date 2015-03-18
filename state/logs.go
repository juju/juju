// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Low-level functionality for interacting with the logs collection.

package state

import (
	"time"

	"github.com/dustin/go-humanize"
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
	logsColl := session.DB(logsDB).C(logsC)
	for _, key := range [][]string{{"e", "t"}, {"e", "n"}} {
		err := logsColl.EnsureIndex(mgo.Index{Key: key})
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

type DbLogger struct {
	logsColl *mgo.Collection
	envUUID  string
	entity   string
}

// NewDbLogger returns a DbLogger instance which is used to write logs
// to the database.
func NewDbLogger(st *State, entity names.Tag) *DbLogger {
	_, logsColl := initLogsSession(st)
	return &DbLogger{
		logsColl: logsColl,
		envUUID:  st.EnvironUUID(),
		entity:   entity.String(),
	}
}

// Log writes a log message to the database.
func (logger *DbLogger) Log(t time.Time, module string, location string, level loggo.Level, msg string) error {
	return logger.logsColl.Insert(&logDoc{
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

// Close cleans up resources used by the DbLogger instance.
func (logger *DbLogger) Close() {
	if logger.logsColl != nil {
		logger.logsColl.Database.Session.Close()
	}
}

// PruneLogs removes old log documents in order to control the size of
// logs collection. All logs older than minLogTime are
// removed. Further removal is also performed if the logs collection
// size is greater than maxLogsMB.
func PruneLogs(st *State, minLogTime time.Time, maxLogsMB int) error {
	session, logsColl := initLogsSession(st)
	defer session.Close()

	envUUIDs, err := getEnvsInLogs(logsColl)
	if err != nil {
		return errors.Annotate(err, "failed to get log counts")
	}

	pruneCounts := make(map[string]int)

	// Remove old log entries (per environment UUID to take advantage
	// of indexes on the logs collection).
	for _, envUUID := range envUUIDs {
		removeInfo, err := logsColl.RemoveAll(bson.M{
			"e": envUUID,
			"t": bson.M{"$lt": minLogTime},
		})
		if err != nil {
			return errors.Annotate(err, "failed to prune logs by time")
		}
		pruneCounts[envUUID] = removeInfo.Removed
	}

	// Do further pruning if the logs collection is over the maximum size.
	for {
		collMB, err := getCollectionMB(logsColl)
		if err != nil {
			return errors.Annotate(err, "failed to retrieve log counts")
		}
		if collMB <= maxLogsMB {
			break
		}

		envUUID, count, err := findEnvWithMostLogs(logsColl, envUUIDs)
		if err != nil {
			return errors.Annotate(err, "log count query failed")
		}
		if count < 5000 {
			break // Pruning is not worthwhile
		}

		// Remove the oldest 1% of log records for the environment.
		toRemove := int(float64(count) * 0.01)

		// Find the threshold timestammp to start removing from.
		// NOTE: this assumes that there are no more logs being added
		// for the time range being pruned (which should be true for
		// any realistic minimum log collection size).
		tsQuery := logsColl.Find(bson.M{"e": envUUID}).Sort("t")
		tsQuery = tsQuery.Skip(toRemove)
		tsQuery = tsQuery.Select(bson.M{"t": 1})
		var doc bson.M
		err = tsQuery.One(&doc)
		if err != nil {
			return errors.Annotate(err, "log pruning timestamp query failed")
		}
		thresholdTs := doc["t"].(time.Time)

		// Remove old records.
		removeInfo, err := logsColl.RemoveAll(bson.M{
			"e": envUUID,
			"t": bson.M{"$lt": thresholdTs},
		})
		if err != nil {
			return errors.Annotate(err, "log pruning failed")
		}
		pruneCounts[envUUID] += removeInfo.Removed
	}

	for envUUID, count := range pruneCounts {
		if count > 0 {
			logger.Debugf("pruned %d logs for environment %s", count, envUUID)
		}
	}
	return nil
}

// initLogsSession creates a new session suitable for logging updates,
// returning the session and a logs mgo.Collection connected to that
// session.
func initLogsSession(st *State) (*mgo.Session, *mgo.Collection) {
	// To improve throughput, only wait for the logs to be written to
	// the primary. For some reason, this makes a huge difference even
	// when the replicaset only has one member (i.e. a single primary).
	session := st.MongoSession().Copy()
	session.SetSafe(&mgo.Safe{
		W: 1,
	})
	db := session.DB(logsDB)
	return session, db.C(logsC).With(session)
}

// getCollectionMB returns the size of a MongoDB collection (in
// bytes), excluding space used by indexes.
func getCollectionMB(coll *mgo.Collection) (int, error) {
	var result bson.M
	err := coll.Database.Run(bson.M{
		"collStats": coll.Name,
		"scale":     humanize.MiByte,
	}, &result)
	if err != nil {
		return 0, errors.Trace(err)
	}
	return result["size"].(int), nil
}

// getEnvsInLogs returns the unique envrionment UUIDs that exist in
// the logs collection. This uses the one of the indexes on the
// collection and should be fast.
func getEnvsInLogs(coll *mgo.Collection) ([]string, error) {
	var envUUIDs []string
	err := coll.Find(nil).Distinct("e", &envUUIDs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return envUUIDs, nil
}

// findEnvWithMostLogs returns the envUUID and log count for the
// environment with the most logs in the logs collection.
func findEnvWithMostLogs(logsColl *mgo.Collection, envUUIDs []string) (string, int, error) {
	var maxEnvUUID string
	var maxCount int
	for _, envUUID := range envUUIDs {
		count, err := getLogCountForEnv(logsColl, envUUID)
		if err != nil {
			return "", -1, errors.Trace(err)
		}
		if count > maxCount {
			maxEnvUUID = envUUID
			maxCount = count
		}
	}
	return maxEnvUUID, maxCount, nil
}

// getLogCountForEnv returns the number of log records stored for a
// given environment.
func getLogCountForEnv(coll *mgo.Collection, envUUID string) (int, error) {
	count, err := coll.Find(bson.M{"e": envUUID}).Count()
	if err != nil {
		return -1, errors.Annotate(err, "failed to get log count")
	}
	return count, nil
}
