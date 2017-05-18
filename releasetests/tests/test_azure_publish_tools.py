from argparse import Namespace
import os
from unittest import TestCase

from azure_publish_tools import (
    DELETE,
    delete_files,
    get_option_parser,
    get_local_files,
    get_local_sync_files,
    get_published_files,
    get_md5content,
    JUJU_DIST,
    LIST,
    list_sync_files,
    PUBLISH,
    publish_files,
    RELEASED,
    SYNC,
    SyncFile,
    sync_files,
    )
from tests import QuietTestCase
from utils import (
    temp_dir,
    write_file,
    )

from azure.storage.blob import (
    BlobBlock,
    ContentSettings,
    Include,
    )


md5sum = {
    'qux': '2FsSE0c8L9fCBFAgprnGKw==',
    'bar': 'N7UdGUp1E+RbVvZSTy1R8g==',
}


class TestOptionParser(TestCase):

    def parse_args(self, args):
        parser = get_option_parser()
        return parser.parse_args(args)

    def test_list(self):
        args = self.parse_args(['list', 'mypurpose'])
        self.assertEqual(Namespace(
            command=LIST, purpose='mypurpose',
            account_key=None, account_name=None), args)

    def test_publish(self):
        args = self.parse_args(['publish', 'mypurpose', 'mypath'])
        self.assertEqual(Namespace(
            command=PUBLISH, purpose='mypurpose', dry_run=False, verbose=False,
            path='mypath', account_key=None, account_name=None), args)

    def test_publish_dry_run(self):
        args = self.parse_args(['publish', 'mypurpose', 'mypath', '--dry-run'])
        self.assertIs(True, args.dry_run)

    def test_publish_verbose(self):
        args = self.parse_args(['publish', 'mypurpose', 'mypath', '--verbose'])
        self.assertIs(True, args.verbose)

    def test_publish_path(self):
        args = self.parse_args(['publish', 'mypurpose', 'mypath2'])
        self.assertEqual('mypath2', args.path)

    def test_delete(self):
        args = self.parse_args(['delete', 'mypurpose', 'mypath'])
        self.assertEqual(Namespace(
            command=DELETE, purpose='mypurpose', dry_run=False, verbose=False,
            path=['mypath'], account_key=None, account_name=None), args)

    def test_delete_dry_run(self):
        args = self.parse_args(['delete', 'mypurpose', 'mypath', '--dry-run'])
        self.assertIs(True, args.dry_run)

    def test_delete_verbose(self):
        args = self.parse_args(['delete', 'mypurpose', 'mypath', '--verbose'])
        self.assertIs(True, args.verbose)

    def test_delete_path(self):
        args = self.parse_args(['delete', 'mypurpose', 'mypath', 'mypath2'])
        self.assertEqual(['mypath', 'mypath2'], args.path)

    def test_sync(self):
        args = self.parse_args(['sync', 'mypath', 'myprefix'])
        self.assertEqual(Namespace(
            command=SYNC, prefix='myprefix', dry_run=False, verbose=False,
            local_dir='mypath', account_key=None, account_name=None), args)

    def test_sync_dry_run(self):
        args = self.parse_args(['sync', 'mypath', 'myprefix', '--dry-run'])
        self.assertIs(True, args.dry_run)

    def test_sync_verbose(self):
        args = self.parse_args(['sync', 'mypath', 'myprefix', '--verbose'])
        self.assertIs(True, args.verbose)

    def test_sync_path(self):
        args = self.parse_args(['sync', 'mypath2', 'myprefix'])
        self.assertEqual('mypath2', args.local_dir)


