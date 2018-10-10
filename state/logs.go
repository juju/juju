// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Low-level functionality for interacting with the logs collection
// and tailing logs from the replication oplog.

package state

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/deque"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/mongo"
)

// TODO(wallyworld) - lp:1602508 - collections need to be defined in collections.go
const (
	logsDB      = "logs"
	logsCPrefix = "logs."
	forwardedC  = "forwarded"
)

// ErrNeverForwarded signals to the caller that the ID of a
// previously forwarded log record could not be found.
var ErrNeverForwarded = errors.Errorf("cannot find ID of the last forwarded record")

// MongoSessioner supports creating new mongo sessions.
type MongoSessioner interface {
	// MongoSession creates a new Mongo session.
	MongoSession() *mgo.Session
}

// ControllerSessioner supports creating new mongo sessions for the controller.
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

// logIndexes defines the indexes we need on the log collection.
var logIndexes = [][]string{
	// This index needs to include _id because
	// logTailer.processCollection uses _id to ensure log records with
	// the same time have a consistent ordering.
	{"t", "_id"},
	{"n"},
}

func logCollectionName(modelUUID string) string {
	return logsCPrefix + modelUUID
}

// InitDbLogs sets up the indexes for the logs collection. It should
// be called as state is opened. It is idempotent.
func InitDbLogs(session *mgo.Session, modelUUID string) error {
	logsColl := session.DB(logsDB).C(logCollectionName(modelUUID))
	for _, key := range logIndexes {
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
	session := st.MongoSession().Copy()
	return &LastSentLogTracker{
		id:      fmt.Sprintf("%s#%s", modelUUID, sink),
		model:   modelUUID,
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
// TODO: remove version from this structure: https://pad.lv/1643743
type logDoc struct {
	Id       bson.ObjectId `bson:"_id"`
	Time     int64         `bson:"t"` // unix nano UTC
	Entity   string        `bson:"n"` // e.g. "machine-0"
	Version  string        `bson:"r"`
	Module   string        `bson:"m"` // e.g. "juju.worker.firewaller"
	Location string        `bson:"l"` // "filename:lineno"
	Level    int           `bson:"v"`
	Message  string        `bson:"x"`
}

type DbLogger struct {
	logsColl  *mgo.Collection
	modelUUID string
}

func NewDbLogger(st ModelSessioner) *DbLogger {
	_, logsColl := initLogsSession(st)
	return &DbLogger{
		logsColl:  logsColl,
		modelUUID: st.ModelUUID(),
	}
}

// Log writes log messages to the database. Log records
// are written to the database in bulk; callers should
// buffer log records to and call Log with a batch to
// minimise database writes.
//
// The ModelUUID and ID fields of records are ignored;
// DbLogger is scoped to a single model, and ID is
// controlled by the DbLogger code.
func (logger *DbLogger) Log(records []LogRecord) error {
	for _, r := range records {
		if err := validateInputLogRecord(r); err != nil {
			return errors.Annotate(err, "validating input log record")
		}
	}
	bulk := logger.logsColl.Bulk()
	for _, r := range records {
		var versionString string
		if r.Version != version.Zero {
			versionString = r.Version.String()
		}
		bulk.Insert(&logDoc{
			// TODO(axw) Use a controller-global int
			// sequence for Id, so we can order by
			// insertion.
			Id:       bson.NewObjectId(),
			Time:     r.Time.UnixNano(),
			Entity:   r.Entity.String(),
			Version:  versionString,
			Module:   r.Module,
			Location: r.Location,
			Level:    int(r.Level),
			Message:  r.Message,
		})
	}
	_, err := bulk.Run()
	return errors.Annotatef(err, "inserting %d log record(s)", len(records))
}

func validateInputLogRecord(r LogRecord) error {
	if r.Entity == nil {
		return errors.NotValidf("missing Entity")
	}
	return nil
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

// maxInitialLines limits the number of documents we will load into memory
// so that we can iterate them in the correct order.
var maxInitialLines = 10000

// LogTailerState describes the methods on State required for logging to
// the database.
type LogTailerState interface {
	ModelSessioner

	// IsController indicates whether or not the model is the admin model.
	IsController() bool
}

// NewLogTailer returns a LogTailer which filters according to the
// parameters given.
func NewLogTailer(st LogTailerState, params LogTailerParams) (LogTailer, error) {
	session := st.MongoSession().Copy()
	t := &logTailer{
		modelUUID:       st.ModelUUID(),
		session:         session,
		logsColl:        session.DB(logsDB).C(logCollectionName(st.ModelUUID())).With(session),
		params:          params,
		logCh:           make(chan *LogRecord),
		recentIds:       newRecentIdTracker(maxRecentLogIds),
		maxInitialLines: maxInitialLines,
	}
	t.tomb.Go(func() error {
		defer close(t.logCh)
		defer session.Close()
		err := t.loop()
		return errors.Cause(err)
	})
	return t, nil
}

type logTailer struct {
	tomb            tomb.Tomb
	modelUUID       string
	session         *mgo.Session
	logsColl        *mgo.Collection
	params          LogTailerParams
	logCh           chan *LogRecord
	lastID          int64
	lastTime        time.Time
	recentIds       *recentIdTracker
	maxInitialLines int
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
	// NOTE: don't trace or annotate the errors returned
	// from this method as the error may be tomb.ErrDying, and
	// the tomb code is sensitive about equality.
	err := t.processCollection()
	if err != nil {
		return err
	}

	if t.params.NoTail {
		return nil
	}

	return t.tailOplog()
}

func (t *logTailer) processReversed(query *mgo.Query) error {
	// We must sort by exactly the fields in the index and exactly reversed
	// so that Mongo will use the index and not try to sort in memory.
	// Note (jam): 2017-04-19 if this is truly too much memory load we should
	// a) limit InitialLines to something reasonable
	// b) we *could* just load the object _ids and then do a forward query
	//    on exactly those ids (though it races with new items being inserted)
	// c) use the aggregation pipeline in Mongo 3.2 to write the docs to
	//    a temp location and iterate them forward from there.
	// (a) makes the most sense for me :)
	if t.params.InitialLines > t.maxInitialLines {
		return errors.Errorf("too many lines requested (%d) maximum is %d",
			t.params.InitialLines, maxInitialLines)
	}
	query.Sort("-t", "-_id")
	query.Limit(t.params.InitialLines)
	iter := query.Iter()
	defer iter.Close()
	queue := make([]logDoc, t.params.InitialLines)
	cur := t.params.InitialLines
	var doc logDoc
	for iter.Next(&doc) {
		select {
		case <-t.tomb.Dying():
			return errors.Trace(tomb.ErrDying)
		default:
		}
		cur--
		queue[cur] = doc
		if cur == 0 {
			break
		}
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	// We loaded the queue in reverse order, truncate it to just the actual
	// contents, and then return them in the correct order.
	queue = queue[cur:]
	for _, doc := range queue {
		rec, err := logDocToRecord(t.modelUUID, &doc)
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
	return nil
}

func (t *logTailer) processCollection() error {
	// Create a selector from the params.
	sel := t.paramsToSelector(t.params, "")
	query := t.logsColl.Find(sel)

	var doc logDoc
	if t.params.InitialLines > 0 {
		return t.processReversed(query)
	}
	// In tests, sorting by time can leave the result ordering
	// underconstrained. Since object ids are (timestamp, machine id,
	// process id, counter)
	// https://docs.mongodb.com/manual/reference/bson-types/#objectid
	// and the tests only run one mongod process, including _id
	// guarantees getting log messages in a predictable order.
	//
	// Important: it is critical that the sort index includes _id,
	// otherwise MongoDB won't use the index, which risks hitting
	// MongoDB's 32MB sort limit.  See https://pad.lv/1590605.
	//
	// If we get a deserialisation error, write out the first failure,
	// but don't write out any additional errors until we either hit
	// a good value, or end the method.
	deserialisationFailures := 0
	iter := query.Sort("t", "_id").Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		rec, err := logDocToRecord(t.modelUUID, &doc)
		if err != nil {
			if deserialisationFailures == 0 {
				logger.Warningf("log deserialization failed (possible DB corruption), %v", err)
			}
			// Add the id to the recentIds so we don't try to look at it again
			// during the oplog traversal.
			t.recentIds.Add(doc.Id)
			deserialisationFailures++
			continue
		} else {
			if deserialisationFailures > 1 {
				logger.Debugf("total of %d log serialisation errors", deserialisationFailures)
			}
			deserialisationFailures = 0
		}
		select {
		case <-t.tomb.Dying():
			return tomb.ErrDying
		case t.logCh <- rec:
			t.lastID = rec.ID
			t.lastTime = rec.Time
			t.recentIds.Add(doc.Id)
		}
	}
	if deserialisationFailures > 1 {
		logger.Debugf("total of %d log serialisation errors", deserialisationFailures)
	}

	return errors.Trace(iter.Close())
}

func (t *logTailer) tailOplog() error {
	recentIds := t.recentIds.AsSet()

	newParams := t.params
	newParams.StartID = t.lastID // (t.lastID + 1) once Id is a sequential int.
	oplogSel := append(t.paramsToSelector(newParams, "o."),
		bson.DocElem{"ns", logsDB + "." + logCollectionName(t.modelUUID)},
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

	// If we get a deserialisation error, write out the first failure,
	// but don't write out any additional errors until we either hit
	// a good value, or end the method.
	deserialisationFailures := 0
	skipCount := 0
	for {
		select {
		case <-t.tomb.Dying():
			return tomb.ErrDying
		case oplogDoc, ok := <-oplogTailer.Out():
			if !ok {
				return errors.Annotate(oplogTailer.Err(), "oplog tailer died")
			}
			if oplogDoc.Operation != "i" {
				// We only care about inserts.
				continue
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
			rec, err := logDocToRecord(t.modelUUID, doc)
			if err != nil {
				if deserialisationFailures == 0 {
					logger.Warningf("log deserialization failed (possible DB corruption), %v", err)
				}
				deserialisationFailures++
				continue
			} else {
				if deserialisationFailures > 1 {
					logger.Debugf("total of %d log serialisation errors", deserialisationFailures)
				}
				deserialisationFailures = 0
			}
			select {
			case <-t.tomb.Dying():
				return tomb.ErrDying
			case t.logCh <- rec:
			}
		}
	}
}

func (t *logTailer) paramsToSelector(params LogTailerParams, prefix string) bson.D {
	sel := bson.D{}
	if !params.StartTime.IsZero() {
		sel = append(sel, bson.DocElem{"t", bson.M{"$gte": params.StartTime.UnixNano()}})
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

func logDocToRecord(modelUUID string, doc *logDoc) (*LogRecord, error) {
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

		ModelUUID: modelUUID,
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
func PruneLogs(st ControllerSessioner, minLogTime time.Time, maxLogsMB int) error {
	if !st.IsController() {
		return errors.Errorf("pruning logs requires a controller state")
	}
	session, logsDB := initLogsSessionDB(st)
	defer session.Close()

	logColls, err := getLogCollections(logsDB)
	if err != nil {
		return errors.Annotate(err, "failed to get log counts")
	}

	pruneCounts := make(map[string]int)

	// Remove old log entries for each model.
	for modelUUID, logColl := range logColls {
		removeInfo, err := logColl.RemoveAll(bson.M{
			"t": bson.M{"$lt": minLogTime.UnixNano()},
		})
		if err != nil {
			return errors.Annotate(err, "failed to prune logs by time")
		}
		pruneCounts[modelUUID] = removeInfo.Removed
	}

	// Do further pruning if the total size of the log collections is
	// over the maximum size.
	for {
		collMB, err := getCollectionTotalMB(logColls)
		if err != nil {
			return errors.Annotate(err, "failed to retrieve log counts")
		}
		if collMB <= maxLogsMB {
			break
		}

		modelUUID, count, err := findModelWithMostLogs(logColls)
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
		logColl := logColls[modelUUID]
		tsQuery := logColl.Find(nil).Sort("t", "_id")
		tsQuery = tsQuery.Skip(toRemove)
		tsQuery = tsQuery.Select(bson.M{"t": 1})
		var doc bson.M
		err = tsQuery.One(&doc)
		if err != nil {
			return errors.Annotate(err, "log pruning timestamp query failed")
		}
		thresholdTs := doc["t"]

		// Remove old records.
		removeInfo, err := logColl.RemoveAll(bson.M{
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

func initLogsSessionDB(st MongoSessioner) (*mgo.Session, *mgo.Database) {
	// To improve throughput, only wait for the logs to be written to
	// the primary. For some reason, this makes a huge difference even
	// when the replicaset only has one member (i.e. a single primary).
	session := st.MongoSession().Copy()
	session.SetSafe(&mgo.Safe{
		W: 1,
	})
	return session, session.DB(logsDB)
}

// initLogsSession creates a new session suitable for logging updates,
// returning the session and a logs mgo.Collection connected to that
// session.
func initLogsSession(st ModelSessioner) (*mgo.Session, *mgo.Collection) {
	session, db := initLogsSessionDB(st)
	return session, db.C(logCollectionName(st.ModelUUID()))
}

// dbCollectionSizeToInt processes the result of Database.collStats()
func dbCollectionSizeToInt(result bson.M, collectionName string) (int, error) {
	size, ok := result["size"]
	if !ok {
		logger.Warningf("mongo collStats did not return a size field for %q", collectionName)
		// this wasn't considered an error in the past, just treat it as size 0
		return 0, nil
	}
	if asint, ok := size.(int); ok {
		if asint < 0 {
			return 0, errors.Errorf("mongo collStats for %q returned a negative value: %v", collectionName, size)
		}
		return asint, nil
	}
	if asint64, ok := size.(int64); ok {
		// 2billion megabytes is 2 petabytes, which is outside our range anyway.
		if asint64 > math.MaxInt32 {
			return math.MaxInt32, nil
		}
		if asint64 < 0 {
			return 0, errors.Errorf("mongo collStats for %q returned a negative value: %v", collectionName, size)
		}
		return int(asint64), nil
	}
	return 0, errors.Errorf(
		"mongo collStats for %q did not return an int or int64 for size, returned %T: %v",
		collectionName, size, size)
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
	return dbCollectionSizeToInt(result, coll.Name)
}

// getCollectionTotalMB returns the total size of the log collections
// passed.
func getCollectionTotalMB(colls map[string]*mgo.Collection) (int, error) {
	total := 0
	for _, coll := range colls {
		size, err := getCollectionMB(coll)
		if err != nil {
			return 0, errors.Trace(err)
		}
		total += size
	}
	return total, nil
}

// getLogCollections returns all of the log collections in the DB by
// model UUID.
func getLogCollections(db *mgo.Database) (map[string]*mgo.Collection, error) {
	result := make(map[string]*mgo.Collection)
	names, err := db.CollectionNames()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, name := range names {
		if !strings.HasPrefix(name, logsCPrefix) {
			continue
		}
		uuid := name[len(logsCPrefix):]
		result[uuid] = db.C(name)
	}
	return result, nil
}

// findModelWithMostLogs returns the modelUUID and row count for the
// collection with the most logs in the logs DB.
func findModelWithMostLogs(colls map[string]*mgo.Collection) (string, int, error) {
	var maxModelUUID string
	var maxCount int
	for modelUUID, coll := range colls {
		count, err := getRowCountForCollection(coll)
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

// getRowCountForCollection returns the number of log records stored for a
// given model log collection.
func getRowCountForCollection(coll *mgo.Collection) (int, error) {
	count, err := coll.Count()
	if err != nil {
		return -1, errors.Annotate(err, "failed to get log count")
	}
	return count, nil
}

func removeModelLogs(session *mgo.Session, modelUUID string) error {
	logsDB := session.DB(logsDB)
	logsColl := logsDB.C(logCollectionName(modelUUID))
	if err := logsColl.DropCollection(); err != nil {
		return errors.Trace(err)
	}

	// Also remove the tracked high-water times.
	trackersColl := logsDB.C(forwardedC)
	_, err := trackersColl.RemoveAll(bson.M{"model-uuid": modelUUID})
	return errors.Trace(err)
}
