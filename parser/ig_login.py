"""
Run this ONCE to log in to Instagram and save a session file.
After this, the parser will use the saved session for uploads.

Usage:
    ./venv/bin/python ig_login.py
"""
import hashlib
import json
import os
from pathlib import Path

try:
    from instagrapi import Client
except ImportError:
    print("ERROR: instagrapi not installed. Run: ./venv/bin/pip install instagrapi")
    exit(1)

username = input("Instagram username: ").strip()
password = input("Instagram password: ").strip()

session_dir = Path(__file__).parent / "ig_sessions"
session_dir.mkdir(exist_ok=True)
session_file = session_dir / (hashlib.md5(username.encode()).hexdigest() + ".json")

cl = Client()
cl.delay_range = [1, 3]

def challenge_code_handler(username, choice):
    code = input(f"Enter the verification code sent to your {choice.name.lower()}: ").strip()
    return code

cl.challenge_code_handler = challenge_code_handler

print(f"\nLogging in as {username}...")
try:
    cl.login(username, password)
    cl.dump_settings(session_file)
    print(f"\nSuccess! Session saved to: {session_file}")
    print("You can now use Instagram upload from the admin panel.")
except Exception as e:
    print(f"\nLogin failed: {e}")
    exit(1)
