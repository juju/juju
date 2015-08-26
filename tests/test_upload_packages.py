import os
from unittest import TestCase

from upload_packages import (
    get_changes,
)
from utils import temp_dir


CHANGES_DATA = """\
-----BEGIN PGP SIGNED MESSAGE-----
Hash: SHA1

Format: 1.8
Date: Mon, 10 Aug 2015 20:16:09 +0000
Source: juju-core
Binary: juju-core juju juju-local juju-local-kvm
Architecture: source
Version: 1.24.5-0ubuntu1~14.04.1~juju1
Distribution: trusty
"""


class UploadPackageTestCase(TestCase):

    def test_get_changes(self):
        with temp_dir() as package_dir:
            changes_path = os.path.join(package_dir, 'foo_source.changes')
            with open(changes_path, 'w') as changes_file:
                changes_file.write(CHANGES_DATA)
            with open(os.path.join(package_dir, 'foo.dsc'), 'w') as other_file:
                other_file.write('other_file')
            source_name, version, file_name = get_changes(package_dir)
        self.assertEqual('juju-core', source_name)
        self.assertEqual('1.24.5-0ubuntu1~14.04.1~juju1', version)
        self.assertEqual('foo_source.changes', file_name)
