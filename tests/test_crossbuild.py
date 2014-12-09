from mock import patch
from unittest import TestCase

from crossbuild import (
    main,
)
from utils import temp_dir


class CrossBuildTestCase(TestCase):

    def test_main_setup(self):
        with temp_dir() as base_dir:
            with patch('crossbuild.setup_cross_building') as mock:
                main(['-d', '-v', 'setup', '--build-dir', base_dir])
        args, kwargs = mock.call_args
        self.assertEqual((base_dir, ), args)
        self.assertEqual({'dry_run': True, 'verbose': True}, kwargs)
