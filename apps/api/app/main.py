from uuid import uuid4

from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse

from app.api.public.auth import router as auth_router
from app.application.errors import ApplicationError
from app.core.settings import Settings
from app.db.session import SessionFactory, create_session_factory


def create_app(
    settings: Settings | None = None, session_factory: SessionFactory | None = None
) -> FastAPI:
    app_settings = settings or Settings()
    app = FastAPI(
        title="GeeTest Service Platform",
        version="1.0.0",
    )
    app.state.settings = app_settings
    if session_factory is None:
        engine, session_factory = create_session_factory(app_settings)
        app.state.db_engine = engine
    app.state.session_factory = session_factory

    @app.exception_handler(ApplicationError)
    async def application_error_handler(
        _: Request, error: ApplicationError
    ) -> JSONResponse:
        return JSONResponse(
            status_code=error.status_code,
            content={
                "success": False,
                "request_id": f"req_{uuid4().hex}",
                "error": {
                    "code": error.code,
                    "message": error.message,
                    "retryable": False,
                },
            },
        )

    @app.exception_handler(RequestValidationError)
    async def request_validation_error_handler(
        _: Request, __: RequestValidationError
    ) -> JSONResponse:
        return JSONResponse(
            status_code=422,
            content={
                "success": False,
                "request_id": f"req_{uuid4().hex}",
                "error": {
                    "code": "INVALID_REQUEST",
                    "message": "Request validation failed.",
                    "retryable": False,
                },
            },
        )

    @app.get("/healthz", include_in_schema=False)
    async def healthz() -> dict[str, str]:
        return {"status": "ok"}

    app.include_router(auth_router)
    return app


app = create_app()
