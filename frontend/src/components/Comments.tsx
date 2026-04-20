"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import { ChevronDown, ChevronRight, MessageCircle, Trash2 } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import {
  getMovieComments,
  getTargetComments,
  getEpisodeComments,
  createComment,
  createTargetComment,
  createReply,
  deleteComment,
  Comment,
  CommentWithReplies,
} from "@/lib/comments-api";
import { Locale, DEFAULT_LOCALE } from "@/lib/i18n";
import { formatRelativeAddedTime } from "@/lib/movie-utils";
import { PremiumBadge, resolveIsPremium } from "./PremiumComponents";

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
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  // Track which comment is being replied to
  const [replyTo, setReplyTo] = useState<string | null>(null);
  // Track reply content per comment - using a Map for multiple reply forms
  const [replyContents, setReplyContents] = useState<Map<string, string>>(new Map());
  // Track expanded threads - using comment ID as key
  const [expandedThreads, setExpandedThreads] = useState<Set<string>>(new Set());

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
      // Use episode comments API if targetType is episode
      if (targetType === "episode" && targetId) {
        const data = await getEpisodeComments(targetId);
        setComments(data.data || []);
      } else if (targetType && targetId) {
        // Generic target comments
        const data = await getTargetComments(targetType, targetId);
        setComments(data.data || []);
      } else if (movieId) {
        // Backward compatibility - movie comments
        const data = await getMovieComments(movieId);
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
        newComment.trim()
      );
      if (result.status === "pending") {
        setError("");
      }
      setNewComment("");
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

  const handleDeleteComment = async (commentId: string) => {
    if (!token || !confirm("Are you sure you want to delete this comment?")) {
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

  return (
    <div className="mt-8 pt-8 border-t border-brand-border">
      <h2 className="text-2xl font-display text-white mb-6">{tt.title}</h2>

      {/* Comment form */}
      {isAuthenticated && token ? (
        <form onSubmit={handleSubmitComment} className="mb-8">
          <textarea
            value={newComment}
            onChange={(e) => setNewComment(e.target.value)}
            placeholder={tt.writeComment}
            className="w-full bg-brand-card border border-brand-border rounded-lg p-3 text-white placeholder-gray-500 focus:border-brand-red focus:outline-none resize-none"
            rows={3}
            maxLength={2000}
          />
          {error && <p className="text-red-500 text-sm mt-2">{error}</p>}
          <button
            type="submit"
            disabled={submitting || !newComment.trim()}
            className="mt-2 bg-brand-red hover:bg-orange-700 text-white font-semibold px-4 py-2 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {submitting ? "..." : tt.submit}
          </button>
        </form>
      ) : (
        <div className="mb-8 p-4 bg-brand-card border border-brand-border rounded-lg">
          <p className="text-gray-400">{tt.loginToComment}</p>
        </div>
      )}

      {/* Comments list */}
      {loading ? (
        <div className="text-gray-400">Yuklanmoqda...</div>
      ) : comments.length === 0 ? (
        <div className="text-gray-400">{tt.noComments}</div>
      ) : (
        <div className="space-y-4">
          {comments.map((item) => (
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
              tt={tt}
              expandedThreads={expandedThreads}
              onToggleThread={toggleThread}
              depth={0}
              parentInfo={null}
            />
          ))}
        </div>
      )}
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
  tt: any;
  expandedThreads: Set<string>;
  onToggleThread: (id: string) => void;
  depth?: number;
  parentInfo?: { displayName: string; content: string } | null;
}) {
  // Get reply content for this specific comment
  const replyContent = replyContents.get(comment.id) || "";
  const isOwner = currentUserId === comment.user_id;
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
      <div className="bg-brand-card border border-brand-border rounded-lg p-4">
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
            {comment.user_avatar_url ? (
              <img
                src={comment.user_avatar_url}
                alt={comment.user_display_name}
                className="w-8 h-8 rounded-full"
              />
            ) : (
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

        {/* Comment content */}
        <p className="text-gray-300 mb-2">{comment.content}</p>

        {/* Action row: Reply button + toggle for replies */}
        <div className="flex items-center gap-4">
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
              className="flex-1 bg-brand-dark border border-brand-border rounded px-3 py-2 text-white text-sm"
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
