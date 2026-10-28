"use client";

import { Activity, Plus } from "lucide-react";
import { useEffect, useState } from "react";
import { api, type Username } from "@/lib/api";
import { VIEWS, type ViewKey } from "@/lib/views";
import { CommandDialog, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/ui/command";
import { Command } from "@/components/ui/command";
import { StatusPill } from "./bits";

interface Props {
  open: boolean;
  setOpen: (o: boolean) => void;
  onOpenName: (name: string) => void;
  onTrack: (name: string) => void;
  onView: (v: ViewKey) => void;
  /** admins only */
  onJobs?: () => void;
}

export function CommandPalette({ open, setOpen, onOpenName, onTrack, onView, onJobs }: Props) {
  const [q, setQ] = useState("");
  const [hits, setHits] = useState<Username[]>([]);

  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setOpen(!open);
      }
    };
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  }, [open, setOpen]);

  const clean = q.trim().replace(/^@/, "").toLowerCase();
  useEffect(() => {
    if (clean.length < 2) {
      setHits([]);
      return;
    }
    const t = setTimeout(() => {
      api
        .list(new URLSearchParams({ q: `^${clean}`, sort: "len", limit: "8" }))
        .then((r) => setHits(r.items))
        .catch(() => {});
    }, 120);
    return () => clearTimeout(t);
  }, [clean]);

  const go = (fn: () => void) => {
    fn();
    setOpen(false);
    setQ("");
  };
  const valid = /^[a-z0-9_]{4,32}$/.test(clean);

  return (
    <CommandDialog open={open} onOpenChange={setOpen} title="Jump to a username" description="Search usernames, switch views, run collectors">
      <Command shouldFilter={false}>
        <CommandInput autoFocus value={q} onValueChange={setQ} placeholder="Type a username, like darkmoon" />
        <CommandList>
          <CommandEmpty>{valid ? "Not in the database yet." : "Type at least two letters."}</CommandEmpty>
          {hits.length > 0 && (
            <CommandGroup heading="Usernames">
              {hits.map((u) => (
                <CommandItem key={u.name} value={u.name} onSelect={() => go(() => onOpenName(u.name))}>
                  <span className="font-medium">@{u.name}</span>
                  {u.meaning && <span className="truncate text-muted-foreground">{u.meaning}</span>}
                  <span className="ml-auto">
                    <StatusPill u={u} />
                  </span>
                </CommandItem>
              ))}
            </CommandGroup>
          )}
          {valid && !hits.some((h) => h.name === clean) && (
            <CommandGroup heading="Add">
              <CommandItem value={`track-${clean}`} onSelect={() => go(() => onTrack(clean))}>
                <Plus />
                Check @{clean} on Fragment and track it
              </CommandItem>
            </CommandGroup>
          )}
          {clean.length < 2 && (
            <>
              <CommandGroup heading="Go to">
                {VIEWS.map((v) => (
                  <CommandItem key={v.key} value={v.key} onSelect={() => go(() => onView(v.key))}>
                    <v.icon />
                    {v.label}
                  </CommandItem>
                ))}
              </CommandGroup>
              {onJobs && (
                <CommandGroup heading="Data">
                  <CommandItem value="collectors" onSelect={() => go(onJobs)}>
                    <Activity />
                    Open collectors
                  </CommandItem>
                </CommandGroup>
              )}
            </>
          )}
        </CommandList>
      </Command>
    </CommandDialog>
  );
}
