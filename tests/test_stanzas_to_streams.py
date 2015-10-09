import json
import os
from StringIO import StringIO
from tempfile import NamedTemporaryFile
from unittest import TestCase

from mock import patch

from generate_simplestreams import (
    FileNamer,
    Item,
    json_dump,
    )
from stanzas_to_streams import (
    JujuFileNamer,
    read_items_file,
    write_release_index,
    )
from utils import temp_dir


class TestJujuFileNamer(TestCase):

    def test_get_index_path(self):
        self.assertEqual('streams/v1/index2.json',
                         JujuFileNamer.get_index_path())

    def test_get_content_path(self):
        self.assertEqual('streams/v1/foo-bar-baz.json',
                         JujuFileNamer.get_content_path('foo:bar-baz'))


class TestReadItemsFile(TestCase):

    def test_read_items_file(self):
        pedigree = {
            'content_id': 'cid', 'product_name': 'pname',
            'version_name': 'vname', 'item_name': 'iname',
            }
        with NamedTemporaryFile() as items_file:
            item_dict = {'size': '27'}
            item_dict.update(pedigree)
            with patch('sys.stderr', StringIO()):
                json_dump([item_dict], items_file.name)
            items = list(read_items_file(items_file.name))
        self.assertEqual([Item(data={'size': 27}, **pedigree)], items)


class TestWriteReleaseIndex(TestCase):

    def write_full_index(self, out_d, content):
        os.mkdir(os.path.join(out_d, 'streams'))
        os.mkdir(os.path.join(out_d, 'streams/v1'))
        path = os.path.join(out_d, JujuFileNamer.get_index_path())
        with patch('sys.stderr', StringIO()):
            json_dump(content, path)

    def read_release_index(self, out_d):
        path = os.path.join(out_d, FileNamer.get_index_path())
        with open(path) as release_index_file:
            return json.load(release_index_file)

    def test_empty_index(self):
        with temp_dir() as out_d:
            self.write_full_index(out_d, {'index': {}, 'foo': 'bar'})
            with patch('sys.stderr', StringIO()):
                write_release_index(out_d)
            release_index = self.read_release_index(out_d)
        self.assertEqual({'foo': 'bar', 'index': {}}, release_index)

    def test_release_index(self):
        with temp_dir() as out_d:
            self.write_full_index(out_d, {
                'index': {'com.ubuntu.juju:released:tools': 'foo'},
                'foo': 'bar'})
            with patch('sys.stderr', StringIO()):
                write_release_index(out_d)
            release_index = self.read_release_index(out_d)
        self.assertEqual({'foo': 'bar', 'index':
            {'com.ubuntu.juju:released:tools': 'foo'}}, release_index)

    def test_multi_index(self):
        with temp_dir() as out_d:
            self.write_full_index(out_d, {
                'index': {
                    'com.ubuntu.juju:proposed:tools': 'foo',
                    'com.ubuntu.juju:released:tools': 'foo',
                    },
                'foo': 'bar'})
            with patch('sys.stderr', StringIO()):
                write_release_index(out_d)
            release_index = self.read_release_index(out_d)
        self.assertEqual({'foo': 'bar', 'index':
            {'com.ubuntu.juju:released:tools': 'foo'}}, release_index)
