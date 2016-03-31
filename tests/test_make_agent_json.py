import json
import os
from tempfile import NamedTemporaryFile
from unittest import TestCase

from mock import patch

from build_package import juju_series
from make_agent_json import (
    GUIStanzaWriter,
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

    def test_for_living_ubuntu_revision_build(self):
        writer = StanzaWriter.for_living_ubuntu('IA64', '1.27',
                                                'tarfile.tar.gz', 3565)
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

    def test_for_living_ubuntu_agent_stream(self):
        writer = StanzaWriter.for_living_ubuntu('IA64', '1.27',
                                                'tarfile.tar.gz',
                                                agent_stream='escaped')
        releases = [
            (juju_series.get_version(name), name) for name
            in juju_series.get_living_names()]
        self.assertEqual(releases, writer.releases)
        self.assertEqual('IA64', writer.arch)
        self.assertEqual('1.27', writer.version)
        self.assertEqual('escaped', writer.agent_stream)
        self.assertEqual('agent/1.27/tarfile.tar.gz',
                         writer.agent_path)
        self.assertEqual('tarfile.tar.gz', writer.tarfile)
        self.assertEqual('escaped-1.27-ubuntu-IA64.json', writer.filename)

    def test_for_windows_revision_build(self):
        writer = StanzaWriter.for_windows('1.27', 'tarfile.tar.gz', 3565)
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

    def test_for_windows_agent_stream(self):
        writer = StanzaWriter.for_windows('1.27', 'tarfile.tar.gz',
                                          agent_stream='escaped')
        releases = [(r, r) for r in supported_windows_releases]
        self.assertEqual(releases, writer.releases)
        self.assertEqual('amd64', writer.arch)
        self.assertEqual('1.27', writer.version)
        self.assertEqual('escaped', writer.agent_stream)
        self.assertEqual('agent/1.27/tarfile.tar.gz',
                         writer.agent_path)
        self.assertEqual('tarfile.tar.gz', writer.tarfile)
        self.assertEqual('escaped-1.27-windows.json', writer.filename)

    def test_for_centos_revision_build(self):
        writer = StanzaWriter.for_centos('1.27', 'tarfile.tar.gz', 3565)
        self.assertEqual([('centos7', 'centos7')], writer.releases)
        self.assertEqual('amd64', writer.arch)
        self.assertEqual('1.27', writer.version)
        self.assertEqual('revision-build-3565', writer.agent_stream)
        self.assertEqual('agent/revision-build-3565/tarfile.tar.gz',
                         writer.agent_path)
        self.assertEqual('tarfile.tar.gz', writer.tarfile)
        self.assertEqual('revision-build-3565-centos.json',
                         writer.filename)

    def test_for_centos_agent_stream(self):
        writer = StanzaWriter.for_centos('1.27', 'tarfile.tar.gz',
                                         agent_stream='escaped')
        self.assertEqual([('centos7', 'centos7')], writer.releases)
        self.assertEqual('amd64', writer.arch)
        self.assertEqual('1.27', writer.version)
        self.assertEqual('escaped', writer.agent_stream)
        self.assertEqual('agent/1.27/tarfile.tar.gz',
                         writer.agent_path)
        self.assertEqual('tarfile.tar.gz', writer.tarfile)
        self.assertEqual('escaped-1.27-centos.json', writer.filename)

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


class TestGUIStanzaWriter(TestCase):

    def test_content_id(self):
        writer = GUIStanzaWriter('a', 'testing', 'c', 'd', 'e', 'f')
        self.assertEqual('com.canonical.streams:testing:gui',
                         writer.content_id)

    def test_from_tarfile(self):
        writer = GUIStanzaWriter.from_tarfile('3.14.tar.gz', 'escape')
        self.assertEqual('juju-gui-escape-3.14.json', writer.filename)
        self.assertEqual('escape', writer.agent_stream)
        self.assertEqual('3.14', writer.version)
        self.assertEqual('tar.gz', writer.ftype)
        self.assertEqual('3.14.tar.gz', writer.tarfile)
        self.assertEqual('gui/3.14/3.14.tar.gz', writer.agent_path)

    def test_make_stanzas(self):
        writer = GUIStanzaWriter('a', 'b', 'c', 'd', 'e', 'f')
        writer.version_name = 'g'
        result, = writer.make_stanzas({'h': 'i'}, 314)
        self.assertEqual({
            'content_id': 'com.canonical.streams:b:gui',
            'item_name': 'c',
            'version': 'c',
            'ftype': 'd',
            'path': 'f',
            'version_name': 'g',
            'h': 'i',
            'size': 314,
            'format': 'products:1.0',
            'product_name': 'com.canonical.streams:gui',
            }, result)
