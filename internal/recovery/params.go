// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package recovery

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/errors"
)

const (
	// ParamsFile is the recovery marker and populated parameters file.
	ParamsFile = agent.RecoveryParamsFile

	// ArchiveFile is the data-directory-relative path of the uploaded archive.
	ArchiveFile = "recovery/archive.tar.gz"

	// StartedFile records that database initialisation was claimed. It is
	// retained on failure so an interrupted initialisation is never
	// replayed.
	StartedFile = "recovery/initialisation-started"
)

// Params holds the recovery inputs delivered with the replacement instance.
type Params struct {
	ArchivePath  string                                `yaml:"archive-path"`
	SHA256       string                                `yaml:"sha256"`
	AgentVersion semversion.Number                     `yaml:"agent-version"`
	MachineName  string                                `yaml:"machine-name"`
	Node         instancecfg.StateInitializationParams `yaml:"-"`
	NodeConfig   string                                `yaml:"node-config"`
}

// Marshal returns the recovery parameters with the node configuration embedded.
func (p Params) Marshal() ([]byte, error) {
	data, err := p.Node.Marshal()
	if err != nil {
		return nil, errors.Capture(err)
	}
	p.NodeConfig = string(data)
	return yaml.Marshal(p)
}

// Unmarshal parses and validates the recovery parameters, including the
// embedded node configuration. The caller reads the parameters file once
// itself and decides what an empty file means: the agent-startup stage
// carries no parameters to validate.
func (p *Params) Unmarshal(data []byte) error {
	if err := yaml.Unmarshal(data, p); err != nil {
		return errors.Capture(err)
	}
	if err := p.Node.Unmarshal([]byte(p.NodeConfig)); err != nil {
		return errors.Capture(err)
	}
	sum := strings.TrimPrefix(p.SHA256, "sha256-")
	if p.ArchivePath == "" || len(sum) != 64 || p.AgentVersion == semversion.Zero ||
		p.MachineName == "" || p.Node.AgentVersion != p.AgentVersion {
		return errors.New("incomplete recovery parameters")
	}
	if _, err := hex.DecodeString(sum); err != nil {
		return errors.New("invalid recovery checksum")
	}
	if err := machine.Name(p.MachineName).Validate(); err != nil {
		return errors.Capture(err)
	}
	return nil
}

// ReadParams reads and validates the recovery parameters from dataDir.
func ReadParams(dataDir string) (Params, error) {
	data, err := os.ReadFile(filepath.Join(dataDir, ParamsFile))
	if err != nil {
		return Params{}, errors.Errorf("reading recovery parameters: %w", err)
	}
	var p Params
	if err := p.Unmarshal(data); err != nil {
		return Params{}, err
	}
	return p, nil
}

// IsRecovery reports whether dataDir contains recovery parameters. A missing
// file means ordinary startup; any regular file, empty or populated, marks a
// recovery target.
func IsRecovery(dataDir string) (bool, error) {
	return agent.IsRecovery(dataDir)
}

// HasMarker reports whether the marker exists and belongs to the archive.
func HasMarker(dataDir, name, checksum string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(dataDir, name))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, errors.Capture(err)
	}
	if strings.TrimSpace(string(data)) != checksum {
		return false, errors.New("recovery marker belongs to another archive")
	}
	return true, nil
}

// BeginInitialisation records exclusive ownership before database
// initialisation. The marker is retained on failure: a partially
// initialised database must never be replayed or opened by another
// initialiser.
func BeginInitialisation(dataDir, checksum string) error {
	filename := filepath.Join(dataDir, StartedFile)
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return errors.Capture(err)
	}
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errors.Errorf("claiming recovery initialisation: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(checksum + "\n"); err != nil {
		return errors.Capture(err)
	}
	if err := f.Sync(); err != nil {
		return errors.Capture(err)
	}
	dir, err := os.Open(filepath.Dir(filename))
	if err != nil {
		return errors.Capture(err)
	}
	defer dir.Close()
	return errors.Capture(dir.Sync())
}
