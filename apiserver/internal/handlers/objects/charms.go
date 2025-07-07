// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objects

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	jujuerrors "github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	internalhttp "github.com/juju/juju/apiserver/internal/http"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/domain/application/architecture"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/rpc/params"
)

var (
	logger = internallogger.GetLogger("juju.apiserver.objects")
)

const (
	// maxUploadSize is the maximum size of a charm that can be uploaded.
	// TODO (stickupkid): This should be configurable either by a model config
	// or a controller config.
	// This number was derived from the max size of a charm in the charmhub.
	// There are a few larger charms, but they're corrupted (charms within
	// charms - inception!) and should be considered outliers.
	// It's better to have an upper limit rather than no limit at all.
	maxUploadSize = 500 * 1024 * 1024 // 500MB

	// uploadTimeout is the maximum time allowed for a single charm upload.
	// TODO (stickupkid): This should be configurable either by a model config
	// or a controller config.
	uploadTimeout = time.Minute * 5
)

// ApplicationService is an interface for the application domain service.
type ApplicationService interface {
	// GetCharmArchiveBySHA256Prefix returns a ReadCloser stream for the charm
	// archive who's SHA256 hash starts with the provided prefix.
	//
	// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
	// returned.
	GetCharmArchiveBySHA256Prefix(ctx context.Context, sha256Prefix string) (io.ReadCloser, error)

	// ResolveUploadCharm resolves the upload of a charm archive.
	ResolveUploadCharm(context.Context, applicationcharm.ResolveUploadCharm) (applicationcharm.CharmLocator, error)
}

// ApplicationServiceGetter is an interface for getting an ApplicationService.
type ApplicationServiceGetter interface {
	// Application returns the model's application service.
	Application(*http.Request) (ApplicationService, error)
}

// CharmURLMakerFunc is a function that creates a charm URL from a charm
// locator.
type CharmURLMakerFunc func(locator applicationcharm.CharmLocator, includeArchitecture bool) (*charm.URL, error)

// ObjectsCharmHTTPHandler is an http.Handler for the "/objects/charms"
// endpoint.
type ObjectsCharmHTTPHandler struct {
	applicationServiceGetter ApplicationServiceGetter
	makeCharmURL             CharmURLMakerFunc
}

// NewObjectsCharmHTTPHandler returns a new ObjectsCharmHTTPHandler.
func NewObjectsCharmHTTPHandler(
	applicationServiceGetter ApplicationServiceGetter,
	charmURLMaker CharmURLMakerFunc,
) http.Handler {
	return &ObjectsCharmHTTPHandler{
		applicationServiceGetter: applicationServiceGetter,
		makeCharmURL:             charmURLMaker,
	}
}

// ServeHTTP implements the http.Handler interface.
func (h *ObjectsCharmHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	switch r.Method {
	case "GET":
		err = h.ServeGet(w, r)
		if err != nil {
			err = errors.Errorf("retrieving charm: %w", err)
		}
	case "PUT":
		err = h.ServePut(w, r)
		if err != nil {
			err = errors.Errorf("uploading charm: %w", err)
		}
	default:
		http.Error(w, fmt.Sprintf("http method %s not implemented", r.Method), http.StatusNotImplemented)
		return
	}

	if err == nil {
		return
	}

	if err := sendJSONError(w, errors.Capture(err)); err != nil {
		logger.Errorf(r.Context(), "%v", errors.Errorf("returning error to user: %w", err))
	}
}

