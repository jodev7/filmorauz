"use client";

import {
  createContext,
  useContext,
  useState,
  useEffect,
  ReactNode,
  useCallback,
} from "react";
import Cookies from "js-cookie";
import {
  startTelegramLogin,
  getTelegramAuthStatus,
  getAuthBootstrap,
  logout as apiLogout,
  refreshAuthToken,
  CurrentUser,
  TelegramAuthStartResponse,
  TelegramAuthStatusResponse,
} from "@/lib/api";
import { logger } from "@/lib/logger";

interface BanInfo {
  is_banned: boolean;
  reason?: string;
  banned_at?: string;
  banned_until?: string | null;
  banned_by_username?: string;
}

interface AuthContextType {
  // Auth state
  isAuthenticated: boolean;
  isLoading: boolean;
  user: CurrentUser | null;
  token: string | null;
  setToken: (token: string) => void;
  isBanned: boolean;
  banInfo: BanInfo | null;
  unreadNotificationCount: number;
  setUnreadNotificationCount: (count: number) => void;

  // Actions
  startTelegramAuth: () => Promise<TelegramAuthStartResponse | null>;
  checkAuthStatus: (code: string) => Promise<boolean>;
  logout: () => Promise<void>;
  refreshUser: () => Promise<void>;
}

const AuthContext = createContext<AuthContextType>({
  isAuthenticated: false,
  isLoading: true,
  user: null,
  token: null,
  setToken: () => {},
  isBanned: false,
  banInfo: null,
  unreadNotificationCount: 0,
  setUnreadNotificationCount: () => {},
  startTelegramAuth: async () => null,
  checkAuthStatus: async () => false,
  logout: async () => {},
  refreshUser: async () => {},
});

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setTokenState] = useState<string | null>(null);
  const [user, setUser] = useState<CurrentUser | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [banInfo, setBanInfo] = useState<BanInfo | null>(null);
  const [unreadNotificationCount, setUnreadNotificationCount] = useState(0);

  // Check if user is banned
  const isBanned = user?.ban?.is_banned === true;

  // Load token from cookie on mount.
  // If a token exists, we stay in isLoading=true until the /auth/me fetch below
  // resolves — otherwise the Navbar would flash the guest login button between
  // "cookie read" and "user fetched".
  useEffect(() => {
    const saved = Cookies.get("auth_token");
    if (saved) {
      setTokenState(saved);
      // keep isLoading=true; the user-fetch effect flips it off
    } else {
      setIsLoading(false);
    }
  }, []);

  // Fetch user when token is available
  // More resilient: try token refresh on 401 before logging out
  useEffect(() => {
    if (token && !user) {
      getAuthBootstrap(token)
        .then((response) => {
          if (response.authenticated && response.user) {
            // Check if user is banned
            if (response.user.ban?.is_banned) {
              setBanInfo({
                is_banned: true,
                reason: response.user.ban.reason,
                banned_at: response.user.ban.banned_at,
                banned_until: response.user.ban.banned_until,
                banned_by_username: response.user.ban.banned_by_username,
              });
            }
            setUser(response.user);
            setUnreadNotificationCount(response.unread_count || 0);
          } else {
            // Token invalid (not just expired), clear it
            Cookies.remove("auth_token");
            setTokenState(null);
          }
        })
        .catch(async (error: unknown) => {
          const err = error as { response?: { status?: number } };
          const status = err?.response?.status;
          
          // Only logout on confirmed auth failure (401/403)
          if (status === 401 || status === 403) {
            // Try to refresh the token first
            try {
              const refreshResult = await refreshAuthToken(token);
              if (refreshResult.authenticated && refreshResult.token) {
                // Token refreshed successfully - update token and retry fetch
                logger.debug("[Auth] Token refreshed");
                Cookies.set("auth_token", refreshResult.token, { expires: 7 });
                setTokenState(refreshResult.token);
                
                // Fetch user with new token
                const userResponse = await getAuthBootstrap(refreshResult.token);
                if (userResponse.authenticated && userResponse.user) {
                  setUser(userResponse.user);
                  setUnreadNotificationCount(userResponse.unread_count || 0);
                }
              }
            } catch {
              // Refresh failed - clear auth
              logger.warn("[Auth] Token refresh failed");
              Cookies.remove("auth_token");
              setTokenState(null);
            }
          } else {
            // Network error or 5xx - keep user logged in, they'll try again
            logger.warn("[Auth] User fetch failed", status);
          }
        })
        .finally(() => {
          setIsLoading(false);
        });
    }
  }, [token, user]);

  // Update ban info when user is set or ban changes
  useEffect(() => {
    if (user?.ban?.is_banned) {
      setBanInfo({
        is_banned: true,
        reason: user.ban.reason,
        banned_at: user.ban.banned_at,
        banned_until: user.ban.banned_until,
        banned_by_username: user.ban.banned_by_username,
      });
    } else {
      setBanInfo(null);
    }
  }, [user]);

  const startTelegramAuth = useCallback(async (): Promise<TelegramAuthStartResponse | null> => {
    try {
      const response = await startTelegramLogin();
      return response;
    } catch (error) {
      console.error("Failed to start Telegram login:", error);
      return null;
    }
  }, []);

  const checkAuthStatus = useCallback(async (code: string): Promise<boolean> => {
    try {
      const status: TelegramAuthStatusResponse = await getTelegramAuthStatus(code);

      if (status.status === "completed" && status.token) {
        // Store token in cookie
        Cookies.set("auth_token", status.token, { expires: 7 });
        setTokenState(status.token);

        // Fetch full profile so we get the sanitized first_name, photo, etc.
        try {
          const fullProfile = await getAuthBootstrap(status.token);
          if (fullProfile.authenticated && fullProfile.user) {
            setUser(fullProfile.user);
            setUnreadNotificationCount(fullProfile.unread_count || 0);
            if (fullProfile.user.ban?.is_banned) {
              setBanInfo({
                is_banned: true,
                reason: fullProfile.user.ban.reason,
                banned_at: fullProfile.user.ban.banned_at,
                banned_until: fullProfile.user.ban.banned_until,
                banned_by_username: fullProfile.user.ban.banned_by_username,
              });
            }
          }
        } catch {
          // Fallback: the useEffect on token will pick up the user
        }

        return true;
      }

      if (status.status === "expired") {
        return false;
      }

      return false;
    } catch (error) {
      console.error("Failed to check auth status:", error);
      return false;
    }
  }, []);

  const logout = useCallback(async () => {
    if (token) {
      try {
        await apiLogout(token);
      } catch (error) {
        console.error("Logout API error:", error);
      }
    }
    Cookies.remove("auth_token");
    setTokenState(null);
    setUser(null);
    setBanInfo(null);
  }, [token]);

  const refreshUser = useCallback(async () => {
    if (token) {
      try {
        const response = await getAuthBootstrap(token);
        if (response.authenticated && response.user) {
          // Check if user is banned
          if (response.user.ban?.is_banned) {
            setBanInfo({
              is_banned: true,
              reason: response.user.ban.reason,
              banned_at: response.user.ban.banned_at,
              banned_until: response.user.ban.banned_until,
              banned_by_username: response.user.ban.banned_by_username,
            });
          } else {
            setBanInfo(null);
          }
          setUser(response.user);
          setUnreadNotificationCount(response.unread_count || 0);
        }
      } catch (error) {
        console.error("Failed to refresh user:", error);
      }
    }
  }, [token]);

  // Exposed setToken for external use (e.g., admin login)
  const setToken = useCallback((t: string) => {
    Cookies.set("auth_token", t, { expires: 7 });
    setTokenState(t);
  }, []);

  return (
    <AuthContext.Provider
      value={{
        isAuthenticated: !!token && !!user,
        isLoading,
        user,
        token,
        setToken,
        isBanned,
        banInfo,
        unreadNotificationCount,
        setUnreadNotificationCount,
        startTelegramAuth,
        checkAuthStatus,
        logout,
        refreshUser,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export const useAuth = () => useContext(AuthContext);
