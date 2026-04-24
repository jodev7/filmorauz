let cachedB2Auth = null;

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    if (!url.pathname.startsWith("/media/")) {
      return new Response("not found", { status: 404 });
    }

    const extractedMediaPath = extractMediaPath(url.pathname);
    const mediaPath = normalizeMediaPath(extractedMediaPath);
    const isImagePath = mediaPath.startsWith("/images/");
    const debugMode = isImagePath && url.searchParams.get("debug") === "1";
    const normalizedWorkerMediaPath = mediaPath.replace(/^\/+/, "");

    let token = null;
    if (!isImagePath) {
      token = readToken(request, url, env.MEDIA_COOKIE_NAME || "filmorauz_media");
      if (!token?.value) {
        return new Response("missing media token", { status: 403 });
      }

      const claims = decodeToken(token.value);
      if (!claims) {
        return new Response("invalid media token", { status: 403 });
      }

      if (Math.floor(Date.now() / 1000) > Number(claims.exp)) {
        return new Response("expired media token", { status: 403 });
      }

      const expectedSig = await sign(
        env.MEDIA_SIGNING_SECRET,
        claims.scope,
        claims.exp,
        claims.ip || "",
        claims.uaHash || ""
      );
      if (expectedSig !== claims.sig) {
        return new Response("bad media signature", { status: 403 });
      }

      if (!matchesMediaScope(normalizedWorkerMediaPath, claims.scope || "")) {
        return new Response("scope mismatch", { status: 403 });
      }

      if (claims.ip) {
        const requestIP = getRequestIP(request);
        if (!requestIP || requestIP !== claims.ip) {
          return new Response("ip mismatch", { status: 403 });
        }
      }

      if (claims.uaHash) {
        const userAgent = request.headers.get("user-agent") || "";
        const requestUAHash = await sha256Hex(userAgent);
        if (requestUAHash !== claims.uaHash) {
          return new Response("user-agent mismatch", { status: 403 });
        }
      }
    }

    const b2Auth = await getB2Authorization(env);
    const mediaResult = await fetchMediaFromB2(env, b2Auth, mediaPath, isImagePath, debugMode);
    const originResponse = mediaResult.response;

    if (debugMode) {
      return Response.json({
        pathname: url.pathname,
        extractedMediaPath,
        normalizedMediaPath: mediaPath,
        candidateKeys: mediaResult.candidateKeys,
        attemptedUrlsWithoutSecrets: mediaResult.attemptedUrlsWithoutSecrets,
        statuses: mediaResult.statuses,
      }, {
        status: originResponse.ok ? 200 : originResponse.status,
      });
    }

    if (!originResponse.ok) {
      if (isImagePath) {
        console.log(`media not found: requested=${mediaPath}`);
      }
      return new Response(`b2 fetch failed: ${originResponse.status}`, { status: originResponse.status });
    }

    const contentType = originResponse.headers.get("content-type") || "";
    if (contentType.includes("application/vnd.apple.mpegurl") || mediaPath.endsWith(".m3u8")) {
      const playlist = await originResponse.text();
      const rewritten = rewritePlaylist(url, playlist, token);
      const headers = new Headers(originResponse.headers);
      headers.set("cache-control", "private, max-age=60");
      return new Response(rewritten, {
        status: originResponse.status,
        statusText: originResponse.statusText,
        headers,
      });
    }

    const headers = new Headers(originResponse.headers);
    if (isImagePath) {
      headers.set("cache-control", "public, max-age=31536000, immutable");
      headers.set("x-b2-key", mediaResult.resolvedKey || mediaPath.replace(/^\//, ""));
      headers.set("x-media-fallback", mediaResult.fallbackUsed ? "true" : "false");
    }

    return new Response(originResponse.body, {
      status: originResponse.status,
      statusText: originResponse.statusText,
      headers,
    });
  },
};

