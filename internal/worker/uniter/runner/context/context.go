// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/proxy"
	"k8s.io/client-go/rest"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/quota"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/version"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/hooks"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/worker/common/charmrunner"
	"github.com/juju/juju/internal/worker/uniter/api"
	"github.com/juju/juju/internal/worker/uniter/runner/context/resources"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/juju/sockets"
	"github.com/juju/juju/rpc/params"
)

// Context exposes hooks.Context, and additional methods needed by Runner.
type Context interface {
	jujuc.Context
	Id() string
	HookVars(
		ctx context.Context,
		paths Paths,
		env Environmenter) ([]string, error)
	ActionData() (*ActionData, error)
	SetProcess(process HookProcess)
	HasExecutionSetUnitStatus() bool
	ResetExecutionSetUnitStatus()
	ModelType() model.ModelType

	Prepare(ctx context.Context) error
	Flush(ctx context.Context, badge string, failure error) error

	GetLoggerByName(module string) logger.Logger
}

// Paths exposes the paths needed by Context.
type Paths interface {
	// GetToolsDir returns the filesystem path to the dirctory containing
	// the hook tool symlinks.
	GetToolsDir() string

	// GetBaseDir returns the filesystem path to the directory in which
	// the charm is installed.
	GetBaseDir() string

	// GetCharmDir returns the filesystem path to the directory in which
	// the charm is installed.
	GetCharmDir() string

	// GetJujucServerSocket returns the path to the socket used by the hook tools
	// to communicate back to the executing uniter process. It might be a
	// filesystem path, or it might be abstract.
	GetJujucServerSocket() sockets.Socket

	// GetJujucClientSocket returns the path to the socket used by the hook tools
	// to communicate back to the executing uniter process. It might be a
	// filesystem path, or it might be abstract.
	GetJujucClientSocket() sockets.Socket

	// GetMetricsSpoolDir returns the path to a metrics spool dir, used
	// to store metrics recorded during a single hook run.
	GetMetricsSpoolDir() string

	// GetResourcesDir returns the filesystem path to the directory
	// containing resource data files.
	GetResourcesDir() string
}

// Clock defines the methods of the full clock.Clock that are needed here.
type Clock interface {
	// After waits for the duration to elapse and then sends the
	// current time on the returned channel.
	After(time.Duration) <-chan time.Time
}

var ErrIsNotLeader = errors.Errorf("this unit is not the leader")

// HookProcess is an interface representing a process running a hook.
type HookProcess interface {
	Pid() int
	Kill() error
}

// HookUnit represents the functions needed by a unit in a hook context to
// call into state.
type HookUnit interface {
	Application(context.Context) (api.Application, error)
	ApplicationName() string
	ConfigSettings(context.Context) (charm.Config, error)
	LogActionMessage(context.Context, names.ActionTag, string) error
	Name() string
	NetworkInfo(ctx context.Context, bindings []string, relationId *int) (map[string]params.NetworkInfoResult, error)
	RequestReboot(context.Context) error
	SetUnitStatus(ctx context.Context, unitStatus status.Status, info string, data map[string]interface{}) error
	SetAgentStatus(ctx context.Context, agentStatus status.Status, info string, data map[string]interface{}) error
	State(context.Context) (params.UnitStateResult, error)
	Tag() names.UnitTag
	UnitStatus(context.Context) (params.StatusResult, error)
	CommitHookChanges(context.Context, params.CommitHookChangesArgs) error
	PublicAddress(context.Context) (string, error)
}

// HookContext is the implementation of runner.Context.
type HookContext struct {
	*resources.ResourcesHookContext
	unit HookUnit

	// uniter is the handle to the uniter client so that HookContext can make
	// API calls on the uniter facade.
	// NOTE: We would like to be rid of the fake-remote-Unit and switch
	// over fully to API calls on the uniter.  This adds that ability, but we're
	// not fully there yet.
	uniter api.UniterClient

	// secretsClient allows the context to access the secrets backend.
	secretsClient api.SecretsAccessor

	// secretsBackendGetter is used to get a client to access the secrets backend.
	secretsBackendGetter SecretsBackendGetter
	// secretsBackend is the secrets backend client, created only when needed.
	secretsBackend api.SecretsBackend

	// LeadershipContext supplies several hooks.Context methods.
	LeadershipContext

	// inClusterConfig supplies the K8s rest.InClusterConfig function,
	// overrideable for testing.
	inClusterConfig func() (*rest.Config, error)

	// principal is the unitName of the principal charm.
	principal string

	// privateAddress is the cached value of the unit's private
	// address.
	privateAddress string

	// publicAddress is the cached value of the unit's public
	// address.
	publicAddress string

	// availabilityZone is the cached value of the unit's
	// availability zone name.
	availabilityZone string

	// configSettings holds the application configuration.
	configSettings charm.Config

	// goalState holds the goal state struct
	goalState application.GoalState

	// id identifies the context.
	id string

	hookName string

	// actionData contains the values relevant to the run of an Action:
	// its tag, its parameters, and its results.
	actionData *ActionData
	// actionDataMu protects against concurrent access to actionData.
	actionDataMu sync.Mutex

	// uuid is the universally unique identifier of the environment.
	uuid string

	// modelName is the human friendly name of the environment.
	modelName string

	// modelType
	modelType model.ModelType

	// unitName is the human friendly name of the local unit.
	unitName string

	// status is the status of the local unit.
	status *jujuc.StatusInfo

	// relationId identifies the relation for which a relation hook is
	// executing. If it is -1, the context is not running a relation hook;
	// otherwise, its value must be a valid key into the relations map.
	relationId int

	// remoteUnitName identifies the changing unit of the executing relation
	// hook. It will be empty if the context is not running a relation hook,
	// or if it is running a relation-broken hook.
	remoteUnitName string

	// remoteApplicationName identifies the application name in response to
	// relation-set --app.
	remoteApplicationName string

	// relations contains the context for every relation the unit is a member
	// of, keyed on relation id.
	relations map[int]*ContextRelation

	// departingUnitName identifies the unit that goes away from the relation.
	// It is only populated when running a RelationDeparted hook.
	departingUnitName string

	// apiAddrs contains the API server addresses.
	apiAddrs []string

	// legacyProxySettings are the current legacy proxy settings
	// that the uniter knows about.
	legacyProxySettings proxy.Settings

	// jujuProxySettings are the current juju proxy settings
	// that the uniter knows about.
	jujuProxySettings proxy.Settings

	// a helper for recording requests to open/close port ranges for this unit.
	portRangeChanges *portRangeChangeRecorder

	// assignedMachineTag contains the tag of the unit's assigned
	// machine.
	assignedMachineTag names.MachineTag

	// process is the process of the command that is being run in the local context,
	// like a juju-exec command or a hook
	process HookProcess

	// rebootPriority tells us when the hook wants to reboot. If rebootPriority
	// is hooks.RebootNow the hook will be killed and requeued.
	rebootPriority jujuc.RebootPriority

	// storageId is the tag of the storage instance associated
	// with the running hook.
	storageTag names.StorageTag

	// storageTags returns the tags of storage instances attached to the unit
	storageTags []names.StorageTag

	// storageAttachmentCache holds cached storage attachments so that hook
	// calls are consistent.
	storageAttachmentCache map[names.StorageTag]jujuc.ContextStorageAttachment

	// hasRunSetStatus is true if a call to the status-set was made during the
	// invocation of a hook.
	// This attribute is persisted to local uniter state at the end of the hook
	// execution so that the uniter can ultimately decide if it needs to update
	// a charm's workload status, or if the charm has already taken care of it.
	hasRunStatusSet bool

	// storageAddDirectives is a collection of storage directives
	// keyed on storage name as specified in the charm.
	// This collection will be added to the unit on successful
	// hook run, so the actual add will happen in a flush.
	storageAddDirectives map[string][]params.StorageDirectives

	// clock is used for any time operations.
	clock Clock

	// logger is used for context logging.
	logger logger.Logger

	// The cloud specification
	cloudSpec *params.CloudSpec

	// The cloud API version, if available.
	cloudAPIVersion string

	// A cached view of the unit's charm state that gets persisted by juju
	// once the context is flushed.
	cachedCharmState map[string]string

	// A flag that keeps track of whether the unit's state has been mutated.
	charmStateCacheDirty bool

	// workloadName is the name of the container which the hook is in relation to.
	workloadName string

	// noticeID is the Pebble notice ID associated with the hook.
	noticeID string

	// noticeType is the Pebble notice type associated with the hook.
	noticeType string

	// noticeKey is the Pebble notice key associated with the hook.
	noticeKey string

	// checkName is the Pebble check name associated with the hook.
	checkName string

	// secretURI is the reference to the secret relevant to the hook.
	secretURI string

	// secretRevision is the secret revision relevant to the hook.
	secretRevision int

	// secretLabel is the secret label to expose to the hook.
	secretLabel string

	// secretMetadata contains the metadata for secrets created by this charm.
	secretMetadata map[string]jujuc.SecretMetadata

	// secretChanges records changes to secrets during a hook execution.
	secretChanges *secretsChangeRecorder

	mu sync.Mutex
}

