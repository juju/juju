package charm

import (
	"runtime"
	"testing"
)

// This function and its tests are being submitted upstream through:
//
//     http://codereview.appspot.com/4981049
//
// Meanwhile, we'll inline them here.

type RelTests struct {
	root, path, want string
}

var reltests = []RelTests{
	{"a/b", "a/b", "."},
	{"a/b/.", "a/b", "."},
	{"a/b", "a/b/.", "."},
	{"./a/b", "a/b", "."},
	{"a/b", "./a/b", "."},
	{"a/b", "a/bc", "../bc"},
	{"a/bc", "a/b", "../b"},
	{"a/b", "a/b/c/d", "c/d"},
	{"a/b", "a/b/../c", "../c"},
	{"a/b/../c", "a/b", "../b"},
	{"a/b/c", "a/c/d", "../../c/d"},
	{"a/b", "c/d", "../../c/d"},
	{"../../a/b", "../../a/b/c/d", "c/d"},
	{"/a/b", "/a/b", "."},
	{"/a/b/.", "/a/b", "."},
	{"/a/b", "/a/b/.", "."},
	{"/a/b", "/a/bc", "../bc"},
	{"/a/bc", "/a/b", "../b"},
	{"/a/b", "/a/b/c/d", "c/d"},
	{"/a/b", "/a/b/../c", "../c"},
	{"/a/b/../c", "/a/b", "../b"},
	{"/a/b/c", "/a/c/d", "../../c/d"},
	{"/a/b", "/c/d", "../../c/d"},
	{"/../../a/b", "/../../a/b/c/d", "c/d"},
	{".", "a/b", "a/b"},
	{".", "..", ".."},

	// can't do purely lexically
	{"..", ".", "err"},
	{"..", "a", "err"},
	{"../..", "..", "err"},
	{"a", "/a", "err"},
	{"/a", "a", "err"},
}

var winreltests = []RelTests{
	{`C:a\b\c`, `C:a/b/d`, `..\d`},
	{`C:\`, `D:\`, `err`},
	{`C:`, `D:`, `err`},
}

func TestRel(t *testing.T) {
	tests := append([]RelTests{}, reltests...)
	if runtime.GOOS == "windows" {
		tests = append(tests, winreltests...)
	}
	for _, test := range tests {
		got, err := filepath_Rel(test.root, test.path)
		if test.want == "err" {
			if err == nil {
				t.Errorf("Rel(%q, %q)=%q, want error", test.root, test.path, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Rel(%q, %q): want %q, got error: %s", test.root, test.path, test.want, err)
		}
		if got != test.want {
			t.Errorf("Rel(%q, %q)=%q, want %q", test.root, test.path, got, test.want)
		}
	}
}

