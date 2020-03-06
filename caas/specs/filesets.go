// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"fmt"
	"reflect"

	"github.com/juju/errors"
	"k8s.io/apimachinery/pkg/api/resource"
)

// FileSetV2 defines a set of files to mount
// into the container.
type FileSetV2 struct {
	Name      string            `json:"name" yaml:"name"`
	MountPath string            `json:"mountPath" yaml:"mountPath"`
	Files     map[string]string `json:"files" yaml:"files"`
}

// Validate validates FileSetV2.
func (fs *FileSetV2) Validate() error {
	if fs.Name == "" {
		return errors.New("file set name is missing")
	}
	if fs.MountPath == "" {
		return errors.Errorf("mount path is missing for file set %q", fs.Name)
	}
	return nil
}

// FileSet defines a set of files to mount
// into the container.
type FileSet struct {
	VolumeSource `json:",inline" yaml:",inline"`
	Name         string `json:"name" yaml:"name"`
	MountPath    string `json:"mountPath" yaml:"mountPath"`
}

// Equal compares if two FileSet are same.
func (fs FileSet) Equal(another FileSet) bool {
	return reflect.DeepEqual(fs, another)
}

// EqualVolume checks if two fileset definitions will create same volume.
func (fs FileSet) EqualVolume(another FileSet) bool {
	if fs.Name != another.Name {
		return false
	}
	if !reflect.DeepEqual(fs.VolumeSource, another.VolumeSource) {
		return false
	}
	return true
}

// Validate validates FileSet.
func (fs *FileSet) Validate() error {
	if fs.Name == "" {
		return errors.New("file set name is missing")
	}
	if fs.MountPath == "" {
		return errors.Errorf("mount path is missing for file set %q", fs.Name)
	}
	if err := fs.VolumeSource.Validate(fs.Name); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// VolumeSource represents the source of a volume to mount.
type VolumeSource struct {
	Files     []File          `json:"files" yaml:"files"`
	HostPath  *HostPathVol    `json:"hostPath" yaml:"hostPath"`
	EmptyDir  *EmptyDirVol    `json:"emptyDir" yaml:"emptyDir"`
	ConfigMap *ResourceRefVol `json:"configMap" yaml:"configMap"`
	Secret    *ResourceRefVol `json:"secret" yaml:"secret"`
}

type validator interface {
	Validate(string) error
}

// Validate validates VolumeSource.
func (vs VolumeSource) Validate(name string) error {
	nonNilSource := 0
	if len(vs.Files) > 0 {
		nonNilSource++
		for _, f := range vs.Files {
			if err := f.Validate(name); err != nil {
				return errors.Trace(err)
			}
		}
	}
	if vs.HostPath != nil {
		nonNilSource++
		if err := vs.HostPath.Validate(name); err != nil {
			return errors.Trace(err)
		}
	}
	if vs.EmptyDir != nil {
		nonNilSource++
		if err := vs.EmptyDir.Validate(name); err != nil {
			return errors.Trace(err)
		}
	}
	if vs.Secret != nil {
		nonNilSource++
		if err := vs.Secret.Validate(name); err != nil {
			return errors.Trace(err)
		}
	}
	if vs.ConfigMap != nil {
		nonNilSource++
		if err := vs.ConfigMap.Validate(name); err != nil {
			return errors.Trace(err)
		}
	}
	if nonNilSource == 0 {
		return errors.NewNotValid(nil, fmt.Sprintf("file set %q requires volume source", name))
	}
	if nonNilSource > 1 {
		return errors.NewNotValid(nil, fmt.Sprintf("file set %q can only have one volume source", name))
	}
	return nil
}

// File describes a file to mount into a pod.
type File struct {
	Path    string `json:"path" yaml:"path"`
	Content string `json:"content" yaml:"content"`
	Mode    *int32 `json:"mode,omitempty" yaml:"mode,omitempty"`
}

// Validate validates File.
func (f *File) Validate(name string) error {
	if f.Path == "" {
		return errors.Errorf("Path is missing for %q", name)
	}
	if f.Content == "" {
		return errors.Errorf("Content is missing for %q", name)
	}
	return nil
}

// FileRef describes a file to mount into a pod.
type FileRef struct {
	Key  string `json:"key" yaml:"key"`
	Path string `json:"path" yaml:"path"`
	Mode *int32 `json:"mode,omitempty" yaml:"mode,omitempty"`
}

// Validate validates FileRef.
func (f *FileRef) Validate(name string) error {
	if f.Key == "" {
		return errors.Errorf("Key is missing for %q", name)
	}
	if f.Path == "" {
		return errors.Errorf("Path is missing for %q", name)
	}
	return nil
}

// HostPathVol represents a host path mapped into a pod.
type HostPathVol struct {
	Path string `json:"path" yaml:"path"`
	Type string `json:"type,omitempty" yaml:"type,omitempty"`
}

// Validate validates HostPathVol.
func (hpv *HostPathVol) Validate(name string) error {
	if hpv.Path == "" {
		return errors.Errorf("Path is missing for %q", name)
	}
	return nil
}

// EmptyDirVol represents an empty directory for a pod.
type EmptyDirVol struct {
	Medium    string             `json:"medium,omitempty" yaml:"medium,omitempty"`
	SizeLimit *resource.Quantity `json:"sizeLimit,omitempty" yaml:"sizeLimit,omitempty"`
}

// Validate validates EmptyDirVol.
func (edv *EmptyDirVol) Validate(name string) error {
	return nil
}

// ResourceRefVol reprents a configmap or secret source could be referenced by a volume.
type ResourceRefVol struct {
	Name        string    `json:"name" yaml:"name"`
	Files       []FileRef `json:"files,omitempty" yaml:"files,omitempty"`
	DefaultMode *int32    `json:"defaultMode,omitempty" yaml:"defaultMode,omitempty"`
}

// Validate validates ResourceRefVol.
func (rrv *ResourceRefVol) Validate(name string) error {
	if rrv.Name == "" {
		return errors.Errorf("Name is missing for %q", name)
	}
	for _, f := range rrv.Files {
		if err := f.Validate(name); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
