from fastapi.testclient import TestClient

from app.main import create_app


def test_platform_health_is_public_and_does_not_check_solver() -> None:
    response = TestClient(create_app()).get("/healthz")
    assert response.status_code == 200
    assert response.json() == {"status": "ok"}
