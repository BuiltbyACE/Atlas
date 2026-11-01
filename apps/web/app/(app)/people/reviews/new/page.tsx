"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { Button, Field, Select, TextInput, TextArea } from "@/components/form";
import { ProblemNotice } from "@/components/ui";
import { useToast } from "@/components/toast";
import {
  createReview,
  type Review,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { useMutation } from "@/lib/use-mutation";

/**
 * Create a new performance review (`POST /api/v1/people/reviews`,
 * gated by `people.review.manage`). Person, reviewer, period and rating
 * are required; comments and status are optional.
 */
export default function NewReviewPage() {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [personID, setPersonID] = useState("");
  const [reviewerID, setReviewerID] = useState("");
  const [periodStart, setPeriodStart] = useState("");
  const [periodEnd, setPeriodEnd] = useState("");
  const [rating, setRating] = useState("");
  const [comments, setComments] = useState("");
  const [status, setStatus] = useState("draft");

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const created = await run(async () => {
      return createReview({
        person_id: String(data.get("person_id") ?? "").trim(),
        reviewer_id: String(data.get("reviewer_id") ?? "").trim(),
        period_start: String(data.get("period_start") ?? "").trim(),
        period_end: String(data.get("period_end") ?? "").trim(),
        rating: parseInt(String(data.get("rating") ?? ""), 10),
        comments: String(data.get("comments") ?? "").trim() || undefined,
        status: String(data.get("status") ?? "draft").trim() || undefined,
      });
    });

    if (created) {
      reset();
      notify(`Review ${created.id.slice(0, 8)} created`);
      router.push(`/people/reviews`);
    }
  }

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
      {problem && (
        <ProblemNotice error={problem} title="The review wasn't created" />
      )}

      <Field label="Person (reviewee)" htmlFor="review-person-id">
        <TextInput
          id="review-person-id"
          name="person_id"
          required
          data-autofocus
          placeholder="Person UUID"
          value={personID}
          onChange={(e) => setPersonID(e.target.value)}
        />
        <p className="mt-1 text-[11.5px] text-muted">
          The person being reviewed.
        </p>
      </Field>

      <Field label="Reviewer" htmlFor="review-reviewer-id">
        <TextInput
          id="review-reviewer-id"
          name="reviewer_id"
          required
          placeholder="Reviewer UUID"
          value={reviewerID}
          onChange={(e) => setReviewerID(e.target.value)}
        />
        <p className="mt-1 text-[11.5px] text-muted">
          The person conducting the review.
        </p>
      </Field>

      <div className="grid grid-cols-2 gap-4">
        <Field label="Period start" htmlFor="review-period-start">
          <TextInput
            id="review-period-start"
            name="period_start"
            type="date"
            required
            value={periodStart}
            onChange={(e) => setPeriodStart(e.target.value)}
          />
        </Field>
        <Field label="Period end" htmlFor="review-period-end">
          <TextInput
            id="review-period-end"
            name="period_end"
            type="date"
            required
            value={periodEnd}
            onChange={(e) => setPeriodEnd(e.target.value)}
          />
        </Field>
      </div>

      <Field label="Rating" htmlFor="review-rating">
        <Select
          id="review-rating"
          name="rating"
          required
          value={rating}
          onChange={(e) => setRating(e.target.value)}
        >
          <option value="">Select rating</option>
          <option value="1">1 — Needs improvement</option>
          <option value="2">2 — Developing</option>
          <option value="3">3 — Meets expectations</option>
          <option value="4">4 — Exceeds expectations</option>
          <option value="5">5 — Exceptional</option>
        </Select>
        <p className="mt-1 text-[11.5px] text-muted">
          Rating from 1 to 5.
        </p>
      </Field>

      <Field label="Status" htmlFor="review-status">
        <Select
          id="review-status"
          name="status"
          value={status}
          onChange={(e) => setStatus(e.target.value)}
        >
          <option value="draft">Draft</option>
          <option value="submitted">Submitted</option>
          <option value="approved">Approved</option>
          <option value="rejected">Rejected</option>
        </Select>
      </Field>

      <Field label="Comments" htmlFor="review-comments">
        <TextArea
          id="review-comments"
          name="comments"
          rows={4}
          placeholder="Comments on performance during the period…"
          value={comments}
          onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) => setComments(e.target.value)}
        />
        <p className="mt-1 text-[11.5px] text-muted">
          Free-form comments on the person's performance.
        </p>
      </Field>

      <div className="flex justify-end gap-2 pt-4">
        <Button type="button" variant="secondary" onClick={() => router.back()}>
          Cancel
        </Button>
        <Button type="submit" variant="primary" disabled={busy}>
          {busy ? "Creating…" : "Create review"}
        </Button>
      </div>
    </form>
  );
}
