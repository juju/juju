// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package audit_test

import (
	"sync"
	"time"

	"github.com/juju/juju/audit"
	stateaudit "github.com/juju/juju/state/internal/audit"
	"github.com/juju/juju/state/internal/audit/fakeiterator"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
)

type readerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&readerSuite{})

func (*readerSuite) TestNewAuditTailer_ReadFromIterCancellable(c *gc.C) {
	done := make(chan struct{})

	openAuditIter := func(time.Time) stateaudit.Iterator {
		// Default fake iterator, with no records, will block until
		// close is called.
		return (&fakeiterator.Instance{}).Init()
	}

	records := stateaudit.NewAuditTailer(
		stateaudit.AuditTailerContext{
			Done:          done,
			Logger:        loggo.GetLogger("juju.state.auditing"),
			OpenAuditIter: openAuditIter,
		},
		time.Now(),
	)

	close(done)

	_, ok := <-records
	c.Check(ok, jc.IsFalse)
}

func (*readerSuite) TestNewAuditTailer_ReturnsRecords(c *gc.C) {
	openAuditIter := func(time.Time) stateaudit.Iterator {
		iter := &fakeiterator.Instance{
			TestRecordJson: []string{`
{
  "Data": {
    "\uff04a\uff0eb": {
      "b\uff0e\uff04a": "c"
    },
    "a": "b"
  },
  "Operation": "status",
  "OriginName": "bob",
  "OriginType": "user",
  "RemoteAddress": "8.8.8.8",
  "Timestamp": "2016-07-15T00:00:00Z",
  "ModelUUID": "c9a0f40e-ac16-4227-8bf3-dd6675205ab2",
  "JujuServerVersion": "1.0.0"
}`[1:],
			},
		}

		return iter.Init()
	}

	done := make(chan struct{})
	records := stateaudit.NewAuditTailer(
		stateaudit.AuditTailerContext{
			Done:          done,
			Logger:        loggo.GetLogger("juju.state.auditing"),
			OpenAuditIter: openAuditIter,
		},
		time.Now(),
	)

	timestamp, err := time.Parse("2006-01-02", "2016-07-15")
	if err != nil {
		c.Fatalf("cannot parse time: %v", err)
	}

	expected := []stateaudit.FetchedAuditEntry{
		{
			Value: audit.AuditEntry{
				JujuServerVersion: version.MustParse("1.0.0"),
				ModelUUID:         "c9a0f40e-ac16-4227-8bf3-dd6675205ab2",
				Timestamp:         timestamp,
				RemoteAddress:     "8.8.8.8",
				OriginType:        "user",
				OriginName:        "bob",
				Operation:         "status",
				Data: map[string]interface{}{
					"a": "b",
					"$a.b": map[string]interface{}{
						"b.$a": "c",
					},
				},
			},
		},
	}

	for _, e := range expected {
		auditEntry, ok := <-records
		c.Assert(ok, jc.IsTrue)
		if c.Check(auditEntry.Error, jc.ErrorIsNil) {
			c.Check(auditEntry, jc.DeepEquals, e)
		}
	}

	close(done)

	// We don't expect any more records
	_, ok := <-records
	c.Check(ok, jc.IsFalse)
}

func (*readerSuite) TestNewAuditTailer_ReopensCursorOnClose(c *gc.C) {
	done := make(chan struct{})
	defer close(done)

	// This WaitGroup allows control to be bounced between opening an
	// iterator, and closing it.
	var iterOpened sync.WaitGroup
	iterOpened.Add(1)

	var failingIter *fakeiterator.Instance
	openAuditIter := func(time.Time) stateaudit.Iterator {
		defer iterOpened.Done()
		failingIter = &fakeiterator.Instance{}
		return failingIter.Init()
	}

	// We don't care about the records, just start processing.
	stateaudit.NewAuditTailer(
		stateaudit.AuditTailerContext{
			Done:          done,
			Logger:        loggo.GetLogger("juju.state.auditing"),
			OpenAuditIter: openAuditIter,
		},
		time.Now(),
	)

	// This loop will bounce between waiting for openAuditIter above
	// to be called, and closing the iterator.
	for i := 5; i >= 0; i-- {
		iterOpened.Wait()
		iterOpened.Add(1)
		c.Check(failingIter.Close(), jc.ErrorIsNil)
	}
}

func (*readerSuite) TestNewAuditTailer_ReopensPastHighWatermark(c *gc.C) {
	done := make(chan struct{})
	defer close(done)

	var iter *fakeiterator.Instance
	var highWatermark time.Time
	var iterOpened sync.WaitGroup
	iterOpened.Add(1)
	openAuditIter := func(after time.Time) stateaudit.Iterator {
		defer iterOpened.Done()
		highWatermark = after
		iter = &fakeiterator.Instance{
			TestRecordJson: []string{`
{
  "Data": {
    "\uff04a\uff0eb": {
      "b\uff0e\uff04a": "c"
    },
    "a": "b"
  },
  "Operation": "status",
  "OriginName": "bob",
  "OriginType": "user",
  "RemoteAddress": "8.8.8.8",
  "Timestamp": "2016-07-15T00:00:00Z",
  "ModelUUID": "c9a0f40e-ac16-4227-8bf3-dd6675205ab2",
  "JujuServerVersion": "1.0.0"
}`[1:],
			},
		}
		return iter.Init()
	}

	highWatermark, err := time.Parse("2006-01-02", "1900-01-01")
	if err != nil {
		c.Fatalf("cannot parse timestamp: %v", err)
	}
	records := stateaudit.NewAuditTailer(
		stateaudit.AuditTailerContext{
			Done:          done,
			Logger:        loggo.GetLogger("juju.state.auditing"),
			OpenAuditIter: openAuditIter,
		},
		highWatermark,
	)

	// Grab the 1 record.
	_, ok := <-records
	c.Assert(ok, jc.IsTrue)

	// We should now be calling Next. Close the Iterator to trigger
	// opening a new one, and wait for it to be opened.
	iterOpened.Add(1)
	c.Check(iter.Close(), jc.ErrorIsNil)
	iterOpened.Wait()

	c.Check(highWatermark.Format(time.RFC3339), gc.Equals, "2016-07-15T00:00:00Z")
}
