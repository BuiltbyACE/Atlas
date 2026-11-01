-- +goose Up
-- +goose StatementBegin
-- Phase 2: performance reviews (docs/ATLAS.md §8.5, decision D8).
-- Reviews are owned by the people module; both CEOs hold people.review.read
-- and people.review.manage (already seeded).
CREATE TABLE people.reviews (
  id uuid PRIMARY KEY,
  person_id uuid NOT NULL REFERENCES people.persons(id) ON DELETE CASCADE,
  reviewer_id uuid NOT NULL REFERENCES people.persons(id) ON DELETE CASCADE,
  period_start date NOT NULL,
  period_end date NOT NULL CHECK (period_end >= period_start),
  rating integer NOT NULL CHECK (rating >= 1 AND rating <= 5),
  comments text,
  status text NOT NULL CHECK (status IN ('draft', 'submitted', 'approved', 'rejected')),
  version integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_people_reviews_person ON people.reviews (person_id);
CREATE INDEX idx_people_reviews_reviewer ON people.reviews (reviewer_id);
CREATE INDEX idx_people_reviews_status ON people.reviews (status);
CREATE INDEX idx_people_reviews_period ON people.reviews (period_start, period_end);

CREATE TRIGGER reviews_touch_updated_at BEFORE UPDATE ON people.reviews
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS people.reviews;
-- +goose StatementEnd
