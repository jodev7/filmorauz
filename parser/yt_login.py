"""
YouTube OAuth2 login helper.

Run ONCE per account to authorize and save a token file.
After this, the token auto-refreshes — no need to run again unless revoked.

Usage:
    python yt_login.py --client-secret client_secret.json --account main

Steps before running:
  1. Go to https://console.cloud.google.com/
  2. Create a project, enable "YouTube Data API v3"
  3. Create OAuth2 credentials → Desktop app → Download JSON as client_secret.json
  4. Place client_secret.json next to this script
  5. Run: python yt_login.py --client-secret client_secret.json --account main
  6. A browser window opens → log in with the YouTube channel's Google account → allow
  7. Token saved to: yt_sessions/<account>.json

Then add to backend/.env:
    YOUTUBE_ACCOUNTS_JSON=[{"name":"main","token_file":"yt_sessions/main_token.json"}]
"""

import argparse
import json
import os
from pathlib import Path

def main():
    parser = argparse.ArgumentParser(description="YouTube OAuth2 login")
    parser.add_argument("--client-secret", required=True, help="Path to client_secret.json from Google Cloud Console")
    parser.add_argument("--account", required=True, help="Account name (used for token filename, e.g. 'main')")
    args = parser.parse_args()

    try:
        from google_auth_oauthlib.flow import InstalledAppFlow
        from google.oauth2.credentials import Credentials
    except ImportError:
        print("ERROR: Missing dependency. Run:\n  pip install google-auth-oauthlib google-api-python-client")
        exit(1)

    SCOPES = ["https://www.googleapis.com/auth/youtube.upload"]

    client_secret_path = Path(args.client_secret)
    if not client_secret_path.exists():
        print(f"ERROR: client_secret file not found: {client_secret_path}")
        exit(1)

    print(f"[yt_login] Starting OAuth2 flow for account '{args.account}'...")
    print("[yt_login] A browser window will open. Log in with the YouTube channel's Google account.")

    flow = InstalledAppFlow.from_client_secrets_file(str(client_secret_path), SCOPES)
    creds = flow.run_local_server(port=0)

    # Save token
    session_dir = Path(__file__).parent / "yt_sessions"
    session_dir.mkdir(exist_ok=True)
    token_file = session_dir / f"{args.account}_token.json"

    token_data = {
        "token": creds.token,
        "refresh_token": creds.refresh_token,
        "token_uri": creds.token_uri,
        "client_id": creds.client_id,
        "client_secret": creds.client_secret,
        "scopes": list(creds.scopes) if creds.scopes else [],
    }
    token_file.write_text(json.dumps(token_data, indent=2))

    print(f"\n[yt_login] Token saved: {token_file}")
    print(f"\nAdd to backend/.env:")
    print(f'YOUTUBE_ACCOUNTS_JSON=[{{"name":"{args.account}","token_file":"yt_sessions/{args.account}_token.json"}}]')
    print("\nDone!")

if __name__ == "__main__":
    main()
