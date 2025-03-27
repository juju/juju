// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/schema"
	"github.com/juju/utils/v4"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/charm/assumes"
	"github.com/juju/juju/internal/charm/hooks"
	"github.com/juju/juju/internal/charm/resource"
)

// RelationScope describes the scope of a relation.
type RelationScope string

// Note that schema doesn't support custom string types,
// so when we use these values in a schema.Checker,
// we must store them as strings, not RelationScopes.

const (
	ScopeGlobal    RelationScope = "global"
	ScopeContainer RelationScope = "container"
)

// RelationRole defines the role of a relation.
type RelationRole string

const (
	RoleProvider RelationRole = "provider"
	RoleRequirer RelationRole = "requirer"
	RolePeer     RelationRole = "peer"
)

// StorageType defines a storage type.
type StorageType string

const (
	StorageBlock      StorageType = "block"
	StorageFilesystem StorageType = "filesystem"
)

// Storage represents a charm's storage requirement.
type Storage struct {
	// Name is the name of the store.
	//
	// Name has no default, and must be specified.
	Name string

	// Description is a description of the store.
	//
	// Description has no default, and is optional.
	Description string

	// Type is the storage type: filesystem or block-device.
	//
	// Type has no default, and must be specified.
	Type StorageType

	// Shared indicates that the storage is shared between all units of
	// an application deployed from the charm. It is an error to attempt to
	// assign non-shareable storage to a "shared" storage requirement.
	//
	// Shared defaults to false.
	Shared bool

	// ReadOnly indicates that the storage should be made read-only if
	// possible. If the storage cannot be made read-only, Juju will warn
	// the user.
	//
	// ReadOnly defaults to false.
	ReadOnly bool

	// CountMin is the number of storage instances that must be attached
	// to the charm for it to be useful; the charm will not install until
	// this number has been satisfied. This must be a non-negative number.
	//
	// CountMin defaults to 1 for singleton stores.
	CountMin int

	// CountMax is the largest number of storage instances that can be
	// attached to the charm. If CountMax is -1, then there is no upper
	// bound.
	//
	// CountMax defaults to 1 for singleton stores.
	CountMax int

	// MinimumSize is the minimum size of store that the charm needs to
	// work at all. This is not a recommended size or a comfortable size
	// or a will-work-well size, just a bare minimum below which the charm
	// is going to break.
	// MinimumSize requires a unit, one of MGTPEZY, and is stored as MiB.
	//
	// There is no default MinimumSize; if left unspecified, a provider
	// specific default will be used, typically 1GB for block storage.
	MinimumSize uint64

	// Location is the mount location for filesystem stores. For multi-
	// stores, the location acts as the parent directory for each mounted
	// store.
	//
	// Location has no default, and is optional.
	Location string

	// Properties allow the charm author to characterise the relative storage
	// performance requirements and sensitivities for each store.
	// eg “transient” is used to indicate that non persistent storage is acceptable,
	// such as tmpfs or ephemeral instance disks.
	//
	// Properties has no default, and is optional.
	Properties []string
}

// DeviceType defines a device type.
type DeviceType string

// Device represents a charm's device requirement (GPU for example).
type Device struct {
	// Name is the name of the device.
	Name string

	// Description is a description of the device.
	Description string

	// Type is the device type.
	// currently supported types are
	// - gpu
	// - nvidia.com/gpu
	// - amd.com/gpu
	Type DeviceType

	// CountMin is the min number of devices that the charm requires.
	CountMin int64

	// CountMax is the max number of devices that the charm requires.
	CountMax int64
}

// Relation represents a single relation defined in the charm
// metadata.yaml file.
type Relation struct {
	Name      string
	Role      RelationRole
	Interface string
	Optional  bool
	Limit     int
	Scope     RelationScope
}

// ImplementedBy returns whether the relation is implemented by the supplied charm.
func (r Relation) ImplementedBy(meta *Meta) bool {
	if r.IsImplicit() {
		return true
	}
	var m map[string]Relation
	switch r.Role {
	case RoleProvider:
		m = meta.Provides
	case RoleRequirer:
		m = meta.Requires
	case RolePeer:
		m = meta.Peers
	default:
		panic(errors.Errorf("unknown relation role %q", r.Role))
	}
	rel, found := m[r.Name]
	if !found {
		return false
	}
	if rel.Interface == r.Interface {
		switch r.Scope {
		case ScopeGlobal:
			return rel.Scope != ScopeContainer
		case ScopeContainer:
			return true
		default:
			panic(errors.Errorf("unknown relation scope %q", r.Scope))
		}
	}
	return false
}

// IsImplicit returns whether the relation is supplied by juju itself,
// rather than by a charm.
func (r Relation) IsImplicit() bool {
	return (r.Name == "juju-info" &&
		r.Interface == "juju-info" &&
		r.Role == RoleProvider)
}

// RunAs defines which user to run a certain process as.
type RunAs string

