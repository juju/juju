from mock import patch
import os
from StringIO import StringIO
from unittest import TestCase

from update_lxc_cache import (
    INDEX,
    INDEX_PATH,
    INSTALL_SCRIPT,
    LxcCache,
    main,
    parse_args,
    PUT_SCRIPT,
    System
    )
from utility import temp_dir


INDEX_DATA = """\
ubuntu;trusty;arm64;default;201501;/images/ubuntu/trusty/arm64/default/201501/
ubuntu;trusty;armhf;default;201502;/images/ubuntu/trusty/armhf/default/201502/
ubuntu;trusty;i386;default;201503;/images/ubuntu/trusty/i386/default/201503/
ubuntu;trusty;ppc64el;default;20154;/images/ubuntu/trusty/ppc64el/default/20154/
"""


def make_systems(data=INDEX_DATA):
    systems = {}
    for line in data.splitlines():
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


class FakeResponse(StringIO):

    def __init__(self, code, data, *args, **kwargs):
        StringIO.__init__(self, *args, **kwargs)
        self.code = code
        self.write(data)
        self.seek(0)

    def getcode(self):
        return self.code


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
        url = 'https://images.linuxcontainers.org/meta/1.0/index-system'
        response = FakeResponse(200, INDEX_DATA)
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
                new_system, new_data = lxc_cache.get_updates(
                    'ubuntu', 'trusty', 'ppc64el', 'default')
        self.assertIsNone(new_system)
        self.assertIsNone(new_data)
        is_mock.assert_called_with(
            'https://images.linuxcontainers.org/meta/1.0/index-system')

    def test_get_updates_found(self):
        remote_data = INDEX_DATA.replace(
            'ppc64el;default;20154', 'ppc64el;default;20159')
        remote_systems = make_systems(remote_data)
        remote_updates = (remote_systems, remote_data)
        with temp_dir() as workspace:
            make_local_cache(workspace)
            lxc_cache = LxcCache(workspace)
            with patch.object(lxc_cache, 'init_systems', autospec=True,
                              return_value=remote_updates) as is_mock:
                new_system, new_data = lxc_cache.get_updates(
                    'ubuntu', 'trusty', 'ppc64el', 'default')
        key = ('ubuntu', 'trusty', 'ppc64el', 'default')
        self.assertEqual(remote_systems[key], new_system)
        self.assertEqual(remote_data, new_data)
        is_mock.assert_called_with(
            'https://images.linuxcontainers.org/meta/1.0/index-system')

    def test_get_lxc_data(self):
        systems = make_systems()
        system = systems[('ubuntu', 'trusty', 'ppc64el', 'default')]
        with temp_dir() as workspace:
            make_local_cache(workspace)
            lxc_cache = LxcCache(workspace)
            with patch.object(lxc_cache, 'download', autospec=True) as d_mock:
                rootfs_path, meta_path = lxc_cache.get_lxc_data(system)
            image_path = os.path.join(
                workspace, 'images/ubuntu/trusty/ppc64el/default/20154/')
            self.assertTrue(os.path.isdir(image_path))
        expected_rootfs_path = os.path.join(image_path, 'rootfs.tar.xz')
        expected_meta_path = os.path.join(image_path, 'meta.tar.xz')
        self.assertEqual(expected_rootfs_path, rootfs_path)
        self.assertEqual(expected_meta_path, meta_path)
        d_mock.assert_any_call(
            'https://images.linuxcontainers.org'
            '/images/ubuntu/trusty/ppc64el/default/20154/rootfs.tar.xz',
            rootfs_path)
        d_mock.assert_any_call(
            'https://images.linuxcontainers.org'
            '/images/ubuntu/trusty/ppc64el/default/20154/meta.tar.xz',
            meta_path)

    def test_put_lxc_data(self):
        systems = make_systems()
        system = systems[('ubuntu', 'trusty', 'ppc64el', 'default')]
        with patch('subprocess.check_call', autospec=True) as cc_mock:
            lxc_cache = LxcCache('workspace')
            lxc_cache.put_lxc_data(
                'user@host', system, '/rootfs_path', '/meta_path')
        put_script = PUT_SCRIPT.format(
            user_host='user@host', rootfs_path='/rootfs_path',
            meta_path='/meta_path')
        cc_mock.assert_any_call([put_script], shell=True)
        cache_path = '/var/cache/lxc/download/ubuntu/trusty/ppc64el/default'
        install_script = INSTALL_SCRIPT.format(
            user_host='user@host', lxc_cache=cache_path,
            rootfs='rootfs.tar.xz', meta='meta.tar.xz')
        cc_mock.assert_any_call([install_script], shell=True)

    def test_save_index(self):
        with temp_dir() as workspace:
            lxc_cache = LxcCache(workspace)
            lxc_cache.save_index(INDEX_DATA)
            index_path = os.path.join(workspace, 'meta/1.0/index-system')
            self.assertTrue(os.path.isfile(index_path))
            with open(index_path) as f:
                data = f.read()
        self.assertEqual(INDEX_DATA, data)

    def test_download(self):
        response = FakeResponse(200, 'rootfs.tar.xz')
        with temp_dir() as workspace:
            with patch('urllib2.Request', autospec=True,
                       return_value='request') as r_mock:
                with patch('urllib2.urlopen', autospec=True,
                           return_value=response) as ul_mock:
                    lxc_cache = LxcCache(workspace)
                    file_path = os.path.join(workspace, 'rootfs.tar.xz')
                    lxc_cache.download('url', file_path)
            self.assertTrue(os.path.isfile(file_path))
            with open(file_path) as f:
                data = f.read()
        self.assertEqual('rootfs.tar.xz', data)
        r_mock.assert_called_with('url')
        ul_mock.assert_called_with('request')
