package charm

import (
	"os"
	"path/filepath"
)

// This function and its tests are being submitted upstream through:
//
//     http://codereview.appspot.com/4981049
//
// Meanwhile, we'll inline them here.

// Rel returns a relative path that is equivalent to targpath when
// joined to basepath with an intervening separator. That is,
// Join(basepath, Rel(basepath, targpath)) is equivalent to targpath itself.
// An error is returned if targpath can't be made relative to basepath or if
// knowing the current working directory would be necessary to compute it.
func filepath_Rel(basepath, targpath string) (string, os.Error) {
	baseVol := filepath.VolumeName(basepath)
	targVol := filepath.VolumeName(targpath)
	base := filepath.Clean(basepath)
	targ := filepath.Clean(targpath)
	if targ == base {
		return ".", nil
	}
	base = base[len(baseVol):]
	targ = targ[len(targVol):]
	if base == "." {
		base = ""
	}
	// Can't use IsAbs - `\a` and `a` are both relative in Windows.
	baseSlashed := len(base) > 0 && base[0] == filepath.Separator
	targSlashed := len(targ) > 0 && targ[0] == filepath.Separator
	if baseSlashed != targSlashed || baseVol != targVol {
		return "", os.NewError("Rel: can't make " + targ + " relative to " + base)
	}
	// Position base[b0:bi] and targ[t0:ti] at the first differing elements.
	bl := len(base)
	tl := len(targ)
	var b0, bi, t0, ti int
	for {
		for bi < bl && base[bi] != filepath.Separator {
			bi++
		}
		for ti < tl && targ[ti] != filepath.Separator {
			ti++
		}
		if targ[t0:ti] != base[b0:bi] {
			break
		}
		if bi < bl {
			bi++
		}
		if ti < tl {
			ti++
		}
		b0 = bi
		t0 = ti
	}
	if base[b0:bi] == ".." {
		return "", os.NewError("Rel: can't make " + targ + " relative to " + base)
	}
	if b0 != bl {
		// Base elements left. Must go up before going down.
		buf := []byte("..")
		for i := b0; i < bl; i++ {
			if base[i] == filepath.Separator {
				buf = append(buf, '/', '.', '.')
			}
		}
		if t0 != tl {
			buf = append(buf, '/')
			buf = append(buf, []byte(targ[t0:])...)
		}
		return string(buf), nil
	}
	return targ[t0:], nil
}


