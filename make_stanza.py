#!/usr/bin/env python
from argparse import ArgumentParser
from copy import deepcopy
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
    windows = parsers.add_parser('windows')
    for subparser in [ubuntu, windows]:
        subparser.add_argument('version')
        subparser.add_argument('revision_build')
        subparser.add_argument('tarfile')
    return parser.parse_args()


class StanzaWriter:

    def __init__(self, releases, arch, version, revision_build,
                 tarfile, filename):
        self.releases = releases
        self.arch = arch
        self.version = version
        self.revision_build = revision_build
        self.tarfile = tarfile
        self.version_name = date.today().strftime('%Y%m%d')
        self.filename = filename

    @classmethod
    def for_ubuntu(cls, release, series, arch, version, revision_build,
                   tarfile):
        filename = 'revision-build-{}-{}-{}.json'.format(
            revision_build, series, arch)
        return cls(
            [(release, series)], arch, version, revision_build, tarfile,
            filename)

    @classmethod
    def for_windows(cls, version, revision_build, tarfile):
        filename = 'revision-build-{}-windows.json'.format(
            revision_build)
        releases = [(r, r) for r in [
            'win2012', 'win2012hv', 'win2012hvr2', 'win2012r2', 'win7',
            'win81', 'win8',
            ]]
        return cls(releases, 'amd64', version, revision_build, tarfile,
                   filename)

    def write_stanzas(self):
        path = 'agent/revision-build-{}/{}'.format(
            self.revision_build, os.path.basename(self.tarfile))
        with open(self.tarfile) as tarfile_fp:
            content = tarfile_fp.read()
        hashes = {}
        for hash_algorithm in ['sha256', 'md5', 'sha1']:
            hash_obj = hashlib.new(hash_algorithm)
            hash_obj.update(content)
            hashes[hash_algorithm] = hash_obj.hexdigest()
        stanzas = list(self.make_stanzas(path, hashes, len(content)))
        json_dump(stanzas, self.filename)

    def make_stanzas(self, path, hashes, size):
        for release, series in self.releases:
            stanza = {
                'content_id': 'com.ubuntu.juju:revision-build-{}:tools'.format(
                    self.revision_build),
                'version_name': self.version_name,
                'item_name': '{}-{}-{}'.format(self.version, series,
                                               self.arch),
                'product_name': 'com.ubuntu.juju:{}:{}'.format(release,
                                                               self.arch),
                'path': path,
                'arch': self.arch,
                'version': self.version,
                'format': 'products:1.0',
                'release': series,
                'ftype': 'tar.gz',
                'size': size,
                }
            stanza.update(deepcopy(hashes))
            yield stanza


def main():
    args = parse_args()
    kwargs = dict(args.__dict__)
    del kwargs['command']
    if args.command == 'ubuntu':
        writer = StanzaWriter.for_ubuntu(**kwargs)
    if args.command == 'windows':
        writer = StanzaWriter.for_windows(**kwargs)
    writer.write_stanzas()

if __name__ == '__main__':
    sys.exit(main())
