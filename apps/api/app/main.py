from fastapi import FastAPI

from app.core.settings import Settings


def create_app(settings: Settings | None = None) -> FastAPI:
    app_settings = settings or Settings()
    app = FastAPI(
        title="GeeTest Service Platform",
        version="1.0.0",
    )
    app.state.settings = app_settings

    @app.get("/healthz", include_in_schema=False)
    async def healthz() -> dict[str, str]:
        return {"status": "ok"}

    return app


app = create_app()
