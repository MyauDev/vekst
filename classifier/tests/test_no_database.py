"""Task 5.6: ARCHITECTURE.md A-4, made machine-checkable.

The classifier never receives database credentials, in either form. The
strongest version of that rule is that it cannot: no driver is installed, so
there is nothing to hand a connection string to.

This is checked against the *resolved* dependency set, not against
pyproject.toml, so a driver pulled in transitively is caught too.
"""

import importlib.metadata
import tomllib
from pathlib import Path

# Anything that could open a connection to Postgres, or an ORM that would.
FORBIDDEN = {
    "psycopg",
    "psycopg2",
    "psycopg2-binary",
    "psycopg-binary",
    "asyncpg",
    "pg8000",
    "sqlalchemy",
    "alembic",
    "databases",
    "peewee",
    "tortoise-orm",
    "django",
    "pony",
    "aiopg",
}


def test_no_database_package_is_installed() -> None:
    installed = {
        (dist.metadata["Name"] or "").lower()
        for dist in importlib.metadata.distributions()
    }

    found = installed & FORBIDDEN

    assert not found, (
        f"the classifier has a database package installed: {sorted(found)}.\n"
        "ARCHITECTURE.md A-4: the classifier is a pure function over its inputs and "
        "never holds database credentials. Tenant isolation must stay "
        "single-mechanism -- one process holds the connection, and RLS is the only "
        "thing between two customers' data. If you need data here, pass it in the "
        "request. Move the logic, not the credentials."
    )


def test_no_database_package_is_declared() -> None:
    """The same rule at the source, so the failure names the line to delete."""
    path = Path(__file__).parent.parent / "pyproject.toml"
    pyproject = tomllib.loads(path.read_text())

    declared = list(pyproject["project"]["dependencies"])
    for group in pyproject.get("dependency-groups", {}).values():
        declared.extend(group)

    names = {
        d.split(">")[0].split("=")[0].split("[")[0].split("<")[0].strip().lower()
        for d in declared
    }

    found = names & FORBIDDEN
    assert not found, f"declared in pyproject.toml: {sorted(found)}"
