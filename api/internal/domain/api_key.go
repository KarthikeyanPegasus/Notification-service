package domain

import "time"

type APIKey struct {
	ID        string     `json:"id" db:"id"`
	Name      string     `json:"name" db:"name"`
	Prefix    string     `json:"prefix" db:"prefix"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty" db:"revoked_at"`
}
