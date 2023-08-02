// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/utils/v3"
	"github.com/kr/pretty"
	"golang.org/x/crypto/pbkdf2"

	"github.com/juju/juju/charmhub/path"
	"github.com/juju/juju/charmhub/transport"
	corebase "github.com/juju/juju/core/base"
	charmmetrics "github.com/juju/juju/core/charm/metrics"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/version"
)

// action represents the type of refresh is performed.
type action string

const (
	// installAction defines a install action.
	installAction action = "install"

	// downloadAction defines a download action.
	downloadAction action = "download"

	// refreshAction defines a refresh action.
	refreshAction action = "refresh"
)

var (
	// A set of fields that are always requested when performing refresh calls
	requiredRefreshFields = set.NewStrings(
		"download", "id", "license", "name", "publisher", "resources",
		"revision", "summary", "type", "version", "bases", "config-yaml",
		"metadata-yaml",
	).SortedValues()
)

const (
	// notAvailable is used a placeholder for Name and Channel for a refresh
	// base request, if the Name and Channel is not known.
	notAvailable = "NA"
)

// RefreshBase defines a base for selecting a specific charm.
// Continues to exist to allow for incoming bases to be converted
// to bases inside this package.
type RefreshBase struct {
	Architecture string
	Name         string
	Channel      string
}

func (p RefreshBase) String() string {
	path := p.Architecture
	if p.Channel != "" {
		if p.Name != "" {
			path = fmt.Sprintf("%s/%s", path, p.Name)
		}
		path = fmt.Sprintf("%s/%s", path, p.Channel)
	}
	return path
}

// refreshClient defines a client for refresh requests.
type refreshClient struct {
	path   path.Path
	client RESTClient
	logger Logger
}

// newRefreshClient creates a refreshClient for requesting
func newRefreshClient(path path.Path, client RESTClient, logger Logger) *refreshClient {
	return &refreshClient{
		path:   path,
		client: client,
		logger: logger,
	}
}

// Refresh is used to refresh installed charms to a more suitable revision.
func (c *refreshClient) Refresh(ctx context.Context, config RefreshConfig) ([]transport.RefreshResponse, error) {
	if c.logger.IsTraceEnabled() {
		c.logger.Tracef("Refresh(%s)", pretty.Sprint(config))
	}
	req, err := config.Build()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.refresh(ctx, config.Ensure, req)
}

// RefreshWithRequestMetrics is to get refreshed charm data and provide metrics
// at the same time.  Used as part of the charm revision updater facade.
func (c *refreshClient) RefreshWithRequestMetrics(ctx context.Context, config RefreshConfig, metrics map[charmmetrics.MetricKey]map[charmmetrics.MetricKey]string) ([]transport.RefreshResponse, error) {
	if c.logger.IsTraceEnabled() {
		c.logger.Tracef("RefreshWithRequestMetrics(%s, %+v)", pretty.Sprint(config), metrics)
	}
	req, err := config.Build()
	if err != nil {
		return nil, errors.Trace(err)
	}
	m, err := contextMetrics(metrics)
	if err != nil {
		return nil, errors.Trace(err)
	}
	req.Metrics = m
	return c.refresh(ctx, config.Ensure, req)
}

// RefreshWithMetricsOnly is to provide metrics without context or actions. Used
// as part of the charm revision updater facade.
func (c *refreshClient) RefreshWithMetricsOnly(ctx context.Context, metrics map[charmmetrics.MetricKey]map[charmmetrics.MetricKey]string) error {
	c.logger.Tracef("RefreshWithMetricsOnly(%+v)", metrics)
	m, err := contextMetrics(metrics)
	if err != nil {
		return errors.Trace(err)
	}
	req := transport.RefreshRequest{
		Context: []transport.RefreshRequestContext{},
		Actions: []transport.RefreshRequestAction{},
		Metrics: m,
	}

	// No need to ensure data which is not expected.
	ensure := func(responses []transport.RefreshResponse) error { return nil }

	_, err = c.refresh(ctx, ensure, req)
	return err
}

func contextMetrics(metrics map[charmmetrics.MetricKey]map[charmmetrics.MetricKey]string) (transport.RequestMetrics, error) {
	m := make(transport.RequestMetrics)
	for k, v := range metrics {
		// verify top level "model" and "controller" keys
		if k != charmmetrics.Controller && k != charmmetrics.Model {
			return nil, errors.Trace(errors.NotValidf("highlevel metrics label %q", k))
		}
		ctxM := make(map[string]string, len(v))
		for k2, v2 := range v {
			ctxM[k2.String()] = v2
		}
		m[k.String()] = ctxM
	}
	return m, nil
}

