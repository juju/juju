// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v5 // import "gopkg.in/juju/charmstore.v5/internal/v5"

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/juju/utils/debugstatus"
	"golang.org/x/net/context"
	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2/bson"

	"gopkg.in/juju/charmstore.v5/internal/mongodoc"
)

// GET /debug/status
// https://github.com/juju/charmstore/blob/v4/docs/API.md#get-debugstatus
func (h *ReqHandler) serveDebugStatus(_ http.Header, req *http.Request) (interface{}, error) {
	h.Store.SetReconnectTimeout(500 * time.Millisecond)
	return debugstatus.Check(
		context.TODO(),
		debugstatus.ServerStartTime,
		debugstatus.Connection(h.Store.DB.Session),
		debugstatus.MongoCollections(h.Store.DB),
		h.checkElasticSearch,
		h.checkEntities,
		h.checkBaseEntities,
	), nil
}

func (h *ReqHandler) checkElasticSearch(context.Context) (key string, result debugstatus.CheckResult) {
	key = "elasticsearch"
	result.Name = "Elastic search is running"
	if h.Store.ES == nil || h.Store.ES.Database == nil {
		result.Value = "Elastic search is not configured"
		result.Passed = true
		return key, result
	}
	health, err := h.Store.ES.Health()
	if err != nil {
		result.Value = "Connection issues to Elastic Search: " + err.Error()
		return key, result
	}
	result.Value = health.String()
	result.Passed = health.Status == "green"
	return key, result
}

func (h *ReqHandler) checkEntities(context.Context) (key string, result debugstatus.CheckResult) {
	result.Name = "Entities in charm store"
	charms, err := h.Store.DB.Entities().Find(bson.D{{"series", bson.D{{"$ne", "bundle"}}}}).Count()
	if err != nil {
		result.Value = "Cannot count charms for consistency check: " + err.Error()
		return "entities", result
	}
	bundles, err := h.Store.DB.Entities().Find(bson.D{{"series", "bundle"}}).Count()
	if err != nil {
		result.Value = "Cannot count bundles for consistency check: " + err.Error()
		return "entities", result
	}
	promulgated, err := h.Store.DB.Entities().Find(bson.D{{"promulgated-url", bson.D{{"$exists", true}}}}).Count()
	if err != nil {
		result.Value = "Cannot count promulgated for consistency check: " + err.Error()
		return "entities", result
	}
	result.Value = fmt.Sprintf("%d charms; %d bundles; %d promulgated", charms, bundles, promulgated)
	result.Passed = true
	return "entities", result
}

func (h *ReqHandler) checkBaseEntities(context.Context) (key string, result debugstatus.CheckResult) {
	resultKey := "base_entities"
	result.Name = "Base entities in charm store"

	// Retrieve the number of base entities.
	baseNum, err := h.Store.DB.BaseEntities().Count()
	if err != nil {
		result.Value = "Cannot count base entities: " + err.Error()
		return resultKey, result
	}

	// Retrieve the number of entities.
	num, err := h.Store.DB.Entities().Count()
	if err != nil {
		result.Value = "Cannot count entities for consistency check: " + err.Error()
		return resultKey, result
	}

	result.Value = fmt.Sprintf("count: %d", baseNum)
	result.Passed = num >= baseNum
	return resultKey, result
}

// findTimesInLogs goes through logs in reverse order finding when the start and
// end messages were last added.
func (h *ReqHandler) findTimesInLogs(logType mongodoc.LogType, startPrefix, endPrefix string) (start, end time.Time, err error) {
	var log mongodoc.Log
	iter := h.Store.DB.Logs().Find(bson.D{
		{"level", mongodoc.InfoLevel},
		{"type", logType},
	}).Sort("-time", "-id").Iter()
	for iter.Next(&log) {
		var msg string
		if err := json.Unmarshal(log.Data, &msg); err != nil {
			// an error here probably means the log isn't in the form we are looking for.
			continue
		}
		if start.IsZero() && strings.HasPrefix(msg, startPrefix) {
			start = log.Time
		}
		if end.IsZero() && strings.HasPrefix(msg, endPrefix) {
			end = log.Time
		}
		if !start.IsZero() && !end.IsZero() {
			break
		}
	}
	if err = iter.Close(); err != nil {
		return time.Time{}, time.Time{}, errgo.Notef(err, "Cannot query logs")
	}
	return
}
