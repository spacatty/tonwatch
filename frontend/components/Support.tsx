"use client";

import { ArrowLeft, CheckCircle2, Loader2, MessageCircle, Plus, RotateCcw, Send } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { adminSupport, support, supportChanged, type SupportMessage, type Ticket, type TicketThread, type User } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Dialog, DialogBody, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import { Avatar } from "./AccountDialog";
import { ago } from "./bits";

// Support requests, from both sides. People write here or in the bot; the team answers in the admin
// panel or by replying in the bot. Both sides poll while open, so an answer typed in Telegram
// shows up here within seconds.

const POLL = 8000;

/** Re-runs `fn` every few seconds while the page is visible. */
function usePoll(fn: () => void, on: boolean) {
  useEffect(() => {
    if (!on) return;
    const id = setInterval(() => document.visibilityState === "visible" && fn(), POLL);
    return () => clearInterval(id);
  }, [fn, on]);
}

/** How many support answers the signed-in account has not read; refreshed every minute. */
export function useSupportUnread() {
  const [n, setN] = useState(0);
  useEffect(() => {
    const load = () => support.unread().then((r) => setN(r.unread)).catch(() => {});
    load();
    const id = setInterval(() => document.visibilityState === "visible" && load(), 60_000);
    supportChanged.addEventListener("change", load);
    return () => {
      clearInterval(id);
      supportChanged.removeEventListener("change", load);
    };
  }, []);
  return n;
}

// --- the thread ---

function Thread({ messages, staffView, empty }: { messages: SupportMessage[]; staffView: boolean; empty?: string }) {
  const t = useT().support;
  const end = useRef<HTMLDivElement>(null);
  const last = messages.at(-1)?.id;
  useEffect(() => end.current?.scrollIntoView({ block: "end" }), [last]);
  if (!messages.length) return <p className="py-10 text-center text-[13px] text-muted-foreground">{empty ?? t.empty}</p>;
  return (
    <div className="space-y-3">
      {messages.map((m) => {
        const mine = m.staff === staffView;
        return (
          <div key={m.id} className={cn("flex flex-col", mine ? "items-end" : "items-start")}>
            <div
              className={cn(
                "max-w-[85%] rounded-lg px-3 py-2 text-[13.5px] leading-relaxed break-words whitespace-pre-wrap",
                mine ? "bg-primary text-primary-foreground" : "border border-border bg-card",
              )}
            >
              {m.body}
            </div>
            <div className="mt-1 px-1 text-[11px] text-muted-foreground">
              {m.staff ? (staffView && m.author ? m.author : t.team) : staffView ? "" : t.you}
              {m.via === "bot" && <span> · {t.viaBot}</span>}
              <span> · {ago(m.created_at)}</span>
            </div>
          </div>
        );
      })}
      <div ref={end} />
    </div>
  );
}

function Composer({ onSend, placeholder, hint, autoFocus }: { onSend: (body: string) => Promise<void>; placeholder: string; hint?: string; autoFocus?: boolean }) {
  const t = useT().support;
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const send = async () => {
    const body = text.trim();
    if (!body || busy) return;
    setBusy(true);
    try {
      await onSend(body);
      setText("");
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="space-y-1.5">
      <div className="flex items-end gap-2">
        <Textarea
          value={text}
          autoFocus={autoFocus}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
              e.preventDefault();
              send();
            }
          }}
          placeholder={placeholder}
          maxLength={4000}
          className="max-h-48 min-h-10 text-[13.5px]"
        />
        <Button onClick={send} disabled={busy || !text.trim()} size="icon" className="size-10 shrink-0" title={t.send}>
          {busy ? <Loader2 className="animate-spin" /> : <Send />}
        </Button>
      </div>
      <p className="px-0.5 text-[11.5px] text-muted-foreground">{hint ?? t.sendHint}</p>
    </div>
  );
}

function StatusBadge({ ticket }: { ticket: Ticket }) {
  const t = useT().support;
  return (
    <span className={cn("rounded-sm border px-1 text-[10.5px]", ticket.status === "open" ? "border-primary/40 text-primary" : "border-border text-muted-foreground")}>
      {ticket.status === "open" ? t.open : t.closed}
    </span>
  );
}

