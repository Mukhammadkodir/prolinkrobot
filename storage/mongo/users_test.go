package mongo

import (
	"get-link-tg-bot/models"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func TestBuildSaveTelegramUserUpdateAvoidsMongoFieldConflicts(t *testing.T) {
	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	req := &models.SaveTelegramUserRequest{
		TelegramID: 42,
		Name:       "Alice",
		Username:   "alice",
		Language:   "uz",
	}

	update := buildSaveTelegramUserUpdate(req, now, 1, "2026-04-01")

	setOnInsert, ok := update["$setOnInsert"].(bson.M)
	if !ok {
		t.Fatalf("$setOnInsert has unexpected type %T", update["$setOnInsert"])
	}
	setFields, ok := update["$set"].(bson.M)
	if !ok {
		t.Fatalf("$set has unexpected type %T", update["$set"])
	}

	for key := range setOnInsert {
		if _, exists := setFields[key]; exists {
			t.Fatalf("field %q appears in both $setOnInsert and $set", key)
		}
	}

	if got := setOnInsert["lang"]; got != "uz" {
		t.Fatalf("lang mismatch: got %v", got)
	}
	if got := setFields["name"]; got != "Alice" {
		t.Fatalf("name mismatch: got %v", got)
	}
	if got := setFields["username"]; got != "alice" {
		t.Fatalf("username mismatch: got %v", got)
	}
}
