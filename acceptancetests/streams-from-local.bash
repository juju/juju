#!/bin/bash
# ./streams-from-local.bash ./streams /usr/bin/jujud
set -eu

# GO (and Juju) compile by OS and arch. All Linux releases are the same!
RELEASES="precise trusty xenial centos7"
WIN_RELEASES="win2012hvr2 win2012r2"

STREAM_DIR=$1
JUJUD=$2
WIN_JUJUD=${3:-}


if [[ ! -d $STREAM_DIR ]]; then
    echo "Provide a path to a dir to hold the streams."
    exit 1
fi

if [[ ! -f $JUJUD ]]; then
    echo "Provide a path to a dir to hold the streams."
    exit 1
fi

full_version=$($JUJUD version)
version=$(echo $full_version | sed -r 's,(.*)-[^-]+-[^-]+,\1,')
arch=$(echo $full_version | sed -r 's,.*-([^-]+),\1,')

# Juju wont permit devel versions to be in released streams.
if [[ $version =~ (alpha|beta) ]]; then
    agent_dir="devel"
else
    agent_dir="released"
fi

mkdir -p $STREAM_DIR/tools/$agent_dir
mkdir -p $STREAM_DIR/tools/streams/v1

change_dir=$(dirname $JUJUD)
base_agent="agent.tgz"
tar cvfz $base_agent -C $change_dir jujud
for series in $RELEASES; do
    agent="juju-$version-$series-$arch.tgz"
    cp $base_agent $STREAM_DIR/tools/$agent_dir/$agent
done
rm $base_agent

if [[ -n $WIN_JUJUD ]]; then
    change_dir=$(dirname $WIN_JUJUD)
    base_agent="agent.tgz"
    tar cvfz $base_agent -C $change_dir jujud.exe
    for series in $WIN_RELEASES; do
        agent="juju-$version-$series-$arch.tgz"
        cp $base_agent $STREAM_DIR/tools/$agent_dir/$agent
    done
    rm $base_agent
fi

juju metadata generate-tools -d $STREAM_DIR --stream $agent_dir

echo "You can boostrap using these local streams like so:"
echo "juju bootstrap --metadata-source $STREAM_DIR"
echo ""

echo "or"
echo "Publish the $STREAM_DIR tree to a website or maybe your localhost."
echo "cd $STREAM_DIR"
echo "python -m SimpleHTTPServer"
echo "set agent-metadata-url: <HOST/path/tools>"