const (
	RunAsDefault RunAs = ""
	RunAsRoot    RunAs = "root"
	RunAsSudoer  RunAs = "sudoer"
	RunAsNonRoot RunAs = "non-root"
)

// Meta represents all the known content that may be defined
// within a charm's metadata.yaml file.
type Meta struct {
	Name           string                   `json:"Name"`
	Summary        string                   `json:"Summary"`
	Description    string                   `json:"Description"`
	Subordinate    bool                     `json:"Subordinate"`
	Provides       map[string]Relation      `json:"Provides,omitempty"`
	Requires       map[string]Relation      `json:"Requires,omitempty"`
	Peers          map[string]Relation      `json:"Peers,omitempty"`
	ExtraBindings  map[string]ExtraBinding  `json:"ExtraBindings,omitempty"`
	Categories     []string                 `json:"Categories,omitempty"`
	Tags           []string                 `json:"Tags,omitempty"`
	Storage        map[string]Storage       `json:"Storage,omitempty"`
	Devices        map[string]Device        `json:"Devices,omitempty"`
	Resources      map[string]resource.Meta `json:"Resources,omitempty"`
	Terms          []string                 `json:"Terms,omitempty"`
	MinJujuVersion semversion.Number        `json:"min-juju-version,omitempty"`

	// v2
	Containers map[string]Container    `json:"containers,omitempty" yaml:"containers,omitempty"`
	Assumes    *assumes.ExpressionTree `json:"assumes,omitempty" yaml:"assumes,omitempty"`
	CharmUser  RunAs                   `json:"charm-user,omitempty" yaml:"charm-user,omitempty"`
}

// Container specifies the possible systems it supports and mounts it wants.
type Container struct {
	Resource string  `json:"resource,omitempty" yaml:"resource,omitempty"`
	Mounts   []Mount `json:"mounts,omitempty" yaml:"mounts,omitempty"`
	Uid      *int    `json:"uid,omitempty" yaml:"uid,omitempty"`
	Gid      *int    `json:"gid,omitempty" yaml:"gid,omitempty"`
}

// Mount allows a container to mount a storage filesystem from the storage top-level directive.
type Mount struct {
	Storage  string `json:"storage,omitempty" yaml:"storage,omitempty"`
	Location string `json:"location,omitempty" yaml:"location,omitempty"`
}

func generateRelationHooks(relName string, allHooks map[string]bool) {
	for _, hookName := range hooks.RelationHooks() {
		allHooks[fmt.Sprintf("%s-%s", relName, hookName)] = true
	}
}

func generateContainerHooks(containerName string, allHooks map[string]bool) {
	// Containers using pebble trigger workload hooks.
	for _, hookName := range hooks.WorkloadHooks() {
		allHooks[fmt.Sprintf("%s-%s", containerName, hookName)] = true
	}
}

func generateStorageHooks(storageName string, allHooks map[string]bool) {
	for _, hookName := range hooks.StorageHooks() {
		allHooks[fmt.Sprintf("%s-%s", storageName, hookName)] = true
	}
}

// Hooks returns a map of all possible valid hooks, taking relations
// into account. It's a map to enable fast lookups, and the value is
// always true.
func (m Meta) Hooks() map[string]bool {
	allHooks := make(map[string]bool)
	// Unit hooks
	for _, hookName := range hooks.UnitHooks() {
		allHooks[string(hookName)] = true
	}
	// Secret hooks
	for _, hookName := range hooks.SecretHooks() {
		allHooks[string(hookName)] = true
	}
	// Relation hooks
	for hookName := range m.Provides {
		generateRelationHooks(hookName, allHooks)
	}
	for hookName := range m.Requires {
		generateRelationHooks(hookName, allHooks)
	}
	for hookName := range m.Peers {
		generateRelationHooks(hookName, allHooks)
	}
	for storageName := range m.Storage {
		generateStorageHooks(storageName, allHooks)
	}
	for containerName := range m.Containers {
		generateContainerHooks(containerName, allHooks)
	}
	return allHooks
}

// Used for parsing Categories and Tags.
func parseStringList(list interface{}) []string {
	if list == nil {
		return nil
	}
	slice := list.([]interface{})
	result := make([]string, 0, len(slice))
	for _, elem := range slice {
		result = append(result, elem.(string))
	}
	return result
}

var validTermName = regexp.MustCompile(`^[a-z](-?[a-z0-9]+)+$`)

// TermsId represents a single term id. The term can either be owned
// or "public" (meaning there is no owner).
// The Revision starts at 1. Therefore a value of 0 means the revision
// is unset.
type TermsId struct {
	Tenant   string
	Owner    string
	Name     string
	Revision int
}

