// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshinit

const (
	// aptSourcesList is the location of the APT sources list
	// configuration file.
	aptSourcesList = "/etc/apt/sources.list"

	// aptListsDirectory is the location of the APT lists directory.
	aptListsDirectory = "/var/lib/apt/lists"

	// extractAptSource is a shell command that will extract the
	// currently configured APT source location. We assume that
	// the first source for "main" in the file is the one that
	// should be replaced throughout the file.
	extractAptSource = `awk "/^deb .* $(lsb_release -sc) .*main.*\$/{print \$2;exit}" ` + aptSourcesList

	// aptSourceListPrefix is a shell program that translates an
	// APT source (piped from stdin) to a file prefix. The algorithm
	// involves stripping up to one trailing slash, stripping the
	// URL scheme prefix, and finally translating slashes to
	// underscores.
	aptSourceListPrefix = `sed 's,.*://,,' | sed 's,/$,,' | tr / _`
)

// renameAptListFilesCommands takes a new and old mirror string,
// and returns a sequence of commands that will rename the files
// in aptListsDirectory.
func renameAptListFilesCommands(newMirror, oldMirror string) []string {
	oldPrefix := "old_prefix=" + aptListsDirectory + "/$(echo " + oldMirror + " | " + aptSourceListPrefix + ")"
	newPrefix := "new_prefix=" + aptListsDirectory + "/$(echo " + newMirror + " | " + aptSourceListPrefix + ")"
	renameFiles := `
for old in ${old_prefix}_*; do
    new=$(echo $old | sed s,^$old_prefix,$new_prefix,)
    mv $old $new
done`

	return []string{
		oldPrefix,
		newPrefix,
		// Don't do anything unless the mirror/source has changed.
		`[ "$old_prefix" != "$new_prefix" ] &&` + renameFiles,
	}
}
