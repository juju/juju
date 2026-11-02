// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"time"

	"github.com/juju/errors"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/api/client/backups"
	"github.com/juju/juju/cmd/modelcmd"
	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/rpc/params"
)

// APIClient represents the backups API client functionality used by
// the backups command.
type APIClient interface {
	io.Closer
	// Create asks the controller to create a new backup and stream
	// the archive back. The result carries the backup's metadata,
	// including its checksum; the returned reader holds the archive
	// bytes and must be closed by the caller.
	Create(ctx context.Context, notes string) (params.BackupsMetadataResult, io.ReadCloser, error)
}

// CommandBase is the base type for backups sub-commands.
type CommandBase struct {
	modelcmd.ModelCommandBase
}

// NewAPIClient returns a client for the backups api endpoint.
func (c *CommandBase) NewAPIClient(ctx context.Context) (APIClient, error) {
	return newAPIClient(ctx, c)
}

// NewGetAPI returns a client for the backups api endpoint.
func (c *CommandBase) NewGetAPI(ctx context.Context) (APIClient, error) {
	return getAPI(ctx, c)
}

var newAPIClient = func(ctx context.Context, c *CommandBase) (APIClient, error) {
	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return backups.NewClient(root), nil
}

// GetAPI returns a client and the api version of the controller
var getAPI = func(ctx context.Context, c *CommandBase) (APIClient, error) {
	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	client := backups.NewClient(root)
	return client, nil
}

// Assertion records the summary of a created backup archive: the
// metadata the controller reported alongside the archive, saved to a
// local "<archive>.assert" file. The assertion is unsigned YAML; its
// checksum is the archive's SHA-256 digest, which recovery verifies
// against the archive before use. The format version is the archive
// format version, not a version of the assertion itself.
type Assertion struct {
	FormatVersion  int64             `yaml:"format-version"`
	Checksum       string            `yaml:"checksum"`
	ChecksumFormat string            `yaml:"checksum-format"`
	Size           int64             `yaml:"size"`
	Stored         time.Time         `yaml:"stored"`
	Started        time.Time         `yaml:"started"`
	Finished       time.Time         `yaml:"finished"`
	Notes          string            `yaml:"notes"`
	ControllerUUID string            `yaml:"controller-uuid"`
	HANodes        int64             `yaml:"ha-nodes"`
	ModelUUID      string            `yaml:"model-uuid"`
	MachineID      string            `yaml:"machine-id"`
	Hostname       string            `yaml:"hostname"`
	JujuVersion    semversion.Number `yaml:"juju-version"`
	Base           string            `yaml:"base"`
}

func (c *CommandBase) metadata(result *params.BackupsMetadataResult) ([]byte, error) {
	a := Assertion{
		FormatVersion:  result.FormatVersion,
		Checksum:       corebackups.ChecksumSHA256(result.Checksum),
		ChecksumFormat: result.ChecksumFormat,
		Size:           result.Size,
		Stored:         result.Stored,
		Started:        result.Started,
		Finished:       result.Finished,
		Notes:          result.Notes,
		ControllerUUID: result.ControllerUUID,
		HANodes:        result.HANodes,
		ModelUUID:      result.Model,
		MachineID:      result.Machine,
		Hostname:       result.Hostname,
		JujuVersion:    result.Version,
		Base:           result.Base,
	}
	return yaml.Marshal(a)
}

// ReadAssertion reads and validates a backup assertion from r. The
// assertion must be exactly one YAML document with only known keys,
// and must record a SHA-256 checksum in the current checksum format:
// recovery refuses an assertion that cannot pin the archive's digest.
// Empty input and null documents are rejected as empty assertions.
// The remaining metadata is descriptive and is not validated here.
func ReadAssertion(r io.Reader) (*Assertion, error) {
	decoder := yaml.NewDecoder(r)
	decoder.KnownFields(true)
	var assertion *Assertion
	if err := decoder.Decode(&assertion); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("backup assertion is empty")
		}
		return nil, errors.Annotate(err, "cannot parse backup assertion")
	}
	if assertion == nil {
		return nil, errors.New("backup assertion is empty")
	}
	var extra Assertion
	if err := decoder.Decode(&extra); err != nil {
		if !errors.Is(err, io.EOF) {
			return nil, errors.Annotate(err, "cannot parse backup assertion")
		}
	} else {
		return nil, errors.New("backup assertion must contain exactly one YAML document")
	}
	if assertion.ChecksumFormat != corebackups.ChecksumFormatSHA256 {
		return nil, errors.Errorf(
			"unsupported backup assertion checksum format %q", assertion.ChecksumFormat)
	}
	// The checksum is the archive's SHA-256 digest in the canonical
	// prefixed form; the digest itself is 64 hexadecimal characters,
	// optionally carrying the "sha256-" prefix. An unusable checksum
	// must fail here: an empty expected digest would make recovery
	// skip the archive comparison altogether.
	decoded, err := hex.DecodeString(strings.TrimPrefix(assertion.Checksum, "sha256-"))
	if err != nil || len(decoded) != sha256.Size {
		return nil, errors.New(
			"backup assertion checksum must be a SHA-256 checksum: 64 hexadecimal characters")
	}
	return assertion, nil
}
