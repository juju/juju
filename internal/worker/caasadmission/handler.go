// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasadmission

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/juju/errors"
	admission "k8s.io/api/admission/v1beta1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"

	providerconst "github.com/juju/juju/caas/kubernetes/provider/constants"
	providerutils "github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/core/logger"
)

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

type RBACMapper interface {
	// AppNameForServiceAccount fetches the juju application name associated
	// with a given kubernetes service account UID. If no result is found
	// errors.NotFound is returned. All other errors should be considered
	// internal to the interface operation.
	AppNameForServiceAccount(types.UID) (string, error)
}

const (
	ExpectedContentType = "application/json"
	HeaderContentType   = "Content-Type"
	addOp               = "add"
	replaceOp           = "replace"
)

var (
	AdmissionGVK = schema.GroupVersionKind{
		Group:   admission.SchemeGroupVersion.Group,
		Version: admission.SchemeGroupVersion.Version,
		Kind:    "AdmissionReview",
	}
)

func admissionHandler(logger logger.Logger, rbacMapper RBACMapper, labelVersion providerconst.LabelVersion) http.Handler {
	codecFactory := serializer.NewCodecFactory(runtime.NewScheme())

	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		data, err := io.ReadAll(req.Body)
		if err != nil {
			logger.Errorf(req.Context(), "digesting admission request body: %v", err)
			http.Error(res, fmt.Sprintf("%s: reading request body",
				http.StatusText(http.StatusInternalServerError)), http.StatusInternalServerError)
			return
		}

		if len(data) == 0 {
			http.Error(res, fmt.Sprintf("%s: empty request body",
				http.StatusText(http.StatusBadRequest)), http.StatusBadRequest)
			return
		}

		if req.Header.Get(HeaderContentType) != ExpectedContentType {
			http.Error(res, fmt.Sprintf("%s: supported content types = [%s]",
				http.StatusText(http.StatusUnsupportedMediaType),
				ExpectedContentType), http.StatusUnsupportedMediaType)
			return
		}

		finalise := func(review *admission.AdmissionReview, response *admission.AdmissionResponse) {
			var uid types.UID
			if review != nil && review.Request != nil {
				uid = review.Request.UID
			}
			response.UID = uid

			body, err := json.Marshal(admission.AdmissionReview{
				Response: response,
			})
			if err != nil {
				logger.Errorf(req.Context(), "marshaling admission request response body: %v", err)
				http.Error(res, fmt.Sprintf("%s: building response body",
					http.StatusText(http.StatusInternalServerError)), http.StatusInternalServerError)
			}
			if _, err := res.Write(body); err != nil {
				logger.Errorf(req.Context(), "writing admission request response body: %v", err)
			}
		}

		admissionReview := &admission.AdmissionReview{}
		obj, _, err := codecFactory.UniversalDecoder().Decode(data, nil, admissionReview)
		if err != nil {
			finalise(admissionReview, errToAdmissionResponse(err))
			return
		}

		var ok bool
		if admissionReview, ok = obj.(*admission.AdmissionReview); !ok {
			finalise(admissionReview,
				errToAdmissionResponse(errors.NewNotValid(nil, "converting admission request")))
			return
		}

		logger.Debugf(req.Context(), "received admission request for %s of %s in namespace %s",
			admissionReview.Request.Name,
			admissionReview.Request.Kind,
			admissionReview.Request.Namespace,
		)

		reviewResponse := &admission.AdmissionResponse{
			Allowed: true,
		}

		for _, ignoreObjKind := range admissionObjectIgnores {
			if compareAPIGroupVersionKind(ignoreObjKind, admissionReview.Request.Kind) {
				logger.Debugf(req.Context(), "ignoring admission request for gvk %s", ignoreObjKind)
				finalise(admissionReview, reviewResponse)
				return
			}
		}

		appName, err := rbacMapper.AppNameForServiceAccount(
			types.UID(admissionReview.Request.UserInfo.UID))
		if err != nil && !errors.Is(err, errors.NotFound) {
			http.Error(res, fmt.Sprintf(
				"could not determine if admission request belongs to juju: %v", err,
			),
				http.StatusInternalServerError)
			return
		} else if errors.Is(err, errors.NotFound) {
			finalise(admissionReview, reviewResponse)
			return
		}

		metaObj := struct {
			meta.ObjectMeta `json:"metadata,omitempty"`
		}{}

		err = json.Unmarshal(admissionReview.Request.Object.Raw, &metaObj)
		if err != nil {
			http.Error(res,
				fmt.Sprintf("unmarshalling admission object from json: %v", err),
				http.StatusInternalServerError)
			return
		}

		patchJSON, err := json.Marshal(
			patchForLabels(metaObj.Labels, appName, labelVersion))
		if err != nil {
			http.Error(res,
				fmt.Sprintf("marshalling patch object to json: %v", err),
				http.StatusInternalServerError)
			return
		}

		patchType := admission.PatchTypeJSONPatch
		reviewResponse.Patch = patchJSON
		reviewResponse.PatchType = &patchType
		finalise(admissionReview, reviewResponse)
	})
}

func compareGroupVersionKind(a, b *schema.GroupVersionKind) bool {
	if a == nil || b == nil {
		return false
	}
	return a.Group == b.Group && a.Version == b.Version && a.Kind == b.Kind
}

func errToAdmissionResponse(err error) *admission.AdmissionResponse {
	return &admission.AdmissionResponse{
		Allowed: false,
		Result: &meta.Status{
			Message: err.Error(),
		},
	}
}

func patchEscape(s string) string {
	r := strings.Replace(s, "~", "~0", -1)
	r = strings.Replace(r, "/", "~1", -1)
	return r
}

func patchForLabels(
	labels map[string]string,
	appName string,
	labelVersion providerconst.LabelVersion) []patchOperation {
	patches := []patchOperation{}

	neededLabels := providerutils.LabelForKeyValue(
		providerconst.LabelJujuAppCreatedBy, appName)

	if len(labels) == 0 {
		patches = append(patches, patchOperation{
			Op:    addOp,
			Path:  "/metadata/labels",
			Value: map[string]string{},
		})
	}

	for k, v := range neededLabels {
		if extVal, found := labels[k]; found && extVal != v {
			patches = append(patches, patchOperation{
				Op:    replaceOp,
				Path:  fmt.Sprintf("/metadata/labels/%s", patchEscape(k)),
				Value: patchEscape(v),
			})
		} else if !found {
			patches = append(patches, patchOperation{
				Op:    addOp,
				Path:  fmt.Sprintf("/metadata/labels/%s", patchEscape(k)),
				Value: patchEscape(v),
			})
		}
	}

	return patches
}
