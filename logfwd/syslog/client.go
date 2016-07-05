// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslog

import (
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/logfwd"
	"github.com/juju/juju/standards/rfc5424"
	"github.com/juju/juju/standards/rfc5424/sdelements"
	"github.com/juju/juju/standards/tls"
)

// Sender exposes the underlying functionality needed by Client.
type Sender interface {
	io.Closer

	// Send sends the RFC 5424 message over its connection.
	Send(rfc5424.Message) error
}

// SenderOpener supports opening a syslog connection.
type SenderOpener interface {
	DialFunc(cfg tls.Config, timeout time.Duration) (rfc5424.DialFunc, error)

	Open(host string, cfg rfc5424.ClientConfig, dial rfc5424.DialFunc) (Sender, error)
}

type senderOpener struct{}

func (senderOpener) DialFunc(cfg tls.Config, timeout time.Duration) (rfc5424.DialFunc, error) {
	dial, err := rfc5424.TLSDialFunc(cfg, timeout)
	return dial, errors.Trace(err)
}

func (senderOpener) Open(host string, cfg rfc5424.ClientConfig, dial rfc5424.DialFunc) (Sender, error) {
	sender, err := rfc5424.Open(host, cfg, dial)
	return sender, errors.Trace(err)
}

// Client is the wrapper around a syslog (RFC 5424) connection.
type Client struct {
	// Sender is the message sender this client wraps.
	Sender Sender
}

// Open connects to a remote syslog host and wraps that connection
// in a new client.
func Open(cfg RawConfig) (*Client, error) {
	client, err := OpenForSender(cfg, &senderOpener{})
	return client, errors.Trace(err)
}

// OpenForSender connects to a remote syslog host and wraps that
// connection in a new client.
func OpenForSender(cfg RawConfig, opener SenderOpener) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	sender, err := open(cfg, opener)
	if err != nil {
		return nil, errors.Trace(err)
	}

	client := &Client{
		Sender: sender,
	}
	return client, nil
}

func open(cfg RawConfig, opener SenderOpener) (Sender, error) {
	tlsCfg := tls.Config{
		RawCert: tls.RawCert{
			CertPEM:   cfg.ClientCert,
			KeyPEM:    cfg.ClientKey,
			CACertPEM: cfg.ClientCACert,
		},
		//ServerName: "",
		ExpectedServerCertPEM: cfg.ExpectedServerCert,
	}
	var timeout time.Duration
	dial, err := opener.DialFunc(tlsCfg, timeout)
	if err != nil {
		return nil, errors.Annotate(err, "obtaining dialer")
	}

	var clientCfg rfc5424.ClientConfig
	client, err := opener.Open(cfg.Host, clientCfg, dial)
	return client, errors.Annotate(err, "opening client connection")
}

// Close closes the client's connection.
func (client Client) Close() error {
	err := client.Sender.Close()
	return errors.Trace(err)
}

