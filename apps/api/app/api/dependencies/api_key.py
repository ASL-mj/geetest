from typing import Annotated

from fastapi import Depends
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer
from sqlalchemy.orm import Session

from app.api.dependencies.session import get_db_session, get_settings
from app.application.errors import ApplicationError
from app.application.keys.authenticate import AuthenticatedCaller, authenticate_api_key
from app.core.settings import Settings

_bearer_scheme = HTTPBearer(auto_error=False)


def require_api_key(
    credentials: Annotated[HTTPAuthorizationCredentials | None, Depends(_bearer_scheme)],
    database_session: Session = Depends(get_db_session),
    settings: Settings = Depends(get_settings),
) -> AuthenticatedCaller:
    if credentials is None or credentials.scheme.lower() != "bearer":
        raise ApplicationError(401, "API_KEY_INVALID", "Bearer API Key is required.")
    return authenticate_api_key(database_session, settings, credentials.credentials)
