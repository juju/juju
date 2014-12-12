from mock import patch
import os
from unittest import TestCase

from build_juju import (
    build_juju,
    main,
)
from utility import temp_dir


class JujuBuildTestCase(TestCase):

    def test_main_options(self):
        with patch('build_juju.build_juju') as mock:
            main(['-d', '-v', '-b', '1234', 'win-client', './foo'])
            args, kwargs = mock.call_args
            self.assertEqual(('win-client', '1234', './foo'), args)
            self.assertTrue(kwargs['dry_run'])
            self.assertTrue(kwargs['verbose'])

    def test_build_juju(self):
        with temp_dir() as base_dir:
            work_dir = os.path.join(base_dir, 'workspace')
            with patch('build_juju.setup_workspace') as sw_mock:
                build_juju(
                    'win-client', work_dir, 'lastSucessful',
                    dry_run=True, verbose=True)
        self.assertEqual((work_dir, ), sw_mock.call_args[0])
        self.assertEqual(
            {'dry_run': True, 'verbose': True}, sw_mock.call_args[1])