// ServeGet serves the GET method for the S3 API. This is the equivalent of the
// `GetObject` method in the AWS S3 API.
func (h *ObjectsCharmHTTPHandler) ServeGet(w http.ResponseWriter, r *http.Request) error {
	applicationService, err := h.applicationServiceGetter.Application(r)
	if err != nil {
		return errors.Capture(err)
	}

	query := r.URL.Query()
	_, charmSha256Prefix, err := splitNameAndSHAFromQuery(query)
	if err != nil {
		return err
	}

	reader, err := applicationService.GetCharmArchiveBySHA256Prefix(r.Context(), charmSha256Prefix)
	if errors.Is(err, applicationerrors.CharmNotFound) {
		return jujuerrors.NotFoundf("charm")
	} else if err != nil {
		return errors.Capture(err)
	}
	defer reader.Close()

	_, err = io.Copy(w, reader)
	if err != nil {
		return errors.Errorf("processing charm archive download: %w", err)
	}

	return nil
}

// ServePut serves the PUT method for the S3 API. This is the equivalent of the
// `PutObject` method in the AWS S3 API.
// Since juju's objects (S3) API only acts as a shim, this method will only
// rewrite the http request for it to be correctly processed by the legacy
// '/charms' handler.
func (h *ObjectsCharmHTTPHandler) ServePut(w http.ResponseWriter, r *http.Request) error {
	// Make sure the content type is zip.
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/zip" {
		return jujuerrors.BadRequestf("expected Content-Type: application/zip, got: %v", contentType)
	}

	ctx, cancel := context.WithTimeout(r.Context(), uploadTimeout)
	defer cancel()

	applicationService, err := h.applicationServiceGetter.Application(r)
	if err != nil {
		return errors.Capture(err)
	}

	// Add a charm to the store provider.
	charmURL, err := h.processPut(ctx, r, applicationService)
	if err != nil {
		return jujuerrors.NewBadRequest(err, "")
	}
	headers := map[string]string{
		params.JujuCharmURLHeader: charmURL.String(),
	}
	return errors.Capture(sendStatusAndHeadersAndJSON(w, http.StatusOK, headers, &params.CharmsResponse{CharmURL: charmURL.String()}))
}

func (h *ObjectsCharmHTTPHandler) processPut(ctx context.Context, r *http.Request, applicationService ApplicationService) (*charm.URL, error) {
	name, shaFromQuery, err := splitNameAndSHAFromQuery(r.URL.Query())
	if err != nil {
		return nil, errors.Capture(err)
	}

	curlStr := r.Header.Get(params.JujuCharmURLHeader)
	if curlStr == "" {
		return nil, jujuerrors.BadRequestf("missing %q header", params.JujuCharmURLHeader)
	}
	curl, err := charm.ParseURL(curlStr)
	if err != nil {
		return nil, jujuerrors.BadRequestf("%q is not a valid charm url", curlStr)
	}
	curl.Name = name

	var source corecharm.Source
	switch curl.Schema {
	case charm.CharmHub.String():
		source = corecharm.CharmHub
	case charm.Local.String():
		source = corecharm.Local
	default:
		return nil, jujuerrors.BadRequestf("unsupported charm source %q", curl.Schema)
	}

	locator, err := applicationService.ResolveUploadCharm(ctx, applicationcharm.ResolveUploadCharm{
		Name:         curl.Name,
		Revision:     curl.Revision,
		Source:       source,
		Architecture: curl.Architecture,
		SHA256Prefix: shaFromQuery,

		// Prevent an upload starvation attack by limiting the size of the
		// charm that can be uploaded.
		Reader: io.LimitReader(r.Body, maxUploadSize),
	})
	if errors.Is(err, applicationerrors.CharmNotFound) {
		return nil, jujuerrors.NotFoundf("charm")
	} else if errors.Is(err, applicationerrors.CharmAlreadyAvailable) {
		return nil, jujuerrors.AlreadyExistsf("charm")
	} else if err != nil {
		return nil, errors.Capture(err)
	}

	return h.makeCharmURL(locator, curl.Architecture != "")
}

