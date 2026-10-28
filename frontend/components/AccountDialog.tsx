"use client";

import { LogOut, Send, ShieldCheck } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";
import { account } from "@/lib/api";
import { openExternal } from "@/lib/telegram";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Dialog, DialogBody, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Switch } from "@/components/ui/switch";
import { useAuth, useHandshake } from "./Auth";

const field =
  "h-9 w-full rounded-md border border-input bg-background px-3 text-[14px] outline-none transition-colors placeholder:text-muted-foreground focus:border-ring focus:ring-3 focus:ring-ring/15";

/** The account: how they sign in (Telegram, login + password, 2FA) and signing out. */
export function AccountDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const { user, info, inTelegram, setUser, signOut } = useAuth();
  const [login, setLogin] = useState(user.login);
  const [email, setEmail] = useState(user.email);
  const [pw, setPw] = useState("");
  const [cur, setCur] = useState("");
  const [busy, setBusy] = useState(false);
  const link = useHandshake((r) => {
    if (r.user) setUser(r.user);
    toast.success("Telegram linked");
  });

  useEffect(() => {
    if (open) {
      setLogin(user.login);
      setEmail(user.email);
      setPw("");
      setCur("");
    }
  }, [open, user.login, user.email]);

  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    try {
      const b: Parameters<typeof account.update>[0] = {};
      if (login !== user.login) b.login = login.trim();
      if (email !== user.email) b.email = email.trim();
      if (pw) {
        b.password = pw;
        b.current_password = cur;
      }
      if (Object.keys(b).length === 0) return;
      const r = await account.update(b);
      setUser(r.user);
      setPw("");
      setCur("");
      toast.success(b.password ? "Saved. Other devices were signed out." : "Saved");
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const startLink = async () => {
    const win = inTelegram ? null : window.open("about:blank", "_blank");
    try {
      const url = await link.start("link");
      if (win) win.location.href = url;
      else openExternal(url);
    } catch (e) {
      win?.close();
      toast.error((e as Error).message);
    }
  };

  const set2FA = async (on: boolean) => {
    try {
      setUser((await account.update({ tg_2fa: on })).user);
      toast.success(on ? "Password sign-ins now need your OK in Telegram" : "Two-step sign-in is off");
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  const unlink = async () => {
    if (!confirm("Unlink Telegram? You will sign in with your login and password, and alerts stop.")) return;
    try {
      setUser((await account.unlinkTelegram()).user);
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Account</DialogTitle>
          <DialogDescription>
            {user.label} · {user.plan === "pro" ? "Pro" : "Free"} plan{user.role === "admin" ? " · admin" : ""}
          </DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-7">
          <section>
            <h3 className="mb-3 text-[14px] font-medium">Telegram</h3>
            {user.tg_id ? (
              <div className="space-y-4">
                <div className="flex items-center gap-3">
                  <Avatar photo={user.tg_photo} label={user.tg_name || user.label} />
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-[14px] font-medium">{user.tg_name || "Telegram"}</div>
                    <div className="text-[13px] text-muted-foreground">{user.tg_username ? `@${user.tg_username}` : `id ${user.tg_id}`}</div>
                  </div>
                  {user.has_password && (
                    <Button variant="ghost" size="sm" onClick={unlink} className="text-muted-foreground">
                      Unlink
                    </Button>
                  )}
                </div>
                <label className={cn("flex items-start justify-between gap-4", !user.has_password && "opacity-60")}>
                  <span>
                    <span className="flex items-center gap-1.5 text-[14px]">
                      <ShieldCheck className="size-4 text-primary" />
                      Two-step sign-in
                    </span>
                    <span className="mt-0.5 block text-[12.5px] leading-snug text-muted-foreground">
                      {user.has_password ? "Password sign-ins wait for your Approve in Telegram." : "Set a password below to use this."}
                    </span>
                  </span>
                  <Switch checked={user.tg_2fa} disabled={!user.has_password} onCheckedChange={set2FA} />
                </label>
              </div>
            ) : info.telegram ? (
              <div className="space-y-2.5">
                <p className="text-[13.5px] leading-snug text-muted-foreground">Link Telegram to sign in with one tap, open the app inside Telegram, get alerts, and pay with Stars.</p>
                {link.status === "idle" || link.status === "denied" || link.status === "expired" ? (
                  <Button variant="outline" onClick={startLink} className="gap-2">
                    <Send className="size-4" />
                    Link Telegram
                  </Button>
                ) : (
                  <div className="flex flex-wrap items-center gap-2 text-[13.5px]">
                    <span className="text-muted-foreground">{link.status === "pending" ? `Press Start in @${info.bot}, then Approve.` : "Tap Approve in Telegram."}</span>
                    {link.url && (
                      <button onClick={() => openExternal(link.url!)} className="font-medium text-primary hover:underline">
                        Open @{info.bot}
                      </button>
                    )}
                    <button onClick={link.cancel} className="text-muted-foreground hover:text-foreground">
                      Cancel
                    </button>
                  </div>
                )}
                {(link.status === "denied" || link.status === "expired") && <p className="text-[13px] text-destructive">{link.status === "denied" ? "Denied in Telegram." : "That link expired."}</p>}
              </div>
            ) : (
              <p className="text-[13.5px] text-muted-foreground">This install has no Telegram bot.</p>
            )}
          </section>

          <form onSubmit={save} className="space-y-3">
            <h3 className="text-[14px] font-medium">Login and password</h3>
            {!user.has_password && (
              <p className="text-[13px] leading-snug text-muted-foreground">You sign in with Telegram. Add a login and password to also sign in without it.</p>
            )}
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="block">
                <span className="mb-1 block text-[12.5px] text-muted-foreground">Login</span>
                <input className={field} value={login} onChange={(e) => setLogin(e.target.value)} autoComplete="username" placeholder="pick one" />
              </label>
              <label className="block">
                <span className="mb-1 block text-[12.5px] text-muted-foreground">Email</span>
                <input className={field} type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" placeholder="optional" />
              </label>
              {user.has_password && (
                <label className="block">
                  <span className="mb-1 block text-[12.5px] text-muted-foreground">Current password</span>
                  <input className={field} type="password" value={cur} onChange={(e) => setCur(e.target.value)} autoComplete="current-password" />
                </label>
              )}
              <label className="block">
                <span className="mb-1 block text-[12.5px] text-muted-foreground">{user.has_password ? "New password" : "Password"}</span>
                <input className={field} type="password" value={pw} onChange={(e) => setPw(e.target.value)} autoComplete="new-password" minLength={8} placeholder="8+ characters" />
              </label>
            </div>
            <Button type="submit" disabled={busy || (login === user.login && email === user.email && !pw)} className="h-9">
              Save
            </Button>
          </form>

          <section className="flex flex-wrap items-center justify-between gap-3 border-t border-border pt-5">
            <Button
              variant="ghost"
              className="h-9 px-2 text-muted-foreground"
              onClick={async () => {
                await account.logoutOthers();
                toast.success("Signed out on every other device");
              }}
            >
              Sign out other devices
            </Button>
            {!inTelegram && (
              <Button variant="outline" className="h-9 gap-2" onClick={signOut}>
                <LogOut className="size-4" />
                Sign out
              </Button>
            )}
          </section>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}

export function Avatar({ photo, label, className }: { photo?: string; label: string; className?: string }) {
  const [broken, setBroken] = useState(false);
  if (photo && !broken)
    // eslint-disable-next-line @next/next/no-img-element
    return <img src={photo} alt="" onError={() => setBroken(true)} className={cn("size-9 shrink-0 rounded-full object-cover", className)} />;
  return (
    <span className={cn("grid size-9 shrink-0 place-items-center rounded-full bg-accent text-[14px] font-semibold text-accent-foreground uppercase", className)}>
      {label.replace(/^@/, "").slice(0, 1) || "?"}
    </span>
  );
}
