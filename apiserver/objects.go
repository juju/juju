// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	jujuerrors "github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	corecharm "github.com/juju/juju/core/charm"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/downloader"
	"github.com/juju/juju/internal/charm/services"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type objectsCharmHTTPHandler struct {
	ctxt                     httpContext
	stateAuthFunc            func(*http.Request) (*state.PooledState, error)
	applicationServiceGetter ApplicationServiceGetter
}

// ApplicationService is an interface for the application domain service.
type ApplicationService interface {
	// GetCharmArchiveBySHA256Prefix returns a ReadCloser stream for the charm
	// archive who's SHA256 hash starts with the provided prefix.
	//
	// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
	// returned.
	GetCharmArchiveBySHA256Prefix(ctx context.Context, sha256Prefix string) (io.ReadCloser, error)

	// SetCharm persists the charm metadata, actions, config and manifest to
	// state.
	// If there are any non-blocking issues with the charm metadata, actions,
	// config or manifest, a set of warnings will be returned.
	SetCharm(ctx context.Context, args applicationcharm.SetCharmArgs) (corecharm.ID, []string, error)
}

// ApplicationServiceGetter is an interface for getting an ApplicationService.
type ApplicationServiceGetter interface {

	// Application returns the model's application service.
	Application(context.Context) (ApplicationService, error)
}

func (h *objectsCharmHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	switch r.Method {
	case "GET":
		err = h.ServeGet(w, r)
		if err != nil {
			err = errors.Errorf("cannot retrieve charm: %w", err)
		}
	case "PUT":
		err = h.ServePut(w, r)
		if err != nil {
			err = errors.Errorf("cannot upload charm: %w", err)
		}
	default:
		http.Error(w, fmt.Sprintf("http method %s not implemented", r.Method), http.StatusNotImplemented)
		return
	}

	if err != nil {
		if err := sendJSONError(w, r, errors.Capture(err)); err != nil {
			logger.Errorf("%v", errors.Errorf("cannot return error to user: %w", err))
		}
	}
}

// ServeGet serves the GET method for the S3 API. This is the equivalent of the
// `GetObject` method in the AWS S3 API.
func (h *objectsCharmHTTPHandler) ServeGet(w http.ResponseWriter, r *http.Request) error {
	applicationService, err := h.applicationServiceGetter.Application(r.Context())
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
	}
	if err != nil {
		return errors.Capture(err)
	}

	_, err = io.Copy(w, reader)
	if err != nil {
		return errors.Errorf("error processing charm archive download: %w", err)
	}

	return nil
}

// ServePut serves the PUT method for the S3 API. This is the equivalent of the
// `PutObject` method in the AWS S3 API.
// Since juju's objects (S3) API only acts as a shim, this method will only
// rewrite the http request for it to be correctly processed by the legacy
// '/charms' handler.
func (h *objectsCharmHTTPHandler) ServePut(w http.ResponseWriter, r *http.Request) error {
	if r.Method != "PUT" {
		return errors.Capture(emitUnsupportedMethodErr(r.Method))
	}

	// Make sure the content type is zip.
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/zip" {
		return jujuerrors.BadRequestf("expected Content-Type: application/zip, got: %v", contentType)
	}

	st, err := h.stateAuthFunc(r)
	if err != nil {
		return errors.Capture(err)
	}
	defer st.Release()

	applicationService, err := h.applicationServiceGetter.Application(r.Context())
	if err != nil {
		return errors.Capture(err)
	}

	// Add a charm to the store provider.
	charmURL, err := h.processPut(r, st.State, applicationService)
	if err != nil {
		return jujuerrors.NewBadRequest(err, "")
	}
	return errors.Capture(sendStatusAndHeadersAndJSON(w, http.StatusOK, map[string]string{"Juju-Curl": charmURL.String()}, &params.CharmsResponse{CharmURL: charmURL.String()}))
}

