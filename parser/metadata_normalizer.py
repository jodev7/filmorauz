"""
Metadata Normalization Module
Provides functions to clean and normalize movie metadata extracted from parsers.
"""
import re
import logging
from typing import Any, Dict, List, Optional, Tuple
from urllib.parse import urljoin, urlparse

logger = logging.getLogger(__name__)

# Site-specific words to remove from titles
SITE_TITLE_PATTERNS = [
    r'uzmovi\.tv\b',
    r'freekino\.net\b',
    r'asilmedia\.org\b',
    r'kinolar\.uz\b',
    r'\[uzmovi\]',
    r'\[freekino\]',
    r'\[asilmedia\]',
    r'\[kinolar\]',
    r'смотреть\s+онлайн\s*',
    r'скачать\s+бесплатно\s*',
    r'online\s+free\s*',
    r'watch\s+online\s*',
]

# Common quality strings to normalize
QUALITY_MAPPINGS = {
    'hd': 'HD',
    '1080p': 'Full HD',
    '1080': 'Full HD',
    '720p': 'HD',
    '720': 'HD',
    '480p': 'SD',
    '480': 'SD',
    '2160p': '4K',
    '2160': '4K',
    '4k': '4K',
    'hdrip': 'HD',
    'dvdrip': 'DVD',
    'webrip': 'WEB',
    'web-dl': 'WEB',
    'bluray': 'BluRay',
    'blu-ray': 'BluRay',
}

# Common translations
TRANSLATION_PATTERNS = [
    (r'uzbek\s*tarjima', 'Uzbek'),
    (r'uzbek\s*tilida', 'Uzbek'),
    (r'русский\s*язык', 'Russian'),
    (r'русская\s*озвучка', 'Russian'),
    (r'russian\s*voice', 'Russian'),
    (r'english\s*subtitles', 'English'),
    (r'english\s*dubbed', 'English'),
    (r'turkce\s*altyazi', 'Turkish'),
    (r'türkçe\s*altyazı', 'Turkish'),
]


def normalize_title(title: str, source: str = "") -> str:
    """
    Normalize movie title by removing site-specific words and cleaning.
    
    Args:
        title: Raw title from parser
        source: Source name (e.g., 'uzmovi')
    
    Returns:
        Cleaned title string
    """
    if not title:
        return ""
    
    # Strip and lowercase for pattern matching
    cleaned = title.strip()
    
    # Remove site-specific patterns
    for pattern in SITE_TITLE_PATTERNS:
        cleaned = re.sub(pattern, ' ', cleaned, flags=re.IGNORECASE)
    
    # Remove extra whitespace
    cleaned = re.sub(r'\s+', ' ', cleaned)
    
    # Remove leading/trailing special characters but keep alphanumeric
    cleaned = cleaned.strip(" -_.:;|")
    
    # Capitalize first letter of each word (title case)
    if cleaned:
        cleaned = cleaned.title()
    
    logger.debug(f"[NORMALIZER] normalize_title: '{title}' -> '{cleaned}'")
    return cleaned


def clean_html_text(text: str) -> str:
    """
    Clean HTML from text - remove all tags and decode entities.
    
    Args:
        text: Raw text that may contain HTML
    
    Returns:
        Clean text without HTML
    """
    if not text:
        return ""
    
    # Remove HTML tags (replace <br/> with space to preserve word separation)
    cleaned = re.sub(r'<br\s*/?>', ' ', text)
    cleaned = re.sub(r'<[^>]+>', '', cleaned)
    
    # Decode common HTML entities
    entities = {
        '&nbsp;': ' ',
        '&': '&',
        '<': '<',
        '>': '>',
        '"': '"',
        ''': "'",
        ''': "'",
        '&mdash;': '—',
        '&ndash;': '–',
        '&hellip;': '...',
        '&copy;': '(c)',
        '&reg;': '(R)',
        '&trade;': '(TM)',
    }
    
    for entity, replacement in entities.items():
        cleaned = cleaned.replace(entity, replacement)
    
    # Remove extra whitespace and newlines
    cleaned = re.sub(r'\s+', ' ', cleaned)
    cleaned = re.sub(r'\n+', ' ', cleaned)
    
    # Strip leading/trailing whitespace
    cleaned = cleaned.strip()
    
    # Remove trailing punctuation that might be left over
    cleaned = cleaned.strip('.,;:!?-')
    
    logger.debug(f"[NORMALIZER] clean_html_text: {len(text)} chars -> {len(cleaned)} chars")
    return cleaned


