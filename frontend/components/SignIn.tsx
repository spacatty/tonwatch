"use client";

import { Loader2, Send, ShieldCheck } from "lucide-react";
import { motion, MotionConfig, type Variants } from "motion/react";
import dynamic from "next/dynamic";
import { useState } from "react";
import type { AuthInfo, User } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { useHandshake } from "./Auth";
import { Mark } from "./bits";
import { LangSwitch } from "./LangSwitch";

// three.js only loads on the sign-in screen, and only in the browser
const HeroScene = dynamic(() => import("./HeroScene").then((m) => m.HeroScene), { ssr: false });

interface Props {
  info: AuthInfo | null;
  error?: string;
  onRetry: () => void;
  onSignedIn: (u: User, token?: string) => void;
}

// the form rises in after the globe, one piece at a time
const rise: Variants = {
  hidden: { opacity: 0, y: 14, filter: "blur(6px)" },
  show: { opacity: 1, y: 0, filter: "blur(0px)", transition: { duration: 0.55, ease: [0.22, 1, 0.36, 1] } },
};
const stagger: Variants = { show: { transition: { staggerChildren: 0.08, delayChildren: 0.1 } } };

/** The front door: Telegram is the only way in (one tap, nothing to remember). */
export function SignIn({ info, error, onRetry, onSignedIn }: Props) {
  const [msg, setMsg] = useState<string | null>(null);
  const t = useT();
  const s = t.signIn;
  const hs = useHandshake((r) => r.user && onSignedIn(r.user, r.token));

  const telegram = async () => {
    setMsg(null);
    // open the tab inside the click, then point it at the bot: popup blockers allow that
    const win = window.open("about:blank", "_blank");
    try {
      const url = await hs.start();
      if (win) win.location.href = url;
    } catch (e) {
      win?.close();
      setMsg((e as Error).message);
    }
  };

  return (
    <div className="grid min-h-dvh bg-background lg:grid-cols-[1.05fr_1fr]">
      <aside className="relative hidden overflow-hidden border-r border-border bg-sidebar lg:block">
        <HeroScene className="absolute inset-0" />
        {/* keeps the copy readable where it overlaps the globe */}
        <div className="pointer-events-none absolute inset-0 bg-gradient-to-t from-sidebar via-sidebar/70 via-30% to-transparent to-55%" />
        <div className="pointer-events-none relative flex h-full flex-col justify-between p-14">
          <div className="flex items-center gap-2.5">
            <Mark className="size-8" />
            <span className="text-lg font-semibold">TonWatch</span>
          </div>
          <div className="max-w-[30rem]">
            <h1 className="text-[40px] leading-[1.08] font-bold tracking-tight">{s.headline}</h1>
            <p className="mt-5 text-[16px] leading-relaxed text-muted-foreground">
              {s.pitch}
            </p>
          </div>
        </div>
      </aside>

      <main className="auth-stage relative flex items-start justify-center overflow-hidden px-4 pb-10 sm:px-8 lg:items-center lg:py-10">
        <div aria-hidden className="auth-dots pointer-events-none absolute inset-0" />
        <LangSwitch className="absolute top-3 right-4 z-10 bg-background/60 backdrop-blur sm:right-8 lg:top-6" />
        <MotionConfig reducedMotion="user">
          <motion.div className="relative w-full max-w-[420px]" initial="hidden" animate="show" variants={stagger}>
            <div className="relative -mx-4 mb-4 h-56 overflow-hidden sm:-mx-8 lg:hidden">
              <HeroScene className="absolute inset-0" density={4500} />
              <div className="pointer-events-none absolute inset-x-0 bottom-0 h-16 bg-gradient-to-t from-background to-transparent" />
              <div className="pointer-events-none absolute top-3 left-4 flex items-center gap-2.5 sm:left-8">
                <Mark className="size-7" />
                <span className="text-[17px] font-semibold">TonWatch</span>
              </div>
            </div>

            <motion.div variants={rise} className="mb-6">
              <span className="data inline-flex items-center gap-2 rounded-full border border-primary/30 bg-primary/10 px-2.5 py-1 text-[11.5px] tracking-wide text-primary uppercase">
                <span className="relative flex size-1.5">
                  <span className="absolute inline-flex size-full animate-ping rounded-full bg-primary opacity-70" />
                  <span className="relative inline-flex size-1.5 rounded-full bg-primary" />
                </span>
                {s.live}
              </span>
              <h2 className="mt-4 bg-gradient-to-br from-foreground via-foreground to-foreground/55 bg-clip-text text-[30px] leading-tight font-bold tracking-tight text-transparent">
                {s.title}
              </h2>
              <p className="mt-1.5 text-[15px] text-muted-foreground">{s.subtitle}</p>
            </motion.div>

            <motion.div variants={rise} className="auth-card p-5 sm:p-7">
              {error && (
                <div className="mb-5 rounded-lg border border-destructive/40 bg-destructive/10 px-3.5 py-3 text-[13.5px]">
                  {error}
                  <button onClick={onRetry} className="ml-2 font-medium text-primary hover:underline">
                    {t.common.retry}
                  </button>
                </div>
              )}

              {hs.status !== "idle" ? (
                <Waiting status={hs.status} url={hs.url} bot={info?.bot ?? ""} onCancel={hs.cancel} onRetry={telegram} />
              ) : info?.telegram ? (
                <div className="space-y-3">
                  <button onClick={telegram} className="auth-btn">
                    <Send className="size-4" />
                    {s.continue}
                  </button>
                  {msg && <p className="text-[13.5px] text-destructive">{msg}</p>}
                </div>
              ) : (
                info && (
                  <p className="text-[14px] leading-snug text-muted-foreground">
                    {s.noBot[0]}
                    <code className="data">TELEGRAM_BOT_TOKEN</code>
                    {s.noBot[1]}
                  </p>
                )
              )}
            </motion.div>

            {info?.telegram && hs.status === "idle" && (
              <motion.p variants={rise} className="mt-5 text-center text-[12.5px] text-muted-foreground">
                {s.newHere}
              </motion.p>
            )}
          </motion.div>
        </MotionConfig>
      </main>
    </div>
  );
}

