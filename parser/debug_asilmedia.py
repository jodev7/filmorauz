import os
import sys
import logging
import json
import requests
from urllib.parse import urlparse

# Add current directory to path so we can import local modules
sys.path.append(os.path.dirname(os.path.abspath(__file__)))

from asilmedia import AsilmediaParser
from downloader_service import DownloaderService, _build_stream_headers

# Configure logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(name)s - %(levelname)s - %(message)s')
logger = logging.getLogger("debug_asilmedia")

def debug_url(url):
    parser = AsilmediaParser()
    downloader = DownloaderService()
    
    logger.info(f"Debugging Asilmedia URL: {url}")
    
    # 1. Get details
    try:
        details = parser.get_details(url)
        if not details:
            logger.error("Failed to get details")
            return
        
        logger.info(f"Title: {details.title}")
        logger.info(f"Main Video URL: {details.video_url}")
        logger.info(f"Available Qualities: {len(details.video_urls)}")
        
        for i, source in enumerate(details.video_urls):
            logger.info(f"Source {i}: Quality={source['quality']}, Type={source['type']}, URL={source['url'][:100]}...")
            
        # 2. Select quality
        candidates = details.video_urls
        if not candidates and details.video_url:
            candidates = [{"url": details.video_url, "quality": details.quality, "type": "unknown"}]
            
        # Sort candidates (simulate worker logic)
        def quality_rank(q):
            if not q: return 0
            import re
            m = re.search(r'(\d+)', q)
            return int(m.group(1)) if m else 0
            
        candidates.sort(key=lambda x: quality_rank(x['quality']), reverse=True)
        
        if not candidates:
            logger.error("No candidates found")
            return
            
        selected = candidates[0]
        logger.info(f"Selected: Quality={selected['quality']}, URL={selected['url'][:100]}...")
        
        # 3. Probe URL
        headers = _build_stream_headers(url)
        logger.info(f"Probing with headers: {headers}")
        
        try:
            # Try HEAD first
            resp = requests.head(selected['url'], headers=headers, timeout=15, allow_redirects=True)
            logger.info(f"HEAD Status: {resp.status_code}")
            logger.info(f"Content-Type: {resp.headers.get('Content-Type')}")
            logger.info(f"Content-Length: {resp.headers.get('Content-Length')} ({int(resp.headers.get('Content-Length', 0))/1024/1024:.2f} MB)")
            
            if resp.status_code != 200:
                # Try GET if HEAD fails
                logger.info("HEAD failed, trying GET (stream=True)...")
                resp = requests.get(selected['url'], headers=headers, timeout=15, allow_redirects=True, stream=True)
                logger.info(f"GET Status: {resp.status_code}")
                logger.info(f"Content-Type: {resp.headers.get('Content-Type')}")
                logger.info(f"Content-Length: {resp.headers.get('Content-Length')}")
                resp.close()
                
        except Exception as e:
            logger.error(f"Probe failed: {e}")
            
    except Exception as e:
        logger.error(f"Error during debug: {e}", exc_info=True)

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python debug_asilmedia.py <asilmedia_url>")
        sys.exit(1)
        
    debug_url(sys.argv[1])
