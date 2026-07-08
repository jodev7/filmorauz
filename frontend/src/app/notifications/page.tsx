"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth-context";
import { getNotifications, markNotificationAsRead, markAllNotificationsAsRead, Notification } from "@/lib/api";
import { Bell, Check, CheckCheck, Clock, Gift, AlertTriangle, MessageCircle, Ban, Crown, ChevronRight, FileText } from "lucide-react";

export default function NotificationsPage() {
  const { user, token, isLoading: authLoading, setUnreadNotificationCount } = useAuth();
  const router = useRouter();
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const [total, setTotal] = useState(0);
  const [unreadCount, setUnreadCount] = useState(0);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState<"all" | "unread">("all");

  // Auth check
  useEffect(() => {
    if (authLoading) return;
    
    if (!token || !user) {
      router.push("/");
      return;
    }
    
    loadNotifications();
  }, [authLoading, token, user, router]);

  const loadNotifications = async () => {
    if (!token) return;
    
    setLoading(true);
    try {
      const data = await getNotifications(token, { per_page: 50 });
      setNotifications(data.notifications);
      setTotal(data.total);
      setUnreadCount(data.unread_count);
    } catch (error) {
      console.error("Failed to load notifications:", error);
    } finally {
      setLoading(false);
    }
  };

  const handleMarkAsRead = async (notification: Notification) => {
    if (!token || notification.is_read) return;

    try {
      await markNotificationAsRead(token, notification.id);
      setNotifications(notifications.map(n => 
        n.id === notification.id ? { ...n, is_read: true } : n
      ));
      setUnreadCount(Math.max(0, unreadCount - 1));
      setUnreadNotificationCount(Math.max(0, unreadCount - 1));
    } catch (error) {
      console.error("Failed to mark as read:", error);
    }
  };

  const handleMarkAllAsRead = async () => {
    if (!token) return;

    try {
      await markAllNotificationsAsRead(token);
      setNotifications(notifications.map(n => ({ ...n, is_read: true })));
      setUnreadCount(0);
      setUnreadNotificationCount(0);
    } catch (error) {
      console.error("Failed to mark all as read:", error);
    }
  };

  const handleNotificationClick = async (notification: Notification) => {
    await handleMarkAsRead(notification);
    if (notification.action_url) {
      router.push(notification.action_url);
    }
  };

  const getNotificationIcon = (type: string) => {
    switch (type) {
      case "PREMIUM_ACTIVATED":
        return <Gift className="w-6 h-6 text-yellow-400" />;
      case "PREMIUM_EXPIRING_SOON":
        return <Crown className="w-6 h-6 text-orange-400" />;
      case "PREMIUM_EXPIRED":
        return <Crown className="w-6 h-6 text-gray-400" />;
      case "BAN_APPLIED":
        return <Ban className="w-6 h-6 text-red-400" />;
      case "BAN_REMOVED":
        return <Check className="w-6 h-6 text-green-400" />;
      case "APPEAL_SUBMITTED":
        return <FileText className="w-6 h-6 text-blue-400" />;
      case "APPEAL_APPROVED":
        return <Check className="w-6 h-6 text-green-400" />;
      case "APPEAL_REJECTED":
        return <AlertTriangle className="w-6 h-6 text-red-400" />;
      case "COMMENT_REPLY":
        return <MessageCircle className="w-6 h-6 text-blue-400" />;
      default:
        return <Bell className="w-6 h-6 text-gray-400" />;
    }
  };

  const formatDate = (dateString: string) => {
    const date = new Date(dateString);
    return date.toLocaleDateString("uz-UZ", {
      year: "numeric",
      month: "long",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  };

  const filteredNotifications = filter === "unread" 
    ? notifications.filter(n => !n.is_read)
    : notifications;

  if (authLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin w-10 h-10 border-3 border-brand-red border-t-transparent rounded-full" />
      </div>
    );
  }

  return (
    <div className="min-h-screen">
      <div className="max-w-3xl mx-auto px-4 py-8">
        {/* Header */}
        <div className="flex items-center justify-between mb-8">
          <div className="flex items-center gap-3">
            <div className="w-12 h-12 rounded-xl bg-brand-red/20 flex items-center justify-center">
              <Bell className="w-6 h-6 text-brand-red" />
            </div>
            <div>
              <h1 className="text-2xl font-display text-white">Bildirishnomalar</h1>
              <p className="text-gray-400 text-sm">{total} ta bildirishnoma</p>
            </div>
          </div>
          
          {unreadCount > 0 && (
            <button
              onClick={handleMarkAllAsRead}
              className="flex items-center gap-2 px-4 py-2 glass-card hover:bg-brand-border text-white rounded-lg transition-colors"
            >
              <CheckCheck className="w-4 h-4" />
              <span>Hammasini o'qilgan deb belgilash</span>
            </button>
          )}
        </div>

        {/* Filters */}
        <div className="flex gap-2 mb-6">
          <button
            onClick={() => setFilter("all")}
            className={`px-4 py-2 rounded-lg transition-colors ${
              filter === "all"
                ? "bg-brand-red text-white"
                : "glass-card text-gray-400 hover:text-white"
            }`}
          >
            Barchasi ({total})
          </button>
          <button
            onClick={() => setFilter("unread")}
            className={`px-4 py-2 rounded-lg transition-colors ${
              filter === "unread"
                ? "bg-brand-red text-white"
                : "glass-card text-gray-400 hover:text-white"
            }`}
          >
            O'qilmagan ({unreadCount})
          </button>
        </div>

        {/* Notifications List */}
        {loading ? (
          <div className="flex items-center justify-center py-20">
            <div className="animate-spin w-10 h-10 border-3 border-brand-red border-t-transparent rounded-full" />
          </div>
        ) : filteredNotifications.length === 0 ? (
          <div className="glass-card border border-white/10 rounded-xl p-12 text-center">
            <Bell className="w-16 h-16 text-gray-600 mx-auto mb-4" />
            <h2 className="text-xl text-white mb-2">
              {filter === "unread" ? "O'qilmagan bildirishnomalar yo'q" : "Hozircha bildirishnomalar yo'q"}
            </h2>
            <p className="text-gray-400">
              {filter === "unread" 
                ? "Barcha bildirishnomalarni o'qdingiz"
                : "Yangi bildirishnomalar kelganda bu yerda ko'rinadi"}
            </p>
          </div>
        ) : (
          <div className="glass-card border border-white/10 rounded-xl overflow-hidden">
            {filteredNotifications.map((notification, index) => (
              <div
                key={notification.id}
                className={`flex items-start gap-4 p-4 hover:bg-brand-dark/50 cursor-pointer transition-colors ${
                  !notification.is_read ? "bg-brand-dark/30" : ""
                } ${index !== filteredNotifications.length - 1 ? "border-b border-white/5" : ""}`}
                onClick={() => handleNotificationClick(notification)}
              >
                {/* Icon */}
                <div className={`flex-shrink-0 w-12 h-12 rounded-xl flex items-center justify-center ${
                  !notification.is_read ? "bg-brand-red/20" : "bg-brand-dark"
                }`}>
                  {getNotificationIcon(notification.type)}
                </div>

                {/* Content */}
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 mb-1">
                    <h3 className={`font-medium ${!notification.is_read ? "text-white" : "text-gray-300"}`}>
                      {notification.title}
                    </h3>
                    {!notification.is_read && (
                      <span className="w-2 h-2 bg-brand-red rounded-full" />
                    )}
                  </div>
                  <p className="text-gray-400 text-sm mb-2">{notification.message}</p>
                  <div className="flex items-center gap-1 text-gray-500 text-xs">
                    <Clock className="w-3 h-3" />
                    <span>{formatDate(notification.created_at)}</span>
                  </div>
                </div>

                {/* Mark as Read Button */}
                {!notification.is_read && (
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      handleMarkAsRead(notification);
                    }}
                    className="flex-shrink-0 p-2 text-gray-400 hover:text-white hover:bg-brand-border rounded-lg transition-colors"
                    title="O'qilgan deb belgilash"
                  >
                    <Check className="w-4 h-4" />
                  </button>
                )}

                {/* Action Arrow */}
                {notification.action_url && (
                  <ChevronRight className="flex-shrink-0 w-5 h-5 text-gray-500" />
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
