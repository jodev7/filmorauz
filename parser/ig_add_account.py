"""
Exchange an Instagram OAuth `code` for a 60-day token and append the account
to ig_accounts.json. Idempotent on user_id (re-adding updates the token).

Usage:
    python ig_add_account.py "<code>"

Reads INSTAGRAM_LOGIN_APP_ID / _SECRET / INSTAGRAM_REDIRECT_URI from .env.
"""
import json
import sys
from datetime import date
from pathlib import Path

import requests
from dotenv import load_dotenv
import os

BASE = Path(__file__).parent
load_dotenv(BASE / ".env")
ACCOUNTS_FILE = BASE / "ig_accounts.json"

APP_ID = os.environ["INSTAGRAM_LOGIN_APP_ID"]
APP_SECRET = os.environ["INSTAGRAM_LOGIN_APP_SECRET"]
REDIRECT_URI = os.environ["INSTAGRAM_REDIRECT_URI"]


def clean_code(raw: str) -> str:
    raw = raw.strip()
    if "code=" in raw:
        raw = raw.split("code=", 1)[1]
    return raw.split("#", 1)[0].split("&", 1)[0].strip()


def main():
    if len(sys.argv) < 2:
        print("Usage: python ig_add_account.py \"<code or callback url>\"")
        raise SystemExit(1)

    code = clean_code(sys.argv[1])

    # 1) code -> short-lived token
    r = requests.post(
        "https://api.instagram.com/oauth/access_token",
        data={
            "client_id": APP_ID,
            "client_secret": APP_SECRET,
            "grant_type": "authorization_code",
            "redirect_uri": REDIRECT_URI,
            "code": code,
        },
        timeout=30,
    )
    r.raise_for_status()
    short = r.json()["access_token"]

    # 2) short -> long-lived (60 day) token
    r = requests.get(
        "https://graph.instagram.com/access_token",
        params={
            "grant_type": "ig_exchange_token",
            "client_secret": APP_SECRET,
            "access_token": short,
        },
        timeout=30,
    )
    r.raise_for_status()
    long_token = r.json()["access_token"]

    # 3) fetch account identity
    r = requests.get(
        "https://graph.instagram.com/v21.0/me",
        params={"fields": "user_id,username,account_type", "access_token": long_token},
        timeout=30,
    )
    r.raise_for_status()
    me = r.json()
    user_id = str(me.get("user_id") or me.get("id"))
    username = me.get("username", "")

    # 4) upsert into ig_accounts.json (keyed by user_id)
    data = {"accounts": []}
    if ACCOUNTS_FILE.exists():
        data = json.loads(ACCOUNTS_FILE.read_text())
    accounts = data.setdefault("accounts", [])

    entry = {
        "name": username,
        "username": username,
        "user_id": user_id,
        "access_token": long_token,
        "token_obtained_at": date.today().isoformat(),
        "filter": {},
    }
    for i, a in enumerate(accounts):
        if a.get("user_id") == user_id:
            entry["filter"] = a.get("filter", {})
            accounts[i] = entry
            break
    else:
        accounts.append(entry)

    ACCOUNTS_FILE.write_text(json.dumps(data, indent=2, ensure_ascii=False) + "\n")
    print(f"OK: {username} (user_id={user_id}) saved. Total accounts: {len(accounts)}")


if __name__ == "__main__":
    main()
