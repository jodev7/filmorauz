"""
Base Parser Class
All parsers must inherit from this class and implement required methods.
"""
import requests
from abc import ABC, abstractmethod
from typing import List, Dict, Any, Optional
from bs4 import BeautifulSoup
import logging
import os

from source_config import get_source_config, get_search_paths

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Enable debug logging
DEBUG = os.environ.get("PARSER_DEBUG", "false").lower() == "true"


class SearchResult:
    """Represents a search result from a source"""
    def __init__(
        self,
        title: str,
        year: int,
        poster: str,
        description: str,
        source_id: str,
        detail_url: str,
        source: str,
        content_type: str = "movie"  # "movie" or "serial"
    ):
        self.title = title
        self.year = year
        self.poster = poster
        self.description = description
        self.source_id = source_id
        self.detail_url = detail_url
        self.source = source
        self.content_type = content_type

    def to_dict(self) -> Dict[str, Any]:
        return {
            "title": self.title,
            "year": self.year,
            "poster": self.poster,
            "description": self.description,
            "source_id": self.source_id,
            "detail_url": self.detail_url,
            "source": self.source,
            "type": self.content_type
        }


class MovieDetails:
    """Represents detailed movie information"""
    def __init__(
        self,
        title: str,
        description: str,
        poster: str,
        backdrop: Optional[str],
        year: int,
        genres: List[str],
        country: str,
        duration: int,
        video_page_url: str = "",
        video_urls: List[Dict[str, str]] = None,
        source: str = "",
        source_id: str = "",
        type: str = "movie",
        quality: str = "",
        detail_url: str = "",
        player_url: str = ""
    ):
        self.title = title
        self.description = description
        self.poster = poster
        self.backdrop = backdrop
        self.year = year
        self.genres = genres
        self.country = country
        self.duration = duration
        self.video_page_url = video_page_url
        self.video_urls = video_urls or []
        self.source = source
        self.source_id = source_id
        self.type = type
        self.quality = quality
        self.detail_url = detail_url
        self.player_url = player_url

    def to_dict(self) -> Dict[str, Any]:
        return {
            "title": self.title,
            "description": self.description,
            "poster": self.poster,
            "backdrop": self.backdrop,
            "year": self.year,
            "genres": self.genres,
            "country": self.country,
            "duration": self.duration,
            "video_page_url": self.video_page_url,
            "video_urls": self.video_urls,
            "source": self.source,
            "source_id": self.source_id,
            "type": self.type,
            "quality": self.quality,
            "detail_url": self.detail_url,
            "player_url": self.player_url
        }


