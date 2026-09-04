import hashlib
import hmac
import secrets


def hmac_sha256(value: str, pepper: str) -> bytes:
    return hmac.digest(pepper.encode("utf-8"), value.encode("utf-8"), hashlib.sha256)


def normalize_cdk(value: str) -> str:
    return "".join(character for character in value.upper() if character.isalnum())


def generate_opaque_token() -> str:
    return secrets.token_urlsafe(32)
