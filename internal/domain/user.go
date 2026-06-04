package domain

import "time"

// User Aggregate
type User struct {
	ID                  string
	Name                string
	Email               string
	Phone               string
	Address             string
	PasswordHash        string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DeletionRequestedAt *time.Time
	IsDeleted           bool
	Version             int
}

// User Commands
type RegisterUserCommand struct {
	UserID     string
	Name       string
	Email      string
	Phone      string
	Address    string
	Password   string
}

type UpdateUserDetailsCommand struct {
	UserID   string
	Name     string
	Email    string
	Phone    string
	Address  string
}

type RequestAccountDeletionCommand struct {
	UserID string
}

// User Events
type UserRegisteredEvent struct {
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone"`
	Address   string    `json:"address"`
	Timestamp time.Time `json:"timestamp"`
}

func (e *UserRegisteredEvent) EventType() string {
	return "UserRegistered"
}

type UserDetailsUpdatedEvent struct {
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone"`
	Address   string    `json:"address"`
	Timestamp time.Time `json:"timestamp"`
}

func (e *UserDetailsUpdatedEvent) EventType() string {
	return "UserDetailsUpdated"
}

type UserDeletionRequestedEvent struct {
	UserID    string    `json:"user_id"`
	Timestamp time.Time `json:"timestamp"`
}

func (e *UserDeletionRequestedEvent) EventType() string {
	return "UserDeletionRequested"
}
