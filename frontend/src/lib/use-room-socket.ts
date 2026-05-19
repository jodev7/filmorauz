"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { getRoomWebSocketURL } from "./api";

export type RoomSyncState = {
  position: number;
  isPlaying: boolean;
  asOfMs: number;
};

export type RoomMember = {
  userID: string;
  userName: string;
  userAvatar?: string;
  isHost: boolean;
};

export type RoomChatEntry = {
  userID: string;
  userName: string;
  userAvatar?: string;
  kind: "text" | "emoji";
  text?: string;
  emoji?: string;
  createdAt: string;
};

export type RoomReaction = {
  userID: string;
  userName: string;
  emoji: string;
  ts: number;
};

export type RoomTyping = {
  userID: string;
  userName: string;
  isTyping: boolean;
};

export type RoomEvent =
  | { type: "state"; state: RoomSyncState }
  | { type: "chat"; chat: RoomChatEntry }
  | { type: "member_snapshot"; members: RoomMember[] }
  | { type: "member_joined"; member: RoomMember }
  | { type: "member_left"; userID: string; kicked?: boolean }
  | { type: "closed"; reason: string }
  | { type: "kicked"; reason: string }
  | { type: "typing"; typing: RoomTyping }
  | { type: "reaction"; reaction: RoomReaction }
  | { type: "host_disconnected"; deadlineMs: number; graceSeconds: number }
  | { type: "host_reconnected" }
  | { type: "episode_change"; episodeID: string; episodeTitle: string }
  | { type: "episode_request"; userID: string; userName: string; targetEpisodeID?: string; reason: string }
  | { type: "theme_change"; from: string; to: string }
  | { type: "error"; message: string };

export type UseRoomSocketResult = {
  connected: boolean;
  state: RoomSyncState | null;
  events: RoomEvent[];
  sendHostAction: (action: "play" | "pause" | "seek", position: number) => void;
  sendChat: (kind: "text" | "emoji", payload: string) => void;
  sendTyping: (isTyping: boolean) => void;
  sendReaction: (emoji: string) => void;
  sendKick: (userID: string) => void;
  sendEpisodeRequest: (targetEpisodeID: string, reason: "next" | "prev" | "ended" | "manual") => void;
  close: () => void;
};

