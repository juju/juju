// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rfc5424_test

import (
	"fmt"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/standards/rfc5424"
)

type MessageSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&MessageSuite{})

func (s *MessageSuite) TestStringFull(c *gc.C) {
	stub := &testing.Stub{}
	msg := rfc5424.Message{
		Header: rfc5424.Header{
			Priority: rfc5424.Priority{
				Severity: rfc5424.SeverityWarning,
				Facility: rfc5424.FacilityDaemon,
			},
			Timestamp: rfc5424.Timestamp{time.Unix(54321, 123).UTC()},
			Hostname:  rfc5424.Hostname{FQDN: "a.b.org"},
			AppName:   "an-app",
			ProcID:    "119",
			MsgID:     "xyz...",
		},
		StructuredData: rfc5424.StructuredData{
			newStubElement(stub, "spam", "x=y"),
		},
		Msg: "a message",
	}

	str := msg.String()

	c.Check(str, gc.Equals, `<28>1 1970-01-01T15:05:21.000000123Z a.b.org an-app 119 xyz... [spam x="y"] a message`)
}

func (s *MessageSuite) TestStringZeroValue(c *gc.C) {
	var msg rfc5424.Message

	str := msg.String()

	c.Check(str, gc.Equals, "<8>1 - - - - - -")
}

func (s *MessageSuite) TestValidateOkay(c *gc.C) {
	stub := &testing.Stub{}
	msg := rfc5424.Message{
		Header: rfc5424.Header{
			Priority: rfc5424.Priority{
				Severity: rfc5424.SeverityWarning,
				Facility: rfc5424.FacilityDaemon,
			},
			Timestamp: rfc5424.Timestamp{time.Unix(54321, 0)},
			Hostname:  rfc5424.Hostname{FQDN: "a.b.org"},
			AppName:   "an-app",
			ProcID:    "119",
			MsgID:     "xyz...",
		},
		StructuredData: rfc5424.StructuredData{
			newStubElement(stub, "spam", "x=y"),
		},
		Msg: "a message",
	}

	err := msg.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *MessageSuite) TestValidateZeroValue(c *gc.C) {
	var msg rfc5424.Message

	err := msg.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *MessageSuite) TestValidateBadHeader(c *gc.C) {
	stub := &testing.Stub{}
	msg := rfc5424.Message{
		Header: rfc5424.Header{
			Priority: rfc5424.Priority{
				Severity: rfc5424.Severity(-1),
				Facility: rfc5424.FacilityDaemon,
			},
			Timestamp: rfc5424.Timestamp{time.Unix(54321, 0)},
			Hostname:  rfc5424.Hostname{FQDN: "a.b.org"},
			AppName:   "an-app",
			ProcID:    "119",
			MsgID:     "xyz...",
		},
		StructuredData: rfc5424.StructuredData{
			newStubElement(stub, "spam", "x=y"),
		},
		Msg: "a message",
	}

	err := msg.Validate()

	c.Check(err, gc.ErrorMatches, `bad Header: bad Priority: bad Severity: severity -1 not recognized`)
}

func (s *MessageSuite) TestValidateBadStructuredData(c *gc.C) {
	stub := &testing.Stub{}
	stub.SetErrors(fmt.Errorf("<invalid>"))
	msg := rfc5424.Message{
		Header: rfc5424.Header{
			Priority: rfc5424.Priority{
				Severity: rfc5424.SeverityWarning,
				Facility: rfc5424.FacilityDaemon,
			},
			Timestamp: rfc5424.Timestamp{time.Unix(54321, 0)},
			Hostname:  rfc5424.Hostname{FQDN: "a.b.org"},
			AppName:   "an-app",
			ProcID:    "119",
			MsgID:     "xyz...",
		},
		StructuredData: rfc5424.StructuredData{
			newStubElement(stub, "spam", "x=y"),
		},
		Msg: "a message",
	}

	err := msg.Validate()

	c.Check(err, gc.ErrorMatches, `bad StructuredData: element 0 not valid: <invalid>`)
}

func (s *MessageSuite) TestValidateEmptyMessage(c *gc.C) {
	stub := &testing.Stub{}
	msg := rfc5424.Message{
		Header: rfc5424.Header{
			Priority: rfc5424.Priority{
				Severity: rfc5424.SeverityWarning,
				Facility: rfc5424.FacilityDaemon,
			},
			Timestamp: rfc5424.Timestamp{time.Unix(54321, 0)},
			Hostname:  rfc5424.Hostname{FQDN: "a.b.org"},
			AppName:   "an-app",
			ProcID:    "119",
			MsgID:     "xyz...",
		},
		StructuredData: rfc5424.StructuredData{
			newStubElement(stub, "spam", "x=y"),
		},
		Msg: "",
	}

	err := msg.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *MessageSuite) TestValidateBadMessage(c *gc.C) {
	stub := &testing.Stub{}
	msg := rfc5424.Message{
		Header: rfc5424.Header{
			Priority: rfc5424.Priority{
				Severity: rfc5424.SeverityWarning,
				Facility: rfc5424.FacilityDaemon,
			},
			Timestamp: rfc5424.Timestamp{time.Unix(54321, 0)},
			Hostname:  rfc5424.Hostname{FQDN: "a.b.org"},
			AppName:   "an-app",
			ProcID:    "119",
			MsgID:     "xyz...",
		},
		StructuredData: rfc5424.StructuredData{
			newStubElement(stub, "spam", "x=y"),
		},
		Msg: invalidUTF8,
	}

	err := msg.Validate()

	c.Check(err, gc.ErrorMatches, `bad Msg: invalid UTF-8`)
}
