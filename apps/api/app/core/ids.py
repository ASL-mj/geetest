from uuid import uuid4


def new_request_id() -> str:
    return f"req_{uuid4().hex}"
