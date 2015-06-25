// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Low-level functionality for interacting with the logs collection
// and tailing logs from the replication oplog.

package state

import (
	"regexp"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/deque"
	"github.com/juju/utils/set"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/tomb"

	"github.com/juju/juju/mongo"
)

const logsDB = "logs"
const logsC = "logs"

// LoggingState describes the methods on State required for logging to
// the database.
type LoggingState interface {
	EnvironUUID() string
	MongoSession() *mgo.Session
}

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
func NewDbLogger(st LoggingState, entity names.Tag) *DbLogger {
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

// LogTailer allows for retrieval of Juju's logs from MongoDB. It
// first returns any matching already recorded logs and then waits for
// additional matching logs as they appear.
type LogTailer interface {
	// Logs returns the channel through which the LogTailer returns
	// Juju logs. It will be closed when the tailer stops.
	Logs() <-chan *LogRecord

	// Dying returns a channel which will be closed as the LogTailer
	// stops.
	Dying() <-chan struct{}

	// Stop is used to request that the LogTailer stops. It blocks
	// unil the LogTailer has stopped.
	Stop() error

	// Err returns the error that caused the LogTailer to stopped. If
	// it hasn't stopped or stopped without error nil will be
	// returned.
	Err() error
}

// LogRecord defines a single Juju log message as returned by
// LogTailer.
type LogRecord struct {
	Time     time.Time
	Entity   string
	Module   string
	Location string
	Level    loggo.Level
	Message  string
}

// LogTailerParams specifies the filtering a LogTailer should apply to
// logs in order to decide which to return.
type LogTailerParams struct {
	StartTime     time.Time
	MinLevel      loggo.Level
	InitialLines  int
	IncludeEntity []string
	ExcludeEntity []string
	IncludeModule []string
	ExcludeModule []string
	Oplog         *mgo.Collection // For testing only
}

// oplogOverlap is used to decide on the initial oplog timestamp to
// use when the LogTailer transitions from querying the logs
// collection to tailing the oplog. Oplog records with a timestamp >=
// tolastTsFromLogsCollection - oplogOverlap will be considered. This
// is to allow for delayed log writes, clock skew between the Juju
// cluster hosts and log writes that occur during the transition
// period.
const oplogOverlap = time.Minute

// This is the maximum number of log document ids that will be tracked
// to avoid re-reporting logs when transitioning between querying the
// logs collection and tailing the oplog.
//
// The value was calculated by looking at the per-minute peak log
// output of large broken environments with logging at DEBUG.
var maxRecentLogIds = int(oplogOverlap.Minutes() * 150000)

// NewLogTailer returns a LogTailer which filters according to the
// parameters given.
func NewLogTailer(st LoggingState, params *LogTailerParams) LogTailer {
	session := st.MongoSession().Copy()
	t := &logTailer{
		envUUID:   st.EnvironUUID(),
		session:   session,
		logsColl:  session.DB(logsDB).C(logsC).With(session),
		params:    params,
		logCh:     make(chan *LogRecord),
		recentIds: newRecentIdTracker(maxRecentLogIds),
	}
	go func() {
		err := t.loop()
		t.tomb.Kill(errors.Cause(err))
		close(t.logCh)
		session.Close()
		t.tomb.Done()
	}()
	return t
}

type logTailer struct {
	tomb      tomb.Tomb
	envUUID   string
	session   *mgo.Session
	logsColl  *mgo.Collection
	params    *LogTailerParams
	logCh     chan *LogRecord
	lastTime  time.Time
	recentIds *recentIdTracker
}

// Logs implements the LogTailer interface.
func (t *logTailer) Logs() <-chan *LogRecord {
	return t.logCh
}

// Dying implements the LogTailer interface.
func (t *logTailer) Dying() <-chan struct{} {
	return t.tomb.Dying()
}

// Stop implements the LogTailer interface.
func (t *logTailer) Stop() error {
	t.tomb.Kill(nil)
	return t.tomb.Wait()
}

// Err implements the LogTailer interface.
func (t *logTailer) Err() error {
	return t.tomb.Err()
}

func (t *logTailer) loop() error {
	err := t.processCollection()
	if err != nil {
		return errors.Trace(err)
	}

	err = t.tailOplog()
	return errors.Trace(err)
}

func (t *logTailer) processCollection() error {
	// Create a selector from the params.
	sel := t.paramsToSelector(t.params, "")
	query := t.logsColl.Find(sel)

	if t.params.InitialLines > 0 {
		// This is a little racy but it's good enough.
		count, err := query.Count()
		if err != nil {
			return errors.Annotate(err, "query count failed")
		}
		if skipOver := count - t.params.InitialLines; skipOver > 0 {
			query = query.Skip(skipOver)
		}
	}

	iter := query.Sort("t", "_id").Iter()
	doc := new(logDoc)
	for iter.Next(doc) {
		select {
		case <-t.tomb.Dying():
			return errors.Trace(tomb.ErrDying)
		case t.logCh <- logDocToRecord(doc):
			t.lastTime = doc.Time
			t.recentIds.Add(doc.Id)
		}
	}
	return errors.Trace(iter.Close())
}

func (t *logTailer) tailOplog() error {
	recentIds := t.recentIds.AsSet()

	newParams := t.params
	newParams.StartTime = t.lastTime
	oplogSel := append(t.paramsToSelector(newParams, "o."),
		bson.DocElem{"ns", logsDB + "." + logsC},
	)

	oplog := t.params.Oplog
	if oplog == nil {
		oplog = mongo.GetOplog(t.session)
	}

	minOplogTs := t.lastTime.Add(-oplogOverlap)
	oplogTailer := mongo.NewOplogTailer(oplog, oplogSel, minOplogTs)
	defer oplogTailer.Stop()

	logger.Tracef("LogTailer starting oplog tailing: recent id count=%d, lastTime=%s, minOplogTs=%s",
		recentIds.Length(), t.lastTime, minOplogTs)

	skipCount := 0
	for {
		select {
		case <-t.tomb.Dying():
			return errors.Trace(tomb.ErrDying)
		case oplogDoc, ok := <-oplogTailer.Out():
			if !ok {
				return errors.Annotate(oplogTailer.Err(), "oplog tailer died")
			}

			doc := new(logDoc)
			err := oplogDoc.UnmarshalObject(doc)
			if err != nil {
				return errors.Annotate(err, "oplog unmarshalling failed")
			}

			if recentIds.Contains(doc.Id) {
				// This document has already been reported.
				skipCount++
				if skipCount%1000 == 0 {
					logger.Tracef("LogTailer duplicates skipped: %d", skipCount)
				}
				continue
			}

			select {
			case <-t.tomb.Dying():
				return errors.Trace(tomb.ErrDying)
			case t.logCh <- logDocToRecord(doc):
			}
		}
	}
}

func (t *logTailer) paramsToSelector(params *LogTailerParams, prefix string) bson.D {
	sel := bson.D{
		{"e", t.envUUID},
		{"t", bson.M{"$gte": params.StartTime}},
	}
	if params.MinLevel > loggo.UNSPECIFIED {
		sel = append(sel, bson.DocElem{"v", bson.M{"$gte": params.MinLevel}})
	}
	if len(params.IncludeEntity) > 0 {
		sel = append(sel,
			bson.DocElem{"n", bson.RegEx{Pattern: makeEntityPattern(params.IncludeEntity)}})
	}
	if len(params.ExcludeEntity) > 0 {
		sel = append(sel,
			bson.DocElem{"n", bson.M{"$not": bson.RegEx{Pattern: makeEntityPattern(params.ExcludeEntity)}}})
	}
	if len(params.IncludeModule) > 0 {
		sel = append(sel,
			bson.DocElem{"m", bson.RegEx{Pattern: makeModulePattern(params.IncludeModule)}})
	}
	if len(params.ExcludeModule) > 0 {
		sel = append(sel,
			bson.DocElem{"m", bson.M{"$not": bson.RegEx{Pattern: makeModulePattern(params.ExcludeModule)}}})
	}

	if prefix != "" {
		for i, elem := range sel {
			sel[i].Name = prefix + elem.Name
		}
	}
	return sel
}

func makeEntityPattern(entities []string) string {
	var patterns []string
	for _, entity := range entities {
		// Convert * wildcard to the regex equivalent. This is safe
		// because * never appears in entity names.
		patterns = append(patterns, strings.Replace(entity, "*", ".*", -1))
	}
	return `^(` + strings.Join(patterns, "|") + `)$`
}

func makeModulePattern(modules []string) string {
	var patterns []string
	for _, module := range modules {
		patterns = append(patterns, regexp.QuoteMeta(module))
	}
	return `^(` + strings.Join(patterns, "|") + `)(\..+)?$`
}

func newRecentIdTracker(maxLen int) *recentIdTracker {
	return &recentIdTracker{
		ids: deque.NewWithMaxLen(maxLen),
	}
}

type recentIdTracker struct {
	ids *deque.Deque
}

func (t *recentIdTracker) Add(id bson.ObjectId) {
	t.ids.PushBack(id)
}

func (t *recentIdTracker) AsSet() *objectIdSet {
	out := newObjectIdSet()
	for {
		id, ok := t.ids.PopFront()
		if !ok {
			break
		}
		out.Add(id.(bson.ObjectId))
	}
	return out
}

func newObjectIdSet() *objectIdSet {
	return &objectIdSet{
		ids: set.NewStrings(),
	}
}

type objectIdSet struct {
	ids set.Strings
}

func (s *objectIdSet) Add(id bson.ObjectId) {
	s.ids.Add(string(id))
}

func (s *objectIdSet) Contains(id bson.ObjectId) bool {
	return s.ids.Contains(string(id))
}

func (s *objectIdSet) Length() int {
	return len(s.ids)
}

func logDocToRecord(doc *logDoc) *LogRecord {
	return &LogRecord{
		Time:     doc.Time,
		Entity:   doc.Entity,
		Module:   doc.Module,
		Location: doc.Location,
		Level:    doc.Level,
		Message:  doc.Message,
	}
}

// PruneLogs removes old log documents in order to control the size of
// logs collection. All logs older than minLogTime are
// removed. Further removal is also performed if the logs collection
// size is greater than maxLogsMB.
func PruneLogs(st LoggingState, minLogTime time.Time, maxLogsMB int) error {
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
func initLogsSession(st LoggingState) (*mgo.Session, *mgo.Collection) {
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
	err := coll.Database.Run(bson.D{
		{"collStats", coll.Name},
		{"scale", humanize.MiByte},
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
