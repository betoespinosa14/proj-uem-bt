"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useMemo } from "react";
import { ArrowLeft, ArrowRight, BookOpen, FileQuestion } from "lucide-react";
import { useCan } from "@/lib/session";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { EmptyState, ErrorState } from "@/components/common";
import { Markdown, outline } from "@/components/markdown";
import { reviewedOn, useKnowledgeBase } from "@/components/knowledge";

export default function KnowledgeArticlePage() {
  const { slug } = useParams<{ slug: string }>();
  const can = useCan();
  const { data, isLoading, error, refetch } = useKnowledgeBase();
  const articles = useMemo(() => data?.articles ?? [], [data]);
  const i = articles.findIndex((a) => a.slug === slug);
  const article = i >= 0 ? articles[i] : undefined;
  const toc = useMemo(() => (article ? outline(article.body) : []), [article]);
  const prev = i > 0 ? articles[i - 1] : undefined;
  const next = i >= 0 && i < articles.length - 1 ? articles[i + 1] : undefined;

  const back = (
    <Link href="/knowledge" className="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground">
      <ArrowLeft className="h-4 w-4" aria-hidden /> Knowledge base
    </Link>
  );

  if (!can("kb.read")) {
    return <EmptyState icon={BookOpen} title="Owners and admins only" description="Ask an owner or admin if you need something from the knowledge base." />;
  }
  if (error) return <ErrorState error={error} onRetry={() => void refetch()} />;
  if (isLoading) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-6 w-40" />
        <Skeleton className="h-9 w-2/3" />
        <Skeleton className="h-96 rounded-2xl" />
      </div>
    );
  }
  if (!article) {
    return (
      <>
        {back}
        <EmptyState icon={FileQuestion} title="Article not found" description="It may have been renamed. Search the knowledge base instead." />
      </>
    );
  }

  return (
    <>
      {back}
      <div className="grid grid-cols-1 gap-8 lg:grid-cols-[minmax(0,1fr)_14rem]">
        <article className="min-w-0">
          <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{article.section}</p>
          <h1 className="mt-1 text-2xl font-semibold tracking-tight">{article.title}</h1>
          <p className="mt-1 text-muted-foreground">{article.summary}</p>
          <Card className="mt-6 px-5 py-6 sm:px-8">
            <Markdown>{article.body}</Markdown>
          </Card>
          <p className="mt-3 text-xs text-muted-foreground">
            Source: <code className="font-mono">{article.file}</code> · Reviewed {reviewedOn(article)}
          </p>
          <nav className="mt-6 grid grid-cols-1 gap-3 sm:grid-cols-2" aria-label="More articles">
            {prev ? (
              <Link href={`/knowledge/${prev.slug}`} className="block">
                <Card className="px-4 py-3 transition-shadow hover:shadow-lift">
                  <p className="text-xs text-muted-foreground">Previous</p>
                  <p className="font-medium">{prev.title}</p>
                </Card>
              </Link>
            ) : (
              <span className="hidden sm:block" />
            )}
            {next && (
              <Link href={`/knowledge/${next.slug}`} className="block">
                <Card className="px-4 py-3 text-right transition-shadow hover:shadow-lift">
                  <p className="text-xs text-muted-foreground">
                    Next <ArrowRight className="inline h-3 w-3" aria-hidden />
                  </p>
                  <p className="font-medium">{next.title}</p>
                </Card>
              </Link>
            )}
          </nav>
        </article>
        {toc.length > 1 && (
          <aside className="hidden lg:block">
            <div className="sticky top-24 max-h-[calc(100vh-7rem)] overflow-y-auto pb-4">
              <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">On this page</p>
              <ul className="space-y-1.5 border-l border-border text-sm">
                {toc.map((h) => (
                  <li key={h.id}>
                    <a href={`#${h.id}`} className="-ml-px block border-l border-transparent pl-3 text-muted-foreground hover:border-foreground hover:text-foreground">
                      {h.text}
                    </a>
                  </li>
                ))}
              </ul>
            </div>
          </aside>
        )}
      </div>
    </>
  );
}