func (c *refreshClient) refresh(ctx context.Context, ensure func(responses []transport.RefreshResponse) error, req transport.RefreshRequest) ([]transport.RefreshResponse, error) {
	httpHeaders := make(http.Header)

	var resp transport.RefreshResponses
	restResp, err := c.client.Post(ctx, c.path, httpHeaders, req, &resp)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if restResp.StatusCode == http.StatusNotFound {
		return nil, logAndReturnError(errors.NotFoundf("refresh"))
	}
	if err := handleBasicAPIErrors(resp.ErrorList, c.logger); err != nil {
		return nil, errors.Trace(err)
	}
	// Ensure that all the results contain the correct instance keys.
	if err := ensure(resp.Results); err != nil {
		return nil, errors.Trace(err)
	}
	// Exit early.
	if len(resp.Results) <= 1 {
		return resp.Results, nil
	}

	// As the results are not expected to be in the correct order, sort them
	// to prevent others falling into not RTFM!
	indexes := make(map[string]int, len(req.Actions))
	for i, action := range req.Actions {
		indexes[action.InstanceKey] = i
	}
	results := make([]transport.RefreshResponse, len(resp.Results))
	for _, result := range resp.Results {
		results[indexes[result.InstanceKey]] = result
	}

	if c.logger.IsTraceEnabled() {
		c.logger.Tracef("Refresh() unmarshalled: %s", pretty.Sprint(results))
	}
	return results, nil
}

// RefreshOne creates a request config for requesting only one charm.
func RefreshOne(key, id string, revision int, channel string, base RefreshBase) (RefreshConfig, error) {
	if id == "" {
		return nil, logAndReturnError(errors.NotValidf("empty id"))
	}
	if key == "" {
		// This is for compatibility reasons.  With older clients, the
		// key created in GetCharmURLOrigin will be lost to and from
		// the client.  Since a key is required, ensure we have one.
		uuid, err := utils.NewUUID()
		if err != nil {
			return nil, logAndReturnError(err)
		}
		key = uuid.String()
	}
	if err := validateBase(base); err != nil {
		return nil, logAndReturnError(err)
	}
	return refreshOne{
		instanceKey: key,
		ID:          id,
		Revision:    revision,
		Channel:     channel,
		Base:        base,
		fields:      requiredRefreshFields,
	}, nil
}

// CreateInstanceKey creates an InstanceKey which can be unique and stable
// from Refresh action to Refresh action.  Required for KPI collection
// on the charmhub side, see LP:1944582.  Rather than saving in
// state, use the model uuid + the app name, which are unique.  Modeled
// after the applicationDoc DocID and globalKey in state.
func CreateInstanceKey(app names.ApplicationTag, model names.ModelTag) string {
	h := pbkdf2.Key([]byte(app.Id()), []byte(model.Id()), 8192, 32, sha512.New)
	return base64.RawURLEncoding.EncodeToString(h)
}

// InstallOneFromRevision creates a request config using the revision and not
// the channel for requesting only one charm.
func InstallOneFromRevision(name string, revision int) (RefreshConfig, error) {
	if name == "" {
		return nil, logAndReturnError(errors.NotValidf("empty name"))
	}
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, logAndReturnError(err)
	}
	return executeOneByRevision{
		action:      installAction,
		instanceKey: uuid.String(),
		Name:        name,
		Revision:    &revision,
		fields:      requiredRefreshFields,
	}, nil
}

// AddResource adds resource revision data to a executeOne config.
// Used for install by revision.
func AddResource(config RefreshConfig, name string, revision int) (RefreshConfig, bool) {
	c, ok := config.(executeOneByRevision)
	if !ok {
		return config, false
	}
	if len(c.resourceRevisions) == 0 {
		c.resourceRevisions = make([]transport.RefreshResourceRevision, 0)
	}
	c.resourceRevisions = append(c.resourceRevisions, transport.RefreshResourceRevision{
		Name:     name,
		Revision: revision,
	})
	return c, true
}

// AddConfigMetrics adds metrics to a refreshOne config.  All values are
// applied at once, subsequent calls, replace all values.
func AddConfigMetrics(config RefreshConfig, metrics map[charmmetrics.MetricKey]string) (RefreshConfig, error) {
	c, ok := config.(refreshOne)
	if !ok {
		return config, nil // error?
	}
	if len(metrics) < 1 {
		return c, nil
	}
	c.metrics = make(transport.ContextMetrics)
	for k, v := range metrics {
		c.metrics[k.String()] = v
	}
	return c, nil
}

// InstallOneFromChannel creates a request config using the channel and not the
// revision for requesting only one charm.
func InstallOneFromChannel(name string, channel string, base RefreshBase) (RefreshConfig, error) {
	if name == "" {
		return nil, logAndReturnError(errors.NotValidf("empty name"))
	}
	if err := validateBase(base); err != nil {
		return nil, logAndReturnError(err)
	}
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, logAndReturnError(err)
	}
	return executeOne{
		action:      installAction,
		instanceKey: uuid.String(),
		Name:        name,
		Channel:     &channel,
		Base:        base,
		fields:      requiredRefreshFields,
	}, nil
}

