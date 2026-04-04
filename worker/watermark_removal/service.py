#!/usr/bin/env python3
"""
Watermark Removal HTTP Service

Provides HTTP API for the Go worker to call watermark removal.
This allows the watermark removal to be performed by a separate Python service.

Endpoints:
- POST /remove - Remove watermark from video
- POST /detect - Detect watermark only (no removal)
- GET /health - Health check

Usage:
    python -m watermark_removal.service
    
Environment Variables:
    WATERMARK_PORT: HTTP server port (default: 8084)
    WATERMARK_HOST: HTTP server host (default: 0.0.0.0)
    (plus all WATERMARK_* config variables)
"""

import os
import sys
import json
import logging
import argparse
from http.server import HTTPServer, BaseHTTPRequestHandler
from urllib.parse import urlparse, parse_qs

from . import (
    WatermarkRemovalPipeline, 
    WatermarkConfig, 
    load_config,
    detect_watermark
)


# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s"
)
logger = logging.getLogger(__name__)


class WatermarkServiceHandler(BaseHTTPRequestHandler):
    """HTTP request handler for watermark removal service."""
    
    # Store config for access in handlers
    _config: WatermarkConfig = None
    _pipeline = None
    
    def _send_json(self, data, status=200):
        """Send JSON response."""
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps(data).encode())
    
    def _send_error(self, message, status=400):
        """Send error response."""
        self._send_json({"error": message}, status)
    
    def _read_json_body(self):
        """Read and parse JSON body."""
        content_length = int(self.headers.get("Content-Length", 0))
        if content_length == 0:
            return {}
        body = self.rfile.read(content_length)
        return json.loads(body.decode("utf-8"))
    
    def _get_pipeline(self):
        """Get or create the pipeline instance."""
        if self._pipeline is None:
            self._pipeline = WatermarkRemovalPipeline(config=self._config)
        return self._pipeline
    
    def do_GET(self):
        """Handle GET requests."""
        parsed = urlparse(self.path)
        path = parsed.path
        
        # Health check
        if path == "/health":
            self._send_json({
                "status": "ok",
                "service": "watermark-removal",
                "config": {
                    "enabled": self._config.enabled,
                    "mode": self._config.mode,
                    "sample_count": self._config.sample_count,
                    "ocr_enabled": self._config.ocr_enabled,
                }
            })
            return
        
        # Detect endpoint
        elif path == "/detect":
            query = parse_qs(parsed.query)
            video_path = query.get("path", [None])[0]
            
            if not video_path:
                self._send_error("Missing 'path' parameter")
                return
            
            if not os.path.exists(video_path):
                self._send_error(f"Video not found: {video_path}")
                return
            
            try:
                result = detect_watermark(video_path, self._config)
                self._send_json(result.to_dict())
            except Exception as e:
                logger.error(f"Detection failed: {e}", exc_info=True)
                self._send_error(f"Detection failed: {str(e)}", 500)
            return
        
        # Default: 404
        self._send_error("Not found", 404)
    
    def do_POST(self):
        """Handle POST requests."""
        parsed = urlparse(self.path)
        path = parsed.path
        
        # Remove watermark endpoint
        if path == "/remove":
            body = self._read_json_body()
            
            input_path = body.get("input_path") or body.get("input")
            output_path = body.get("output_path") or body.get("output")
            
            if not input_path:
                self._send_error("Missing 'input_path' in request body")
                return
            
            if not os.path.exists(input_path):
                self._send_error(f"Video not found: {input_path}")
                return
            
            logger.info(f"Watermark removal request: {input_path}")
            
            try:
                pipeline = self._get_pipeline()
                result = pipeline.process(input_path, output_path)
                
                logger.info(f"Watermark removal complete: success={result.success}")
                
                self._send_json(result.to_dict())
                
            except Exception as e:
                logger.error(f"Removal failed: {e}", exc_info=True)
                self._send_error(f"Removal failed: {str(e)}", 500)
            return
        
        # Default: 404
        self._send_error("Not found", 404)
    
    def log_message(self, format, *args):
        """Custom log message format."""
        logger.info(f"[HTTP] {args[0]}")


def run_server(host: str = "0.0.0.0", port: int = 8084):
    """
    Run the watermark removal HTTP server.
    
    Args:
        host: Server host
        port: Server port
    """
    # Load configuration
    config = load_config()
    WatermarkServiceHandler._config = config
    
    server_address = (host, port)
    httpd = HTTPServer(server_address, WatermarkServiceHandler)
    
    logger.info(f"Starting Watermark Removal Service on {host}:{port}")
    logger.info(f"Configuration: enabled={config.enabled}, mode={config.mode}")
    
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        logger.info("Shutting down server...")
        httpd.shutdown()


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(description="Watermark Removal HTTP Service")
    parser.add_argument("--host", default=os.environ.get("WATERMARK_HOST", "0.0.0.0"),
                        help="Server host (default: 0.0.0.0)")
    parser.add_argument("--port", type=int, default=int(os.environ.get("WATERMARK_PORT", "8084")),
                        help="Server port (default: 8084)")
    
    args = parser.parse_args()
    run_server(args.host, args.port)


if __name__ == "__main__":
    main()
