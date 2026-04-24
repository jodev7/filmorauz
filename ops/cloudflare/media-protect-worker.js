let cachedB2Auth = null;

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    if (!url.pathname.startsWith("/media/")) {
      return new Response("not found", { status: 404 });
    }

    const tokenValue = readToken(request, url, env.MEDIA_COOKIE_NAME || "filmorauz_media");
    if (!tokenValue) {
      return new Response("missing media token", { status: 403 });
    }

    const token = decodeToken(tokenValue);
    if (!token) {
      return new Response("invalid media token", { status: 403 });
    }

    if (Math.floor(Date.now() / 1000) > Number(token.exp)) {
      return new Response("expired media token", { status: 403 });
    }

    const expectedSig = await sign(env.MEDIA_SIGNING_SECRET, token.scope, token.exp);
    if (expectedSig !== token.sig) {
      return new Response("bad media signature", { status: 403 });
    }

    const mediaPath = url.pathname.replace(/^\/media/, "");
    if (!mediaPath.startsWith(token.scope)) {
      return new Response("scope mismatch", { status: 403 });
    }

    const b2Auth = await getB2Authorization(env);
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
      const rewritten = rewritePlaylist(url, playlist, tokenValue);
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
  if (fromQuery) return fromQuery;

  const cookieHeader = request.headers.get("cookie") || "";
  const pair = cookieHeader
    .split(";")
    .map((part) => part.trim())
    .find((part) => part.startsWith(`${cookieName}=`));
  if (!pair) return null;
  return pair.slice(cookieName.length + 1);
}

function decodeToken(tokenValue) {
  try {
    const decoded = atob(tokenValue.replace(/-/g, "+").replace(/_/g, "/").padEnd(Math.ceil(tokenValue.length / 4) * 4, "="));
    const [scope, exp, sig] = decoded.split("\n");
    if (!scope || !exp || !sig) return null;
    return { scope, exp, sig };
  } catch {
    return null;
  }
}

async function sign(secret, scope, exp) {
  const cryptoKey = await crypto.subtle.importKey(
    "raw",
    new TextEncoder().encode(secret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"]
  );
  const raw = await crypto.subtle.sign("HMAC", cryptoKey, new TextEncoder().encode(`${scope}\n${exp}`));
  return [...new Uint8Array(raw)].map((b) => b.toString(16).padStart(2, "0")).join("");
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
  const bucket = env.B2_BUCKET_NAME;
  const normalizedPath = mediaPath.replace(/^\/+/, "");
  const base = env.B2_DOWNLOAD_URL || "https://f005.backblazeb2.com";
  return `${base}/file/${bucket}/${normalizedPath}`;
}

function rewritePlaylist(requestURL, playlist, tokenValue) {
  const base = new URL(requestURL.toString());
  return playlist
    .split("\n")
    .map((line) => {
      if (!line || line.startsWith("#")) return line;
      const rewritten = new URL(line, base);
      rewritten.searchParams.set("token", tokenValue);
      return rewritten.toString();
    })
    .join("\n");
}