def parse_year(value: Any) -> Optional[int]:
    """
    Parse year to integer, handling various formats.
    
    Args:
        value: Year as string, int, or other
    
    Returns:
        Year as int or None if invalid
    """
    if value is None:
        return None
    
    # Already an integer
    if isinstance(value, int):
        # Validate it's a reasonable year (1900-2100)
        if 1900 <= value <= 2100:
            return value
        logger.warning(f"[NORMALIZER] Invalid year value: {value}")
        return None
    
    # String or other
    if isinstance(value, str):
        value = value.strip()
        
        # Try direct conversion
        try:
            year = int(value)
            if 1900 <= year <= 2100:
                return year
        except ValueError:
            pass
        
        # Try extracting from text (e.g., "2021", "2022 yil")
        match = re.search(r'(19|20)\d{2}', value)
        if match:
            year = int(match.group())
            if 1900 <= year <= 2100:
                return year
    
    logger.warning(f"[NORMALIZER] Could not parse year from: {value}")
    return None


def parse_list(value: Any, item_type: str = "string") -> List:
    """
    Ensure value is a list, converting from string or single value if needed.
    
    Args:
        value: Could be string, list, or single item
        item_type: Type hint for logging
    
    Returns:
        List (possibly empty)
    """
    if value is None:
        return []
    
    # Already a list
    if isinstance(value, list):
        # Clean each item if strings
        if item_type == "string" and value and isinstance(value[0], str):
            return [str(v).strip() for v in value if v]
        return list(value)
    
    # String (comma-separated)
    if isinstance(value, str):
        if not value.strip():
            return []
        
        # Split by common separators
        items = re.split(r'[,;|/]', value)
        items = [item.strip() for item in items if item.strip()]
        
        logger.debug(f"[NORMALIZER] parse_list (from string): '{value}' -> {items}")
        return items
    
    # Single value (int, etc)
    if isinstance(value, (int, float)):
        if item_type == "year":
            parsed = parse_year(value)
            return [parsed] if parsed is not None else []
        return [value]
    
    # Try to convert
    try:
        return [str(value)]
    except:
        pass
    
    logger.warning(f"[NORMALIZER] Could not parse list from: {type(value).__name__}")
    return []


def absolutize_url(url: str, base_url: str = "") -> str:
    """
    Convert relative URL to absolute, or validate absolute URL.
    
    Args:
        url: URL to process (relative or absolute)
        base_url: Base URL for resolving relative URLs
    
    Returns:
        Absolute URL or empty string
    """
    if not url:
        return ""
    
    url = url.strip()
    
    # Already absolute
    if url.startswith(('http://', 'https://', '//')):
        # Handle protocol-relative URLs
        if url.startswith('//'):
            return 'https:' + url
        return url
    
    # Relative URL - join with base
    if base_url:
        return urljoin(base_url, url)
    
    # No base URL, return as-is (log warning)
    logger.warning(f"[NORMALIZER] Relative URL without base: {url}")
    return url


def parse_duration(value: Any) -> Optional[int]:
    """
    Parse duration to integer minutes.
    
    Args:
        value: Duration as string, int, or other
    
    Returns:
        Duration in minutes or None
    """
    if value is None:
        return None
    
    # Already an integer (assume it's minutes)
    if isinstance(value, int):
        if 0 < value <= 600:  # Reasonable range (0-10 hours)
            return value
        return None
    
    # String
    if isinstance(value, str):
        value = value.strip()
        
        # Try direct int conversion
        try:
            minutes = int(value)
            if 0 < minutes <= 600:
                return minutes
        except ValueError:
            pass
        
        # Pattern: "136 min", "2h 30m", "2:30:00", etc.
        # Order matters: check more specific patterns first
        patterns = [
            r'(\d+)\s*h\s*(?:(\d+)\s*m)?\s*',  # "2h 30m" or "2h" FIRST
            r'(\d+):(\d+):(\d+)',  # "02:30:00" (HH:MM:SS)
            r'(\d+):(\d+)',  # "2:30" (H:MM or MM:SS)
            r'(\d+)\s*(?:min|m|dakika|minutes?|мин|daqiqa)\s*',  # "136 min" LAST
        ]
        
        for pattern in patterns:
            match = re.search(pattern, value, re.IGNORECASE)
            if match:
                groups = match.groups()
                
                if len(groups) == 1:
                    return int(groups[0])
                elif len(groups) == 2:
                    # "2h 30m" or "2:30"
                    try:
                        hours = int(groups[0]) if groups[0] else 0
                        minutes = int(groups[1]) if groups[1] else 0
                        return hours * 60 + minutes
                    except:
                        pass
                elif len(groups) == 3:
                    # "2:30:00"
                    try:
                        hours = int(groups[0])
                        minutes = int(groups[1])
                        return hours * 60 + minutes
                    except:
                        pass
    
    logger.warning(f"[NORMALIZER] Could not parse duration from: {value}")
    return None


