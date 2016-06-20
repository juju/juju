// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rfc5424

import (
	"fmt"
)

// These are the supported logging severity levels.
const (
	SeverityEmergency Severity = iota
	SeverityAlert
	SeverityCrit
	SeverityError
	SeverityWarning
	SeverityNotice
	SeverityInformational
	SeverityDebug

	severityTooLarge
)

// These are the supported logging facilities.
const (
	facilityDefault Facility = iota

	FacilityKern
	FacilityUser // default
	FacilityMail
	FacilityDaemon
	FacilityAuth
	FacilitySyslog
	FacilityLPR
	FacilityNews
	FacilityUUCP
	FacilityCron
	FacilityAuthpriv
	FacilityFTP
	FacilityNTP

	facilityLogAudit
	facilityLogAlert
	facilityCron2

	FacilityLocal0
	FacilityLocal1
	FacilityLocal2
	FacilityLocal3
	FacilityLocal4
	FacilityLocal5
	FacilityLocal6
	FacilityLocal7

	facilityTooLarge
)

// Priority identifies the importance of a log record.
type Priority struct {
	// Severity is the criticality of the log record.
	Severity Severity

	// Facility is the system component for which the log record
	// was created.
	Facility Facility
}

func (p Priority) encode() int {
	return p.Facility.encode()<<3 + p.Severity.encode()
}

func (p *Priority) decode(code int) {
	copied := *p
	copied.Severity.decode(code & 0x07)
	copied.Facility.decode(code >> 3)
	*p = copied
}

// String returns the RFC 5424 representation of the priority.
func (p Priority) String() string {
	return fmt.Sprintf("<%d>", p.encode())
}

// Validated ensures that the priority is correct.
func (p Priority) Validate() error {
	if err := p.Severity.Validate(); err != nil {
		return err
	}
	if err := p.Facility.Validate(); err != nil {
		return err
	}
	return nil
}

// Severity is the criticality of the log record.
type Severity int

func (s Severity) encode() int {
	return int(s)
}

func (s *Severity) decode(code int) {
	*s = Severity(code)
}

// String returns the name of the severity.
func (s Severity) String() string {
	switch s {
	case SeverityEmergency:
		return "EMERGENCY"
	case SeverityAlert:
		return "ALERT"
	case SeverityCrit:
		return "CRIT"
	case SeverityError:
		return "ERROR"
	case SeverityWarning:
		return "WARNING"
	case SeverityNotice:
		return "NOTICE"
	case SeverityInformational:
		return "INFO"
	case SeverityDebug:
		return "DEBUG"
	default:
		return fmt.Sprintf("Severity %d", int(s))
	}
}

// Validate ensures that the severity is correct. This will fail only
// in cases where an unsupported int is converted into a Severity.
func (s Severity) Validate() error {
	if s < 0 || s >= severityTooLarge {
		return fmt.Errorf("severity %d not recognized", s)
	}
	return nil
}

// Facility is the system component for which the log record
// was created.
type Facility int

func (f Facility) encode() int {
	if f == facilityDefault {
		f = FacilityUser
	}
	return int(f) - 1
}

func (f *Facility) decode(code int) {
	*f = Facility(code + 1)
}

// String returns the name of the facility.
func (f Facility) String() string {
	if f == facilityDefault {
		f = FacilityUser
	}
	switch f {
	case FacilityKern:
		return "KERN"
	case FacilityUser:
		return "USER"
	case FacilityMail:
		return "MAIL"
	case FacilityDaemon:
		return "DAEMON"
	case FacilityAuth:
		return "AUTH"
	case FacilitySyslog:
		return "SYSLOG"
	case FacilityLPR:
		return "LPR"
	case FacilityNews:
		return "NEWS"
	case FacilityUUCP:
		return "UUCP"
	case FacilityCron:
		return "CRON"
	case FacilityAuthpriv:
		return "AUTHPRIV"
	case FacilityFTP:
		return "FTP"
	case FacilityLocal0:
		return "LOCAL0"
	case FacilityLocal1:
		return "LOCAL1"
	case FacilityLocal2:
		return "LOCAL2"
	case FacilityLocal3:
		return "LOCAL3"
	case FacilityLocal4:
		return "LOCAL4"
	case FacilityLocal5:
		return "LOCAL5"
	case FacilityLocal6:
		return "LOCAL6"
	case FacilityLocal7:
		return "LOCAL7"
	default:
		return fmt.Sprint("Facility %d", int(f))
	}
}

// Validate ensures that the facility is correct.
func (f Facility) Validate() error {
	if f == facilityDefault {
		return nil
	}
	if f < 0 || f >= facilityTooLarge {
		return fmt.Errorf("facility %d not recognized", f)
	}
	return nil
}