// CharmURLFromLocator returns a charm URL from a charm locator.
// This will always include the architecture.
func CharmURLFromLocator(locator applicationcharm.CharmLocator, _ bool) (*charm.URL, error) {
	schema, err := convertSource(locator.Source)
	if err != nil {
		return nil, errors.Capture(err)
	}

	architecture, err := encodeArchitecture(locator.Architecture)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return &charm.URL{
		Schema:       schema,
		Name:         locator.Name,
		Revision:     locator.Revision,
		Architecture: architecture,
	}, nil
}

// CharmURLFromLocatorDuringMigration returns a charm URL from a charm locator
// during model migration.
// By including the architecture only when it's passed in, will allow us to
// move forward with prior versions (3.x) not passing in the architecture and
// current versions (4.x) passing in the architecture.
func CharmURLFromLocatorDuringMigration(locator applicationcharm.CharmLocator, includeArchitecture bool) (*charm.URL, error) {
	schema, err := convertSource(locator.Source)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var architecture string
	if includeArchitecture {
		architecture, err = encodeArchitecture(locator.Architecture)
		if err != nil {
			return nil, errors.Capture(err)
		}
	}

	return &charm.URL{
		Schema:       schema,
		Name:         locator.Name,
		Revision:     locator.Revision,
		Architecture: architecture,
	}, nil
}

func splitNameAndSHAFromQuery(query url.Values) (string, string, error) {
	charmObjectID := query.Get(":object")

	// Path param is {charmName}-{charmSha256[0:7]} so we need to split it.
	// NOTE: charmName can contain "-", so we cannot simply strings.Split
	splitIndex := strings.LastIndex(charmObjectID, "-")
	if splitIndex == -1 {
		return "", "", jujuerrors.BadRequestf("%q is not a valid charm object path", charmObjectID)
	}
	name, sha := charmObjectID[:splitIndex], charmObjectID[splitIndex+1:]
	if len(sha) < corecharm.MinSHA256PrefixLength {
		return "", "", jujuerrors.BadRequestf("invalid sha length: %q", sha)
	}
	return name, sha, nil
}

// sendJSONError sends a JSON-encoded error response.  Note the
// difference from the error response sent by the sendError function -
// the error is encoded in the Error field as a string, not an Error
// object.
func sendJSONError(w http.ResponseWriter, err error) error {
	perr, status := apiservererrors.ServerErrorAndStatus(err)
	return errors.Capture(internalhttp.SendStatusAndJSON(w, status, &params.CharmsResponse{
		Error:     perr.Message,
		ErrorCode: perr.Code,
		ErrorInfo: perr.Info,
	}))
}

func convertSource(source applicationcharm.CharmSource) (string, error) {
	switch source {
	case applicationcharm.CharmHubSource:
		return charm.CharmHub.String(), nil
	case applicationcharm.LocalSource:
		return charm.Local.String(), nil
	default:
		return "", errors.Errorf("unsupported source %q", source)
	}
}

func encodeArchitecture(a architecture.Architecture) (string, error) {
	switch a {
	case architecture.AMD64:
		return arch.AMD64, nil
	case architecture.ARM64:
		return arch.ARM64, nil
	case architecture.PPC64EL:
		return arch.PPC64EL, nil
	case architecture.S390X:
		return arch.S390X, nil
	case architecture.RISCV64:
		return arch.RISCV64, nil

	// This is a valid case if we're uploading charms and the value isn't
	// supplied.
	case architecture.Unknown:
		return "", nil
	default:
		return "", errors.Errorf("unsupported architecture %q", a)
	}
}

// sendStatusAndHeadersAndJSON send an HTTP status code, custom headers
// and a JSON-encoded response to a client
func sendStatusAndHeadersAndJSON(w http.ResponseWriter, statusCode int, headers map[string]string, response interface{}) error {
	for k, v := range headers {
		if !strings.HasPrefix(k, "Juju-") {
			return errors.Errorf(`custom header %q must be prefixed with "Juju-"`, k)
		}
		w.Header().Set(k, v)
	}
	return internalhttp.SendStatusAndJSON(w, statusCode, response)
}
