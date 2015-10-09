import json
import os
from StringIO import StringIO
from tempfile import NamedTemporaryFile
from unittest import TestCase

from mock import patch

from generate_simplestreams import (
    FileNamer,
    generate_index,
    Item,
    items2content_trees,
    json_dump as json_dump_verbose,
    )
from stanzas_to_streams import (
    dict_to_item,
    filenames_to_streams,
    JujuFileNamer,
    read_items_file,
    write_release_index,
    )
from test_generate_simplestreams import load_stream_dir
from utils import temp_dir


class TestJujuFileNamer(TestCase):

    def test_get_index_path(self):
        self.assertEqual('streams/v1/index2.json',
                         JujuFileNamer.get_index_path())

    def test_get_content_path(self):
        self.assertEqual('streams/v1/foo-bar-baz.json',
                         JujuFileNamer.get_content_path('foo:bar-baz'))


def json_dump(json, filename):
    with patch('sys.stderr', StringIO()):
        json_dump_verbose(json, filename)


class TestDictToItem(TestCase):

    def test_dict_to_item(self):
        pedigree = {
            'content_id': 'cid', 'product_name': 'pname',
            'version_name': 'vname', 'item_name': 'iname',
            }
        item_dict = {'size': '27'}
        item_dict.update(pedigree)
        item = dict_to_item(item_dict)
        self.assertEqual(Item(data={'size': 27}, **pedigree), item)


class TestReadItemsFile(TestCase):

    def test_read_items_file(self):
        pedigree = {
            'content_id': 'cid', 'product_name': 'pname',
            'version_name': 'vname', 'item_name': 'iname',
            }
        with NamedTemporaryFile() as items_file:
            item_dict = {'size': '27'}
            item_dict.update(pedigree)
            json_dump([item_dict], items_file.name)
            items = list(read_items_file(items_file.name))
        self.assertEqual([Item(data={'size': 27}, **pedigree)], items)


class TestWriteReleaseIndex(TestCase):

    def write_full_index(self, out_d, content):
        os.makedirs(os.path.join(out_d, 'streams/v1'))
        path = os.path.join(out_d, JujuFileNamer.get_index_path())
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
        self.assertEqual({'foo': 'bar', 'index': {
            'com.ubuntu.juju:released:tools': 'foo'}
            }, release_index)

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
        self.assertEqual({'foo': 'bar', 'index': {
            'com.ubuntu.juju:released:tools': 'foo'}
            }, release_index)


class TestFilenamesToStreams(TestCase):

    def test_filenames_to_streams(self):
        item = {
            'content_id': 'foo:1',
            'product_name': 'bar',
            'version_name': 'baz',
            'item_name': 'qux',
            'size': '27',
            }
        item2 = dict(item)
        item2.update({
            'size': '42',
            'item_name': 'quxx'})
        updated = 'updated'
        file_a = NamedTemporaryFile()
        file_b = NamedTemporaryFile()
        with temp_dir() as out_d, file_a, file_b:
            json_dump([item], file_a.name)
            json_dump([item2], file_b.name)
            stream_dir = os.path.join(out_d, 'streams/v1')
            with patch('sys.stderr', StringIO()):
                filenames_to_streams([file_a.name, file_b.name], updated,
                                     out_d)
            content = load_stream_dir(stream_dir)
        self.assertItemsEqual(content.keys(), ['index.json', 'index2.json',
                                               'foo-1.json'])
        items = [dict_to_item(item), dict_to_item(item2)]
        trees = items2content_trees(items, {
            'updated': updated, 'datatype': 'content-download'})
        expected = generate_index(trees, 'updated', JujuFileNamer)
        self.assertEqual(expected, content['index2.json'])
        index_expected = generate_index({}, 'updated', FileNamer)
        self.assertEqual(index_expected, content['index.json'])
        self.assertEqual(trees['foo:1'], content['foo-1.json'])
