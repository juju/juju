// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

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
	"github.com/juju/juju/worker/uniter"
)

type unitInitializer struct {
	catacomb catacomb.Catacomb

	config  InitializeUnitParams
	unitTag names.UnitTag
}

// UnitInitType describes how to initilize the remote pod.
type UnitInitType string

const (
	// UnitInit initilizes the caas init container.
	UnitInit UnitInitType = "init"
	// UnitUpgrade re-initilizes the caas workload container.
	UnitUpgrade UnitInitType = "upgrade"
)

// InitializeUnitParams contains parameters and dependencies for initializing
// a unit.
type InitializeUnitParams struct {
	// UnitTag of the unit being initialized.
	UnitTag names.UnitTag

	// ProviderID is the pod-name or pod-uid
	ProviderID string

	// InitType of how to initilize the pod.
	InitType UnitInitType

	// Logger for the worker.
	Logger Logger

	// Paths provides CAAS operator paths.
	Paths Paths

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
	if p.ProviderID == "" {
		return errors.NotValidf("missing ProviderID")
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
	switch p.InitType {
	case UnitInit:
	case UnitUpgrade:
	default:
		return errors.NotValidf("invalid InitType %q", string(p.InitType))
	}
	return nil
}

// InitializeUnit with the charm and configuration.
func InitializeUnit(params InitializeUnitParams, cancel <-chan struct{}) error {
	if err := params.Validate(); err != nil {
		return errors.Trace(err)
	}

	params.Logger.Infof("started pod init on %q", params.UnitTag.Id())
	container := ""
	switch params.InitType {
	case UnitInit:
		container = caas.InitContainerName
	case UnitUpgrade:
		container = ""
	}

	initArgs := []string{"--unit", params.UnitTag.String()}

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
		PodName:       params.ProviderID,
		ContainerName: container,
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
			PodName:       params.ProviderID,
			ContainerName: container,
		},
	}, cancel)
	if err != nil {
		return errors.Trace(err)
	}
	initArgs = append(initArgs,
		"--charm-dir", tempCharmDir)

	if params.InitType == UnitInit {
		tempOperatorCacheFile, tempCACertFile, err := setupRemoteConfiguration(params, cancel, unitPaths, tempDir, container)
		if err != nil {
			return errors.Trace(err)
		}
		initArgs = append(initArgs,
			"--send", // Init container will wait for us to send the data.
			"--operator-file", tempOperatorCacheFile,
			"--operator-ca-cert-file", tempCACertFile)
	} else if params.InitType == UnitUpgrade {
		initArgs = append(initArgs,
			"--upgrade")
	}

	stdout := &bytes.Buffer{}
	err = params.ExecClient.Exec(exec.ExecParams{
		Commands:      append([]string{jujudPath, "caas-unit-init"}, initArgs...),
		PodName:       params.ProviderID,
		ContainerName: container,
		WorkingDir:    cmdutil.DataDir,
		Stdout:        stdout,
		Stderr:        stdout,
	}, cancel)
	if err != nil {
		return errors.Annotatef(err, "caas-unit-init failed: %s", string(stdout.Bytes()))
	}

	return nil
}

func setupRemoteConfiguration(params InitializeUnitParams, cancel <-chan struct{},
	unitPaths uniter.Paths, tempDir string, container string) (string, string, error) {
	tempCACertFile := filepath.Join(tempDir, caas.CACertFile)
	if err := params.WriteFile(tempCACertFile, []byte(params.OperatorInfo.CACert), 0644); err != nil {
		return "", "", errors.Trace(err)
	}
	err := params.ExecClient.Copy(exec.CopyParams{
		Src: exec.FileResource{
			Path: tempCACertFile,
		},
		Dest: exec.FileResource{
			Path:          tempCACertFile,
			PodName:       params.ProviderID,
			ContainerName: container,
		},
	}, cancel)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	serviceAddress := os.Getenv(provider.OperatorServiceIPEnvName)
	params.Logger.Debugf("operator service address: %v", serviceAddress)
	token, err := utils.RandomPassword()
	if err != nil {
		return "", "", errors.Trace(err)
	}
	clientInfo := caas.OperatorClientInfo{
		ServiceAddress: serviceAddress,
		Token:          token,
	}
	data, err := clientInfo.Marshal()
	if err != nil {
		return "", "", errors.Trace(err)
	}
	operatorCacheFile := filepath.Join(unitPaths.State.BaseDir, caas.OperatorClientInfoCacheFile)
	if err := params.WriteFile(operatorCacheFile, data, 0644); err != nil {
		return "", "", errors.Trace(err)
	}
	tempOperatorCacheFile := filepath.Join(tempDir, caas.OperatorClientInfoCacheFile)
	err = params.ExecClient.Copy(exec.CopyParams{
		Src: exec.FileResource{
			Path: operatorCacheFile,
		},
		Dest: exec.FileResource{
			Path:          tempOperatorCacheFile,
			PodName:       params.ProviderID,
			ContainerName: container,
		},
	}, cancel)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	return tempOperatorCacheFile, tempCACertFile, nil
}
