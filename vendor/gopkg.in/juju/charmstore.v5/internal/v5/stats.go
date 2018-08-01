// Copyright 2012 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v5 // import "gopkg.in/juju/charmstore.v5/internal/v5"

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charmrepo.v3/csclient/params"

	"gopkg.in/juju/charmstore.v5/internal/charmstore"
	"gopkg.in/juju/charmstore.v5/internal/mongodoc"
)

const dateFormat = "2006-01-02"

// parseDateRange parses a date range as specified in an http
// request. The returned times will be zero if not specified.
func parseDateRange(form url.Values) (start, stop time.Time, err error) {
	if v := form.Get("start"); v != "" {
		var err error
		start, err = time.Parse(dateFormat, v)
		if err != nil {
			return time.Time{}, time.Time{}, badRequestf(err, "invalid 'start' value %q", v)
		}
	}
	if v := form.Get("stop"); v != "" {
		var err error
		stop, err = time.Parse(dateFormat, v)
		if err != nil {
			return time.Time{}, time.Time{}, badRequestf(err, "invalid 'stop' value %q", v)
		}
		// Cover all timestamps within the stop day.
		stop = stop.Add(24*time.Hour - 1*time.Second)
	}
	return
}

// GET stats/counter/key[:key]...?[by=unit]&start=date][&stop=date][&list=1]
// https://github.com/juju/charmstore/blob/v4/docs/API.md#get-statscounter
func (h *ReqHandler) serveStatsCounter(_ http.Header, r *http.Request) (interface{}, error) {
	base := strings.TrimPrefix(r.URL.Path, "/")
	if strings.Index(base, "/") > 0 {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "invalid key")
	}
	if base == "" {
		return nil, params.ErrForbidden
	}
	var by charmstore.CounterRequestBy
	switch v := r.Form.Get("by"); v {
	case "":
		by = charmstore.ByAll
	case "day":
		by = charmstore.ByDay
	case "week":
		by = charmstore.ByWeek
	default:
		return nil, badRequestf(nil, "invalid 'by' value %q", v)
	}
	req := charmstore.CounterRequest{
		Key:  strings.Split(base, ":"),
		List: r.Form.Get("list") == "1",
		By:   by,
	}
	var err error
	req.Start, req.Stop, err = parseDateRange(r.Form)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	if req.Key[len(req.Key)-1] == "*" {
		req.Prefix = true
		req.Key = req.Key[:len(req.Key)-1]
		if len(req.Key) == 0 {
			return nil, errgo.WithCausef(nil, params.ErrForbidden, "unknown key")
		}
	}
	entries, err := h.Store.Counters(&req)
	if err != nil {
		return nil, errgo.Notef(err, "cannot query counters")
	}

	var buf []byte
	var items []params.Statistic
	for i := range entries {
		entry := &entries[i]
		buf = buf[:0]
		if req.List {
			for j := range entry.Key {
				buf = append(buf, entry.Key[j]...)
				buf = append(buf, ':')
			}
			if entry.Prefix {
				buf = append(buf, '*')
			} else {
				buf = buf[:len(buf)-1]
			}
		}
		stat := params.Statistic{
			Key:   string(buf),
			Count: entry.Count,
		}
		if !entry.Time.IsZero() {
			stat.Date = entry.Time.Format("2006-01-02")
		}
		items = append(items, stat)
	}

	return items, nil
}

// PUT stats/update
// https://github.com/juju/charmstore/blob/v4/docs/API.md#put-statsupdate
func (h *ReqHandler) serveStatsUpdate(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.authorize(authorizeParams{
		req: r,
		acls: []mongodoc.ACL{{
			Write: []string{"statsupdate@cs"},
		}},
		ops: []string{OpWrite},
	}); err != nil {
		return err
	}
	if r.Method != "PUT" {
		return errgo.WithCausef(nil, params.ErrMethodNotAllowed, "%s not allowed", r.Method)
	}

	var req params.StatsUpdateRequest
	if ct := r.Header.Get("Content-Type"); ct != "application/json" {
		return errgo.WithCausef(nil, params.ErrBadRequest, "unexpected Content-Type %q; expected %q", ct, "application/json")
	}

	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		return errgo.Notef(err, "cannot unmarshal body")
	}

	errors := make([]error, 0)
	for _, entry := range req.Entries {
		rid, err := h.Router.Context.ResolveURL(entry.CharmReference)
		if err != nil {
			errors = append(errors, errgo.Notef(err, "cannot find entity for url %s", entry.CharmReference))
			continue
		}

		logger.Infof("Increase download stats for id: %s at time: %s", rid, entry.Timestamp)

		if err := h.Store.IncrementDownloadCountsAtTime(rid, entry.Timestamp); err != nil {
			errors = append(errors, err)
			continue
		}
	}

	if len(errors) != 0 {
		logger.Infof("Errors detected during /stats/update processing: %v", errors)
		if len(errors) > 1 {
			return errgo.Newf("%s (and %d more errors)", errors[0], len(errors)-1)
		}
		return errors[0]
	}

	return nil
}

// StatsEnabled reports whether statistics should be gathered for
// the given HTTP request.
func StatsEnabled(req *http.Request) bool {
	// It's fine to parse the form more than once, and it avoids
	// bugs from not parsing it.
	req.ParseForm()
	return req.Form.Get("stats") != "0"
}
