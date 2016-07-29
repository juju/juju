// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package mock

import (
	"flag"
	"github.com/altoros/gosigma/https"
	"net/http/httputil"
	"strings"
	"testing"
)

var logFlag = flag.String("log.mock", "n", "log mock server requests: none|n, url|u, detail|d")

type severity int

const (
	logNone severity = iota
	logURL
	logDetail
)

func parseLogSeverity(s *string) severity {
	if s == nil || len(*s) == 0 {
		return logNone
	}
	switch (*s)[0] {
	case 'n':
		return logNone
	case 'u':
		return logURL
	case 'd':
		return logDetail
	default:
		return logNone
	}
}

func log() severity {
	return parseLogSeverity(logFlag)
}

// LogResponse log journal entries associated with response to testing log
func LogResponse(t *testing.T, r *https.Response) {
	id := GetIDFromResponse(r)
	jj := GetJournal(id)
	Log(t, jj)
}

// Log journal entry to testing log
func Log(t *testing.T, jj []JournalEntry) {
	for _, j := range jj {
		switch log() {
		case logURL:
			LogURL(t, j)
		case logDetail:
			LogDetail(t, j)
		}
	}
}

// LogURL writes URL from journal entry to testing log
func LogURL(t *testing.T, j JournalEntry) {
	t.Log(j.Request.RequestURI)
}

// LogDetail writes detailed information about journal entry to testing log
func LogDetail(t *testing.T, j JournalEntry) {
	req := j.Request
	buf, err := httputil.DumpRequest(req, true)
	if err != nil {
		t.Error("Error dumping request:", err)
		return
	}

	t.Log(string(buf))
	t.Log()

	resp := j.Response
	t.Logf("HTTP/%d", resp.Code)
	for header, values := range resp.Header() {
		t.Log(header+":", strings.Join(values, ","))
	}
	t.Log()
	t.Log(resp.Body.String())
	t.Log()
}
