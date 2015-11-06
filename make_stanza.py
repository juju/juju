#!/usr/bin/env python
from argparse import ArgumentParser
from datetime import date
import hashlib
import os
import sys

from generate_simplestreams import json_dump


def parse_args():
    parser = ArgumentParser()
    parsers = parser.add_subparsers(dest='command')
    ubuntu = parsers.add_parser('ubuntu')
    ubuntu.add_argument('release')
    ubuntu.add_argument('series')
    ubuntu.add_argument('arch')
    ubuntu.add_argument('version')
    ubuntu.add_argument('revision_build')
    ubuntu.add_argument('tarfile')
    return parser.parse_args()


def write_stanzas(release, series, arch, version, revision_build, tarfile):
    path = 'agent/revision-build-{}/{}'.format(revision_build,
                                               os.path.basename(tarfile))
    hash_obj = hashlib.sha256()
    with open(tarfile) as tarfile_fp:
        content = tarfile_fp.read()
    stanza = {
        'content_id': 'com.ubuntu.juju:revision-build-{}:tools'.format(
            revision_build),
        'version_name': date.today().strftime('%Y%m%d'),
        'item_name': '{}-{}-{}'.format(version, series, arch),
        'product_name': 'com.ubuntu.juju:{}:{}'.format(release, arch),
        'path': path,
        'arch': arch,
        'version': version,
        'format': 'products:1.0',
        'release': series,
        'ftype': 'tar.gz',
        'size': len(content),
        }
    for hash_algorithm in ['sha256', 'md5', 'sha1']:
        hash_obj = hashlib.new(hash_algorithm)
        hash_obj.update(content)
        stanza[hash_algorithm] = hash_obj.hexdigest()
    filename = 'revision-build-{}-{}-{}.json'.format(revision_build, series,
                                                     arch)
    json_dump([stanza], filename)


def main():
    args = parse_args()
    kwargs = dict(args.__dict__)
    del kwargs['command']
    write_stanzas(**kwargs)


if __name__ == '__main__':
    sys.exit(main())
