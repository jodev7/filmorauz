// src/lib/comments-api.ts
// Comment API for FILMORAUZ

const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080/api";

// Comment types
export interface Comment {
  id: string;
  movie_id: string;
  target_type?: string;
  target_id?: string;
  user_id: string;
  parent_id: string | null;
  content: string;
  status: string;
  has_blocked_word: boolean;
  has_link: boolean;
  created_at: string;
  updated_at: string;
  user_display_name: string;
  user_avatar_url?: string;
  user_role?: string;
  user_is_premium?: boolean;
  user_is_premium_active?: boolean;
  replies_count?: number;
  replies?: Comment[]; // Nested replies
  user?: {
    id: string;
    display_name?: string;
    username?: string;
    avatar_url?: string;
    role?: string;
    is_premium?: boolean;
    is_premium_active?: boolean;
  };
  movie_title?: string;
}

export type CommentStatus = "pending" | "approved" | "rejected";

export interface CommentWithReplies {
  comment: Comment;
  replies: Comment[];
}

export interface CommentListResponse {
  data: CommentWithReplies[];
  page: number;
  limit: number;
}

export interface CreateCommentResponse {
  message?: string;
  status?: string;
  data?: Comment;
}

export interface CommentModerationSettings {
  id: string;
  comments_enabled: boolean;
  replies_enabled: boolean;
  block_links: boolean;
  max_comment_length: number;
  max_reply_depth: number;
  require_moderation: boolean;
  banned_words: string[];
  auto_hide_banned_content: boolean;
  comment_cooldown_seconds: number;
  max_comments_per_minute: number;
  default_sort: string;
  updated_at: string;
}

export interface AdminComment extends Comment {}

export interface AdminCommentsOptions {
  page?: number;
  limit?: number;
  search?: string;
  status?: CommentStatus;
}

// Auth helper
function authHeaders(token: string) {
  return {
    "Content-Type": "application/json",
    Authorization: `Bearer ${token}`,
  };
}

// Get comments for a movie (public)
export async function getMovieComments(
  movieId: string,
  page: number = 1,
  limit: number = 20
): Promise<CommentListResponse> {
  const res = await fetch(
    `${API_URL}/v1/movies/${movieId}/comments?page=${page}&limit=${limit}`,
    {
      cache: "no-store",
    }
  );
  if (!res.ok) throw new Error("Failed to fetch comments");
  return res.json();
}

// Get comments for a target (movie or episode)
export async function getTargetComments(
  targetType: string,
  targetId: string,
  page: number = 1,
  limit: number = 20
): Promise<CommentListResponse> {
  const res = await fetch(
    `${API_URL}/v1/comments?target_type=${targetType}&target_id=${targetId}&page=${page}&limit=${limit}`,
    {
      cache: "no-store",
    }
  );
  if (!res.ok) throw new Error("Failed to fetch comments");
  return res.json();
}

// Get comments for an episode
export async function getEpisodeComments(
  episodeId: string,
  page: number = 1,
  limit: number = 20
): Promise<CommentListResponse> {
  return getTargetComments("episode", episodeId, page, limit);
}

// Create a comment (authenticated)
export async function createComment(
  token: string,
  movieId: string,
  content: string,
  targetType?: string,
  targetId?: string
): Promise<CreateCommentResponse> {
  const body: Record<string, string> = { content };
  
  // If targetType and targetId are provided, use the new format
  if (targetType && targetId) {
    body.target_type = targetType;
    body.target_id = targetId;
  }
  
  const res = await fetch(`${API_URL}/v1/movies/${movieId}/comments`, {
    method: "POST",
    headers: authHeaders(token),
    body: JSON.stringify(body),
  });
  const json = await res.json();
  if (!res.ok) {
    throw new Error(json.error || "Failed to create comment");
  }
  return json;
}