// GetLoggerByName returns a Logger for the specified module name.
func (c *HookContext) GetLoggerByName(module string) logger.Logger {
	return c.logger.GetChildByName(module)
}

// GetCharmState returns a copy of the cached charm state.
// Implements jujuc.HookContext.unitCharmStateContext, part of runner.Context.
func (c *HookContext) GetCharmState(ctx context.Context) (map[string]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureCharmStateLoaded(ctx); err != nil {
		return nil, err
	}

	if len(c.cachedCharmState) == 0 {
		return nil, nil
	}

	retVal := make(map[string]string, len(c.cachedCharmState))
	for k, v := range c.cachedCharmState {
		retVal[k] = v
	}
	return retVal, nil
}

// GetCharmStateValue returns the value of the given key.
// Implements jujuc.HookContext.unitCharmStateContext, part of runner.Context.
func (c *HookContext) GetCharmStateValue(ctx context.Context, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureCharmStateLoaded(ctx); err != nil {
		return "", err
	}

	value, ok := c.cachedCharmState[key]
	if !ok {
		return "", errors.NotFoundf("%q", key)
	}
	return value, nil
}

// SetCharmStateValue sets the key/value pair provided in the cache.
// Implements jujuc.HookContext.unitCharmStateContext, part of runner.Context.
func (c *HookContext) SetCharmStateValue(ctx context.Context, key, value string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureCharmStateLoaded(ctx); err != nil {
		return err
	}

	// Enforce fixed quota limit for key/value sizes. Performing this check
	// as early as possible allows us to provide feedback to charm authors
	// who might be tempted to exploit this feature for storing CLOBs/BLOBs.
	if err := quota.CheckTupleSize(key, value, quota.MaxCharmStateKeySize, quota.MaxCharmStateValueSize); err != nil {
		return errors.Trace(err)
	}

	curValue, exists := c.cachedCharmState[key]
	if exists && curValue == value {
		return nil // no-op
	}

	c.cachedCharmState[key] = value
	c.charmStateCacheDirty = true
	return nil
}

// DeleteCharmStateValue deletes the key/value pair for the given key from
// the cache.
// Implements jujuc.HookContext.unitCharmStateContext, part of runner.Context.
func (c *HookContext) DeleteCharmStateValue(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureCharmStateLoaded(ctx); err != nil {
		return err
	}

	if _, exists := c.cachedCharmState[key]; !exists {
		return nil // no-op
	}

	delete(c.cachedCharmState, key)
	c.charmStateCacheDirty = true
	return nil
}

// ensureCharmStateLoaded retrieves and caches the unit's charm state from the
// controller. The caller of this method must be holding the ctx mutex.
func (c *HookContext) ensureCharmStateLoaded(ctx context.Context) error {
	// NOTE: Assuming lock to be held!
	if c.cachedCharmState != nil {
		return nil
	}

	// Load from controller
	var charmState map[string]string
	unitState, err := c.unit.State(ctx)
	if err != nil {
		return errors.Annotate(err, "loading unit state from database")
	}
	if unitState.CharmState == nil {
		charmState = make(map[string]string)
	} else {
		charmState = unitState.CharmState
	}

	c.cachedCharmState = charmState
	c.charmStateCacheDirty = false
	return nil
}

// RequestReboot will set the reboot flag to true on the machine agent
// Implements jujuc.HookContext.ContextInstance, part of runner.Context.
func (c *HookContext) RequestReboot(priority jujuc.RebootPriority) error {
	// Must set reboot priority first, because killing the hook
	// process will trigger the completion of the hook. If killing
	// the hook fails, then we can reset the priority.
	c.setRebootPriority(priority)

	var err error
	if priority == jujuc.RebootNow {
		// At this point, the hook should be running
		err = c.killCharmHook(context.TODO())
	}

	switch err {
	case nil, charmrunner.ErrNoProcess:
		// ErrNoProcess almost certainly means we are running in debug hooks
	default:
		c.setRebootPriority(jujuc.RebootSkip)
	}
	return err
}

func (c *HookContext) GetRebootPriority() jujuc.RebootPriority {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rebootPriority
}

func (c *HookContext) setRebootPriority(priority jujuc.RebootPriority) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rebootPriority = priority
}

func (c *HookContext) GetProcess() HookProcess {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.process
}

// SetProcess implements runner.Context.
func (c *HookContext) SetProcess(process HookProcess) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.process = process
}

// Id returns an integer which uniquely identifies the relation.
// Implements jujuc.HookContext.ContextRelation, part of runner.Context.
func (c *HookContext) Id() string {
	return c.id
}

// UnitName returns the executing unit's name.
// UnitName implements jujuc.HookContext.ContextUnit, part of runner.Context.
func (c *HookContext) UnitName() string {
	return c.unitName
}

// ModelType of the context we are running in.
// SetProcess implements runner.Context.
func (c *HookContext) ModelType() model.ModelType {
	return c.modelType
}

// UnitStatus will return the status for the current Unit.
// Implements jujuc.HookContext.ContextStatus, part of runner.Context.
func (c *HookContext) UnitStatus(ctx context.Context) (*jujuc.StatusInfo, error) {
	if c.status == nil {
		var err error
		unitStatus, err := c.unit.UnitStatus(ctx)
		if err != nil {
			return nil, err
		}
		c.status = &jujuc.StatusInfo{
			Status: unitStatus.Status,
			Info:   unitStatus.Info,
			Data:   unitStatus.Data,
		}
	}
	return c.status, nil
}

// ApplicationStatus returns the status for the application and all the units on
// the application to which this context unit belongs, only if this unit is
// the leader.
// Implements jujuc.HookContext.ContextStatus, part of runner.Context.
func (c *HookContext) ApplicationStatus(ctx context.Context) (jujuc.ApplicationStatusInfo, error) {
	var err error
	isLeader, err := c.IsLeader()
	if err != nil {
		return jujuc.ApplicationStatusInfo{}, errors.Annotatef(err, "cannot determine leadership")
	}
	if !isLeader {
		return jujuc.ApplicationStatusInfo{}, ErrIsNotLeader
	}
	app, err := c.unit.Application(ctx)
	if err != nil {
		return jujuc.ApplicationStatusInfo{}, errors.Trace(err)
	}
	appStatus, err := app.Status(ctx, c.unit.Name())
	if err != nil {
		return jujuc.ApplicationStatusInfo{}, errors.Trace(err)
	}
	us := make([]jujuc.StatusInfo, len(appStatus.Units))
	i := 0
	for t, s := range appStatus.Units {
		us[i] = jujuc.StatusInfo{
			Tag:    t,
			Status: s.Status,
			Info:   s.Info,
			Data:   s.Data,
		}
		i++
	}
	return jujuc.ApplicationStatusInfo{
		Application: jujuc.StatusInfo{
			Tag:    app.Tag().String(),
			Status: appStatus.Application.Status,
			Info:   appStatus.Application.Info,
			Data:   appStatus.Application.Data,
		},
		Units: us,
	}, nil
}

