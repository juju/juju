// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslog

import (
	"crypto/tls"
	"fmt"
	"io"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/rfc/v2/rfc5424"
	"github.com/juju/rfc/v2/rfc5424/sdelements"

	"github.com/juju/juju/internal/logfwd"
)

// Sender exposes the underlying functionality needed by Client.
type Sender interface {
	io.Closer

	// Send sends the RFC 5424 message over its connection.
	Send(rfc5424.Message) error
}

// SenderOpener supports opening a syslog connection.
type SenderOpener interface {
	DialFunc(cfg *tls.Config, timeout time.Duration) (rfc5424.DialFunc, error)

	Open(host string, cfg rfc5424.ClientConfig, dial rfc5424.DialFunc) (Sender, error)
}

type senderOpener struct{}

func (senderOpener) DialFunc(cfg *tls.Config, timeout time.Duration) (rfc5424.DialFunc, error) {
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
	tlsCfg, err := cfg.tlsConfig()
	if err != nil {
		return nil, errors.Annotate(err, "constructing TLS config")
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
func (client Client) Send(records []logfwd.Record) error {
	for _, rec := range records {
		msg, err := messageFromRecord(rec)
		if err != nil {
			return errors.Trace(err)
		}
		if err := client.Sender.Send(msg); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func messageFromRecord(rec logfwd.Record) (rfc5424.Message, error) {
	msg := rfc5424.Message{
		Header: rfc5424.Header{
			Priority: rfc5424.Priority{
				Severity: rfc5424.SeverityWarning,
				Facility: rfc5424.FacilityUser,
			},
			Timestamp: rfc5424.Timestamp{rec.Timestamp},
			Hostname: rfc5424.Hostname{
				FQDN: rec.Origin.Hostname,
			},
			AppName: rfc5424.AppName((rec.Origin.Software.Name + "-" + rec.Origin.ModelUUID)[:48]),
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
			&sdelements.Private{
				Name: "log",
				PEN:  sdelements.PrivateEnterpriseNumber(rec.Origin.Software.PrivateEnterpriseNumber),
				Data: []rfc5424.StructuredDataParam{{
					Name:  "module",
					Value: rfc5424.StructuredDataParamValue(rec.Location.Module),
				}, {
					Name:  "source",
					Value: rfc5424.StructuredDataParamValue(fmt.Sprintf("%s:%d", rec.Location.Filename, rec.Location.Line)),
				}},
			},
		},
		Msg: rec.Message,
	}

	switch rec.Level {
	case loggo.ERROR:
		msg.Priority.Severity = rfc5424.SeverityError
	case loggo.WARNING:
		msg.Priority.Severity = rfc5424.SeverityWarning
	case loggo.INFO:
		msg.Priority.Severity = rfc5424.SeverityInformational
	case loggo.DEBUG, loggo.TRACE:
		msg.Priority.Severity = rfc5424.SeverityDebug
	default:
		return msg, errors.Errorf("unsupported log level %q", rec.Level)
	}

	if err := msg.Validate(); err != nil {
		return msg, errors.Trace(err)
	}
	return msg, nil
}
