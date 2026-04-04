"""
Binary Manager for N_m3u8DL-RE

This module ensures the N_m3u8DL-RE binary exists in parser/bin directory.
If missing, it downloads the latest release from GitHub, extracts it,
and makes it executable.

The downloader service cannot run without this binary.
"""
import os
import sys
import subprocess
import logging
import platform
import tarfile
import zipfile
import shutil
import requests

logger = logging.getLogger(__name__)

# Binary configuration
BIN_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'bin')
BINARY_NAME = 'N_m3u8DL-RE'
GITHUB_REPO = 'nilaoda/N_m3u8DL-RE'

# Platform-specific binary names
if platform.system() == 'Windows':
    BINARY_NAME_FULL = f'{BINARY_NAME}.exe'
elif platform.system() == 'Darwin':  # macOS
    BINARY_NAME_FULL = BINARY_NAME
else:  # Linux and others
    BINARY_NAME_FULL = BINARY_NAME


def get_binary_path() -> str:
    """Get the full path to the N_m3u8DL-RE binary"""
    return os.path.join(BIN_DIR, BINARY_NAME_FULL)


def ensure_bin_dir():
    """Ensure the bin directory exists"""
    os.makedirs(BIN_DIR, exist_ok=True)


def binary_exists() -> bool:
    """Check if N_m3u8DL-RE binary exists"""
    binary_path = get_binary_path()
    exists = os.path.isfile(binary_path)
    if exists:
        # Also check if it's executable
        if not os.access(binary_path, os.X_OK):
            logger.warning(f"Binary exists but is not executable: {binary_path}")
            return False
    return exists


def get_platform_string() -> str:
    """Get the platform string for downloading the correct binary"""
    system = platform.system()
    machine = platform.machine().lower()
    
    # Normalize machine names
    if machine in ('x86_64', 'amd64'):
        arch = 'x64'
    elif machine in ('aarch64', 'arm64'):
        arch = 'arm64'
    elif machine == 'armv7l':
        arch = 'armv7'
    elif machine == 'i386' or machine.startswith('i686'):
        arch = 'x86'
    else:
        arch = machine
    
    if system == 'Windows':
        return f'Windows-{arch}'
    elif system == 'Darwin':
        return f'MacOS-{arch}'
    else:  # Linux
        return f'Linux-{arch}'


def get_latest_release_url() -> tuple[str, str]:
    """
    Get the download URL for the latest N_m3u8DL-RE release.
    Returns (download_url, extension) tuple.
    """
    api_url = f'https://api.github.com/repos/{GITHUB_REPO}/releases/latest'
    
    try:
        logger.info(f"Fetching latest release info from GitHub API: {api_url}")
        response = requests.get(api_url, timeout=30)
        response.raise_for_status()
        release_data = response.json()
        
        platform_str = get_platform_string()
        logger.info(f"Target platform: {platform_str}")
        
        # Find the correct asset for our platform
        for asset in release_data.get('assets', []):
            asset_name = asset['name'].lower()
            browser_download_url = asset['browser_download_url']
            
            if platform_str.lower() in asset_name:
                # Determine archive extension
                if browser_download_url.endswith('.zip'):
                    ext = '.zip'
                elif browser_download_url.endswith('.tar.gz'):
                    ext = '.tar.gz'
                else:
                    ext = ''
                
                logger.info(f"Found matching asset: {asset['name']}")
                return browser_download_url, ext
        
        # If no specific platform match, try to find any Linux binary
        logger.warning(f"No binary found for platform {platform_str}, trying Linux-x64 fallback")
        for asset in release_data.get('assets', []):
            asset_name = asset['name'].lower()
            browser_download_url = asset['browser_download_url']
            
            if 'linux' in asset_name and 'x64' in asset_name:
                if browser_download_url.endswith('.tar.gz'):
                    ext = '.tar.gz'
                else:
                    ext = ''
                return browser_download_url, ext
        
        raise RuntimeError(f"No suitable binary found for platform {platform_str}")
        
    except requests.exceptions.RequestException as e:
        raise RuntimeError(f"Failed to fetch latest release info: {e}")


def download_binary(download_url: str, extension: str) -> str:
    """
    Download the binary from the given URL and extract it.
    Returns the path to the extracted binary.
    """
    logger.info(f"Downloading N_m3u8DL-RE from: {download_url}")
    
    # Create temp directory for download
    temp_dir = os.path.join(BIN_DIR, '_temp')
    os.makedirs(temp_dir, exist_ok=True)
    
    try:
        # Download the file
        temp_file = os.path.join(temp_dir, f'binary{extension}')
        response = requests.get(download_url, stream=True, timeout=120)
        response.raise_for_status()
        
        total_size = int(response.headers.get('content-length', 0))
        downloaded = 0
        
        with open(temp_file, 'wb') as f:
            for chunk in response.iter_content(chunk_size=8192):
                if chunk:
                    f.write(chunk)
                    downloaded += len(chunk)
                    if total_size > 0:
                        progress = (downloaded / total_size) * 100
                        if downloaded % (1024 * 1024) == 0:  # Log every MB
                            logger.info(f"Download progress: {progress:.1f}%")
        
        logger.info(f"Download complete: {temp_file}")
        
        # Extract the archive
        binary_path = extract_archive(temp_file, extension)
        
        return binary_path
        
    finally:
        # Cleanup temp directory
        if os.path.exists(temp_dir):
            shutil.rmtree(temp_dir, ignore_errors=True)