async function fetchMediaFromB2(env, b2Auth, mediaPath, isImagePath, debugMode = false) {
  const candidates = isImagePath
    ? buildImageCandidateKeys(mediaPath)
    : buildVideoCandidateKeys(mediaPath);
  let lastResponse = null;
  const attemptedUrlsWithoutSecrets = [];
  const statuses = [];

  for (const candidate of candidates) {
    const b2Key = candidate.replace(/^\/+/, "");
    const originURL = buildB2Url(env, b2Key);
    console.log("mediaPath", mediaPath);
    console.log("b2Key", b2Key);
    console.log("b2Url", originURL);
    attemptedUrlsWithoutSecrets.push(originURL);
    const response = await fetch(originURL, {
      headers: {
        Authorization: b2Auth.authorizationToken,
      },
      cf: isImagePath
        ? {
            cacheEverything: true,
            cacheTtl: 31536000,
          }
        : undefined,
    });

    if (response.ok) {
      console.log("media hit", candidate);
      console.log("resolved b2Key", b2Key);
      if (candidate !== mediaPath.replace(/^\//, "")) {
        console.log(`media fallback: requested=${mediaPath} resolved=/${candidate}`);
      }
      return {
        response,
        resolvedKey: candidate,
        fallbackUsed: candidate !== mediaPath.replace(/^\//, ""),
        candidateKeys: candidates,
        attemptedUrlsWithoutSecrets,
        statuses,
      };
    }

    console.log("media miss", candidate, response.status);
    if (!isImagePath && response.status === 404) {
      const nextCandidate = candidates[candidates.indexOf(candidate) + 1];
      if (nextCandidate) {
        console.log("fallback b2Key", nextCandidate.replace(/^\/+/, ""));
      }
    }
    const debugStatus = { key: candidate, status: response.status };
    if (debugMode) {
      try {
        debugStatus.body = await response.clone().text();
      } catch {
        debugStatus.body = "";
      }
    }
    statuses.push(debugStatus);
    lastResponse = response;
    if (response.status !== 404) {
      return {
        response,
        resolvedKey: candidate,
        fallbackUsed: false,
        candidateKeys: candidates,
        attemptedUrlsWithoutSecrets,
        statuses,
      };
    }
  }

  return {
    response: lastResponse || new Response("b2 fetch failed: 404", { status: 404 }),
    resolvedKey: "",
    fallbackUsed: false,
    candidateKeys: candidates,
    attemptedUrlsWithoutSecrets,
    statuses,
  };
}

function readToken(request, url, cookieName) {
  const fromQuery = url.searchParams.get("token");
  if (fromQuery) {
    return { value: fromQuery, source: "query" };
  }

  const cookieHeader = request.headers.get("cookie") || "";
  const pair = cookieHeader
    .split(";")
    .map((part) => part.trim())
    .find((part) => part.startsWith(`${cookieName}=`));
  if (!pair) return null;
  return { value: pair.slice(cookieName.length + 1), source: "cookie" };
}

function decodeToken(tokenValue) {
  try {
    const decoded = atob(tokenValue.replace(/-/g, "+").replace(/_/g, "/").padEnd(Math.ceil(tokenValue.length / 4) * 4, "="));
    const [scope, exp, sig, ip = "", uaHash = ""] = decoded.split("\n");
    if (!scope || !exp || !sig) return null;
    return { scope: normalizeScope(scope), exp, sig, ip, uaHash };
  } catch {
    return null;
  }
}

async function sign(secret, scope, exp, ip = "", uaHash = "") {
  const cryptoKey = await crypto.subtle.importKey(
    "raw",
    new TextEncoder().encode(secret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"]
  );
  const payload =
    !ip && !uaHash
      ? `${scope}\n${exp}`
      : `${scope}\n${exp}\n${ip}\n${uaHash}`;
  const raw = await crypto.subtle.sign(
    "HMAC",
    cryptoKey,
    new TextEncoder().encode(payload)
  );
  return [...new Uint8Array(raw)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

async function sha256Hex(input) {
  const raw = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(input));
  return [...new Uint8Array(raw)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

function getRequestIP(request) {
  const cfIP = (request.headers.get("cf-connecting-ip") || "").trim();
  if (cfIP) return cfIP;

  const forwardedFor = (request.headers.get("x-forwarded-for") || "").trim();
  if (!forwardedFor) return "";
  return forwardedFor.split(",")[0].trim();
}

async function getB2Authorization(env) {
  if (cachedB2Auth && cachedB2Auth.expiresAt > Date.now() + 60_000) {
    return cachedB2Auth;
  }

  const basic = btoa(`${env.B2_KEY_ID}:${env.B2_APPLICATION_KEY}`);
  const response = await fetch("https://api.backblazeb2.com/b2api/v2/b2_authorize_account", {
    headers: {
      Authorization: `Basic ${basic}`,
    },
  });
  if (!response.ok) {
    throw new Error(`B2 authorization failed: ${response.status}`);
  }

  const data = await response.json();
  cachedB2Auth = {
    authorizationToken: data.authorizationToken,
    downloadUrl: data.downloadUrl,
    expiresAt: Date.now() + 60 * 60 * 1000,
  };
  return cachedB2Auth;
}

function buildB2Url(env, key) {
  const trimmedKey = String(key || "").replace(/^\/+/, "");
  const base = String(env.B2_DOWNLOAD_URL || "").replace(/\/+$/, "");
  const bucket = String(env.B2_BUCKET_NAME || "").replace(/^\/+|\/+$/g, "");

  if (base.includes(`/file/${bucket}`)) {
    return `${base}/${trimmedKey}`;
  }
  return `${base}/file/${bucket}/${trimmedKey}`;
}

function rewritePlaylist(requestURL, playlist, token) {
  const base = new URL(requestURL.toString());
  return playlist
    .split("\n")
    .map((line) => {
      if (!line || line.startsWith("#")) return line;
      const proxiedURL = toMediaProxyURL(line, base);
      if (token?.value) {
        proxiedURL.searchParams.set("token", token.value);
      }
      return `${proxiedURL.pathname}${proxiedURL.search}`;
    })
    .join("\n");
}

function toMediaProxyURL(line, base) {
  const absolute = new URL(line, base);
  const path = absolute.pathname.startsWith("/media/")
    ? normalizeMediaPath(absolute.pathname.replace(/^\/media/, ""))
    : normalizeMediaPath(extractMediaPath(absolute.pathname));
  const proxied = new URL(`/media${path}`, base.origin);
  return proxied;
}

function extractMediaPath(pathname) {
  const trimmed = String(pathname || "").replace(/^\/+/, "");
  if (trimmed.startsWith("media/")) {
    return `/${trimmed.slice("media/".length)}`;
  }
  return `/${trimmed}`;
}

function normalizeScope(scope) {
  return String(scope || "").replace(/^\/+/, "");
}

function matchesMediaScope(mediaPath, scope) {
  const normalizedMediaPath = String(mediaPath || "").replace(/^\/+/, "");
  const normalizedScope = normalizeScope(scope);

  if (!normalizedScope) {
    return false;
  }

  if (normalizedMediaPath === normalizedScope) {
    return true;
  }

  if (normalizedMediaPath.startsWith(normalizedScope)) {
    return true;
  }

  const mediaDir = mediaFolderScope(normalizedMediaPath);
  const scopeDir = mediaFolderScope(normalizedScope);
  return mediaDir !== "" && mediaDir === scopeDir;
}

function mediaFolderScope(value) {
  const normalized = String(value || "").replace(/^\/+/, "");
  if (!normalized) return "";
  if (normalized.endsWith("/")) return normalized;
  const slash = normalized.lastIndexOf("/");
  if (slash === -1) return normalized;
  return `${normalized.slice(0, slash + 1)}`;
}

function normalizeMediaPath(pathname) {
  const path = pathname.startsWith("/") ? pathname : `/${pathname}`;
  const replacements = [
    ["/posters/", "/images/posters/"],
    ["/backdrops/", "/images/backdrops/"],
    ["/avatars/", "/images/profile/"],
    ["/profile/", "/images/profile/"],
    ["/ads/", "/images/ads/"],
    ["/telegram-posts/", "/images/telegram-posts/"],
    ["/suggestions/", "/images/suggestions/"],
    ["/collections/", "/images/collections/"],
  ];

  for (const [legacyPrefix, canonicalPrefix] of replacements) {
    if (path.startsWith(legacyPrefix)) {
      return `${canonicalPrefix}${path.slice(legacyPrefix.length)}`;
    }
  }

  return path;
}

function buildImageCandidateKeys(mediaPath) {
  const path = mediaPath.replace(/^\/+/, "");

  if (path.startsWith("images/posters/")) {
    const file = path.slice("images/posters/".length);
    return uniqueKeys([`images/posters/${file}`, `posters/${file}`, file]);
  }

  if (path.startsWith("images/backdrops/")) {
    const file = path.slice("images/backdrops/".length);
    return uniqueKeys([`images/backdrops/${file}`, `backdrops/${file}`, file]);
  }

  if (path.startsWith("images/profile/")) {
    const file = path.slice("images/profile/".length);
    return uniqueKeys([`images/profile/${file}`, `avatars/${file}`, `profile/${file}`, file]);
  }

  if (path.startsWith("images/telegram-posts/")) {
    const file = path.slice("images/telegram-posts/".length);
    return uniqueKeys([`images/telegram-posts/${file}`, `telegram-posts/${file}`, file]);
  }

  if (path.startsWith("images/ads/")) {
    const file = path.slice("images/ads/".length);
    return uniqueKeys([`images/ads/${file}`, `ads/images/${file}`, `ads/${file}`]);
  }

  if (path.startsWith("images/suggestions/")) {
    const file = path.slice("images/suggestions/".length);
    return uniqueKeys([`images/suggestions/${file}`, `suggestions/${file}`]);
  }

  if (path.startsWith("images/")) {
    const rest = path.slice("images/".length);
    return uniqueKeys([`images/${rest}`, rest]);
  }

  return uniqueKeys([path]);
}

function uniqueKeys(keys) {
  return [...new Set(keys.filter(Boolean))];
}

function buildVideoCandidateKeys(mediaPath) {
  const key = String(mediaPath || "").replace(/^\/+/, "");
  const candidates = [key];

  if (key.startsWith("videos/serials/")) {
    return uniqueKeys(candidates);
  }

  if (key.startsWith("videos/movies/")) {
    candidates.push(key.replace(/^videos\/movies\//, "videos/"));
    return uniqueKeys(candidates);
  }

  if (key.startsWith("videos/") && !key.startsWith("videos/movies/")) {
    candidates.push(key.replace(/^videos\//, "videos/movies/"));
    return uniqueKeys(candidates);
  }

  return uniqueKeys(candidates);
}
