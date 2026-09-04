from typing import Annotated, Literal
from uuid import UUID

from fastapi import APIRouter, Depends, status
from pydantic import BaseModel, StringConstraints, model_validator
from sqlalchemy import select
from sqlalchemy.orm import Session

from app.api.dependencies.session import (
    CurrentUserSession,
    get_db_session,
    get_settings,
    require_user_session,
)
from app.application.keys.create import create_api_key
from app.application.keys.revoke import delete_api_key
from app.application.keys.update import update_api_key
from app.core.ids import new_request_id
from app.core.settings import Settings
from app.domain.models import ApiKey

router = APIRouter(prefix="/v1/keys", tags=["api-keys"])
KeyName = Annotated[str, StringConstraints(strip_whitespace=True, min_length=1, max_length=128)]


class CreateApiKeyRequest(BaseModel):
    name: KeyName


class UpdateApiKeyRequest(BaseModel):
    name: KeyName | None = None
    status: Literal["ACTIVE", "DISABLED"] | None = None

    @model_validator(mode="after")
    def has_change(self) -> "UpdateApiKeyRequest":
        if self.name is None and self.status is None:
            raise ValueError("At least one field must be provided")
        return self


def serialize_api_key(api_key: ApiKey) -> dict[str, object]:
    return {
        "id": str(api_key.id),
        "name": api_key.name,
        "prefix": api_key.key_prefix,
        "last4": api_key.key_last4,
        "status": api_key.status,
        "total_calls": api_key.total_calls,
        "created_at": api_key.created_at.isoformat() if api_key.created_at else None,
        "last_used_at": api_key.last_used_at.isoformat() if api_key.last_used_at else None,
    }


@router.post("", status_code=status.HTTP_201_CREATED)
def create_key(
    payload: CreateApiKeyRequest,
    current: CurrentUserSession = Depends(require_user_session),
    database_session: Session = Depends(get_db_session),
    settings: Settings = Depends(get_settings),
) -> dict[str, object]:
    created = create_api_key(database_session, settings, user_id=current.user.id, name=payload.name)
    database_session.commit()
    return {
        "success": True,
        "request_id": new_request_id(),
        "data": {**serialize_api_key(created.record), "secret": created.secret},
    }


@router.get("")
def list_keys(
    current: CurrentUserSession = Depends(require_user_session),
    database_session: Session = Depends(get_db_session),
) -> dict[str, object]:
    keys = database_session.scalars(
        select(ApiKey).where(ApiKey.user_id == current.user.id).order_by(ApiKey.created_at.desc())
    )
    return {
        "success": True,
        "request_id": new_request_id(),
        "data": {"items": [serialize_api_key(api_key) for api_key in keys]},
    }


@router.patch("/{key_id}")
def patch_key(
    key_id: UUID,
    payload: UpdateApiKeyRequest,
    current: CurrentUserSession = Depends(require_user_session),
    database_session: Session = Depends(get_db_session),
) -> dict[str, object]:
    api_key = update_api_key(
        database_session,
        current.user.id,
        key_id,
        name=payload.name,
        status=payload.status,
    )
    database_session.commit()
    return {"success": True, "request_id": new_request_id(), "data": serialize_api_key(api_key)}


@router.delete("/{key_id}")
def delete_key(
    key_id: UUID,
    current: CurrentUserSession = Depends(require_user_session),
    database_session: Session = Depends(get_db_session),
) -> dict[str, object]:
    api_key = delete_api_key(database_session, current.user.id, key_id)
    database_session.commit()
    return {"success": True, "request_id": new_request_id(), "data": serialize_api_key(api_key)}
