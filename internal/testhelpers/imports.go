// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testhelpers

import (
	"fmt"
	"go/build"
	"path/filepath"
	"sort"
	"strings"
)

// stdlib repesents the packages that belong to the standard library.
var stdlib = map[string]bool{
	"C":                   true, // not really a package
	"archive/tar":         true,
	"archive/zip":         true,
	"bufio":               true,
	"bytes":               true,
	"compress/bzip2":      true,
	"compress/flate":      true,
	"compress/gzip":       true,
	"compress/lzw":        true,
	"compress/zlib":       true,
	"container/heap":      true,
	"container/list":      true,
	"container/ring":      true,
	"crypto":              true,
	"crypto/aes":          true,
	"crypto/cipher":       true,
	"crypto/des":          true,
	"crypto/dsa":          true,
	"crypto/ecdsa":        true,
	"crypto/elliptic":     true,
	"crypto/hmac":         true,
	"crypto/md5":          true,
	"crypto/rand":         true,
	"crypto/rc4":          true,
	"crypto/rsa":          true,
	"crypto/sha1":         true,
	"crypto/sha256":       true,
	"crypto/sha512":       true,
	"crypto/subtle":       true,
	"crypto/tls":          true,
	"crypto/x509":         true,
	"crypto/x509/pkix":    true,
	"database/sql":        true,
	"database/sql/driver": true,
	"debug/dwarf":         true,
	"debug/elf":           true,
	"debug/goobj":         true,
	"debug/gosym":         true,
	"debug/macho":         true,
	"debug/pe":            true,
	"debug/plan9obj":      true,
	"encoding":            true,
	"encoding/ascii85":    true,
	"encoding/asn1":       true,
	"encoding/base32":     true,
	"encoding/base64":     true,
	"encoding/binary":     true,
	"encoding/csv":        true,
	"encoding/gob":        true,
	"encoding/hex":        true,
	"encoding/json":       true,
	"encoding/pem":        true,
	"encoding/xml":        true,
	"errors":              true,
	"expvar":              true,
	"flag":                true,
	"fmt":                 true,
	"go/ast":              true,
	"go/build":            true,
	"go/doc":              true,
	"go/format":           true,
	"go/parser":           true,
	"go/printer":          true,
	"go/scanner":          true,
	"go/token":            true,
	"hash":                true,
	"hash/adler32":        true,
	"hash/crc32":          true,
	"hash/crc64":          true,
	"hash/fnv":            true,
	"html":                true,
	"html/template":       true,
	"image":               true,
	"image/color":         true,
	"image/color/palette": true,
	"image/draw":          true,
	"image/gif":           true,
	"image/jpeg":          true,
	"image/png":           true,
	"index/suffixarray":   true,
	"io":                  true,
	"io/ioutil":           true,
	"log":                 true,
	"log/syslog":          true,
	"math":                true,
	"math/big":            true,
	"math/cmplx":          true,
	"math/rand":           true,
	"mime":                true,
	"mime/multipart":      true,
	"net":                 true,
	"net/http":            true,
	"net/http/cgi":        true,
	"net/http/cookiejar":  true,
	"net/http/fcgi":       true,
	"net/http/httptest":   true,
	"net/http/httputil":   true,
	"net/http/pprof":      true,
	"net/mail":            true,
	"net/rpc":             true,
	"net/rpc/jsonrpc":     true,
	"net/smtp":            true,
	"net/textproto":       true,
	"net/url":             true,
	"os":                  true,
	"os/exec":             true,
	"os/signal":           true,
	"os/user":             true,
	"path":                true,
	"path/filepath":       true,
	"reflect":             true,
	"regexp":              true,
	"regexp/syntax":       true,
	"runtime":             true,
	"runtime/cgo":         true,
	"runtime/debug":       true,
	"runtime/pprof":       true,
	"runtime/race":        true,
	"sort":                true,
	"strconv":             true,
	"strings":             true,
	"sync":                true,
	"sync/atomic":         true,
	"syscall":             true,
	"testing":             true,
	"testing/iotest":      true,
	"testing/quick":       true,
	"text/scanner":        true,
	"text/tabwriter":      true,
	"text/template":       true,
	"text/template/parse": true,
	"time":                true,
	"unicode":             true,
	"unicode/utf16":       true,
	"unicode/utf8":        true,
	"unsafe":              true,
}

// FindImports returns a sorted list of packages imported by the package
// with the given name that have the given prefix. The resulting list
// removes the common prefix, leaving just the short names.
func FindImports(packageName, prefix string) ([]string, error) {
	allPkgs := make(map[string]bool)
	if err := findImports(packageName, allPkgs, findProjectDir(prefix)); err != nil {
		return nil, err
	}
	var result []string
	for name := range allPkgs {
		if strings.HasPrefix(name, prefix) {
			result = append(result, name[len(prefix):])
		}
	}
	sort.Strings(result)
	return result, nil
}

func findProjectDir(projectPath string) string {
	return filepath.Join(build.Default.GOPATH, "src", projectPath)
}

// findImports recursively adds all imported packages of given
// package (packageName) to allPkgs map.
func findImports(packageName string, allPkgs map[string]bool, srcDir string) error {
	// skip packages defined in the standard library.
	if stdlib[packageName] {
		return nil
	}
	pkg, err := build.Default.Import(packageName, srcDir, 0)
	if err != nil {
		return fmt.Errorf("cannot find %q: %v", packageName, err)
	}
	for _, name := range pkg.Imports {
		if !allPkgs[name] {
			allPkgs[name] = true
			if err := findImports(name, allPkgs, srcDir); err != nil {
				return err
			}
		}
	}
	return nil
}
