package people

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Session activity (§8.5). A *session* is one sign-in: a row in core.sessions,
// opened when the account logs in and closed when it signs out, is revoked
// (off-boarding, "sign out everywhere") or expires. The blueprint does not name
// this view, so it is deliberately read-only and derived entirely from the
// session rows the auth module already keeps — no new business state.
//
// While a tab is open the web app sends a heartbeat that refreshes
// last_seen_at. A live session whose last_seen_at is older than the presence
// window has therefore been left, even though the row is still usable: that is
// what makes "they were active from 09:12 to 17:40" possible.

// Session is one sign-in with the person it belongs to (core.sessions joined
// through core.user_accounts to people.persons).
type Session struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	PersonID   uuid.UUID
	FullName   string
	IP         string
	UserAgent  string
	StartedAt  time.Time
	LastSeenAt time.Time
	RevokedAt  *time.Time
	ExpiresAt  time.Time
}

// SessionActivity is a sign-in plus the fields the activity view derives from
// it: whether it is online now, when it ended (only when explicitly ended) and
// how long it was — or has been — active.
type SessionActivity struct {
	Session Session
	// Online is true for a live, unexpired session seen within the presence
	// window.
	Online bool
	// EndedAt is the explicit end (revoked_at), nil while the session is live
	// or was simply abandoned. For "from when to when", an abandoned session
	// ends at Session.LastSeenAt.
	EndedAt *time.Time
	// ActiveSeconds is the span from StartedAt to the effective end (EndedAt,
	// or LastSeenAt while live).
	ActiveSeconds int64
}

// ListSessionsQuery filters the session history. From is inclusive and To is
// exclusive; both bound *activity* (a session that started before the window
// but was still active inside it is included).
type ListSessionsQuery struct {
	PersonID *uuid.UUID
	From     *time.Time
	To       *time.Time
}

// Presence returns the sessions that are online right now: live, unexpired and
// seen within the presence window, most recently active first.
func (s *Service) Presence(ctx context.Context) ([]SessionActivity, error) {
	now := s.now()
	sessions, err := s.store.ListActiveSessions(ctx, now.Add(-s.presenceWindow))
	if err != nil {
		return nil, err
	}
	return s.classify(sessions, now), nil
}

// ListSessions returns a page of sign-in sessions, most recently active first,
// each with its derived activity.
func (s *Service) ListSessions(ctx context.Context, q ListSessionsQuery, limit int, before *Cursor) ([]SessionActivity, error) {
	sessions, err := s.store.ListSessions(ctx, q, limit, before)
	if err != nil {
		return nil, err
	}
	return s.classify(sessions, s.now()), nil
}

// classify derives Online, EndedAt and ActiveSeconds for a batch of sessions.
func (s *Service) classify(sessions []Session, now time.Time) []SessionActivity {
	out := make([]SessionActivity, 0, len(sessions))
	for _, sess := range sessions {
		activity := SessionActivity{Session: sess}
		activity.Online = sess.RevokedAt == nil &&
			now.Before(sess.ExpiresAt) &&
			now.Sub(sess.LastSeenAt) <= s.presenceWindow

		end := sess.LastSeenAt
		if sess.RevokedAt != nil {
			end = *sess.RevokedAt
			activity.EndedAt = sess.RevokedAt
		}
		if span := end.Sub(sess.StartedAt); span > 0 {
			activity.ActiveSeconds = int64(span.Seconds())
		}
		out = append(out, activity)
	}
	return out
}
