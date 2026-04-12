"""
TikTok OAuth2 login helper for Content Posting API.

Run ONCE per account to authorize and save a token file.
Access token auto-refreshes (expires 24h, refresh token lasts 365 days).

Usage:
    python tt_login.py --client-key YOUR_KEY --client-secret YOUR_SECRET --account main

Steps before running:
  1. Go to https://developers.tiktok.com/
  2. Create an app → request "Content Posting API" product
  3. In app settings, add redirect URI: http://localhost:8765/callback
  4. Copy your Client Key and Client Secret
  5. Run: python tt_login.py --client-key xxx --client-secret yyy --account main
  6. A browser window opens → log in with TikTok account → allow
  7. Token saved to: tt_sessions/<account>_token.json

Then add to backend/.env:
    TIKTOK_ACCOUNTS_JSON=[{"name":"main","token_file":"tt_sessions/main_token.json"}]
"""

import argparse
import json
import secrets
import threading
import time
import urllib.parse
import urllib.request
import webbrowser
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path

REDIRECT_URI = "http://localhost:8765/callback"
AUTH_BASE = "https://www.tiktok.com/v2/auth/authorize/"
TOKEN_URL = "https://open.tiktokapis.com/v2/oauth/token/"
SCOPES = "video.upload,video.publish"

auth_code_result = {}


class CallbackHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        parsed = urllib.parse.urlparse(self.path)
        if parsed.path == "/callback":
            params = urllib.parse.parse_qs(parsed.query)
            code = params.get("code", [None])[0]
            error = params.get("error", [None])[0]
            auth_code_result["code"] = code
            auth_code_result["error"] = error
            self.send_response(200)
            self.send_header("Content-Type", "text/html")
            self.end_headers()
            if code:
                self.wfile.write(b"<h2>Authorized! You can close this window.</h2>")
            else:
                self.wfile.write(f"<h2>Error: {error}</h2>".encode())

    def log_message(self, format, *args):
        pass  # suppress request logs


def exchange_code(code, client_key, client_secret):
    payload = {
        "client_key": client_key,
        "client_secret": client_secret,
        "code": code,
        "grant_type": "authorization_code",
        "redirect_uri": REDIRECT_URI,
    }
    data = urllib.parse.urlencode(payload).encode("utf-8")
    req = urllib.request.Request(
        TOKEN_URL,
        data=data,
        headers={"Content-Type": "application/x-www-form-urlencoded"},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=15) as resp:
        return json.loads(resp.read().decode("utf-8"))


def main():
    parser = argparse.ArgumentParser(description="TikTok OAuth2 login")
    parser.add_argument("--client-key", required=True, help="TikTok app Client Key")
    parser.add_argument("--client-secret", required=True, help="TikTok app Client Secret")
    parser.add_argument("--account", required=True, help="Account name (e.g. 'main')")
    args = parser.parse_args()

    state = secrets.token_urlsafe(16)

    auth_params = {
        "client_key": args.client_key,
        "scope": SCOPES,
        "response_type": "code",
        "redirect_uri": REDIRECT_URI,
        "state": state,
    }
    auth_url = AUTH_BASE + "?" + urllib.parse.urlencode(auth_params)

    # Start local callback server
    server = HTTPServer(("localhost", 8765), CallbackHandler)
    thread = threading.Thread(target=server.handle_request)
    thread.daemon = True
    thread.start()

    print(f"[tt_login] Opening browser for account '{args.account}'...")
    print(f"[tt_login] If browser doesn't open, visit:\n  {auth_url}\n")
    webbrowser.open(auth_url)

    # Wait up to 120 seconds for callback
    deadline = time.time() + 120
    while "code" not in auth_code_result and time.time() < deadline:
        time.sleep(0.5)

    server.server_close()

    if auth_code_result.get("error"):
        print(f"ERROR: TikTok returned error: {auth_code_result['error']}")
        exit(1)
    if not auth_code_result.get("code"):
        print("ERROR: No code received — timed out or browser was closed.")
        exit(1)

    code = auth_code_result["code"]
    print(f"[tt_login] Got auth code, exchanging for tokens...")

    try:
        token_resp = exchange_code(code, args.client_key, args.client_secret)
    except Exception as e:
        print(f"ERROR: Token exchange failed: {e}")
        exit(1)

    if "data" not in token_resp:
        print(f"ERROR: Unexpected response: {token_resp}")
        exit(1)

    data = token_resp["data"]

    # Save token file
    session_dir = Path(__file__).parent / "tt_sessions"
    session_dir.mkdir(exist_ok=True)
    token_file = session_dir / f"{args.account}_token.json"

    token_data = {
        "access_token": data.get("access_token", ""),
        "refresh_token": data.get("refresh_token", ""),
        "open_id": data.get("open_id", ""),
        "client_key": args.client_key,
        "client_secret": args.client_secret,
        "scope": data.get("scope", SCOPES),
        "expires_in": data.get("expires_in", 86400),
        "refresh_expires_in": data.get("refresh_expires_in", 31536000),
    }
    token_file.write_text(json.dumps(token_data, indent=2))

    print(f"\n[tt_login] Token saved: {token_file}")
    print(f"  open_id: {token_data['open_id']}")
    print(f"\nAdd to backend/.env:")
    print(f'TIKTOK_ACCOUNTS_JSON=[{{"name":"{args.account}","token_file":"tt_sessions/{args.account}_token.json"}}]')
    print("\nDone!")


if __name__ == "__main__":
    main()
