from argparse import Namespace
import hashlib
import os
from unittest import TestCase

from azure_publish_tools import (
    DELETE,
    get_option_parser,
    get_published_files,
    get_md5content,
    JUJU_DIST,
    LIST,
    list_sync_files,
    PUBLISH,
    publish_files,
    RELEASED,
    SyncFile,
    )
from generate_simplestreams import json_dump
from utils import temp_dir

class TestOptionParser(TestCase):

    def parse_args(self, args):
        parser = get_option_parser()
        return parser.parse_args(args)

    def test_list(self):
        args = self.parse_args(['list', 'mypurpose'])
        self.assertEqual(Namespace(
            command=LIST, purpose='mypurpose'), args)

    def test_publish(self):
        args = self.parse_args(['publish', 'mypurpose', 'mypath'])
        self.assertEqual(Namespace(
            command=PUBLISH, purpose='mypurpose', dry_run=False, verbose=False,
            path=['mypath']), args)

    def test_publish_dry_run(self):
        args = self.parse_args(['publish', 'mypurpose', 'mypath', '--dry-run'])
        self.assertIs(True, args.dry_run)

    def test_publish_verbose(self):
        args = self.parse_args(['publish', 'mypurpose', 'mypath', '--verbose'])
        self.assertIs(True, args.verbose)

    def test_publish_path(self):
        args = self.parse_args(['publish', 'mypurpose', 'mypath', 'mypath2'])
        self.assertEqual(['mypath', 'mypath2'], args.path)

    def test_delete(self):
        args = self.parse_args(['delete', 'mypurpose', 'mypath'])
        self.assertEqual(Namespace(
            command=DELETE, purpose='mypurpose', dry_run=False, verbose=False,
            path=['mypath']), args)

    def test_delete_dry_run(self):
        args = self.parse_args(['delete', 'mypurpose', 'mypath', '--dry-run'])
        self.assertIs(True, args.dry_run)

    def test_delete_verbose(self):
        args = self.parse_args(['delete', 'mypurpose', 'mypath', '--verbose'])
        self.assertIs(True, args.verbose)

    def test_delete_path(self):
        args = self.parse_args(['delete', 'mypurpose', 'mypath', 'mypath2'])
        self.assertEqual(['mypath', 'mypath2'], args.path)


class FakeBlobProperties:

    def __init__(self, md5, length, content_type):
        self.content_md5 = md5
        self.content_length = length
        self.content_type = content_type


class FakeBlob:

    def __init__(self, name='', md5=None, length=None, content_type=None):
        self.name = name
        self.snapshot = ''
        self.url = ''
        self.properties = FakeBlobProperties(md5, length, content_type)
        self.metadata = {}
        self._blocks = {}

    @classmethod
    def from_sync_file(cls, sync_file):
        return FakeBlob(sync_file.path, sync_file.md5content,
                        sync_file.size, sync_file.mimetype)


class FakeBlobService:

    def __init__(self, blobs=None):
        if blobs is None:
            blobs = {}
        self.containers = {JUJU_DIST: blobs}

    def list_blobs(self, container_name, prefix=None, marker=None,
                   maxresults=None, include=None, delimiter=None):
        if marker is not None:
            raise NotImplementedError('marker not implemented.')
        if maxresults is not None:
            raise NotImplementedError('maxresults not implemented.')
        if include != 'metadata':
            raise NotImplementedError('include must be "metadata".')
        if delimiter is not None:
            raise NotImplementedError('delimiter not implemented.')
        return [b for p, b in self.containers[container_name].items()
                if p.startswith(prefix)]

    def put_blob(self, container_name, blob_name, blob, x_ms_blob_type):
        if x_ms_blob_type != 'BlockBlob':
            raise NotImplementedError('x_ms_blob_type not implemented.')
        if blob != '':
            raise NotImplementedError('blob not implemented.')
        self.containers[container_name][blob_name] = FakeBlob(blob_name)

    def put_block(self, container_name, blob_name, block, block_id):
        self.containers[container_name][blob_name]._blocks[block_id] = block

    def put_block_list(self, container_name, blob_name, block_list,
                       content_md5=None, x_ms_blob_content_type=None,
                       x_ms_blob_content_encoding=None,
                       x_ms_blob_content_language=None,
                       x_ms_blob_content_md5=None):
        pass