// --- the account's side ---

/** The signed-in account's support requests: the list, one thread, and writing a new one. */
export function SupportDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const t = useT().support;
  const [tickets, setTickets] = useState<Ticket[] | null>(null);
  const [bot, setBot] = useState("");
  const [view, setView] = useState<number | "new" | null>(null);
  const [thread, setThread] = useState<TicketThread | null>(null);

  const loadList = useCallback(() => {
    support
      .list()
      .then((r) => {
        setTickets(r.tickets);
        setBot(r.bot);
      })
      .catch(() => setTickets([]));
  }, []);
  const loadThread = useCallback(() => {
    if (typeof view !== "number") return;
    support
      .get(view)
      .then((r) => {
        setThread(r);
        supportChanged.dispatchEvent(new Event("change"));
      })
      .catch(() => {});
  }, [view]);

  useEffect(() => {
    if (!open) return;
    setThread(null);
    setView(null);
    loadList();
  }, [open, loadList]);
  useEffect(() => {
    // nothing written yet: go straight to the form
    if (open && tickets && !tickets.length && view == null) setView("new");
  }, [open, tickets, view]);
  useEffect(loadThread, [loadThread]);
  usePoll(view === null ? loadList : loadThread, open && view !== "new");

  const show = (r: TicketThread) => {
    setThread(r);
    setView(r.ticket.id);
    loadList();
    supportChanged.dispatchEvent(new Event("change"));
  };
  const back = () => {
    setView(null);
    setThread(null);
    loadList();
  };

  const current = typeof view === "number" && thread?.ticket.id === view ? thread : null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[min(720px,90dvh)] flex-col sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            {view !== null && !!tickets?.length && (
              <button onClick={back} className="-ml-1 rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground" title={t.back}>
                <ArrowLeft className="size-4" />
              </button>
            )}
            {current ? t.ticket(current.ticket.id) : view === "new" ? t.newTicket : t.title}
            {current && <StatusBadge ticket={current.ticket} />}
          </DialogTitle>
          <DialogDescription>{current ? current.ticket.subject : t.description}</DialogDescription>
        </DialogHeader>

        {view === null && (
          <DialogBody className="min-h-0 flex-1 space-y-3 overflow-y-auto">
            <Button variant="outline" size="sm" className="gap-1.5" onClick={() => setView("new")}>
              <Plus className="size-3.5" />
              {t.newTicket}
            </Button>
            {tickets === null ? (
              <Loader2 className="mx-auto my-8 size-5 animate-spin text-muted-foreground" />
            ) : (
              <div className="divide-y divide-border overflow-hidden rounded-lg border border-border">
                {tickets.map((k) => (
                  <button key={k.id} onClick={() => setView(k.id)} className="flex w-full items-start gap-3 bg-card px-3 py-2.5 text-left transition-colors hover:bg-muted">
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <span className="truncate text-[13.5px] font-medium">{k.subject || t.ticket(k.id)}</span>
                        <StatusBadge ticket={k} />
                      </div>
                      <div className="truncate text-[12.5px] text-muted-foreground">{k.preview}</div>
                    </div>
                    <div className="flex shrink-0 flex-col items-end gap-1">
                      <span className="data text-[11.5px] text-muted-foreground">{ago(k.updated_at)}</span>
                      {k.unread > 0 && <span className="data rounded-full bg-primary px-1.5 text-[11px] font-medium text-primary-foreground">{k.unread}</span>}
                    </div>
                  </button>
                ))}
              </div>
            )}
            {bot && <p className="text-[12px] text-muted-foreground">{t.botHint(bot)}</p>}
          </DialogBody>
        )}

        {view === "new" && (
          <DialogBody className="space-y-3">
            <Composer autoFocus placeholder={t.newPlaceholder} onSend={async (body) => show(await support.create(body))} />
            {bot && <p className="text-[12px] text-muted-foreground">{t.botHint(bot)}</p>}
          </DialogBody>
        )}

        {typeof view === "number" && (
          <>
            <DialogBody className="min-h-40 flex-1 overflow-y-auto">
              {current ? <Thread messages={current.messages} staffView={false} /> : <Loader2 className="mx-auto my-8 size-5 animate-spin text-muted-foreground" />}
            </DialogBody>
            {current && (
              <div className="space-y-2 border-t border-border pt-3">
                {current.ticket.status === "closed" && <p className="text-[12px] text-muted-foreground">{t.closedNote}</p>}
                <Composer placeholder={t.placeholder} onSend={async (body) => show(await support.send(current.ticket.id, body))} />
                {current.ticket.status === "open" && (
                  <Button variant="ghost" size="sm" className="h-7 gap-1.5 px-2 text-[12.5px] text-muted-foreground" onClick={async () => show(await support.close(current.ticket.id))}>
                    <CheckCircle2 className="size-3.5" />
                    {t.close}
                  </Button>
                )}
              </div>
            )}
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}

// --- the team's side ---

type Filter = "open" | "closed" | "all";

/** The admin panel's Support tab: the queue on the left, a thread on the right. `focus` opens one
 *  account's conversation (the Message button in the account sheet), with or without a request yet. */
export function SupportTab({ focus, onOpenUser }: { focus: { user: User } | null; onOpenUser: (id: number) => void }) {
  const t = useT().support;
  const [filter, setFilter] = useState<Filter>("open");
  const [queue, setQueue] = useState<Ticket[] | null>(null);
  const [waiting, setWaiting] = useState(0);
  const [sel, setSel] = useState<number | null>(null);
  const [thread, setThread] = useState<TicketThread | null>(null);
  // writing first to an account that has no open request
  const [fresh, setFresh] = useState<User | null>(null);

  const loadQueue = useCallback(() => {
    adminSupport
      .list(filter)
      .then((r) => {
        setQueue(r.tickets);
        setWaiting(r.waiting);
      })
      .catch((e) => toast.error(e.message));
  }, [filter]);
  const loadThread = useCallback(() => {
    if (sel == null) return;
    adminSupport.get(sel).then(setThread).catch(() => {});
  }, [sel]);

  useEffect(loadQueue, [loadQueue]);
  useEffect(loadThread, [loadThread]);
  usePoll(
    useCallback(() => {
      loadQueue();
      loadThread();
    }, [loadQueue, loadThread]),
    true,
  );

  useEffect(() => {
    if (!focus) return;
    adminSupport
      .userTicket(focus.user.id)
      .then((r) => {
        if (r.ticket) {
          setFresh(null);
          setThread(r as TicketThread);
          setSel(r.ticket.id);
        } else {
          setSel(null);
          setThread(null);
          setFresh(focus.user);
        }
      })
      .catch((e) => toast.error(e.message));
  }, [focus]);

  const show = (r: TicketThread) => {
    setFresh(null);
    setThread(r);
    setSel(r.ticket.id);
    loadQueue();
  };

  const current = sel != null && thread?.ticket.id === sel ? thread : null;
  const who = current?.user ?? fresh;

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(280px,360px)_1fr]">
      <div className="space-y-3">
        <div className="flex items-center gap-3">
          <div className="flex rounded-md border border-border bg-card p-0.5 text-[13px]">
            {(["open", "closed", "all"] as const).map((k) => (
              <button
                key={k}
                onClick={() => setFilter(k)}
                className={cn("rounded px-2.5 py-0.5 transition-colors", filter === k ? "bg-accent font-medium text-accent-foreground" : "text-muted-foreground hover:text-foreground")}
              >
                {t.filters[k]}
              </button>
            ))}
          </div>
          {waiting > 0 && <span className="text-[12.5px] text-warning">{t.waiting(waiting)}</span>}
        </div>
        <div className="divide-y divide-border overflow-hidden rounded-lg border border-border bg-card">
          {queue === null ? (
            <Loader2 className="mx-auto my-8 size-5 animate-spin text-muted-foreground" />
          ) : !queue.length ? (
            <p className="py-8 text-center text-[13px] text-muted-foreground">{t.noQueue}</p>
          ) : (
            queue.map((k) => {
              const waits = k.status === "open" && k.last_from === "user";
              return (
                <button
                  key={k.id}
                  onClick={() => {
                    setFresh(null);
                    setSel(k.id);
                  }}
                  className={cn("flex w-full items-start gap-3 px-3 py-2.5 text-left transition-colors hover:bg-muted", sel === k.id && "bg-muted")}
                >
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="truncate text-[13.5px] font-medium">{k.user}</span>
                      <span className="data text-[11px] text-muted-foreground">#{k.id}</span>
                    </div>
                    <div className="truncate text-[12.5px] text-muted-foreground">{k.preview || k.subject}</div>
                  </div>
                  <div className="flex shrink-0 flex-col items-end gap-1">
                    <span className="data text-[11.5px] text-muted-foreground">{ago(k.updated_at)}</span>
                    {k.unread > 0 ? (
                      <span className="data rounded-full bg-primary px-1.5 text-[11px] font-medium text-primary-foreground">{k.unread}</span>
                    ) : (
                      k.status === "open" && <span className={cn("text-[11px]", waits ? "text-warning" : "text-muted-foreground")}>{waits ? t.waitsOnUs : t.answered}</span>
                    )}
                  </div>
                </button>
              );
            })
          )}
        </div>
      </div>

      <div className="flex min-h-[420px] flex-col rounded-lg border border-border bg-card">
        {!who ? (
          <div className="flex flex-1 flex-col items-center justify-center gap-2 text-[13px] text-muted-foreground">
            <MessageCircle className="size-6" />
            {t.pick}
          </div>
        ) : (
          <>
            <div className="flex items-center gap-3 border-b border-border px-4 py-3">
              <Avatar photo={who.tg_photo} label={who.label} className="size-8 text-[13px]" />
              <div className="min-w-0">
                <div className="flex items-center gap-2 text-[14px] font-medium">
                  <span className="truncate">{fresh ? t.messageTitle(who.label) : who.label}</span>
                  {current && <StatusBadge ticket={current.ticket} />}
                </div>
                <div className="truncate text-[12px] text-muted-foreground">{current ? `#${current.ticket.id} · ${current.ticket.subject}` : who.tg_username ? `@${who.tg_username}` : ""}</div>
              </div>
              <div className="ml-auto flex items-center gap-1.5">
                <Button variant="ghost" size="sm" className="h-7 text-[12.5px]" onClick={() => onOpenUser(who.id)}>
                  {t.openAccount}
                </Button>
                {current && (
                  <Button
                    variant="outline"
                    size="sm"
                    className="h-7 gap-1.5 text-[12.5px]"
                    onClick={async () => {
                      try {
                        show(await adminSupport.setStatus(current.ticket.id, current.ticket.status === "open" ? "closed" : "open"));
                      } catch (e) {
                        toast.error((e as Error).message);
                      }
                    }}
                  >
                    {current.ticket.status === "open" ? <CheckCircle2 className="size-3.5" /> : <RotateCcw className="size-3.5" />}
                    {current.ticket.status === "open" ? t.close : t.reopen}
                  </Button>
                )}
              </div>
            </div>
            <div className="max-h-[560px] min-h-0 flex-1 overflow-y-auto p-4">
              {current ? <Thread messages={current.messages} staffView /> : fresh ? <Thread messages={[]} staffView empty={t.firstMessage} /> : <Loader2 className="mx-auto my-8 size-5 animate-spin text-muted-foreground" />}
            </div>
            <div className="border-t border-border p-3">
              <Composer
                key={current?.ticket.id ?? `u${fresh?.id}`}
                autoFocus={!!fresh}
                placeholder={t.replyPlaceholder}
                hint={t.replyHint(!!who.tg_id)}
                onSend={async (body) => show(current ? await adminSupport.send(current.ticket.id, body) : await adminSupport.message(fresh!.id, body))}
              />
            </div>
          </>
        )}
      </div>
    </div>
  );
}
