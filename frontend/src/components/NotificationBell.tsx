"use client";

import { useEffect, useState, useRef } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth-context";
import { getNotifications, getUnreadNotificationCount, markNotificationAsRead, markAllNotificationsAsRead, Notification } from "@/lib/api";
import { Bell, Check, CheckCheck, X, Clock, Gift, AlertTriangle, MessageCircle, Ban, FileText } from "lucide-react";

export default function NotificationBell() {
  const { isAuthenticated, token, unreadNotificationCount, setUnreadNotificationCount } = useAuth();
  const router = useRouter();
  const [showDropdown, setShowDropdown] = useState(false);
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const [loading, setLoading] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);

  // Fetch unread count on mount and periodically
  useEffect(() => {
    if (!token) return;

    const fetchUnreadCount = async () => {
      try {
        const data = await getUnreadNotificationCount(token);
        setUnreadNotificationCount(data.count);
      } catch (error) {
        console.error("Failed to fetch unread count:", error);
      }
    };

    fetchUnreadCount();
    // Poll every 30 seconds
    const interval = setInterval(fetchUnreadCount, 30000);
    return () => clearInterval(interval);
  }, [token, setUnreadNotificationCount]);

  // Fetch notifications when dropdown opens
  useEffect(() => {
    if (!showDropdown || !token) return;

    const fetchNotifications = async () => {
      setLoading(true);
      try {
        const data = await getNotifications(token, { per_page: 10 });
        setNotifications(data.notifications);
      } catch (error) {
        console.error("Failed to fetch notifications:", error);
      } finally {
        setLoading(false);
      }
    };

    fetchNotifications();
  }, [showDropdown, token]);

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
        setShowDropdown(false);
      }
    };

    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  const handleNotificationClick = async (notification: Notification) => {
    // Mark as read if not already
    if (!notification.is_read && token) {
      try {
        await markNotificationAsRead(token, notification.id);
        setUnreadNotificationCount(Math.max(0, unreadNotificationCount - 1));
        setNotifications(notifications.map(n => 
          n.id === notification.id ? { ...n, is_read: true } : n
        ));
      } catch (error) {
        console.error("Failed to mark as read:", error);
      }
    }

    // Navigate to action URL
    if (notification.action_url) {
      router.push(notification.action_url);
      setShowDropdown(false);
    }
  };

  const handleMarkAllAsRead = async () => {
    if (!token) return;

    try {
      await markAllNotificationsAsRead(token);
      setUnreadNotificationCount(0);
      setNotifications(notifications.map(n => ({ ...n, is_read: true })));
    } catch (error) {
      console.error("Failed to mark all as read:", error);
    }
  };

  const getNotificationIcon = (type: string) => {
    switch (type) {
      case "PREMIUM_ACTIVATED":
      case "PREMIUM_EXPIRING_SOON":
      case "PREMIUM_EXPIRED":
        return <Gift className="w-5 h-5 text-yellow-400" />;
      case "BAN_APPLIED":
        return <Ban className="w-5 h-5 text-red-400" />;
      case "BAN_REMOVED":
      case "APPEAL_APPROVED":
      case "APPEAL_REJECTED":
      case "APPEAL_SUBMITTED":
        return <AlertTriangle className="w-5 h-5 text-orange-400" />;
      case "COMMENT_REPLY":
        return <MessageCircle className="w-5 h-5 text-blue-400" />;
      default:
        return <Bell className="w-5 h-5 text-gray-400" />;
    }
  };

  const formatTime = (dateString: string) => {
    const date = new Date(dateString);
    const now = new Date();
    const diff = now.getTime() - date.getTime();
    const minutes = Math.floor(diff / 60000);
    const hours = Math.floor(diff / 3600000);
    const days = Math.floor(diff / 86400000);

    if (minutes < 1) return "hozir";
    if (minutes < 60) return `${minutes} daqiqa oldin`;
    if (hours < 24) return `${hours} soat oldin`;
    if (days < 7) return `${days} kun oldin`;
    return date.toLocaleDateString("uz-UZ");
  };

  if (!isAuthenticated) {
    return null;
  }

  return (
    <div className="relative" ref={dropdownRef}>
      {/* Bell Button */}
      <button
        onClick={() => setShowDropdown(!showDropdown)}
        className="relative p-2 text-gray-400 hover:text-white transition-colors"
      >
        <Bell className="w-6 h-6" />
        {unreadNotificationCount > 0 && (
          <span className="absolute -top-1 -right-1 min-w-[20px] h-5 px-1.5 bg-brand-red text-white text-xs font-bold rounded-full flex items-center justify-center">
            {unreadNotificationCount > 99 ? "99+" : unreadNotificationCount}
          </span>
        )}
      </button>

      {/* Dropdown */}
      {showDropdown && (
        <div className="absolute right-0 mt-2 w-[calc(100vw-2rem)] max-w-[20rem] sm:w-80 bg-brand-card border border-brand-border rounded-xl shadow-2xl z-[60] overflow-hidden">
          {/* Header */}
          <div className="flex items-center justify-between px-4 py-3 border-b border-brand-border shrink-0">
            <h3 className="text-white font-medium">Bildirishnomalar</h3>
            {unreadNotificationCount > 0 && (
              <button
                onClick={handleMarkAllAsRead}
                className="flex items-center gap-1 text-sm text-gray-400 hover:text-white transition-colors"
              >
                <CheckCheck className="w-4 h-4" />
                <span className="hidden sm:inline">Hammasini o'qilgan deb belgilash</span>
              </button>
            )}
          </div>

          {/* Notification List */}
          <div className="max-h-[70vh] sm:max-h-96 overflow-y-auto">
            {loading ? (
              <div className="flex items-center justify-center py-8">
                <div className="animate-spin w-6 h-6 border-2 border-brand-red border-t-transparent rounded-full" />
              </div>
            ) : notifications.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-8 px-4">
                <Bell className="w-12 h-12 text-gray-600 mb-3" />
                <p className="text-gray-400 text-center">Hozircha bildirishnomalar yo'q</p>
              </div>
            ) : (
              notifications.map((notification) => (
                <div
                  key={notification.id}
                  onClick={() => handleNotificationClick(notification)}
                  className={`flex items-start gap-3 px-4 py-3 hover:bg-brand-dark/50 cursor-pointer transition-colors ${
                    !notification.is_read ? "bg-brand-dark/30" : ""
                  }`}
                >
                  <div className="flex-shrink-0 mt-1">
                    {getNotificationIcon(notification.type)}
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <p className={`text-sm truncate ${!notification.is_read ? "text-white font-medium" : "text-gray-300"}`}>
                        {notification.title}
                      </p>
                      {!notification.is_read && (
                        <span className="w-2 h-2 bg-brand-red rounded-full flex-shrink-0" />
                      )}
                    </div>
                    <p className="text-xs text-gray-500 mt-0.5 line-clamp-2">{notification.message}</p>
                    <div className="flex items-center gap-1 mt-1">
                      <Clock className="w-3 h-3 text-gray-500" />
                      <span className="text-xs text-gray-500">{formatTime(notification.created_at)}</span>
                    </div>
                  </div>
                </div>
              ))
            )}
          </div>

          {/* Footer */}
          {notifications.length > 0 && (
            <div className="px-4 py-3 border-t border-brand-border">
              <button
                onClick={() => {
                  router.push("/notifications");
                  setShowDropdown(false);
                }}
                className="w-full text-center text-sm text-brand-red hover:text-red-400 transition-colors"
              >
                Barcha bildirishnomalarni ko'rish
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
