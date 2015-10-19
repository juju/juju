from argparse import Namespace
from unittest import TestCase
from urllib2 import HTTPError

from mock import patch

from manta_sync import (
    Client,
    get_files,
    makedirs,
    PUT,
    PutRequest,
    sync,
    )
from tests import QuietTestCase


EPOCH_MTIME = '1970-01-01T00:00:00.000Z'


class BadMKDir(Exception):
    """Raised when a bad mkdir is requested."""


class FakeClient:

    def __init__(self, user=None):
        self._files_by_dir = {}
        if user is not None:
            self._add_subdir(user)
            self._add_subdir('{}/public'.format(user))
            self._add_to_parent(user, 'public')

    def ls(self, container_name):
        try:
            return self._files_by_dir[container_name]
        except KeyError:
            raise HTTPError('', 404, 'Not Found', None, None)

    def _add_subdir(self, mdir):
        self._files_by_dir.setdefault(mdir, {})

    def _add_to_parent(self, parent, base):
        self._files_by_dir[parent][base] = {
            'type': 'directory', 'name': base,
            'mtime': EPOCH_MTIME,
            }

    def mkdir(self, mdir, parents=False):
        self._add_subdir(mdir)
        if mdir.count('/') < 2:
            raise BadMKDir('All paths must start with user/prefix/')
        parent, base = mdir.rsplit('/', 1)
        self._add_to_parent(parent, base)


class TestFakeClient(TestCase):

    def test_bad_mkdir(self):
        client = FakeClient()
        with self.assertRaises(BadMKDir):
            client.mkdir('jrandom')
        with self.assertRaises(BadMKDir):
            client.mkdir('jrandom/public')
        client.mkdir('jrandom/public/foo')
        self.assertEqual({}, client.ls('jrandom/public/foo'))


class TestClient(TestCase):

    def test_mkdir(self):
        client = Client('http://example.com', 'jrandom', 27)
        with patch('urllib2.urlopen') as uo_mock:
            with patch.object(client, 'make_request_headers',
                              return_value={'foo': 'bar'}) as mrh_mock:
                client.mkdir('jrandom/public/foo')
        mrh_mock.assert_called_once_with({
            'Content-Type': 'application/json; type=directory'
            })
        self.assertEqual(1, uo_mock.call_count)
        request = uo_mock.call_args[0][0]
        self.assertIsInstance(request, PutRequest)
        self.assertEqual({'Foo': 'bar'}, request.headers)
        self.assertEqual('http://example.com/jrandom/public/foo',
                         request.get_full_url())
        self.assertEqual(PUT, request.get_method())

    def test_mkdir_dry_run(self):
        client = Client('http://example.com', 'jrandom', 27, dry_run=True)
        with patch('urllib2.urlopen') as uo_mock:
            with patch.object(client, 'make_request_headers',
                              return_value={'foo': 'bar'}):
                client.mkdir('jrandom/public/foo')
        uo_mock.assert_not_called()


class TestGetFiles(TestCase):

    def test_no_directory(self):
        client = FakeClient()
        self.assertIs(None, get_files('foo', client))

    def test_empty_directory(self):
        client = FakeClient(user='jrandom')
        client.mkdir('jrandom/public/foo')
        self.assertEqual({}, get_files('jrandom/public/foo', client))


class TestSync(QuietTestCase):

    def test_creates_directory(self):
        client = FakeClient(user='jrandom')
        sync(Namespace(files=[], verbose=False),
             'jrandom/public/foo/bar/baz', client)
        self.assertEqual(client.ls('jrandom/public/foo'), {
            'bar': {
                'name': 'bar',
                'type': 'directory',
                'mtime': EPOCH_MTIME,
                }
            })
        self.assertEqual(client.ls('jrandom/public/foo/bar'), {
            'baz': {
                'name': 'baz',
                'type': 'directory',
                'mtime': EPOCH_MTIME,
                }
            })
        self.assertEqual(client.ls('jrandom/public/foo/bar/baz'), {})


class TestMakedirs(TestCase):

    def test_makedirs(self):
        client = FakeClient('jrandom')
        makedirs('jrandom/public/foo/bar/baz', client)
        self.assertEqual(client.ls('jrandom/public'), {
            'foo': {
                'name': 'foo',
                'type': 'directory',
                'mtime': EPOCH_MTIME,
                }})
        self.assertEqual(client.ls('jrandom/public/foo'), {
            'bar': {
                'name': 'bar',
                'type': 'directory',
                'mtime': EPOCH_MTIME,
                }})
        self.assertEqual(client.ls('jrandom/public/foo'), {
            'bar': {
                'name': 'bar',
                'type': 'directory',
                'mtime': EPOCH_MTIME,
                }})
        self.assertEqual(client.ls('jrandom/public/foo/bar'), {
            'baz': {
                'name': 'baz',
                'type': 'directory',
                'mtime': EPOCH_MTIME,
                }})
        self.assertEqual(client.ls('jrandom/public/foo/bar/baz'), {})
