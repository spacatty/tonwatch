"use client";

import { Gift, Loader2 } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { creditsChanged, studio, type Credits } from "@/lib/api";
import { getT, num, useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";

type TurnstileApi = {
  render: (el: HTMLElement, o: { sitekey: string; callback: (token: string) => void; "error-callback"?: () => void; theme?: string; language?: string }) => string;
  remove: (id: string) => void;
};
declare global {
  interface Window {
    turnstile?: TurnstileApi;
  }
}

let turnstileScript: Promise<TurnstileApi> | null = null;
function loadTurnstile(): Promise<TurnstileApi> {
  turnstileScript ??= new Promise((resolve, reject) => {
    const s = document.createElement("script");
    s.src = "https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit";
    s.async = true;
    s.onload = () => (window.turnstile ? resolve(window.turnstile) : reject(new Error("turnstile")));
    s.onerror = () => {
      turnstileScript = null;
      reject(new Error("turnstile"));
    };
    document.head.appendChild(s);
  });
  return turnstileScript;
}

const HOLD_MS = 1200;

/** Today's free credits, claimed by hand: a Turnstile check when the install has keys, else press and hold. */
export function ClaimDaily({ c, onClaimed }: { c: Credits; onClaimed: (c: Credits) => void }) {
  const t = useT().credits;
  const [busy, setBusy] = useState(false);
  const [checking, setChecking] = useState(false);

  const claim = async (token = "") => {
    setBusy(true);
    try {
      const r = await studio.claim(token);
      onClaimed(r.credits);
      toast.success(r.added ? getT().credits.claimed(num(r.added)) : getT().credits.claimedNone);
      creditsChanged.dispatchEvent(new Event("change"));
    } catch (e) {
      toast.error((e as Error).message);
      setChecking(false);
    } finally {
      setBusy(false);
    }
  };

  if (!c.claim.available) return null;
  return (
    <div className="rounded-lg border border-primary/40 bg-primary/5 px-4 py-3.5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-[14px] font-medium">
            <Gift className="size-4 text-primary" />
            {t.claimTitle(num(c.daily))}
          </div>
          <p className="mt-1 text-[12.5px] leading-snug text-muted-foreground">{c.claim.adds ? t.claimBody(num(c.claim.adds)) : t.claimFull}</p>
        </div>
        {c.claim.turnstile ? (
          !checking && (
            <button onClick={() => setChecking(true)} className="h-9 rounded-md bg-primary px-4 text-[13.5px] font-medium text-primary-foreground hover:bg-primary/90">
              {t.claim}
            </button>
          )
        ) : (
          <HoldButton busy={busy} onDone={() => claim()} />
        )}
      </div>
      {c.claim.turnstile && checking && <Turnstile sitekey={c.claim.turnstile} busy={busy} onToken={claim} />}
    </div>
  );
}

function Turnstile({ sitekey, busy, onToken }: { sitekey: string; busy: boolean; onToken: (token: string) => void }) {
  const t = useT().credits;
  const el = useRef<HTMLDivElement>(null);
  const done = useRef(onToken);
  done.current = onToken;
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    let id: string | null = null;
    let alive = true;
    loadTurnstile()
      .then((ts) => {
        if (!alive || !el.current) return;
        id = ts.render(el.current, {
          sitekey,
          theme: document.documentElement.classList.contains("dark") ? "dark" : "light",
          callback: (token) => done.current(token),
          "error-callback": () => setFailed(true),
        });
      })
      .catch(() => alive && setFailed(true));
    return () => {
      alive = false;
      if (id) window.turnstile?.remove(id);
    };
  }, [sitekey]);
  return (
    <div className="mt-3 flex min-h-[65px] items-center gap-3">
      <div ref={el} />
      {busy && <Loader2 className="size-4 animate-spin text-muted-foreground" />}
      {failed && <span className="text-[12.5px] text-destructive">{t.checkFailed}</span>}
    </div>
  );
}

/** Press and hold to claim: a moment of attention rather than a stray tap. */
function HoldButton({ busy, onDone }: { busy: boolean; onDone: () => void }) {
  const t = useT().credits;
  const [holding, setHolding] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const start = () => {
    if (busy) return;
    setHolding(true);
    timer.current = setTimeout(() => {
      setHolding(false);
      onDone();
    }, HOLD_MS);
  };
  const stop = () => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = null;
    setHolding(false);
  };
  return (
    <button
      onPointerDown={start}
      onPointerUp={stop}
      onPointerLeave={stop}
      onPointerCancel={stop}
      onContextMenu={(e) => e.preventDefault()}
      disabled={busy}
      className="relative h-9 touch-none select-none overflow-hidden rounded-md bg-primary px-4 text-[13.5px] font-medium text-primary-foreground disabled:opacity-60"
    >
      <span
        className={cn("absolute inset-y-0 left-0 bg-primary-foreground/25", holding ? "w-full transition-[width] ease-linear" : "w-0")}
        style={{ transitionDuration: holding ? `${HOLD_MS}ms` : "0ms" }}
      />
      <span className="relative flex items-center gap-2">
        {busy && <Loader2 className="size-3.5 animate-spin" />}
        {t.holdToClaim}
      </span>
    </button>
  );
}
