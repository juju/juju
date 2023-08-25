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
	"github.com/juju/clock"
	"github.com/juju/collections/deque"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/controller"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/mongo"
)

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
	// clock returns the clock used by the state instance.
	clock() clock.Clock
}

// ModelSessioner supports creating new mongo sessions for a model.
type ModelSessioner interface {
	MongoSessioner

	// ModelUUID returns the ID of the current model.
	ModelUUID() string
}

// LogIndex represents a way to configure a series of log indexes.
type LogIndex struct {
	Key    []string
	Sparse bool
}

// logIndexes defines the indexes we need on the log collection.
var logIndexes = []LogIndex{
	// This index needs to include _id because
	// logTailer.processCollection uses _id to ensure log records with
	// the same time have a consistent ordering.
	{Key: []string{"t", "_id"}},
	{Key: []string{"n"}},
	// The label field is optional, so we need a sparse index.
	// See: https://docs.mongodb.com/manual/core/index-sparse/
	{Key: []string{"c"}, Sparse: true},
}

func logCollectionName(modelUUID string) string {
	return logsCPrefix + modelUUID
}

// InitDbLogs sets up the capped collections for the logging, along with the
// indexes for the logs collection. It should be called as state is opened. It
// is idempotent.
func InitDbLogs(session *mgo.Session) error {
	// Read the capped collection size from controller config.
	size, err := modelLogsSize(session)
	if errors.Cause(err) == mgo.ErrNotFound {
		// We are in early stages of database initialization, so nothing to do
		// here. The controller model and the default model (most likely) will
		// be initialized during bootstrap, and when the machine agent starts
		// properly, the logs collections will be either created for them,
		// or converted to capped collections.
		logger.Infof("controller settings not found, early stage initialization assumed")
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}

	models, err := modelUUIDs(session)
	if err != nil {
		return err
	}

	encounteredError := false
	for _, uuid := range models {
		if err := InitDbLogsForModel(session, uuid, size); err != nil {
			encounteredError = true
			logger.Errorf("unable to initialize model logs: %v", err)
		}
	}

	if encounteredError {
		return errors.New("one or more errors initializing logs")
	}
	return nil
}

// modelLogsSize reads the model-logs-size value from the controller
// config document and returns it. If the value isn't found the default
// size value is returned.
func modelLogsSize(session *mgo.Session) (int, error) {
	// This is executed very early in the opening of the database, so there
	// is no State, Controller, nor StatePool objects just now. Use low level
	// mgo to access the settings.
	var doc settingsDoc
	err := session.DB(jujuDB).C(controllersC).FindId(ControllerSettingsGlobalKey).One(&doc)
	if err != nil {
		return 0, errors.Trace(err)
	}
	// During initial migration there is no guarantee that the value exists
	// in the settings document.
	if value, ok := doc.Settings[controller.ModelLogsSize]; ok {
		if s, ok := value.(string); ok {
			size, _ := utils.ParseSize(s)
			if size > 0 {
				// If the value is there we know it fits in an int without wrapping.
				return int(size), nil
			}
		}
	}
	return controller.DefaultModelLogsSizeMB, nil
}

// modelUUIDs returns the UUIDs of all models currently stored in the database.
// This function is called very early in the opening of the database, so it uses
// lower level mgo methods rather than any helpers from State objects.
func modelUUIDs(session *mgo.Session) ([]string, error) {
	var docs []modelDoc
	err := session.DB(jujuDB).C(modelsC).Find(nil).All(&docs)
	if err != nil {
		return nil, err
	}
	var result []string
	for _, doc := range docs {
		result = append(result, doc.UUID)
	}
	return result, nil
}

