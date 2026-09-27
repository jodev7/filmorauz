"use client";

import { useState, useEffect, useMemo } from "react";
import Link from "next/link";
import { ChevronDown, ChevronRight, MessageCircle, Trash2, Heart, Flag, EyeOff, ScrollText, MessagesSquare, User as UserIcon, Loader2, Send, LogIn } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import {
  getMovieComments,
  getTargetComments,
  getEpisodeComments,
  createComment,
  createTargetComment,
  createReply,
  deleteComment,
  toggleCommentLike,
  reportComment,
  REPORT_REASON_LABELS,
  ReportReason,
  Comment,
  CommentWithReplies,
} from "@/lib/comments-api";
import { Locale, DEFAULT_LOCALE } from "@/lib/i18n";
import { formatRelativeAddedTime } from "@/lib/movie-utils";
import { PremiumBadge, resolveIsPremium } from "./PremiumComponents";
import { DEFAULT_AVATAR_PLACEHOLDER, normalizeMediaUrl } from "@/lib/image-utils";
import MediaImage from "@/components/ui/MediaImage";
import CommentRulesModal from "@/components/comments/CommentRules";

type CommentSort = "newest" | "oldest" | "popular" | "discussed";

const SORTS: { key: CommentSort; label: string }[] = [
  { key: "newest", label: "Eng yangi" },
  { key: "popular", label: "Mashhur" },
  { key: "discussed", label: "Ko'p muhokama" },
  { key: "oldest", label: "Eng eski" },
];

const COMMENTS_LIMIT = 100;

interface CommentsSectionProps {
  movieId?: string;
  targetType?: string;
  targetId?: string;
}

