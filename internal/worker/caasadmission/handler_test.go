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

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
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

var _ = gc.Suite(&HandlerSuite{})

func (h *HandlerSuite) SetUpTest(c *gc.C) {
	h.logger = loggertesting.WrapCheckLog(c)
}

func (h *HandlerSuite) TestCompareGroupVersionKind(c *gc.C) {
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
		c.Assert(compareGroupVersionKind(test.A, test.B), gc.Equals, test.ShouldMatch)
	}
}

func (h *HandlerSuite) TestEmptyBodyFails(c *gc.C) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	recorder := httptest.NewRecorder()

	admissionHandler(h.logger, &rbacmappertest.Mapper{}, providerconst.LabelVersion1).ServeHTTP(recorder, req)

	c.Assert(recorder.Code, gc.Equals, http.StatusBadRequest)
}

func (h *HandlerSuite) TestUnknownContentType(c *gc.C) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("junk"))
	req.Header.Set("junk", "junk")
	recorder := httptest.NewRecorder()

	admissionHandler(h.logger, &rbacmappertest.Mapper{}, providerconst.LabelVersion1).ServeHTTP(recorder, req)

	c.Assert(recorder.Code, gc.Equals, http.StatusUnsupportedMediaType)
}

func (h *HandlerSuite) TestUnknownServiceAccount(c *gc.C) {
	inReview := &admission.AdmissionReview{
		Request: &admission.AdmissionRequest{
			UID: types.UID("test"),
			UserInfo: authentication.UserInfo{
				UID: "juju-tst-sa",
			},
		},
	}

	body, err := json.Marshal(inReview)
	c.Assert(err, gc.IsNil)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set(HeaderContentType, ExpectedContentType)
	recorder := httptest.NewRecorder()

	admissionHandler(h.logger, &rbacmappertest.Mapper{}, providerconst.LabelVersion1).ServeHTTP(recorder, req)
	c.Assert(recorder.Code, gc.Equals, http.StatusOK)
	c.Assert(recorder.Body, gc.NotNil)

	outReview := admission.AdmissionReview{}
	err = json.Unmarshal(recorder.Body.Bytes(), &outReview)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(outReview.Response.Allowed, jc.IsTrue)
	c.Assert(outReview.Response.UID, gc.Equals, inReview.Request.UID)
}

func (h *HandlerSuite) TestRBACMapperFailure(c *gc.C) {
	inReview := &admission.AdmissionReview{
		Request: &admission.AdmissionRequest{
			UID: types.UID("test"),
			UserInfo: authentication.UserInfo{
				UID: "juju-tst-sa",
			},
		},
	}

	body, err := json.Marshal(inReview)
	c.Assert(err, gc.IsNil)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set(HeaderContentType, ExpectedContentType)
	recorder := httptest.NewRecorder()

	rbacMapper := rbacmappertest.Mapper{
		AppNameForServiceAccountFunc: func(_ types.UID) (string, error) {
			return "", errors.New("test error")
		},
	}

	admissionHandler(h.logger, &rbacMapper, providerconst.LabelVersion1).ServeHTTP(recorder, req)
	c.Assert(recorder.Code, gc.Equals, http.StatusInternalServerError)
}