// InitDbLogsForModel sets up the indexes for the logs collection for the
// specified model. It should be called as state is opened. It is idempotent.
// This function also ensures that the logs collection is capped at the right
// size.
func InitDbLogsForModel(session *mgo.Session, modelUUID string, size int) error {
	// Get the collection from the logs DB.
	logsColl := session.DB(logsDB).C(logCollectionName(modelUUID))

	// First try to create it.
	logger.Infof("ensuring logs collection for %s, capped at %v MiB", modelUUID, size)
	err := createCollection(logsColl, &mgo.CollectionInfo{
		Capped:   true,
		MaxBytes: size * humanize.MiByte,
	})
	if err != nil {
		return errors.Trace(err)
	}

	capped, maxSize, err := getCollectionCappedInfo(logsColl)
	if err != nil {
		return errors.Trace(err)
	}

	if capped {
		if maxSize == size {
			// The logs collection size matches, so nothing to do here.
			logger.Tracef("logs collection for %s already capped at %v MiB", modelUUID, size)
		} else {
			logger.Infof("resizing logs collection for %s from %d to %v MiB", modelUUID, maxSize, size)
			err := convertToCapped(logsColl, size)
			if err != nil {
				return errors.Trace(err)
			}
		}
	} else {
		logger.Infof("converting logs collection for %s to capped with max size %v MiB", modelUUID, size)
		err := convertToCapped(logsColl, size)
		if err != nil {
			return errors.Trace(err)
		}
	}

	// Ensure all the right indices are created. When converting to a capped
	// collection, the indices are dropped.
	for _, index := range logIndexes {
		err := logsColl.EnsureIndex(mgo.Index{Key: index.Key, Sparse: index.Sparse})
		if err != nil {
			return errors.Annotatef(err, "cannot create index for logs collection %v", logsColl.Name)
		}
	}

	return nil
}

func mgoAlreadyExistsErr(err error) bool {
	err = errors.Cause(err)
	queryError, ok := err.(*mgo.QueryError)
	if !ok {
		return false
	}
	// Mongo doesn't provide a list of all error codes in their documentation,
	// but we can review their source code. Weirdly already exists error comes
	// up as namespace exists.
	//
	// See the following links:
	//  - Error codes document:  https://github.com/mongodb/mongo/blob/2eefd197e50c5d90b3ec0e0ad9ac15a8b14e3331/src/mongo/base/error_codes.yml#L84
	//  - Mongo returning the NS error: https://github.com/mongodb/mongo/blob/8c9fa5aa62c28280f35494b091f1ae5b810d349b/src/mongo/db/catalog/create_collection.cpp#L245-L246
	return queryError.Code == 48
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
	Labels   []string      `bson:"c,omitempty"` // e.g. http
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
func (logger *DbLogger) Log(records []corelogger.LogRecord) error {
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
			Entity:   r.Entity,
			Version:  versionString,
			Module:   r.Module,
			Location: r.Location,
			Level:    int(r.Level),
			Message:  r.Message,
			Labels:   r.Labels,
		})
	}
	_, err := bulk.Run()
	return errors.Annotatef(err, "inserting %d log record(s)", len(records))
}

func validateInputLogRecord(r corelogger.LogRecord) error {
	if r.Entity == "" {
		return errors.NotValidf("missing Entity")
	}
	return nil
}

