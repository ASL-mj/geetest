from datetime import UTC, datetime
from typing import Annotated

from fastapi import APIRouter, Depends, Response, status
from fastapi.responses import JSONResponse
from pydantic import BaseModel, StringConstraints, field_validator
from sqlalchemy.orm import Session

from app.api.dependencies.session import (
    CurrentUserSession,
    get_db_session,
    get_settings,
    require_user_session,
)
from app.application.auth.activate import activate_cdk
from app.application.errors import ApplicationError
from app.core.ids import new_request_id
from app.core.settings import Settings

router = APIRouter(prefix="/v1/auth", tags=["authentication"])

CdkCode = Annotated[str, StringConstraints(strip_whitespace=True, min_length=4, max_length=128)]


class ActivationRequest(BaseModel):
    cdk: CdkCode

    @field_validator("cdk")
    @classmethod
    def cdk_must_include_letters_or_digits(cls, value: str) -> str:
        if not any(character.isalnum() for character in value):
            raise ValueError("CDK must include at least one letter or digit")
        return value


def error_response(error: ApplicationError, correlation_id: str) -> JSONResponse:
    return JSONResponse(
        status_code=error.status_code,
        content={
            "success": False,
            "request_id": correlation_id,
            "error": {"code": error.code, "message": error.message, "retryable": False},
        },
    )


def user_data(user_id: object, cdk_prefix: str) -> dict[str, str]:
    return {"id": str(user_id), "cdk_prefix": cdk_prefix}


def set_session_cookie(response: Response, token: str, settings: Settings) -> None:
    response.set_cookie(
        key="session",
        value=token,
        max_age=settings.session_ttl_seconds,
        httponly=True,
        secure=True,
        samesite="lax",
        path="/",
    )


@router.post("/activate", status_code=status.HTTP_200_OK, response_model=None)
def activate(
    payload: ActivationRequest,
    response: Response,
    database_session: Session = Depends(get_db_session),
    settings: Settings = Depends(get_settings),
) -> dict[str, object] | JSONResponse:
    correlation_id = new_request_id()
    try:
        result = activate_cdk(
            database_session,
            settings,
            cdk_code=payload.cdk,
        )
    except ApplicationError as error:
        database_session.rollback()
        return error_response(error, correlation_id)

    response.status_code = status.HTTP_201_CREATED if result.newly_activated else status.HTTP_200_OK
    set_session_cookie(response, result.session.token, settings)
    return {
        "success": True,
        "request_id": correlation_id,
        "data": {
            "user": user_data(result.user.id, result.cdk_prefix),
            **({"default_api_key": result.default_api_key} if result.default_api_key else {}),
        },
    }


@router.post("/logout")
def logout(
    response: Response,
    current: CurrentUserSession = Depends(require_user_session),
    database_session: Session = Depends(get_db_session),
) -> dict[str, object]:
    current.session.revoked_at = datetime.now(UTC)
    database_session.commit()
    response.delete_cookie("session", path="/")
    return {"success": True, "request_id": new_request_id(), "data": {}}
