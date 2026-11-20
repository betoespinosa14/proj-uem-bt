"use client";

import * as React from "react";
import Link from "next/link";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { Check, Copy } from "lucide-react";
import { copyToClipboard } from "@/lib/utils";
import { cn } from "@/lib/utils";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";

/** The plain text inside rendered Markdown children. */
export function textOf(node: React.ReactNode): string {
  if (node == null || typeof node === "boolean") return "";
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(textOf).join("");
  if (React.isValidElement<{ children?: React.ReactNode }>(node)) return textOf(node.props.children);
  return "";
}

/** Heading anchor, e.g. "Restore a backup" → "restore-a-backup". */
export function headingId(text: string): string {
  return text
    .toLowerCase()
    .replace(/[`*_]/g, "")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "");
}

/**
 * An article's "##" headings, for its "On this page" list. FAQ articles are
 * just "###" questions, so those are listed when there are no "##" sections.
 */
export function outline(body: string): { id: string; text: string }[] {
  const by: Record<2 | 3, { id: string; text: string }[]> = { 2: [], 3: [] };
  let fenced = false;
  for (const line of body.split("\n")) {
    if (line.startsWith("```")) fenced = !fenced;
    if (fenced) continue;
    const m = /^(##|###) (.+)$/.exec(line);
    if (m?.[1] && m[2]) {
      const text = m[2].replace(/[`*_]/g, "").trim();
      by[m[1].length as 2 | 3].push({ id: headingId(text), text });
    }
  }
  return by[2].length > 1 ? by[2] : by[3];
}

/** Turns "backups.md#restore" into "/knowledge/backups#restore"; leaves other links alone. */
function articleHref(href: string): string | null {
  const m = /^([a-z0-9-]+)\.md(#.*)?$/i.exec(href);
  return m ? `/knowledge/${m[1]}${m[2] ?? ""}` : null;
}

function CodeBlock({ code, lang }: { code: string; lang?: string }) {
  const [copied, setCopied] = React.useState(false);
  return (
    <div className="group relative my-4">
      <pre className="overflow-x-auto rounded-xl border border-border bg-muted/50 px-4 py-3 font-mono text-[13px] leading-relaxed">
        <code data-lang={lang}>{code}</code>
      </pre>
      <button
        type="button"
        onClick={async () => {
          if (await copyToClipboard(code)) {
            setCopied(true);
            setTimeout(() => setCopied(false), 1500);
          }
        }}
        aria-label={copied ? "Copied" : "Copy code"}
        title={copied ? "Copied" : "Copy"}
        className={cn(
          "absolute right-2 top-2 inline-flex items-center gap-1 rounded-md border border-border bg-card px-1.5 py-1 text-xs text-muted-foreground shadow-card transition-opacity hover:text-foreground focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
          copied ? "opacity-100" : "opacity-0 group-hover:opacity-100 [@media(hover:none)]:opacity-100",
        )}
      >
        {copied ? (
          <>
            <Check className="h-3.5 w-3.5 text-success" aria-hidden /> Copied
          </>
        ) : (
          <Copy className="h-3.5 w-3.5" aria-hidden />
        )}
      </button>
    </div>
  );
}

const components: Components = {
  h2: ({ children }) => {
    const id = headingId(textOf(children));
    return (
      <h2 id={id} className="mb-3 mt-9 scroll-mt-24 border-b border-border pb-2 text-lg font-semibold tracking-tight first:mt-0">
        <a href={`#${id}`} className="hover:underline">
          {children}
        </a>
      </h2>
    );
  },
  h3: ({ children }) => (
    <h3 id={headingId(textOf(children))} className="mb-2 mt-6 scroll-mt-24 text-[15px] font-semibold">
      {children}
    </h3>
  ),
  h4: ({ children }) => <h4 className="mb-2 mt-4 text-sm font-semibold">{children}</h4>,
  p: ({ children }) => <p className="my-3 leading-relaxed">{children}</p>,
  ul: ({ children, className }) => (
    <ul className={cn("my-3 space-y-1.5 pl-5", className?.includes("contains-task-list") ? "list-none pl-1" : "list-disc")}>{children}</ul>
  ),
  ol: ({ children }) => <ol className="my-3 list-decimal space-y-1.5 pl-5">{children}</ol>,
  li: ({ children }) => <li className="leading-relaxed marker:text-muted-foreground">{children}</li>,
  input: ({ checked }) => (
    // Checklist boxes are for reading, not ticking: the article is the same for everyone.
    <input type="checkbox" checked={!!checked} readOnly disabled className="mr-2 h-3.5 w-3.5 translate-y-0.5 accent-current" />
  ),
  blockquote: ({ children }) => (
    <blockquote className="my-4 rounded-r-lg border-l-4 border-brand/60 bg-muted/40 px-4 py-1 text-muted-foreground">{children}</blockquote>
  ),
  hr: () => <hr className="my-8 border-border" />,
  a: ({ href = "", children }) => {
    const inner = articleHref(href);
    if (inner) {
      return (
        <Link href={inner} className="font-medium text-primary underline-offset-2 hover:underline">
          {children}
        </Link>
      );
    }
    if (href.startsWith("#")) {
      return (
        <a href={href} className="font-medium text-primary underline-offset-2 hover:underline">
          {children}
        </a>
      );
    }
    return (
      <a href={href} target="_blank" rel="noreferrer noopener" className="font-medium text-primary underline-offset-2 hover:underline">
        {children}
      </a>
    );
  },
  strong: ({ children }) => {
    // "**To fill in:**" marks a fact that isn't in the code and needs a person to supply it.
    if (/^to fill in:?$/i.test(textOf(children).trim())) {
      return (
        <Badge variant="warning" className="mr-1 align-[1px]">
          To fill in
        </Badge>
      );
    }
    return <strong className="font-semibold">{children}</strong>;
  },
  code: ({ children }) => <code className="rounded bg-muted px-1 py-0.5 font-mono text-[0.86em]">{children}</code>,
  pre: ({ children }) => {
    const child = React.Children.only(children) as React.ReactElement<{ className?: string; children?: React.ReactNode }>;
    const lang = /language-(\w+)/.exec(child.props.className ?? "")?.[1];
    return <CodeBlock code={textOf(child.props.children).replace(/\n$/, "")} lang={lang} />;
  },
  table: ({ children }) => (
    <div className="my-4 rounded-xl border border-border">
      <Table>{children}</Table>
    </div>
  ),
  thead: ({ children }) => <THead>{children}</THead>,
  tbody: ({ children }) => <TBody>{children}</TBody>,
  tr: ({ children }) => <TR>{children}</TR>,
  th: ({ children }) => <TH className="whitespace-nowrap">{children}</TH>,
  td: ({ children }) => <TD className="align-top">{children}</TD>,
};

/** Renders a knowledge base article. Raw HTML in the Markdown is ignored. */
export function Markdown({ children, className }: { children: string; className?: string }) {
  return (
    <div className={cn("text-sm text-foreground", className)}>
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
        {children}
      </ReactMarkdown>
    </div>
  );
}
