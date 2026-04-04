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
  getCurrentUser,
  logout as apiLogout,
  CurrentUser,
  TelegramAuthStartResponse,
  TelegramAuthStatusResponse,
} from "@/lib/api";

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

  // Load token from cookie on mount
  useEffect(() => {
    const saved = Cookies.get("auth_token");
    if (saved) {
      setTokenState(saved);
    }
    setIsLoading(false);
  }, []);

  // Fetch user when token is available
  useEffect(() => {
    if (token && !user) {
      getCurrentUser(token)
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
          } else {
            // Token invalid, clear it
            Cookies.remove("auth_token");
            setTokenState(null);
          }
        })
        .catch(() => {
          Cookies.remove("auth_token");
          setTokenState(null);
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

      if (status.status === "completed" && status.token && status.user) {
        // Store token in cookie
        Cookies.set("auth_token", status.token, { expires: 7 });
        setTokenState(status.token);
        setUser(status.user);
        
        // Check if user is banned
        if (status.user.ban?.is_banned) {
          setBanInfo({
            is_banned: true,
            reason: status.user.ban.reason,
            banned_at: status.user.ban.banned_at,
            banned_until: status.user.ban.banned_until,
            banned_by_username: status.user.ban.banned_by_username,
          });
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
        const response = await getCurrentUser(token);
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