// useRoomSocket connects to /ws/rooms/:id and exposes the live state plus an
// event stream the consuming component can drain to update UI (chat,
// member list, errors, etc.).
export function useRoomSocket(
  roomID: string | null,
  token: string | null,
  inviteCode?: string,
): UseRoomSocketResult {
  const [connected, setConnected] = useState(false);
  const [state, setState] = useState<RoomSyncState | null>(null);
  const [events, setEvents] = useState<RoomEvent[]>([]);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectRef = useRef(0);
  const closedByUserRef = useRef(false);

  const pushEvent = useCallback((e: RoomEvent) => {
    setEvents((prev) => {
      const next = [...prev, e];
      // Bound the buffer so memory doesn't grow unbounded over a long session.
      return next.length > 500 ? next.slice(-500) : next;
    });
  }, []);

  const connect = useCallback(() => {
    if (!roomID || !token) return;
    const url = getRoomWebSocketURL(roomID, token, inviteCode);
    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => {
      setConnected(true);
      reconnectRef.current = 0;
    };
    ws.onclose = () => {
      setConnected(false);
      if (closedByUserRef.current) return;
      // Exponential backoff reconnect, capped at 10s.
      const delay = Math.min(10_000, 500 * Math.pow(2, reconnectRef.current));
      reconnectRef.current += 1;
      setTimeout(() => {
        if (!closedByUserRef.current) connect();
      }, delay);
    };
    ws.onerror = () => {
      // onclose fires next, handle reconnect there.
    };
    ws.onmessage = (evt) => {
      let msg: { type?: string; payload?: Record<string, unknown> } = {};
      try {
        msg = JSON.parse(evt.data);
      } catch {
        return;
      }
      const p = msg.payload || {};
      switch (msg.type) {
        case "state_sync": {
          const next: RoomSyncState = {
            position: Number(p.position) || 0,
            isPlaying: Boolean(p.is_playing),
            asOfMs: Number(p.as_of_ms) || Date.now(),
          };
          setState(next);
          pushEvent({ type: "state", state: next });
          break;
        }
        case "chat_message":
          pushEvent({
            type: "chat",
            chat: {
              userID: String(p.user_id || ""),
              userName: String(p.user_name || ""),
              userAvatar: typeof p.user_avatar === "string" ? p.user_avatar : undefined,
              kind: (p.kind as "text" | "emoji") || "text",
              text: typeof p.text === "string" ? p.text : undefined,
              emoji: typeof p.emoji === "string" ? p.emoji : undefined,
              createdAt: String(p.created_at || new Date().toISOString()),
            },
          });
          break;
        case "member_snapshot": {
          const arr = Array.isArray(p.members) ? (p.members as Array<Record<string, unknown>>) : [];
          pushEvent({
            type: "member_snapshot",
            members: arr.map((m) => ({
              userID: String(m.user_id || ""),
              userName: String(m.user_name || ""),
              userAvatar: typeof m.user_avatar === "string" ? m.user_avatar : undefined,
              isHost: Boolean(m.is_host),
            })),
          });
          break;
        }
        case "member_joined":
          pushEvent({
            type: "member_joined",
            member: {
              userID: String(p.user_id || ""),
              userName: String(p.user_name || ""),
              userAvatar: typeof p.user_avatar === "string" ? p.user_avatar : undefined,
              isHost: Boolean(p.is_host),
            },
          });
          break;
        case "member_left":
          pushEvent({
            type: "member_left",
            userID: String(p.user_id || ""),
            kicked: Boolean(p.kicked),
          });
          break;
        case "room_closed":
          pushEvent({ type: "closed", reason: String(p.reason || "closed") });
          closedByUserRef.current = true;
          ws.close();
          break;
        case "kicked":
          pushEvent({ type: "kicked", reason: String(p.reason || "kicked") });
          closedByUserRef.current = true;
          ws.close();
          break;
        case "typing":
          pushEvent({
            type: "typing",
            typing: {
              userID: String(p.user_id || ""),
              userName: String(p.user_name || ""),
              isTyping: Boolean(p.is_typing),
            },
          });
          break;
        case "reaction":
          pushEvent({
            type: "reaction",
            reaction: {
              userID: String(p.user_id || ""),
              userName: String(p.user_name || ""),
              emoji: String(p.emoji || ""),
              ts: Number(p.ts) || Date.now(),
            },
          });
          break;
        case "host_disconnected":
          pushEvent({
            type: "host_disconnected",
            deadlineMs: Number(p.deadline_ms) || Date.now(),
            graceSeconds: Number(p.grace_seconds) || 300,
          });
          break;
        case "host_reconnected":
          pushEvent({ type: "host_reconnected" });
          break;
        case "episode_change":
          pushEvent({
            type: "episode_change",
            episodeID: String(p.episode_id || ""),
            episodeTitle: String(p.episode_title || ""),
          });
          break;
        case "episode_request":
          pushEvent({
            type: "episode_request",
            userID: String(p.user_id || ""),
            userName: String(p.user_name || ""),
            targetEpisodeID: typeof p.target_episode_id === "string" ? p.target_episode_id : undefined,
            reason: String(p.reason || ""),
          });
          break;
        case "theme_change":
          pushEvent({
            type: "theme_change",
            from: String(p.from || ""),
            to: String(p.to || ""),
          });
          break;
        case "error":
          pushEvent({ type: "error", message: String(p.error || "") });
          break;
      }
    };
  }, [roomID, token, inviteCode, pushEvent]);

  useEffect(() => {
    closedByUserRef.current = false;
    connect();
    return () => {
      closedByUserRef.current = true;
      wsRef.current?.close();
    };
  }, [connect]);

  const send = useCallback((type: string, payload: Record<string, unknown>) => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({ type, payload }));
  }, []);

  return {
    connected,
    state,
    events,
    sendHostAction: (action, position) => send("host_action", { action, position }),
    sendChat: (kind, payload) => {
      if (kind === "emoji") send("chat_send", { kind: "emoji", emoji: payload });
      else send("chat_send", { kind: "text", text: payload });
    },
    sendTyping: (isTyping: boolean) => send("typing", { is_typing: isTyping }),
    sendReaction: (emoji: string) => send("reaction", { emoji }),
    sendKick: (userID: string) => send("host_kick", { user_id: userID }),
    sendEpisodeRequest: (targetEpisodeID, reason) =>
      send("episode_request", { target_episode_id: targetEpisodeID, reason }),
    close: () => {
      closedByUserRef.current = true;
      wsRef.current?.close();
    },
  };
}

// effectivePosition computes the playhead a guest should be at, given the
// host's last state_sync. Adds elapsed wall-clock time when playing.
export function effectivePosition(state: RoomSyncState | null): number {
  if (!state) return 0;
  if (!state.isPlaying) return state.position;
  const elapsed = (Date.now() - state.asOfMs) / 1000;
  return Math.max(0, state.position + elapsed);
}
