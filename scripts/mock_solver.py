"""Mock GeeTest solver for local E2E verification of /v1/captcha/solve.

Implements POST /v1/geetest/solve with the same contract as the real solver:
header X-Service-Key must match MOCK_SERVICE_KEY; returns the standard
success envelope or 401/422 errors.
"""
import json
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

MOCK_SERVICE_KEY = "demo-service-key"


class Handler(BaseHTTPRequestHandler):
    def _send(self, status, payload):
        body = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        if self.path != "/v1/geetest/solve":
            return self._send(404, {"success": False, "error": "not found"})
        if self.headers.get("X-Service-Key") != MOCK_SERVICE_KEY:
            return self._send(401, {"success": False, "error": "unauthorized"})
        length = int(self.headers.get("Content-Length", 0))
        try:
            data = json.loads(self.rfile.read(length) or b"{}")
        except json.JSONDecodeError:
            return self._send(422, {"success": False, "error": "bad json"})
        captcha_id = data.get("captcha_id")
        if not captcha_id:
            return self._send(422, {"success": False, "error": "captcha_id required"})
        print(f"[mock-solver] solve captcha_id={captcha_id}", flush=True)
        return self._send(200, {
            "success": True,
            "data": {
                "captcha_id": captcha_id,
                "lot_number": "lot-e2e-" + captcha_id[-4:],
                "captcha_output": "out-e2e",
                "pass_token": "tok-e2e",
                "gen_time": str(int(time.time())),
            },
        })

    def log_message(self, *args):
        pass


if __name__ == "__main__":
    print("[mock-solver] listening on 127.0.0.1:18081", flush=True)
    HTTPServer(("127.0.0.1", 18081), Handler).serve_forever()
