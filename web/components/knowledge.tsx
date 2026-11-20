"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { api, qk, type KBArticle } from "@/lib/api";
import { formatDate } from "@/lib/utils";

/** All knowledge base articles. They're fetched once and shared by the list and article pages. */
export function useKnowledgeBase() {
  return useQuery({ queryKey: qk.kb, queryFn: api.kb.list, staleTime: Infinity });
}

/** "2026-11-20" → "Nov 20, 2026", read as a local date (not UTC midnight). */
export function reviewedOn(a: KBArticle): string {
  return formatDate(`${a.reviewed}T00:00:00`);
}

/** Article text without Markdown punctuation, for searching and snippets. */
export function plainText(body: string): string {
  return body
    .replace(/```[\s\S]*?```/g, (block) => block.replace(/```\w*/g, " "))
    .replace(/!\[[^\]]*\]\([^)]*\)/g, " ")
    .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/^#+\s*/gm, "")
    .replace(/[*_`>|]/g, " ")
    .replace(/^\s*[-:]{3,}\s*$/gm, " ")
    .replace(/\s+/g, " ")
    .trim();
}

export type SearchHit = { article: KBArticle; snippet: React.ReactNode | null };

/**
 * Articles containing every word of the query (in the title, summary or text).
 * Title matches come first; each hit has a short snippet around the first
 * place the text mentions the query.
 */
export function searchArticles(articles: KBArticle[], query: string): SearchHit[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean);
  const first = words[0];
  if (first === undefined) return [];
  const hits: { hit: SearchHit; score: number }[] = [];
  for (const a of articles) {
    const title = a.title.toLowerCase();
    const text = plainText(a.body);
    const hay = `${title} ${a.summary.toLowerCase()} ${text.toLowerCase()}`;
    if (!words.every((w) => hay.includes(w))) continue;
    const score = words.filter((w) => title.includes(w)).length * 10 + (a.summary.toLowerCase().includes(first) ? 2 : 0);
    hits.push({ hit: { article: a, snippet: snippet(text, words) }, score });
  }
  return hits.sort((x, y) => y.score - x.score).map((h) => h.hit);
}

function snippet(text: string, words: string[]): React.ReactNode | null {
  const lower = text.toLowerCase();
  const phrase = words.join(" ");
  let at = lower.indexOf(phrase);
  let len = phrase.length;
  const first = words[0] ?? "";
  if (at < 0 && first) {
    at = lower.indexOf(first);
    len = first.length;
  }
  if (at < 0) return null;
  const start = Math.max(0, at - 70);
  const end = Math.min(text.length, at + len + 110);
  return (
    <>
      {start > 0 && "…"}
      {text.slice(start, at)}
      <mark className="rounded bg-warning/25 px-0.5 text-foreground">{text.slice(at, at + len)}</mark>
      {text.slice(at + len, end)}
      {end < text.length && "…"}
    </>
  );
}