def normalize_quality(value: str) -> str:
    """
    Normalize quality string to standard values.
    
    Args:
        value: Raw quality string
    
    Returns:
        Normalized quality (HD, Full HD, SD, 4K, etc.)
    """
    if not value:
        return ""
    
    value = value.strip().lower()
    
    # Check mappings
    for pattern, normalized in QUALITY_MAPPINGS.items():
        if pattern in value:
            return normalized
    
    # Return capitalized if no mapping
    return value.title()


def parse_translation(text: str) -> str:
    """
    Extract translation/language info from text.
    
    Args:
        text: Text that might contain translation info
    
    Returns:
        Translation string (e.g., "Uzbek", "Russian") or empty
    """
    if not text:
        return ""
    
    text_lower = text.lower()
    
    for pattern, translation in TRANSLATION_PATTERNS:
        if re.search(pattern, text_lower, re.IGNORECASE):
            return translation
    
    return ""


def normalize_countries(value: Any) -> List[str]:
    """
    Normalize countries to a list.
    
    Args:
        value: Countries as string, list, or single value
    
    Returns:
        List of country names
    """
    countries = parse_list(value, "string")
    
    # Clean each country name
    cleaned = []
    for country in countries:
        # Remove common prefixes/suffixes
        country = re.sub(r'^(to|g\.?)\s+', '', country, flags=re.IGNORECASE)
        country = country.strip()
        
        if country:
            cleaned.append(country)
    
    return cleaned


def normalize_metadata(raw_metadata: Dict[str, Any], source: str, base_url: str = "") -> Dict[str, Any]:
    """
    Normalize all metadata fields.
    
    Args:
        raw_metadata: Raw metadata from parser
        source: Source name
        base_url: Base URL for relative URLs
    
    Returns:
        Normalized metadata dictionary
    """
    logger.info(f"[NORMALIZER] Starting normalization for source: {source}")
    logger.debug(f"[NORMALIZER] Raw metadata: {raw_metadata}")
    
    normalized = {}
    
    # Title
    title = raw_metadata.get('title', '')
    original_title = raw_metadata.get('original_title', '')
    
    # If no original_title, use title as original
    if not original_title and title:
        original_title = title
    
    # Normalize title (use original_title if available for cleaner version)
    normalized_title = normalize_title(original_title or title, source)
    
    # If title still has issues, try the other field
    if not normalized_title:
        normalized_title = normalize_title(title, source)
    
    normalized['title'] = normalized_title or "Unknown"
    normalized['original_title'] = original_title if original_title != title else ""
    
    # Description
    raw_desc = raw_metadata.get('description', '')
    normalized['description'] = clean_html_text(raw_desc)
    
    # Year
    year = raw_metadata.get('year')
    normalized['year'] = parse_year(year)
    
    # Genres
    genres = raw_metadata.get('genres', [])
    normalized['genres'] = parse_list(genres, "string")
    
    # Countries
    country = raw_metadata.get('country', raw_metadata.get('countries', []))
    normalized['countries'] = normalize_countries(country)
    
    # Poster URL
    poster = raw_metadata.get('poster', '')
    normalized['poster_url'] = absolutize_url(poster, base_url)
    
    # Duration (use duration_minutes as key for clarity)
    duration = raw_metadata.get('duration', raw_metadata.get('duration_minutes'))
    normalized['duration_minutes'] = parse_duration(duration)
    
    # Quality
    quality = raw_metadata.get('quality', '')
    normalized['quality'] = normalize_quality(quality)
    
    # Translation - look in various fields
    translation = ""
    translation_fields = ['translation', 'lang', 'language', 'audio']
    for field in translation_fields:
        if field in raw_metadata:
            translation = parse_translation(str(raw_metadata[field]))
            if translation:
                break
    
    # Also check description for translation hints
    if not translation:
        translation = parse_translation(normalized['description'])
    
    normalized['translation'] = translation
    
    logger.info(f"[NORMALIZER] Normalized metadata: {normalized}")
    return normalized


