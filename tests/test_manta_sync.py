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


class FakeClient:

    def __init__(self):
        self._files_by_dir = {}

    def ls(self, container_name):
        try:
            return self._files_by_dir[container_name]
        except KeyError:
            raise HTTPError('', 404, 'Not Found', None, None)

    def mkdir(self, mdir, parents=False):
        self._files_by_dir.setdefault(mdir, {})


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
        client = FakeClient()
        client.mkdir('foo')
        self.assertEqual({}, get_files('foo', client))


class TestSync(QuietTestCase):

    def test_creates_directory(self):
        client = FakeClient()
        sync(Namespace(files=[], verbose=False), 'foo/bar/baz', client)
        self.assertEqual(client.ls('foo'), {})
        self.assertEqual(client.ls('foo/bar'), {})
        self.assertEqual(client.ls('foo/bar/baz'), {})


class TestMakedirs(TestCase):

    def test_makedirs(self):
        client = FakeClient()
        makedirs('jrandom/public/foo/bar/baz', client)
        self.assertEqual(client.ls('jrandom/public/foo'), {})
        self.assertEqual(client.ls('jrandom/public/foo/bar'), {})
        self.assertEqual(client.ls('jrandom/public/foo/bar/baz'), {})
