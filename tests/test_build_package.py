"""Tests for build_package script."""

from mock import patch
import os
import shutil
import tempfile
import unittest

from build_package import (
    build_binary,
    get_args,
    main,
)


class BuildPackageTestCase(unittest.TestCase):

    def test_get_args_binary(self):
        args = get_args(['prog', 'binary', 'my.dsc', '~/workspace'])
        self.assertEqual('binary', args.command)
        self.assertEqual('my.dsc', args.dsc)
        self.assertEqual('~/workspace', args.location)
        self.assertFalse(args.verbose)

    def test_main_binary(self):
        with patch('build_package.build_binary', autospec=True,
                   return_value=0) as bb_mock:
            code = main(['prog', 'binary', 'my.dsc', '~/workspace'])
        self.assertEqual(0, code)
        bb_mock.assert_called_with('my.dsc', '~/workspace', verbose=False)

    def test_build_binary(self):
        pass
