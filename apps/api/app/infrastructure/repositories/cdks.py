from sqlalchemy import select
from sqlalchemy.orm import Session

from app.domain.models import Cdk, CdkBatch


class CdkRepository:
    def __init__(self, session: Session) -> None:
        self._session = session

    def lock_by_code_hash(self, code_hash: bytes) -> tuple[Cdk, CdkBatch] | None:
        statement = (
            select(Cdk, CdkBatch)
            .join(CdkBatch, Cdk.batch_id == CdkBatch.id)
            .where(Cdk.code_hash == code_hash)
            .with_for_update()
        )
        return self._session.execute(statement).tuples().one_or_none()
