package formula

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// The Formula interface is implemented by any type that
// may be handled as a formula.
type Formula interface {
	Path() string
	Meta() *Meta
	Config() *Config
	IsExpanded() bool
	//ExpandTo(path string) Formula
	//BundleTo(path string) Formula
}

// ParseId splits a formula identifier into its constituting parts.
func ParseId(id string) (namespace string, name string, rev int, err os.Error) {
	colon := strings.Index(id, ":")
	if colon == -1 {
		err = errorf("Missing formula namespace: %q", id)
		return
	}
	dash := strings.LastIndex(id, "-")
	if dash != -1 {
		rev, err = strconv.Atoi(id[dash+1:])
	}
	if dash == -1 || err != nil {
		err = errorf("Missing formula revision: %q", id)
		return
	}
	namespace = id[:colon]
	name = id[colon+1 : dash]
	return
}

func errorf(format string, args ...interface{}) os.Error {
	return os.NewError(fmt.Sprintf(format, args...))
}
