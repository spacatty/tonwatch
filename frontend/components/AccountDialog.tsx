"use client";

import { LogOut } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { account } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Dialog, DialogBody, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { useAuth } from "./Auth";
import { LangSwitch } from "./LangSwitch";

/** The account: the Telegram identity it signs in with, and signing out. */
export function AccountDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const { user, inTelegram, signOut } = useAuth();
  const t = useT().account;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t.title}</DialogTitle>
          <DialogDescription>
            {user.label} · {t.plan(user.plan === "pro")}
            {user.role === "admin" ? t.admin : ""}
          </DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-7">
          <section>
            <h3 className="mb-3 text-[14px] font-medium">{t.signedInWith}</h3>
            <div className="flex items-center gap-3">
              <Avatar photo={user.tg_photo} label={user.tg_name || user.label} />
              <div className="min-w-0 flex-1">
                <div className="truncate text-[14px] font-medium">{user.tg_name || "Telegram"}</div>
                <div className="text-[13px] text-muted-foreground">{user.tg_username ? `@${user.tg_username}` : `id ${user.tg_id}`}</div>
              </div>
            </div>
          </section>

          <section className="border-t border-border pt-5">
            <h3 className="mb-1 text-[14px] font-medium">{t.language}</h3>
            <p className="mb-3 text-[13px] text-muted-foreground">{t.languageHint}</p>
            <LangSwitch full />
          </section>

          <section className="flex flex-wrap items-center justify-between gap-3 border-t border-border pt-5">
            <Button
              variant="ghost"
              className="h-9 px-2 text-muted-foreground"
              onClick={async () => {
                await account.logoutOthers();
                toast.success(t.signedOutOthers);
              }}
            >
              {t.signOutOthers}
            </Button>
            {!inTelegram && (
              <Button variant="outline" className="h-9 gap-2" onClick={signOut}>
                <LogOut className="size-4" />
                {t.signOut}
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