export default function CommentsSection({
  movieId,
  targetType,
  targetId,
}: CommentsSectionProps) {
  const { token, isAuthenticated, user } = useAuth();
  const [comments, setComments] = useState<CommentWithReplies[]>([]);
  const [loading, setLoading] = useState(true);
  const [newComment, setNewComment] = useState("");
  const [newIsSpoiler, setNewIsSpoiler] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  // Track which comment is being replied to
  const [replyTo, setReplyTo] = useState<string | null>(null);
  // Track reply content per comment - using a Map for multiple reply forms
  const [replyContents, setReplyContents] = useState<Map<string, string>>(new Map());
  // Track expanded threads - using comment ID as key
  const [expandedThreads, setExpandedThreads] = useState<Set<string>>(new Set());
  // Filters
  const [sort, setSort] = useState<CommentSort>("newest");
  const [hideSpoilers, setHideSpoilers] = useState(false);
  const [onlyMine, setOnlyMine] = useState(false);
  const [rulesOpen, setRulesOpen] = useState(false);

  // Determine the actual target to use for comments
  const effectiveTargetId = targetId || movieId;
  const effectiveTargetType = targetType || "movie";

  // Fetch comments
  useEffect(() => {
    if (effectiveTargetId) {
      loadComments();
    }
  }, [effectiveTargetId, targetType]);

  const loadComments = async () => {
    try {
      setLoading(true);
      const tk = token || undefined;
      if (targetType === "episode" && targetId) {
        const data = await getEpisodeComments(targetId, 1, COMMENTS_LIMIT, tk);
        setComments(data.data || []);
      } else if (targetType && targetId) {
        const data = await getTargetComments(targetType, targetId, 1, COMMENTS_LIMIT, tk);
        setComments(data.data || []);
      } else if (movieId) {
        const data = await getMovieComments(movieId, 1, COMMENTS_LIMIT, tk);
        setComments(data.data || []);
      }
    } catch (err) {
      console.error("Failed to load comments:", err);
    } finally {
      setLoading(false);
    }
  };

  const handleSubmitComment = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token || !isAuthenticated) return;
    if (!newComment.trim()) return;
    if (!effectiveTargetId) return;

    setSubmitting(true);
    setError("");

    try {
      // Use target-based comment creation if targetType is provided
      const result = await createTargetComment(
        token,
        effectiveTargetType,
        effectiveTargetId,
        newComment.trim(),
        newIsSpoiler
      );
      if (result.status === "pending") {
        setError("");
      }
      setNewComment("");
      setNewIsSpoiler(false);
      loadComments();
    } catch (err: any) {
      setError(err.message);
    } finally {
      setSubmitting(false);
    }
  };

  const handleSubmitReply = async (parentId: string) => {
    const content = replyContents.get(parentId) || "";
    if (!token || !content.trim()) return;

    setSubmitting(true);
    setError("");

    try {
      await createReply(token, parentId, content.trim());
      // Clear only this reply's content
      setReplyContents((prev) => {
        const next = new Map(prev);
        next.delete(parentId);
        return next;
      });
      setReplyTo(null);
      // Auto-expand the thread after posting a reply
      setExpandedThreads((prev) => new Set(prev).add(parentId));
      loadComments();
    } catch (err: any) {
      setError(err.message);
    } finally {
      setSubmitting(false);
    }
  };

  const handleToggleLike = async (commentId: string) => {
    if (!token || !isAuthenticated) return;
    try {
      const res = await toggleCommentLike(token, commentId);
      const patch = (c: Comment): Comment =>
        c.id === commentId
          ? { ...c, liked_by_me: res.liked, likes_count: res.likes_count }
          : { ...c, replies: c.replies?.map(patch) };
      setComments((prev) =>
        prev.map((item) => ({
          comment: patch(item.comment),
          replies: item.replies?.map(patch) || [],
        }))
      );
    } catch (err: any) {
      setError(err.message);
    }
  };

  const handleDeleteComment = async (commentId: string) => {
    if (!token || !confirm("Izohni o'chirasizmi?")) {
      return;
    }

    try {
      await deleteComment(token, commentId);
      loadComments();
    } catch (err: any) {
      setError(err.message);
    }
  };

  // Toggle expanded state for a thread
  const toggleThread = (commentId: string) => {
    setExpandedThreads((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(commentId)) {
        newSet.delete(commentId);
      } else {
        newSet.add(commentId);
      }
      return newSet;
    });
  };

  const t = {
    uz: {
      title: "Izohlar",
      noComments: "Hozircha izohlar yo'q. Birinchi bo'lib izoh qoldiring!",
      writeComment: "Izohingizni yozing...",
      submit: "Yuborish",
      loginToComment: "Izoh qoldirish uchun tizimga kirish kerak",
      reply: "Javob",
      delete: "O'chirish",
      pending: "Izohingiz moderatsiyaga yuborildi. Tasdiqlanishini kuting.",
      cancel: "Bekor qilish",
      admin: "Admin",
      superAdmin: "Super Admin",
      premium: "Premium",
      showReplies: "ta javobni ko'rish",
      hideReplies: "Javoblarni yashirish",
      showReply: "ta javobni ko'rish",
      hideReply: "Javoblarni yashirish",
      replyingTo: "ga javob",
    },
  };

  const tt = t.uz;

  const visibleComments = useMemo(() => {
    const time = (c: Comment) => new Date(c.created_at).getTime() || 0;
    const list = comments.filter((item) => {
      if (hideSpoilers && item.comment.is_spoiler) return false;
      if (onlyMine && item.comment.user_id !== user?.id) return false;
      return true;
    });
    const replies = (item: CommentWithReplies) => item.comment.replies_count || item.replies?.length || 0;
    return [...list].sort((a, b) => {
      switch (sort) {
        case "oldest":
          return time(a.comment) - time(b.comment);
        case "popular":
          return (b.comment.likes_count || 0) - (a.comment.likes_count || 0) || time(b.comment) - time(a.comment);
        case "discussed":
          return replies(b) - replies(a) || time(b.comment) - time(a.comment);
        default:
          return time(b.comment) - time(a.comment);
      }
    });
  }, [comments, sort, hideSpoilers, onlyMine, user?.id]);

  const spoilerCount = useMemo(() => comments.filter((c) => c.comment.is_spoiler).length, [comments]);

  return (
    <div className="mt-8 border-t border-white/10 pt-8">
      {/* Header */}
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <span className="flex h-10 w-10 items-center justify-center rounded-2xl bg-gradient-to-br from-orange-500 to-rose-600 shadow-lg shadow-orange-500/20">
            <MessagesSquare size={19} className="text-white" />
          </span>
          <div>
            <h2 className="font-display text-2xl leading-none text-white">{tt.title}</h2>
            <p className="mt-1 text-xs text-gray-500">{loading ? "Yuklanmoqda..." : `${comments.length} ta izoh`}</p>
          </div>
        </div>
        <button
          type="button"
          onClick={() => setRulesOpen(true)}
          className="inline-flex items-center gap-1.5 rounded-xl border border-white/10 bg-white/[0.04] px-3 py-2 text-xs font-medium text-gray-200 transition hover:border-orange-500/50 hover:text-white sm:text-sm"
        >
          <ScrollText size={15} className="text-orange-400" />
          Izoh qoidalari
        </button>
      </div>

      {/* Comment form */}
      {isAuthenticated && token ? (
        <form onSubmit={handleSubmitComment} className="mb-6 rounded-2xl border border-white/10 bg-white/[0.02] p-3 transition focus-within:border-orange-500/50 sm:p-4">
          <textarea
            value={newComment}
            onChange={(e) => setNewComment(e.target.value)}
            placeholder={tt.writeComment}
            className="w-full resize-none bg-transparent text-white placeholder-gray-500 focus:outline-none"
            rows={3}
            maxLength={2000}
          />
          {error && <p className="mt-2 text-sm text-red-400">{error}</p>}
          <div className="mt-2 flex flex-wrap items-center gap-3 border-t border-white/5 pt-3">
            <label className="inline-flex cursor-pointer select-none items-center gap-2 text-sm text-gray-400">
              <input
                type="checkbox"
                checked={newIsSpoiler}
                onChange={(e) => setNewIsSpoiler(e.target.checked)}
                className="h-4 w-4 accent-orange-500"
              />
              Spoyler bor
            </label>
            <span className="text-[11px] tabular-nums text-gray-600">{newComment.length}/2000</span>
            <p className="hidden text-[11px] text-gray-500 sm:block">
              Yuborish orqali{" "}
              <button type="button" onClick={() => setRulesOpen(true)} className="text-orange-300 underline-offset-2 hover:underline">
                izoh qoidalariga
              </button>{" "}
              rozilik bildirasiz
            </p>
            <button
              type="submit"
              disabled={submitting || !newComment.trim()}
              className="ml-auto inline-flex items-center gap-2 rounded-xl bg-orange-500 px-4 py-2 text-sm font-semibold text-white shadow-lg shadow-orange-500/20 transition hover:bg-orange-400 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {submitting ? <Loader2 size={15} className="animate-spin" /> : <Send size={15} />}
              {tt.submit}
            </button>
          </div>
        </form>
      ) : (
        <div className="mb-6 flex items-center gap-3 rounded-2xl border border-dashed border-white/10 bg-white/[0.02] p-4">
          <LogIn size={18} className="shrink-0 text-gray-500" />
          <p className="text-sm text-gray-400">{tt.loginToComment}</p>
        </div>
      )}

      {/* Filters */}
      {comments.length > 0 && (
        <div className="mb-4 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <div className="scrollbar-hide flex gap-1 overflow-x-auto rounded-xl border border-white/10 bg-black/20 p-1" role="tablist" aria-label="Izohlarni saralash">
            {SORTS.map((o) => (
              <button
                key={o.key}
                type="button"
                role="tab"
                aria-selected={sort === o.key}
                onClick={() => setSort(o.key)}
                className={`whitespace-nowrap rounded-lg px-3 py-1.5 text-xs font-medium transition ${
                  sort === o.key ? "bg-white/10 text-white" : "text-gray-400 hover:text-white"
                }`}
              >
                {o.label}
              </button>
            ))}
          </div>
          <div className="flex flex-wrap gap-2">
            {spoilerCount > 0 && (
              <button
                type="button"
                aria-pressed={hideSpoilers}
                onClick={() => setHideSpoilers((v) => !v)}
                className={`inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs transition ${
                  hideSpoilers ? "border-amber-500/50 bg-amber-500/10 text-amber-200" : "border-white/10 text-gray-400 hover:text-white"
                }`}
              >
                <EyeOff size={13} /> Spoylersiz
              </button>
            )}
            {isAuthenticated && (
              <button
                type="button"
                aria-pressed={onlyMine}
                onClick={() => setOnlyMine((v) => !v)}
                className={`inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs transition ${
                  onlyMine ? "border-orange-500/50 bg-orange-500/10 text-orange-200" : "border-white/10 text-gray-400 hover:text-white"
                }`}
              >
                <UserIcon size={13} /> Mening izohlarim
              </button>
            )}
          </div>
        </div>
      )}

      {/* Comments list */}
      {loading ? (
        <div className="space-y-3">
          {[0, 1, 2].map((i) => (
            <div key={i} className="h-24 animate-pulse rounded-2xl bg-white/[0.04]" />
          ))}
        </div>
      ) : comments.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-white/10 py-10 text-center">
          <MessageCircle size={28} className="mx-auto mb-2 text-gray-600" />
          <p className="text-sm text-gray-400">{tt.noComments}</p>
        </div>
      ) : visibleComments.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-white/10 py-8 text-center text-sm text-gray-500">
          Bu filtr bo&apos;yicha izoh topilmadi.
        </div>
      ) : (
        <div className="space-y-4">
          {visibleComments.map((item) => (
            <CommentThread
              key={item.comment.id}
              comment={item.comment}
              replies={item.replies}
              repliesCount={item.comment.replies_count || 0}
              isAuthenticated={isAuthenticated}
              currentUserId={user?.id}
              token={token || undefined}
              onReply={(id) => setReplyTo(id)}
              onReplySubmit={handleSubmitReply}
              onReplyCancel={() => {
                setReplyTo(null);
              }}
              replyContents={replyContents}
              setReplyContents={setReplyContents}
              replyingTo={replyTo}
              submitting={submitting}
              onDelete={handleDeleteComment}
              onLike={handleToggleLike}
              tt={tt}
              expandedThreads={expandedThreads}
              onToggleThread={toggleThread}
              depth={0}
              parentInfo={null}
            />
          ))}
        </div>
      )}

      <CommentRulesModal open={rulesOpen} onClose={() => setRulesOpen(false)} />
    </div>
  );
}

