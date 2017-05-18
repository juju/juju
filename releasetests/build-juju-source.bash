echo "Getting and updating juju-core dependencies to the required versions."
GOPATH=$WORK go get -v github.com/rogpeppe/godeps
GODEPS=$WORK/bin/godeps
if [[ ! -f $GODEPS ]]; then
    echo "! Could not install godeps."
    exit 1
fi
GOPATH=$WORK $GODEPS -u "$WORKPACKAGE/dependencies.tsv"

# Remove godeps, and non-free data
echo "Removing godeps and non-free data."
rm -rf $WORK/src/github.com/rogpeppe/godeps
rm -rf $WORK/src/github.com/kisielk
rm -rf $WORK/src/code.google.com/p/go.net/html/charset/testdata/
rm -f $WORK/src/code.google.com/p/go.net/html/charset/*test.go
rm -rf $WORK/src/golang.org/x/net/html/charset/testdata/
rm -f $WORK/src/golang.org/x/net/html/charset/*test.go
rm -rf $WORK/src/github.com/prometheus/procfs/fixtures
# Remove backup files that confuse lintian.
echo "Removing backup files"
find $WORK/src/ -type f -name *.go.orig -delete

# Validate the go src tree against dependencies.tsv
echo "Validating dependencies.tsv"
$SCRIPT_DIR/check_dependencies.py --delete-unknown --ignore $PACKAGE \
    "$WORKPACKAGE/dependencies.tsv" "$WORK/src"

# Apply patches against the whole source tree from the juju project
echo "Applying Patches"
if [[ -d "$WORKPACKAGE/patches" ]]; then
    $SCRIPT_DIR/apply_patches.py "$WORKPACKAGE/patches" "$WORK/src"
fi

# Run juju's fmt and vet script on the source after finding the right version
echo "Running format and vetting the build"
CHECKSCRIPT=./scripts/verify.bash
if [[ ! -f $WORKPACKAGE/scripts/verify.bash ]]; then
    CHECKSCRIPT=./scripts/pre-push.bash
fi
(cd $WORKPACKAGE && GOPATH=$WORK $CHECKSCRIPT)

# Remove binaries and build artefacts
echo "Removing binaries and build artifacts"
rm -r $WORK/bin
if [[ -d $WORK/pkg ]]; then
    rm -r $WORK/pkg
fi

echo "Rename to proper release version"
VERSION=$(sed -n 's/^const version = "\(.*\)"/\1/p' \
    $WORKPACKAGE/version/version.go)

# Change the generic release to the proper juju-core version.
mv $WORK $TMP_DIR/juju-core_${VERSION}/
