from app.application.auth.passwords import hash_password, verify_password


def test_passwords_are_hashed_with_argon2id() -> None:
    password_hash = hash_password("A-long-password-123")

    assert password_hash.startswith("$argon2id$")
    assert verify_password("A-long-password-123", password_hash)
    assert not verify_password("wrong-password", password_hash)
