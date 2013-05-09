package testing

import (
	"fmt"
	"strings"
)

// Generate an line for inclusion in an images metadata file.
// See https://help.ubuntu.com/community/UEC/Images
func ImagesFields(srcs ...string) string {
	strs := make([]string, len(srcs))
	for i, src := range srcs {
		parts := strings.Split(src, " ")
		if len(parts) != 5 {
			panic("bad clouddata field input")
		}
		args := make([]interface{}, len(parts))
		for i, part := range parts {
			args[i] = part
		}
		// Ignored fields are left empty for clarity's sake, and two additional
		// tabs are tacked on to the end to verify extra columns are ignored.
		strs[i] = fmt.Sprintf("\t\t\t\t%s\t%s\t%s\t%s\t\t\t%s\t\t\n", args...)
	}
	return strings.Join(strs, "")
}
