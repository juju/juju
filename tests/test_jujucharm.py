"""Tests for charm related classes and helpers."""

import os

import yaml

from jujucharm import (
    Charm,
    local_charm_path,
    make_charm,
)
from tests import (
    temp_os_env,
    TestCase,
)
from utility import (
    temp_dir,
)


class TestLocalCharm(TestCase):

    def test_make_local_charm_1x(self):
        charm = 'mysql'
        path = local_charm_path(charm, '1.25.0')
        self.assertEqual(path, 'local:mysql')

    def test_make_local_charm_1x_series(self):
        charm = 'mysql'
        path = local_charm_path(charm, '1.25.0', series='trusty')
        self.assertEqual(path, 'local:trusty/mysql')

    def test_make_local_charm_2x(self):
        charm = 'mysql'
        path = local_charm_path(charm, '2.0.0', repository='/tmp/charms')
        self.assertEqual(path, '/tmp/charms/mysql')

    def test_make_local_charm_2x_os_env(self):
        charm = 'mysql'
        with temp_os_env('JUJU_REPOSITORY', '/home/foo/repository'):
            path = local_charm_path(charm, '2.0.0')
        self.assertEqual(path, '/home/foo/repository/charms/mysql')

    def test_make_local_charm_2x_win(self):
        charm = 'mysql'
        with temp_os_env('JUJU_REPOSITORY', '/home/foo/repository'):
            path = local_charm_path(charm, '2.0.0', platform='win')
        self.assertEqual(path, '/home/foo/repository/charms-win/mysql')

    def test_make_local_charm_2x_centos(self):
        charm = 'mysql'
        with temp_os_env('JUJU_REPOSITORY', '/home/foo/repository'):
            path = local_charm_path(charm, '2.0.0', platform='centos')
        self.assertEqual(path, '/home/foo/repository/charms-centos/mysql')


class TestMakeCharm(TestCase):

    def test_make_charm(self):
        with temp_dir() as charm_dir:
            make_charm(charm_dir)
            metadata = os.path.join(charm_dir, 'metadata.yaml')
            with open(metadata, 'r') as f:
                content = yaml.load(f)
        self.assertEqual(content['name'], 'dummy')
        self.assertEqual(content['min-juju-version'], '1.25.0')
        self.assertEqual(content['summary'], 'summary')

    def test_make_charm_non_default(self):
        with temp_dir() as charm_dir:
            make_charm(charm_dir, min_ver='2.0.0', name='foo',
                       description='bar', summary='foobar',
                       series=['trusty', 'xenial'])
            metadata = os.path.join(charm_dir, 'metadata.yaml')
            with open(metadata, 'r') as f:
                content = yaml.load(f)
        expected = {'series': ['trusty', 'xenial'],
                    'name': 'foo',
                    'description': 'bar',
                    'maintainer': Charm.DEFAULT_MAINTAINER,
                    'min-juju-version': '2.0.0',
                    'summary': 'foobar'}
        self.assertEqual(content, expected)

    def test_make_charm_none(self):
        with temp_dir() as charm_dir:
            make_charm(charm_dir, min_ver=None, name='mycharm',
                       description='foo-description', summary='foo-summary',
                       series=None)
            metadata = os.path.join(charm_dir, 'metadata.yaml')
            with open(metadata, 'r') as f:
                content = yaml.safe_load(f)
        expected = {'name': 'mycharm',
                    'description': 'foo-description',
                    'maintainer': Charm.DEFAULT_MAINTAINER,
                    'summary': 'foo-summary'}
        self.assertEqual(content, expected)