// Validate returns an error if the Term contains invalid data.
func (t *TermsId) Validate() error {
	if t.Tenant != "" && t.Tenant != "cs" {
		if !validTermName.MatchString(t.Tenant) {
			return errors.Errorf("wrong term tenant format %q", t.Tenant)
		}
	}
	if t.Owner != "" && !names.IsValidUser(t.Owner) {
		return errors.Errorf("wrong owner format %q", t.Owner)
	}
	if !validTermName.MatchString(t.Name) {
		return errors.Errorf("wrong term name format %q", t.Name)
	}
	if t.Revision < 0 {
		return errors.Errorf("negative term revision")
	}
	return nil
}

// String returns the term in canonical form.
// This would be one of:
//
//	tenant:owner/name/revision
//	tenant:name
//	owner/name/revision
//	owner/name
//	name/revision
//	name
func (t *TermsId) String() string {
	id := make([]byte, 0, len(t.Tenant)+1+len(t.Owner)+1+len(t.Name)+4)
	if t.Tenant != "" {
		id = append(id, t.Tenant...)
		id = append(id, ':')
	}
	if t.Owner != "" {
		id = append(id, t.Owner...)
		id = append(id, '/')
	}
	id = append(id, t.Name...)
	if t.Revision != 0 {
		id = append(id, '/')
		id = strconv.AppendInt(id, int64(t.Revision), 10)
	}
	return string(id)
}

// ParseTerm takes a termID as a string and parses it into a Term.
// A complete term is in the form:
// tenant:owner/name/revision
// This function accepts partially specified identifiers
// typically in one of the following forms:
// name
// owner/name
// owner/name/27 # Revision 27
// name/283 # Revision 283
// cs:owner/name # Tenant cs
func ParseTerm(s string) (*TermsId, error) {
	tenant := ""
	termid := s
	if t := strings.SplitN(s, ":", 2); len(t) == 2 {
		tenant = t[0]
		termid = t[1]
	}

	tokens := strings.Split(termid, "/")
	var term TermsId
	switch len(tokens) {
	case 1: // "name"
		term = TermsId{
			Tenant: tenant,
			Name:   tokens[0],
		}
	case 2: // owner/name or name/123
		termRevision, err := strconv.Atoi(tokens[1])
		if err != nil { // owner/name
			term = TermsId{
				Tenant: tenant,
				Owner:  tokens[0],
				Name:   tokens[1],
			}
		} else { // name/123
			term = TermsId{
				Tenant:   tenant,
				Name:     tokens[0],
				Revision: termRevision,
			}
		}
	case 3: // owner/name/123
		termRevision, err := strconv.Atoi(tokens[2])
		if err != nil {
			return nil, errors.Errorf("invalid revision number %q %v", tokens[2], err)
		}
		term = TermsId{
			Tenant:   tenant,
			Owner:    tokens[0],
			Name:     tokens[1],
			Revision: termRevision,
		}
	default:
		return nil, errors.Errorf("unknown term id format %q", s)
	}
	if err := term.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return &term, nil
}

// ReadMeta reads the content of a metadata.yaml file and returns
// its representation.
// The data has verified as unambiguous, but not validated.
func ReadMeta(r io.Reader) (*Meta, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var meta Meta
	err = yaml.Unmarshal(data, &meta)
	if err != nil {
		return nil, err
	}
	return &meta, nil
}

// UnmarshalYAML
func (meta *Meta) UnmarshalYAML(f func(interface{}) error) error {
	raw := make(map[interface{}]interface{})
	err := f(&raw)
	if err != nil {
		return err
	}

	if err := ensureUnambiguousFormat(raw); err != nil {
		return err
	}

	v, err := charmSchema.Coerce(raw, nil)
	if err != nil {
		return errors.New("metadata: " + err.Error())
	}

	m := v.(map[string]interface{})
	meta1, err := parseMeta(m)
	if err != nil {
		return err
	}

	*meta = *meta1

	// Assumes blocks have their own dedicated parser so we need to invoke
	// it here and attach the resulting expression tree (if any) to the
	// metadata
	var assumesBlock = struct {
		Assumes *assumes.ExpressionTree `yaml:"assumes"`
	}{}
	if err := f(&assumesBlock); err != nil {
		return err
	}
	meta.Assumes = assumesBlock.Assumes

	return nil
}

