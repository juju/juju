from mock import patch
import os
from unittest import TestCase

from update_lxc_cache import (
    INDEX,
    INDEX_PATH,
    LxcCache,
    main,
    parse_args,
    SITE,
    System
    )
from utility import temp_dir


INDEX_DATA = """\
ubuntu;trusty;arm64;default;201505;/images/ubuntu/trusty/arm64/default/201505/
ubuntu;trusty;armhf;default;201505;/images/ubuntu/trusty/armhf/default/201505/
ubuntu;trusty;i386;default;201505;/images/ubuntu/trusty/i386/default/201505/
ubuntu;trusty;ppc64el;default;20150;/images/ubuntu/trusty/ppc64el/default/20150/
"""


def make_systems():
    systems = {}
    for line in INDEX_DATA.splitlines():
        system = System(*line.split(';'))
        key = (system.dist, system.release, system.arch, system.variant)
        systems[key] = system
    return systems


def make_local_cache(workspace):
    meta_dir = os.path.join(workspace, INDEX_PATH)
    os.makedirs(meta_dir)
    index_path = os.path.join(meta_dir, INDEX)
    with open(index_path, 'w') as f:
        f.write(INDEX_DATA)
    return index_path


class FakeResponse:

    def __init__(self, data, code=200):
        self.data = data
        self.code = code

    def getcode(self):
        return self.code

    def read(self):
        return self.data


class UpdateLxcCacheTestCase(TestCase):

    def test_parse_args(self):
        args = parse_args(
            ['-d', '-v', 'user@host', 'trusty', 'ppc64el', './workspace'])
        self.assertEqual('./workspace', args.workspace)
        self.assertEqual('trusty', args.release)
        self.assertEqual('ppc64el', args.arch)
        self.assertEqual('ubuntu', args.dist)
        self.assertEqual('default', args.variant)
        self.assertTrue(args.verbose)
        self.assertTrue(args.dry_run)

    @patch('update_lxc_cache.LxcCache.save_index', autospec=True)
    @patch('update_lxc_cache.LxcCache.put_lxc_data', autospec=True)
    @patch('update_lxc_cache.LxcCache.get_lxc_data', autospec=True,
           return_value=('rootfs_path', 'meta_path'))
    @patch('update_lxc_cache.LxcCache.get_updates', autospec=True,
           return_value=('system', 'data'))
    def test_main(self, gu_mock, gl_mock, pl_mock, si_mock):
        lxc_cache = LxcCache('./workspace')
        rc = main(
            ['-d', '-v', 'user@host', 'trusty', 'ppc64el', './workspace'])
        self.assertEqual(0, rc)
        lxc_cache = gu_mock.call_args[0][0]
        self.assertIsInstance(lxc_cache, LxcCache)
        self.assertEqual(os.path.abspath('./workspace'), lxc_cache.workspace)
        self.assertTrue(lxc_cache.verbose)
        self.assertTrue(lxc_cache.dry_run)
        gu_mock.assert_called_with(
            lxc_cache, 'ubuntu', 'trusty', 'ppc64el', 'default')
        gl_mock.assert_called_with(lxc_cache, 'system')
        pl_mock.assert_called_with(
            lxc_cache, 'user@host', 'system', 'rootfs_path', 'meta_path')
        si_mock.assert_called_with(lxc_cache, 'data')


class LxcCacheTestCase(TestCase):

    def test_init(self):
        lxc_cache = LxcCache('./workspace', True, True)
        self.assertEqual(os.path.abspath('./workspace'), lxc_cache.workspace)
        self.assertTrue(lxc_cache.verbose)
        self.assertTrue(lxc_cache.dry_run)
        self.assertEqual({}, lxc_cache.systems)

    def test_init_with_local_cache(self):
        with temp_dir() as workspace:
            make_local_cache(workspace)
            lxc_cache = LxcCache(workspace)
        expected_systems = make_systems()
        self.assertEqual(expected_systems, lxc_cache.systems)

    def test_init_systems_without_local_cache(self):
        with temp_dir() as workspace:
            lxc_cache = LxcCache(workspace)
            systems, data = lxc_cache.init_systems('workspace')
        self.assertEqual({}, systems)
        self.assertIsNone(data)

    def test_init_systems_with_local_cache(self):
        with temp_dir() as workspace:
            index_path = make_local_cache(workspace)
            lxc_cache = LxcCache(workspace)
            systems, data = lxc_cache.init_systems(index_path)
        expected_systems = make_systems()
        self.assertEqual(expected_systems, systems)
        self.assertEqual(INDEX_DATA, data)

    def test_init_systems_with_remote_location(self):
        url = '%s/%s/%s' % (SITE, INDEX_PATH, INDEX)
        response = FakeResponse(INDEX_DATA)
        with patch('urllib2.Request', autospec=True,
                   return_value='request') as r_mock:
            with patch('urllib2.urlopen', autospec=True,
                       return_value=response) as ul_mock:
                with temp_dir() as workspace:
                    lxc_cache = LxcCache(workspace)
                    systems, data = lxc_cache.init_systems(url)
        expected_systems = make_systems()
        self.assertEqual(expected_systems, systems)
        self.assertEqual(INDEX_DATA, data)
        ul_mock.assert_called_with('request')
        r_mock.assert_called_with(url)

    def test_get_updates_none(self):
        systems = make_systems()
        with temp_dir() as workspace:
            make_local_cache(workspace)
            lxc_cache = LxcCache(workspace)
            with patch.object(lxc_cache, 'init_systems', autospec=True,
                              return_value=(systems, INDEX_DATA)) as is_mock:
                new_system, data = lxc_cache.get_updates(
                    'ubuntu', 'trusty', 'ppc64el', 'default')
        self.assertIsNone(new_system)
        self.assertIsNone(data)
        url = '%s/%s/%s' % (SITE, INDEX_PATH, INDEX)
        is_mock.assert_called_with(url)
