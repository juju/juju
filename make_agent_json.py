#!/usr/bin/env python
from argparse import ArgumentParser
from copy import deepcopy
from datetime import datetime
import hashlib
import os
import re
import sys

from simplestreams.generate_simplestreams import json_dump

from build_package import juju_series

__metaclass__ = type


supported_windows_releases = (
    'win2012', 'win2012hv', 'win2012hvr2', 'win2012r2', 'win2016',
    'win2016nano', 'win7', 'win81', 'win8', 'win10',
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
    gui = parsers.add_parser('gui')
    gui.add_argument('tarfile')
    gui.add_argument('stream')
    return parser.parse_args()


class FileStanzaWriter:
    """Base class to write stanzas about files."""

    def __init__(self, filename, stream, version, ftype, tarfile,
                 path):
        self.filename = filename
        self.stream = stream
        self.version = version
        self.tarfile = tarfile
        self.path = path
        self.version_name = datetime.utcnow().strftime('%Y%m%d')
        self.ftype = ftype

    def make_path_stanza(self, product_name, item_name, hashes, size):
        """Make a path stanza.

        :param product_name: The simplestreams product name.
        :param item_name: The simplestream item name.
        :param hahes: A dict mapping hash name to the hash of the file with
            that hash.  hashlib names (e.g. "sha256") should be used.
        """
        stanza = {
            'content_id': self.content_id,
            'product_name': product_name,
            'item_name': item_name,
            'version_name': self.version_name,
            'path': self.path,
            'size': size,
            'version': self.version,
            'format': 'products:1.0',
            'ftype': self.ftype,
            }
        stanza.update(deepcopy(hashes))
        return stanza

    def write_stanzas(self):
        """Write stanzas about the file to the filename.

        This calculates the hashes as part of the procedure.
        """
        with open(self.tarfile) as tarfile_fp:
            content = tarfile_fp.read()
        hashes = {}
        for hash_algorithm in self.hash_algorithms:
            hash_obj = hashlib.new(hash_algorithm)
            hash_obj.update(content)
            hashes[hash_algorithm] = hash_obj.hexdigest()
        stanzas = list(self.make_stanzas(hashes, len(content)))
        json_dump(stanzas, self.filename)


class StanzaWriter(FileStanzaWriter):

    def __init__(self, releases, arch, version, tarfile, filename,
                 revision_build=None, agent_stream=None):
        if agent_stream is None:
            agent_stream = 'revision-build-{}'.format(revision_build)
        if revision_build is None:
            path = 'agent/{}/{}'.format(
                version, os.path.basename(tarfile))
        else:
            path = 'agent/revision-build-{}/{}'.format(
                revision_build, os.path.basename(tarfile))
        super(StanzaWriter, self).__init__(filename, agent_stream, version,
                                           'tar.gz', tarfile, path)
        self.releases = releases
        self.arch = arch
        self.filename = filename

    hash_algorithms = frozenset(['sha256', 'md5'])

    @property
    def content_id(self):
        return 'com.ubuntu.juju:{}:tools'.format(self.stream)

    @classmethod
    def for_ubuntu(cls, release, series, arch, version, tarfile,
                   revision_build=None, agent_stream=None):
        if revision_build is None:
            filename = '{}-{}-{}-{}.json'.format(
                agent_stream, version, series, arch)
        else:
            filename = 'revision-build-{}-{}-{}.json'.format(
                revision_build, series, arch)
        return cls(
            [(release, series)], arch, version, tarfile, filename,
            revision_build, agent_stream)

    @classmethod
    def for_living_ubuntu(cls, arch, version, tarfile, revision_build=None,
                          agent_stream=None):
        if revision_build is None:
            filename = '{}-{}-ubuntu-{}.json'.format(
                    agent_stream, version, arch)
        else:
            filename = 'revision-build-{}-ubuntu-{}.json'.format(
                    revision_build, arch)
        releases = [
            (juju_series.get_version(name), name) for name
            in juju_series.get_living_names()]
        return cls(
            releases, arch, version, tarfile, filename, revision_build,
            agent_stream)

    @classmethod
    def for_windows(cls, version, tarfile, revision_build=None,
                    agent_stream=None):
        if revision_build is None:
            filename = '{}-{}-windows.json'.format(agent_stream, version)
        else:
            filename = 'revision-build-{}-windows.json'.format(
                revision_build)
        releases = [(r, r) for r in supported_windows_releases]
        return cls(releases, 'amd64', version, tarfile, filename,
                   revision_build, agent_stream)

    @classmethod
    def for_centos(cls, version, tarfile, revision_build=None,
                   agent_stream=None):
        if revision_build is None:
            filename = '{}-{}-centos.json'.format(agent_stream, version)
        else:
            filename = 'revision-build-{}-centos.json'.format(revision_build)
        return cls([('centos7', 'centos7')], 'amd64', version, tarfile,
                   filename, revision_build, agent_stream)

    def make_stanzas(self, hashes, size):
        for release, series in self.releases:
            item_name = '{}-{}-{}'.format(self.version, series, self.arch)
            product_name = 'com.ubuntu.juju:{}:{}'.format(release, self.arch)
            stanza = self.make_path_stanza(product_name, item_name, hashes,
                                           size)
            stanza.update({
                'arch': self.arch,
                'release': series,
                })
            yield stanza


class GUIStanzaWriter(FileStanzaWriter):

    hash_algorithms = frozenset(['sha256', 'sha1', 'md5'])

    @property
    def content_id(self):
        return 'com.canonical.streams:{}:gui'.format(self.stream)

    @classmethod
    def from_tarfile(cls, tarfile, stream):
        """Use a tarfile and stream to instantiate this class."""
        tar_base = os.path.basename(tarfile)
        version = re.match('jujugui-(.*).tar.bz2', tar_base).group(1)
        filename = 'juju-gui-{}-{}.json'.format(stream, version)
        path = '/'.join(['gui', version, tar_base])
        return cls(filename, stream, version, 'tar.bz2', tarfile,
                   path)

    def make_stanzas(self, hashes, size):
        """Return a single stanza for the gui.

        The GUI is arch/os independent, so only one stanza is needed.
        """
        return [self.make_path_stanza(
            'com.canonical.streams:gui', self.version, hashes, size)]


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
    elif args.command == 'gui':
        writer = GUIStanzaWriter.from_tarfile(**kwargs)
    writer.write_stanzas()

if __name__ == '__main__':
    sys.exit(main())
