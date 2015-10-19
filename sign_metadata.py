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


def sign_metadata(signing_key, dest_dist, purpose, juju_dist=None,
                  signing_passphrase_file=None):
    key_option, gpg_options = get_gpg_options(
        signing_key, signing_passphrase_file)
    dists = [dest_dist]
    if purpose in ('released', 'weekly', 'testing'):
        dists.append(juju_dist)
    for dist in dists:
        meta_dir, meta_files = get_meta_files(dist)
        for meta_file in meta_files:
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
                    gpg_options, key_option, output_file, temp_file.name)
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


def get_meta_files(dist):
    meta_dir = join(dist, 'tools', 'streams', 'v1')
    meta_files = listdir(meta_dir)
    return meta_dir, filter(lambda x: x.endswith(".json"),  meta_files)


def ensure_no_file(file_path):
    if isfile(file_path):
        raise ValueError('FAIL! file already exists: {}'.format(file_path))


def parse_args(argv=None):
    parser = ArgumentParser("Sign streams' meta files.")
    parser.add_argument(
        'purpose', help='Purpose: devel, proposed, released,  testing, weekly',
        choices=['devel', 'proposed', 'released',  'testing', 'weekly'])
    parser.add_argument('stream_dir', help='Stream directory.')
    parser.add_argument('signing_key', help='Name to sign with.')
    parser.add_argument(
        '-p', '--signing-passphrase-file', help='Signing passphrase file path.')
    args = parser.parse_args(argv)
    if not isdir(args.stream_dir):
        parser.error(
            'Invalid stream directory path {}'.format(args.stream_dir))
    if (args.signing_passphrase_file and
            not isfile(args.signing_passphrase_file)):
        parser.error(
            'Invalid passphrase file path {}'.format(
                args.signing_passphrase_file))
    return args


def main(args):
    juju_dist = join(args.stream_dir, 'juju-dist')
    dest_dist = join(args.stream_dir, 'juju-dist', args.purpose)
    if args.purpose == 'released':
        dest_dist = join(args.stream_dir, 'juju-dist')
    sign_metadata(args.signing_key, dest_dist, args.purpose, juju_dist,
                  args.signing_passphrase_file)


if __name__ == '__main__':
    args = parse_args()
    main(args)
