package store

import "time"

type Target struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Host      string    `json:"host"`
	CreatedAt time.Time `json:"created_at"`
}
