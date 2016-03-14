#!/usr/bin/env python3
from __future__ import print_function

from argparse import ArgumentParser
from datetime import datetime
import logging
import os
import sys
from textwrap import dedent
import yaml

from boto import ec2
from boto.exception import EC2ResponseError
from simplestreams.generate_simplestreams import (
    items2content_trees,
    )
from simplestreams.json2streams import (
    Item,
    write_juju_streams,
    )
from simplestreams.util import timestamp


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


def iter_centos_images(access_key_id, secret_access_key):
    for conn in iter_region_connection(access_key_id, secret_access_key):
        images = conn.get_all_images(filters={
            'owner_alias': 'aws-marketplace',
            'product_code': 'aw0evgkw8e5c1q413zgy5pjce',
            # 'name': 'CentOS Linux 7*',
            })
        for image in images:
            yield image


def make_aws_credentials(creds):
    for creds in creds.values():
        return {
            'aws_access_key_id': creds['access-key'],
            'aws_secret_access_key': creds['secret-key'],
            }
    else:
        raise LookupError('No credentials found!')


def make_json(image, now):
    version_name = datetime.utcnow().strftime('%Y%m%d')
    content_id = 'com.ubuntu.cloud.released:aws'
    if is_china(image.region):
        content_id = 'com.ubuntu.cloud.released:aws-cn'
    else:
        content_id = 'com.ubuntu.cloud.released:aws'
    return {
            'content_id': content_id,
            'endpoint': 'https://{}'.format(image.region.endpoint),
            'region': image.region.name,
            'arch': 'amd64',
            'os': 'centos',
            'virt': image.virtualization_type,
            'id': image.id,
            'version': 'centos7',
            'product_name': 'com.ubuntu.cloud:server:centos7:amd64',
            'item_name': image.region.name,
            'label': 'release',
            'release': 'centos7',
            'release_codename': 'centos7',
            'release_title': 'Centos 7',
            'root_store': image.root_device_type,
            'version_name': version_name,
        }


def make_item(image, now):
    version_name = datetime.utcnow().strftime('%Y%m%d')
    content_id = 'com.ubuntu.cloud.released:aws'
    if is_china(image.region):
        content_id = 'com.ubuntu.cloud.released:aws-cn'
    else:
        content_id = 'com.ubuntu.cloud.released:aws'
    return Item(
        content_id, 'com.ubuntu.cloud:server:centos7:amd64', version_name,
        image.region.name, {
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
            'version_name': version_name,
        })


def main():
    parser = ArgumentParser(description=dedent("""
        Write image streams for AWS images.  Only CentOS 7 is currently
        supported."""))
    parser.add_argument('streams', help='The directory to write streams to.')
    args = parser.parse_args()
    try:
        juju_data = os.environ['JUJU_DATA']
    except KeyError:
        print('JUJU_DATA must be set to a directory containing'
              ' credentials.yaml.', file=sys.stderr)
        sys.exit(1)
    creds_filename = os.path.join(juju_data, 'credentials.yaml')
    with open(creds_filename) as creds_file:
        credentials = yaml.safe_load(creds_file)['credentials']
    aws = make_aws_credentials(credentials['aws'])
    aws_china = make_aws_credentials(credentials['aws-china'])
    now = datetime.utcnow()
    items = [make_item(i, now) for i in iter_centos_images(aws, aws_china)]
    updated = timestamp()
    data = {'updated': updated, 'datatype': 'image-ids'}
    trees = items2content_trees(items, data)
    write_juju_streams(args.streams, trees, updated)


if __name__ == '__main__':
    main()
