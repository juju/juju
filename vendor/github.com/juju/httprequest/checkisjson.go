package httprequest

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"gopkg.in/errgo.v1"
)

// maxErrorBodySize holds the maximum amount of body that
// we try to read for an error before extracting text from it.
// It's reasonably large because:
// a) HTML often has large embedded scripts which we want
// to skip and
// b) it should be an relatively unusual case so the size
// shouldn't harm.
//
// It's defined as a variable so that it can be redefined in tests.
var maxErrorBodySize = 200 * 1024

// checkIsJSON checks that the content type of the given header implies
// that the content is JSON. If it is not, then reads from the body to
// try to make a useful error message.
func checkIsJSON(header http.Header, body io.Reader) error {
	contentType := header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if mediaType == "application/json" {
		return nil
	}
	if err != nil {
		// Even if there's no media type, we want to see something useful.
		mediaType = fmt.Sprintf("%q", contentType)
	}
	// TODO use charset.NewReader to convert from non-utf8 content?
	// Read the body ignoring any errors - we'll just make do with what we've got.
	bodyData, _ := ioutil.ReadAll(io.LimitReader(noErrorReader{body}, int64(maxErrorBodySize)))
	switch mediaType {
	case "text/html":
		text, err := htmlToText(bytes.NewReader(bodyData))
		if err != nil {
			// Note: it seems that this can never actually
			// happen - the only way that the HTML parser
			// can fail is if there's a read error and we've
			// removed that possibility by using
			// noErrorReader above.
			return errgo.Notef(err, "unexpected (and invalid) content text/html; want application/json; content: %q", sizeLimit(bodyData))
		}
		if len(text) == 0 {
			return errgo.Newf(`unexpected content type text/html; want application/json; content: %q`, sizeLimit(bodyData))
		}
		return errgo.Newf(`unexpected content type text/html; want application/json; content: %s`, sizeLimit(text))
	case "text/plain":
		return errgo.Newf(`unexpected content type text/plain; want application/json; content: %s`, sizeLimit(sanitizeText(string(bodyData), true)))
	default:
		return errgo.Newf(`unexpected content type %s; want application/json; content: %q`, mediaType, sizeLimit(bodyData))
	}
}

// noErrorReader wraps a reader, turning any errors into io.EOF
// so that we can extract some content even if we get an io error.
type noErrorReader struct {
	r io.Reader
}

func (r noErrorReader) Read(buf []byte) (int, error) {
	n, err := r.r.Read(buf)
	if err != nil {
		err = io.EOF
	}
	return n, err
}

func sizeLimit(data []byte) []byte {
	const max = 1024
	if len(data) < max {
		return data
	}
	return append(data[0:max], fmt.Sprintf(" ... [%d bytes omitted]", len(data)-max)...)
}

// htmlToText attempts to return some relevant textual content
// from the HTML content in the given reader, formatted
// as a single line.
func htmlToText(r io.Reader) ([]byte, error) {
	n, err := html.Parse(r)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	htmlNodeToText(&buf, n)
	return buf.Bytes(), nil
}

func htmlNodeToText(w *bytes.Buffer, n *html.Node) {
	for ; n != nil; n = n.NextSibling {
		switch n.Type {
		case html.TextNode:
			data := sanitizeText(n.Data, false)
			if len(data) == 0 {
				break
			}
			if w.Len() > 0 {
				w.WriteString("; ")
			}
			w.Write(data)
		case html.ElementNode:
			if n.DataAtom != atom.Script {
				htmlNodeToText(w, n.FirstChild)
			}
		case html.DocumentNode:
			htmlNodeToText(w, n.FirstChild)
		}
	}
}

// sanitizeText tries to make the given string easier to read when presented
// as a single line. It squashes each run of white space into a single
// space, trims leading and trailing white space and trailing full
// stops. If newlineSemi is true, any newlines will be replaced with a
// semicolon.
func sanitizeText(s string, newlineSemi bool) []byte {
	out := make([]byte, 0, len(s))
	prevWhite := false
	for _, r := range s {
		if newlineSemi && r == '\n' && len(out) > 0 {
			out = append(out, ';')
			prevWhite = true
			continue
		}
		if unicode.IsSpace(r) {
			if len(out) > 0 {
				prevWhite = true
			}
			continue
		}
		if prevWhite {
			out = append(out, ' ')
			prevWhite = false
		}
		out = append(out, string(r)...)
	}
	// Remove final space, any full stops and any final semicolon
	// we might have added.
	out = bytes.TrimRightFunc(out, func(r rune) bool {
		return r == '.' || r == ' ' || r == ';'
	})
	return out
}
