from argparse import Namespace
import errno
import hashlib
import os
import socket
import sys
from unittest import TestCase

from mock import patch, call

from download_juju import (
    _download,
    download_files,
    download_candidate_juju,
    download_released_juju,
    get_md5,
    mkdir_p,
    parse_args,
    s3_download_files,
    select_build
)
from tests import parse_error
from utility import temp_dir


class SelectBuild(TestCase):

    def test_select_build(self):
        builds = [
            'j/p/r/build-win-client/build-829/c/juju-setup-1.25-alpha1.exe']
        build = select_build(builds)
        self.assertEqual(
            build, 's3://juju-qa-data/j/p/r/build-win-client/build-829')

    def test_select_builds(self):
        builds = [
            'j/p/r/build-win-client/build-829/c/juju-setup-1.25-alpha1.exe',
            'j/p/r/build-win-client/build-1000/c/juju-setup-1.25-alpha1.exe',
            'j/p/v/build-win-client/build-2000/c/juju-setup-1.25-alpha1.exe',
            'j/p/r/build-win-client/build-999/c/juju-setup-1.25-alpha1.exe',
        ]
        build = select_build(builds)
        self.assertEqual(
            build, 's3://juju-qa-data/j/p/v/build-win-client/build-2000')


class TestGetMD5(TestCase):

    def test_get_md5_empty(self):
        with temp_dir() as d:
            filename = os.path.join(d, "afile")
            open(filename, "w").close()
            expected_md5 = hashlib.md5("").hexdigest()
            self.assertEqual(expected_md5, get_md5(filename))

    def test_get_md5_size(self):
        with temp_dir() as d:
            filename = os.path.join(d, "afile")
            contents = "some string longer\nthan the size passed\n"
            with open(filename, "w") as f:
                f.write(contents)
            expected_md5 = hashlib.md5(contents).hexdigest()
            self.assertEqual(expected_md5, get_md5(filename, size=4))


class TestMkdir(TestCase):

    def test_mkdir_p(self):
        with temp_dir() as d:
            path = os.path.join(d, 'a/b/c')
            mkdir_p(path)
            self.assertTrue(os.path.isdir(path))


class TestParseArgs(TestCase):

    def test_parse_args(self):
        with parse_error(self) as stderr:
            parse_args([])
        self.assertRegexpMatches(stderr.getvalue(), 'error: too few arguments')
        self.assertEqual(
            parse_args(['/cred/path']),
            Namespace(credential_path='/cred/path', platform=sys.platform,
                      released=False, revision=None, verbose=0))
        self.assertEqual(
            parse_args(['/cred/path', '-r']),
            Namespace(credential_path='/cred/path', platform=sys.platform,
                      released=True, revision=None, verbose=0))
        self.assertEqual(
            parse_args(['/cred/path', '-r', '-c', '2999', '3001']),
            Namespace(credential_path='/cred/path', platform=sys.platform,
                      released=True, revision=['2999', '3001'], verbose=0))


