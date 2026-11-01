"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useState, useEffect } from "react";

import {
  Badge,
  Card,
  EmptyState,
  PageHeader,
  ProblemNotice,
  Table,
  Td,
  Th,
} from "@/components/ui";
import { Button } from "@/components/form";
import { useToast } from "@/components/toast";
import {
  createReview,
  getReview,
  listReviews,
  type Review,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { requireUser } from "@/lib/auth/session";
import { humanise } from "@/lib/format";

/**
 * Performance Reviews (§8.5, decision D8). List reviews and create new ones.
 * Go owns every rule; the page renders what Go returns and routes refusals
 * through ProblemNotice.
 */
export const dynamic = "force-dynamic";

export default async function ReviewsPage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [reviews, setReviews] = useState<Review[]>([]);
  const [error, setError] = useState<ApiError | null>(null);

  useEffect(() => {
    listReviews({ limit: 200 }).then(
      (page) => setReviews(page.items),
      (err) => {
        if (err instanceof ApiError) setError(err);
        else throw err;
      },
    );
  }, []);

  const statusFilter = searchParams.get("status") ?? "";
  const personFilter = searchParams.get("person_id") ?? "";
  const filtered = reviews.filter((r) => {
    if (statusFilter && r.status !== statusFilter) return false;
    if (personFilter && r.person_id !== personFilter) return false;
    return true;
  });

  return (
    <div className="space-y-6">
      <PageHeader
        title="Performance Reviews"
        description="Reviews of people over a review period, rated 1–5 and tracked from draft to approved."
        actions={
          <>
            <Button
              type="button"
              variant="primary"
              size="sm"
              onClick={() => router.push("/people/reviews/new")}
            >
              New review
            </Button>
          </>
        }
      />

      {error && (
        <ProblemNotice
          error={error}
          title={
            error.status === 403
              ? "You do not have permission to view performance reviews"
              : "Performance reviews could not be loaded"
          }
        />
      )}

      <div className="flex flex-wrap items-center justify-between gap-x-5 gap-y-3">
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            variant="secondary"
            size="sm"
            onClick={() => {
              const qs = new URLSearchParams(searchParams);
              qs.delete("status");
              router.push(`/people/reviews?${qs.toString()}`);
            }}
          >
            All statuses
          </Button>
          {["draft", "submitted", "approved", "rejected"].map((s) => (
            <Button
              key={s}
              type="button"
              variant="secondary"
              size="sm"
              onClick={() => {
                const qs = new URLSearchParams(searchParams);
                qs.set("status", s);
                router.push(`/people/reviews?${qs.toString()}`);
              }}
            >
              {humanise(s)}
            </Button>
          ))}
        </div>
      </div>

      <Card padded={false}>
        {filtered.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="clipboard"
              iconTone="info"
              className="min-h-[220px]"
              title={
                statusFilter || personFilter
                  ? "No reviews match"
                  : "No reviews yet"
              }
            >
              {statusFilter || personFilter
                ? "Try a different filter."
                : "Create the first review to start tracking performance."}
            </EmptyState>
          </div>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Person</Th>
                <Th>Reviewer</Th>
                <Th className="text-right">Period</Th>
                <Th className="text-right">Rating</Th>
                <Th>Status</Th>
                <Th className="text-right">Updated</Th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((r) => (
                <tr key={r.id}>
                  <Td>
                    <div className="flex flex-col items-start gap-1">
                      <span className="font-medium">{r.person_id.slice(0, 8)}</span>
                    </div>
                  </Td>
                  <Td className="tabular text-muted text-[13px]">
                    {r.reviewer_id.slice(0, 8)}
                  </Td>
                  <Td className="tabular text-right">
                    {new Date(r.period_start).toLocaleDateString(undefined, {
                      month: "short",
                      day: "numeric",
                    })} –{" "}
                    {new Date(r.period_end).toLocaleDateString(undefined, {
                      month: "short",
                      day: "numeric",
                      year: "numeric",
                    })}
                  </Td>
                  <Td className="tabular text-right font-mono text-[13px]">
                    {r.rating}
                  </Td>
                  <Td>
                    <Badge
                      tone={
                        {
                          draft: "neutral",
                          submitted: "info",
                          approved: "success",
                          rejected: "danger",
                        }[r.status] as import("@/components/ui").Tone
                      }
                    >
                      {humanise(r.status)}
                    </Badge>
                  </Td>
                  <Td className="tabular text-muted">
                    {r.updated_at
                      ? new Date(r.updated_at).toLocaleDateString()
                      : "—"}
                  </Td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>
    </div>
  );
}
