// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/api/resource"
)

// FileSet defines a set of files to mount
// into the container.
type FileSet struct {
	Name         string       `json:"name" yaml:"name"`
	MountPath    string       `json:"mountPath" yaml:"mountPath"`
	VolumeSource volumeSource `json:",inline" yaml:",inline"`
}

type volumeSource struct {
	Files     map[string]string `json:"files" yaml:"files"`
	HostPath  *hostPathVol      `json:"hostPath" yaml:"hostPath"`
	EmptyDir  *emptyDirVol      `json:"emptyDir" yaml:"emptyDir"`
	Secret    *secretVol        `json:"secret" yaml:"secret"`
	ConfigMap *configMapVol     `json:"configMap" yaml:"configMap"`
}

type hostPathVol struct {
	Path string `json:"path" yaml:"path"`
	Type string `json:"type,omitempty" yaml:"type,omitempty"`
}

type emptyDirVol struct {
	Medium    string             `json:"medium,omitempty" yaml:"medium,omitempty"`
	SizeLimit *resource.Quantity `json:"sizeLimit,omitempty" yaml:"sizeLimit,omitempty"`
}

type secretVol struct {
}

type configMapVol struct {
}

// Validate validates FileSet.
func (fs *FileSet) Validate() error {
	if fs.Name == "" {
		return errors.New("file set name is missing")
	}
	if fs.MountPath == "" {
		return errors.Errorf("mount path is missing for file set %q", fs.Name)
	}
	return nil
}