class FakeBlobProperties:

    def __init__(self, md5, length, content_type):
        self.content_length = length
        self.content_settings = ContentSettings(
            content_type=content_type, content_md5=md5)


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
                   timeout=None, include=None, delimiter=None):
        if marker is not None:
            raise NotImplementedError('marker not implemented.')
        if timeout is not None:
            raise NotImplementedError('timeout not implemented.')
        if include != Include.METADATA:
            raise NotImplementedError('include must be "Include.METADATA".')
        if delimiter is not None:
            raise NotImplementedError('delimiter not implemented.')
        return [b for p, b in self.containers[container_name].items()
                if p.startswith(prefix)]

    def put_block(self, container_name, blob_name, block, block_id):
        if blob_name not in self.containers[container_name]:
            self.containers[container_name][blob_name] = FakeBlob(blob_name)
        self.containers[container_name][blob_name]._blocks[
            BlobBlock(block_id).id] = block

    def put_block_list(self, container_name, blob_name, block_list,
                       content_settings=None):
        pass

    def delete_blob(self, container_name, blob_name):
        del self.containers[container_name][blob_name]


class TestGetPublishedFiles(QuietTestCase):

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


class TestPublishFiles(QuietTestCase):

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
        self.assertEqual(['tools/index.json'],
                         service.containers[JUJU_DIST].keys())
        blob = service.containers[JUJU_DIST]['tools/index.json']
        self.assertEqual({}, blob._blocks)

    def test_one_local_file(self):
        args = Namespace(verbose=False, dry_run=False)
        expected = SyncFile(
            'index.json', 33, 'md5-asdf', 'application/json', '')
        service = FakeBlobService({
            'tools/index.json': FakeBlob.from_sync_file(expected)
            })
        with temp_dir() as local_dir:
            write_file(os.path.join(local_dir, 'index2.json'), '{}\n')
            publish_files(service, RELEASED, local_dir, args)
        self.assertEqual(['tools/index.json', 'tools/index2.json'],
                         service.containers[JUJU_DIST].keys())
        blob = service.containers[JUJU_DIST]['tools/index2.json']
        self.assertEqual({BlobBlock('MA==').id: '{}\n'}, blob._blocks)

    def test_same_local_remote(self):
        args = Namespace(verbose=False, dry_run=False)
        with temp_dir() as local_dir:
            file_path = os.path.join(local_dir, 'index2.json')
            write_file(file_path, '{}\n')
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

    def test_different_local_remote(self):
        args = Namespace(verbose=False, dry_run=False)
        expected = SyncFile(
            'tools/index2.json', 33, 'md5-asdf', 'application/json', '')
        service = FakeBlobService({
            'tools/index2.json': FakeBlob.from_sync_file(expected)
            })
        with temp_dir() as local_dir:
            write_file(os.path.join(local_dir, 'index2.json'), '{}\n')
            publish_files(service, RELEASED, local_dir, args)
        self.assertEqual(['tools/index2.json'],
                         service.containers[JUJU_DIST].keys())
        blob = service.containers[JUJU_DIST]['tools/index2.json']
        self.assertEqual({BlobBlock('MA==').id: '{}\n'}, blob._blocks)

    def test_different_local_remote_dry_run(self):
        args = Namespace(verbose=False, dry_run=True)
        expected = SyncFile(
            'tools/index2.json', 33, 'md5-asdf', 'application/json', '')
        service = FakeBlobService({
            'tools/index2.json': FakeBlob.from_sync_file(expected)
            })
        with temp_dir() as local_dir:
            write_file(os.path.join(local_dir, 'index2.json'), '{}\n')
            publish_files(service, RELEASED, local_dir, args)
        self.assertEqual(['tools/index2.json'],
                         service.containers[JUJU_DIST].keys())
        blob = service.containers[JUJU_DIST]['tools/index2.json']
        self.assertEqual({}, blob._blocks)


