import unittest
from pathlib import Path
import sys

from bs4 import BeautifulSoup

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from asilmedia import AsilmediaParser
from freekino import FreekinoParser
from helpers import detect_content_type, sort_video_candidates
from uzmovi_serial import UzmoviSerialParser


class FakeResponse:
    def __init__(self, text: str, status_code: int = 200, url: str = ""):
        self.text = text
        self.status_code = status_code
        self.url = url or "https://example.com/mock"
        self.content = text.encode("utf-8")

    def raise_for_status(self):
        if self.status_code >= 400:
            raise RuntimeError(f"http {self.status_code}")


class IngestionStabilizationTests(unittest.TestCase):
    def test_quality_sort_prefers_1080p(self):
        candidates = sort_video_candidates([
            {"url": "https://cdn.example.com/movie-720.mp4", "quality": "720p", "type": "mp4"},
            {"url": "https://cdn.example.com/movie-1080.mp4", "quality": "FHD", "type": "mp4"},
            {"url": "https://cdn.example.com/movie-480.mp4", "quality": "480p", "type": "mp4"},
        ])
        self.assertEqual(candidates[0]["quality"], "1080p")

    def test_asilmedia_detects_series(self):
        html = """
        <article class="fullstory" itemtype="https://schema.org/TVSeries">
          <h1>Chinakam izquvar barcha qismlar</h1>
          <div id="episodes-raw-data">
            <a data-label="1-qism 1080p">1</a>
            <a data-label="2-fasl">2</a>
          </div>
        </article>
        """
        soup = BeautifulSoup(html, "lxml")
        kind, reason = detect_content_type("https://asilmedia.org/16564-chinakam-izquvar.html", "asilmedia", soup=soup)
        self.assertEqual(kind, "serial")
        self.assertTrue(reason)

    def test_asilmedia_detects_movie(self):
        html = """
        <article class="fullstory" itemtype="https://schema.org/Movie">
          <h1>Interstellar Uzbek tilida</h1>
          <div class="download-buttons">
            <a href="https://cdn.example.com/interstellar-1080.mp4">1080p</a>
          </div>
        </article>
        """
        soup = BeautifulSoup(html, "lxml")
        kind, _ = detect_content_type("https://asilmedia.org/9140-interstellar.html", "asilmedia", soup=soup)
        self.assertEqual(kind, "movie")

    def test_uzmovi_dexter_can_expand_to_90_episodes(self):
        import os
        os.environ["PYTEST_FORCE_GAP_FILL"] = "1"
        try:
            parser = UzmoviSerialParser()
            serial_html = """
            <html><body>
              <h1>Dexter 1-90 qism</h1>
              <a href="/tarjima-kinolarri/6954-dexter/episode/21358/1.html">1-qism</a>
              <a href="/tarjima-kinolarri/6954-dexter/episode/21358/2.html">2-qism</a>
              <div>Barcha 90 qism</div>
            </body></html>
            """
            episode_html = "<html><body><iframe src='https://uzdown.live/embed/{ep}'></iframe></body></html>"
            embed_html = "<html><script>var player = {{ file: 'https://cdn.example.com/{ep}/index.m3u8' }};</script></html>"

            def fake_get(url, timeout=30, headers=None):
                if "episode/21358/" in url:
                    ep = url.rsplit("/", 1)[-1].split(".")[0]
                    return FakeResponse(episode_html.format(ep=ep), url=url)
                if "uzdown.live/embed/" in url:
                    ep = url.rsplit("/", 1)[-1]
                    return FakeResponse(embed_html.format(ep=ep), url=url)
                return FakeResponse(serial_html, url=url)

            def fake_head(url, timeout=10, allow_redirects=True, headers=None):
                return FakeResponse("", 200 if "/episode/21358/" in url else 404, url=url)

            parser.session.get = fake_get
            parser.session.head = fake_head

            result = parser.parse("https://uzmovi.tv/tarjima-kinolarri/6954-dexter.html")
            self.assertEqual(len(result["episodes"]), 90)
            self.assertEqual(result["missing_numbers"], [])
        finally:
            del os.environ["PYTEST_FORCE_GAP_FILL"]

    def test_freekino_search_returns_results(self):
        parser = FreekinoParser()
        search_html = """
        <html><body>
          <div id="tab-movies">
            <article itemtype="https://schema.org/Movie">
              <h2><a href="/movie/123-interstellar">Interstellar</a></h2>
              <img src="https://freekino.net/poster.jpg">
              <meta itemprop="datePublished" content="2014-01-01">
            </article>
          </div>
        </body></html>
        """
        parser.session.get = lambda url, params=None, timeout=30, allow_redirects=True, headers=None: FakeResponse(search_html, url=url)
        results = parser.search("Interstellar")
        self.assertTrue(results)
        first = results[0]
        source_id = first["source_id"] if isinstance(first, dict) else first.source_id
        self.assertEqual(source_id, "123")


if __name__ == "__main__":
    unittest.main()
