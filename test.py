import click

@click.command()
@click.option('--message', '-m', multiple=True, default=['dns', 'storage', 'dashboard', 'ingress', 'metallb:10.64.140.43-10.64.140.49'])
def commit(message):
    for i in message:
        if not i:
            click.echo('empty')
    click.echo(f'""{message}""')

commit()
