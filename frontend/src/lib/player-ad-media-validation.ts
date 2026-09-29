export const PLAYER_AD_MAX_SECONDS = 65;

// Browsers expose audio tracks through different media APIs. A missing API is
// inconclusive; a supported API returning zero tracks means a silent upload.
function hasAudioTrack(video: HTMLVideoElement): boolean | null {
  const media = video as HTMLVideoElement & {
    audioTracks?: { length: number };
    mozHasAudio?: boolean;
    captureStream?: () => MediaStream;
  };
  if (media.audioTracks) return media.audioTracks.length > 0;
  if (typeof media.mozHasAudio === "boolean") return media.mozHasAudio;
  if (media.captureStream) {
    try {
      return media.captureStream().getAudioTracks().length > 0;
    } catch {
      return null;
    }
  }
  return null;
}

/** Check the local video before uploading an in-player ad. */
export function validatePlayerAdVideo(file: File): Promise<string | null> {
  return new Promise((resolve) => {
    const url = URL.createObjectURL(file);
    const video = document.createElement("video");
    video.preload = "metadata";
    video.onloadedmetadata = () => {
      const duration = video.duration;
      const audio = hasAudioTrack(video);
      video.removeAttribute("src");
      URL.revokeObjectURL(url);
      if (!Number.isFinite(duration) || duration < 1 || duration > PLAYER_AD_MAX_SECONDS) {
        resolve(`Video ${PLAYER_AD_MAX_SECONDS} soniyadan oshmasin (joriy: ${Math.round(duration)}s)`);
      } else if (audio === false) {
        resolve("Videoda ovoz treki yo'q. Ovozli video yuklang.");
      } else {
        resolve(null);
      }
    };
    video.onerror = () => {
      video.removeAttribute("src");
      URL.revokeObjectURL(url);
      resolve("Video brauzerda ochilmadi. MP4 yoki WebM formatini yuklang.");
    };
    video.src = url;
  });
}