// Send sends the record to the remote syslog host.
func (client Client) Send(rec logfwd.Record) error {
	msg, err := messageFromRecord(rec)
	if err != nil {
		return errors.Trace(err)
	}
	if err := client.Sender.Send(msg); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func messageFromRecord(rec logfwd.Record) (rfc5424.Message, error) {
	msg, err := messageFromBaseRecord(rec.Base())
	if err != nil {
		return msg, errors.Trace(err)
	}
	origin := rec.Base().Origin

	switch rec.Kind() {
	case logfwd.RecordKindLog:
		severity, err := severityFromLogLevel(rec.(logfwd.LogRecord).Level)
		if err != nil {
			return msg, errors.Trace(err)
		}
		msg.Priority.Severity = severity

		loc := rec.(logfwd.LogRecord).Location
		msg.StructuredData = append(msg.StructuredData, &sdelements.Private{
			Name: "log",
			PEN:  sdelements.PrivateEnterpriseNumber(origin.Software.PrivateEnterpriseNumber),
			Data: []rfc5424.StructuredDataParam{{
				Name:  "module",
				Value: rfc5424.StructuredDataParamValue(loc.Module),
			}, {
				Name:  "source",
				Value: rfc5424.StructuredDataParamValue(fmt.Sprintf("%s:%d", loc.Filename, loc.Line)),
			}},
		})
	case logfwd.RecordKindAudit:
		msg.Priority.Severity = rfc5424.SeverityInformational

		audit := rec.(logfwd.AuditRecord).Audit
		elem := &sdelements.Private{
			Name: "audit",
			PEN:  sdelements.PrivateEnterpriseNumber(origin.Software.PrivateEnterpriseNumber),
			Data: []rfc5424.StructuredDataParam{{
				Name:  "origin-type",
				Value: rfc5424.StructuredDataParamValue(origin.Type.String()),
			}, {
				Name:  "origin-name",
				Value: rfc5424.StructuredDataParamValue(origin.Name),
			}, {
				Name:  "operation",
				Value: rfc5424.StructuredDataParamValue(audit.Operation),
			}},
		}
		for name, value := range audit.Args {
			elem.Data = append(elem.Data, rfc5424.StructuredDataParam{
				Name:  rfc5424.StructuredDataName(name),
				Value: rfc5424.StructuredDataParamValue(value),
			})
		}
		msg.StructuredData = append(msg.StructuredData, elem)
	default:
		return msg, errors.Errorf("unsupported record kind %q", rec.Kind())
	}

	if err := msg.Validate(); err != nil {
		return msg, errors.Trace(err)
	}
	return msg, nil
}

func messageFromBaseRecord(rec logfwd.BaseRecord) (rfc5424.Message, error) {
	swName := strings.Split(rec.Origin.Software.Name, "-")[0]
	appName := swName + "-" + rec.Origin.ModelUUID

	var hostname rfc5424.Hostname
	hostname.StaticIP = net.ParseIP(rec.Origin.Hostname)
	if hostname.StaticIP == nil {
		hostname.FQDN = rec.Origin.Hostname
	}

	msg := rfc5424.Message{
		Header: rfc5424.Header{
			Priority: rfc5424.Priority{
				Severity: rfc5424.SeverityWarning,
				Facility: rfc5424.FacilityUser,
			},
			Timestamp: rfc5424.Timestamp{rec.Timestamp},
			Hostname:  hostname,
			AppName:   rfc5424.AppName(appName),
		},
		StructuredData: rfc5424.StructuredData{
			&sdelements.Origin{
				EnterpriseID: sdelements.OriginEnterpriseID{
					Number: sdelements.PrivateEnterpriseNumber(rec.Origin.Software.PrivateEnterpriseNumber),
				},
				SoftwareName:    rec.Origin.Software.Name,
				SoftwareVersion: rec.Origin.Software.Version,
			},
			&sdelements.Private{
				Name: "model",
				PEN:  sdelements.PrivateEnterpriseNumber(rec.Origin.Software.PrivateEnterpriseNumber),
				Data: []rfc5424.StructuredDataParam{{
					Name:  "controller-uuid",
					Value: rfc5424.StructuredDataParamValue(rec.Origin.ControllerUUID),
				}, {
					Name:  "model-uuid",
					Value: rfc5424.StructuredDataParamValue(rec.Origin.ModelUUID),
				}},
			},
		},
		Msg: rec.Message,
	}

	return msg, nil
}

func severityFromLogLevel(level loggo.Level) (rfc5424.Severity, error) {
	switch level {
	case loggo.ERROR:
		return rfc5424.SeverityError, nil
	case loggo.WARNING:
		return rfc5424.SeverityWarning, nil
	case loggo.INFO:
		return rfc5424.SeverityInformational, nil
	case loggo.DEBUG, loggo.TRACE:
		return rfc5424.SeverityDebug, nil
	default:
		return -1, errors.Errorf("unsupported log level %q", level)
	}
}