func (h *objectsCharmHTTPHandler) processPut(r *http.Request, st *state.State, applicationService ApplicationService) (*charm.URL, error) {
	query := r.URL.Query()
	name, shaFromQuery, err := splitNameAndSHAFromQuery(query)
	if err != nil {
		return nil, errors.Capture(err)
	}

	curlStr := r.Header.Get("Juju-Curl")
	curl, err := charm.ParseURL(curlStr)
	if err != nil {
		return nil, jujuerrors.BadRequestf("%q is not a valid charm url", curlStr)
	}
	curl.Name = name

	schema := curl.Schema
	if schema != "local" {
		// charmhub charms may only be uploaded into models
		// which are being imported during model migrations.
		// There's currently no other time where it makes sense
		// to accept repository charms through this endpoint.
		if isImporting, err := modelIsImporting(st); err != nil {
			return nil, errors.Capture(err)
		} else if !isImporting {
			return nil, errors.New("non-local charms may only be uploaded during model migration import")
		}
	}

	// Attempt to get the object store early, so we're not unnecessarily
	// creating a parsing/reading if we can't get the object store.
	objectStore, err := h.ctxt.objectStoreForRequest(r.Context())
	if err != nil {
		return nil, errors.Capture(err)
	}

	hash := sha256.New()
	var charmArchiveBuf bytes.Buffer
	_, err = io.Copy(io.MultiWriter(hash, &charmArchiveBuf), r.Body)
	if err != nil {
		return nil, errors.Errorf("reading charm archive upload: %w", err)
	}

	charmSHA := hex.EncodeToString(hash.Sum(nil))

	// Charm refs only use the first 7 chars. So truncate the sha to match
	if sha := charmSHA[0:7]; sha != shaFromQuery {
		return nil, jujuerrors.BadRequestf("Uploaded charm sha256 (%v) does not match sha in url (%v)", sha, shaFromQuery)
	}

	archive, err := charm.ReadCharmArchiveBytes(charmArchiveBuf.Bytes())
	if err != nil {
		return nil, jujuerrors.BadRequestf("invalid charm archive: %v", err)
	}

	if curl.Revision == -1 {
		curl.Revision = archive.Revision()
	}

	source := charm.Schema(schema)
	switch source {
	case charm.Local:
		curl, err = st.PrepareLocalCharmUpload(curl.String())
		if err != nil {
			return nil, errors.Capture(err)
		}

	case charm.CharmHub:
		if _, err := st.PrepareCharmUpload(curl.String()); err != nil {
			return nil, errors.Capture(err)
		}

	default:
		return nil, errors.Errorf("unsupported schema %q", schema)
	}

	if archive.Revision() != curl.Revision {
		archive, charmArchiveBuf, charmSHA, err = repackageCharmWithRevision(archive, curl.Revision)
		if err != nil {
			return nil, errors.Capture(err)
		}
	}

	charmStorage := services.NewCharmStorage(services.CharmStorageConfig{
		Logger:       logger,
		StateBackend: storageStateShim{State: st},
		ObjectStore:  objectStore,
	})

	storagePath, err := charmStorage.Store(r.Context(), curl.String(), downloader.DownloadedCharm{
		Charm:        archive,
		CharmData:    &charmArchiveBuf,
		CharmVersion: archive.Version(),
		Size:         int64(charmArchiveBuf.Len()),
		SHA256:       charmSHA,
		LXDProfile:   archive.LXDProfile(),
	})

	if err != nil {
		return nil, errors.Errorf("cannot store charm: %w", err)
	}

	// Dual write the charm to the service.
	// TODO(stickupkid): There should be a SetCharm method on a charm
	// service, which increments the charm revision and uploads the charm to
	// the object store.
	// This can be done, once all the charm service methods are being used,
	// instead of the state methods.
	csSource := corecharm.CharmHub
	provenance := applicationcharm.ProvenanceMigration
	if source == charm.Local {
		csSource = corecharm.Local
		provenance = applicationcharm.ProvenanceUpload
	}

	if _, _, err := applicationService.SetCharm(r.Context(), applicationcharm.SetCharmArgs{
		Charm:         archive,
		Source:        csSource,
		ReferenceName: curl.Name,
		Revision:      curl.Revision,
		Hash:          charmSHA,
		ArchivePath:   storagePath,
		Version:       archive.Version(),
		Architecture:  curl.Architecture,
		// If this is a charmhub charm, this will be coming from a migration.
		// We can not re-download this charm from the charm store again, without
		// another call directly to the charm store.
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance: provenance,
		},
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return curl, nil
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
	return name, sha, nil
}

// sendJSONError sends a JSON-encoded error response.  Note the
// difference from the error response sent by the sendError function -
// the error is encoded in the Error field as a string, not an Error
// object.
func sendJSONError(w http.ResponseWriter, req *http.Request, err error) error {

	perr, status := apiservererrors.ServerErrorAndStatus(err)
	return errors.Capture(sendStatusAndJSON(w, status, &params.CharmsResponse{
		Error:     perr.Message,
		ErrorCode: perr.Code,
		ErrorInfo: perr.Info,
	}))
}

func modelIsImporting(st *state.State) (bool, error) {
	model, err := st.Model()
	if err != nil {
		return false, errors.Capture(err)
	}
	return model.MigrationMode() == state.MigrationModeImporting, nil
}

func emitUnsupportedMethodErr(method string) error {
	return jujuerrors.MethodNotAllowedf("unsupported method: %q", method)
}

type storageStateShim struct {
	*state.State
}

func (s storageStateShim) UpdateUploadedCharm(info state.CharmInfo) (services.UploadedCharm, error) {
	ch, err := s.State.UpdateUploadedCharm(info)
	return ch, err
}

func (s storageStateShim) PrepareCharmUpload(curl string) (services.UploadedCharm, error) {
	ch, err := s.State.PrepareCharmUpload(curl)
	return ch, err
}

type applicationServiceGetter struct {
	ctxt httpContext
}

func (a *applicationServiceGetter) Application(ctx context.Context) (ApplicationService, error) {
	domainServices, err := a.ctxt.domainServicesForRequest(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return domainServices.Application(), nil
}
