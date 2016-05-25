"""Tests for charm related classes and helpers."""

import os

import yaml

from jujucharm import (
    Charm,
    local_charm_path,
)
from tests import (
    temp_os_env,
    TestCase,
)
from utility import (
    temp_dir,
)


class TestCharm(TestCase):

    def test_create_default(self):
        charm = Charm('test', 'a summary')
        expected = {
            'name': 'test',
            'summary': 'a summary',
            'series': ('xenial', 'trusty'),
            'maintainer': 'juju-qa@lists.canonical.com',
        }
        self.assertEqual(charm.metadata, expected)

    def test_default_series_default(self):
        charm = Charm('test', 'a summary')
        self.assertEqual(charm.default_series, 'xenial')

    def test_default_series_unset(self):
        charm = Charm('test', 'a summary')
        del charm.metadata['series']
        self.assertEqual(charm.default_series, 'xenial')

    def test_default_series_single(self):
        charm = Charm('test', 'a summary', series='wily')
        self.assertEqual(charm.default_series, 'wily')

    def test_default_series_list(self):
        charm = Charm('test', 'a summary', series=['trusty', 'xenial'])
        self.assertEqual(charm.default_series, 'trusty')

    def test_to_dir(self):
        charm = Charm('test', 'a summary')
        charm.metadata['description'] = 'a description'
        del charm.metadata['maintainer']
        with temp_dir() as charm_dir:
            charm.to_dir(charm_dir)
            metafile = os.path.join(charm_dir, 'metadata.yaml')
            with open(metafile) as f:
                metadata = yaml.load(f)
        expected = {
            'name': 'test',
            'summary': 'a summary',
            'description': 'a description',
            'series': ['xenial', 'trusty'],
        }
        self.assertEqual(metadata, expected)

    def test_to_repo_dir(self):
        charm = Charm('test', 'a summary', series='wily')
        with temp_dir() as repo_dir:
            charm.to_repo_dir(repo_dir)
            metafile = os.path.join(repo_dir, 'wily', 'test', 'metadata.yaml')
            with open(metafile) as f:
                metadata = yaml.load(f)
        expected = {
            'name': 'test',
            'summary': 'a summary',
            'series': 'wily',
            'maintainer': Charm.DEFAULT_MAINTAINER,
        }
        self.assertEqual(metadata, expected)

    def test_add_hook_script(self):
        charm = Charm('test', 'a summary')
        config_changed = '#!/bin/sh\necho changed'
        charm.add_hook_script('config-changed', config_changed)
        with temp_dir() as charm_dir:
            charm.to_dir(charm_dir)
            hookfile = os.path.join(charm_dir, 'hooks', 'config-changed')
            self.assertTrue(os.access(hookfile, os.X_OK))
            with open(hookfile) as f:
                self.assertEqual(f.read(), config_changed)

    def test_add_hook_multiple(self):
        charm = Charm('test', 'a summary')
        config_changed = '#!/bin/sh\necho changed'
        upgrade_charm = '#!/bin/sh\necho upgraded'
        charm.add_hook_script('config-changed', config_changed)
        charm.add_hook_script('upgrade-charm', upgrade_charm)
        with temp_dir() as charm_dir:
            charm.to_dir(charm_dir)
            changedfile = os.path.join(charm_dir, 'hooks', 'config-changed')
            self.assertTrue(os.access(changedfile, os.X_OK))
            with open(changedfile) as f:
                self.assertEqual(f.read(), config_changed)
            upgradedfile = os.path.join(charm_dir, 'hooks', 'upgrade-charm')
            self.assertTrue(os.access(upgradedfile, os.X_OK))
            with open(upgradedfile) as f:
                self.assertEqual(f.read(), upgrade_charm)


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