function Waiting({
  status,
  url,
  bot,
  onCancel,
  onRetry,
}: {
  status: string;
  url: string | null;
  bot: string;
  onCancel: () => void;
  onRetry: () => void;
}) {
  const t = useT();
  const s = t.signIn;
  const over = status === "denied" || status === "expired";
  const text = over ? (status === "denied" ? s.denied : s.expired) : status === "pending" ? s.pressStart(bot) : s.tapApprove;
  return (
    <div>
      <div className="flex items-start gap-3">
        <div className={cn("mt-0.5 rounded-full p-2", over ? "bg-muted text-muted-foreground" : "bg-primary/12 text-primary shadow-[0_0_24px_-6px_var(--primary)]")}>
          {over ? <ShieldCheck className="size-5" /> : <Loader2 className="size-5 animate-spin" />}
        </div>
        <div className="min-w-0">
          <div className="text-[15px] font-medium">{s.continueIn}</div>
          <p className="mt-1 text-[14px] leading-snug text-muted-foreground">{text}</p>
        </div>
      </div>
      <div className="mt-5 flex flex-wrap gap-2">
        {over ? (
          <Button onClick={onRetry} className="h-9">
            {s.startAgain}
          </Button>
        ) : (
          url && (
            <a href={url} target="_blank" rel="noopener noreferrer" className="auth-btn !h-10 !w-auto px-4 text-sm">
              <Send className="size-4" />
              {s.openBot(bot)}
            </a>
          )
        )}
        <Button variant="ghost" onClick={onCancel} className="h-9">
          {over ? t.common.back : t.common.cancel}
        </Button>
      </div>
    </div>
  );
}
