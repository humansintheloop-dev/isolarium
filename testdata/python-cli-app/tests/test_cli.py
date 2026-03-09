from click.testing import CliRunner

from greeter.cli import main


def test_default_greeting():
    result = CliRunner().invoke(main)
    assert result.exit_code == 0
    assert "Hello, World!" in result.output


def test_named_greeting():
    result = CliRunner().invoke(main, ["Alice"])
    assert result.exit_code == 0
    assert "Hello, Alice!" in result.output