class TestGetPublishedFiles(TestCase):

    def test_none(self):
        self.assertEqual([], get_published_files(RELEASED, FakeBlobService()))

    def test_prefix_wrong(self):
        expected = SyncFile(
            'index.json', 33, 'md5-asdf', 'application/json', '')
        service = FakeBlobService({
            'fools/index.json': FakeBlob.from_sync_file(expected)
            })
        self.assertEqual([], get_published_files(RELEASED, service))

    def test_prefix_right(self):
        expected = SyncFile(
            'index.json', 33, 'md5-asdf', 'application/json', '')
        service = FakeBlobService({
            'tools/index.json': FakeBlob.from_sync_file(expected)
            })
        self.assertEqual([expected], get_published_files(RELEASED, service))


class TestListSyncFiles(TestCase):

    def test_none(self):
        self.assertEqual([], list_sync_files('tools', FakeBlobService()))

    def test_prefix_wrong(self):
        expected = SyncFile(
            'index.json', 33, 'md5-asdf', 'application/json', '')
        service = FakeBlobService({
            'fools/index.json': FakeBlob.from_sync_file(expected)
            })
        self.assertEqual([], list_sync_files('tools', service))

    def test_prefix_right(self):
        expected = SyncFile(
            'index.json', 33, 'md5-asdf', 'application/json', '')
        service = FakeBlobService({
            'tools/index.json': FakeBlob.from_sync_file(expected)
            })
        self.assertEqual([expected], list_sync_files('tools', service))


class TestPublishFiles(TestCase):

    def test_no_files(self):
        args = Namespace(verbose=False, dry_run=False)
        with temp_dir() as local_dir:
            publish_files(FakeBlobService(), RELEASED, local_dir, args)

    def test_one_remote_file(self):
        args = Namespace(verbose=False, dry_run=False)
        expected = SyncFile(
            'index.json', 33, 'md5-asdf', 'application/json', '')
        service = FakeBlobService({
            'tools/index.json': FakeBlob.from_sync_file(expected)
            })
        with temp_dir() as local_dir:
            publish_files(service, RELEASED, local_dir, args)

    def test_one_local_file(self):
        args = Namespace(verbose=False, dry_run=False)
        expected = SyncFile(
            'index.json', 33, 'md5-asdf', 'application/json', '')
        service = FakeBlobService({
            'tools/index.json': FakeBlob.from_sync_file(expected)
            })
        with temp_dir() as local_dir:
            json_dump({}, os.path.join(local_dir, 'index2.json'))
            publish_files(service, RELEASED, local_dir, args)
        self.assertEqual(['tools/index.json', 'tools/index2.json'],
            service.containers[JUJU_DIST].keys())
        blob = service.containers[JUJU_DIST]['tools/index2.json']
        self.assertEqual({'MA==': '{}\n'}, blob._blocks)

    def test_different_local_remote(self):
        args = Namespace(verbose=False, dry_run=False)
        expected = SyncFile(
            'tools/index2.json', 33, 'md5-asdf', 'application/json', '')
        service = FakeBlobService({
            'tools/index2.json': FakeBlob.from_sync_file(expected)
            })
        with temp_dir() as local_dir:
            json_dump({}, os.path.join(local_dir, 'index2.json'))
            publish_files(service, RELEASED, local_dir, args)
        self.assertEqual(['tools/index2.json'],
            service.containers[JUJU_DIST].keys())
        blob = service.containers[JUJU_DIST]['tools/index2.json']
        self.assertEqual({'MA==': '{}\n'}, blob._blocks)

    def test_same_local_remote(self):
        args = Namespace(verbose=False, dry_run=False)
        with temp_dir() as local_dir:
            file_path = os.path.join(local_dir, 'index2.json')
            json_dump({}, file_path)
            md5_sum = get_md5content(file_path)
            expected = SyncFile(
                'tools/index2.json', 33, md5_sum, 'application/json', '')
            service = FakeBlobService({
                'tools/index2.json': FakeBlob.from_sync_file(expected)
                })
            publish_files(service, RELEASED, local_dir, args)
        self.assertEqual(['tools/index2.json'],
            service.containers[JUJU_DIST].keys())
        blob = service.containers[JUJU_DIST]['tools/index2.json']
        self.assertEqual({}, blob._blocks)
