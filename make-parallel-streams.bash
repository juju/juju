#!/bin/bash
set -eux
rb_stanzas=$1
revision_build=$2
stanza_dir=$3
testing=$4
set_stream.py $stanza_dir/release.json $stanza_dir/release-$revision_build.json $revision_build
cp $rb_stanzas $stanza_dir
json2streams --juju-format $stanza_dir/* $testing
