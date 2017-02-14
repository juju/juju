#!/bin/bash
set -eu

REVISION_BUILD=$1
CONTAINER_BASE="juju-qa-data/juju-ci/products/version-$REVISION_BUILD"

ALL_FILES=$(s3cmd -c $JUJU_HOME/juju-qa.s3cfg ls -r s3://$CONTAINER_BASE/*)

JOBS="build-revision build-osx-client
      build-win-agent build-win-client build-centos
      build-ubuntu-amd64-agent build-ubuntu-arm64-agent
      build-ubuntu-ppc64el-agent build-ubuntu-s390x-agent"

for job in $JOBS; do
    artefacts=$(echo "$ALL_FILES" | sed -r "/$job\/build-/!d; s,^.*s3:,s3:,")
    s3cmd -c $JUJU_HOME/juju-qa.s3cfg get --skip-existing $artefacts ./
    rm consoleText
done