class TestSyncFiles(QuietTestCase):

    def test_no_files(self):
        args = Namespace(verbose=False, dry_run=False)
        with temp_dir() as local_dir:
            sync_files(FakeBlobService(), 'tools', local_dir, args)

    def test_one_remote_file(self):
        args = Namespace(verbose=False, dry_run=False)
        expected = SyncFile(
            'index.json', 33, 'md5-asdf', 'application/json', '')
        service = FakeBlobService({
            'tools/index.json': FakeBlob.from_sync_file(expected)
            })
        with temp_dir() as local_dir:
            sync_files(service, 'tools', local_dir, args)
        self.assertEqual(['tools/index.json'],
                         service.containers[JUJU_DIST].keys())
        blob = service.containers[JUJU_DIST]['tools/index.json']
        self.assertEqual({}, blob._blocks)

    def test_one_local_file(self):
        args = Namespace(verbose=False, dry_run=False)
        expected = SyncFile(
            'index.json', 33, 'md5-asdf', 'application/json', '')
        service = FakeBlobService({
            'tools/index.json': FakeBlob.from_sync_file(expected)
            })
        with temp_dir() as local_dir:
            write_file(os.path.join(local_dir, 'index2.json'), '{}\n')
            sync_files(service, 'tools', local_dir, args)
        self.assertEqual(['tools/index.json', 'tools/index2.json'],
                         service.containers[JUJU_DIST].keys())
        blob = service.containers[JUJU_DIST]['tools/index2.json']
        self.assertEqual({BlobBlock('MA==').id: '{}\n'}, blob._blocks)

    def test_different_local_remote(self):
        args = Namespace(verbose=False, dry_run=False)
        expected = SyncFile(
            'tools/index2.json', 33, 'md5-asdf', 'application/json', '')
        service = FakeBlobService({
            'tools/index2.json': FakeBlob.from_sync_file(expected)
            })
        with temp_dir() as local_dir:
            write_file(os.path.join(local_dir, 'index2.json'), '{}\n')
            sync_files(service, 'tools', local_dir, args)
        self.assertEqual(['tools/index2.json'],
                         service.containers[JUJU_DIST].keys())
        blob = service.containers[JUJU_DIST]['tools/index2.json']
        self.assertEqual({BlobBlock('MA==').id: '{}\n'}, blob._blocks)

    def test_different_local_remote_dry_run(self):
        args = Namespace(verbose=False, dry_run=True)
        expected = SyncFile(
            'tools/index2.json', 33, 'md5-asdf', 'application/json', '')
        service = FakeBlobService({
            'tools/index2.json': FakeBlob.from_sync_file(expected)
            })
        with temp_dir() as local_dir:
            write_file(os.path.join(local_dir, 'index2.json'), '{}\n')
            sync_files(service, 'tools', local_dir, args)
        self.assertEqual(['tools/index2.json'],
                         service.containers[JUJU_DIST].keys())
        blob = service.containers[JUJU_DIST]['tools/index2.json']
        self.assertEqual({}, blob._blocks)

    def test_same_local_remote(self):
        args = Namespace(verbose=False, dry_run=False)
        with temp_dir() as local_dir:
            file_path = os.path.join(local_dir, 'index2.json')
            write_file(file_path, '{}\n')
            md5_sum = get_md5content(file_path)
            expected = SyncFile(
                'tools/index2.json', 33, md5_sum, 'application/json', '')
            service = FakeBlobService({
                'tools/index2.json': FakeBlob.from_sync_file(expected)
                })
            sync_files(service, 'tools', local_dir, args)
        self.assertEqual(['tools/index2.json'],
                         service.containers[JUJU_DIST].keys())
        blob = service.containers[JUJU_DIST]['tools/index2.json']
        self.assertEqual({}, blob._blocks)


