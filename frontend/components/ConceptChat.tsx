"use client";

import { ArrowUp, Loader2, MessageSquare } from "lucide-react";
import { Fragment, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { studio, type ChatMsg } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Sheet, SheetContent, SheetDescription, SheetTitle } from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";

export interface ChatTarget {
  id: number;
  project: string;
  pitch: string;
  names: string[];
}

const STARTERS = [
  "How would I build it, step by step?",
  "What are the inputs and outputs? Show an example.",
  "Which public data or APIs does it rely on?",
  "How do I get the first 100 users?",
];

/** A short chat about one concept. Every message is paid; the thread is kept and can be reopened any time. */
export function ConceptChat({ target, onClose, onCount }: { target: ChatTarget | null; onClose: () => void; onCount?: (id: number, n: number) => void }) {
  const [msgs, setMsgs] = useState<ChatMsg[] | null>(null);
  const [price, setPrice] = useState(2);
  const [q, setQ] = useState("");
  const [sending, setSending] = useState(false);
  const end = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!target) return;
    setMsgs(null);
    setQ("");
    studio
      .chat(target.id)
      .then((r) => {
        setMsgs(r.messages);
        setPrice(r.price);
      })
      .catch(() => setMsgs([]));
  }, [target]);

  useEffect(() => {
    end.current?.scrollIntoView({ block: "end" });
  }, [msgs, sending]);

  const send = async (text = q) => {
    if (!target || !text.trim() || sending) return;
    setSending(true);
    setMsgs((m) => [...(m ?? []), { role: "user", content: text.trim(), ts: Date.now() / 1000 }]);
    setQ("");
    try {
      const r = await studio.ask(target.id, text.trim());
      setMsgs(r.messages);
      onCount?.(target.id, r.messages.length);
    } catch (e) {
      toast.error((e as Error).message);
      setMsgs((m) => m?.slice(0, -1) ?? null);
      setQ(text);
    } finally {
      setSending(false);
    }
  };

  return (
    <Sheet open={!!target} onOpenChange={(o) => !o && onClose()}>
      <SheetContent side="right" className="w-full gap-0 p-0 sm:max-w-[520px]">
        <div className="border-b border-border px-5 pt-6 pb-4">
          <SheetTitle className="flex items-center gap-2 text-[18px] font-semibold tracking-tight">
            <MessageSquare className="size-4 text-primary" />
            {target?.project}
          </SheetTitle>
          <SheetDescription className="mt-1 text-[13px] leading-snug">{target?.pitch}</SheetDescription>
          {target && target.names.length > 0 && <div className="data mt-1.5 text-[12px] text-muted-foreground">{target.names.map((n) => "@" + n).join(", ")}</div>}
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
          {msgs == null ? (
            <div className="space-y-3">
              <Skeleton className="h-10 w-2/3" />
              <Skeleton className="h-24 w-full" />
            </div>
          ) : msgs.length === 0 ? (
            <div>
              <p className="text-sm text-muted-foreground">Ask anything about building or launching it. Each answer costs {price} credits; the thread is kept with the concept.</p>
              <div className="mt-4 flex flex-col items-start gap-1.5">
                {STARTERS.map((s) => (
                  <button key={s} onClick={() => send(s)} disabled={sending} className="rounded-md border border-border px-3 py-1.5 text-left text-[13px] hover:border-primary hover:text-primary">
                    {s}
                  </button>
                ))}
              </div>
            </div>
          ) : (
            <div className="space-y-4">
              {msgs.map((m, i) =>
                m.role === "user" ? (
                  <div key={i} className="ml-10 rounded-lg bg-accent px-3.5 py-2.5 text-[14px] leading-snug whitespace-pre-wrap text-accent-foreground">
                    {m.content}
                  </div>
                ) : (
                  <div key={i} className="text-[14px] leading-relaxed">
                    <Md text={m.content} />
                  </div>
                ),
              )}
              {sending && (
                <div className="flex items-center gap-2 text-[13px] text-muted-foreground">
                  <Loader2 className="size-3.5 animate-spin" />
                  Thinking…
                </div>
              )}
            </div>
          )}
          <div ref={end} />
        </div>

        <form
          onSubmit={(e) => {
            e.preventDefault();
            send();
          }}
          className="border-t border-border p-3"
        >
          <div className="flex items-end gap-2 rounded-lg border border-input px-3 py-2 focus-within:border-primary">
            <textarea
              value={q}
              onChange={(e) => setQ(e.target.value.slice(0, 1200))}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.shiftKey) {
                  e.preventDefault();
                  send();
                }
              }}
              rows={2}
              placeholder="How would I find bots to audit with public data?"
              className="min-h-[40px] flex-1 resize-none bg-transparent text-[14px] outline-none"
            />
            <button type="submit" disabled={!q.trim() || sending} className="inline-flex h-8 shrink-0 items-center gap-1 rounded-md bg-primary px-2.5 text-[12px] font-semibold text-primary-foreground disabled:opacity-40">
              <ArrowUp className="size-3.5" />
              Ask <span className="data opacity-80">{price} cr</span>
            </button>
          </div>
        </form>
      </SheetContent>
    </Sheet>
  );
}

/** Just enough Markdown for chat answers: paragraphs, lists, ### headings, **bold** and `code`. */
function Md({ text }: { text: string }) {
  const blocks: React.ReactNode[] = [];
  let list: { ordered: boolean; items: string[] } | null = null;
  const flush = () => {
    if (!list) return;
    const L = list.ordered ? "ol" : "ul";
    blocks.push(
      <L key={blocks.length} className={cn("my-1.5 space-y-1 pl-5", list.ordered ? "list-decimal" : "list-disc", "marker:text-muted-foreground")}>
        {list.items.map((it, i) => (
          <li key={i}>{inline(it)}</li>
        ))}
      </L>,
    );
    list = null;
  };
  let para: string[] = [];
  const flushPara = () => {
    if (para.length) blocks.push(<p key={blocks.length} className="my-1.5">{inline(para.join(" "))}</p>);
    para = [];
  };
  for (const raw of text.split("\n")) {
    const line = raw.trimEnd();
    const ul = line.match(/^\s*[-*•]\s+(.*)$/);
    const ol = line.match(/^\s*\d+[.)]\s+(.*)$/);
    const h = line.match(/^#{1,4}\s+(.*)$/);
    if (ul || ol) {
      flushPara();
      const ordered = !!ol;
      if (list && list.ordered !== ordered) flush();
      list = list ?? { ordered, items: [] };
      list.items.push((ul ?? ol)![1]);
    } else if (h) {
      flushPara();
      flush();
      blocks.push(<p key={blocks.length} className="mt-3 mb-1 font-semibold">{inline(h[1])}</p>);
    } else if (line.trim() === "") {
      flushPara();
      flush();
    } else {
      flush();
      para.push(line.trim());
    }
  }
  flushPara();
  flush();
  return <>{blocks}</>;
}

function inline(s: string): React.ReactNode {
  const parts = s.split(/(\*\*[^*]+\*\*|`[^`]+`)/g);
  return parts.map((p, i) =>
    p.startsWith("**") && p.endsWith("**") ? (
      <strong key={i} className="font-semibold">
        {p.slice(2, -2)}
      </strong>
    ) : p.startsWith("`") && p.endsWith("`") ? (
      <code key={i} className="data rounded bg-muted px-1 text-[12.5px]">
        {p.slice(1, -1)}
      </code>
    ) : (
      <Fragment key={i}>{p}</Fragment>
    ),
  );
}