func parseMeta(m map[string]interface{}) (*Meta, error) {
	var meta Meta
	var err error

	meta.Name = m["name"].(string)
	// Schema decodes as int64, but the int range should be good
	// enough for revisions.
	meta.Summary = m["summary"].(string)
	meta.Description = m["description"].(string)
	meta.Provides = parseRelations(m["provides"], RoleProvider)
	meta.Requires = parseRelations(m["requires"], RoleRequirer)
	meta.Peers = parseRelations(m["peers"], RolePeer)
	if meta.ExtraBindings, err = parseMetaExtraBindings(m["extra-bindings"]); err != nil {
		return nil, err
	}
	meta.Categories = parseStringList(m["categories"])
	meta.Tags = parseStringList(m["tags"])
	if subordinate := m["subordinate"]; subordinate != nil {
		meta.Subordinate = subordinate.(bool)
	}
	meta.Storage = parseStorage(m["storage"])
	meta.Devices = parseDevices(m["devices"])
	if err != nil {
		return nil, err
	}
	meta.MinJujuVersion, err = parseMinJujuVersion(m["min-juju-version"])
	if err != nil {
		return nil, err
	}
	meta.Terms = parseStringList(m["terms"])

	meta.Resources, err = parseMetaResources(m["resources"])
	if err != nil {
		return nil, err
	}

	// v2 parsing
	meta.Containers, err = parseContainers(m["containers"], meta.Resources, meta.Storage)
	if err != nil {
		return nil, errors.Annotatef(err, "parsing containers")
	}
	meta.CharmUser, err = parseCharmUser(m["charm-user"])
	if err != nil {
		return nil, errors.Annotatef(err, "parsing charm-user")
	}
	return &meta, nil
}

// MarshalYAML implements yaml.Marshaler (yaml.v2).
// It is recommended to call Check() before calling this method,
// otherwise you make get metadata which is not v1 nor v2 format.
func (m Meta) MarshalYAML() (interface{}, error) {
	var minver string
	if m.MinJujuVersion != semversion.Zero {
		minver = m.MinJujuVersion.String()
	}

	return struct {
		Name           string                           `yaml:"name"`
		Summary        string                           `yaml:"summary"`
		Description    string                           `yaml:"description"`
		Provides       map[string]marshaledRelation     `yaml:"provides,omitempty"`
		Requires       map[string]marshaledRelation     `yaml:"requires,omitempty"`
		Peers          map[string]marshaledRelation     `yaml:"peers,omitempty"`
		ExtraBindings  map[string]interface{}           `yaml:"extra-bindings,omitempty"`
		Categories     []string                         `yaml:"categories,omitempty"`
		Tags           []string                         `yaml:"tags,omitempty"`
		Subordinate    bool                             `yaml:"subordinate,omitempty"`
		Storage        map[string]Storage               `yaml:"storage,omitempty"`
		Devices        map[string]Device                `yaml:"devices,omitempty"`
		Terms          []string                         `yaml:"terms,omitempty"`
		MinJujuVersion string                           `yaml:"min-juju-version,omitempty"`
		Resources      map[string]marshaledResourceMeta `yaml:"resources,omitempty"`
		Containers     map[string]marshaledContainer    `yaml:"containers,omitempty"`
		Assumes        *assumes.ExpressionTree          `yaml:"assumes,omitempty"`
	}{
		Name:           m.Name,
		Summary:        m.Summary,
		Description:    m.Description,
		Provides:       marshaledRelations(m.Provides),
		Requires:       marshaledRelations(m.Requires),
		Peers:          marshaledRelations(m.Peers),
		ExtraBindings:  marshaledExtraBindings(m.ExtraBindings),
		Categories:     m.Categories,
		Tags:           m.Tags,
		Subordinate:    m.Subordinate,
		Storage:        m.Storage,
		Devices:        m.Devices,
		Terms:          m.Terms,
		MinJujuVersion: minver,
		Resources:      marshaledResources(m.Resources),
		Containers:     marshaledContainers(m.Containers),
		Assumes:        m.Assumes,
	}, nil
}

type marshaledResourceMeta struct {
	Path        string `yaml:"filename"` // TODO(ericsnow) Change to "path"?
	Type        string `yaml:"type,omitempty"`
	Description string `yaml:"description,omitempty"`
}

func marshaledResources(rs map[string]resource.Meta) map[string]marshaledResourceMeta {
	rs1 := make(map[string]marshaledResourceMeta, len(rs))
	for name, r := range rs {
		r1 := marshaledResourceMeta{
			Path:        r.Path,
			Description: r.Description,
		}
		if r.Type != resource.TypeFile {
			r1.Type = r.Type.String()
		}
		rs1[name] = r1
	}
	return rs1
}

func marshaledRelations(relations map[string]Relation) map[string]marshaledRelation {
	marshaled := make(map[string]marshaledRelation)
	for name, relation := range relations {
		marshaled[name] = marshaledRelation(relation)
	}
	return marshaled
}

type marshaledRelation Relation

func (r marshaledRelation) MarshalYAML() (interface{}, error) {
	// See calls to ifaceExpander in charmSchema.
	var noLimit int
	if !r.Optional && r.Limit == noLimit && r.Scope == ScopeGlobal {
		// All attributes are default, so use the simple string form of the relation.
		return r.Interface, nil
	}
	mr := struct {
		Interface string        `yaml:"interface"`
		Limit     *int          `yaml:"limit,omitempty"`
		Optional  bool          `yaml:"optional,omitempty"`
		Scope     RelationScope `yaml:"scope,omitempty"`
	}{
		Interface: r.Interface,
		Optional:  r.Optional,
	}
	if r.Limit != noLimit {
		mr.Limit = &r.Limit
	}
	if r.Scope != ScopeGlobal {
		mr.Scope = r.Scope
	}
	return mr, nil
}