class TestGetLocalFiles(QuietTestCase):

    def test_empty(self):
        with temp_dir() as local_dir:
            pass
        self.assertIs(None, get_local_files(RELEASED, local_dir))
        self.assertRegexpMatches(self.stdout.getvalue(), '.* not found.\n')

    def test_two_files(self):
        with temp_dir() as local_dir:
            foo_path = os.path.join(local_dir, 'foo')
            baz_path = os.path.join(local_dir, 'baz')
            expected = [
                SyncFile('tools/baz', size=3, local_path=baz_path,
                         md5content=md5sum['qux'], mimetype=None),
                SyncFile('tools/foo', size=3, local_path=foo_path,
                         md5content=md5sum['bar'], mimetype=None),
                ]
            write_file(foo_path, 'bar')
            write_file(baz_path, 'qux')
            result = get_local_files(RELEASED, local_dir)
            self.assertEqual(expected, result)

    def test_skips_mirror(self):
        with temp_dir() as local_dir:
            foo_path = os.path.join(local_dir, 'foo-mirror')
            write_file(foo_path, 'bar')
            result = get_local_files(RELEASED, local_dir)
            self.assertEqual([], result)

    def test_skips_symlinks(self):
        with temp_dir() as local_dir:
            foo_path = os.path.join(local_dir, 'foo')
            os.symlink('foo', foo_path)
            result = get_local_files(RELEASED, local_dir)
            self.assertEqual([], result)


class TestGetLocalSyncFiles(TestCase):

    def test_empty(self):
        with temp_dir() as local_dir:
            pass
        self.assertEqual([], get_local_sync_files('bools', local_dir))

    def test_two_files(self):
        with temp_dir() as local_dir:
            foo_path = os.path.join(local_dir, 'foo')
            baz_path = os.path.join(local_dir, 'baz')
            expected = [
                SyncFile('bools/baz', size=3, local_path=baz_path,
                         md5content=md5sum['qux'], mimetype=None),
                SyncFile('bools/foo', size=3, local_path=foo_path,
                         md5content=md5sum['bar'], mimetype=None),
                ]
            write_file(foo_path, 'bar')
            write_file(baz_path, 'qux')
            result = get_local_sync_files('bools', local_dir)
            self.assertEqual(expected, result)

    def test_includes_mirror(self):
        with temp_dir() as local_dir:
            foo_path = os.path.join(local_dir, 'foo-mirror')
            expected = [
                SyncFile('bools/foo-mirror', size=3, local_path=foo_path,
                         md5content=md5sum['bar'], mimetype=None),
                ]
            write_file(foo_path, 'bar')
            result = get_local_sync_files('bools', local_dir)
            self.assertEqual(expected, result)

    def test_skips_symlinks(self):
        with temp_dir() as local_dir:
            foo_path = os.path.join(local_dir, 'foo')
            os.symlink('foo', foo_path)
            result = get_local_sync_files('bools', local_dir)
            self.assertEqual([], result)


class TestDeleteFiles(TestCase):

    def test_delete_files(self):
        args = Namespace(verbose=False, dry_run=False)
        file1 = SyncFile(
            'index.json', 33, 'md5-asdf', 'application/json', '')
        file2 = SyncFile(
            'other.json', 33, 'md5-asdf', 'application/json', '')
        blob_service = FakeBlobService({
            'tools/index.json': FakeBlob.from_sync_file(file1),
            'tools/other.json': FakeBlob.from_sync_file(file2)
            })
        delete_files(blob_service, 'released', ['index.json'], args)
        self.assertIsNone(
            blob_service.containers[JUJU_DIST].get('tools/index.json'))
        self.assertEqual(
            file2.path,
            blob_service.containers[JUJU_DIST]['tools/other.json'].name)

    def test_delete_files_dry_run(self):
        args = Namespace(verbose=False, dry_run=True)
        file1 = SyncFile(
            'index.json', 33, 'md5-asdf', 'application/json', '')
        blob_service = FakeBlobService({
            'tools/index.json': FakeBlob.from_sync_file(file1),
            })
        delete_files(blob_service, 'released', ['index.json'], args)
        self.assertEqual(
            file1.path,
            blob_service.containers[JUJU_DIST]['tools/index.json'].name)
