package persistence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PayloadStore offloads larger message and tool bodies outside the
// row tables. Callers receive an opaque ID that they store on the
// owning row; the actual bytes live in the payload_store table.
type PayloadStore struct {
	db *sql.DB
}

// NewPayloadStore returns a payload store backed by db.
func NewPayloadStore(db *DB) *PayloadStore {
	return &PayloadStore{db: db.SQL()}
}

// MaxInlineMessageBytes is the threshold above which message content
// must be offloaded into the payload store instead of being inlined.
const MaxInlineMessageBytes = 16 * 1024

// ErrPayloadNotFound is returned when Get cannot resolve an ID.
var ErrPayloadNotFound = errors.New("persistence: payload not found")

// Put writes content into the payload store and returns its ID. The
// hash is recorded so identical payloads still produce a fresh row id
// but tooling can spot duplicates if a future plan calls for dedup.
func (s *PayloadStore) Put(ctx context.Context, content []byte, contentType string) (PayloadDescriptor, error) {
	if len(content) == 0 {
		return PayloadDescriptor{}, errors.New("persistence: empty payload")
	}
	sum := sha256.Sum256(content)
	desc := PayloadDescriptor{
		ID:          uuid.NewString(),
		SizeBytes:   int64(len(content)),
		ContentType: contentType,
		Sha256:      hex.EncodeToString(sum[:]),
		CreatedAt:   time.Now().UTC(),
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO payload_store (id, sha256, content_type, size_bytes, body, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		desc.ID, desc.Sha256, desc.ContentType, desc.SizeBytes, content, desc.CreatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return PayloadDescriptor{}, fmt.Errorf("insert payload: %w", err)
	}
	return desc, nil
}

// Get returns the bytes and descriptor for id.
func (s *PayloadStore) Get(ctx context.Context, id string) ([]byte, PayloadDescriptor, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, sha256, content_type, size_bytes, body, created_at FROM payload_store WHERE id = ?`, id)
	var desc PayloadDescriptor
	var body []byte
	var createdAt string
	if err := row.Scan(&desc.ID, &desc.Sha256, &desc.ContentType, &desc.SizeBytes, &body, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, PayloadDescriptor{}, ErrPayloadNotFound
		}
		return nil, PayloadDescriptor{}, fmt.Errorf("load payload: %w", err)
	}
	t, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, PayloadDescriptor{}, fmt.Errorf("parse payload created_at: %w", err)
	}
	desc.CreatedAt = t
	return body, desc, nil
}

// Describe returns metadata without loading the body.
func (s *PayloadStore) Describe(ctx context.Context, id string) (PayloadDescriptor, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, sha256, content_type, size_bytes, created_at FROM payload_store WHERE id = ?`, id)
	var desc PayloadDescriptor
	var createdAt string
	if err := row.Scan(&desc.ID, &desc.Sha256, &desc.ContentType, &desc.SizeBytes, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PayloadDescriptor{}, ErrPayloadNotFound
		}
		return PayloadDescriptor{}, fmt.Errorf("describe payload: %w", err)
	}
	t, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return PayloadDescriptor{}, fmt.Errorf("parse payload created_at: %w", err)
	}
	desc.CreatedAt = t
	return desc, nil
}
