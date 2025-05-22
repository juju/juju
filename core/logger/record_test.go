// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

import (
	"encoding/json"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type LogRecordSuite struct {
	testhelpers.IsolationSuite
}

func TestLogRecordSuite(t *stdtesting.T) {
	tc.Run(t, &LogRecordSuite{})
}

func (s *LogRecordSuite) TestMarshall(c *tc.C) {
	rec := &logger.LogRecord{
		Time:      time.Date(2024, 1, 1, 9, 8, 7, 0, time.UTC),
		ModelUUID: coretesting.ModelTag.Id(),
		Entity:    "some-entity",
		Level:     2,
		Module:    "some-module",
		Location:  "some-location",
		Message:   "some-message",
		Labels:    map[string]string{"foo": "bar"},
	}
	data, err := json.Marshal(rec)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, `{"model-uuid":"deadbeef-0bad-400d-8000-4b1d0d06f00d","timestamp":"2024-01-01T09:08:07Z","entity":"some-entity","level":"DEBUG","module":"some-module","location":"some-location","message":"some-message","labels":{"foo":"bar"}}`)

	rec.ModelUUID = ""
	data, err = json.Marshal(rec)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, `{"timestamp":"2024-01-01T09:08:07Z","entity":"some-entity","level":"DEBUG","module":"some-module","location":"some-location","message":"some-message","labels":{"foo":"bar"}}`)
}

func (s *LogRecordSuite) TestMarshallRoundTrip(c *tc.C) {
	rec := &logger.LogRecord{
		Time:      time.Date(2024, 1, 1, 9, 8, 7, 0, time.UTC),
		ModelUUID: coretesting.ModelTag.Id(),
		Entity:    "some-entity",
		Level:     2,
		Module:    "some-module",
		Location:  "some-location",
		Message:   "some-message",
		Labels:    map[string]string{"foo": "bar"},
	}
	data, err := json.Marshal(rec)
	c.Assert(err, tc.ErrorIsNil)
	var got logger.LogRecord
	err = json.Unmarshal(data, &got)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, *rec)
}
