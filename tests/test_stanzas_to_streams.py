from tempfile import NamedTemporaryFile
from unittest import TestCase

from generate_simplestreams import (
    Item,
    json_dump,
    )
from stanzas_to_streams import (
    JujuFileNamer,
    read_items_file,
    )


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
            json_dump([item_dict], items_file.name)
            items = list(read_items_file(items_file.name))
        self.assertEqual([Item(data={'size': 27}, **pedigree)], items)
