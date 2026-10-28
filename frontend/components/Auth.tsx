"use client";

import { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";
import { useTheme } from "next-themes";
import { account, authLost, setBearer, type AuthInfo, type HandshakeStatus, type User } from "@/lib/api";
import { loadMiniApp } from "@/lib/telegram";
import { SignIn } from "./SignIn";

interface AuthCtx {
  user: User;
  info: AuthInfo;
  /** inside the Telegram Mini App */
  inTelegram: boolean;
  setUser: (u: User) => void;
  refresh: () => Promise<void>;
  signOut: () => Promise<void>;
}

const Ctx = createContext<AuthCtx | null>(null);

export function useAuth() {
  const c = useContext(Ctx);
  if (!c) throw new Error("useAuth outside AuthGate");
  return c;
}

/**
 * Shows the app to a signed-in account and the sign-in screen to everyone else. Inside Telegram it
 * signs in by itself from the Mini App's launch data: opening the app from the bot is the login.
 */
export function AuthGate({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<{ user: User | null; info: AuthInfo | null; inTelegram: boolean; error?: string } | null>(null);
  const { setTheme } = useTheme();

  const load = useCallback(async () => {
    const tg = await loadMiniApp();
    if (tg) {
      tg.ready();
      tg.expand();
      setTheme(tg.colorScheme);
      tg.onEvent("themeChanged", () => setTheme(tg.colorScheme));
      try {
        const [s, me] = await Promise.all([account.webApp(tg.initData), account.me()]);
        setBearer(s.token);
        setState({ user: s.user, info: me.auth, inTelegram: true });
        return;
      } catch (e) {
        const me = await account.me().catch(() => null);
        setState({ user: null, info: me?.auth ?? null, inTelegram: true, error: `Telegram sign-in failed: ${(e as Error).message}` });
        return;
      }
    }
    try {
      const me = await account.me();
      setState({ user: me.user, info: me.auth, inTelegram: false });
    } catch (e) {
      setState({ user: null, info: null, inTelegram: false, error: `Cannot reach the backend: ${(e as Error).message}` });
    }
  }, [setTheme]);

  useEffect(() => {
    load();
  }, [load]);

  // a 401 anywhere means the session ended (signed out elsewhere, expired): back to sign-in
  useEffect(() => {
    const lost = () => setState((s) => (s && s.user ? { ...s, user: null } : s));
    authLost.addEventListener("lost", lost);
    return () => authLost.removeEventListener("lost", lost);
  }, []);

  if (!state) return <div className="min-h-dvh bg-background" />;
  if (!state.user)
    return (
      <SignIn
        info={state.info}
        error={state.error}
        onRetry={load}
        onSignedIn={(u, token) => {
          if (token && state.inTelegram) setBearer(token);
          setState({ ...state, user: u, error: undefined });
        }}
      />
    );

  const ctx: AuthCtx = {
    user: state.user,
    info: state.info!,
    inTelegram: state.inTelegram,
    setUser: (u) => setState((s) => s && { ...s, user: u }),
    refresh: async () => {
      const me = await account.me();
      if (me.user) setState((s) => s && { ...s, user: me.user });
    },
    signOut: async () => {
      await account.logout().catch(() => {});
      setBearer(null);
      setState((s) => s && { ...s, user: null });
    },
  };
  // key: a different account gets a fresh dashboard, not the last one's state
  return (
    <Ctx.Provider value={ctx} key={state.user.id}>
      {children}
    </Ctx.Provider>
  );
}

/**
 * Drives a bot handshake: start() opens the t.me link and polls until the person answers in
 * Telegram. Used for "Continue with Telegram", linking Telegram, and the 2FA approval.
 */
export function useHandshake(onDone: (r: { user?: User; token?: string }) => void) {
  const [status, setStatus] = useState<HandshakeStatus | "idle">("idle");
  const [url, setUrl] = useState<string | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const done = useRef(onDone);
  done.current = onDone;

  const stop = useCallback(() => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = null;
  }, []);

  const poll = useCallback(
    (secret: string, until: number) => {
      stop();
      const tick = async () => {
        if (Date.now() > until) {
          setStatus("expired");
          return;
        }
        try {
          const r = await account.tgPoll(secret);
          setStatus(r.status);
          if (r.status === "ok") return done.current(r);
          if (r.status === "denied" || r.status === "expired") return;
        } catch {}
        timer.current = setTimeout(tick, 1500);
      };
      timer.current = setTimeout(tick, 1200);
    },
    [stop],
  );

  /** a bot link: returns the URL so the caller can open it inside the click (popup blockers) */
  const start = useCallback(
    async (purpose: "login" | "link") => {
      const r = await account.tgStart(purpose);
      setUrl(r.url);
      setStatus("pending");
      poll(r.secret, Date.now() + r.expires_in * 1000);
      return r.url;
    },
    [poll],
  );

  /** a 2FA challenge from a password sign-in: the bot already asked */
  const wait = useCallback(
    (secret: string, expiresIn: number) => {
      setUrl(null);
      setStatus("waiting");
      poll(secret, Date.now() + expiresIn * 1000);
    },
    [poll],
  );

  const cancel = useCallback(() => {
    stop();
    setStatus("idle");
    setUrl(null);
  }, [stop]);

  useEffect(() => stop, [stop]);
  return { status, url, start, wait, cancel };
}