func marshaledExtraBindings(bindings map[string]ExtraBinding) map[string]interface{} {
	marshaled := make(map[string]interface{})
	for _, binding := range bindings {
		marshaled[binding.Name] = nil
	}
	return marshaled
}

type marshaledContainer Container

func marshaledContainers(c map[string]Container) map[string]marshaledContainer {
	marshaled := make(map[string]marshaledContainer)
	for k, v := range c {
		marshaled[k] = marshaledContainer(v)
	}
	return marshaled
}

func (c marshaledContainer) MarshalYAML() (interface{}, error) {
	mc := struct {
		Resource string  `yaml:"resource,omitempty"`
		Mounts   []Mount `yaml:"mounts,omitempty"`
	}{
		Resource: c.Resource,
		Mounts:   c.Mounts,
	}
	return mc, nil
}

// Format of the parsed charm.
type Format int

// Formats are the different versions of charm metadata supported.
const (
	FormatUnknown Format = iota
	FormatV1      Format = iota
	FormatV2      Format = iota
)

// Check checks that the metadata is well-formed.
func (m Meta) Check(format Format, reasons ...FormatSelectionReason) error {
	switch format {
	case FormatV1:
		return errors.NotValidf("charm metadata without bases in manifest")
	case FormatV2:
		err := m.checkV2(reasons)
		if err != nil {
			return errors.Trace(err)
		}
	default:
		return errors.Errorf("unknown format %v", format)
	}

	if err := validateMetaExtraBindings(m); err != nil {
		return errors.Errorf("charm %q has invalid extra bindings: %v", m.Name, err)
	}

	// Subordinate charms must have at least one relation that
	// has container scope, otherwise they can't relate to the
	// principal.
	if m.Subordinate {
		valid := false
		if m.Requires != nil {
			for _, relationData := range m.Requires {
				if relationData.Scope == ScopeContainer {
					valid = true
					break
				}
			}
		}
		if !valid {
			return errors.Errorf("subordinate charm %q lacks \"requires\" relation with container scope", m.Name)
		}
	}

	names := make(map[string]bool)
	for name, store := range m.Storage {
		if store.Location != "" && store.Type != StorageFilesystem {
			return errors.Errorf(`charm %q storage %q: location may not be specified for "type: %s"`, m.Name, name, store.Type)
		}
		if store.Type == "" {
			return errors.Errorf("charm %q storage %q: type must be specified", m.Name, name)
		}
		if store.CountMin < 0 {
			return errors.Errorf("charm %q storage %q: invalid minimum count %d", m.Name, name, store.CountMin)
		}
		if store.CountMax == 0 || store.CountMax < -1 {
			return errors.Errorf("charm %q storage %q: invalid maximum count %d", m.Name, name, store.CountMax)
		}
		if names[name] {
			return errors.Errorf("charm %q storage %q: duplicated storage name", m.Name, name)
		}
		names[name] = true
	}

	names = make(map[string]bool)
	for name, device := range m.Devices {
		if device.Type == "" {
			return errors.Errorf("charm %q device %q: type must be specified", m.Name, name)
		}
		if device.CountMax >= 0 && device.CountMin >= 0 && device.CountMin > device.CountMax {
			return errors.Errorf(
				"charm %q device %q: maximum count %d can not be smaller than minimum count %d",
				m.Name, name, device.CountMax, device.CountMin)
		}
		if names[name] {
			return errors.Errorf("charm %q device %q: duplicated device name", m.Name, name)
		}
		names[name] = true
	}

	if err := validateMetaResources(m.Resources); err != nil {
		return err
	}

	for _, term := range m.Terms {
		if _, terr := ParseTerm(term); terr != nil {
			return errors.Trace(terr)
		}
	}

	return nil
}

func (m Meta) checkV2(reasons []FormatSelectionReason) error {
	if len(reasons) == 0 {
		return errors.NotValidf("metadata v2 without manifest.yaml")
	}
	if m.MinJujuVersion != semversion.Zero {
		return errors.NotValidf("min-juju-version in metadata v2")
	}
	return nil
}

func reservedName(charmName, endpointName string) (reserved bool, reason string) {
	if strings.HasPrefix(charmName, "juju-") {
		return false, ""
	}
	if endpointName == "juju" {
		return true, `"juju" is a reserved name`
	}
	if strings.HasPrefix(endpointName, "juju-") {
		return true, `the "juju-" prefix is reserved`
	}
	return false, ""
}