// Create a comment for a specific target (movie or episode)
export async function createTargetComment(
  token: string,
  targetType: string,
  targetId: string,
  content: string
): Promise<CreateCommentResponse> {
  // For backward compatibility, use movieId as targetId when targetType is movie
  const movieId = targetId; // Use targetId as movieId for backward compat
  
  return createComment(token, movieId, content, targetType, targetId);
}

// Create a reply (authenticated)
export async function createReply(
  token: string,
  commentId: string,
  content: string
): Promise<CreateCommentResponse> {
  const res = await fetch(`${API_URL}/v1/comments/${commentId}/replies`, {
    method: "POST",
    headers: authHeaders(token),
    body: JSON.stringify({ content }),
  });
  const json = await res.json();
  if (!res.ok) {
    throw new Error(json.error || "Failed to create reply");
  }
  return json;
}

// Update a comment (authenticated, owner only)
export async function updateComment(
  token: string,
  commentId: string,
  content: string
): Promise<Comment> {
  const res = await fetch(`${API_URL}/v1/comments/${commentId}`, {
    method: "PUT",
    headers: authHeaders(token),
    body: JSON.stringify({ content }),
  });
  const json = await res.json();
  if (!res.ok) {
    throw new Error(json.error || "Failed to update comment");
  }
  return json.data;
}

// Delete a comment (authenticated, owner only)
export async function deleteComment(
  token: string,
  commentId: string
): Promise<void> {
  const res = await fetch(`${API_URL}/v1/comments/${commentId}`, {
    method: "DELETE",
    headers: authHeaders(token),
  });
  if (!res.ok) {
    const json = await res.json();
    throw new Error(json.error || "Failed to delete comment");
  }
}

// Admin: Get comments by status
export async function getAdminComments(
  token: string,
  options: AdminCommentsOptions = {}
): Promise<{ data: AdminComment[]; total: number; page: number; limit: number; total_pages: number }> {
  const params = new URLSearchParams();
  if (options.status) params.set("status", options.status);
  if (options.search) params.set("search", options.search);
  params.set("page", String(options.page || 1));
  params.set("limit", String(options.limit || 20));

  const res = await fetch(`${API_URL}/v1/admin/comments?${params}`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch admin comments");
  return res.json();
}

// Admin: Update comment status
export async function updateCommentStatus(
  token: string,
  commentId: string,
  status: string
): Promise<void> {
  const res = await fetch(`${API_URL}/v1/admin/comments/${commentId}/status`, {
    method: "PATCH",
    headers: authHeaders(token),
    body: JSON.stringify({ status }),
  });
  if (!res.ok) {
    const json = await res.json();
    throw new Error(json.error || "Failed to update comment status");
  }
}

// Admin: Delete comment
export async function adminDeleteComment(
  token: string,
  commentId: string
): Promise<void> {
  const res = await fetch(`${API_URL}/v1/admin/comments/${commentId}`, {
    method: "DELETE",
    headers: authHeaders(token),
  });
  if (!res.ok) {
    const json = await res.json();
    throw new Error(json.error || "Failed to delete comment");
  }
}

// Admin: Get moderation settings
export async function getCommentSettings(
  token: string
): Promise<CommentModerationSettings> {
  const res = await fetch(`${API_URL}/v1/admin/comment-settings`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch comment settings");
  return res.json();
}

// Admin: Update moderation settings
export async function updateCommentSettings(
  token: string,
  settings: {
    comments_enabled?: boolean;
    replies_enabled?: boolean;
    block_links?: boolean;
    max_comment_length?: number;
    max_reply_depth?: number;
    require_moderation?: boolean;
    banned_words?: string[];
    auto_hide_banned_content?: boolean;
    comment_cooldown_seconds?: number;
    max_comments_per_minute?: number;
    default_sort?: string;
  }
): Promise<CommentModerationSettings> {
  const res = await fetch(`${API_URL}/v1/admin/comment-settings`, {
    method: "PUT",
    headers: authHeaders(token),
    body: JSON.stringify(settings),
  });
  if (!res.ok) {
    const json = await res.json();
    throw new Error(json.error || "Failed to update comment settings");
  }
  return res.json();
}