#!/bin/bash

JUJU_DIST='juju-dist'


usage() {
    echo "usage: $0 PATH FILES"
    echo "  sync new and changed files to the juju-dist container."
    echo ""
    echo "  PATH: the path in the juju-dist container to upload to"
    echo "  FILES: One or more files to upload."
    exit 1
}


test $# -gt 1 || usage
DEST=$(echo "$1" | sed -r 's,/$,,')
shift
FILES=$@


for file in $FILES; do
    md5=$(md5sum $file | cut -d ' ' -f1)
    etag=$(swift stat $JUJU_DIST $DEST/$file 2>&1 |
        grep ETag | sed -r 's,.*: ,,')
    if [[ $md5 == $etag ]]; then
        echo "$file is unchanged"
        continue
    fi
    if [[ -z $etag ]]; then
        echo "$file is new"
    else
        echo "$file is different"
        echo "$md5 == $etag"
    fi
    echo "Uploading $JUJU_DIST/$DEST/$file"
    swift upload $JUJU_DIST/$DEST/ $file
done
