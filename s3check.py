#!/usr/bin/env python
from __future__ import print_function

from argparse import ArgumentParser
from ConfigParser import ConfigParser
import logging
import os
import re
import sys

from boto.s3.connection import S3Connection


JUJU_QA_DATA = 'juju-qa-data'
JUJU_PIP_ARCHIVES = 'juju-pip-archives'
CLOUD_CITY = os.path.join(os.environ['HOME'], 'cloud-city')
jujuqa_config = os.path.join(CLOUD_CITY, 'juju-qa.s3cfg')


log = logging.getLogger("s3-check")
handler = logging.StreamHandler(sys.stderr)
handler.setFormatter(logging.Formatter(
    fmt='%(asctime)s %(levelname)s %(message)s',
    datefmt='%Y-%m-%d %H:%M:%S'))
log.addHandler(handler)
log.setLevel(logging.INFO)


def parse_args(args=None):
    parser = ArgumentParser()
    parser.add_argument(
        '--configs', nargs='*', default=[jujuqa_config],
        help='The job to get files from')
    parser.add_argument(
        'buckets', nargs='*', default=[JUJU_QA_DATA, JUJU_PIP_ARCHIVES],
        help='The job to get files from')

    return parser.parse_args(args)


def get_s3_credentials(s3cfg_path):
    config = ConfigParser()
    with open(s3cfg_path) as fp:
        config.readfp(fp)
    access_key = config.get('default', 'access_key')
    secret_key = config.get('default', 'secret_key')
    return access_key, secret_key


def get_qa_data_bucket(config, bucket):
    credentials = get_s3_credentials(config)
    conn = S3Connection(*credentials)
    return conn.get_bucket(bucket)


def list_file_keys(bucket, prefix, file_regex):
    pattern = re.compile(file_regex)
    keys = bucket.list(prefix)
    filtered = []
    for number, key in enumerate(keys):
        if pattern.match(key.name):
            filtered.append(key.name)
        if number >= 5:
            break
    return filtered


def main():
    args = parse_args()
    for config in args.configs:
        for bucket in args.buckets:
            log.info('trying {} with {}'.format(config, bucket))
            bucket = get_qa_data_bucket(config, bucket)
            log.info(bucket.generate_url(999999999))
            keys = list_file_keys(bucket, '', '.*')
            for key in keys:
                log.info('    Found {}'.format(key))


if __name__ == '__main__':
    sys.exit(main())
