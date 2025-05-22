// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasadmission

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/juju/tc"
	admission "k8s.io/api/admission/v1beta1"
	authentication "k8s.io/api/authentication/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	providerconst "github.com/juju/juju/caas/kubernetes/provider/constants"
	providerutils "github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	rbacmappertest "github.com/juju/juju/internal/worker/caasrbacmapper/test"
)

type HandlerSuite struct {
	logger logger.Logger
}

func TestHandlerSuite(t *testing.T) {
	tc.Run(t, &HandlerSuite{})
}

func (h *HandlerSuite) SetUpTest(c *tc.C) {
	h.logger = loggertesting.WrapCheckLog(c)
}

func (h *HandlerSuite) TestCompareGroupVersionKind(c *tc.C) {
	tests := []struct {
		A           *schema.GroupVersionKind
		B           *schema.GroupVersionKind
		ShouldMatch bool
	}{
		{
			A: &schema.GroupVersionKind{
				Group:   admission.SchemeGroupVersion.Group,
				Version: admission.SchemeGroupVersion.Version,
				Kind:    "AdmissionReview",
			},
			B: &schema.GroupVersionKind{
				Group:   admission.SchemeGroupVersion.Group,
				Version: admission.SchemeGroupVersion.Version,
				Kind:    "AdmissionReview",
			},
			ShouldMatch: true,
		},
		{
			A: &schema.GroupVersionKind{
				Group:   admission.SchemeGroupVersion.Group,
				Version: admission.SchemeGroupVersion.Version,
				Kind:    "AdmissionReview",
			},
			B: &schema.GroupVersionKind{
				Group:   admission.SchemeGroupVersion.Group,
				Version: admission.SchemeGroupVersion.Version,
				Kind:    "Junk",
			},
			ShouldMatch: false,
		},
		{
			A: &schema.GroupVersionKind{
				Group:   admission.SchemeGroupVersion.Group,
				Version: admission.SchemeGroupVersion.Version,
				Kind:    "AdmissionReview",
			},
			B:           nil,
			ShouldMatch: false,
		},
	}

	for _, test := range tests {
		c.Assert(compareGroupVersionKind(test.A, test.B), tc.Equals, test.ShouldMatch)
	}
}

func (h *HandlerSuite) TestEmptyBodyFails(c *tc.C) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	recorder := httptest.NewRecorder()

	admissionHandler(h.logger, &rbacmappertest.Mapper{}, providerconst.LabelVersion1).ServeHTTP(recorder, req)

	c.Assert(recorder.Code, tc.Equals, http.StatusBadRequest)
}

func (h *HandlerSuite) TestUnknownContentType(c *tc.C) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("junk"))
	req.Header.Set("junk", "junk")
	recorder := httptest.NewRecorder()

	admissionHandler(h.logger, &rbacmappertest.Mapper{}, providerconst.LabelVersion1).ServeHTTP(recorder, req)

	c.Assert(recorder.Code, tc.Equals, http.StatusUnsupportedMediaType)
}

func (h *HandlerSuite) TestUnknownServiceAccount(c *tc.C) {
	inReview := &admission.AdmissionReview{
		Request: &admission.AdmissionRequest{
			UID: types.UID("test"),
			UserInfo: authentication.UserInfo{
				UID: "juju-tst-sa",
			},
		},
	}

	body, err := json.Marshal(inReview)
	c.Assert(err, tc.IsNil)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set(HeaderContentType, ExpectedContentType)
	recorder := httptest.NewRecorder()

	admissionHandler(h.logger, &rbacmappertest.Mapper{}, providerconst.LabelVersion1).ServeHTTP(recorder, req)
	c.Assert(recorder.Code, tc.Equals, http.StatusOK)
	c.Assert(recorder.Body, tc.NotNil)

	outReview := admission.AdmissionReview{}
	err = json.Unmarshal(recorder.Body.Bytes(), &outReview)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(outReview.Response.Allowed, tc.IsTrue)
	c.Assert(outReview.Response.UID, tc.Equals, inReview.Request.UID)
}

func (h *HandlerSuite) TestRBACMapperFailure(c *tc.C) {
	inReview := &admission.AdmissionReview{
		Request: &admission.AdmissionRequest{
			UID: types.UID("test"),
			UserInfo: authentication.UserInfo{
				UID: "juju-tst-sa",
			},
		},
	}

	body, err := json.Marshal(inReview)
	c.Assert(err, tc.IsNil)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set(HeaderContentType, ExpectedContentType)
	recorder := httptest.NewRecorder()

	rbacMapper := rbacmappertest.Mapper{
		AppNameForServiceAccountFunc: func(_ types.UID) (string, error) {
			return "", errors.New("test error")
		},
	}

	admissionHandler(h.logger, &rbacMapper, providerconst.LabelVersion1).ServeHTTP(recorder, req)
	c.Assert(recorder.Code, tc.Equals, http.StatusInternalServerError)
}

