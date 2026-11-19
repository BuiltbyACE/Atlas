package chat

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func chatTestURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("ATLAS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ATLAS_TEST_DATABASE_URL is not set; skipping chat integration tests")
	}
	return url
}

func chatTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), chatTestURL(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// chatTestUser inserts a person and its user account: chat rows reference
// core.user_accounts, which in turn references people.persons.
func chatTestUser(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	personID, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("new person id: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO people.persons (id, full_name, employment_type) VALUES ($1, 'Chat Test', 'full_time')`,
		personID); err != nil {
		t.Fatalf("insert person: %v", err)
	}
	userID, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("new user id: %v", err)
	}
	email := fmt.Sprintf("chat-test-%s@example.test", uuid.NewString())
	if _, err := pool.Exec(ctx,
		`INSERT INTO core.user_accounts (id, person_id, email, status) VALUES ($1, $2, $3, 'active')`,
		userID, personID, email); err != nil {
		t.Fatalf("insert user account: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM core.user_accounts WHERE id = $1`, userID)
		_, _ = pool.Exec(ctx, `DELETE FROM people.persons WHERE id = $1`, personID)
	})
	return userID
}

func chatTestConversation(t *testing.T, svc *Service, creator, other uuid.UUID) *Conversation {
	t.Helper()
	conv, err := svc.CreateConversation(context.Background(), CreateConversationOptions{
		Type: "direct", CreatedBy: creator, Participant: []uuid.UUID{other},
	})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	return conv
}

func chatTestDocument(t *testing.T, pool *pgxpool.Pool, uploadedBy uuid.UUID) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("new document id: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO core.documents (id, key, filename, content_type, size_bytes, uploaded_by)
		VALUES ($1, $2, 'brief.pdf', 'application/pdf', 1024, $3)`,
		id, "chat-test/"+uuid.NewString()+".pdf", uploadedBy); err != nil {
		t.Fatalf("insert document: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM core.documents WHERE id = $1`, id)
	})
	return id
}

func attachmentDocumentID(t *testing.T, pool *pgxpool.Pool, messageID uuid.UUID) *uuid.UUID {
	t.Helper()
	var docID *uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`SELECT document_id FROM core.chat_message_attachments WHERE message_id = $1`, messageID).Scan(&docID); err != nil {
		t.Fatalf("read attachment: %v", err)
	}
	return docID
}

// TestSendMessageAttachmentWithDocument proves a document-backed attachment
// keeps its document ID.
func TestSendMessageAttachmentWithDocument(t *testing.T) {
	ctx := context.Background()
	pool := chatTestPool(t)
	svc := NewService(NewPGStore(pool), nil)
	sender := chatTestUser(t, pool)
	other := chatTestUser(t, pool)
	conv := chatTestConversation(t, svc, sender, other)
	docID := chatTestDocument(t, pool, sender)

	msg, err := svc.SendMessage(ctx, SendMessageOptions{
		ConversationID: conv.ID, SenderID: sender, BodyType: "file",
		Attachments: []MessageAttachmentInput{{
			DocumentID: &docID, Filename: "brief.pdf", ContentType: "application/pdf", SizeBytes: 1024,
		}},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	got := attachmentDocumentID(t, pool, msg.ID)
	if got == nil {
		t.Fatal("attachment document_id = NULL, want the document ID")
	}
	if *got != docID {
		t.Fatalf("attachment document_id = %s, want %s", *got, docID)
	}
}

// TestSendMessageAttachmentWithoutDocument proves an attachment with no
// document does not panic and stores SQL NULL.
func TestSendMessageAttachmentWithoutDocument(t *testing.T) {
	ctx := context.Background()
	pool := chatTestPool(t)
	svc := NewService(NewPGStore(pool), nil)
	sender := chatTestUser(t, pool)
	other := chatTestUser(t, pool)
	conv := chatTestConversation(t, svc, sender, other)

	msg, err := svc.SendMessage(ctx, SendMessageOptions{
		ConversationID: conv.ID, SenderID: sender, BodyType: "file",
		Attachments: []MessageAttachmentInput{{
			DocumentID: nil, Filename: "notes.txt", ContentType: "text/plain", SizeBytes: 12,
		}},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if got := attachmentDocumentID(t, pool, msg.ID); got != nil {
		t.Fatalf("attachment document_id = %s, want NULL", *got)
	}
}

// TestAttachmentDocumentDeleteSetsNull proves the 0035 foreign key clears the
// reference when the document disappears.
func TestAttachmentDocumentDeleteSetsNull(t *testing.T) {
	ctx := context.Background()
	pool := chatTestPool(t)
	svc := NewService(NewPGStore(pool), nil)
	sender := chatTestUser(t, pool)
	other := chatTestUser(t, pool)
	conv := chatTestConversation(t, svc, sender, other)
	docID := chatTestDocument(t, pool, sender)

	msg, err := svc.SendMessage(ctx, SendMessageOptions{
		ConversationID: conv.ID, SenderID: sender, BodyType: "file",
		Attachments: []MessageAttachmentInput{{
			DocumentID: &docID, Filename: "brief.pdf", ContentType: "application/pdf", SizeBytes: 1024,
		}},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM core.documents WHERE id = $1`, docID); err != nil {
		t.Fatalf("delete document: %v", err)
	}
	if got := attachmentDocumentID(t, pool, msg.ID); got != nil {
		t.Fatalf("attachment document_id = %s, want NULL after document delete", *got)
	}
}
