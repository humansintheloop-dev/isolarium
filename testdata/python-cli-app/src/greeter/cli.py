import click


@click.command()
@click.argument("name", default="World")
def main(name):
    click.echo(f"Hello, {name}!")
