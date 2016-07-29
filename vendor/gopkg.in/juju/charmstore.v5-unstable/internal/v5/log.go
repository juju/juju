// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v5 // import "gopkg.in/juju/charmstore.v5-unstable/internal/v5"

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/mgo.v2/bson"

	"gopkg.in/juju/charmstore.v5-unstable/internal/mongodoc"
)

// GET /log
// https://github.com/juju/charmstore/blob/v4/docs/API.md#get-log
//
// POST /log
// https://github.com/juju/charmstore/blob/v4/docs/API.md#post-log
func (h *ReqHandler) serveLog(w http.ResponseWriter, req *http.Request) error {
	if err := h.authenticateAdmin(req); err != nil {
		return errgo.Mask(err, errgo.Any)
	}
	switch req.Method {
	case "GET":
		return h.getLogs(w, req)
	case "POST":
		return h.postLogs(w, req)
	}
	return errgo.WithCausef(nil, params.ErrMethodNotAllowed, "%s method not allowed", req.Method)
}

func (h *ReqHandler) getLogs(w http.ResponseWriter, req *http.Request) error {
	w.Header().Set("content-type", "application/json")
	encoder := json.NewEncoder(w)

	// Retrieve values from the query string.
	limit, err := intValue(req.Form.Get("limit"), 1, 1000)
	if err != nil {
		return badRequestf(err, "invalid limit value")
	}
	offset, err := intValue(req.Form.Get("skip"), 0, 0)
	if err != nil {
		return badRequestf(err, "invalid skip value")
	}
	id := req.Form.Get("id")
	strLevel := req.Form.Get("level")
	strType := req.Form.Get("type")

	// Build the Mongo query.
	query := make(bson.D, 0, 3)
	if id != "" {
		url, err := charm.ParseURL(id)
		if err != nil {
			return badRequestf(err, "invalid id value")
		}
		query = append(query, bson.DocElem{"urls", url})
	}
	if strLevel != "" {
		logLevel, ok := paramsLogLevels[params.LogLevel(strLevel)]
		if !ok {
			return badRequestf(nil, "invalid log level value")
		}
		query = append(query, bson.DocElem{"level", logLevel})
	}
	if strType != "" {
		logType, ok := paramsLogTypes[params.LogType(strType)]
		if !ok {
			return badRequestf(nil, "invalid log type value")
		}
		query = append(query, bson.DocElem{"type", logType})
	}
	// Retrieve the logs.
	outputStarted := false
	closingContent := "[]"
	var log mongodoc.Log
	iter := h.Store.DB.Logs().Find(query).Sort("-_id").Skip(offset).Limit(limit).Iter()
	for iter.Next(&log) {
		// Start writing the response body. The logs are streamed, but we wrap
		// the output in square brackets and we separate entries with commas so
		// that it's more easy for clients to parse the response.
		closingContent = "]"
		if outputStarted {
			if err := writeString(w, ","); err != nil {
				return errgo.Notef(err, "cannot write response")
			}
		} else {
			if err := writeString(w, "["); err != nil {
				return errgo.Notef(err, "cannot write response")
			}
			outputStarted = true
		}
		logResponse := &params.LogResponse{
			Data:  json.RawMessage(log.Data),
			Level: mongodocLogLevels[log.Level],
			Type:  mongodocLogTypes[log.Type],
			URLs:  log.URLs,
			Time:  log.Time.UTC(),
		}
		if err := encoder.Encode(logResponse); err != nil {
			// Since we only allow properly encoded JSON messages to be stored
			// in the database, this should never happen. Moreover, at this
			// point we already sent a chunk of the 200 response, so we just
			// log the error.
			logger.Errorf("cannot marshal log: %s", err)
		}
	}
	if err := iter.Close(); err != nil {
		return errgo.Notef(err, "cannot retrieve logs")
	}

	// Close the JSON list, or just write an empty list, depending on whether
	// we had results.
	if err := writeString(w, closingContent); err != nil {
		return errgo.Notef(err, "cannot write response")
	}
	return nil
}

func (h *ReqHandler) postLogs(w http.ResponseWriter, req *http.Request) error {
	// Check the request content type.
	if ctype := req.Header.Get("Content-Type"); ctype != "application/json" {
		return badRequestf(nil, "unexpected Content-Type %q; expected 'application/json'", ctype)
	}

	// Unmarshal the request body.
	var logs []params.Log
	decoder := json.NewDecoder(req.Body)
	if err := decoder.Decode(&logs); err != nil {
		return badRequestf(err, "cannot unmarshal body")
	}

	for _, log := range logs {
		// Validate the provided level and type.
		logLevel, ok := paramsLogLevels[log.Level]
		if !ok {
			return badRequestf(nil, "invalid log level")
		}
		logType, ok := paramsLogTypes[log.Type]
		if !ok {
			return badRequestf(nil, "invalid log type")
		}

		// Add the log to the database.
		if err := h.Store.AddLog(log.Data, logLevel, logType, log.URLs); err != nil {
			return errgo.Notef(err, "cannot add log")
		}
	}
	return nil
}

func writeString(w io.Writer, content string) error {
	_, err := w.Write([]byte(content))
	return err
}

// TODO (frankban): use slices instead of maps for the data structures below.
var (
	// mongodocLogLevels maps internal mongodoc log levels to API ones.
	mongodocLogLevels = map[mongodoc.LogLevel]params.LogLevel{
		mongodoc.InfoLevel:    params.InfoLevel,
		mongodoc.WarningLevel: params.WarningLevel,
		mongodoc.ErrorLevel:   params.ErrorLevel,
	}
	// paramsLogLevels maps API params log levels to internal mongodoc ones.
	paramsLogLevels = map[params.LogLevel]mongodoc.LogLevel{
		params.InfoLevel:    mongodoc.InfoLevel,
		params.WarningLevel: mongodoc.WarningLevel,
		params.ErrorLevel:   mongodoc.ErrorLevel,
	}
	// mongodocLogTypes maps internal mongodoc log types to API ones.
	mongodocLogTypes = map[mongodoc.LogType]params.LogType{
		mongodoc.IngestionType:        params.IngestionType,
		mongodoc.LegacyStatisticsType: params.LegacyStatisticsType,
	}
	// paramsLogTypes maps API params log types to internal mongodoc ones.
	paramsLogTypes = map[params.LogType]mongodoc.LogType{
		params.IngestionType:        mongodoc.IngestionType,
		params.LegacyStatisticsType: mongodoc.LegacyStatisticsType,
	}
)

// intValue checks that the given string value is a number greater than or
// equal to the given minValue. If the provided value is an empty string, the
// defaultValue is returned without errors.
func intValue(strValue string, minValue, defaultValue int) (int, error) {
	if strValue == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(strValue)
	if err != nil {
		return 0, errgo.New("value must be a number")
	}
	if value < minValue {
		return 0, errgo.Newf("value must be >= %d", minValue)
	}
	return value, nil
}
