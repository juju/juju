// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Low-level functionality for interacting with the logs collection
// and tailing logs from the replication oplog.

package state

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/deque"
	"github.com/juju/utils/set"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/tomb.v1"

	"github.com/juju/juju/mongo"
)

// TODO(wallyworld) - lp:1602508 - collections need to be defined in collections.go
const (
	logsDB     = "logs"
	logsC      = "logs"
	forwardedC = "forwarded"
)

// ErrNeverForwarded signals to the caller that the ID of a
// previously forwarded log record could not be found.
var ErrNeverForwarded = errors.Errorf("cannot find ID of the last forwarded record")

// MongoSessioner supports creating new mongo sessions.
type MongoSessioner interface {
	// MongoSession creates a new Mongo session.
	MongoSession() *mgo.Session
}

// ModelSessioner supports creating new mongo sessions for the controller.
type ControllerSessioner interface {
	MongoSessioner

	// IsController indicates if current state is controller.
	IsController() bool
}

// ModelSessioner supports creating new mongo sessions for a model.
type ModelSessioner interface {
	MongoSessioner

	// ModelUUID returns the ID of the current model.
	ModelUUID() string
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

// lastSentDoc captures timestamp of the last log record forwarded
// to a log sink.
type lastSentDoc struct {
	// ID is the unique ID mongo will use for the doc.
	ID string `bson:"_id"`

	// ModelUUID identifies the model for which the identified record
	// was last sent.
	ModelUUID string `bson:"model-uuid"`

	// Sink identifies the log forwarding target to which the identified
	// log record was sent.
	Sink string `bson:"sink"`

	// TODO(ericsnow) Solve the problems with using the timestamp
	// as the record ID.

	// RecordTimestamp identifies the last record sent to the log sink
	// for the model. It is used to look up log records in the DB.
	//
	// Currently we use the record's timestamp (unix nano UTC), which
	// has a risk of collisions. Log record timestamps have nanosecond
	// precision. The likelihood of multiple log records having the
	// same timestamp is small, though it increases with the size and
	// activity of the model.
	//
	// Using the timestamp also has the problem that a faulty clock may
	// introduce records with a timestamp earlier that the "last sent"
	// value. Such records would never get forwarded.
	//
	// The solution to both these issues will likely involve using an
	// int sequence for the ID rather than the timestamp. That sequence
	// would be shared by all models.
	RecordTimestamp int64 `bson:"record-timestamp"`

	// RecordID is the ID of the last record sent to the log sink.
	// We record it but currently just use the timestamp when querying
	// the log collection.
	RecordID int64 `bson:"record-id"`
}

// LastSentLogTracker records and retrieves timestamps of the most recent
// log records forwarded to a log sink for a model.
type LastSentLogTracker struct {
	session *mgo.Session
	id      string
	model   string
	sink    string
}

// NewLastSentLogTracker returns a new tracker that records and retrieves
// the timestamps of the most recent log records forwarded to the
// identified log sink for the current model.
func NewLastSentLogTracker(st ModelSessioner, modelUUID, sink string) *LastSentLogTracker {
	return newLastSentLogTracker(st, modelUUID, sink)
}

// NewAllLastSentLogTracker returns a new tracker that records and retrieves
// the timestamps of the most recent log records forwarded to the
// identified log sink for *all* models.
func NewAllLastSentLogTracker(st ControllerSessioner, sink string) (*LastSentLogTracker, error) {
	if !st.IsController() {
		return nil, errors.New("only the admin model can track all log records")
	}
	return newLastSentLogTracker(st, "", sink), nil
}

func newLastSentLogTracker(st MongoSessioner, model, sink string) *LastSentLogTracker {
	session := st.MongoSession().Copy()
	return &LastSentLogTracker{
		id:      fmt.Sprintf("%s#%s", model, sink),
		model:   model,
		sink:    sink,
		session: session,
	}
}

// Close implements io.Closer
func (logger *LastSentLogTracker) Close() error {
	logger.session.Close()
	return nil
}

// Set records the timestamp.
func (logger *LastSentLogTracker) Set(recID, recTimestamp int64) error {
	collection := logger.session.DB(logsDB).C(forwardedC)
	_, err := collection.UpsertId(
		logger.id,
		lastSentDoc{
			ID:              logger.id,
			ModelUUID:       logger.model,
			Sink:            logger.sink,
			RecordID:        recID,
			RecordTimestamp: recTimestamp,
		},
	)
	return errors.Trace(err)
}

// Get retrieves the id and timestamp.
func (logger *LastSentLogTracker) Get() (int64, int64, error) {
	collection := logger.session.DB(logsDB).C(forwardedC)
	var doc lastSentDoc
	err := collection.FindId(logger.id).One(&doc)
	if err != nil {
		if err == mgo.ErrNotFound {
			return 0, 0, errors.Trace(ErrNeverForwarded)
		}
		return 0, 0, errors.Trace(err)
	}
	return doc.RecordID, doc.RecordTimestamp, nil
}

// logDoc describes log messages stored in MongoDB.
//
// Single character field names are used for serialisation to save
// space. These documents will be inserted 1000's of times and each
// document includes the field names.
// (alesstimec) It would be really nice if we could store Time as int64
// for increased precision.
type logDoc struct {
	Id        bson.ObjectId `bson:"_id"`
	Time      int64         `bson:"t"` // unix nano UTC
	ModelUUID string        `bson:"e"`
	Entity    string        `bson:"n"` // e.g. "machine-0"
	Version   string        `bson:"r"`
	Module    string        `bson:"m"` // e.g. "juju.worker.firewaller"
	Location  string        `bson:"l"` // "filename:lineno"
	Level     int           `bson:"v"`
	Message   string        `bson:"x"`
}

type DbLogger struct {
	logsColl  *mgo.Collection
	modelUUID string
	entity    string
	version   string
}

// NewDbLogger returns a DbLogger instance which is used to write logs
// to the database.
func NewDbLogger(st ModelSessioner, entity names.Tag, ver version.Number) *DbLogger {
	_, logsColl := initLogsSession(st)
	return &DbLogger{
		logsColl:  logsColl,
		modelUUID: st.ModelUUID(),
		entity:    entity.String(),
		version:   ver.String(),
	}
}

// Log writes a log message to the database.
func (logger *DbLogger) Log(t time.Time, module string, location string, level loggo.Level, msg string) error {
	// TODO(ericsnow) Use a controller-global int sequence for Id.

	// UnixNano() returns the "absolute" (UTC) number of nanoseconds
	// since the Unix "epoch".
	unixEpochNanoUTC := t.UnixNano()
	return logger.logsColl.Insert(&logDoc{
		Id:        bson.NewObjectId(),
		Time:      unixEpochNanoUTC,
		ModelUUID: logger.modelUUID,
		Entity:    logger.entity,
		Version:   logger.version,
		Module:    module,
		Location:  location,
		Level:     int(level),
		Message:   msg,
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
	// universal fields
	ID   int64
	Time time.Time

	// origin fields
	ModelUUID string
	Entity    names.Tag
	Version   version.Number

	// logging-specific fields
	Level    loggo.Level
	Module   string
	Location string
	Message  string
}

// LogTailerParams specifies the filtering a LogTailer should apply to
// logs in order to decide which to return.
type LogTailerParams struct {
	StartID       int64
	StartTime     time.Time
	MinLevel      loggo.Level
	InitialLines  int
	NoTail        bool
	IncludeEntity []string
	ExcludeEntity []string
	IncludeModule []string
	ExcludeModule []string
	Oplog         *mgo.Collection // For testing only
	AllModels     bool
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
// output of large broken models with logging at DEBUG.
var maxRecentLogIds = int(oplogOverlap.Minutes() * 150000)

// LogTailerState describes the methods on State required for logging to
// the database.
type LogTailerState interface {
	ModelSessioner

	// IsController indicates whether or not the model is the admin model.
	IsController() bool
}

// NewLogTailer returns a LogTailer which filters according to the
// parameters given.
func NewLogTailer(st LogTailerState, params *LogTailerParams) (LogTailer, error) {
	if !st.IsController() && params.AllModels {
		return nil, errors.NewNotValid(nil, "not allowed to tail logs from all models: not a controller")
	}

	session := st.MongoSession().Copy()
	t := &logTailer{
		modelUUID: st.ModelUUID(),
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
	return t, nil
}

type logTailer struct {
	tomb      tomb.Tomb
	modelUUID string
	session   *mgo.Session
	logsColl  *mgo.Collection
	params    *LogTailerParams
	logCh     chan *LogRecord
	lastID    int64
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

	if t.params.NoTail {
		return nil
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

	// In tests, sorting by time can leave the result ordering
	// underconstrained. Since object ids are (timestamp, machine id,
	// process id, counter)
	// https://docs.mongodb.com/manual/reference/bson-types/#objectid
	// and the tests only run one mongod process, including _id
	// guarantees getting log messages in a predictable order.
	//
	// Important: it is critical that the sort on _id is done
	// separately from the sort on {model, time}. Combining the sort
	// fields means that MongoDB won't use the indexes that are in
	// place, which risks hitting MongoDB's 32MB sort limit.  See
	// https://pad.lv/1590605.
	//
	// TODO(ericsnow) Sort only by _id once it is a sequential int.
	iter := query.Sort("e", "t").Sort("_id").Iter()
	doc := new(logDoc)
	for iter.Next(doc) {
		rec, err := logDocToRecord(doc)
		if err != nil {
			return errors.Annotate(err, "deserialization failed (possible DB corruption)")
		}
		select {
		case <-t.tomb.Dying():
			return errors.Trace(tomb.ErrDying)
		case t.logCh <- rec:
			t.lastID = rec.ID
			t.lastTime = rec.Time
			t.recentIds.Add(doc.Id)
		}
	}
	return errors.Trace(iter.Close())
}

func (t *logTailer) tailOplog() error {
	recentIds := t.recentIds.AsSet()

	newParams := t.params
	newParams.StartID = t.lastID // (t.lastID + 1) once Id is a sequential int.
	oplogSel := append(t.paramsToSelector(newParams, "o."),
		bson.DocElem{"ns", logsDB + "." + logsC},
	)

	oplog := t.params.Oplog
	if oplog == nil {
		oplog = mongo.GetOplog(t.session)
	}

	minOplogTs := t.lastTime.Add(-oplogOverlap)
	oplogTailer := mongo.NewOplogTailer(mongo.NewOplogSession(oplog, oplogSel), minOplogTs)
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
			rec, err := logDocToRecord(doc)
			if err != nil {
				return errors.Annotate(err, "deserialization failed (possible DB corruption)")
			}
			select {
			case <-t.tomb.Dying():
				return errors.Trace(tomb.ErrDying)
			case t.logCh <- rec:
			}
		}
	}
}

func (t *logTailer) paramsToSelector(params *LogTailerParams, prefix string) bson.D {
	sel := bson.D{}
	if !params.StartTime.IsZero() {
		sel = append(sel, bson.DocElem{"t", bson.M{"$gte": params.StartTime.UnixNano()}})
	}
	if !params.AllModels {
		sel = append(sel, bson.DocElem{"e", t.modelUUID})
	}
	if params.MinLevel > loggo.UNSPECIFIED {
		sel = append(sel, bson.DocElem{"v", bson.M{"$gte": int(params.MinLevel)}})
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

func logDocToRecord(doc *logDoc) (*LogRecord, error) {
	var ver version.Number
	if doc.Version != "" {
		parsed, err := version.Parse(doc.Version)
		if err != nil {
			return nil, errors.Annotatef(err, "invalid version %q", doc.Version)
		}
		ver = parsed
	}

	level := loggo.Level(doc.Level)
	if level > loggo.CRITICAL {
		return nil, errors.Errorf("unrecognized log level %q", doc.Level)
	}

	entity, err := names.ParseTag(doc.Entity)
	if err != nil {
		return nil, errors.Annotate(err, "while parsing entity tag")
	}

	rec := &LogRecord{
		ID:   doc.Time,
		Time: time.Unix(0, doc.Time).UTC(), // not worth preserving TZ

		ModelUUID: doc.ModelUUID,
		Entity:    entity,
		Version:   ver,

		Level:    level,
		Module:   doc.Module,
		Location: doc.Location,
		Message:  doc.Message,
	}
	return rec, nil
}

// PruneLogs removes old log documents in order to control the size of
// logs collection. All logs older than minLogTime are
// removed. Further removal is also performed if the logs collection
// size is greater than maxLogsMB.
func PruneLogs(st MongoSessioner, minLogTime time.Time, maxLogsMB int) error {
	session, logsColl := initLogsSession(st)
	defer session.Close()

	modelUUIDs, err := getEnvsInLogs(logsColl)
	if err != nil {
		return errors.Annotate(err, "failed to get log counts")
	}

	pruneCounts := make(map[string]int)

	// Remove old log entries (per model UUID to take advantage
	// of indexes on the logs collection).
	for _, modelUUID := range modelUUIDs {
		removeInfo, err := logsColl.RemoveAll(bson.M{
			"e": modelUUID,
			"t": bson.M{"$lt": minLogTime.UnixNano()},
		})
		if err != nil {
			return errors.Annotate(err, "failed to prune logs by time")
		}
		pruneCounts[modelUUID] = removeInfo.Removed
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

		modelUUID, count, err := findEnvWithMostLogs(logsColl, modelUUIDs)
		if err != nil {
			return errors.Annotate(err, "log count query failed")
		}
		if count < 5000 {
			break // Pruning is not worthwhile
		}

		// Remove the oldest 1% of log records for the model.
		toRemove := int(float64(count) * 0.01)

		// Find the threshold timestammp to start removing from.
		// NOTE: this assumes that there are no more logs being added
		// for the time range being pruned (which should be true for
		// any realistic minimum log collection size).
		tsQuery := logsColl.Find(bson.M{"e": modelUUID}).Sort("e", "t")
		tsQuery = tsQuery.Skip(toRemove)
		tsQuery = tsQuery.Select(bson.M{"t": 1})
		var doc bson.M
		err = tsQuery.One(&doc)
		if err != nil {
			return errors.Annotate(err, "log pruning timestamp query failed")
		}
		thresholdTs := doc["t"]

		// Remove old records.
		removeInfo, err := logsColl.RemoveAll(bson.M{
			"e": modelUUID,
			"t": bson.M{"$lt": thresholdTs},
		})
		if err != nil {
			return errors.Annotate(err, "log pruning failed")
		}
		pruneCounts[modelUUID] += removeInfo.Removed
	}

	for modelUUID, count := range pruneCounts {
		if count > 0 {
			logger.Debugf("pruned %d logs for model %s", count, modelUUID)
		}
	}
	return nil
}

// initLogsSession creates a new session suitable for logging updates,
// returning the session and a logs mgo.Collection connected to that
// session.
func initLogsSession(st MongoSessioner) (*mgo.Session, *mgo.Collection) {
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

// getEnvsInLogs returns the unique model UUIDs that exist in
// the logs collection. This uses the one of the indexes on the
// collection and should be fast.
func getEnvsInLogs(coll *mgo.Collection) ([]string, error) {
	var modelUUIDs []string
	err := coll.Find(nil).Distinct("e", &modelUUIDs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelUUIDs, nil
}

// findEnvWithMostLogs returns the modelUUID and log count for the
// model with the most logs in the logs collection.
func findEnvWithMostLogs(logsColl *mgo.Collection, modelUUIDs []string) (string, int, error) {
	var maxModelUUID string
	var maxCount int
	for _, modelUUID := range modelUUIDs {
		count, err := getLogCountForEnv(logsColl, modelUUID)
		if err != nil {
			return "", -1, errors.Trace(err)
		}
		if count > maxCount {
			maxModelUUID = modelUUID
			maxCount = count
		}
	}
	return maxModelUUID, maxCount, nil
}

// getLogCountForEnv returns the number of log records stored for a
// given model.
func getLogCountForEnv(coll *mgo.Collection, modelUUID string) (int, error) {
	count, err := coll.Find(bson.M{"e": modelUUID}).Count()
	if err != nil {
		return -1, errors.Annotate(err, "failed to get log count")
	}
	return count, nil
}
