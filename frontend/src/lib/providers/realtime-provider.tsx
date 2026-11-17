"use client";

import * as React from "react";
import { useQueryClient } from "@tanstack/react-query";
import { chatKeys, prependMessage } from "@/lib/hooks/use-chat";
import { notificationKeys } from "@/lib/hooks/use-notifications";
import { unreadKey } from "@/lib/hooks/use-unread";
import { useSession } from "@/lib/providers/session-provider";
import type { RealtimeFrame, RealtimeListener } from "@/lib/types";

type Status = "connecting" | "live" | "offline";

type RealtimeValue = {
  status: Status;
  subscribe: (listener: RealtimeListener) => () => void;
};

const RealtimeContext = React.createContext<RealtimeValue | null>(null);

const STREAM_ENDPOINT = "/api/stream";

/** The two named events the gateway writes. Both arrive on one connection. */
const STREAM_EVENTS = ["notifications", "chat"] as const;

/**
 * One Server-Sent Events connection for the whole app.
 *
 * A single connection serves both streams because the gateway relays them over
 * one Redis subscription, and a per-page connection would multiply the open
 * sockets by the number of mounted screens.
 *
 * `EventSource` parses the SSE wire format itself, so each listener receives a
 * complete, already-decoded `data` payload. Reconnection is delegated to
 * `EventSource`, which retries on the server's `retry:` interval.
 *
 * This connection only makes updates faster. The query hooks keep their
 * polling as the floor, so a backgrounded tab — where a browser may drop the
 * socket without an `error` event — still converges.
 */
export function RealtimeProvider({ children }: { children: React.ReactNode }) {
  const queryClient = useQueryClient();
  const { status: sessionStatus } = useSession();

  const listeners = React.useRef(new Set<RealtimeListener>());
  const [status, setStatus] = React.useState<Status>("connecting");

  const dispatch = React.useCallback((frame: RealtimeFrame) => {
    for (const listener of listeners.current) listener(frame);
  }, []);

  React.useEffect(() => {
    if (sessionStatus !== "authenticated") {
      setStatus("offline");
      return;
    }

    const source = new EventSource(STREAM_ENDPOINT, { withCredentials: true });

    const onOpen = () => setStatus("live");
    const onError = () => setStatus("connecting");

    const onFrame = (event: MessageEvent<string>) => {
      let frame: RealtimeFrame;
      try {
        frame = JSON.parse(event.data) as RealtimeFrame;
      } catch {
        // A malformed frame is dropped; the next one carries the same state.
        return;
      }
      setStatus("live");
      dispatch(frame);
    };

    source.addEventListener("open", onOpen);
    source.addEventListener("error", onError);
    for (const name of STREAM_EVENTS) {
      source.addEventListener(name, onFrame);
    }

    return () => {
      source.removeEventListener("open", onOpen);
      source.removeEventListener("error", onError);
      for (const name of STREAM_EVENTS) {
        source.removeEventListener(name, onFrame);
      }
      source.close();
      setStatus("offline");
    };
  }, [sessionStatus, dispatch]);

  // The cache writes live in one effect so a screen mounting two hooks cannot
  // register the same patch twice.
  React.useEffect(() => {
    /**
     * The frame carries the notification-service's authoritative unread count,
     * so the badge moves immediately without a round trip. The list itself is
     * refetched rather than hand-patched: a filtered tab (Mentions, Follows)
     * must not gain a row it would never show.
     */
    const applyNotification = (frame: RealtimeFrame) => {
      if (frame.stream !== "notifications") return;
      // The backend marshals this field as `unread_count`. Reading the camelCase
      // spelling silently writes undefined over a real badge count.
      queryClient.setQueryData<number>(unreadKey, frame.data.unread_count);
      void queryClient.invalidateQueries({ queryKey: notificationKeys.list });
    };

    /**
     * A new message invalidates the inbox, so the per-thread unread count and
     * the total come back from the server instead of being re-derived from a
     * heuristics. The open thread, when there is one, gets the row appended
     * locally so the transcript updates before the refetch lands.
     */
    const applyChat = (frame: RealtimeFrame) => {
      if (frame.stream !== "chat") return;
      if (frame.type !== "message.created" && frame.type !== "message.deleted") {
        return;
      }

      const conversationID = frame.data.conversation_id;
      if (!conversationID) return;

      if (frame.type === "message.deleted") {
        void queryClient.invalidateQueries({ queryKey: chatKeys.conversations });
        return;
      }

      const row = {
        id: frame.data.message_id ?? "",
        conversationId: conversationID,
        senderId: frame.data.sender_id ?? "",
        content: frame.data.content ?? "",
        createdAt: new Date(frame.receivedAt).toISOString(),
        isDeleted: false,
        sender: undefined,
      };

      queryClient.setQueryData<{ messages: { id: string }[] }>(
        chatKeys.messages(conversationID),
        (current) => prependMessage(current, row),
      );

      void queryClient.invalidateQueries({ queryKey: chatKeys.conversations });
    };

    const listener = (frame: RealtimeFrame) => {
      applyNotification(frame);
      applyChat(frame);
    };

    listeners.current.add(listener);
    return () => {
      listeners.current.delete(listener);
    };
  }, [queryClient]);

  const subscribe = React.useCallback((listener: RealtimeListener) => {
    listeners.current.add(listener);
    return () => {
      listeners.current.delete(listener);
    };
  }, []);

  const value = React.useMemo(() => ({ status, subscribe }), [status, subscribe]);

  return (
    <RealtimeContext.Provider value={value}>{children}</RealtimeContext.Provider>
  );
}

export function useRealtime(): RealtimeValue {
  const context = React.useContext(RealtimeContext);
  if (!context) {
    throw new Error("useRealtime must be used inside RealtimeProvider");
  }
  return context;
}