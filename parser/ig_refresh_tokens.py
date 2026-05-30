"""
Refresh all 60-day Instagram tokens in ig_accounts.json.

Long-lived Instagram tokens last 60 days and can be refreshed any time after
they are 24h old, each refresh extending validity another 60 days. Run this on
a cron (e.g. weekly) so tokens never expire while the account stays active.

    python ig_refresh_tokens.py

A token that is already expired / invalid cannot be refreshed — that account
is reported as FAILED and must be re-linked via ig_add_account.py.
"""
import json
from datetime import date
from pathlib import Path

import requests

BASE = Path(__file__).parent
ACCOUNTS_FILE = BASE / "ig_accounts.json"
REFRESH_URL = "https://graph.instagram.com/refresh_access_token"


def main():
    data = json.loads(ACCOUNTS_FILE.read_text())
    accounts = data.get("accounts", [])
    changed = False
    failures = []

    for a in accounts:
        name = a.get("name") or a.get("username")
        token = (a.get("access_token") or "").strip()
        if not token:
            failures.append((name, "no token"))
            continue
        try:
            r = requests.get(
                REFRESH_URL,
                params={"grant_type": "ig_refresh_token", "access_token": token},
                timeout=30,
            )
            body = r.json()
            new_token = body.get("access_token")
            if r.status_code >= 400 or not new_token:
                failures.append((name, str(body)))
                continue
            a["access_token"] = new_token
            a["token_obtained_at"] = date.today().isoformat()
            changed = True
            print(f"OK   {name}: refreshed (expires_in={body.get('expires_in')}s)")
        except Exception as exc:
            failures.append((name, str(exc)))

    if changed:
        ACCOUNTS_FILE.write_text(json.dumps(data, indent=2, ensure_ascii=False) + "\n")

    if failures:
        print("\nFAILED (re-link via ig_add_account.py):")
        for name, reason in failures:
            print(f"  - {name}: {reason}")
        raise SystemExit(1)
    print(f"\nAll {len(accounts)} tokens refreshed.")


if __name__ == "__main__":
    main()