func parseRelations(relations interface{}, role RelationRole) map[string]Relation {
	if relations == nil {
		return nil
	}
	result := make(map[string]Relation)
	for name, rel := range relations.(map[string]interface{}) {
		relMap := rel.(map[string]interface{})
		relation := Relation{
			Name:      name,
			Role:      role,
			Interface: relMap["interface"].(string),
			Optional:  relMap["optional"].(bool),
		}
		if scope := relMap["scope"]; scope != nil {
			relation.Scope = RelationScope(scope.(string))
		}
		if relMap["limit"] != nil {
			// Schema defaults to int64, but we know
			// the int range should be more than enough.
			relation.Limit = int(relMap["limit"].(int64))
		}
		result[name] = relation
	}
	return result
}

// CombinedRelations returns all defined relations, regardless of their type in
// a single map.
func (m Meta) CombinedRelations() map[string]Relation {
	combined := make(map[string]Relation)
	for name, relation := range m.Provides {
		combined[name] = relation
	}
	for name, relation := range m.Requires {
		combined[name] = relation
	}
	for name, relation := range m.Peers {
		combined[name] = relation
	}
	return combined
}

// Schema coercer that expands the interface shorthand notation.
// A consistent format is easier to work with than considering the
// potential difference everywhere.
//
// Supports the following variants::
//
//	provides:
//	  server: riak
//	  admin: http
//	  foobar:
//	    interface: blah
//
//	provides:
//	  server:
//	    interface: mysql
//	    limit:
//	    optional: false
//
// In all input cases, the output is the fully specified interface
// representation as seen in the mysql interface description above.
func ifaceExpander(limit interface{}) schema.Checker {
	return ifaceExpC{limit}
}

type ifaceExpC struct {
	limit interface{}
}

var (
	stringC = schema.String()
	mapC    = schema.StringMap(schema.Any())
)

func (c ifaceExpC) Coerce(v interface{}, path []string) (newv interface{}, err error) {
	s, err := stringC.Coerce(v, path)
	if err == nil {
		newv = map[string]interface{}{
			"interface": s,
			"limit":     c.limit,
			"optional":  false,
			"scope":     string(ScopeGlobal),
		}
		return
	}

	v, err = mapC.Coerce(v, path)
	if err != nil {
		return
	}
	m := v.(map[string]interface{})
	if _, ok := m["limit"]; !ok {
		m["limit"] = c.limit
	}
	return ifaceSchema.Coerce(m, path)
}

var ifaceSchema = schema.FieldMap(
	schema.Fields{
		"interface": schema.String(),
		"limit":     schema.OneOf(schema.Const(nil), schema.Int()),
		"scope":     schema.OneOf(schema.Const(string(ScopeGlobal)), schema.Const(string(ScopeContainer))),
		"optional":  schema.Bool(),
	},
	schema.Defaults{
		"scope":    string(ScopeGlobal),
		"optional": false,
	},
)

func parseStorage(stores interface{}) map[string]Storage {
	if stores == nil {
		return nil
	}
	result := make(map[string]Storage)
	for name, store := range stores.(map[string]interface{}) {
		storeMap := store.(map[string]interface{})
		store := Storage{
			Name:     name,
			Type:     StorageType(storeMap["type"].(string)),
			Shared:   storeMap["shared"].(bool),
			ReadOnly: storeMap["read-only"].(bool),
			CountMin: 1,
			CountMax: 1,
		}
		if desc, ok := storeMap["description"].(string); ok {
			store.Description = desc
		}
		if multiple, ok := storeMap["multiple"].(map[string]interface{}); ok {
			if r, ok := multiple["range"].([2]int); ok {
				store.CountMin, store.CountMax = r[0], r[1]
			}
		}
		if minSize, ok := storeMap["minimum-size"].(uint64); ok {
			store.MinimumSize = minSize
		}
		if loc, ok := storeMap["location"].(string); ok {
			store.Location = loc
		}
		if properties, ok := storeMap["properties"].([]interface{}); ok {
			for _, p := range properties {
				store.Properties = append(store.Properties, p.(string))
			}
		}
		result[name] = store
	}
	return result
}

func parseDevices(devices interface{}) map[string]Device {
	if devices == nil {
		return nil
	}
	result := make(map[string]Device)
	for name, device := range devices.(map[string]interface{}) {
		deviceMap := device.(map[string]interface{})
		device := Device{
			Name:     name,
			Type:     DeviceType(deviceMap["type"].(string)),
			CountMin: 1,
			CountMax: 1,
		}
		if desc, ok := deviceMap["description"].(string); ok {
			device.Description = desc
		}
		if countmin, ok := deviceMap["countmin"].(int64); ok {
			device.CountMin = countmin
		}
		if countmax, ok := deviceMap["countmax"].(int64); ok {
			device.CountMax = countmax
		}
		result[name] = device
	}
	return result
}

