"""
Source Configuration
Contains search endpoints and configuration for each source
"""
from typing import List, Dict, Any

# Source-specific search configuration
SOURCES = {
    "uzmovi": {
        "base_url": "https://uzmovi.net",
        "search_paths": [
            "/search",
        ],
        # Real form on uzmovi.net: <form action="/search" method="get"><input name="q"></form>.
        # Sending DLE-style params (do=search&subaction=search&story=) returns the homepage.
        "search_method": "GET",
        "search_params": {},
        "search_param_key": "q",
    },
    "freekino": {
        "base_url": "https://freekino.net",
        "search_paths": [
            "/index.php",
            "/search",
            "/",
        ],
        "search_method": "POST",
        "search_params": {
            "do": "search",
            "subaction": "search",
        },
        "search_param_key": "story",
    },
    "asilmedia": {
        "base_url": "http://asilmedia.org",
        "search_paths": [
            "/index.php?do=search",
            "/search",
            "/",
        ],
        "search_method": "POST",
        "search_params": {
            "do": "search",
            "subaction": "search",
        },
        "search_param_key": "story",
    },
    "kinolar": {
        "base_url": "https://kinolar.tv",
        "search_paths": [
            "/index.php",
            "/search",
            "/",
        ],
        "search_method": "POST",  # Uses POST for search
        "search_params": {
            "do": "search",
            "subaction": "search",
        },
        "search_param_key": "story",
    },
    "kinochilar": {
        "base_url": "https://kinochilar.com",
        "search_paths": [
            "/index.php",
            "/search",
            "/",
        ],
        "search_method": "POST",
        "search_params": {
            "do": "search",
            "subaction": "search",
        },
        "search_param_key": "story",
    },
    "uzbeklar": {
        "base_url": "https://uzbeklar.biz",
        "search_paths": [
            "/",
            "/index.php?do=search",
        ],
        "search_method": "POST",
        "search_params": {
            "do": "search",
            "subaction": "search",
        },
        "search_param_key": "story",
    },
    "uzmedia": {
        "base_url": "https://uzmedia.tv",
        "search_paths": [
            "/search/",
            "/search",
            "/",
        ],
        "search_method": "GET",
        "search_params": {},
        "search_param_key": "q",
    },
    "seezntv": {
        # seezntv.uz is a Next.js SPA backed by a JSON REST API at
        # v2.seezntv.uz. The parser (seezntv.py) talks to that API directly and
        # does NOT use the generic HTML search infra below — this entry only
        # supplies base_url for metadata normalisation / logging.
        "base_url": "https://seezntv.uz",
        "search_paths": [],
        "search_method": "NONE",
        "search_params": {},
        "search_param_key": "",
    },
    "manual": {
        "base_url": "",
        "search_paths": [],
        "search_method": "NONE",
        "search_params": {},
        "search_param_key": "",
    },
}


def get_source_config(source: str) -> Dict[str, Any]:
    """Get configuration for a source"""
    return SOURCES.get(source, {})


def get_search_paths(source: str) -> List[str]:
    """Get search path candidates for a source"""
    config = get_source_config(source)
    return config.get("search_paths", ["/"])


def get_base_url(source: str) -> str:
    """Get base URL for a source"""
    config = get_source_config(source)
    return config.get("base_url", "")