func (h *HandlerSuite) TestPatchLabelsAdd(c *tc.C) {
	pod := core.Pod{
		ObjectMeta: meta.ObjectMeta{
			Name: "pod",
		},
	}
	podBytes, err := json.Marshal(&pod)
	c.Assert(err, tc.ErrorIsNil)

	inReview := &admission.AdmissionReview{
		Request: &admission.AdmissionRequest{
			UID: types.UID("test"),
			UserInfo: authentication.UserInfo{
				UID: "juju-tst-sa",
			},
			Object: runtime.RawExtension{
				Raw: podBytes,
			},
		},
	}

	body, err := json.Marshal(inReview)
	c.Assert(err, tc.ErrorIsNil)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set(HeaderContentType, ExpectedContentType)
	recorder := httptest.NewRecorder()

	appName := "test-app"
	rbacMapper := rbacmappertest.Mapper{
		AppNameForServiceAccountFunc: func(_ types.UID) (string, error) {
			return appName, nil
		},
	}

	admissionHandler(h.logger, &rbacMapper, providerconst.LabelVersion1).ServeHTTP(recorder, req)
	c.Assert(recorder.Code, tc.Equals, http.StatusOK)
	c.Assert(recorder.Body, tc.NotNil)

	outReview := admission.AdmissionReview{}
	err = json.Unmarshal(recorder.Body.Bytes(), &outReview)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(outReview.Response.Allowed, tc.IsTrue)
	c.Assert(outReview.Response.UID, tc.Equals, inReview.Request.UID)

	patchOperations := []patchOperation{}
	err = json.Unmarshal(outReview.Response.Patch, &patchOperations)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(len(patchOperations), tc.Equals, 2)
	c.Assert(patchOperations[0].Op, tc.Equals, "add")
	c.Assert(patchOperations[0].Path, tc.Equals, "/metadata/labels")

	expectedLabels := providerutils.LabelForKeyValue(
		providerconst.LabelJujuAppCreatedBy, appName)

	for k, v := range expectedLabels {
		found := false
		for _, patchOp := range patchOperations[1:] {
			if patchOp.Path == fmt.Sprintf("/metadata/labels/%s", patchEscape(k)) {
				c.Assert(patchOp.Op, tc.Equals, "add")
				c.Assert(patchOp.Value, tc.DeepEquals, v)
				found = true
				break
			}
		}
		c.Assert(found, tc.IsTrue)
	}

	for k, v := range expectedLabels {
		found := false
		for _, op := range patchOperations {
			c.Assert(op.Op, tc.Equals, addOp)
			if op.Path == fmt.Sprintf("/metadata/labels/%s", patchEscape(k)) &&
				op.Value.(string) == v {
				found = true
				break
			}
			continue
		}
		c.Assert(found, tc.IsTrue)
	}
}

