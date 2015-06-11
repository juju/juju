from argparse import Namespace
import hashlib
from mock import patch
import os
from subprocess import CalledProcessError
from unittest import TestCase

from swift_sync import (
    upload_changes,
)
from utils import (
    temp_dir,
)


def make_local_files(base, files):
    local_files = []
    for name in files:
        file_path = os.path.join(base, name)
        local_files.append(file_path)
        with open(file_path, 'w') as f:
            f.write(name)
    return local_files


class SwiftSyncTestCase(TestCase):

    def test_upload_changes(self):
        # Only new and changed files are uploaded.
        md5 = hashlib.md5()
        md5.update('one')
        one_hash = md5.hexdigest()
        with temp_dir() as base:
            remote_name = os.path.join(base, 'one')
            remote_files = {
                remote_name: {'name': remote_name, 'hash': one_hash}
            }
            local_files = make_local_files(base, ['one', 'two'])
            args = Namespace(
                container='foo', path=base, files=local_files,
                verbose=False, dry_run=False)
            with patch('subprocess.check_output', autospec=True,
                       return_value='two') as co_mock:
                uploaded_files = upload_changes(args, remote_files)
        self.assertEqual(['two'], uploaded_files)
        co_mock.assert_called_once_with(
            ['swift', 'upload', base, os.path.join(base, 'two')])

    def test_upload_changes_reties(self):
        with temp_dir() as base:
            remote_files = {}
            local_files = make_local_files(base, ['one'])
            args = Namespace(
                container='foo', path=base, files=local_files,
                verbose=False, dry_run=False)
            outputs = [
                CalledProcessError(1, 'a'),
                CalledProcessError(2, 'b'),
                'one']
            with patch('subprocess.check_output', autospec=True,
                       side_effect=outputs) as co_mock:
                uploaded_files = upload_changes(args, remote_files)
        self.assertEqual(['one'], uploaded_files)
        self.assertEqual(3, co_mock.call_count)
        co_mock.assert_called_with(
            ['swift', 'upload', base, os.path.join(base, 'one')])

    def test_upload_changes_error(self):
        with temp_dir() as base:
            remote_files = {}
            local_files = make_local_files(base, ['one'])
            args = Namespace(
                container='foo', path=base, files=local_files,
                verbose=False, dry_run=False)
            outputs = [
                CalledProcessError(1, 'a'),
                CalledProcessError(2, 'b'),
                CalledProcessError(3, 'c')]
            with patch('subprocess.check_output', autospec=True,
                       side_effect=outputs) as co_mock:
                with self.assertRaises(CalledProcessError):
                    upload_changes(args, remote_files)
            self.assertEqual(3, co_mock.call_count)
