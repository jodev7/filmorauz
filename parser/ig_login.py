"""
Log in to Instagram for a named account and save a reusable session file.

Usage:
    python ig_login.py main
    python ig_login.py backup1
"""
import argparse
import getpass
import os
import re
from pathlib import Path
from urllib.parse import urlparse

from dotenv import load_dotenv

try:
    from instagrapi import Client
except ImportError:
    print("ERROR: instagrapi not installed. Run: ./venv/bin/pip install instagrapi")
    raise SystemExit(1)


BASE_DIR = Path(__file__).parent
load_dotenv(BASE_DIR / ".env")
SESSION_DIR = BASE_DIR / "ig_sessions"
SESSION_DIR.mkdir(exist_ok=True)


def normalize_account_name(name: str) -> str:
    return re.sub(r"[^A-Za-z0-9]+", "_", (name or "").strip()).strip("_").lower()


def env_prefix(account: str) -> str:
    return normalize_account_name(account).upper()


def mask_proxy(proxy: str) -> str:
    if not proxy:
        return ""
    parsed = urlparse(proxy)
    if not parsed.scheme or not parsed.netloc:
        return "***"
    if "@" not in parsed.netloc:
        return f"{parsed.scheme}://{parsed.netloc}"
    creds, host = parsed.netloc.rsplit("@", 1)
    if ":" in creds:
        user, _ = creds.split(":", 1)
        creds = f"{user}:***"
    else:
        creds = "***"
    return f"{parsed.scheme}://{creds}@{host}"


def classify_error(exc: Exception) -> str:
    text = str(exc).lower()
    if "proxy" in text or "connection refused" in text or "connect timeout" in text:
        return "proxy_failed"
    if any(token in text for token in ("challenge_required", "checkpoint_required", "account recovery", "confirm", "verify", "two_factor")):
        return "challenge_required"
    if any(token in text for token in ("feedback_required", "action blocked", "please wait", "too many", "spam", "rate limit")):
        return "action_blocked"
    if any(token in text for token in ("login_required", "session_expired", "forbidden", "403", "not authenticated")):
        return "session_expired"
    return "upload_failed"


def build_client(proxy: str) -> Client:
    cl = Client()
    cl.delay_range = [1, 3]
    if proxy:
        cl.set_proxy(proxy)
    return cl


def challenge_code_handler(username, choice):
    prompt = f"Enter the verification code sent to your {choice.name.lower()} for {username}: "
    return input(prompt).strip()


def main():
    parser = argparse.ArgumentParser(description="Log in to Instagram and save account session")
    parser.add_argument("account", nargs="?", help="Configured account name, e.g. main or backup1")
    args = parser.parse_args()

    account = normalize_account_name(args.account or input("Account name (main/backup1): ").strip())
    if not account:
        print("ERROR: account name is required")
        raise SystemExit(1)

    prefix = env_prefix(account)
    username = os.environ.get(f"IG_{prefix}_USERNAME", "").strip() or input("Instagram username: ").strip()
    password = os.environ.get(f"IG_{prefix}_PASSWORD", "").strip() or getpass.getpass("Instagram password: ").strip()
    proxy = os.environ.get(f"IG_{prefix}_PROXY", "").strip()
    session_file = SESSION_DIR / f"{account}.json"

    print(f"Account: {account}")
    print(f"Username: {username}")
    print(f"Session file: {session_file}")
    print(f"Proxy: {mask_proxy(proxy) if proxy else 'not set'}")

    cl = build_client(proxy)
    cl.challenge_code_handler = challenge_code_handler

    try:
        cl.login(username, password)
        cl.dump_settings(session_file)
        print(f"\nSuccess. Session saved to: {session_file}")
        print("Instagram uploader endi shu session fayldan foydalanadi.")
    except Exception as exc:
        error_type = classify_error(exc)
        print(f"\nLogin failed [{error_type}]: {exc}")
        if error_type == "challenge_required":
            print("Instagram app yoki email/SMS challenge ni tasdiqlang, keyin shu buyruqni qayta ishga tushiring.")
        elif error_type == "proxy_failed":
            print("Proxy ishlamayapti. IG_<ACCOUNT>_PROXY ni tekshiring yoki almashtiring.")
        raise SystemExit(1)


if __name__ == "__main__":
    main()
