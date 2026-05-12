import os
import sys
import logging
import requests

# Add current directory to path so we can import local modules
sys.path.append(os.path.dirname(os.path.abspath(__file__)))

from asilmedia import AsilmediaParser
from downloader_service import _build_stream_headers
from helpers import quality_height, sort_video_candidates

# Configure logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(name)s - %(levelname)s - %(message)s')
logger = logging.getLogger("debug_asilmedia")

def debug_url(url):
    parser = AsilmediaParser()
    
    print(f"detail_url: {url}")
    
    # 1. Get details
    try:
        raw_candidates = []
        try:
            soup = parser._fetch_page(url)
            raw_candidates, _ = parser._extract_quality_links(soup, url)
            for candidate in raw_candidates:
                if candidate.get("url"):
                    candidate["url"] = parser._sanitize_video_url(candidate["url"])
        except Exception as e:
            print(f"raw_quality_error: {e}")

        details = parser.get_details(url)
        if not details:
            print("error: failed to get details")
            return
        
        print(f"title: {details.title}")

        validated_candidates = list(getattr(details, "video_urls", None) or [])
        candidates = raw_candidates or validated_candidates

        candidates = sort_video_candidates(candidates)
        print("all_qualities:")
        for source in candidates:
            q = source.get("quality") or "unknown"
            media_type = source.get("type") or "unknown"
            source_url = source.get("url") or ""
            print(f"- quality={q} height={quality_height(q, source_url)} type={media_type} url={source_url}")
        
        if not candidates:
            print("selected_url:")
            print("probe_error: no qualities found")
            return
            
        selected = candidates[0]
        selected_url = selected.get("url", "")
        print(f"selected_quality: {selected.get('quality') or 'unknown'}")
        print(f"selected_url: {selected_url}")
        
        # 3. Probe URL
        headers = _build_stream_headers(url)
        print(f"probe_headers: User-Agent={headers.get('User-Agent', '')} Referer={headers.get('Referer', '')} Origin={headers.get('Origin', '')}")
        
        try:
            resp = requests.head(selected_url, headers=headers, timeout=15, allow_redirects=True)
            print(f"HEAD status: {resp.status_code}")
            print(f"HEAD content-length: {resp.headers.get('Content-Length', '')}")
            print(f"HEAD content-type: {resp.headers.get('Content-Type', '')}")
            resp.close()

            get_headers = dict(headers)
            get_headers["Range"] = "bytes=0-0"
            resp = requests.get(selected_url, headers=get_headers, timeout=15, allow_redirects=True, stream=True)
            print(f"GET status: {resp.status_code}")
            print(f"GET content-length: {resp.headers.get('Content-Length', '')}")
            print(f"GET content-type: {resp.headers.get('Content-Type', '')}")
            resp.close()
                
        except Exception as e:
            print(f"probe_error: {e}")

    except Exception as e:
        logger.error(f"Error during debug: {e}", exc_info=True)

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python debug_asilmedia.py <asilmedia_url>")
        sys.exit(1)
        
    debug_url(sys.argv[1])
