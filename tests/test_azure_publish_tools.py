from argparse import Namespace
from unittest import TestCase

from azure_publish_tools import (
    DELETE,
    get_option_parser,
    get_published_files,
    LIST,
    PUBLISH,
    RELEASED,
    SyncFile,
    )

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

    @classmethod
    def from_sync_file(cls, sync_file):
        return FakeBlob(sync_file.path, sync_file.md5content,
                        sync_file.size, sync_file.mimetype)


class FakeBlobService:

    def __init__(self, blobs=None):
        if blobs is None:
            blobs = {}
        self.blobs = blobs

    def list_blobs(self, container_name, prefix=None, marker=None,
                   maxresults=None, include=None, delimiter=None):
        return [b for p,b in self.blobs.items() if p.startswith(prefix)]


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
