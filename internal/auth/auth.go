package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"cargo.mleczki.pl/internal/domain"
	"cargo.mleczki.pl/internal/eventstore"
)

// AuthManager handles user authentication and registration.
type AuthManager struct {
	db         *sql.DB
	eventStore eventstore.EventStore
}

// NewAuthManager creates a new authentication manager.
func NewAuthManager(db *sql.DB, eventStore eventstore.EventStore) *AuthManager {
	return &AuthManager{
		db:         db,
		eventStore: eventStore,
	}
}

// RegisterUser registers a new user and emits UserRegistered event.
func (am *AuthManager) RegisterUser(ctx context.Context, cmd *domain.RegisterUserCommand) error {
	// Check if user already exists
	var exists bool
	err := am.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE email = ?)", cmd.Email).Scan(&exists)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to check user existence: %w", err)
	}
	if exists {
		return fmt.Errorf("user with email %s already exists", cmd.Email)
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(cmd.Password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Insert user into database
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = am.db.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, name, phone, address, is_adult, accepted_tos, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, cmd.UserID, cmd.Email, string(hash), cmd.Name, cmd.Phone, cmd.Address, cmd.IsAdult, cmd.AcceptedTOS, now, now)
	if err != nil {
		return fmt.Errorf("failed to insert user: %w", err)
	}

	// Emit UserRegistered event
	event := &domain.UserRegisteredEvent{
		UserID:      cmd.UserID,
		Name:        cmd.Name,
		Email:       cmd.Email,
		Phone:       cmd.Phone,
		Address:     cmd.Address,
		IsAdult:     cmd.IsAdult,
		AcceptedTOS: cmd.AcceptedTOS,
		Timestamp:   time.Now().UTC(),
	}

	eventData, err := eventstore.ToEvent(cmd.UserID, "user", event, 1)
	if err != nil {
		return fmt.Errorf("failed to create event: %w", err)
	}

	if err := am.eventStore.Save(ctx, eventData); err != nil {
		return fmt.Errorf("failed to save event: %w", err)
	}

	return nil
}

// Login authenticates a user and returns session token.
func (am *AuthManager) Login(ctx context.Context, email, password string) (string, *domain.User, error) {
	// Query user by email
	var user domain.User
	err := am.db.QueryRowContext(ctx, `
		SELECT id, email, password_hash, name, phone, address, is_adult, accepted_tos, 
		       deletion_requested, deletion_requested_at, created_at, updated_at
		FROM users WHERE email = ?
	`, email).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.Phone, &user.Address,
		&user.IsAdult, &user.AcceptedTOS, &user.DeletionRequested, &user.DeletionRequestedAt,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, fmt.Errorf("invalid email or password")
	}
	if err != nil {
		return "", nil, fmt.Errorf("failed to query user: %w", err)
	}

	// Check if user requested deletion
	if user.DeletionRequested {
		return "", nil, fmt.Errorf("account deletion requested, cannot login")
	}

	// Compare password hash
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return "", nil, fmt.Errorf("invalid email or password")
	}

	// Generate session token
	sessionToken := generateSessionToken()

	// Insert session into database
	expiresAt := time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.RFC3339)
	now := time.Now().UTC().Format(time.RFC3339)

	// Check if user is admin (hardcoded for now, should be from database)
	isAdmin := false // TODO: Add is_admin field to users table

	_, err = am.db.ExecContext(ctx, `
		INSERT INTO user_sessions (id, user_id, is_admin, created_at, expires_at, last_activity)
		VALUES (?, ?, ?, ?, ?, ?)
	`, sessionToken, user.ID, isAdmin, now, expiresAt, now)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create session: %w", err)
	}

	return sessionToken, &user, nil
}

// VerifySession verifies a session token and returns the user.
func (am *AuthManager) VerifySession(ctx context.Context, sessionToken string) (*domain.User, bool, error) {
	// Query session
	var userID string
	var isAdmin bool
	var expiresAt string
	err := am.db.QueryRowContext(ctx, `
		SELECT user_id, is_admin, expires_at FROM user_sessions WHERE id = ?
	`, sessionToken).Scan(&userID, &isAdmin, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, fmt.Errorf("invalid session")
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to query session: %w", err)
	}

	// Check if session is expired
	expiryTime, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return nil, false, fmt.Errorf("failed to parse expiry time: %w", err)
	}
	if time.Now().UTC().After(expiryTime) {
		return nil, false, fmt.Errorf("session expired")
	}

	// Update last activity
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = am.db.ExecContext(ctx, `
		UPDATE user_sessions SET last_activity = ? WHERE id = ?
	`, now, sessionToken)
	if err != nil {
		// Log error but don't fail the request
		fmt.Printf("Failed to update session activity: %v\n", err)
	}

	// Query user
	var user domain.User
	err = am.db.QueryRowContext(ctx, `
		SELECT id, email, password_hash, name, phone, address, is_adult, accepted_tos, 
		       deletion_requested, deletion_requested_at, created_at, updated_at
		FROM users WHERE id = ?
	`, userID).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.Phone, &user.Address,
		&user.IsAdult, &user.AcceptedTOS, &user.DeletionRequested, &user.DeletionRequestedAt,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, false, fmt.Errorf("failed to query user: %w", err)
	}

	return &user, isAdmin, nil
}

// Logout removes a session.
func (am *AuthManager) Logout(ctx context.Context, sessionToken string) error {
	_, err := am.db.ExecContext(ctx, "DELETE FROM user_sessions WHERE id = ?", sessionToken)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}

// generateSessionToken generates a random session token.
func generateSessionToken() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