func (h *HandlerSuite) TestPatchLabelsAdd(c *gc.C) {
	pod := core.Pod{
		ObjectMeta: meta.ObjectMeta{
			Name: "pod",
		},
	}
	podBytes, err := json.Marshal(&pod)
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(recorder.Code, gc.Equals, http.StatusOK)
	c.Assert(recorder.Body, gc.NotNil)

	outReview := admission.AdmissionReview{}
	err = json.Unmarshal(recorder.Body.Bytes(), &outReview)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(outReview.Response.Allowed, jc.IsTrue)
	c.Assert(outReview.Response.UID, gc.Equals, inReview.Request.UID)

	patchOperations := []patchOperation{}
	err = json.Unmarshal(outReview.Response.Patch, &patchOperations)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(len(patchOperations), gc.Equals, 2)
	c.Assert(patchOperations[0].Op, gc.Equals, "add")
	c.Assert(patchOperations[0].Path, gc.Equals, "/metadata/labels")

	expectedLabels := providerutils.LabelForKeyValue(
		providerconst.LabelJujuAppCreatedBy, appName)

	for k, v := range expectedLabels {
		found := false
		for _, patchOp := range patchOperations[1:] {
			if patchOp.Path == fmt.Sprintf("/metadata/labels/%s", patchEscape(k)) {
				c.Assert(patchOp.Op, gc.Equals, "add")
				c.Assert(patchOp.Value, jc.DeepEquals, v)
				found = true
				break
			}
		}
		c.Assert(found, jc.IsTrue)
	}

	for k, v := range expectedLabels {
		found := false
		for _, op := range patchOperations {
			c.Assert(op.Op, gc.Equals, addOp)
			if op.Path == fmt.Sprintf("/metadata/labels/%s", patchEscape(k)) &&
				op.Value.(string) == v {
				found = true
				break
			}
			continue
		}
		c.Assert(found, jc.IsTrue)
	}
}

func (h *HandlerSuite) TestPatchLabelsReplace(c *gc.C) {
	pod := core.Pod{
		ObjectMeta: meta.ObjectMeta{
			Name: "pod",
			Labels: providerutils.LabelForKeyValue(
				providerconst.LabelJujuAppCreatedBy, "replace-app",
			),
		},
	}
	podBytes, err := json.Marshal(&pod)
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(recorder.Code, gc.Equals, http.StatusOK)
	c.Assert(recorder.Body, gc.NotNil)

	outReview := admission.AdmissionReview{}
	err = json.Unmarshal(recorder.Body.Bytes(), &outReview)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(outReview.Response.Allowed, jc.IsTrue)
	c.Assert(outReview.Response.UID, gc.Equals, inReview.Request.UID)

	patchOperations := []patchOperation{}
	err = json.Unmarshal(outReview.Response.Patch, &patchOperations)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(patchOperations), gc.Equals, 1)

	expectedLabels := providerutils.LabelForKeyValue(
		providerconst.LabelJujuAppCreatedBy, appName)
	for k, v := range expectedLabels {
		found := false
		for _, patchOp := range patchOperations {
			if patchOp.Path == fmt.Sprintf("/metadata/labels/%s", patchEscape(k)) {
				c.Assert(patchOp.Op, gc.Equals, "replace")
				c.Assert(patchOp.Value, jc.DeepEquals, v)
				found = true
				break
			}
		}
		c.Assert(found, jc.IsTrue)
	}

	for k, v := range expectedLabels {
		found := false
		for _, op := range patchOperations {
			c.Assert(op.Op, gc.Equals, replaceOp)
			if op.Path == fmt.Sprintf("/metadata/labels/%s", patchEscape(k)) &&
				op.Value.(string) == v {
				found = true
				break
			}
			continue
		}
		c.Assert(found, jc.IsTrue)
	}
}

func (h *HandlerSuite) TestSelfSubjectAccessReviewIgnore(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(recorder.Code, gc.Equals, http.StatusOK)
	c.Assert(recorder.Body, gc.NotNil)

	outReview := admission.AdmissionReview{}
	err = json.Unmarshal(recorder.Body.Bytes(), &outReview)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(outReview.Response.Allowed, jc.IsTrue)
	c.Assert(outReview.Response.UID, gc.Equals, inReview.Request.UID)

	c.Assert(len(outReview.Response.Patch), gc.Equals, 0)
}

func (h *HandlerSuite) TestSelfSubjectAccessReviewIgnoreLabelsV2(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(recorder.Code, gc.Equals, http.StatusOK)
	c.Assert(recorder.Body, gc.NotNil)

	outReview := admission.AdmissionReview{}
	err = json.Unmarshal(recorder.Body.Bytes(), &outReview)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(outReview.Response.Allowed, jc.IsTrue)
	c.Assert(outReview.Response.UID, gc.Equals, inReview.Request.UID)

	c.Assert(len(outReview.Response.Patch), gc.Equals, 0)
}