class TestDownloadFiles(TestCase):

    def test_download(self):
        content = "hello"
        key = KeyStub("test", "hello")
        with temp_dir() as d:
            dst = os.path.join(d, "test.txt")
            _download(key, dst)
            self.assertEqual(content, get_file_content(dst))

    def test_download_retries(self):
        def se(e):
            raise socket.error(errno.ECONNRESET, "reset")
        key = KeyStub("test", "foo")
        with temp_dir() as d:
            dst = os.path.join(d, "test.txt")
            with patch.object(key, 'get_contents_to_filename', autospec=True,
                              side_effect=se) as gcf:
                with self.assertRaises(socket.error):
                    _download(key, dst)
        self.assertEqual(gcf.call_count, 3)

    def test_download_files(self):
        keys = [KeyStub("test.txt", "foo"), KeyStub("test2.txt", "foo2")]
        with temp_dir() as dst:
            downloaded_files = download_files(keys, dst)
            self.assertItemsEqual(os.listdir(dst), ['test.txt', 'test2.txt'])
            self.assertEqual("foo", get_file_content('test.txt', dst))
            self.assertEqual("foo2", get_file_content('test2.txt', dst))
        self.assertItemsEqual(downloaded_files, ['test.txt', 'test2.txt'])

    def test_download_files__matching_file_already_exists(self):
        keys = [KeyStub("test.txt", "foo"), KeyStub("test2.txt", "foo2")]
        with temp_dir() as dst:
            set_file_content("test.txt", "foo", dst)
            downloaded_files = download_files(keys, dst)
            self.assertItemsEqual(os.listdir(dst), ['test.txt', 'test2.txt'])
            self.assertEqual("foo", get_file_content('test.txt', dst))
            self.assertEqual("foo2", get_file_content('test2.txt', dst))
        self.assertItemsEqual(downloaded_files, ['test2.txt'])

    def test_download_files__md5_mismatch(self):
        keys = [KeyStub("test.txt", "foo"), KeyStub("test2.txt", "foo2")]
        with temp_dir() as dst:
            set_file_content("test.txt", "not foo", dst)
            downloaded_files = download_files(keys, dst)
            self.assertItemsEqual(os.listdir(dst), ['test.txt', 'test2.txt'])
            self.assertEqual("foo", get_file_content('test.txt', dst))
            self.assertEqual("foo2", get_file_content('test2.txt', dst))
        self.assertItemsEqual(downloaded_files, ['test.txt', 'test2.txt'])

    def test_download_files__overwrite(self):
        keys = [KeyStub("test.txt", "foo"), KeyStub("test2.txt", "foo2")]
        with temp_dir() as dst:
            set_file_content("test.txt", "foo", dst)
            downloaded_files = download_files(keys, dst, overwrite=True)
            self.assertItemsEqual(os.listdir(dst), ['test.txt', 'test2.txt'])
            self.assertEqual("foo", get_file_content('test.txt', dst))
            self.assertEqual("foo2", get_file_content('test2.txt', dst))
        self.assertItemsEqual(downloaded_files, ['test.txt', 'test2.txt'])

    def test_download_files__suffix(self):
        keys = [KeyStub("test.txt", "foo"), KeyStub("test2.tar", "foo2")]
        with temp_dir() as dst:
            downloaded_files = download_files(keys, dst, suffix=".tar")
            self.assertItemsEqual(os.listdir(dst), ['test2.tar'])
            self.assertEqual("foo2", get_file_content('test2.tar', dst))
        self.assertItemsEqual(downloaded_files, ['test2.tar'])

    def test_download_files__dst_dir_none(self):
        keys = [KeyStub("test.txt", "foo"), KeyStub("test2.txt", "foo2")]
        with patch('download_juju._download', autospec=True) as dj:
            download_files(keys)
        self.assertFalse(dj.called)

    def test_s3_download_files(self):
        with temp_dir() as dst:
            with patch('download_juju.s3_auth_with_rc', autospec=True) as ds:
                with patch(
                        'download_juju.download_files', autospec=True) as dd:
                    s3_download_files('s3://foo/path', '/cred/path', dst)
            ds.assert_called_once_with('/cred/path')
            dd.assert_called_once_with(
                ds.return_value.get_bucket.return_value.list.return_value,
                dst, False, None)

    def test_download_released_juju(self):
        with patch('download_juju.s3_download_files', autospec=True) as dd:
            download_released_juju(
                Namespace(platform='win32', credential_path='/tmp'))
        s3path = 's3://juju-qa-data/client-archive/win/'
        dst = os.path.join(os.environ['HOME'], 'old-juju', 'win')
        dd.assert_called_once_with(s3path, credential_path='/tmp', dst_dir=dst)

    def test_download_candidate_juju(self):
        with patch('download_juju.s3_download_files', autospec=True) as ds:
            with patch('download_juju.select_build', autospec=True) as db:
                download_candidate_juju(
                    Namespace(platform='darwin', credential_path='/tmp',
                              revision=['11', '33']))
        dst = os.path.join(os.environ['HOME'], 'candidate', 'osx')
        self.assertEqual(ds.call_count, 4)
        exp = 's3://juju-qa-data/juju-ci/products/version-{}/build-{}-client/'
        calls = [
            call(exp.format('11', 'osx'), credential_path='/tmp',
                 suffix='.tar.gz'),
            call(db.return_value, credential_path='/tmp', suffix='.tar.gz',
                 dst_dir=dst),
            call(exp.format('33', 'osx'), credential_path='/tmp',
                 suffix='.tar.gz'),
            call(db.return_value, credential_path='/tmp', suffix='.tar.gz',
                 dst_dir=dst),
            ]
        self.assertEqual(ds.call_args_list, calls)
        self.assertEqual(db.call_count, 2)


class KeyStub():

    def __init__(self, name, content=""):
        self.name = name
        self.etag = '"{}"'.format(hashlib.md5(content).hexdigest())
        self.content = content

    def get_contents_to_filename(self, filename):
        with open(filename, "wb") as f:
            f.write(self.content)


def set_file_content(filename, content, dst_dir=None):
    dst_path = os.path.join(dst_dir, filename) if dst_dir else filename
    with open(dst_path, "wb") as f:
        f.write(content)


def get_file_content(filename, dst_dir=None):
    dst_path = os.path.join(dst_dir, filename) if dst_dir else filename
    with open(dst_path) as f:
        return f.read()
