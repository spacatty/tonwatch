"use client";

import { Coins, Languages, LifeBuoy, LogOut, Monitor, Moon, Sun, UserRound } from "lucide-react";
import { useTheme } from "next-themes";
import { LANGS, setLang, useLang, useT } from "@/lib/i18n";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Avatar } from "./AccountDialog";
import { useAuth } from "./Auth";
import { useSupportUnread } from "./Support";

const THEMES = [
  { id: "light", icon: Sun },
  { id: "dark", icon: Moon },
  { id: "system", icon: Monitor },
] as const;

/** The account button at the foot of the sidebar: one menu for the account, credits, theme and language.
 *  Inside Telegram the theme follows Telegram and signing out makes no sense, so both are left out. */
export function ProfileMenu({ onAccount, onCredits, onSupport }: { onAccount: () => void; onCredits: () => void; onSupport: () => void }) {
  const { user, inTelegram, signOut } = useAuth();
  const { theme, setTheme } = useTheme();
  const lang = useLang();
  const t = useT().sidebar;
  const ts = useT().support;
  const unread = useSupportUnread();

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<button className="flex min-w-0 flex-1 items-center gap-2.5 rounded-md px-1 py-1 text-left transition-colors hover:bg-muted hover:text-foreground data-popup-open:bg-muted" title={t.account} />}
      >
        <span className="relative shrink-0">
          <Avatar photo={user.tg_photo} label={user.label} className="size-7 text-[12px]" />
          {unread > 0 && <span className="absolute -top-0.5 -right-0.5 size-2.5 rounded-full border-2 border-background bg-primary" />}
        </span>
        <span className="min-w-0">
          <span className="block truncate text-[13px] font-medium text-foreground">{user.label}</span>
          <span className="block text-[11.5px]">{t.plan(user.plan === "pro")}</span>
        </span>
      </DropdownMenuTrigger>
      <DropdownMenuContent side="top" align="start" sideOffset={8} className="w-60">
        <DropdownMenuGroup>
          <DropdownMenuLabel className="flex items-center gap-2.5 py-2 text-foreground">
            <Avatar photo={user.tg_photo} label={user.label} className="size-8 text-[13px]" />
            <span className="min-w-0">
              <span className="block truncate text-[13px] font-medium">{user.label}</span>
              <span className="block text-[11.5px] font-normal text-muted-foreground">{t.plan(user.plan === "pro")}</span>
            </span>
          </DropdownMenuLabel>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={onAccount}>
          <UserRound />
          {t.account}
        </DropdownMenuItem>
        <DropdownMenuItem onClick={onCredits}>
          <Coins />
          {t.credits}
        </DropdownMenuItem>
        <DropdownMenuItem onClick={onSupport}>
          <LifeBuoy />
          {ts.menu}
          {unread > 0 && <span className="data ml-auto rounded-full bg-primary px-1.5 text-[11px] font-medium text-primary-foreground">{unread}</span>}
        </DropdownMenuItem>

        {!inTelegram && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuLabel>{t.theme}</DropdownMenuLabel>
              <DropdownMenuRadioGroup value={theme ?? "system"} onValueChange={(v) => setTheme(v as string)}>
                {THEMES.map(({ id, icon: Icon }) => (
                  <DropdownMenuRadioItem key={id} value={id} closeOnClick={false}>
                    <Icon />
                    {t.themes[id]}
                  </DropdownMenuRadioItem>
                ))}
              </DropdownMenuRadioGroup>
            </DropdownMenuGroup>
          </>
        )}

        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuLabel className="flex items-center gap-1.5">
            <Languages className="size-3.5" />
            {t.language}
          </DropdownMenuLabel>
          <DropdownMenuRadioGroup value={lang} onValueChange={(v) => setLang(v as typeof lang)}>
            {LANGS.map((l) => (
              <DropdownMenuRadioItem key={l.id} value={l.id} closeOnClick={false}>
                <span className="data w-6 text-[11px] text-muted-foreground">{l.short}</span>
                {l.label}
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
        </DropdownMenuGroup>

        {!inTelegram && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={() => signOut()} className="text-destructive focus:text-destructive">
              <LogOut />
              {t.signOut}
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
