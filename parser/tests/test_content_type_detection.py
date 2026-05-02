import unittest
from pathlib import Path
import sys
from unittest.mock import Mock

from bs4 import BeautifulSoup

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from asilmedia import AsilmediaParser
from helpers import detect_content_type


class FakeResponse:
    def __init__(self, text: str, status_code: int = 200):
        self.text = text
        self.status_code = status_code
        self.ok = 200 <= status_code < 300


ASILMEDIA_INTERSTELLAR_HTML = """
<div class="sidebar">
  <a href="/films/serial/" class="sidebar__sub-item">Seriallar</a>
</div>
<article class="fullstory" itemscope itemtype="https://schema.org/Movie">
  <div class="fs-poster">
    <img alt="Interstellar" src="/poster.webp" />
  </div>
  <h1 class="fs-title">Interstellar Uzbek tarjima 2014</h1>
  <div class="fs-meta"><span>Fantastika</span></div>
  <div class="fs-description">
    Fazoda omon qolish haqida kino.
  </div>
</article>
<div class="related">
  <a href="/films/serial/">Seriallar</a>
</div>
"""

ASILMEDIA_CHINAKAM_HTML = """
<article class="fullstory" itemscope itemtype="https://schema.org/TVSeries">
  <div class="fs-poster__serial-badge">
    <span class="badge badge--series">1-3 fasllar to'liq!</span>
  </div>
  <h1 class="fs-title">Chinakam izquvar Barcha qismlar</h1>
  <div class="fs-meta"><span>Seriallar</span></div>
  <div class="fs-episodes" id="episodes-section">
    <div class="fs-episodes__header">Qismlar</div>
    <div id="episodes-raw-data">
      <a href="https://cdn.example/episode1.mp4" data-label="1-qism 1080p">1</a>
      <a href="https://cdn.example/episode2.mp4" data-label="2-fasl 1-qism 1080p">2</a>
    </div>
  </div>
</article>
"""


class ContentTypeDetectionTests(unittest.TestCase):
    def test_asilmedia_interstellar_stays_movie(self):
        parser = AsilmediaParser()
        parser.session.get = Mock(return_value=FakeResponse(ASILMEDIA_INTERSTELLAR_HTML))

        content_type = parser._resolve_search_result_content_type(
            detail_url="https://asilmedia.org/9140-interstellar-uzbek-tarjima-2014-hd-ozbek-tilida-tas-ix-skachat.html",
            title="Interstellar Uzbek tarjima 2014",
            description="",
            query="interstellar",
        )

        self.assertEqual(content_type, "movie")

    def test_asilmedia_chinakam_izquvar_is_series(self):
        parser = AsilmediaParser()
        parser.session.get = Mock(return_value=FakeResponse(ASILMEDIA_CHINAKAM_HTML))

        content_type = parser._resolve_search_result_content_type(
            detail_url="https://asilmedia.org/16564-chinakam-izquvar-barcha-qismlar-ozbek-tilida-2014-uzbekcha-tarjima.html",
            title="Chinakam izquvar",
            description="",
            query="chinakam izquvar",
        )

        self.assertEqual(content_type, "serial")

    def test_uzmovi_dexter_is_series(self):
        content_type, _ = detect_content_type(
            "https://uzmovi.tv/seriallar/12345-dexter-uzbek-tilida.html",
            "uzmovi",
        )
        self.assertEqual(content_type, "serial")

    def test_normal_movie_stays_movie(self):
        soup = BeautifulSoup(
            """
            <article class="fullstory" itemscope itemtype="https://schema.org/Movie">
              <h1>Inception</h1>
              <div class="description">Aqlni bukuvchi kino.</div>
            </article>
            """,
            "lxml",
        )

        content_type, _ = detect_content_type(
            "https://asilmedia.org/999-inception.html",
            "asilmedia",
            soup=soup,
        )
        self.assertEqual(content_type, "movie")


if __name__ == "__main__":
    unittest.main()
