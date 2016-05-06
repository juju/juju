#!/bin/bash
set -eu
export JUJU_HOME=~/cloud-city
source $JUJU_HOME/rackspacerc
set -x

BUCKET_NAME=${1:-juju-ci-image-streams}
STREAMS_DIR=`mktemp -d -t juju-ci-image-metadata.XXXXXX`
CDN_ENDPOINT=`keystone endpoint-get --service "rax:object-cdn"|grep publicURL|cut -d "|" -f 3`

IMAGE_ID=`nova image-list|grep "14\.04.*PVHVM"|cut -d "|" -f 2`

# Create simplestreams for trusty image
juju metadata generate-image -d $STREAMS_DIR -i $IMAGE_ID -s trusty \
	-r $OS_REGION_NAME -u $OS_AUTH_URL

# Allow read access to bucket without token
swift post -r ".r:*" $BUCKET_NAME
# Enable CDN replication for bucket via rax:object-cdn service
# <https://developer.rackspace.com/docs/cloud-files/getting-started/>
swift post --os-storage-url $CDN_ENDPOINT --header "X-CDN-Enabled: True" $BUCKET_NAME

# Upload images directory to bucket
(cd $STREAMS_DIR && swift upload $BUCKET_NAME images)

CDN_HEADER=`swift stat --os-storage-url $CDN_ENDPOINT $BUCKET_NAME|grep -i X-Cdn-Uri`
echo ${CDN_HEADER#*: }/images
