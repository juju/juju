#!/usr/bin/env python3
from __future__ import print_function

from argparse import ArgumentParser
from datetime import datetime
import os
import sys
from textwrap import dedent
import yaml

from boto import ec2
from simplestreams.generate_simplestreams import (
    items2content_trees,
    )
from simplestreams.json2streams import (
    Item,
    write_juju_streams,
    )
from simplestreams.util import timestamp


def get_parameters(argv=None):
    """Return streams, creds_filename for this invocation.

    streams is the directory to write streams into.
    creds_filename is the filename to get credentials from.
    """
    parser = ArgumentParser(description=dedent("""
        Write image streams for AWS images.  Only CentOS 7 is currently
        supported."""))
    parser.add_argument('streams', help='The directory to write streams to.')
    args = parser.parse_args(argv)
    try:
        juju_data = os.environ['JUJU_DATA']
    except KeyError:
        print('JUJU_DATA must be set to a directory containing'
              ' credentials.yaml.', file=sys.stderr)
        sys.exit(1)
    creds_filename = os.path.join(juju_data, 'credentials.yaml')
    return args.streams, creds_filename


def make_aws_credentials(creds):
    """Convert credentials from juju format to AWS/Boto format."""
    for creds in creds.values():
        return {
            'aws_access_key_id': creds['access-key'],
            'aws_secret_access_key': creds['secret-key'],
            }
    else:
        raise LookupError('No credentials found!')


def is_china(region):
    """Determine whether the supplied region is in AWS-China."""
    return region.endpoint.endswith('.amazonaws.com.cn')


def iter_region_connection(credentials, china_credentials):
    """Iterate through connections for all regions except gov.

    AWS-China regions will be connected using china_credentials.
    US-GOV regions will be skipped.
    All other regions will be connected using credentials.
    """
    regions = ec2.regions()
    for region in regions:
        if 'us-gov' in region.name:
            continue
        if is_china(region):
            yield region.connect(**china_credentials)
        else:
            yield region.connect(**credentials)


def iter_centos_images(credentials, china_credentials):
    """Iterate through CentOS 7 images in standard AWS and AWS China."""
    for conn in iter_region_connection(credentials, china_credentials):
        images = conn.get_all_images(filters={
            'owner_alias': 'aws-marketplace',
            'product_code': 'aw0evgkw8e5c1q413zgy5pjce',
            # 'name': 'CentOS Linux 7*',
            })
        for image in images:
            yield image


def make_item_name(region, vtype, store):
    """Determine the item_name, given an image's attributes.

    :param region: The region name
    :param vtype: The virtualization type
    :param store: The root device type.
    """
    # This is a port of code from simplestreams/tools/make-test-data, which is
    # not provided as library code.
    dmap = {
        "north": "nn",
        "northeast": "ne",
        "east": "ee",
        "southeast": "se",
        "south": "ss",
        "southwest": "sw",
        "west": "ww",
        "northwest": "nw",
        "central": "cc",
    }
    itmap = {
        'pv': {'instance': "pi", "ebs": "pe", "ssd": "es", "io1": "eo"},
        'hvm': {'instance': "hi", "ebs": "he", "ssd": "hs", "io1": "ho"}
    }
    if store == "instance-store":
        store = 'instance'
    elif '-' in store:
        store = store.split('-')[-1]
    if vtype == "paravirtual":
        vtype = "pv"

    # create the item key:
    #  - 2 letter country code (us) . 3 for govcloud (gww)
    #  - 2 letter direction (nn=north, nw=northwest, cc=central)
    #  - 1 digit number
    #  - 1 char for virt type
    #  - 1 char for root-store type

    # Handle special case of 'gov' regions
    pre_cc = ""
    _region = region
    if '-gov-' in region:
        _region = region.replace('gov-', '')
        pre_cc = "g"

    (cc, direction, num) = _region.split("-")

    ikey = pre_cc + cc + dmap[direction] + num + itmap[vtype][store]
    return ikey


def make_item(image, now):
    """Convert Centos 7 Boto image to simplestreams Item.

    :param now: the current datetime.
    """
    if image.architecture != 'x86_64':
        raise ValueError(
            'Architecture is "{}", not "x86_64".'.format(image.architecture))
    if not image.name.startswith('CentOS Linux 7 '):
        raise ValueError(
            'Name "{}" does not begin with "CentOS Linux 7".'.format(
                image.name))
    item_name = make_item_name(image.region.name, image.virtualization_type,
                               image.root_device_type)
    version_name = now.strftime('%Y%m%d')
    content_id = 'com.ubuntu.cloud.released:aws'
    if is_china(image.region):
        content_id = 'com.ubuntu.cloud.released:aws-cn'
    else:
        content_id = 'com.ubuntu.cloud.released:aws'
    return Item(
        content_id, 'com.ubuntu.cloud:server:centos7:amd64', version_name,
        item_name, {
            'endpoint': 'https://{}'.format(image.region.endpoint),
            'region': image.region.name,
            'arch': 'amd64',
            'os': 'centos',
            'virt': image.virtualization_type,
            'id': image.id,
            'version': 'centos7',
            'label': 'release',
            'release': 'centos7',
            'release_codename': 'centos7',
            'release_title': 'Centos 7',
            'root_store': image.root_device_type,
        })


def write_streams(credentials, china_credentials, now, streams):
    """Write image streams for Centos 7.

    :param credentials: The standard AWS credentials.
    :param china_credentials: The AWS China crentials.
    :param now: The current datetime.
    :param streams: The directory to store streams metadata in.
    """
    items = [make_item(i, now) for i in iter_centos_images(
        credentials, china_credentials)]
    updated = timestamp()
    data = {'updated': updated, 'datatype': 'image-ids'}
    trees = items2content_trees(items, data)
    write_juju_streams(streams, trees, updated)


def main():
    streams, creds_filename = get_parameters()
    with open(creds_filename) as creds_file:
        all_credentials = yaml.safe_load(creds_file)['credentials']
    credentials = make_aws_credentials(all_credentials['aws'])
    china_credentials = make_aws_credentials(all_credentials['aws-china'])
    now = datetime.utcnow()
    write_streams(credentials, china_credentials, now, streams)


if __name__ == '__main__':
    main()
