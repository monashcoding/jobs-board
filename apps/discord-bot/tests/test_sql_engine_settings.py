"""How DATABASE_URL becomes an engine URL and its TLS setting."""

from __future__ import annotations

import ssl

from src.backend.sql.client import engine_settings


def test_managed_host_is_reached_over_verified_tls() -> None:
    url, connect_args = engine_settings(
        "postgresql://bot:pw@ep-x.ap-southeast-2.aws.neon.tech/jobs_bot"
        "?sslmode=require&channel_binding=require"
    )

    assert url.drivername == "postgresql+asyncpg"
    assert dict(url.query) == {}
    context = connect_args["ssl"]
    assert context.verify_mode == ssl.CERT_REQUIRED
    assert context.check_hostname


def test_compose_service_and_localhost_connect_in_the_clear() -> None:
    for dsn in (
        "postgresql://bot:pw@postgres:5432/bot_db",
        "postgresql://bot:pw@localhost:5432/bot_db",
        "postgresql+asyncpg://bot:pw@127.0.0.1/bot_db",
    ):
        url, connect_args = engine_settings(dsn)

        assert url.drivername == "postgresql+asyncpg"
        assert connect_args == {}


def test_password_survives_the_rewrite() -> None:
    url, _ = engine_settings("postgresql://bot:p-w_1@db.example.com/bot_db")

    assert url.password == "p-w_1"
    assert url.render_as_string(hide_password=False).startswith(
        "postgresql+asyncpg://bot:p-w_1@db.example.com"
    )
