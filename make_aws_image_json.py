#!/usr/bin/env python3
from argparse import ArgumentParser
from datetime import datetime
import json
import logging
import os
import yaml

from boto import ec2
from boto.exception import EC2ResponseError


def is_china(region):
    return region.endpoint.endswith('.amazonaws.com.cn')


def iter_region_connection(aws, aws_china):
    regions = ec2.regions()
    for region in regions:
        if 'us-gov' in region.name:
            continue
        if is_china(region):
            yield region.connect(**aws_china)
        else:
            yield region.connect(**aws)


def iter_centos_images(access_key_id, secret_access_key):
    for conn in iter_region_connection(access_key_id, secret_access_key):
        try:
            images = conn.get_all_images(filters={
                'owner_alias': 'aws-marketplace',
                'product_code': 'aw0evgkw8e5c1q413zgy5pjce',
                #'name': 'CentOS Linux 7*',
                })
        except EC2ResponseError as e:
            if e.status != 401:
                raise
            logging.warning(
                'Skipping {} due to bad credentials'.format(
                    conn.region.endpoint))
            continue
        for image in images:
            yield image


def mangle(creds):
    for creds in creds.values():
        return {
            'aws_access_key_id': creds['access-key'],
            'aws_secret_access_key': creds['secret-key'],
            }
    else:
        raise LookupError('No Credentials found!')




def make_json(image, now):
    version_name = datetime.utcnow().strftime('%Y%m%d')
    content_id = 'com.ubuntu.cloud.released:aws'
    if is_china(image.region):
        content_id = 'com.ubuntu.cloud.released:aws-cn'
    else:
        content_id = 'com.ubuntu.cloud.released:aws'
    return {
            'format': 'products:1.0',
            'content_id': content_id,
            'datatype': 'image-ids',
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
            'size': '0',
        }


def main():
    parser = ArgumentParser()
    parser.add_argument('output')
    args = parser.parse_args()
    creds_filename = os.path.join(os.environ['JUJU_DATA'], 'credentials.yaml')
    with open(creds_filename) as creds_file:
        credentials = yaml.safe_load(creds_file)['credentials']
    out_json = []
    aws = mangle(credentials['aws'])
    aws_china = mangle(credentials['aws-china'])
    now = datetime.utcnow()
    out_json = [make_json(i, now) for i in iter_centos_images(aws, aws_china)]
    with open(args.output, 'w') as f:
        json.dump(out_json, f, indent=2, sort_keys=True)


if __name__ == '__main__':
    main()
