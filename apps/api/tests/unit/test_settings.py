from pathlib import Path

from _pytest.monkeypatch import MonkeyPatch

from app.core.settings import Settings


def test_settings_do_not_load_an_env_file_from_the_process_directory(
    monkeypatch: MonkeyPatch, tmp_path: Path
) -> None:
    (tmp_path / ".env").write_text(
        "DATABASE_URL=postgresql+asyncpg://wrong:wrong@wrong.example.com:5432/wrong\n",
        encoding="utf-8",
    )
    monkeypatch.chdir(tmp_path)
    monkeypatch.delenv("DATABASE_URL", raising=False)

    settings = Settings()

    assert settings.database_url == "postgresql+asyncpg://postgres:postgres@localhost:5432/geetest_platform"