class BaseParser(ABC):
    """Abstract base class for all source parsers"""
    
    def __init__(self):
        self.session = requests.Session()
        self.session.headers.update({
            "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
            "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
            "Accept-Language": "en-US,en;q=0.5",
        })
    
    @abstractmethod
    def search(self, query: str) -> List[SearchResult]:
        """Search for movies on the source website."""
        pass
    
    @abstractmethod
    def get_details(self, url: str) -> MovieDetails:
        """Get detailed information about a movie from detail page URL."""
        pass
    
    @property
    @abstractmethod
    def source_name(self) -> str:
        """Return the source name (e.g., 'uzmovi', 'freekino')"""
        pass
    
    @property
    @abstractmethod
    def base_url(self) -> str:
        """Return the base URL of the source"""
        pass
    
    def _build_browser_headers(self, referer: Optional[str] = None) -> Dict[str, str]:
        """Standardize browser headers across all parsers"""
        headers = {
            "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
            "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
            "Accept-Language": "en-US,en;q=0.9",
            "Connection": "keep-alive",
        }
        if referer:
            headers["Referer"] = referer
            origin = self._origin_for_referer(referer)
            if origin:
                headers["Origin"] = origin
        return headers

    def _origin_for_referer(self, referer: Optional[str]) -> str:
        """Derive Origin from Referer, handling mirrors correctly"""
        referer = (referer or "").strip()
        if not referer:
            return ""
        try:
            from urllib.parse import urlparse
            parsed = urlparse(referer)
            host = (parsed.netloc or "").lower()
            
            # Known providers with multiple mirrors
            if any(p in host for p in ["asilmedia", "uzmovi", "freekino"]):
                return f"{parsed.scheme}://{parsed.netloc}"
                
            if parsed.scheme and parsed.netloc:
                return f"{parsed.scheme}://{parsed.netloc}"
        except Exception:
            pass
        return ""

    def _fetch_page(self, url: str, timeout: int = 30) -> BeautifulSoup:
        """Fetch a page and return BeautifulSoup object."""
        try:
            response = self.session.get(url, timeout=timeout)
            response.raise_for_status()
            return BeautifulSoup(response.content, "lxml")
        except requests.RequestException as e:
            logger.error(f"Failed to fetch {url}: {e}")
            raise
    
    def _try_search_endpoints(self, query: str, extract_func) -> List[SearchResult]:
        """
        Try multiple search endpoints from source config.
        
        Args:
            query: Search query
            extract_func: Function to extract results from BeautifulSoup
            
        Returns:
            List of SearchResult or empty list
        """
        results = []
        source = self.source_name
        
        # Get search paths from config
        search_paths = get_search_paths(source)
        base_url = self.base_url
        
        if DEBUG:
            logger.info(f"[{source.upper()}] Trying search paths: {search_paths}")
        
        # Get source-specific config
        config = get_source_config(source)
        search_method = config.get("search_method", "GET")
        search_params = config.get("search_params", {}).copy()
        search_param_key = config.get("search_param_key", "q")
        
        for path in search_paths:
            full_url = f"{base_url.rstrip('/')}{path}"
            
            try:
                if DEBUG:
                    logger.info(f"[{source.upper()}] ATTEMPT: {full_url}")
                
                # Build request
                if search_method.upper() == "POST":
                    search_params[search_param_key] = query
                    response = self.session.post(full_url, data=search_params, timeout=30, allow_redirects=True)
                else:
                    # GET request
                    params = search_params.copy()
                    params[search_param_key] = query
                    response = self.session.get(full_url, params=params, timeout=30, allow_redirects=True)
                
                # Log response details
                final_url = response.url
                status = response.status_code
                
                if DEBUG:
                    logger.info(f"[{source.upper()}] STATUS: {status}, FINAL_URL: {final_url}")
                
                # Check for redirect
                if final_url != full_url:
                    if DEBUG:
                        logger.info(f"[{source.upper()}] REDIRECT: {full_url} -> {final_url}")
                
                # Skip 404
                if status == 404:
                    if DEBUG:
                        logger.info(f"[{source.upper()}] 404, trying next path")
                    continue
                
                # Skip non-success
                if status >= 400:
                    if DEBUG:
                        logger.info(f"[{source.upper()}] Error {status}, trying next path")
                    continue
                
                # Check for meaningful content
                if len(response.text) < 100:
                    if DEBUG:
                        logger.info(f"[{source.upper()}] Response too short, trying next path")
                    continue
                
                # Extract results
                soup = BeautifulSoup(response.text, "lxml")
                results = extract_func(soup, query)
                
                if results:
                    if DEBUG:
                        logger.info(f"[{source.upper()}] SUCCESS: {len(results)} results")
                    return results
                else:
                    if DEBUG:
                        logger.info(f"[{source.upper()}] No results, trying next path")
                        
            except requests.RequestException as e:
                if DEBUG:
                    logger.info(f"[{source.upper()}] Error: {e}, trying next path")
                continue
            except Exception as e:
                if DEBUG:
                    logger.info(f"[{source.upper()}] Parse error: {e}")
                continue
        
        if DEBUG:
            logger.info(f"[{source.upper()}] All endpoints failed, returning empty")
        
        return []
