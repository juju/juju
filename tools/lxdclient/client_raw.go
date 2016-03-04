// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"crypto/x509"
	"io"
	"os"

	"github.com/gorilla/websocket"
	"github.com/lxc/lxd"
	"github.com/lxc/lxd/shared"
)

// These interfaces facilitate mocking out the LXD API during tests.
// See https://github.com/lxc/lxd/blob/master/client.go
// and https://github.com/lxc/lxd/blob/master/specs/rest-api.md.

// TODO(ericsnow) Move this to a test suite.
var _ rawClientWrapperFull = (*lxd.Client)(nil)

// rawClientWrapperFull exposes all methods of lxd.Client.
type rawClientWrapperFull interface {
	rawServerMethods
	rawImageMethods
	rawAliasMethods
	rawContainerMethods
	rawProfileMethods
}

type rawServerMethods interface {
	// info
	Finger() error
	ServerStatus() (*shared.ServerState, error)

	// config
	GetServerConfigString() ([]string, error)
	SetServerConfig(key string, value string) (*lxd.Response, error)

	// connection
	WaitFor(waitURL string) (*shared.Operation, error)
	WaitForSuccess(waitURL string) error

	// auth
	AmTrusted() bool
}

type rawCertMethods interface {
	AddMyCertToServer(pwd string) error
	CertificateList() ([]shared.CertInfo, error)
	CertificateAdd(cert *x509.Certificate, name string) error
	CertificateRemove(fingerprint string) error
}

type rawImageMethods interface {
	// info/meta
	ListImages() ([]shared.ImageInfo, error)
	GetImageInfo(image string) (*shared.ImageInfo, error)
	//PutImageProperties(name string, p shared.ImageProperties) error

	// image data (create, upload, download, destroy)
	CopyImage(image string, dest *lxd.Client, copy_aliases bool, aliases []string, public bool, progressHandler func(string)) error
	ImageFromContainer(cname string, public bool, aliases []string, properties map[string]string) (string, error)
	PostImage(imageFile string, rootfsFile string, properties []string, public bool, aliases []string) (string, error)
	ExportImage(image string, target string) (*lxd.Response, string, error)
	DeleteImage(image string) error
}

type rawAliasMethods interface {
	// info
	ListAliases() (shared.ImageAliases, error)
	IsAlias(alias string) (bool, error)

	// alias data (upload, download, destroy)
	PostAlias(alias string, desc string, target string) error
	GetAlias(alias string) string
	DeleteAlias(alias string) error
}

type rawContainerMethods interface {
	// info/meta
	ListContainers() ([]shared.ContainerInfo, error)
	//Rename(name string, newName string) (*lxd.Response, error)
	ContainerState(name string) (*shared.ContainerState, error)

	// container data (create, actions, destroy)
	Init(name string, imgremote string, image string, profiles *[]string, config map[string]string, ephem bool) (*lxd.Response, error)
	LocalCopy(source string, name string, config map[string]string, profiles []string, ephemeral bool) (*lxd.Response, error)
	MigrateFrom(name string, operation string, certificate string, secrets map[string]string, architecture string, config map[string]string, devices shared.Devices, profiles []string, baseImage string, ephemeral bool) (*lxd.Response, error)
	Action(name string, action shared.ContainerAction, timeout int, force bool) (*lxd.Response, error)
	Delete(name string) (*lxd.Response, error)

	// exec
	Exec(name string, cmd []string, env map[string]string, stdin io.ReadCloser, stdout io.WriteCloser, stderr io.WriteCloser, controlHandler func(*lxd.Client, *websocket.Conn)) (int, error)

	// files
	PushFile(container string, p string, gid int, uid int, mode os.FileMode, buf io.ReadSeeker) error
	PullFile(container string, p string) (int, int, os.FileMode, io.ReadCloser, error)

	// config
	GetContainerConfig(container string) ([]string, error)
	SetContainerConfig(container, key, value string) error
	UpdateContainerConfig(container string, st shared.BriefContainerInfo) error

	// devices
	ContainerListDevices(container string) ([]string, error)
	ContainerDeviceDelete(container, devname string) (*lxd.Response, error)
	ContainerDeviceAdd(container, devname, devtype string, props []string) (*lxd.Response, error)

	// snapshots
	RestoreSnapshot(container string, snapshotName string, stateful bool) (*lxd.Response, error)
	Snapshot(container string, snapshotName string, stateful bool) (*lxd.Response, error)
	ListSnapshots(container string) ([]shared.SnapshotInfo, error)
}

type rawProfileMethods interface {
	// info
	ListProfiles() ([]string, error)

	// profile data (create, upload, destroy)
	ProfileCreate(p string) error
	ProfileCopy(name, newname string, dest *lxd.Client) error
	PutProfile(name string, profile shared.ProfileConfig) error
	ProfileDelete(p string) error

	// apply
	ApplyProfile(container, profile string) (*lxd.Response, error)

	// config
	ProfileConfig(name string) (*shared.ProfileConfig, error)
	GetProfileConfig(profile string) (map[string]string, error)
	SetProfileConfigItem(profile, key, value string) error

	// devices
	ProfileListDevices(profile string) ([]string, error)
	ProfileDeviceDelete(profile, devname string) (*lxd.Response, error)
	ProfileDeviceAdd(profile, devname, devtype string, props []string) (*lxd.Response, error)
}
