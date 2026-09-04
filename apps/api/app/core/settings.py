from pathlib import Path
from typing import Literal

from pydantic import Field, ValidationInfo, field_validator
from pydantic_settings import BaseSettings, SettingsConfigDict

Environment = Literal["local", "test", "production"]
_PLACEHOLDER_PREFIXES = ("change-me-", "sample-", "placeholder-")
_REPOSITORY_ROOT = Path(__file__).resolve().parents[4]


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=_REPOSITORY_ROOT / ".env",
        env_file_encoding="utf-8",
        extra="ignore",
        populate_by_name=True,
    )

    environment: Environment = Field(default="local", alias="APP_ENV")
    database_url: str = Field(
        default="postgresql+asyncpg://postgres:postgres@localhost:5432/geetest_platform",
        alias="DATABASE_URL",
    )
    redis_url: str = Field(default="redis://localhost:6379/0", alias="REDIS_URL")
    session_secret: str = Field(default="change-me-session-secret", alias="SESSION_SECRET")
    api_key_pepper: str = Field(default="change-me-api-key-pepper", alias="API_KEY_PEPPER")
    cdk_pepper: str = Field(default="change-me-cdk-pepper", alias="CDK_PEPPER")
    geetest_solver_url: str = Field(
        default="https://solver.internal.example.com",
        alias="GEETEST_SOLVER_URL",
    )
    geetest_service_api_key: str = Field(
        default="change-me-geetest-service-api-key",
        alias="GEETEST_SERVICE_API_KEY",
    )

    @field_validator(
        "session_secret",
        "api_key_pepper",
        "cdk_pepper",
        "geetest_service_api_key",
        mode="after",
    )
    @classmethod
    def require_non_placeholder_secret(cls, value: str, info: ValidationInfo) -> str:
        if info.data.get("environment") != "production":
            return value
        if not value or value.startswith(_PLACEHOLDER_PREFIXES):
            raise ValueError(f"{info.field_name} must be set to a production secret")
        return value

    @field_validator("database_url", "redis_url", "geetest_solver_url", mode="after")
    @classmethod
    def require_non_placeholder_endpoint(cls, value: str, info: ValidationInfo) -> str:
        if info.data.get("environment") != "production":
            return value
        if not value or "example.com" in value or "localhost" in value:
            raise ValueError(f"{info.field_name} must be set for production")
        return value
