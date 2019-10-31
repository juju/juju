// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitinit

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/juju/names.v3"
	"gopkg.in/juju/worker.v1/catacomb"

	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/caas/kubernetes/provider/exec"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/worker/caasoperator"
	"github.com/juju/juju/worker/uniter"
)

type unitInitializer struct {
	catacomb catacomb.Catacomb

	config  InitializeUnitParams
	unitTag names.UnitTag
}

// InitializeUnitParams contains parameters and dependencies for initializing
// a unit.
type InitializeUnitParams struct {
	// UnitTag of the unit being initialized.
	UnitTag names.UnitTag

	// Logger for the worker.
	Logger Logger

	// UnitProviderIDFunc returns the ProviderID for the given unit.
	UnitProviderIDFunc func(unit names.UnitTag) (string, error)

	// Paths provides CAAS operator paths.
	Paths caasoperator.Paths

	// OperatorInfo contains serving information such as Certs and PrivateKeys.
	OperatorInfo caas.OperatorInfo

	// ExecClient is used for initilizing units.
	ExecClient exec.Executor

	// WriteFile is used to write files to the local state.
	WriteFile func(string, []byte, os.FileMode) error

	// TempDir is used for creating a temporary directory.
	TempDir func(string, string) (string, error)
}

// Validate InitializeUnitParams
func (p InitializeUnitParams) Validate() error {
	if p.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	if p.UnitProviderIDFunc == nil {
		return errors.NotValidf("missing UnitProviderIDFunc")
	}
	if p.ExecClient == nil {
		return errors.NotValidf("missing ExecClient")
	}
	if p.WriteFile == nil {
		return errors.NotValidf("missing WriteFile")
	}
	if p.TempDir == nil {
		return errors.NotValidf("missing TempDir")
	}
	return nil
}

// InitializeUnit with the charm and configuration.
func InitializeUnit(params InitializeUnitParams, cancel <-chan struct{}) error {
	if err := params.Validate(); err != nil {
		return errors.Trace(err)
	}

	params.Logger.Infof("started pod init on %q", params.UnitTag.Id())
	providerID, err := params.UnitProviderIDFunc(params.UnitTag)
	if err != nil {
		return errors.Trace(err)
	}

	rootToolsDir := tools.ToolsDir(cmdutil.DataDir, "")
	jujudPath := filepath.Join(rootToolsDir, "jujud")
	unitPaths := uniter.NewPaths(cmdutil.DataDir, params.UnitTag, nil)
	operatorPaths := params.Paths
	tempDir, err := params.TempDir(os.TempDir(), params.UnitTag.String())
	if err != nil {
		return errors.Annotatef(err, "creating temp directory")
	}

	err = params.ExecClient.Exec(exec.ExecParams{
		Commands:      []string{"mkdir", "-p", tempDir},
		PodName:       providerID,
		ContainerName: caas.InitContainerName,
		Stdout:        &bytes.Buffer{},
		Stderr:        &bytes.Buffer{},
	}, cancel)
	if err != nil {
		return errors.Trace(err)
	}

	tempCharmDir := filepath.Join(tempDir, "charm")
	err = params.ExecClient.Copy(exec.CopyParams{
		Src: exec.FileResource{
			Path: operatorPaths.State.CharmDir,
		},
		Dest: exec.FileResource{
			Path:          tempDir,
			PodName:       providerID,
			ContainerName: caas.InitContainerName,
		},
	}, cancel)
	if err != nil {
		return errors.Trace(err)
	}

	tempCACertFile := filepath.Join(tempDir, caas.CACertFile)
	if err := params.WriteFile(tempCACertFile, []byte(params.OperatorInfo.CACert), 0644); err != nil {
		return errors.Trace(err)
	}
	err = params.ExecClient.Copy(exec.CopyParams{
		Src: exec.FileResource{
			Path: tempCACertFile,
		},
		Dest: exec.FileResource{
			Path:          tempCACertFile,
			PodName:       providerID,
			ContainerName: caas.InitContainerName,
		},
	}, cancel)
	if err != nil {
		return errors.Trace(err)
	}

	serviceAddress := os.Getenv(provider.OperatorServiceIPEnvName)
	params.Logger.Debugf("operator service address: %v", serviceAddress)
	token, err := utils.RandomPassword()
	if err != nil {
		return errors.Trace(err)
	}
	clientInfo := caas.OperatorClientInfo{
		ServiceAddress: serviceAddress,
		Token:          token,
	}
	data, err := clientInfo.Marshal()
	if err != nil {
		return errors.Trace(err)
	}
	operatorCacheFile := filepath.Join(unitPaths.State.BaseDir, caas.OperatorClientInfoCacheFile)
	if err := params.WriteFile(operatorCacheFile, data, 0644); err != nil {
		return errors.Trace(err)
	}
	tempOperatorCacheFile := filepath.Join(tempDir, caas.OperatorClientInfoCacheFile)
	err = params.ExecClient.Copy(exec.CopyParams{
		Src: exec.FileResource{
			Path: operatorCacheFile,
		},
		Dest: exec.FileResource{
			Path:          tempOperatorCacheFile,
			PodName:       providerID,
			ContainerName: caas.InitContainerName,
		},
	}, cancel)
	if err != nil {
		return errors.Trace(err)
	}

	stdout := &bytes.Buffer{}
	err = params.ExecClient.Exec(exec.ExecParams{
		Commands: []string{jujudPath, "caas-unit-init", "--send",
			"--unit", params.UnitTag.String(),
			"--charm-dir", tempCharmDir,
			"--operator-file", tempOperatorCacheFile,
			"--operator-ca-cert-file", tempCACertFile,
		},
		PodName:       providerID,
		ContainerName: caas.InitContainerName,
		WorkingDir:    cmdutil.DataDir,
		Stdout:        stdout,
		Stderr:        stdout,
	}, cancel)
	if err != nil {
		return errors.Annotatef(err, "caas-unit-init failed: %s", string(stdout.Bytes()))
	}

	return nil
}
