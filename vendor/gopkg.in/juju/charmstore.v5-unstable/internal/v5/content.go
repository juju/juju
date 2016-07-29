// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v5 // import "gopkg.in/juju/charmstore.v5-unstable/internal/v5"

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/juju/xml"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/juju/jujusvg.v1"

	"gopkg.in/juju/charmstore.v5-unstable/internal/charmstore"
	"gopkg.in/juju/charmstore.v5-unstable/internal/mongodoc"
	"gopkg.in/juju/charmstore.v5-unstable/internal/router"
)

// GET id/diagram.svg
// https://github.com/juju/charmstore/blob/v4/docs/API.md#get-iddiagramsvg
func (h *ReqHandler) serveDiagram(id *router.ResolvedURL, w http.ResponseWriter, req *http.Request) error {
	if id.URL.Series != "bundle" {
		return errgo.WithCausef(nil, params.ErrNotFound, "diagrams not supported for charms")
	}
	entity, err := h.Cache.Entity(&id.URL, charmstore.FieldSelector("bundledata"))
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}

	var urlErr error
	// TODO consider what happens when a charm's SVG does not exist.
	canvas, err := jujusvg.NewFromBundle(entity.BundleData, func(id *charm.URL) string {
		// TODO change jujusvg so that the iconURL function can
		// return an error.
		absPath := h.Handler.rootPath + "/" + id.Path() + "/icon.svg"
		p, err := router.RelativeURLPath(req.RequestURI, absPath)
		if err != nil {
			urlErr = errgo.Notef(err, "cannot make relative URL from %q and %q", req.RequestURI, absPath)
		}
		return p
	}, nil)
	if err != nil {
		return errgo.Notef(err, "cannot create canvas")
	}
	if urlErr != nil {
		return urlErr
	}
	setArchiveCacheControl(w.Header(), h.isPublic(id))
	w.Header().Set("Content-Type", "image/svg+xml")
	canvas.Marshal(w)
	return nil
}

// These are all forms of README files
// actually observed in charms in the wild.
var allowedReadMe = map[string]bool{
	"readme":          true,
	"readme.md":       true,
	"readme.rst":      true,
	"readme.ex":       true,
	"readme.markdown": true,
	"readme.txt":      true,
}

// GET id/readme
// https://github.com/juju/charmstore/blob/v4/docs/API.md#get-idreadme
func (h *ReqHandler) serveReadMe(id *router.ResolvedURL, w http.ResponseWriter, req *http.Request) error {
	entity, err := h.Cache.Entity(&id.URL, charmstore.FieldSelector("contents", "blobname"))
	if err != nil {
		return errgo.NoteMask(err, "cannot get README", errgo.Is(params.ErrNotFound))
	}
	isReadMeFile := func(f *zip.File) bool {
		name := strings.ToLower(path.Clean(f.Name))
		// This is the same condition currently used by the GUI.
		// TODO propagate likely content type from file extension.
		return allowedReadMe[name]
	}
	r, err := h.Store.OpenCachedBlobFile(entity, mongodoc.FileReadMe, isReadMeFile)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	defer r.Close()
	setArchiveCacheControl(w.Header(), h.isPublic(id))
	io.Copy(w, r)
	return nil
}