// Close cleans up resources used by the DbLogger instance.
func (logger *DbLogger) Close() error {
	if logger.logsColl != nil {
		logger.logsColl.Database.Session.Close()
	}
	return nil
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
func NewLogTailer(
	st LogTailerState, params corelogger.LogTailerParams, opLog *mgo.Collection,
) (corelogger.LogTailer, error) {
	session := st.MongoSession().Copy()

	if opLog == nil {
		opLog = mongo.GetOplog(session)
	}

	t := &logTailer{
		modelUUID:       st.ModelUUID(),
		session:         session,
		logsColl:        session.DB(logsDB).C(logCollectionName(st.ModelUUID())).With(session),
		opLog:           opLog,
		params:          params,
		logCh:           make(chan *corelogger.LogRecord),
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
	opLog           *mgo.Collection
	params          corelogger.LogTailerParams
	logCh           chan *corelogger.LogRecord
	lastID          int64
	lastTime        time.Time
	recentIds       *recentIdTracker
	maxInitialLines int
}

// Logs implements the LogTailer interface.
func (t *logTailer) Logs() <-chan *corelogger.LogRecord {
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
	iter := query.Sort("-t", "-_id").
		Limit(t.params.InitialLines).
		Iter()
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

	minOplogTs := t.lastTime.Add(-oplogOverlap)
	oplogTailer := mongo.NewOplogTailer(mongo.NewOplogSession(t.opLog, oplogSel), minOplogTs)
	defer func() { _ = oplogTailer.Stop() }()

	logger.Infof("LogTailer starting oplog tailing: recent id count=%d, lastTime=%s, minOplogTs=%s",
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

func (t *logTailer) paramsToSelector(params corelogger.LogTailerParams, prefix string) bson.D {
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
	if len(params.IncludeLabel) > 0 {
		sel = append(sel,
			bson.DocElem{"c", bson.M{"$in": params.IncludeLabel}})
	}
	if len(params.ExcludeLabel) > 0 {
		sel = append(sel,
			bson.DocElem{"c", bson.M{"$nin": params.ExcludeLabel}})
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
		// because * never appears in entity names. Escape any other regex.
		escaped := regexp.QuoteMeta(entity)
		unescapedWildcards := strings.Replace(escaped, regexp.QuoteMeta("*"), ".*", -1)
		patterns = append(patterns, unescapedWildcards)
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

func logDocToRecord(modelUUID string, doc *logDoc) (*corelogger.LogRecord, error) {
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

	rec := &corelogger.LogRecord{
		ID:   doc.Time,
		Time: time.Unix(0, doc.Time).UTC(), // not worth preserving TZ

		ModelUUID: modelUUID,
		Entity:    doc.Entity,
		Version:   ver,

		Level:    level,
		Module:   doc.Module,
		Location: doc.Location,
		Message:  doc.Message,
		Labels:   doc.Labels,
	}
	return rec, nil
}

// DebugLogger is a logger that implements Debugf.
type DebugLogger interface {
	Debugf(string, ...interface{})
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

func collStats(coll *mgo.Collection) (bson.M, error) {
	var result bson.M
	err := coll.Database.Run(bson.D{
		{"collStats", coll.Name},
		{"scale", humanize.KiByte},
	}, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// For mongo > 4.4, if the collection exists,
	// there will be a "capped" attribute.
	_, ok := result["capped"]
	if !ok {
		return nil, errors.NotFoundf("Collection [%s.%s]", coll.Database.Name, coll.Name)
	}
	return result, nil
}

func convertToCapped(coll *mgo.Collection, maxSizeMB int) error {
	if maxSizeMB < 1 {
		return errors.NotValidf("non-positive maxSize %v", maxSizeMB)
	}
	maxSizeMB *= humanize.MiByte
	var result bson.M
	err := coll.Database.Run(bson.D{
		{"convertToCapped", coll.Name},
		{"size", maxSizeMB},
	}, &result)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// getCollectionCappedInfo returns whether or not the collection is
// capped, and the max size in MB.
func getCollectionCappedInfo(coll *mgo.Collection) (bool, int, error) {
	stats, err := collStats(coll)
	if err != nil {
		return false, 0, errors.Trace(err)
	}
	capped, _ := stats["capped"].(bool)
	if !capped {
		return false, 0, nil
	}
	value, found := stats["maxSize"]
	if !found {
		return false, 0, errors.NotValidf("no maxSize value")
	}
	maxSize, ok := value.(int)
	if !ok {
		return false, 0, errors.NotValidf("size value is not an int")
	}
	return true, maxSize / humanize.KiByte, nil
}

// getCollectionKB returns the size of a MongoDB collection (in
// kilobytes), excluding space used by indexes.
func getCollectionKB(coll *mgo.Collection) (int, error) {
	stats, err := collStats(coll)
	if err != nil {
		return 0, errors.Trace(err)
	}
	return dbCollectionSizeToInt(stats, coll.Name)
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