// DownloadOneFromRevision creates a request config using the revision and not
// the channel for requesting only one charm.
func DownloadOneFromRevision(id string, revision int) (RefreshConfig, error) {
	if id == "" {
		return nil, logAndReturnError(errors.NotValidf("empty id"))
	}
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, logAndReturnError(err)
	}
	return executeOneByRevision{
		action:      downloadAction,
		instanceKey: uuid.String(),
		ID:          id,
		Revision:    &revision,
		fields:      requiredRefreshFields,
	}, nil
}

// DownloadOneFromRevisionByName creates a request config using the revision and not
// the channel for requesting only one charm.
func DownloadOneFromRevisionByName(name string, revision int) (RefreshConfig, error) {
	if name == "" {
		return nil, logAndReturnError(errors.NotValidf("empty name"))
	}
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, logAndReturnError(err)
	}
	return executeOneByRevision{
		action:      downloadAction,
		instanceKey: uuid.String(),
		Name:        name,
		Revision:    &revision,
		fields:      requiredRefreshFields,
	}, nil
}

// DownloadOneFromChannel creates a request config using the channel and not the
// revision for requesting only one charm.
func DownloadOneFromChannel(id string, channel string, base RefreshBase) (RefreshConfig, error) {
	if id == "" {
		return nil, logAndReturnError(errors.NotValidf("empty id"))
	}
	if err := validateBase(base); err != nil {
		return nil, logAndReturnError(err)
	}
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, logAndReturnError(err)
	}
	return executeOne{
		action:      downloadAction,
		instanceKey: uuid.String(),
		ID:          id,
		Channel:     &channel,
		Base:        base,
		fields:      requiredRefreshFields,
	}, nil
}

// DownloadOneFromChannelByName creates a request config using the channel and not the
// revision for requesting only one charm.
func DownloadOneFromChannelByName(name string, channel string, base RefreshBase) (RefreshConfig, error) {
	if name == "" {
		return nil, logAndReturnError(errors.NotValidf("empty name"))
	}
	if err := validateBase(base); err != nil {
		return nil, logAndReturnError(err)
	}
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, logAndReturnError(err)
	}
	return executeOne{
		action:      downloadAction,
		instanceKey: uuid.String(),
		Name:        name,
		Channel:     &channel,
		Base:        base,
		fields:      requiredRefreshFields,
	}, nil
}

// constructRefreshBase creates a refresh request base that allows for
// partial base queries.
func constructRefreshBase(base RefreshBase) (transport.Base, error) {
	if base.Architecture == "" {
		return transport.Base{}, logAndReturnError(errors.NotValidf("refresh arch"))
	}

	name := base.Name
	if name == "" {
		name = notAvailable
	}

	var channel string
	var err error
	switch base.Channel {
	case "":
		channel = notAvailable
	case "kubernetes":
		// Kubernetes is not a valid channel for a base.
		// Instead use the latest LTS version of ubuntu.
		name = "ubuntu"
		channel, err = corebase.SeriesVersion(version.DefaultSupportedLTS())
		if err != nil {
			return transport.Base{}, errors.NotValidf("invalid latest version")
		}
	default:
		// If we have a series, we need to convert it to a stable version.
		// If we have a version, then just pass that through.
		potential, err := corebase.SeriesVersion(base.Channel)
		if err == nil {
			channel = potential
		} else {
			channel = base.Channel
		}
	}

	return transport.Base{
		Architecture: base.Architecture,
		Name:         name,
		Channel:      channel,
	}, nil
}

// validateBase ensures that we do not pass "all" as part of base.
// This function is to help find programming related failures.
func validateBase(rp RefreshBase) error {
	var msg []string
	if rp.Architecture == "all" {
		msg = append(msg, fmt.Sprintf("Architecture %q", rp.Architecture))
	}
	if rp.Name == "all" {
		msg = append(msg, fmt.Sprintf("Name %q", rp.Name))
	}
	if rp.Channel == "all" {
		msg = append(msg, fmt.Sprintf("Channel %q", rp.Channel))
	}
	if len(msg) > 0 {
		return errors.Trace(errors.NotValidf(strings.Join(msg, ", ")))
	}
	return nil
}

type instanceKey interface {
	InstanceKey() string
}

// ExtractConfigInstanceKey is used to get the instance key from a refresh
// config.
func ExtractConfigInstanceKey(cfg RefreshConfig) string {
	key, ok := cfg.(instanceKey)
	if ok {
		return key.InstanceKey()
	}
	return ""
}

// Ideally we'd avoid the package-level logger and use the Client's one, but
// the functions that create a RefreshConfig like RefreshOne don't take
// loggers. This logging can sometimes be quite useful to avoid error sources
// getting lost across the wire, so leave as is for now.
var logger = loggo.GetLoggerWithLabels("juju.charmhub", corelogger.CHARMHUB)

func logAndReturnError(err error) error {
	err = errors.Trace(err)
	logger.Errorf(err.Error())
	return err
}
