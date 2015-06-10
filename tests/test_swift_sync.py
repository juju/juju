from argparse import Namespace
import hashlib
from mock import patch
import os
from unittest import TestCase

from swift_sync import (
    upload_changes,
)
from utils import (
    temp_dir,
)


class SwiftSyncTestCase(TestCase):

    def test_upload_changes(self):
        md5 = hashlib.md5()
        md5.update('one')
        one_hash = md5.hexdigest()
        with temp_dir() as base:
            remote_name = os.path.join(base, 'one')
            remote_files = {
                remote_name: {'name': remote_name, 'hash': one_hash}
            }
            local_files = []
            for name in ['one', 'two']:
                file_path = os.path.join(base, name)
                local_files.append(file_path)
                with open(file_path, 'w') as f:
                    f.write(name)
            args = Namespace(
                container='foo', path=base, files=local_files,
                verbose=False, dry_run=False)
            with patch('subprocess.check_output', autospec=True,
                       return_value='two') as co_mock:
                uploaded_files = upload_changes(args, remote_files)
        self.assertEqual(['two'], uploaded_files)
        co_mock.assert_called_once_with(
            ['swift', 'upload', base, os.path.join(base, 'two')])
