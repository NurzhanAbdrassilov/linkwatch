package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"
)

type Cursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

func EncodeCursor(c Cursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func DecodeCursor(s string) (Cursor, error) {
	if s == "" {
		return Cursor{}, errors.New("empty")
	}
	dec, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, err
	}
	var c Cursor
	if err := json.Unmarshal(dec, &c); err != nil {
		return Cursor{}, err
	}
	return c, nil
}

func CtxTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, d)
}