def validate_metadata(metadata: Dict[str, Any]) -> Tuple[bool, List[str]]:
    """
    Validate normalized metadata.
    
    Args:
        metadata: Normalized metadata dictionary
    
    Returns:
        Tuple of (is_valid, list_of_warnings)
    """
    warnings = []
    
    # Title must be non-empty
    if not metadata.get('title'):
        warnings.append("Title is missing or empty")
    
    # Year should be int or null
    year = metadata.get('year')
    if year is not None and not isinstance(year, int):
        warnings.append(f"Year should be int or null, got {type(year).__name__}")
    
    # Genres should always be arrays
    genres = metadata.get('genres')
    if genres is None or not isinstance(genres, list):
        warnings.append(f"Genres should be list, got {type(genres).__name__}")
    
    # Countries should always be arrays
    countries = metadata.get('countries')
    if countries is None or not isinstance(countries, list):
        warnings.append(f"Countries should be list, got {type(countries).__name__}")
    
    # Poster URL should be absolute if present
    poster_url = metadata.get('poster_url')
    if poster_url and not poster_url.startswith(('http://', 'https://', '//')):
        warnings.append(f"Poster URL should be absolute: {poster_url}")
    
    is_valid = len(warnings) == 0
    
    logger.info(f"[NORMALIZER] Validation result: valid={is_valid}, warnings={warnings}")
    return is_valid, warnings


def create_worker_payload(
    source: str,
    source_url: str,
    page_url: str,
    video_url: Optional[str],
    video_url_type: str,
    metadata: Dict[str, Any],
    local_path: str = ""
) -> Dict[str, Any]:
    """
    Create the final payload for the worker.
    
    This function creates a structured output that includes both:
    1. Flattened fields for backward compatibility with existing worker
    2. Nested 'metadata' object for clean API design
    
    Args:
        source: Source name (e.g., 'uzmovi')
        source_url: Source base URL
        page_url: Detail page URL
        video_url: Video/playback URL
        video_url_type: Type of video URL (mp4, m3u8, missing)
        metadata: Normalized metadata dict
        local_path: Local file path if downloaded
    
    Returns:
        Structured payload for worker (flattened + nested for compatibility)
    """
    # Get metadata values
    title = metadata.get('title', 'Unknown')
    description = metadata.get('description', '')
    poster_url = metadata.get('poster_url', '')
    year = metadata.get('year')
    genres = metadata.get('genres', [])
    countries = metadata.get('countries', [])
    # Use first country for backward compatibility (worker uses 'Country' field)
    country = countries[0] if countries else ''
    duration = metadata.get('duration_minutes')
    quality = metadata.get('quality', '')
    translation = metadata.get('translation', '')
    original_title = metadata.get('original_title', '')
    
    # Build the payload - flatten top-level fields for worker backward compatibility
    # Worker expects: Title, Description, Poster, Year, Genres, Country, Duration, VideoPageURL
    # But we also include the structured 'metadata' object
    payload = {
        # Flattened fields for worker backward compatibility
        "title": title,
        "description": description,
        "poster": poster_url,
        "backdrop": "",
        "year": year if year else 0,
        "genres": genres,
        "country": country,
        "duration": duration if duration else 0,
        "video_page_url": page_url,
        
        # Additional flattened fields for quality/translation
        "quality": quality,
        "translation": translation,
        "original_title": original_title,
        
        # Source tracking
        "source": source,
        "source_url": source_url,
        "page_url": page_url,
        
        # Video info
        "video_url": video_url if video_url else "",
        "video_url_type": video_url_type,
        
        # Nested metadata object for clean API design
        "metadata": {
            "title": title,
            "original_title": original_title,
            "description": description,
            "year": year,
            "genres": genres,
            "countries": countries,
            "poster_url": poster_url,
            "duration_minutes": duration,
            "quality": quality,
            "translation": translation,
        }
    }
    
    # Add local_path if present
    if local_path:
        payload["local_path"] = local_path
    
    logger.info(f"[NORMALIZER] Created worker payload: source={source}, title={title}")
    
    return payload