func parseContainers(input interface{}, resources map[string]resource.Meta, storage map[string]Storage) (map[string]Container, error) {
	var err error
	if input == nil {
		return nil, nil
	}
	containers := map[string]Container{}
	for name, v := range input.(map[string]interface{}) {
		containerMap := v.(map[string]interface{})
		container := Container{}

		if value, ok := containerMap["resource"]; ok {
			container.Resource = value.(string)
		}
		if container.Resource != "" {
			if r, ok := resources[container.Resource]; !ok {
				return nil, errors.NotFoundf("referenced resource %q", container.Resource)
			} else if r.Type != resource.TypeContainerImage {
				return nil, errors.Errorf("referenced resource %q is not a %s",
					container.Resource,
					resource.TypeContainerImage.String())
			}
		}

		container.Mounts, err = parseMounts(containerMap["mounts"], storage)
		if err != nil {
			return nil, errors.Annotatef(err, "container %q", name)
		}

		if value, ok := containerMap["uid"]; ok {
			uid := int(value.(int64))
			container.Uid = &uid
			if uid >= 1000 && uid < 10000 {
				return nil, errors.Errorf("container %q has invalid uid %d: uid cannot be in reserved range 1000-9999",
					name, uid)
			}
		}
		if value, ok := containerMap["gid"]; ok {
			gid := int(value.(int64))
			container.Gid = &gid
			if gid >= 1000 && gid < 10000 {
				return nil, errors.Errorf("container %q has invalid gid %d: gid cannot be in reserved range 1000-9999",
					name, gid)
			}
		}

		containers[name] = container
	}
	if len(containers) == 0 {
		return nil, nil
	}
	return containers, nil
}

func parseMounts(input interface{}, storage map[string]Storage) ([]Mount, error) {
	if input == nil {
		return nil, nil
	}
	mounts := []Mount(nil)
	for _, v := range input.([]interface{}) {
		mount := Mount{}
		mountMap := v.(map[string]interface{})
		if value, ok := mountMap["storage"].(string); ok {
			mount.Storage = value
		}
		if value, ok := mountMap["location"].(string); ok {
			mount.Location = value
		}
		if mount.Storage == "" {
			return nil, errors.Errorf("storage must be specified on mount")
		}
		if mount.Location == "" {
			return nil, errors.Errorf("location must be specified on mount")
		}
		if _, ok := storage[mount.Storage]; !ok {
			return nil, errors.NotValidf("storage %q", mount.Storage)
		}
		mounts = append(mounts, mount)
	}
	return mounts, nil
}

func parseMinJujuVersion(value any) (semversion.Number, error) {
	if value == nil {
		return semversion.Zero, nil
	}
	ver, err := semversion.Parse(value.(string))
	if err != nil {
		return semversion.Zero, errors.Annotate(err, "invalid min-juju-version")
	}
	return ver, nil
}

func parseCharmUser(value any) (RunAs, error) {
	if value == nil {
		return RunAsDefault, nil
	}
	v := RunAs(value.(string))
	switch v {
	case RunAsRoot, RunAsSudoer, RunAsNonRoot:
		return v, nil
	default:
		return RunAsDefault, errors.Errorf("invalid charm-user %q expected one of %s, %s or %s", v,
			RunAsRoot, RunAsSudoer, RunAsNonRoot)
	}
}

var storageSchema = schema.FieldMap(
	schema.Fields{
		"type":      schema.OneOf(schema.Const(string(StorageBlock)), schema.Const(string(StorageFilesystem))),
		"shared":    schema.Bool(),
		"read-only": schema.Bool(),
		"multiple": schema.FieldMap(
			schema.Fields{
				"range": storageCountC{}, // m, m-n, m+, m-
			},
			schema.Defaults{},
		),
		"minimum-size": storageSizeC{},
		"location":     schema.String(),
		"description":  schema.String(),
		"properties":   schema.List(propertiesC{}),
	},
	schema.Defaults{
		"shared":       false,
		"read-only":    false,
		"multiple":     schema.Omit,
		"location":     schema.Omit,
		"description":  schema.Omit,
		"properties":   schema.Omit,
		"minimum-size": schema.Omit,
	},
)

var deviceSchema = schema.FieldMap(
	schema.Fields{
		"description": schema.String(),
		"type":        schema.String(),
		"countmin":    deviceCountC{},
		"countmax":    deviceCountC{},
	}, schema.Defaults{
		"description": schema.Omit,
		"countmin":    schema.Omit,
		"countmax":    schema.Omit,
	},
)

type deviceCountC struct{}

func (c deviceCountC) Coerce(v interface{}, path []string) (interface{}, error) {
	s, err := schema.Int().Coerce(v, path)
	if err != nil {
		return 0, err
	}
	if m, ok := s.(int64); ok {
		if m >= 0 {
			return m, nil
		}
	}
	return 0, errors.Errorf("invalid device count %d", s)
}

type storageCountC struct{}

var storageCountRE = regexp.MustCompile("^([0-9]+)([-+]|-[0-9]+)$")