// SetUnitStatus will set the given status for this unit.
// Implements jujuc.HookContext.ContextStatus, part of runner.Context.
func (c *HookContext) SetUnitStatus(ctx context.Context, unitStatus jujuc.StatusInfo) error {
	c.hasRunStatusSet = true
	c.logger.Tracef(ctx, "[WORKLOAD-STATUS] %s: %s", unitStatus.Status, unitStatus.Info)
	return c.unit.SetUnitStatus(ctx,
		status.Status(unitStatus.Status),
		unitStatus.Info,
		unitStatus.Data,
	)
}

// SetAgentStatus will set the given status for this unit's agent.
// Implements jujuc.HookContext.ContextStatus, part of runner.Context.
func (c *HookContext) SetAgentStatus(ctx context.Context, agentStatus jujuc.StatusInfo) error {
	c.logger.Tracef(ctx, "[AGENT-STATUS] %s: %s", agentStatus.Status, agentStatus.Info)
	return c.unit.SetAgentStatus(
		ctx,
		status.Status(agentStatus.Status),
		agentStatus.Info,
		agentStatus.Data,
	)
}

// SetApplicationStatus will set the given status to the application to which this
// unit's belong, only if this unit is the leader.
// Implements jujuc.HookContext.ContextStatus, part of runner.Context.
func (c *HookContext) SetApplicationStatus(ctx context.Context, applicationStatus jujuc.StatusInfo) error {
	c.logger.Tracef(ctx, "[APPLICATION-STATUS] %s: %s", applicationStatus.Status, applicationStatus.Info)
	isLeader, err := c.IsLeader()
	if err != nil {
		return errors.Annotatef(err, "cannot determine leadership")
	}
	if !isLeader {
		return ErrIsNotLeader
	}

	app, err := c.unit.Application(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	return app.SetStatus(
		ctx,
		c.unit.Name(),
		status.Status(applicationStatus.Status),
		applicationStatus.Info,
		applicationStatus.Data,
	)
}

// HasExecutionSetUnitStatus implements runner.Context.
func (c *HookContext) HasExecutionSetUnitStatus() bool {
	return c.hasRunStatusSet
}

// ResetExecutionSetUnitStatus implements runner.Context.
func (c *HookContext) ResetExecutionSetUnitStatus() {
	c.hasRunStatusSet = false
}

// PublicAddress fetches the executing unit's public address if it has
// not yet been retrieved.
// The cached value is returned, or an error if it is not available.
func (c *HookContext) PublicAddress(ctx context.Context) (string, error) {
	if c.publicAddress == "" {
		var err error
		if c.publicAddress, err = c.unit.PublicAddress(ctx); err != nil && !params.IsCodeNoAddressSet(err) {
			return "", errors.Trace(err)
		}
	}

	if c.publicAddress == "" {
		return "", errors.NotFoundf("public address")
	}
	return c.publicAddress, nil
}

// PrivateAddress returns the executing unit's private address or an
// error if it is not available.
// Implements jujuc.HookContext.ContextNetworking, part of runner.Context.
func (c *HookContext) PrivateAddress() (string, error) {
	if c.privateAddress == "" {
		return "", errors.NotFoundf("private address")
	}
	return c.privateAddress, nil
}

// AvailabilityZone returns the executing unit's availability zone or an error
// if it was not found (or is not available).
// Implements jujuc.HookContext.ContextInstance, part of runner.Context.
func (c *HookContext) AvailabilityZone() (string, error) {
	if c.availabilityZone == "" {
		return "", errors.NotFoundf("availability zone")
	}
	return c.availabilityZone, nil
}

// StorageTags returns a list of tags for storage instances
// attached to the unit or an error if they are not available.
// Implements jujuc.HookContext.ContextStorage, part of runner.Context.
func (c *HookContext) StorageTags(ctx context.Context) ([]names.StorageTag, error) {
	// Comparing to nil on purpose here to cache an empty slice.
	if c.storageTags != nil {
		return c.storageTags, nil
	}
	attachmentIds, err := c.uniter.UnitStorageAttachments(ctx, c.unit.Tag())
	if err != nil {
		return nil, err
	}
	// N.B. zero-length non-nil slice on purpose.
	c.storageTags = make([]names.StorageTag, 0)
	for _, attachmentId := range attachmentIds {
		storageTag, err := names.ParseStorageTag(attachmentId.StorageTag)
		if err != nil {
			return nil, err
		}
		c.storageTags = append(c.storageTags, storageTag)
	}
	return c.storageTags, nil
}

// HookStorage returns the storage attachment associated
// the executing hook if it was found, and an error if it
// was not found or is not available.
// Implements jujuc.HookContext.ContextStorage, part of runner.Context.
func (c *HookContext) HookStorage(ctx context.Context) (jujuc.ContextStorageAttachment, error) {
	emptyTag := names.StorageTag{}
	if c.storageTag == emptyTag {
		return nil, errors.NotFound
	}
	return c.Storage(ctx, c.storageTag)
}

// Storage returns the ContextStorageAttachment with the supplied
// tag if it was found, and an error if it was not found or is not
// available to the context.
// Implements jujuc.HookContext.ContextStorage, part of runner.Context.
func (c *HookContext) Storage(ctx context.Context, tag names.StorageTag) (jujuc.ContextStorageAttachment, error) {
	if ctxStorageAttachment, ok := c.storageAttachmentCache[tag]; ok {
		return ctxStorageAttachment, nil
	}
	attachment, err := c.uniter.StorageAttachment(ctx, tag, c.unit.Tag())
	if err != nil {
		return nil, err
	}
	ctxStorageAttachment := &contextStorage{
		tag:      tag,
		kind:     storage.StorageKind(attachment.Kind),
		location: attachment.Location,
	}
	c.storageAttachmentCache[tag] = ctxStorageAttachment
	return ctxStorageAttachment, nil
}

// AddUnitStorage saves storage directives in the context.
// Implements jujuc.HookContext.ContextStorage, part of runner.Context.
func (c *HookContext) AddUnitStorage(cons map[string]params.StorageDirectives) error {
	// All storage directives are accumulated before context is flushed.
	if c.storageAddDirectives == nil {
		c.storageAddDirectives = make(
			map[string][]params.StorageDirectives,
			len(cons))
	}
	for storage, newConstraints := range cons {
		// Multiple calls for the same storage are accumulated as well.
		c.storageAddDirectives[storage] = append(
			c.storageAddDirectives[storage],
			newConstraints)
	}
	return nil
}

// OpenPortRange marks the supplied port range for opening.
// Implements jujuc.HookContext.ContextNetworking, part of runner.Context.
func (c *HookContext) OpenPortRange(endpointName string, portRange network.PortRange) error {
	return c.portRangeChanges.OpenPortRange(endpointName, portRange)
}

// ClosePortRange ensures the supplied port range is closed even when
// the executing unit's application is exposed (unless it is opened
// separately by a co- located unit).
// Implements jujuc.HookContext.ContextNetworking, part of runner.Context.
func (c *HookContext) ClosePortRange(endpointName string, portRange network.PortRange) error {
	return c.portRangeChanges.ClosePortRange(endpointName, portRange)
}

// OpenedPortRanges returns all port ranges currently opened by this
// unit on its assigned machine grouped by endpoint.
// Implements jujuc.HookContext.ContextNetworking, part of runner.Context.
func (c *HookContext) OpenedPortRanges() network.GroupedPortRanges {
	return c.portRangeChanges.OpenedUnitPortRanges()
}

// ConfigSettings returns the current application configuration of the executing unit.
// Implements jujuc.HookContext.ContextUnit, part of runner.Context.
func (c *HookContext) ConfigSettings(ctx context.Context) (charm.Config, error) {
	if c.configSettings == nil {
		var err error
		c.configSettings, err = c.unit.ConfigSettings(ctx)
		if err != nil {
			return nil, err
		}
	}
	result := charm.Config{}
	for name, value := range c.configSettings {
		result[name] = value
	}
	return result, nil
}

func (c *HookContext) getSecretsBackend() (api.SecretsBackend, error) {
	if c.secretsBackend != nil {
		return c.secretsBackend, nil
	}
	var err error
	c.secretsBackend, err = c.secretsBackendGetter()
	if err != nil {
		return nil, err
	}
	return c.secretsBackend, nil
}

func (c *HookContext) lookupOwnedSecretURIByLabel(ctx context.Context, label string) (*coresecrets.URI, error) {
	mds, err := c.SecretMetadata(ctx)
	if err != nil {
		return nil, err
	}
	isLeader, err := c.IsLeader()
	if err != nil {
		return nil, errors.Annotatef(err, "cannot determine leadership")
	}

	for ID, md := range mds {
		if md.Label != label {
			continue
		}
		if md.Owner.ID == c.unitName {
			return &coresecrets.URI{ID: ID}, nil
		}

		// Leaders own application secrets.
		if isLeader && md.Owner.ID == c.unit.ApplicationName() {
			return &coresecrets.URI{ID: ID}, nil
		}
	}
	for _, md := range c.secretChanges.pendingCreates {
		// Check if we have any pending create changes.
		if md.Label == nil || md.URI == nil {
			continue
		}
		if *md.Label == label {
			return md.URI, nil
		}
	}
	for _, md := range c.secretChanges.pendingUpdates {
		// Check if we have any pending label update changes.
		if md.Label == nil || md.URI == nil {
			continue
		}
		if *md.Label == label {
			return md.URI, nil
		}
	}
	return nil, errors.NotFoundf("secret owned by %q with label %q", c.unitName, label)
}

// GetSecret returns the value of the specified secret.
func (c *HookContext) GetSecret(ctx context.Context, uri *coresecrets.URI, label string, refresh, peek bool) (coresecrets.SecretValue, error) {
	if uri == nil && label == "" {
		return nil, errors.NotValidf("empty URI and label")
	}
	if uri != nil {
		if v, got := c.getPendingSecretValue(uri, label, refresh, peek); got {
			return v, nil
		}
	} else {
		// Try to resolve label to URI by looking up owned secrets.
		ownedSecretURI, err := c.lookupOwnedSecretURIByLabel(ctx, label)
		if err != nil && !errors.Is(err, errors.NotFound) {
			return nil, err
		}
		if ownedSecretURI != nil {
			// We now know the URI, see if there's any pending creates/updates
			// that should be used for the content.
			if v, got := c.getPendingSecretValue(ownedSecretURI, "", refresh, peek); got {
				return v, nil
			}
			// Found owned secret, no need for label anymore.
			uri = ownedSecretURI
			label = ""
		} else {
			// No previously created secret with this label, check if there's
			// any pending creates/updates that should be used for the content.
			if v, got := c.getPendingSecretValue(nil, label, refresh, peek); got {
				return v, nil
			}
		}
	}
	backend, err := c.getSecretsBackend()
	if err != nil {
		return nil, err
	}
	v, err := backend.GetContent(ctx, uri, label, refresh, peek)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (c *HookContext) getPendingSecretValue(uri *coresecrets.URI, label string, refresh, peek bool) (coresecrets.SecretValue, bool) {
	if uri == nil && label == "" {
		return nil, false
	}
	for i, v := range c.secretChanges.pendingCreates {
		if uri != nil && v.URI != nil && v.URI.ID == uri.ID {
			if label != "" {
				pending := c.secretChanges.pendingCreates[i]
				pending.Label = &label
				c.secretChanges.pendingCreates[i] = pending
			}
			// The initial value of the secret is not stored in the database yet.
			return v.Value, true
		}
		if label != "" && v.Label != nil && label == *v.Label {
			// The initial value of the secret is not stored in the database yet.
			return v.Value, true
		}
	}
	if !refresh && !peek {
		return nil, false
	}

	for i, v := range c.secretChanges.pendingUpdates {
		if uri != nil && v.URI != nil && v.URI.ID == uri.ID {
			if label != "" {
				pending := c.secretChanges.pendingUpdates[i]
				pending.Label = &label
				c.secretChanges.pendingUpdates[i] = pending
			}
			if refresh {
				c.secretChanges.pendingTrackLatest[v.URI.ID] = true
			}
			// The new value of the secret is going to be updated to the database.
			return v.Value, v.Value != nil && !v.Value.IsEmpty()
		}
		if label != "" && v.Label != nil && label == *v.Label {
			if refresh {
				c.secretChanges.pendingTrackLatest[v.URI.ID] = true
			}
			// The new value of the secret is going to be updated to the database.
			return v.Value, v.Value != nil && !v.Value.IsEmpty()
		}
	}
	return nil, false
}

// CreateSecret creates a secret with the specified data.
func (c *HookContext) CreateSecret(ctx context.Context, args *jujuc.SecretCreateArgs) (*coresecrets.URI, error) {
	if args.Owner.Kind == coresecrets.ApplicationOwner {
		isLeader, err := c.IsLeader()
		if err != nil {
			return nil, errors.Annotatef(err, "cannot determine leadership")
		}
		if !isLeader {
			return nil, ErrIsNotLeader
		}
	}
	if args.Value == nil || args.Value.IsEmpty() {
		return nil, errors.NotValidf("empty secret content")
	}
	checksum, err := args.Value.Checksum()
	if err != nil {
		return nil, errors.Annotate(err, "calculating secret checksum")
	}
	uris, err := c.secretsClient.CreateSecretURIs(ctx, 1)
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = c.secretChanges.create(uniter.SecretCreateArg{
		SecretUpsertArg: uniter.SecretUpsertArg{
			URI:          uris[0],
			RotatePolicy: args.RotatePolicy,
			ExpireTime:   args.ExpireTime,
			Description:  args.Description,
			Label:        args.Label,
			Value:        args.Value,
			Checksum:     checksum,
		},
		Owner: args.Owner,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return uris[0], nil
}

func (c *HookContext) lazyLoadSecretMetadata(ctx context.Context) error {
	if c.secretMetadata != nil {
		return nil
	}
	info, err := c.secretsClient.SecretMetadata(ctx)
	if err != nil {
		return err
	}
	c.secretMetadata = make(map[string]jujuc.SecretMetadata)
	for _, md := range info {
		c.secretMetadata[md.URI.ID] = jujuc.SecretMetadata{
			Description:      md.Description,
			Label:            md.Label,
			Owner:            md.Owner,
			RotatePolicy:     md.RotatePolicy,
			LatestRevision:   md.LatestRevision,
			LatestChecksum:   md.LatestRevisionChecksum,
			LatestExpireTime: md.LatestExpireTime,
			NextRotateTime:   md.NextRotateTime,
			Access:           md.Access,
		}
	}
	return nil
}

// UpdateSecret creates a secret with the specified data.
func (c *HookContext) UpdateSecret(ctx context.Context, uri *coresecrets.URI, args *jujuc.SecretUpdateArgs) error {
	err := c.lazyLoadSecretMetadata(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	md, knowSecret := c.secretMetadata[uri.ID]
	if knowSecret && md.Owner.Kind == coresecrets.ApplicationOwner {
		isLeader, err := c.IsLeader()
		if err != nil {
			return errors.Annotatef(err, "cannot determine leadership")
		}
		if !isLeader {
			return ErrIsNotLeader
		}
	}
	updateArg := uniter.SecretUpdateArg{
		SecretUpsertArg: uniter.SecretUpsertArg{
			URI:          uri,
			RotatePolicy: args.RotatePolicy,
			ExpireTime:   args.ExpireTime,
			Description:  args.Description,
			Label:        args.Label,
		},
		CurrentRevision: md.LatestRevision,
	}
	if args.Value != nil && !args.Value.IsEmpty() {
		checksum, err := args.Value.Checksum()
		if err != nil {
			return errors.Annotate(err, "calculating secret checksum")
		}
		if !knowSecret || md.LatestChecksum != checksum {
			updateArg.Value = args.Value
			updateArg.Checksum = checksum
		}
	}
	if args.RotatePolicy == nil && args.Description == nil && args.ExpireTime == nil &&
		args.Label == nil && updateArg.Value == nil {
		return nil
	}

	c.secretChanges.update(updateArg)
	return nil
}

// RemoveSecret removes a secret with the specified uri.
func (c *HookContext) RemoveSecret(ctx context.Context, uri *coresecrets.URI, revision *int) error {
	err := c.lazyLoadSecretMetadata(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	md, ok := c.secretMetadata[uri.ID]
	if ok && md.Owner.Kind == coresecrets.ApplicationOwner {
		isLeader, err := c.IsLeader()
		if err != nil {
			return errors.Annotatef(err, "cannot determine leadership")
		}
		if !isLeader {
			return ErrIsNotLeader
		}
	}
	c.secretChanges.remove(uri, revision)
	return nil
}

// SecretMetadata gets the secret ids and their labels and latest revisions created by the charm.
// The result includes any pending updates.
func (c *HookContext) SecretMetadata(ctx context.Context) (map[string]jujuc.SecretMetadata, error) {
	err := c.lazyLoadSecretMetadata(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make(map[string]jujuc.SecretMetadata)
	for _, c := range c.secretChanges.pendingCreates {
		md := jujuc.SecretMetadata{
			Owner:          c.Owner,
			LatestRevision: 1,
			LatestChecksum: c.Checksum,
		}
		if c.Label != nil {
			md.Label = *c.Label
		}
		if c.Description != nil {
			md.Description = *c.Description
		}
		if c.RotatePolicy != nil {
			md.RotatePolicy = *c.RotatePolicy
		}
		if c.ExpireTime != nil {
			md.LatestExpireTime = c.ExpireTime
		}
		result[c.URI.ID] = md
	}
	for id, v := range c.secretMetadata {
		if _, ok := c.secretChanges.pendingDeletes[id]; ok {
			continue
		}
		if u, ok := c.secretChanges.pendingUpdates[id]; ok {
			if u.Label != nil {
				v.Label = *u.Label
			}
			if u.Description != nil {
				v.Description = *u.Description
			}
			if u.RotatePolicy != nil {
				v.RotatePolicy = *u.RotatePolicy
			}
			if u.ExpireTime != nil {
				v.LatestExpireTime = u.ExpireTime
			}
			v.LatestChecksum = u.Checksum
		}
		result[id] = v
	}
	for k, v := range result {
		uri := &coresecrets.URI{ID: k}
		var err error
		if v.Access, err = c.secretChanges.secretGrantInfo(context.TODO(), uri, v.Access...); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return result, nil
}

// GrantSecret grants access to a specified secret.
func (c *HookContext) GrantSecret(ctx context.Context, uri *coresecrets.URI, arg *jujuc.SecretGrantRevokeArgs) error {
	secretMetadata, err := c.SecretMetadata(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	md, ok := secretMetadata[uri.ID]
	if !ok {
		return errors.NotFoundf("secret %q", uri.ID)
	}
	if md.Owner.Kind == coresecrets.ApplicationOwner {
		isLeader, err := c.IsLeader()
		if err != nil {
			return errors.Annotatef(err, "cannot determine leadership")
		}
		if !isLeader {
			return ErrIsNotLeader
		}
	}
	uniterArg := uniter.SecretGrantRevokeArgs{
		URI:             uri,
		ApplicationName: arg.ApplicationName,
		UnitName:        arg.UnitName,
		RelationKey:     arg.RelationKey,
		Role:            coresecrets.RoleView,
	}
	params := uniterArg.ToParams()
	if len(params.SubjectTags) != 1 {
		return errors.NewNotValid(nil, fmt.Sprintf("expected only 1 subject, got %d", len(params.SubjectTags)))
	}
	subjectTag, err := names.ParseTag(params.SubjectTags[0])
	if err != nil {
		return errors.Trace(err)
	}
	for _, g := range md.Access {
		if params.ScopeTag != g.Scope || params.Role != string(g.Role) {
			continue
		}
		existingTargetTag, err := names.ParseTag(g.Target)
		if err != nil {
			return errors.Trace(err)
		}
		if existingTargetTag.String() == subjectTag.String() {
			// No ops.
			return nil
		}
		if existingTargetTag.Kind() == names.ApplicationTagKind {
			// We haven already grant in application level, so no ops for any unit level grant.
			return nil
		}
		if subjectTag.Kind() == names.ApplicationTagKind {
			return errors.NewNotValid(nil, "any unit level grants need to be revoked before granting access to the corresponding application")
		}
	}
	c.secretChanges.grant(uniterArg)
	return nil
}

// RevokeSecret revokes access to a specified secret.
func (c *HookContext) RevokeSecret(ctx context.Context, uri *coresecrets.URI, args *jujuc.SecretGrantRevokeArgs) error {
	err := c.lazyLoadSecretMetadata(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	md, ok := c.secretMetadata[uri.ID]
	if ok && md.Owner.Kind == coresecrets.ApplicationOwner {
		isLeader, err := c.IsLeader()
		if err != nil {
			return errors.Annotatef(err, "cannot determine leadership")
		}
		if !isLeader {
			return ErrIsNotLeader
		}
	}
	c.secretChanges.revoke(uniter.SecretGrantRevokeArgs{
		URI:             uri,
		ApplicationName: args.ApplicationName,
		UnitName:        args.UnitName,
		RelationKey:     args.RelationKey,
	})
	return nil
}

// GoalState returns the goal state for the current unit.
// Implements jujuc.HookContext.ContextUnit, part of runner.Context.
func (c *HookContext) GoalState(ctx context.Context) (*application.GoalState, error) {
	var err error
	c.goalState, err = c.uniter.GoalState(ctx)
	if err != nil {
		return nil, err
	}

	return &c.goalState, nil
}

// CloudSpec return the cloud specification for the running unit's model.
// Implements jujuc.HookContext.ContextUnit, part of runner.Context.
func (c *HookContext) CloudSpec(ctx context.Context) (*params.CloudSpec, error) {
	if c.cloudSpec != nil {
		return c.cloudSpec, nil
	}
	var err error
	if c.modelType == model.CAAS {
		c.cloudSpec, err = c.cloudSpecK8s(ctx)
	} else {
		c.cloudSpec, err = c.uniter.CloudSpec(ctx)
	}
	return c.cloudSpec, err
}

// cloudSpecK8s loads the in-cluster configuration to connect to the Kubernetes API.
func (c *HookContext) cloudSpecK8s(ctx context.Context) (*params.CloudSpec, error) {
	// Hit controller's CloudSpec API to determine whether we have permission,
	// and we need it for some of the fields (but not the credentials).
	modelSpec, err := c.uniter.CloudSpec(ctx)
	if err != nil {
		return nil, errors.Annotatef(err, "getting model credentials")
	}

	inClusterConfig := c.inClusterConfig
	if inClusterConfig == nil {
		inClusterConfig = rest.InClusterConfig
	}
	config, err := inClusterConfig()
	if err != nil {
		return nil, errors.Annotatef(err, "reading in-cluster config")
	}

	// InClusterConfig doesn't actually load the certificate file, so do it ourselves.
	if config.TLSClientConfig.CAFile == "" {
		// Oddly, InClusterConfig logs this error, but does not return it.
		return nil, errors.New("reading certificate file")
	}
	certBytes, err := os.ReadFile(config.TLSClientConfig.CAFile)
	if err != nil {
		return nil, errors.Annotatef(err, "reading certificate file")
	}

	credential := &params.CloudSpec{
		Type:     modelSpec.Type, // "kubernetes"
		Name:     modelSpec.Name,
		Region:   modelSpec.Region,
		Endpoint: config.Host,
		Credential: &params.CloudCredential{
			AuthType: string(cloud.OAuth2AuthType),
			Attributes: map[string]string{
				"Token": config.BearerToken,
			},
		},
		CACertificates:    []string{string(certBytes)},
		IsControllerCloud: modelSpec.IsControllerCloud,
	}
	return credential, nil
}

// ActionParams simply returns the arguments to the Action.
// Implements jujuc.ActionHookContext.actionHookContext, part of runner.Context.
func (c *HookContext) ActionParams() (map[string]interface{}, error) {
	c.actionDataMu.Lock()
	defer c.actionDataMu.Unlock()
	if c.actionData == nil {
		return nil, errors.New("not running an action")
	}
	return c.actionData.Params, nil
}

// LogActionMessage logs a progress message for the Action.
// Implements jujuc.ActionHookContext.actionHookContext, part of runner.Context.
func (c *HookContext) LogActionMessage(ctx context.Context, message string) error {
	c.actionDataMu.Lock()
	defer c.actionDataMu.Unlock()
	if c.actionData == nil {
		return errors.New("not running an action")
	}
	return c.unit.LogActionMessage(ctx, c.actionData.Tag, message)
}

// SetActionMessage sets a message for the Action, usually an error message.
// Implements jujuc.ActionHookContext.actionHookContext, part of runner.Context.
func (c *HookContext) SetActionMessage(message string) error {
	c.actionDataMu.Lock()
	defer c.actionDataMu.Unlock()
	if c.actionData == nil {
		return errors.New("not running an action")
	}
	c.actionData.ResultsMessage = message
	return nil
}

// SetActionFailed sets the fail state of the action.
// Implements jujuc.ActionHookContext.actionHookContext, part of runner.Context.
func (c *HookContext) SetActionFailed() error {
	c.actionDataMu.Lock()
	defer c.actionDataMu.Unlock()
	if c.actionData == nil {
		return errors.New("not running an action")
	}
	c.actionData.Failed = true
	return nil
}

// UpdateActionResults inserts new values for use with action-set and
// action-fail.  The results struct will be delivered to the controller
// upon completion of the Action.  It returns an error if not called on an
// Action-containing HookContext.
// Implements jujuc.ActionHookContext.actionHookContext, part of runner.Context.
func (c *HookContext) UpdateActionResults(keys []string, value interface{}) error {
	c.actionDataMu.Lock()
	defer c.actionDataMu.Unlock()
	if c.actionData == nil {
		return errors.New("not running an action")
	}
	addValueToMap(keys, value, c.actionData.ResultsMap)
	return nil
}

// HookRelation returns the ContextRelation associated with the executing
// hook if it was found, or an error if it was not found (or is not available).
// Implements jujuc.RelationHookContext.relationHookContext, part of runner.Context.
func (c *HookContext) HookRelation() (jujuc.ContextRelation, error) {
	return c.Relation(c.relationId)
}

// RemoteUnitName returns the name of the remote unit the hook execution
// is associated with if it was found, and an error if it was not found or is not
// available.
// Implements jujuc.RelationHookContext.relationHookContext, part of runner.Context.
func (c *HookContext) RemoteUnitName() (string, error) {
	if c.remoteUnitName == "" {
		return "", errors.NotFoundf("remote unit")
	}
	return c.remoteUnitName, nil
}

// RemoteApplicationName returns the name of the remote application the hook execution
// is associated with if it was found, and an error if it was not found or is not
// available.
// Implements jujuc.RelationHookContext.relationHookContext, part of runner.Context.
func (c *HookContext) RemoteApplicationName() (string, error) {
	if c.remoteApplicationName == "" {
		return "", errors.NotFoundf("saas application")
	}
	return c.remoteApplicationName, nil
}

// Relation returns the relation with the supplied id if it was found, and
// an error if it was not found or is not available.
// Implements jujuc.HookContext.ContextRelations, part of runner.Context.
func (c *HookContext) Relation(id int) (jujuc.ContextRelation, error) {
	r, found := c.relations[id]
	if !found {
		return nil, errors.NotFoundf("relation")
	}
	return r, nil
}

// RelationIds returns the ids of all relations the executing unit is
// currently participating in or an error if they are not available.
// Implements jujuc.HookContext.ContextRelations, part of runner.Context.
func (c *HookContext) RelationIds() ([]int, error) {
	ids := []int{}
	for id, r := range c.relations {
		if r.broken {
			c.logger.Debugf(context.Background(), "relation %d is broken, excluding from relations-ids", id)
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// ActionData returns the context's internal action data. It's meant to be
// transitory; it exists to allow uniter and runner code to keep working as
// it did; it should be considered deprecated, and not used by new clients.
// Implements runner.Context.
func (c *HookContext) ActionData() (*ActionData, error) {
	c.actionDataMu.Lock()
	defer c.actionDataMu.Unlock()
	if c.actionData == nil {
		return nil, errors.New("not running an action")
	}
	return c.actionData, nil
}

// HookVars returns an os.Environ-style list of strings necessary to run a hook
// such that it can know what environment it's operating in, and can call back
// into context.
// Implements runner.Context.
func (c *HookContext) HookVars(
	ctx context.Context,
	paths Paths,
	env Environmenter,
) ([]string, error) {
	vars := c.legacyProxySettings.AsEnvironmentValues()
	vars = append(vars, ContextDependentEnvVars(env)...)

	// TODO(thumper): as work on proxies progress, there will be additional
	// proxy settings to be added.
	vars = append(vars,
		"CHARM_DIR="+paths.GetCharmDir(), // legacy, embarrassing
		"JUJU_CHARM_DIR="+paths.GetCharmDir(),
		"JUJU_CONTEXT_ID="+c.id,
		"JUJU_HOOK_NAME="+c.hookName,
		"JUJU_AGENT_SOCKET_ADDRESS="+paths.GetJujucClientSocket().Address,
		"JUJU_AGENT_SOCKET_NETWORK="+paths.GetJujucClientSocket().Network,
		"JUJU_UNIT_NAME="+c.unitName,
		"JUJU_MODEL_UUID="+c.uuid,
		"JUJU_MODEL_NAME="+c.modelName,
		"JUJU_API_ADDRESSES="+strings.Join(c.apiAddrs, " "),
		"JUJU_MACHINE_ID="+c.assignedMachineTag.Id(),
		"JUJU_PRINCIPAL_UNIT="+c.principal,
		"JUJU_AVAILABILITY_ZONE="+c.availabilityZone,
		"JUJU_VERSION="+version.Current.String(),
		"CLOUD_API_VERSION="+c.cloudAPIVersion,
		// Some of these will be empty, but that is fine, better
		// to explicitly export them as empty.
		"JUJU_CHARM_HTTP_PROXY="+c.jujuProxySettings.Http,
		"JUJU_CHARM_HTTPS_PROXY="+c.jujuProxySettings.Https,
		"JUJU_CHARM_FTP_PROXY="+c.jujuProxySettings.Ftp,
		"JUJU_CHARM_NO_PROXY="+c.jujuProxySettings.NoProxy,
	)
	if r, err := c.HookRelation(); err == nil {
		vars = append(vars,
			"JUJU_RELATION="+r.Name(),
			"JUJU_RELATION_ID="+r.FakeId(),
			"JUJU_REMOTE_UNIT="+c.remoteUnitName,
			"JUJU_REMOTE_APP="+c.remoteApplicationName,
		)

		if c.departingUnitName != "" {
			vars = append(vars,
				"JUJU_DEPARTING_UNIT="+c.departingUnitName,
			)
		}
	} else if !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}
	if c.actionData != nil {
		vars = append(vars,
			"JUJU_ACTION_NAME="+c.actionData.Name,
			"JUJU_ACTION_UUID="+c.actionData.Tag.Id(),
			"JUJU_ACTION_TAG="+c.actionData.Tag.String(),
		)
	}
	if c.workloadName != "" {
		vars = append(vars, "JUJU_WORKLOAD_NAME="+c.workloadName)
		if c.noticeID != "" {
			vars = append(vars,
				"JUJU_NOTICE_ID="+c.noticeID,
				"JUJU_NOTICE_TYPE="+c.noticeType,
				"JUJU_NOTICE_KEY="+c.noticeKey,
			)
		}
		if c.checkName != "" {
			vars = append(vars, "JUJU_PEBBLE_CHECK_NAME="+c.checkName)
		}
	}

	if c.secretURI != "" {
		vars = append(vars,
			"JUJU_SECRET_ID="+c.secretURI,
			"JUJU_SECRET_LABEL="+c.secretLabel,
		)
		if c.secretRevision > 0 {
			vars = append(vars,
				"JUJU_SECRET_REVISION="+strconv.Itoa(c.secretRevision),
			)
		}
	}

	if storage, err := c.HookStorage(ctx); err == nil {
		vars = append(vars,
			"JUJU_STORAGE_ID="+storage.Tag().Id(),
			"JUJU_STORAGE_LOCATION="+storage.Location(),
			"JUJU_STORAGE_KIND="+storage.Kind().String(),
		)
	} else if !errors.Is(err, errors.NotFound) && !errors.Is(err, errors.NotProvisioned) {
		return nil, errors.Trace(err)
	}

	if scope := coretrace.SpanFromContext(ctx).Scope(); scope.TraceID() != "" {
		vars = append(vars,
			"JUJU_TRACE_ID="+scope.TraceID(),
			"JUJU_SPAN_ID="+scope.SpanID(),
			fmt.Sprintf("JUJU_TRACE_FLAGS=%d", scope.TraceFlags()),
		)
	}

	return append(vars, UbuntuEnvVars(paths, env)...), nil
}

func (c *HookContext) handleReboot(ctx context.Context, ctxErr error) error {
	c.logger.Tracef(ctx, "checking for reboot request")
	rebootPriority := c.GetRebootPriority()
	switch rebootPriority {
	case jujuc.RebootSkip:
		return ctxErr
	case jujuc.RebootAfterHook:
		// Reboot should only happen after hook finished successfully.
		if ctxErr != nil {
			return ctxErr
		}
		ctxErr = ErrReboot
	case jujuc.RebootNow:
		ctxErr = ErrRequeueAndReboot
	}

	// Do a best-effort attempt to set the unit agent status; we don't care
	// if it fails as we will request a reboot anyway.
	if err := c.unit.SetAgentStatus(ctx, status.Rebooting, "", nil); err != nil {
		c.logger.Errorf(ctx, "updating agent status: %v", err)
	}

	if err := c.unit.RequestReboot(ctx); err != nil {
		return err
	}

	return ctxErr
}

// Prepare implements the runner.Context interface.
func (c *HookContext) Prepare(ctx context.Context) error {
	if c.actionData != nil {
		err := c.uniter.ActionBegin(ctx, c.actionData.Tag)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// Flush implements the runner.Context interface.
func (c *HookContext) Flush(ctx context.Context, process string, ctxErr error) error {
	// Apply the changes if no error reported while the hook was executing.
	var flushErr error
	if ctxErr == nil {
		flushErr = c.doFlush(ctx, process)
	}

	if c.actionData != nil {
		// While an Action is executing, errors may happen as part of
		// its normal flow. We pass both potential action errors
		// (ctxErr) and any flush errors to finalizeAction helper
		// which will figure out if an error needs to be returned back
		// to the uniter.
		return c.finalizeAction(ctx, ctxErr, flushErr)
	}

	// TODO(gsamfira): Just for now, reboot will not be supported in actions.
	if ctxErr == nil {
		ctxErr = flushErr
	}
	return c.handleReboot(ctx, ctxErr)
}

func (c *HookContext) doFlush(ctx context.Context, process string) error {
	b := uniter.NewCommitHookParamsBuilder(c.unit.Tag())

	// When processing config changed hooks we need to ensure that the
	// relation settings for the unit endpoints are up to date after
	// potential changes to already bound endpoints.
	if process == string(hooks.ConfigChanged) {
		b.UpdateNetworkInfo()
	}

	if c.charmStateCacheDirty {
		b.UpdateCharmState(c.cachedCharmState)
	}

	for _, rc := range c.relations {
		unitSettings, appSettings := rc.FinalSettings()
		if len(unitSettings)+len(appSettings) == 0 {
			continue // no settings need updating
		}
		b.UpdateRelationUnitSettings(rc.RelationTag().String(), unitSettings, appSettings)
	}

	for endpointName, portRanges := range c.portRangeChanges.pendingOpenRanges {
		for _, pr := range portRanges {
			b.OpenPortRange(endpointName, pr)
		}
	}
	for endpointName, portRanges := range c.portRangeChanges.pendingCloseRanges {
		for _, pr := range portRanges {
			b.ClosePortRange(endpointName, pr)
		}
	}

	if len(c.storageAddDirectives) > 0 {
		b.AddStorage(c.storageAddDirectives)
	}

	// Before saving the secret metadata to Juju, save the content to an external
	// backend (if configured) - we need the backend id to send to Juju.
	// If the flush to Juju fails, we'll delete the external content.
	var secretsBackend api.SecretsBackend
	if c.secretChanges.haveContentUpdates() {
		var err error
		secretsBackend, err = c.getSecretsBackend()
		if err != nil {
			return errors.Trace(err)
		}
	}

	var (
		cleanups           []coresecrets.ValueRef
		pendingCreates     []uniter.SecretCreateArg
		pendingUpdates     []uniter.SecretUpsertArg
		pendingDeletes     []uniter.SecretDeleteArg
		pendingGrants      []uniter.SecretGrantRevokeArgs
		pendingRevokes     []uniter.SecretGrantRevokeArgs
		pendingTrackLatest []string
	)
	for _, c := range c.secretChanges.pendingCreates {
		ref, err := secretsBackend.SaveContent(ctx, c.URI, 1, c.Value)
		if errors.Is(err, errors.NotSupported) {
			pendingCreates = append(pendingCreates, c)
			continue
		}
		if err != nil {
			return errors.Annotatef(err, "saving content for secret %q", c.URI.ID)
		}
		cleanups = append(cleanups, ref)
		c.ValueRef = &ref
		c.Value = nil
		pendingCreates = append(pendingCreates, c)
	}
	for _, u := range c.secretChanges.pendingUpdates {
		// Juju checks that the current revision is stable when updating metadata so it's
		// safe to increment here knowing the same value will be saved in Juju.
		if u.Value == nil || u.Value.IsEmpty() {
			pendingUpdates = append(pendingUpdates, u.SecretUpsertArg)
			continue
		}
		ref, err := secretsBackend.SaveContent(ctx, u.URI, u.CurrentRevision+1, u.Value)
		if errors.Is(err, errors.NotSupported) {
			pendingUpdates = append(pendingUpdates, u.SecretUpsertArg)
			continue
		}
		if err != nil {
			return errors.Annotatef(err, "saving content for secret %q", u.URI.ID)
		}
		cleanups = append(cleanups, ref)
		u.ValueRef = &ref
		u.Value = nil
		pendingUpdates = append(pendingUpdates, u.SecretUpsertArg)
	}

	for _, d := range c.secretChanges.pendingDeletes {
		pendingDeletes = append(pendingDeletes, d)
		md, ok := c.secretMetadata[d.URI.ID]
		if !ok {
			continue
		}
		var toDelete []int
		if d.Revisions == nil {
			// Delete all known revisions, this avoids a race condition where a
			// new revision is being created concurrently with us asking to
			// delete existing revisions
			allRevs, err := c.secretsClient.OwnedSecretRevisions(
				ctx,
				c.unit.Tag(), d.URI)
			if err != nil {
				return errors.Annotatef(
					err, "getting revisions for %q", d.URI.ID,
				)
			}
			// Do not delete revisions this hook was unaware of.
			toDelete = slices.DeleteFunc(allRevs, func(rev int) bool {
				return rev > md.LatestRevision
			})
		} else {
			// Delete only the requested revisions
			toDelete = d.Revisions
		}
		// TODO: let the controller know that these secret revisions likely
		// don't exist anymore before attempting to delete their content.
		c.logger.Debugf(ctx, "deleting secret %q provider ids: %v", d.URI.ID, toDelete)
		for _, rev := range toDelete {
			if err := secretsBackend.DeleteContent(ctx, d.URI, rev); err != nil {
				if errors.Is(err, secreterrors.SecretRevisionNotFound) {
					continue
				}
				return errors.Annotatef(err, "cannot delete secret %q revision %d from backend: %v", d.URI.ID, rev, err)
			}
		}
	}

	for _, grants := range c.secretChanges.pendingGrants {
		for _, g := range grants {
			pendingGrants = append(pendingGrants, g)
		}
	}

	for _, revokes := range c.secretChanges.pendingRevokes {
		pendingRevokes = append(pendingRevokes, revokes...)
	}

	for uri := range c.secretChanges.pendingTrackLatest {
		pendingTrackLatest = append(pendingTrackLatest, uri)
	}

	if err := b.AddSecretCreates(pendingCreates); err != nil {
		// Should never happen.
		return errors.Trace(err)
	}
	b.AddSecretUpdates(pendingUpdates)
	b.AddSecretDeletes(pendingDeletes)
	b.AddSecretGrants(pendingGrants)
	b.AddSecretRevokes(pendingRevokes)
	b.AddTrackLatest(pendingTrackLatest)

	// Generate change request but skip its execution if no changes are pending.
	commitReq, numChanges := b.Build()
	if numChanges > 0 {
		if err := c.unit.CommitHookChanges(ctx, commitReq); err != nil {
			c.logger.Errorf(ctx, "cannot apply changes: %v", err)
		cleanupDone:
			for _, secretId := range cleanups {
				if err2 := secretsBackend.DeleteExternalContent(ctx, secretId); err2 != nil {
					if errors.Is(err, errors.NotSupported) {
						break cleanupDone
					}
					c.logger.Errorf(ctx, "cannot cleanup secret %q: %v", secretId, err2)
				}
			}
			return errors.Trace(err)
		}
	}

	// Call completed successfully; update local state
	c.charmStateCacheDirty = false
	return nil
}

// finalizeAction passes back the final status of an Action hook to state.
// It wraps any errors which occurred in normal behavior of the Action run;
// only errors passed in unhandledErr will be returned.
func (c *HookContext) finalizeAction(ctx context.Context, err, flushErr error) error {
	// TODO (binary132): synchronize with gsamfira's reboot logic
	c.actionDataMu.Lock()
	defer c.actionDataMu.Unlock()
	message := c.actionData.ResultsMessage
	results := c.actionData.ResultsMap
	tag := c.actionData.Tag
	actionStatus := params.ActionCompleted
	if c.actionData.Failed {
		select {
		case <-c.actionData.Cancel:
			actionStatus = params.ActionAborted
		default:
			actionStatus = params.ActionFailed
		}
	}

	// If we had an action error, we'll simply encapsulate it in the response
	// and discard the error state.  Actions should not error the uniter.
	if err != nil {
		message = err.Error()
		if charmrunner.IsMissingHookError(err) {
			message = fmt.Sprintf("action not implemented on unit %q", c.unitName)
		}
		select {
		case <-c.actionData.Cancel:
			actionStatus = params.ActionAborted
		default:
			actionStatus = params.ActionFailed
		}
	}
	if flushErr != nil {
		if results == nil {
			results = map[string]interface{}{}
		}
		if stderr, ok := results["stderr"].(string); ok {
			results["stderr"] = stderr + "\n" + flushErr.Error()
		} else {
			results["stderr"] = flushErr.Error()
		}
		if code, ok := results["return-code"]; !ok || code == "0" {
			results["return-code"] = "1"
		}
		actionStatus = params.ActionFailed
		if message == "" {
			message = "committing requested changes failed"
		}
	}

	callErr := c.uniter.ActionFinish(ctx, tag, actionStatus, results, message)
	// Prevent the unit agent from looping if it's impossible to finalise the action.
	if params.IsCodeNotFoundOrCodeUnauthorized(callErr) || params.IsCodeAlreadyExists(callErr) {
		c.logger.Warningf(ctx, "error finalising action %v: %v", tag.Id(), callErr)
		callErr = nil
	}
	return errors.Trace(callErr)
}

// killCharmHook tries to kill the current running charm hook.
func (c *HookContext) killCharmHook(ctx context.Context) error {
	proc := c.GetProcess()
	if proc == nil {
		// nothing to kill
		return charmrunner.ErrNoProcess
	}
	c.logger.Infof(ctx, "trying to kill context process %v", proc.Pid())

	tick := c.clock.After(0)
	timeout := c.clock.After(30 * time.Second)
	for {
		// We repeatedly try to kill the process until we fail; this is
		// because we don't control the *Process, and our clients expect
		// to be able to Wait(); so we can't Wait. We could do better,
		//   but not with a single implementation across all platforms.
		// TODO(gsamfira): come up with a better cross-platform approach.
		select {
		case <-tick:
			err := proc.Kill()
			if err != nil {
				c.logger.Infof(ctx, "kill returned: %s", err)
				c.logger.Infof(ctx, "assuming already killed")
				return nil
			}
		case <-timeout:
			return errors.Errorf("failed to kill context process %v", proc.Pid())
		}
		c.logger.Infof(ctx, "waiting for context process %v to die", proc.Pid())
		tick = c.clock.After(100 * time.Millisecond)
	}
}

// UnitWorkloadVersion returns the version of the workload reported by
// the current unit.
// Implements jujuc.HookContext.ContextVersion, part of runner.Context.
func (c *HookContext) UnitWorkloadVersion(ctx context.Context) (string, error) {
	return c.uniter.UnitWorkloadVersion(ctx, c.unit.Tag())
}

// SetUnitWorkloadVersion sets the current unit's workload version to
// the specified value.
// Implements jujuc.HookContext.ContextVersion, part of runner.Context.
func (c *HookContext) SetUnitWorkloadVersion(ctx context.Context, version string) error {
	return c.uniter.SetUnitWorkloadVersion(ctx, c.unit.Tag(), version)
}

// NetworkInfo returns the network info for the given bindings on the given relation.
// Implements jujuc.HookContext.ContextNetworking, part of runner.Context.
func (c *HookContext) NetworkInfo(ctx context.Context, bindingNames []string, relationId int) (map[string]params.NetworkInfoResult, error) {
	var relId *int
	if relationId != -1 {
		relId = &relationId
	}
	return c.unit.NetworkInfo(ctx, bindingNames, relId)
}

// WorkloadName returns the name of the container/workload for workload hooks.
func (c *HookContext) WorkloadName() (string, error) {
	if c.workloadName == "" {
		return "", errors.NotFoundf("workload name")
	}
	return c.workloadName, nil
}

// WorkloadNoticeType returns the type of the notice for workload notice hooks.
func (ctx *HookContext) WorkloadNoticeType() (string, error) {
	if ctx.noticeType == "" {
		return "", errors.NotFoundf("workload notice type")
	}
	return ctx.noticeType, nil
}

// WorkloadNoticeKey returns the key of the notice for workload notice hooks.
func (ctx *HookContext) WorkloadNoticeKey() (string, error) {
	if ctx.noticeKey == "" {
		return "", errors.NotFoundf("workload notice key")
	}
	return ctx.noticeKey, nil
}

// WorkloadCheckName returns the name of the check for workload check hooks.
func (ctx *HookContext) WorkloadCheckName() (string, error) {
	if ctx.checkName == "" {
		return "", errors.NotFoundf("workload check name")
	}
	return ctx.checkName, nil
}

// SecretURI returns the secret URI for secret hooks.
// This is not yet used by any hook commands - it is exported
// for tests to use.
func (c *HookContext) SecretURI() (string, error) {
	if c.secretURI == "" {
		return "", errors.NotFoundf("secret URI")
	}
	return c.secretURI, nil
}

// SecretLabel returns the secret label for secret hooks.
// This is not yet used by any hook commands - it is exported
// for tests to use.
func (c *HookContext) SecretLabel() string {
	return c.secretLabel
}

// SecretRevision returns the secret revision for secret hooks.
// This is not yet used by any hook commands - it is exported
// for tests to use.
func (c *HookContext) SecretRevision() int {
	return c.secretRevision
}
