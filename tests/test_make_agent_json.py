import json
import os
from tempfile import NamedTemporaryFile
from unittest import TestCase

from mock import patch

from build_package import juju_series
from make_agent_json import (
    StanzaWriter,
    supported_windows_releases,
    )


class TestStanzaWriter(TestCase):

    def test_for_ubuntu_revision_build(self):
        writer = StanzaWriter.for_ubuntu(
            '18.04', 'angsty', 'IA64', '1.27', 'tarfile.tar.gz', 3565)
        self.assertEqual([('18.04', 'angsty')], writer.releases)
        self.assertEqual('IA64', writer.arch)
        self.assertEqual('1.27', writer.version)
        self.assertEqual('revision-build-3565', writer.agent_stream)
        self.assertEqual('agent/revision-build-3565/tarfile.tar.gz',
                         writer.agent_path)
        self.assertEqual('tarfile.tar.gz', writer.tarfile)
        self.assertEqual('revision-build-3565-angsty-IA64.json',
                         writer.filename)

    def test_for_ubuntu_agent_stream(self):
        writer = StanzaWriter.for_ubuntu(
            '18.04', 'angsty', 'IA64', '1.27', 'tarfile.tar.gz',
            agent_stream='escaped')
        self.assertEqual([('18.04', 'angsty')], writer.releases)
        self.assertEqual('IA64', writer.arch)
        self.assertEqual('1.27', writer.version)
        self.assertEqual('escaped', writer.agent_stream)
        self.assertEqual('agent/1.27/tarfile.tar.gz',
                         writer.agent_path)
        self.assertEqual('tarfile.tar.gz', writer.tarfile)
        self.assertEqual('escaped-1.27-angsty-IA64.json', writer.filename)

    def test_for_living_ubuntu(self):
        writer = StanzaWriter.for_living_ubuntu('IA64', '1.27', 3565,
                                                'tarfile.tar.gz')
        releases = [
            (juju_series.get_version(name), name) for name
            in juju_series.get_living_names()]
        self.assertEqual(releases, writer.releases)
        self.assertEqual('IA64', writer.arch)
        self.assertEqual('1.27', writer.version)
        self.assertEqual('revision-build-3565', writer.agent_stream)
        self.assertEqual('agent/revision-build-3565/tarfile.tar.gz',
                         writer.agent_path)
        self.assertEqual('tarfile.tar.gz', writer.tarfile)
        self.assertEqual('revision-build-3565-ubuntu-IA64.json',
                         writer.filename)

    def test_for_windows(self):
        writer = StanzaWriter.for_windows('1.27', 3565, 'tarfile.tar.gz')
        releases = [(r, r) for r in supported_windows_releases]
        self.assertEqual(releases, writer.releases)
        self.assertEqual('amd64', writer.arch)
        self.assertEqual('1.27', writer.version)
        self.assertEqual('revision-build-3565', writer.agent_stream)
        self.assertEqual('agent/revision-build-3565/tarfile.tar.gz',
                         writer.agent_path)
        self.assertEqual('tarfile.tar.gz', writer.tarfile)
        self.assertEqual('revision-build-3565-windows.json',
                         writer.filename)

    def test_for_centos(self):
        writer = StanzaWriter.for_centos('1.27', 3565, 'tarfile.tar.gz')
        self.assertEqual([('centos7', 'centos7')], writer.releases)
        self.assertEqual('amd64', writer.arch)
        self.assertEqual('1.27', writer.version)
        self.assertEqual('revision-build-3565', writer.agent_stream)
        self.assertEqual('agent/revision-build-3565/tarfile.tar.gz',
                         writer.agent_path)
        self.assertEqual('tarfile.tar.gz', writer.tarfile)
        self.assertEqual('revision-build-3565-centos.json',
                         writer.filename)

    def test_write_stanzas(self):
        with NamedTemporaryFile() as tempfile:
            writer = StanzaWriter([('18.04', 'angsty')], 'IA64', '2.0-zeta1',
                                  tempfile.name, tempfile.name, 3565)
            writer.version_name = '20160207'
            with patch('sys.stderr'):
                writer.write_stanzas()
            tempfile.seek(0)
            output = json.load(tempfile)
        agent_path = os.path.basename(tempfile.name)
        expected = {
            'format': 'products:1.0',
            'product_name': 'com.ubuntu.juju:18.04:IA64',
            'content_id': 'com.ubuntu.juju:revision-build-3565:tools',
            'item_name': '2.0-zeta1-angsty-IA64',
            'version_name': '20160207',
            'version': '2.0-zeta1',
            'release': 'angsty',
            'arch': 'IA64',
            'path': 'agent/revision-build-3565/{}'.format(agent_path),
            'ftype': 'tar.gz',
            'size': 0,
            'md5': u'd41d8cd98f00b204e9800998ecf8427e',
            'sha256': 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991'
                      'b7852b855',
          }
        self.assertEqual([expected], output)