// CommentThread component - handles collapsible replies
function CommentThread({
  comment,
  replies,
  repliesCount,
  isAuthenticated,
  currentUserId,
  token,
  onReply,
  onReplySubmit,
  onReplyCancel,
  replyContents,
  setReplyContents,
  replyingTo,
  submitting,
  onDelete,
  onLike,
  tt,
  expandedThreads,
  onToggleThread,
  depth = 0,
  parentInfo,
}: {
  comment: Comment;
  replies?: Comment[];
  repliesCount: number;
  isAuthenticated: boolean;
  currentUserId?: string;
  token?: string;
  onReply: (id: string) => void;
  onReplySubmit: (id: string) => void;
  onReplyCancel: () => void;
  replyContents: Map<string, string>;
  setReplyContents: (v: React.SetStateAction<Map<string, string>>) => void;
  replyingTo: string | null;
  submitting: boolean;
  onDelete: (id: string) => void;
  onLike: (id: string) => void;
  tt: any;
  expandedThreads: Set<string>;
  onToggleThread: (id: string) => void;
  depth?: number;
  parentInfo?: { displayName: string; content: string } | null;
}) {
  // Get reply content for this specific comment
  const replyContent = replyContents.get(comment.id) || "";
  const isOwner = currentUserId === comment.user_id;
  const [spoilerRevealed, setSpoilerRevealed] = useState(false);
  const [reportOpen, setReportOpen] = useState(false);
  const [reportState, setReportState] = useState<"idle" | "sending" | "sent">("idle");
  const relativeTime = formatRelativeAddedTime(comment.created_at);
  const isReplying = replyingTo === comment.id;
  const isExpanded = expandedThreads.has(comment.id);
  
  // Visual nesting limit - after this, flatten but keep logical structure
  const visualDepth = Math.min(depth, 2);
  const indentClass = ["", "ml-4", "ml-8"][visualDepth];
  
  // Check if this is a top-level comment (depth 0)
  const isTopLevel = depth === 0;
  const hasReplies = replies && replies.length > 0;

  // Get badge text and style based on role
  const getRoleBadge = () => {
    const role = comment.user_role;
    if (role === "superadmin") {
      return { text: tt.superAdmin || "Super Admin", className: "bg-purple-600" };
    }
    if (role === "admin") {
      return { text: tt.admin || "Admin", className: "bg-brand-red" };
    }
    if (role === "moderator") {
      return { text: "Moderator", className: "bg-blue-600" };
    }
    return null;
  };

  const roleBadge = getRoleBadge();
  const isPremium = resolveIsPremium(comment);

  // Get localized reply count text (always Uzbek)
  const getReplyCountText = (count: number) => {
    return count === 1 ? `1 ta javobni ko'rish` : `${count} ta javobni ko'rish`;
  };

  return (
    <div className={`${indentClass}`}>
      <div className="glass-card border border-white/10 rounded-2xl p-4">
        {/* Reply context label - only show for replies (depth > 0) */}
        {parentInfo && (
          <div className="mb-2 text-xs text-gray-500 flex items-center gap-1">
            <span>↪</span>
            <span>@{parentInfo.displayName}</span>
            <span>{tt.replyingTo}</span>
            {parentInfo.content && (
              <span className="truncate max-w-[150px] italic ml-1">
                "{parentInfo.content.length > 40 ? parentInfo.content.slice(0, 40) + '...' : parentInfo.content}"
              </span>
            )}
          </div>
        )}

        {/* Comment header with clickable author */}
        <div className="flex items-center justify-between mb-2">
          <div className="flex items-center gap-2">
            {comment.user_avatar_url ? (() => {
              const avatarSrc = comment.user_avatar_url;
              return (
              <MediaImage
                src={avatarSrc}
                alt={comment.user_display_name}
                fallbackSrc={DEFAULT_AVATAR_PLACEHOLDER}
                className="w-8 h-8 rounded-full"
              />
              );
            })() : (
              <div className="w-8 h-8 rounded-full bg-brand-red flex items-center justify-center text-white text-sm">
                {comment.user_display_name.charAt(0).toUpperCase()}
              </div>
            )}
            {/* Clickable author name with badges */}
            <div className="flex items-center gap-2 flex-wrap">
              <Link
                href={`/user/${comment.user_id}`}
                className="text-white font-medium hover:text-brand-red transition-colors"
              >
                {comment.user_display_name}
              </Link>
              {/* Role badges */}
              {roleBadge && (
                <span className={`${roleBadge.className} text-white text-xs px-2 py-0.5 rounded-full font-medium`}>
                  {roleBadge.text}
                </span>
              )}
              {/* Premium badge with glow */}
              {isPremium && (
                <PremiumBadge size="sm" showCrown />
              )}
            </div>
            <span className="text-gray-500 text-sm">{relativeTime}</span>
          </div>
          {isOwner && (
            <button
              onClick={() => onDelete(comment.id)}
              className="text-red-500 text-sm hover:underline flex items-center gap-1"
            >
              <Trash2 size={14} />
              {tt.delete}
            </button>
          )}
        </div>

        {/* Comment content — spoilers stay blurred until the reader opts in */}
        {comment.is_spoiler && !spoilerRevealed ? (
          <button
            type="button"
            onClick={() => setSpoilerRevealed(true)}
            className="group relative mb-2 block w-full text-left"
            aria-label="Spoylerni ko'rsatish"
          >
            <p className="text-gray-300 blur-sm select-none" aria-hidden>{comment.content}</p>
            <span className="absolute inset-0 flex items-center justify-center">
              <span className="inline-flex items-center gap-1.5 rounded-full bg-black/70 px-3 py-1 text-xs text-white group-hover:bg-black/85">
                <EyeOff size={13} /> Spoyler — ko&apos;rish uchun bosing
              </span>
            </span>
          </button>
        ) : (
          <p className="text-gray-300 mb-2">
            {comment.is_spoiler && (
              <span className="mr-2 rounded bg-amber-500/15 px-1.5 py-0.5 text-[10px] font-semibold text-amber-300 align-middle">SPOYLER</span>
            )}
            {comment.content}
          </p>
        )}

        {/* Action row: Reply button + toggle for replies */}
        <div className="flex items-center gap-4">
          {/* Like button */}
          <button
            onClick={() => isAuthenticated && onLike(comment.id)}
            disabled={!isAuthenticated}
            className={`text-sm flex items-center gap-1 transition-colors ${
              comment.liked_by_me ? "text-brand-red" : "text-gray-400 hover:text-brand-red"
            } ${!isAuthenticated ? "cursor-not-allowed" : ""}`}
          >
            <Heart size={14} fill={comment.liked_by_me ? "currentColor" : "none"} />
            <span>{comment.likes_count ?? 0}</span>
          </button>

          {/* Reply button */}
          {isAuthenticated && (
            <button
              onClick={() => onReply(comment.id)}
              className="text-brand-red text-sm hover:underline flex items-center gap-1"
            >
              <MessageCircle size={14} />
              {tt.reply}
            </button>
          )}
          
          {/* Toggle replies button - only show for top-level comments with replies */}
          {isTopLevel && hasReplies && (
            <button
              onClick={() => onToggleThread(comment.id)}
              className="text-gray-400 text-sm hover:text-white flex items-center gap-1 transition-colors"
            >
              {isExpanded ? (
                <>
                  <ChevronDown size={14} />
                  {tt.hideReplies}
                </>
              ) : (
                <>
                  <ChevronRight size={14} />
                  {getReplyCountText(replies.length)}
                </>
              )}
            </button>
          )}
          {/* Report ("shikoyat") — logged-in users, not on their own comments */}
          {isAuthenticated && token && !isOwner && (
            <div className="relative ml-auto">
              {reportState === "sent" ? (
                <span className="text-xs text-gray-500">Shikoyat yuborildi</span>
              ) : (
                <button
                  onClick={() => setReportOpen((o) => !o)}
                  className="text-gray-500 text-sm hover:text-amber-400 flex items-center gap-1"
                  aria-expanded={reportOpen}
                  title="Shikoyat qilish"
                >
                  <Flag size={13} />
                  <span className="hidden sm:inline">Shikoyat</span>
                </button>
              )}
              {reportOpen && (
                <div className="absolute right-0 top-full z-20 mt-1 w-52 overflow-hidden rounded-lg border border-white/10 bg-black/95 py-1 text-sm shadow-xl">
                  {(Object.keys(REPORT_REASON_LABELS) as ReportReason[]).map((reason) => (
                    <button
                      key={reason}
                      disabled={reportState === "sending"}
                      onClick={async () => {
                        setReportState("sending");
                        try {
                          await reportComment(token, comment.id, reason);
                          setReportState("sent");
                        } catch {
                          setReportState("idle");
                        } finally {
                          setReportOpen(false);
                        }
                      }}
                      className="block w-full px-3 py-1.5 text-left text-gray-200 hover:bg-white/10 disabled:opacity-50"
                    >
                      {REPORT_REASON_LABELS[reason]}
                    </button>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>

        {/* Reply form */}
        {isReplying && (
          <div className="mt-3 flex gap-2">
            <input
              type="text"
              value={replyContent}
              onChange={(e) => setReplyContents((prev) => {
                const next = new Map(prev);
                next.set(comment.id, e.target.value);
                return next;
              })}
              placeholder={tt.writeComment}
              className="flex-1 bg-brand-dark border border-white/10 rounded px-3 py-2 text-white text-sm"
              autoFocus
            />
            <button
              onClick={() => onReplySubmit(comment.id)}
              disabled={submitting || !replyContent.trim()}
              className="bg-brand-red hover:bg-orange-700 text-white text-sm px-3 py-2 rounded transition-colors disabled:opacity-50"
            >
              {tt.submit}
            </button>
            <button
              onClick={onReplyCancel}
              className="text-gray-400 text-sm px-3 py-2 hover:text-white"
            >
              {tt.cancel}
            </button>
          </div>
        )}
      </div>

      {/* Nested Replies - only render if expanded (for top-level) or always for nested */}
      {hasReplies && (
        <div className={`mt-3 space-y-3 ${isTopLevel ? (isExpanded ? 'block' : 'hidden') : 'block'}`}>
          {replies.map((reply) => (
            <CommentThread
              key={reply.id}
              comment={reply}
              replies={reply.replies}
              repliesCount={reply.replies_count || 0}
              isAuthenticated={isAuthenticated}
              currentUserId={currentUserId}
              token={token}
              onReply={onReply}
              onReplySubmit={onReplySubmit}
              onReplyCancel={onReplyCancel}
              replyContents={replyContents}
              setReplyContents={setReplyContents}
              replyingTo={replyingTo}
              submitting={submitting}
              onDelete={onDelete}
              onLike={onLike}
              tt={tt}
              expandedThreads={expandedThreads}
              onToggleThread={onToggleThread}
              depth={depth + 1}
              // Pass parent info to replies
              parentInfo={{
                displayName: comment.user_display_name,
                content: comment.content,
              }}
            />
          ))}
        </div>
      )}
    </div>
  );
}
