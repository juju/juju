#!/bin/bash
set -eux
testing=$1
sstream-query $testing/streams/v1/index2.json content_id="com.ubuntu.juju:revision-build-$revision_build:tools" version=$VERSION --output-format="%(sha256)s  %(item_url)s" > sha256sums
sha256sum -c sha256sums
