// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"time"

	"github.com/juju/description/v12"
)

// This file contains structures which implement the description package
// interfaces commonly used in model migration integration tests.

type ManifestBase struct {
	Name_          string
	Channel_       string
	Architectures_ []string
}

func (m ManifestBase) Name() string {
	return m.Name_
}
func (m ManifestBase) Channel() string {
	return m.Channel_
}
func (m ManifestBase) Architectures() []string {
	return m.Architectures_
}

type Relation struct {
	Name_          string
	Role_          string
	InterfaceName_ string
	Optional_      bool
	Limit_         int
	Scope_         string
}

func (r Relation) Name() string {
	return r.Name_
}

func (r Relation) Role() string {
	return r.Role_
}

func (r Relation) Interface() string {
	return r.InterfaceName_
}

func (r Relation) Optional() bool {
	return r.Optional_
}

func (r Relation) Limit() int {
	return r.Limit_
}

func (r Relation) Scope() string {
	return r.Scope_
}

type Storage struct {
	Name_        string
	Stype_       string
	Description_ string
	Shared_      bool
	Readonly_    bool
	CountMin_    int
	CountMax_    int
	MinimumSize_ int
	Location_    string
	Properties_  []string
}

func (s Storage) Name() string {
	return s.Name_
}

func (s Storage) Description() string {
	return s.Description_
}

func (s Storage) Type() string {
	return s.Stype_
}

func (s Storage) Shared() bool {
	return s.Shared_
}

func (s Storage) Readonly() bool {
	return s.Readonly_
}

func (s Storage) CountMin() int {
	return s.CountMin_
}

func (s Storage) CountMax() int {
	return s.CountMax_
}

func (s Storage) MinimumSize() int {
	return s.MinimumSize_
}

func (s Storage) Location() string {
	return s.Location_
}

func (s Storage) Properties() []string {
	return s.Properties_
}

type Device struct {
	Name_        string
	Description_ string
	Dtype_       string
	CountMin_    int
	CountMax_    int
}

func (d Device) Name() string {
	return d.Name_
}

func (d Device) Description() string {
	return d.Description_
}

func (d Device) Type() string {
	return d.Dtype_
}

func (d Device) CountMin() int {
	return d.CountMin_
}

func (d Device) CountMax() int {
	return d.CountMax_
}

type Container struct {
	Resource_ string
	Mounts_   []description.CharmMetadataContainerMount
	Uid_      *int
	Gid_      *int
}

func (c Container) Resource() string {
	return c.Resource_
}

func (c Container) Mounts() []description.CharmMetadataContainerMount {
	return c.Mounts_
}

func (c Container) Uid() *int {
	return c.Uid_
}

func (c Container) Gid() *int {
	return c.Gid_
}

type ContainerMount struct {
	Storage_  string
	Location_ string
}

func (cm ContainerMount) Storage() string {
	return cm.Storage_
}

func (cm ContainerMount) Location() string {
	return cm.Location_
}

type ResourceMeta struct {
	Name_        string
	Rtype_       string
	Description_ string
	Path_        string
}

func (r ResourceMeta) Name() string {
	return r.Name_
}

func (r ResourceMeta) Type() string {
	return r.Rtype_
}

func (r ResourceMeta) Description() string {
	return r.Description_
}

func (r ResourceMeta) Path() string {
	return r.Path_
}

type Config struct {
	ConfigType_   string
	DefaultValue_ any
	Description_  string
}

func (c Config) Type() string {
	return c.ConfigType_
}

func (c Config) Default() any {
	return c.DefaultValue_
}

func (c Config) Description() string {
	return c.Description_
}

type Action struct {
	Description_    string
	Parallel_       bool
	ExecutionGroup_ string
	Params_         map[string]any
}

func (a Action) Description() string {
	return a.Description_
}

func (a Action) Parallel() bool {
	return a.Parallel_
}

func (a Action) ExecutionGroup() string {
	return a.ExecutionGroup_
}

func (a Action) Parameters() map[string]any {
	return a.Params_
}

type ActionMessage struct {
	Timestamp_ time.Time
	Message_   string
}

func (a ActionMessage) Timestamp() time.Time {
	return a.Timestamp_
}

func (a ActionMessage) Message() string {
	return a.Message_
}
