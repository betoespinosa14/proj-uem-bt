"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import { BookOpen, ChevronRight, Search } from "lucide-react";
import { useCan } from "@/lib/session";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { EmptyState, ErrorState, PageHeader } from "@/components/common";
import { reviewedOn, searchArticles, useKnowledgeBase } from "@/components/knowledge";

const SECTION_NOTES: Record<string, string> = {
  "Start here": "What BEACON is and what to do first when you take it over.",
  Architecture: "How the pieces fit together and the technologies behind them.",
  Operations: "Running the server day to day: releases, backups, enrolling devices.",
  Troubleshooting: "Step-by-step fixes for the problems most likely to come up.",
  Security: "Sign-in, roles, the audit log, secrets and known gaps.",
  FAQ: "Questions and answers for each part of the console.",
};

export default function KnowledgePage() {
  const can = useCan();
  const { data, isLoading, error, refetch } = useKnowledgeBase();
  const [q, setQ] = useState("");
  const articles = useMemo(() => data?.articles ?? [], [data]);
  const hits = useMemo(() => searchArticles(articles, q), [articles, q]);
  const searching = q.trim() !== "";

  if (!can("kb.read")) {
    return (
      <>
        <PageHeader title="Knowledge base" />
        <EmptyState icon={BookOpen} title="Owners and admins only" description="Ask an owner or admin if you need something from the knowledge base." />
      </>
    );
  }

  return (
    <>
      <PageHeader
        title="Knowledge base"
        description="Everything needed to run, fix and hand over BEACON: the technologies behind it, how it's set up, and answers to common questions."
      />
      <div className="relative mb-6 max-w-xl">
        <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Search, e.g. backup, MFA, offline"
          className="pl-8"
          aria-label="Search the knowledge base"
          type="search"
        />
      </div>

      {error ? (
        <ErrorState error={error} onRetry={() => void refetch()} />
      ) : isLoading ? (
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 6 }, (_, i) => (
            <Skeleton key={i} className="h-24 rounded-2xl" />
          ))}
        </div>
      ) : searching ? (
        hits.length === 0 ? (
          <EmptyState icon={Search} title="Nothing matches" description="Try a shorter or different word." />
        ) : (
          <div className="space-y-2">
            <p className="text-sm text-muted-foreground">
              {hits.length} {hits.length === 1 ? "article" : "articles"}
            </p>
            {hits.map(({ article: a, snippet }) => (
              <Link key={a.slug} href={`/knowledge/${a.slug}`} className="block">
                <Card className="px-5 py-4 transition-shadow hover:shadow-lift">
                  <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{a.section}</p>
                  <p className="mt-0.5 font-semibold">{a.title}</p>
                  <p className="mt-1 text-sm text-muted-foreground">{snippet ?? a.summary}</p>
                </Card>
              </Link>
            ))}
          </div>
        )
      ) : (
        <div className="space-y-8">
          {(data?.sections ?? []).map((section) => {
            const list = articles.filter((a) => a.section === section);
            if (list.length === 0) return null;
            return (
              <section key={section}>
                <h2 className="text-base font-semibold tracking-tight">{section}</h2>
                <p className="mb-3 text-sm text-muted-foreground">{SECTION_NOTES[section]}</p>
                <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
                  {list.map((a) => (
                    <Link key={a.slug} href={`/knowledge/${a.slug}`} className="group block">
                      <Card className="flex h-full flex-col px-5 py-4 transition-shadow group-hover:shadow-lift">
                        <p className="flex items-center justify-between gap-2 font-semibold">
                          {a.title}
                          <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5" aria-hidden />
                        </p>
                        <p className="mt-1 flex-1 text-sm text-muted-foreground">{a.summary}</p>
                        <p className="mt-3 text-xs text-muted-foreground">Reviewed {reviewedOn(a)}</p>
                      </Card>
                    </Link>
                  ))}
                </div>
              </section>
            );
          })}
        </div>
      )}
    </>
  );
}
