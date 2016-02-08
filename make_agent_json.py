#!/usr/bin/env python
from argparse import ArgumentParser
from copy import deepcopy
from datetime import datetime
import hashlib
import os
import sys

from simplestreams.generate_simplestreams import json_dump

from build_package import juju_series


supported_windows_releases = (
    'win2012', 'win2012hv', 'win2012hvr2', 'win2012r2', 'win7', 'win81',
    'win8',
    )


def parse_args():
    parser = ArgumentParser()
    parsers = parser.add_subparsers(dest='command')
    ubuntu = parsers.add_parser('ubuntu')
    living_ubuntu = parsers.add_parser('living-ubuntu')
    windows = parsers.add_parser('windows')
    centos = parsers.add_parser('centos')
    for subparser in [ubuntu, living_ubuntu, windows, centos]:
        subparser.add_argument('tarfile')
        subparser.add_argument('revision_build')
        subparser.add_argument('version')
    for subparser in [ubuntu, living_ubuntu]:
        subparser.add_argument('arch')
    ubuntu.add_argument('release')
    ubuntu.add_argument('series')
    return parser.parse_args()


class StanzaWriter:

    def __init__(self, releases, arch, version, revision_build,
                 tarfile, filename):
        self.releases = releases
        self.arch = arch
        self.version = version
        self.revision_build = revision_build
        self.tarfile = tarfile
        self.version_name = datetime.utcnow().strftime('%Y%m%d')
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
    def for_living_ubuntu(cls, arch, version, revision_build, tarfile):
        filename = 'revision-build-{}-ubuntu-{}.json'.format(revision_build,
                                                             arch)
        releases = [
            (juju_series.get_version(name), name) for name
            in juju_series.get_living_names()]
        return cls(
            releases, arch, version, revision_build, tarfile, filename)

    @classmethod
    def for_windows(cls, version, revision_build, tarfile):
        filename = 'revision-build-{}-windows.json'.format(
            revision_build)
        releases = [(r, r) for r in supported_windows_releases]
        return cls(releases, 'amd64', version, revision_build, tarfile,
                   filename)

    @classmethod
    def for_centos(cls, version, revision_build, tarfile):
        filename = 'revision-build-{}-centos.json'.format(revision_build)
        return cls([('centos7', 'centos7')], 'amd64', version, revision_build,
                   tarfile, filename)

    def write_stanzas(self):
        path = 'agent/revision-build-{}/{}'.format(
            self.revision_build, os.path.basename(self.tarfile))
        with open(self.tarfile) as tarfile_fp:
            content = tarfile_fp.read()
        hashes = {}
        for hash_algorithm in ['sha256', 'md5']:
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
    if args.command == 'living-ubuntu':
        writer = StanzaWriter.for_living_ubuntu(**kwargs)
    elif args.command == 'windows':
        writer = StanzaWriter.for_windows(**kwargs)
    elif args.command == 'centos':
        writer = StanzaWriter.for_centos(**kwargs)
    writer.write_stanzas()

if __name__ == '__main__':
    sys.exit(main())
