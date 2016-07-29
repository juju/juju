// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package mock

import (
	"net/http"
	"net/http/httptest"
)

// JournalEntry contains single journal record
type JournalEntry struct {
	Name     string
	Request  *http.Request
	Response *httptest.ResponseRecorder
}

var journal = make(map[int][]JournalEntry)

type journalRequest struct {
	id    int
	entry JournalEntry
	reply chan []JournalEntry
}

var chPut = make(chan journalRequest)
var chGet = make(chan journalRequest)

func init() {
	go func() {
		for {
			select {
			case req := <-chPut:
				journal[req.id] = append(journal[req.id], req.entry)
			case req := <-chGet:
				req.reply <- journal[req.id]
			}
		}
	}()
}

func recordJournal(name string, r *http.Request, rr *httptest.ResponseRecorder) {
	id := GetIDFromRequest(r)
	SetID(rr.HeaderMap, id)
	PutJournal(id, name, r, rr)
}

// PutJournal adds record to specified journal
func PutJournal(id int, name string, r *http.Request, rr *httptest.ResponseRecorder) {
	entry := JournalEntry{name, r, rr}
	jr := journalRequest{id, entry, nil}
	chPut <- jr
}

// GetJournal retrivies record from specified journal
func GetJournal(id int) []JournalEntry {
	ch := make(chan []JournalEntry)
	jr := journalRequest{id, JournalEntry{}, ch}
	chGet <- jr
	return <-ch
}
