"""
Kinolar (kinolar.uz) Parser
POST-based search implementation
"""
import logging
import os
from typing import List, Dict, Any, Optional
from urllib.parse import urljoin

from base_parser import BaseParser, SearchResult, MovieDetails
from helpers import (
    normalize_url, 
    clean_text, 
    extract_year, 
    extract_duration,
    extract_source_id,
    deduplicate_results,
    filter_and_rank_results
)

logger = logging.getLogger(__name__)

# Enable debug logging in development
DEBUG = os.environ.get("PARSER_DEBUG", "false").lower() == "true"


class KinolarParser(BaseParser):
    """Parser for kinolar.uz - uses POST for search"""
    
    BASE_URL = "https://kinolar.uz"
    
    # POST search endpoint
    SEARCH_ENDPOINT = "/index.php"
    SEARCH_METHOD = "POST"
    
    # Form field names for POST search
    POST_PARAMS = {
        "do": "search",
        "subaction": "search",
        "story": "",  # This will be filled with query
    }
    
    # Card selectors
    CARD_SELECTORS = [
        ".shortstory",
        ".film-item",
        ".movie-item",
        ".search-result",
        "article",
        ".movie-card",
    ]
    
    # Title selectors
    TITLE_SELECTORS = [
        "h2 a", "h3 a", "h4 a",
        ".title a", ".film-title a",
        ".movie-title a", ".short-title a",
    ]
    
    # Image selectors
    IMAGE_SELECTORS = [
        "img[data-src]",
        "img[data-lazy-src]",
        "img[data-original]",
        "img[src]",
    ]
    
    # Year selectors
    YEAR_SELECTORS = [".year", ".date", ".film-year", "[class*='year']"]
    
    @property
    def source_name(self) -> str:
        return "kinolar"
    
    @property
    def base_url(self) -> str:
        return self.BASE_URL
    
    def __init__(self):
        super().__init__()
        self.session.headers.update({
            "Referer": self.BASE_URL,
            "Origin": self.BASE_URL,
        })
    
    def search(self, query: str) -> List[Dict[str, Any]]:
        """
        Search using POST request
        """
        results = []
        
        try:
            # Build POST payload
            post_data = self.POST_PARAMS.copy()
            post_data["story"] = query
            
            logger.info(f"[KINOLAR] POST search: {self.BASE_URL}{self.SEARCH_ENDPOINT}")
            logger.info(f"[KINOLAR] POST data: {post_data}")
            
            # Make POST request
            response = self.session.post(
                f"{self.BASE_URL}{self.SEARCH_ENDPOINT}",
                data=post_data,
                timeout=30,
                allow_redirects=True
            )
            
            final_url = response.url
            status = response.status_code
            
            logger.info(f"[KINOLAR] Status: {status}, Final URL: {final_url}")
            logger.info(f"[KINOLAR] Response length: {len(response.text)}")
            
            if status != 200:
                logger.warning(f"[KINOLAR] Non-200 status: {status}")
                return []
            
            if len(response.text) < 100:
                logger.warning(f"[KINOLAR] Response too short: {len(response.text)}")
                return []
            
            # Parse HTML
            from bs4 import BeautifulSoup
            soup = BeautifulSoup(response.text, "lxml")
            
            # Extract results
            results = self._extract_results(soup, query)
            
            logger.info(f"[KINOLAR] Extracted {len(results)} results")
            
            # Deduplicate
            if results:
                results = deduplicate_results(results, key="link")
                logger.info(f"[KINOLAR] After dedup: {len(results)}")
                
                # Filter by query relevance
                results = filter_and_rank_results(results, query)
                logger.info(f"[KINOLAR] After filter: {len(results)}")
            
        except Exception as e:
            logger.error(f"[KINOLAR] Error: {e}")
        
        return results
    
    def _extract_results(self, soup, query: str = "") -> List[Dict[str, Any]]:
        """Extract movie cards from search results page"""
        
        # Find search container
        container = None
        container_selectors = [
            ".search_results", ".search-results", 
            ".results", ".movie-list", 
            ".film-list", "#content", ".content"
        ]
        
        for sel in container_selectors:
            found = soup.select_one(sel)
            if found:
                logger.info(f"[KINOLAR] Found container: {sel}")
                container = found
                break
        
        if not container:
            logger.warning("[KINOLAR] No container found, using full page")
            container = soup
        
        # Find cards
        cards = []
        for sel in self.CARD_SELECTORS:
            found = container.select(sel)
            if found:
                logger.info(f"[KINOLAR] Card selector '{sel}' = {len(found)} cards")
                cards = found
                break
        
        if not cards:
            # Fallback: find film links
            all_links = container.find_all("a", href=True)
            cards = [a for a in all_links if "/film/" in a.get("href", "")]
            logger.info(f"[KINOLAR] Fallback links: {len(cards)}")
        
        logger.info(f"[KINOLAR] Total cards: {len(cards)}")
        
        # Extract fields from each card
        results = []
        for card in cards:
            result = self._extract_card(card)
            if result and result.get("title") and result.get("link"):
                results.append(result)
        
        logger.info(f"[KINOLAR] Extracted {len(results)} cards with title+link")
        return results
    
    def _extract_card(self, card) -> Optional[Dict[str, Any]]:
        """Extract title, link, image from a single card"""
        
        # Find title and link
        title = ""
        link = ""
        
        # Try title selectors
        for sel in self.TITLE_SELECTORS:
            elem = card.select_one(sel)
            if elem:
                title = clean_text(elem.get_text())
                link = elem.get("href", "")
                if title and link:
                    break
        
        # Fallback: any film link
        if not title or not link:
            film_links = card.find_all("a", href=True)
            for a in film_links:
                href = a.get("href", "")
                if "/film/" in href or "/movie/" in href:
                    link = href
                    # Try title attribute first
                    title = a.get("title", "")
                    if not title:
                        title = a.get_text()
                    title = clean_text(title)
                    if title:
                        break
        
        if not title or not link:
            return None
        
        # Normalize URL
        link = normalize_url(link, self.BASE_URL)
        
        # Find image
        poster = ""
        for sel in self.IMAGE_SELECTORS:
            img = card.select_one(sel)
            if img:
                poster = img.get("data-src") or img.get("data-lazy-src") or img.get("data-original") or img.get("src", "")
                if poster:
                    break
        
        if poster:
            poster = normalize_url(poster, self.BASE_URL)
        
        # Find year
        year = None
        for sel in self.YEAR_SELECTORS:
            year_elem = card.select_one(sel)
            if year_elem:
                extracted = extract_year(year_elem.get_text())
                if extracted:
                    year = extracted
                    break
        
        return {
            "source": self.source_name,
            "title": title,
            "link": link,
            "img": poster,  # Use img field for consistency
            "year": year,
            "quality": ""
        }
    
    def get_details(self, url: str) -> Dict[str, Any]:
        """Get movie details"""
        result = {
            "title": "", "video_url": "", "video_type": "",
            "poster": "", "description": "", "year": None,
            "genres": [], "type": "movie"
        }
        
        try:
            response = self.session.get(url, timeout=30)
            soup = BeautifulSoup(response.text, "lxml")
            
            # Title
            title_elem = soup.select_one("h1, .title, .film-title")
            if title_elem:
                result["title"] = clean_text(title_elem.get_text())
            
            # Poster
            og = soup.select_one("meta[property='og:image']")
            if og:
                result["poster"] = normalize_url(og.get("content", ""), self.BASE_URL)
            
            # Description
            desc = soup.select_one(".description, .desc, .synopsis, .text")
            if desc:
                result["description"] = clean_text(desc.get_text())
            
            # Year
            year_elem = soup.select_one(".year, [class*='year'], .date")
            if year_elem:
                year = extract_year(year_elem.get_text())
                if year:
                    result["year"] = year
            
            # Genres
            genre_elems = soup.select(".genres a, .genre a, [class*='genre'] a")
            result["genres"] = [clean_text(g.get_text()) for g in genre_elems if g.get_text()]
            
            # Type (serial or movie)
            if "/serial/" in url:
                result["type"] = "serial"
            
            # Video (iframe embed)
            iframe = soup.select_one("iframe[src*='//']")
            if iframe:
                result["video_url"] = iframe.get("src", "")
                result["video_type"] = "iframe_embed"
            
        except Exception as e:
            logger.error(f"[KINOLAR] Details error: {e}")
        
        return result
