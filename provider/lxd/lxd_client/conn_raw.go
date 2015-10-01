// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_client

import (
	"crypto/x509"
	"io"
	"os"

	"github.com/lxc/lxd"
	"github.com/lxc/lxd/shared"
)

// These interfaces facilitate mocking out the LXD API during tests.
// See https://github.com/lxc/lxd/blob/master/client.go
// and https://github.com/lxc/lxd/blob/master/specs/rest-api.md.

type rawClientWrapper interface {
	rawServerMethods
	rawImageMethods
	rawAliasMethods
	rawContainerMethods
	rawProfileMethods
}

type rawServerMethods interface {
	Finger() error
	ServerStatus() (*shared.ServerState, error)
	GetServerConfigString() ([]string, error)
	SetServerConfig(key string, value string) (*lxd.Response, error)

	// connection
	WaitFor(waitURL string) (*shared.Operation, error)
	WaitForSuccess(waitURL string) error

	// auth
	UserAuthServerCert(name string, acceptCert bool) error
	CertificateList() ([]shared.CertInfo, error)
	AddMyCertToServer(pwd string) error
	CertificateAdd(cert *x509.Certificate, name string) error
	CertificateRemove(fingerprint string) error
	AmTrusted() bool
}

type rawImageMethods interface {
	ListImages() ([]shared.ImageInfo, error)
	GetImageInfo(image string) (*shared.ImageInfo, error)
	PutImageProperties(name string, p shared.ImageProperties) error

	CopyImage(image string, dest *lxd.Client, copy_aliases bool, aliases []string, public bool) error
	PostImage(imageFile string, rootfsFile string, properties []string, public bool, aliases []string) (string, error)
	ExportImage(image string, target string) (*lxd.Response, string, error)
	DeleteImage(image string) error

	ImageFromContainer(cname string, public bool, aliases []string, properties map[string]string) (string, error)
}

type rawAliasMethods interface {
	ListAliases() ([]shared.ImageAlias, error)
	IsAlias(alias string) (bool, error)

	GetAlias(alias string) string
	PostAlias(alias string, desc string, target string) error
	DeleteAlias(alias string) error
}

type rawContainerMethods interface {
	ListContainers() ([]shared.ContainerInfo, error)
	Rename(name string, newName string) (*lxd.Response, error)
	ContainerStatus(name string, showLog bool) (*shared.ContainerState, error)

	Init(name string, imgremote string, image string, profiles *[]string, ephem bool) (*lxd.Response, error)
	LocalCopy(source string, name string, config map[string]string, profiles []string) (*lxd.Response, error)
	MigrateTo(container string) (*lxd.Response, error)
	MigrateFrom(name string, operation string, secrets map[string]string, config map[string]string, profiles []string, baseImage string) (*lxd.Response, error)
	Action(name string, action shared.ContainerAction, timeout int, force bool) (*lxd.Response, error)
	Delete(name string) (*lxd.Response, error)

	// exec
	Exec(name string, cmd []string, env map[string]string, stdin *os.File, stdout *os.File, stderr *os.File) (int, error)

	// files
	PushFile(container string, p string, gid int, uid int, mode os.FileMode, buf io.ReadSeeker) error
	PullFile(container string, p string) (int, int, os.FileMode, io.ReadCloser, error)

	// config
	GetContainerConfig(container string) ([]string, error)
	SetContainerConfig(container, key, value string) (*lxd.Response, error)
	UpdateContainerConfig(container string, st shared.BriefContainerState) error

	// devices
	ContainerListDevices(container string) ([]string, error)
	ContainerDeviceDelete(container, devname string) (*lxd.Response, error)
	ContainerDeviceAdd(container, devname, devtype string, props []string) (*lxd.Response, error)

	// snapshots
	RestoreSnapshot(container string, snapshotName string, stateful bool) (*lxd.Response, error)
	Snapshot(container string, snapshotName string, stateful bool) (*lxd.Response, error)
	ListSnapshots(container string) ([]string, error)
}

type rawProfileMethods interface {
	ListProfiles() ([]string, error)

	ProfileCreate(p string) error
	PutProfile(name string, profile shared.ProfileConfig) error
	ProfileCopy(name, newname string, dest *lxd.Client) error
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
