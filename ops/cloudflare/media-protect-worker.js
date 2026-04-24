let cachedB2Auth = null;

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    if (!url.pathname.startsWith("/media/")) {
      return new Response("not found", { status: 404 });
    }

    const isImagePath =
      url.pathname.startsWith("/media/posters/") ||
      url.pathname.startsWith("/media/backdrops/") ||
      url.pathname.startsWith("/media/images/") ||
      url.pathname.startsWith("/media/ads/");

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

      const mediaPath = url.pathname.replace(/^\/media/, "");
      if (!mediaPath.startsWith(claims.scope)) {
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
    const mediaPath = extractMediaPath(url.pathname);
    const originURL = buildB2DownloadURL(env, mediaPath);
    const originResponse = await fetch(originURL, {
      headers: {
        Authorization: b2Auth.authorizationToken,
      },
    });

    if (!originResponse.ok) {
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

    return new Response(originResponse.body, {
      status: originResponse.status,
      statusText: originResponse.statusText,
      headers: originResponse.headers,
    });
  },
};

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
    return { scope, exp, sig, ip, uaHash };
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

function buildB2DownloadURL(env, mediaPath) {
  const path = mediaPath.startsWith("/") ? mediaPath : `/${mediaPath}`;
  return `${env.B2_DOWNLOAD_URL}/${env.B2_BUCKET_NAME}${path}`;
}

function rewritePlaylist(requestURL, playlist, token) {
  const base = new URL(requestURL.toString());
  return playlist
    .split("\n")
    .map((line) => {
      if (!line || line.startsWith("#")) return line;
      const proxiedURL = toMediaProxyURL(line, base);
      if (token?.source === "query" && token.value) {
        proxiedURL.searchParams.set("token", token.value);
      }
      return `${proxiedURL.pathname}${proxiedURL.search}`;
    })
    .join("\n");
}

function toMediaProxyURL(line, base) {
  const absolute = new URL(line, base);
  const path = absolute.pathname.startsWith("/media/")
    ? absolute.pathname.replace(/^\/media/, "")
    : extractMediaPath(absolute.pathname);
  const proxied = new URL(`/media${path}`, base.origin);
  return proxied;
}

function extractMediaPath(pathname) {
  const path = pathname.replace(/^\/media\//, "");
  return `/${path}`;
}
