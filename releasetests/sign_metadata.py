from __future__ import print_function

from argparse import ArgumentParser
from os import listdir
from os.path import (
    basename,
    join,
    isdir,
    isfile,
)
from tempfile import NamedTemporaryFile

from lprelease import run


def sign_metadata(signing_key, meta_dir, signing_passphrase_file=None):
    key_option, gpg_options = get_gpg_options(
        signing_key, signing_passphrase_file)
    for meta_file in get_meta_files(meta_dir):
        with NamedTemporaryFile() as temp_file:
            meta_file_path = join(meta_dir, meta_file)
            update_file_content(meta_file_path, temp_file.name)
            meta_basename = basename(meta_file).replace('.json', '.sjson')
            output_file = join(meta_dir, meta_basename)
            ensure_no_file(output_file)
            cmd = 'gpg {} --clearsign {} -o {} {}'.format(
                gpg_options, key_option, output_file, temp_file.name)
            run(cmd.split())
            output_file = join(meta_dir, '{}.gpg'.format(meta_file))
            ensure_no_file(output_file)
            cmd = 'gpg {} --detach-sign {} -o {} {}'.format(
                gpg_options, key_option, output_file, meta_file_path)
            run(cmd.split())


def update_file_content(mete_file, dst_file):
    with open(mete_file) as m:
        with open(dst_file, 'w') as d:
            d.write(m.read().replace('.json', '.sjson'))


def get_gpg_options(signing_key, signing_passphrase_file=None):
    key_option = "--default-key {}".format(signing_key)
    gpg_options = "--no-tty"
    if signing_passphrase_file:
        gpg_options = "--no-use-agent --no-tty --passphrase-file {}".format(
            signing_passphrase_file)
    return key_option, gpg_options


def get_meta_files(meta_dir):
    meta_files = [f for f in listdir(meta_dir) if f.endswith('.json')]
    if not meta_files:
        print('Warning! no meta files found in {}'.format(meta_dir))
    return meta_files


def ensure_no_file(file_path):
    if isfile(file_path):
        raise ValueError('FAIL! file already exists: {}'.format(file_path))


def parse_args(argv=None):
    parser = ArgumentParser("Sign streams' meta files.")
    parser.add_argument('metadata_dir', help='Metadata directory.')
    parser.add_argument('signing_key', help='Key to sign with.')
    parser.add_argument(
        '-p', '--signing-passphrase-file', help='Signing passphrase file path.')
    args = parser.parse_args(argv)
    if not isdir(args.metadata_dir):
        parser.error(
            'Invalid metadata directory path {}'.format(args.metadata_dir))
    if (args.signing_passphrase_file and
            not isfile(args.signing_passphrase_file)):
        parser.error(
            'Invalid passphrase file path {}'.format(
                args.signing_passphrase_file))
    return args


if __name__ == '__main__':
    args = parse_args()
    sign_metadata(args.signing_key, args.metadata_dir,
                  args.signing_passphrase_file)