func (c storageCountC) Coerce(v interface{}, path []string) (newv interface{}, err error) {
	s, err := schema.OneOf(schema.Int(), stringC).Coerce(v, path)
	if err != nil {
		return nil, err
	}
	if m, ok := s.(int64); ok {
		// We've got a count of the form "m": m represents
		// both the minimum and maximum.
		if m <= 0 {
			return nil, errors.Errorf("%s: invalid count %v", strings.Join(path[1:], ""), m)
		}
		return [2]int{int(m), int(m)}, nil
	}
	match := storageCountRE.FindStringSubmatch(s.(string))
	if match == nil {
		return nil, errors.Errorf("%s: value %q does not match 'm', 'm-n', or 'm+'", strings.Join(path[1:], ""), s)
	}
	var m, n int
	if m, err = strconv.Atoi(match[1]); err != nil {
		return nil, err
	}
	if len(match[2]) == 1 {
		// We've got a count of the form "m+" or "m-":
		// m represents the minimum, and there is no
		// upper bound.
		n = -1
	} else {
		if n, err = strconv.Atoi(match[2][1:]); err != nil {
			return nil, err
		}
	}
	return [2]int{m, n}, nil
}

type storageSizeC struct{}

func (c storageSizeC) Coerce(v interface{}, path []string) (newv interface{}, err error) {
	s, err := schema.String().Coerce(v, path)
	if err != nil {
		return nil, err
	}
	return utils.ParseSize(s.(string))
}

type propertiesC struct{}

func (c propertiesC) Coerce(v interface{}, path []string) (newv interface{}, err error) {
	return schema.OneOf(schema.Const("transient")).Coerce(v, path)
}

var containerSchema = schema.FieldMap(
	schema.Fields{
		"resource": schema.String(),
		"mounts":   schema.List(mountSchema),
		"uid":      schema.Int(),
		"gid":      schema.Int(),
	}, schema.Defaults{
		"resource": schema.Omit,
		"mounts":   schema.Omit,
		"uid":      schema.Omit,
		"gid":      schema.Omit,
	})

var mountSchema = schema.FieldMap(
	schema.Fields{
		"storage":  schema.String(),
		"location": schema.String(),
	}, schema.Defaults{
		"storage":  schema.Omit,
		"location": schema.Omit,
	})

var charmSchema = schema.FieldMap(
	schema.Fields{
		"name":             schema.String(),
		"summary":          schema.String(),
		"description":      schema.String(),
		"peers":            schema.StringMap(ifaceExpander(nil)),
		"provides":         schema.StringMap(ifaceExpander(nil)),
		"requires":         schema.StringMap(ifaceExpander(nil)),
		"extra-bindings":   extraBindingsSchema,
		"revision":         schema.Int(), // Obsolete
		"format":           schema.Int(), // Obsolete
		"subordinate":      schema.Bool(),
		"categories":       schema.List(schema.String()),
		"tags":             schema.List(schema.String()),
		"storage":          schema.StringMap(storageSchema),
		"devices":          schema.StringMap(deviceSchema),
		"resources":        schema.StringMap(resourceSchema),
		"terms":            schema.List(schema.String()),
		"min-juju-version": schema.String(),
		"assumes":          schema.List(schema.Any()),
		"containers":       schema.StringMap(containerSchema),
		"charm-user":       schema.String(),
	},
	schema.Defaults{
		"provides":         schema.Omit,
		"requires":         schema.Omit,
		"peers":            schema.Omit,
		"extra-bindings":   schema.Omit,
		"revision":         schema.Omit,
		"format":           schema.Omit,
		"subordinate":      schema.Omit,
		"categories":       schema.Omit,
		"tags":             schema.Omit,
		"storage":          schema.Omit,
		"devices":          schema.Omit,
		"resources":        schema.Omit,
		"terms":            schema.Omit,
		"min-juju-version": schema.Omit,
		"assumes":          schema.Omit,
		"containers":       schema.Omit,
		"charm-user":       schema.Omit,
	},
)

// ensureUnambiguousFormat returns an error if the raw data contains
// both metadata v1 and v2 contents. However is it unable to definitively
// determine which format the charm is as metadata does not contain bases.
func ensureUnambiguousFormat(raw map[interface{}]interface{}) error {
	format := FormatUnknown
	matched := []string(nil)
	mismatched := []string(nil)
	keys := []string(nil)
	for k := range raw {
		key, ok := k.(string)
		if !ok {
			// Non-string keys will be an error handled by the schema lib.
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		detected := FormatUnknown
		switch key {
		case "containers", "assumes", "charm-user":
			detected = FormatV2
		case "series", "deployment", "min-juju-version":
			detected = FormatV1
		}
		if detected == FormatUnknown {
			continue
		}
		if format == FormatUnknown {
			format = detected
		}
		if format == detected {
			matched = append(matched, key)
		} else {
			mismatched = append(mismatched, key)
		}
	}
	if mismatched != nil {
		return errors.Errorf("ambiguous metadata: keys %s cannot be used with %s",
			`"`+strings.Join(mismatched, `", "`)+`"`,
			`"`+strings.Join(matched, `", "`)+`"`)
	}
	return nil
}