def extract_archive(archive_path: str, extension: str) -> str:
    """
    Extract the archive and return the path to the N_m3u8DL-RE binary.
    """
    logger.info(f"Extracting archive: {archive_path}")
    
    if extension == '.zip':
        with zipfile.ZipFile(archive_path, 'r') as zip_ref:
            # List contents first
            names = zip_ref.namelist()
            logger.info(f"Archive contains {len(names)} files: {names[:5]}...")
            
            # Extract all
            zip_ref.extractall(BIN_DIR)
            
            # Find the binary in extracted files
            for name in names:
                if BINARY_NAME.lower() in name.lower() and os.path.isfile(os.path.join(BIN_DIR, name)):
                    extracted_path = os.path.join(BIN_DIR, name)
                    logger.info(f"Found binary: {extracted_path}")
                    return extracted_path
                    
    elif extension == '.tar.gz' or extension == '.tgz':
        with tarfile.open(archive_path, 'r:gz') as tar_ref:
            # List contents first
            members = tar_ref.getmembers()
            logger.info(f"Archive contains {len(members)} files")
            
            # Extract all
            tar_ref.extractall(BIN_DIR)
            
            # Find the binary in extracted files
            for member in members:
                if BINARY_NAME.lower() in member.name.lower() and member.isfile():
                    extracted_path = os.path.join(BIN_DIR, member.name)
                    logger.info(f"Found binary: {extracted_path}")
                    return extracted_path
    
    raise RuntimeError(f"Failed to extract binary from {archive_path}")


def make_executable(binary_path: str):
    """Make the binary executable"""
    os.chmod(binary_path, 0o755)
    logger.info(f"Made executable: {binary_path}")


def verify_binary(binary_path: str) -> bool:
    """
    Verify the binary works by running --help.
    Returns True if verification succeeds.
    """
    try:
        logger.info(f"Verifying binary: {binary_path}")
        result = subprocess.run(
            [binary_path, '--help'],
            capture_output=True,
            text=True,
            timeout=30
        )
        
        # Check if --help was recognized (return code 0 or output contains help text)
        if result.returncode == 0 or 'N_m3u8DL' in result.stdout or 'M3U8' in result.stdout:
            logger.info("Binary verification successful!")
            logger.info(f"Binary version/help output:\n{result.stdout[:500]}")
            return True
        else:
            logger.warning(f"Binary --help returned non-zero: {result.returncode}")
            logger.warning(f"stderr: {result.stderr[:500]}")
            return False
            
    except subprocess.TimeoutExpired:
        logger.error("Binary verification timed out")
        return False
    except Exception as e:
        logger.error(f"Binary verification failed: {e}")
        return False


def ensure_binary_exists() -> str:
    """
    Ensure N_m3u8DL-RE binary exists in parser/bin.
    
    If binary is missing:
    - Downloads latest release from GitHub
    - Extracts it
    - Makes it executable
    - Verifies by running --help
    
    If binary is still missing after all attempts, raises RuntimeError.
    
    Returns the path to the binary.
    """
    ensure_bin_dir()
    
    binary_path = get_binary_path()
    
    # Check if binary already exists
    if binary_exists():
        logger.info(f"N_m3u8DL-RE binary already exists: {binary_path}")
        if verify_binary(binary_path):
            return binary_path
        else:
            logger.warning("Binary exists but verification failed, will try to re-download")
    
    # Binary is missing or invalid, download it
    logger.info("N_m3u8DL-RE binary not found or invalid, downloading latest release...")
    
    try:
        download_url, extension = get_latest_release_url()
        extracted_path = download_binary(download_url, extension)
        
        # If the extracted path differs from the expected path, we might need to rename/move
        if extracted_path != binary_path:
            # Move/rename to expected location
            if os.path.exists(binary_path):
                os.remove(binary_path)
            shutil.move(extracted_path, binary_path)
            extracted_path = binary_path
        
        # Make executable
        make_executable(extracted_path)
        
        # Verify
        if verify_binary(extracted_path):
            logger.info(f"Successfully installed N_m3u8DL-RE: {extracted_path}")
            return extracted_path
        else:
            raise RuntimeError("Binary verification failed after installation")
            
    except Exception as e:
        error_msg = (
            f"FAILED TO INSTALL N_m3u8DL-RE BINARY\n"
            f"{'='*50}\n"
            f"The N_m3u8DL-RE binary is required for the downloader to work.\n"
            f"Failed to download/extract binary: {e}\n\n"
            f"Please either:\n"
            f"1. Manually download N_m3u8DL-RE from: https://github.com/{GITHUB_REPO}/releases\n"
            f"2. Place the binary in: {BIN_DIR}\n"
            f"3. Make it executable: chmod +x {BINARY_NAME_FULL}\n\n"
            f"The parser downloader cannot function without this binary."
        )
        raise RuntimeError(error_msg)


def require_binary() -> str:
    """
    Require that the N_m3u8DL-RE binary exists.
    This is the main entry point that should be called at startup.
    
    Raises:
        SystemExit: If the binary cannot be found/installed.
    
    Returns:
        str: Path to the verified binary.
    """
    try:
        binary_path = ensure_binary_exists()
        logger.info(f"[BINARY_MANAGER] N_m3u8DL-RE binary ready at: {binary_path}")
        return binary_path
    except RuntimeError as e:
        logger.error(str(e))
        sys.stderr.write(f"\n{'!'*60}\n")
        sys.stderr.write("FATAL ERROR: N_m3u8DL-RE binary is missing!\n")
        sys.stderr.write(f"{'!'*60}\n")
        sys.stderr.write(str(e))
        sys.stderr.write(f"\n{'!'*60}\n")
        sys.stderr.write("The downloader cannot run without the N_m3u8DL-RE binary.\n")
        sys.stderr.write("Parser startup aborted.\n")
        sys.stderr.write(f"{'!'*60}\n\n")
        sys.exit(1)


if __name__ == '__main__':
    # Allow running this module directly to install the binary
    logging.basicConfig(level=logging.INFO)
    require_binary()
