// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rfc5424_test

import (
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/standards/rfc5424"
)

type HeaderSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&HeaderSuite{})

func (s *HeaderSuite) TestStringFull(c *gc.C) {
	header := rfc5424.Header{
		Priority: rfc5424.Priority{
			Severity: rfc5424.SeverityWarning,
			Facility: rfc5424.FacilityDaemon,
		},
		Timestamp: rfc5424.Timestamp{time.Unix(54321, 123).UTC()},
		Hostname:  rfc5424.Hostname{FQDN: "a.b.org"},
		AppName:   "an-app",
		ProcID:    "119",
		MsgID:     "xyz...",
	}

	str := header.String()

	c.Check(str, gc.Equals, "<28>1 1970-01-01T15:05:21.000000123Z a.b.org an-app 119 xyz...")
}

func (s *HeaderSuite) TestStringZeroValue(c *gc.C) {
	var header rfc5424.Header

	str := header.String()

	c.Check(str, gc.Equals, "<8>1 - - - - -")
}

func (s *HeaderSuite) TestValidateOkay(c *gc.C) {
	header := rfc5424.Header{
		Priority: rfc5424.Priority{
			Severity: rfc5424.SeverityWarning,
			Facility: rfc5424.FacilityDaemon,
		},
		Timestamp: rfc5424.Timestamp{time.Unix(54321, 0)},
		Hostname:  rfc5424.Hostname{FQDN: "a.b.org"},
		AppName:   "an-app",
		ProcID:    "119",
		MsgID:     "xyz...",
	}

	err := header.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *HeaderSuite) TestValidateZeroValue(c *gc.C) {
	var header rfc5424.Header

	err := header.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *HeaderSuite) TestValidateBadPriority(c *gc.C) {
	header := rfc5424.Header{
		Priority: rfc5424.Priority{
			Severity: rfc5424.Severity(-1),
			Facility: rfc5424.FacilityDaemon,
		},
		Timestamp: rfc5424.Timestamp{time.Unix(54321, 0)},
		Hostname:  rfc5424.Hostname{FQDN: "a.b.org"},
		AppName:   "an-app",
		ProcID:    "119",
		MsgID:     "xyz...",
	}

	err := header.Validate()

	c.Check(err, gc.ErrorMatches, `bad Priority: bad Severity: severity -1 not recognized`)
}

func (s *HeaderSuite) TestValidateEmptyTimestamp(c *gc.C) {
	header := rfc5424.Header{
		Priority: rfc5424.Priority{
			Severity: rfc5424.SeverityWarning,
			Facility: rfc5424.FacilityDaemon,
		},
		Timestamp: rfc5424.Timestamp{},
		Hostname:  rfc5424.Hostname{FQDN: "a.b.org"},
		AppName:   "an-app",
		ProcID:    "119",
		MsgID:     "xyz...",
	}

	err := header.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *HeaderSuite) TestValidateBadHostname(c *gc.C) {
	header := rfc5424.Header{
		Priority: rfc5424.Priority{
			Severity: rfc5424.SeverityWarning,
			Facility: rfc5424.FacilityDaemon,
		},
		Timestamp: rfc5424.Timestamp{time.Unix(54321, 0)},
		Hostname:  rfc5424.Hostname{FQDN: "\x09.b.org"},
		AppName:   "an-app",
		ProcID:    "119",
		MsgID:     "xyz...",
	}

	err := header.Validate()

	c.Check(err, gc.ErrorMatches, `bad Hostname: must be printable US ASCII \(\\x09 at pos 0\)`)
}

func (s *HeaderSuite) TestValidateBadAppName(c *gc.C) {
	header := rfc5424.Header{
		Priority: rfc5424.Priority{
			Severity: rfc5424.SeverityWarning,
			Facility: rfc5424.FacilityDaemon,
		},
		Timestamp: rfc5424.Timestamp{time.Unix(54321, 0)},
		Hostname:  rfc5424.Hostname{FQDN: "a.b.org"},
		AppName:   "\x09",
		ProcID:    "119",
		MsgID:     "xyz...",
	}

	err := header.Validate()

	c.Check(err, gc.ErrorMatches, `bad AppName: must be printable US ASCII \(\\x09 at pos 0\)`)
}

func (s *HeaderSuite) TestValidateBadProcID(c *gc.C) {
	header := rfc5424.Header{
		Priority: rfc5424.Priority{
			Severity: rfc5424.SeverityWarning,
			Facility: rfc5424.FacilityDaemon,
		},
		Timestamp: rfc5424.Timestamp{time.Unix(54321, 0)},
		Hostname:  rfc5424.Hostname{FQDN: "a.b.org"},
		AppName:   "an-app",
		ProcID:    "\x09",
		MsgID:     "xyz...",
	}

	err := header.Validate()

	c.Check(err, gc.ErrorMatches, `bad ProcID: must be printable US ASCII \(\\x09 at pos 0\)`)
}

func (s *HeaderSuite) TestValidateBadMsgID(c *gc.C) {
	header := rfc5424.Header{
		Priority: rfc5424.Priority{
			Severity: rfc5424.SeverityWarning,
			Facility: rfc5424.FacilityDaemon,
		},
		Timestamp: rfc5424.Timestamp{time.Unix(54321, 0)},
		Hostname:  rfc5424.Hostname{FQDN: "a.b.org"},
		AppName:   "an-app",
		ProcID:    "119",
		MsgID:     "\x09",
	}

	err := header.Validate()

	c.Check(err, gc.ErrorMatches, `bad MsgID: must be printable US ASCII \(\\x09 at pos 0\)`)
}
