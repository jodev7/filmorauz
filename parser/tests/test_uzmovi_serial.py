import unittest
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from uzmovi_serial import UzmoviSerialParser


class FakeResponse:
    def __init__(self, text: str, status_code: int = 200, url: str = ""):
        self.text = text
        self.status_code = status_code
        self.url = url or "https://uzmovi.tv/mock"

    def raise_for_status(self):
        if self.status_code >= 400:
            raise RuntimeError(f"http {self.status_code}")


SERIAL_HTML = """
<html>
  <head>
    <meta property="og:title" content="Dexter barcha qismlari">
    <meta property="og:image" content="https://uzmovi.tv/poster.jpg">
    <meta property="og:description" content="Dexter serial description">
  </head>
  <body>
    <h1>Dexter barcha qismlari</h1>
    <div class="tab-pane fade in active" id="player0">
      <div class="batcoh-list">
        <a href="/tarjima-kinolarri/6954-dexter/episode/21358/1.html" class="batcoh-item" title="1-qism">1-qism</a>
        <a href="/tarjima-kinolarri/6954-dexter/episode/21358/2.html" class="batcoh-item" title="2-qism">2-qism</a>
      </div>
    </div>
    <script>
      var hiddenEpisodes = [
        "/tarjima-kinolarri/6954-dexter/episode/21358/3.html",
        "/tarjima-kinolarri/6954-dexter/episode/21358/4.html"
      ];
    </script>
  </body>
</html>
"""

EPISODE_HTML = """
<html><body><iframe src="https://uzdown.live/embed/{ep}?episode={ep}"></iframe></body></html>
"""

EMBED_HTML = """
<html><script>var player = {{ file: "https://cdn.uzdown.live/storage/6954/{ep}/123/index.m3u8" }};</script></html>
"""


class UzmoviSerialParserTests(unittest.TestCase):
    def test_collects_hidden_script_episode_urls_and_sorts_all_episodes(self):
        parser = UzmoviSerialParser()

        def fake_get(url, timeout=30, headers=None):
            if "episode/21358/" in url:
                ep = url.rsplit("/", 1)[-1].split(".")[0]
                return FakeResponse(EPISODE_HTML.format(ep=ep))
            if "uzdown.live/embed/" in url:
                ep = url.split("/embed/")[1].split("?")[0]
                return FakeResponse(EMBED_HTML.format(ep=ep))
            return FakeResponse(SERIAL_HTML)

        parser.session.get = fake_get

        result = parser.parse("https://uzmovi.tv/tarjima-kinolarri/6954-dexter.html")

        self.assertTrue(result["success"])
        self.assertEqual(len(result["episodes"]), 4)
        self.assertEqual([ep["episode"] for ep in result["episodes"]], [1, 2, 3, 4])
        self.assertEqual(len(result["seasons"]), 1)
        self.assertEqual(result["seasons"][0]["season_number"], 1)
        self.assertTrue(all(ep["video_url"].endswith("index.m3u8") for ep in result["episodes"]))

    def test_dedupes_same_episode_url_inventory(self):
        parser = UzmoviSerialParser()

        duplicate_html = """
        <html><body>
          <h1>Dexter</h1>
          <a href="/tarjima-kinolarri/6954-dexter/episode/21358/1.html">1-qism</a>
          <div data-url="/tarjima-kinolarri/6954-dexter/episode/21358/1.html">1-qism</div>
        </body></html>
        """

        def fake_get(url, timeout=30, headers=None):
            if "episode/21358/1.html" in url:
                return FakeResponse(EPISODE_HTML.format(ep=1))
            if "uzdown.live/embed/1" in url:
                return FakeResponse(EMBED_HTML.format(ep=1))
            return FakeResponse(duplicate_html)

        parser.session.get = fake_get

        result = parser.parse("https://uzmovi.tv/tarjima-kinolarri/6954-dexter.html")
        self.assertEqual(len(result["episodes"]), 1)
        self.assertEqual(result["episodes"][0]["episode"], 1)

    def test_detects_expected_range_and_fills_direct_episode_urls(self):
        parser = UzmoviSerialParser()
        progress_events = []

        range_html = """
        <html><body>
          <h1>Dexter 1-6 qism</h1>
          <a href="/tarjima-kinolarri/6954-dexter/episode/21358/1.html">1-qism</a>
          <a href="/tarjima-kinolarri/6954-dexter/episode/21358/2.html">2-qism</a>
          <script>
            var eps = ["/tarjima-kinolarri/6954-dexter/episode/21358/4.html"];
          </script>
          <div>Barcha 6 qism</div>
        </body></html>
        """

        def fake_get(url, timeout=30, headers=None):
            if "episode/21358/" in url:
                ep = url.rsplit("/", 1)[-1].split(".")[0]
                return FakeResponse(EPISODE_HTML.format(ep=ep), url=url)
            if "uzdown.live/embed/" in url:
                ep = url.split("/embed/")[1].split("?")[0]
                return FakeResponse(EMBED_HTML.format(ep=ep), url=url)
            return FakeResponse(range_html, url=url)

        def fake_head(url, timeout=10, allow_redirects=True, headers=None):
            if any(f"/episode/21358/{n}.html" in url for n in range(1, 7)):
                return FakeResponse("", 200, url=url)
            return FakeResponse("", 404, url=url)

        parser.session.get = fake_get
        parser.session.head = fake_head

        result = parser.parse(
            "https://uzmovi.tv/tarjima-kinolarri/6954-dexter.html",
            progress_callback=lambda event: progress_events.append(event),
        )

        self.assertTrue(result["success"])
        self.assertEqual([ep["episode"] for ep in result["episodes"]], [1, 2, 3, 4, 5, 6])
        self.assertEqual(result["missing_numbers"], [])
        self.assertTrue(any(event.get("stage") == "inventory_ready" for event in progress_events))
        self.assertTrue(any(event.get("stage") == "completed" for event in progress_events))


if __name__ == "__main__":
    unittest.main()