// GET id/icon.svg
// https://github.com/juju/charmstore/blob/v4/docs/API.md#get-idiconsvg
func (h *ReqHandler) serveIcon(id *router.ResolvedURL, w http.ResponseWriter, req *http.Request) error {
	if id.URL.Series == "bundle" {
		return errgo.WithCausef(nil, params.ErrNotFound, "icons not supported for bundles")
	}
	entity, err := h.Cache.Entity(&id.URL, charmstore.FieldSelector("contents", "blobname"))
	if err != nil {
		return errgo.NoteMask(err, "cannot get icon", errgo.Is(params.ErrNotFound))
	}
	isIconFile := func(f *zip.File) bool {
		return path.Clean(f.Name) == "icon.svg"
	}
	r, err := h.Store.OpenCachedBlobFile(entity, mongodoc.FileIcon, isIconFile)
	if err != nil {
		logger.Errorf("cannot open icon.svg file for %v: %v", id, err)
		if errgo.Cause(err) != params.ErrNotFound {
			return errgo.Mask(err)
		}
		setArchiveCacheControl(w.Header(), h.isPublic(id))
		w.Header().Set("Content-Type", "image/svg+xml")
		io.Copy(w, strings.NewReader(DefaultIcon))
		return nil
	}
	defer r.Close()
	w.Header().Set("Content-Type", "image/svg+xml")
	setArchiveCacheControl(w.Header(), h.isPublic(id))
	if err := processIcon(w, r); err != nil {
		if errgo.Cause(err) == errProbablyNotXML {
			logger.Errorf("cannot process icon.svg from %s: %v", id, err)
			io.Copy(w, strings.NewReader(DefaultIcon))
			return nil
		}
		return errgo.Mask(err)
	}
	return nil
}

var errProbablyNotXML = errgo.New("probably not XML")

const svgNamespace = "http://www.w3.org/2000/svg"

// processIcon reads an icon SVG from r and writes
// it to w, making any changes that need to be made.
// Currently it adds a viewBox attribute to the <svg>
// element if necessary.
// If there is an error processing the XML before
// the first token has been written, it returns an error
// with errProbablyNotXML as the cause.
func processIcon(w io.Writer, r io.Reader) error {
	// Arrange to save all the content that we find up
	// until the first <svg> element. Then we'll stitch it
	// back together again for the actual processing.
	var saved bytes.Buffer
	dec := xml.NewDecoder(io.TeeReader(r, &saved))
	dec.DefaultSpace = svgNamespace
	found, changed := false, false
	for !found {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errgo.WithCausef(err, errProbablyNotXML, "")
		}
		_, found, changed = ensureViewbox(tok)
	}
	if !found {
		return errgo.WithCausef(nil, errProbablyNotXML, "no <svg> element found")
	}
	// Stitch the input back together again so we can
	// write the output without buffering it in memory.
	r = io.MultiReader(&saved, r)
	if !found || !changed {
		_, err := io.Copy(w, r)
		return err
	}
	return processNaive(w, r)
}

// processNaive is like processIcon but processes all of the
// XML elements. It does not return errProbablyNotXML
// on error because it may have written arbitrary XML
// to w, at which point writing an alternative response would
// be unwise.
func processNaive(w io.Writer, r io.Reader) error {
	dec := xml.NewDecoder(r)
	dec.DefaultSpace = svgNamespace
	enc := xml.NewEncoder(w)
	found := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read token: %v", err)
		}
		if !found {
			tok, found, _ = ensureViewbox(tok)
		}
		if err := enc.EncodeToken(tok); err != nil {
			return fmt.Errorf("cannot encode token %#v: %v", tok, err)
		}
	}
	if err := enc.Flush(); err != nil {
		return fmt.Errorf("cannot flush output: %v", err)
	}
	return nil
}

func ensureViewbox(tok0 xml.Token) (_ xml.Token, found, changed bool) {
	tok, ok := tok0.(xml.StartElement)
	if !ok || tok.Name.Space != svgNamespace || tok.Name.Local != "svg" {
		return tok0, false, false
	}
	var width, height string
	for _, attr := range tok.Attr {
		if attr.Name.Space != "" {
			continue
		}
		switch attr.Name.Local {
		case "width":
			width = attr.Value
		case "height":
			height = attr.Value
		case "viewBox":
			return tok, true, false
		}
	}
	if width == "" || height == "" {
		// Width and/or height have not been specified,
		// so leave viewbox unspecified too.
		return tok, true, false
	}
	tok.Attr = append(tok.Attr, xml.Attr{
		Name: xml.Name{
			Local: "viewBox",
		},
		Value: fmt.Sprintf("0 0 %s %s", width, height),
	})
	return tok, true, true
}