func (h *HandlerSuite) TestPatchLabelsReplace(c *tc.C) {
	pod := core.Pod{
		ObjectMeta: meta.ObjectMeta{
			Name: "pod",
			Labels: providerutils.LabelForKeyValue(
				providerconst.LabelJujuAppCreatedBy, "replace-app",
			),
		},
	}
	podBytes, err := json.Marshal(&pod)
	c.Assert(err, tc.ErrorIsNil)

	inReview := &admission.AdmissionReview{
		Request: &admission.AdmissionRequest{
			UID: types.UID("test"),
			UserInfo: authentication.UserInfo{
				UID: "juju-tst-sa",
			},
			Object: runtime.RawExtension{
				Raw: podBytes,
			},
		},
	}

	body, err := json.Marshal(inReview)
	c.Assert(err, tc.ErrorIsNil)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set(HeaderContentType, ExpectedContentType)
	recorder := httptest.NewRecorder()

	appName := "test-app"
	rbacMapper := rbacmappertest.Mapper{
		AppNameForServiceAccountFunc: func(_ types.UID) (string, error) {
			return appName, nil
		},
	}

	admissionHandler(h.logger, &rbacMapper, providerconst.LabelVersion1).ServeHTTP(recorder, req)
	c.Assert(recorder.Code, tc.Equals, http.StatusOK)
	c.Assert(recorder.Body, tc.NotNil)

	outReview := admission.AdmissionReview{}
	err = json.Unmarshal(recorder.Body.Bytes(), &outReview)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(outReview.Response.Allowed, tc.IsTrue)
	c.Assert(outReview.Response.UID, tc.Equals, inReview.Request.UID)

	patchOperations := []patchOperation{}
	err = json.Unmarshal(outReview.Response.Patch, &patchOperations)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(patchOperations), tc.Equals, 1)

	expectedLabels := providerutils.LabelForKeyValue(
		providerconst.LabelJujuAppCreatedBy, appName)
	for k, v := range expectedLabels {
		found := false
		for _, patchOp := range patchOperations {
			if patchOp.Path == fmt.Sprintf("/metadata/labels/%s", patchEscape(k)) {
				c.Assert(patchOp.Op, tc.Equals, "replace")
				c.Assert(patchOp.Value, tc.DeepEquals, v)
				found = true
				break
			}
		}
		c.Assert(found, tc.IsTrue)
	}

	for k, v := range expectedLabels {
		found := false
		for _, op := range patchOperations {
			c.Assert(op.Op, tc.Equals, replaceOp)
			if op.Path == fmt.Sprintf("/metadata/labels/%s", patchEscape(k)) &&
				op.Value.(string) == v {
				found = true
				break
			}
			continue
		}
		c.Assert(found, tc.IsTrue)
	}
}

func (h *HandlerSuite) TestSelfSubjectAccessReviewIgnore(c *tc.C) {
	inReview := &admission.AdmissionReview{
		Request: &admission.AdmissionRequest{
			Kind: meta.GroupVersionKind{
				Group:   "authorization.k8s.io",
				Kind:    "SelfSubjectAccessReview",
				Version: "v1",
			},
			UID: types.UID("test"),
			UserInfo: authentication.UserInfo{
				UID: "juju-tst-sa",
			},
		},
	}

	body, err := json.Marshal(inReview)
	c.Assert(err, tc.ErrorIsNil)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set(HeaderContentType, ExpectedContentType)
	recorder := httptest.NewRecorder()

	appName := "test-app"
	rbacMapper := rbacmappertest.Mapper{
		AppNameForServiceAccountFunc: func(_ types.UID) (string, error) {
			return appName, nil
		},
	}

	admissionHandler(h.logger, &rbacMapper, providerconst.LabelVersion1).ServeHTTP(recorder, req)
	c.Assert(recorder.Code, tc.Equals, http.StatusOK)
	c.Assert(recorder.Body, tc.NotNil)

	outReview := admission.AdmissionReview{}
	err = json.Unmarshal(recorder.Body.Bytes(), &outReview)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(outReview.Response.Allowed, tc.IsTrue)
	c.Assert(outReview.Response.UID, tc.Equals, inReview.Request.UID)

	c.Assert(len(outReview.Response.Patch), tc.Equals, 0)
}

func (h *HandlerSuite) TestSelfSubjectAccessReviewIgnoreLabelsV2(c *tc.C) {
	inReview := &admission.AdmissionReview{
		Request: &admission.AdmissionRequest{
			Kind: meta.GroupVersionKind{
				Group:   "authorization.k8s.io",
				Kind:    "SelfSubjectAccessReview",
				Version: "v1",
			},
			UID: types.UID("test"),
			UserInfo: authentication.UserInfo{
				UID: "juju-tst-sa",
			},
		},
	}

	body, err := json.Marshal(inReview)
	c.Assert(err, tc.ErrorIsNil)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set(HeaderContentType, ExpectedContentType)
	recorder := httptest.NewRecorder()

	appName := "test-app"
	rbacMapper := rbacmappertest.Mapper{
		AppNameForServiceAccountFunc: func(_ types.UID) (string, error) {
			return appName, nil
		},
	}

	admissionHandler(h.logger, &rbacMapper, providerconst.LabelVersion2).ServeHTTP(recorder, req)
	c.Assert(recorder.Code, tc.Equals, http.StatusOK)
	c.Assert(recorder.Body, tc.NotNil)

	outReview := admission.AdmissionReview{}
	err = json.Unmarshal(recorder.Body.Bytes(), &outReview)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(outReview.Response.Allowed, tc.IsTrue)
	c.Assert(outReview.Response.UID, tc.Equals, inReview.Request.UID)

	c.Assert(len(outReview.Response.Patch), tc.Equals, 0)
}
