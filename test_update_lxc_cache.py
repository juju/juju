from mock import patch
import os
from unittest import TestCase

from update_lxc_cache import (
    LxcCache,
    main,
    parse_args,
    )


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
