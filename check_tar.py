#!/usr/bin/env python3
from argparse import ArgumentParser
import logging
import os
import tarfile
from textwrap import dedent
import re
import sys


class TarfileNotFound(Exception):
    """Raised when specified tarfile cannot be found."""


class TestedDirNotFound(Exception):
    """Raised when specified tested text dir cannot be found."""


def get_fpc_text(juju_tar):
    """Return the fallback-public-cloud.yaml text from a tarball.

    Raises an exception if the tarfile contains more or fewer than one
    falllback-public-cloud.yaml.
    """
    fpc_members = [
        m for m in juju_tar.getmembers()
        if os.path.basename(m.name) == 'fallback-public-cloud.yaml']
    if len(fpc_members) == 1:
        return juju_tar.extractfile(fpc_members[0]).read()
    else:
        if len(fpc_members) == 0:
            raise Exception('Tarfile has no fallback-public-cloud.')
        else:
            raise Exception(
                'Tarfile {:d} copies of fallback-public-cloud.'.format(
                    len(fpc_members)))


def check_tar(tested_texts_dir, tar_filename):
    """Check the contents of the tarfile.

    tested_texts_dir is the name of a directory with the texted
    fallback-public-cloud texts.

    tar_filename is the filename of the tarfile.
    """
    base_tar_name = os.path.basename(tar_filename)
    if re.match(r'juju-core_1\..*\.tar.gz', base_tar_name) is not None:
        logging.info(
            'Juju 1 does not use fallback-public-cloud.yaml.  Skipping.')
        return 0
    try:
        tf = tarfile.open(tar_filename, 'r:*')
    except FileNotFoundError:
        raise TarfileNotFound('Tarfile not found: "{}"'.format(tar_filename))
    with tf:
        fpc_text = get_fpc_text(tf)
    try:
        tested_list = os.listdir(tested_texts_dir)
    except FileNotFoundError:
        raise TestedDirNotFound(
            'Tested dir not found: "{}"'.format(tested_texts_dir))

    for tested in tested_list:
        if tested.startswith('.'):
            continue
        with open(os.path.join(tested_texts_dir, tested), 'rb') as tested_file:
            if tested_file.read() == fpc_text:
                logging.info('fallback-public-cloud.yaml matched {}.'.format(
                    tested))
                return 0
    else:
        print(
            'fallback-public-cloud.yaml does not match a tested version.\n'
            'Please have the QA team test it before landing.',
            file=sys.stderr)
        return 1


def main():
    logging.basicConfig(level=logging.INFO)
    parser = ArgumentParser(description=dedent("""\
        Ensure fallback-public-cloud.yaml has been tested.
        """))
    parser.add_argument('tested_texts_dir', help=(
        'The directory containing previously-tested versions of'
        ' fallback-public-cloud.'))
    parser.add_argument('tarfile', help='The tarfile to check.')
    args = parser.parse_args()
    try:
        return check_tar(args.tested_texts_dir, args.tarfile)
    except (TarfileNotFound, TestedDirNotFound) as e:
        print(e, file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
