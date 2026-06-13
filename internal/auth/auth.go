package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math/big"
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
		INSERT INTO users (id, email, password_hash, name, phone, address, is_adult, accepted_tos, is_admin, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, cmd.UserID, cmd.Email, string(hash), cmd.Name, cmd.Phone, cmd.Address, cmd.IsAdult, cmd.AcceptedTOS, 0, now, now)
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
	var isAdmin int
	var deletionRequestedAtStr *string
	var createdAtStr, updatedAtStr string
	err := am.db.QueryRowContext(ctx, `
		SELECT id, email, password_hash, name, phone, address, is_adult, accepted_tos, is_admin,
		       deletion_requested, deletion_requested_at, created_at, updated_at
		FROM users WHERE email = ?
	`, email).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.Phone, &user.Address,
		&user.IsAdult, &user.AcceptedTOS, &isAdmin, &user.DeletionRequested, &deletionRequestedAtStr,
		&createdAtStr, &updatedAtStr,
	)
	if errors.Is(err, sql.ErrNoRows) {
		// Log failed login attempt for non-existent user
		log.Printf("Failed login attempt: user not found for email %s", email)
		return "", nil, fmt.Errorf("invalid email or password")
	}
	if err != nil {
		log.Printf("Failed login attempt: database error for email %s: %v", email, err)
		return "", nil, fmt.Errorf("failed to query user: %w", err)
	}

	// Parse time fields
	if createdAtStr != "" {
		user.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	}
	if updatedAtStr != "" {
		user.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
	}
	if deletionRequestedAtStr != nil && *deletionRequestedAtStr != "" {
		t, _ := time.Parse(time.RFC3339, *deletionRequestedAtStr)
		user.DeletionRequestedAt = &t
	}

	// Check if user requested deletion
	if user.DeletionRequested {
		log.Printf("Failed login attempt: account deletion requested for user %s", user.ID)
		return "", nil, fmt.Errorf("account deletion requested, cannot login")
	}

	// Compare password hash
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		// Log failed login attempt due to invalid password
		log.Printf("Failed login attempt: invalid password for user %s (email: %s)", user.ID, email)
		return "", nil, fmt.Errorf("invalid email or password")
	}

	// Set IsAdmin on user object
	user.IsAdmin = isAdmin == 1

	// Generate session token
	sessionToken := generateSessionToken()

	// Insert session into database
	expiresAt := time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.RFC3339)
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = am.db.ExecContext(ctx, `
		INSERT INTO user_sessions (id, user_id, is_admin, created_at, expires_at, last_activity)
		VALUES (?, ?, ?, ?, ?, ?)
	`, sessionToken, user.ID, isAdmin, now, expiresAt, now)
	if err != nil {
		log.Printf("Failed login attempt: failed to create session for user %s: %v", user.ID, err)
		return "", nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Log successful login
	log.Printf("Successful login for user %s (email: %s, admin: %t)", user.ID, user.Email, user.IsAdmin)

	return sessionToken, &user, nil
}

// VerifySession verifies a session token and returns the user.
func (am *AuthManager) VerifySession(ctx context.Context, sessionToken string) (*domain.User, bool, error) {
	// Query session
	var userID string
	var isAdmin int
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
	var userIsAdmin int
	var deletionRequestedAtStr *string
	var createdAtStr, updatedAtStr string
	err = am.db.QueryRowContext(ctx, `
		SELECT id, email, password_hash, name, phone, address, is_adult, accepted_tos, is_admin,
		       deletion_requested, deletion_requested_at, created_at, updated_at
		FROM users WHERE id = ?
	`, userID).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.Phone, &user.Address,
		&user.IsAdult, &user.AcceptedTOS, &userIsAdmin, &user.DeletionRequested, &deletionRequestedAtStr,
		&createdAtStr, &updatedAtStr,
	)
	if err != nil {
		return nil, false, fmt.Errorf("failed to query user: %w", err)
	}

	// Parse time fields
	if createdAtStr != "" {
		user.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	}
	if updatedAtStr != "" {
		user.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
	}
	if deletionRequestedAtStr != nil && *deletionRequestedAtStr != "" {
		t, _ := time.Parse(time.RFC3339, *deletionRequestedAtStr)
		user.DeletionRequestedAt = &t
	}

	user.IsAdmin = userIsAdmin == 1

	return &user, isAdmin == 1, nil
}

// Logout removes a session.
func (am *AuthManager) Logout(ctx context.Context, sessionToken string) error {
	_, err := am.db.ExecContext(ctx, "DELETE FROM user_sessions WHERE id = ?", sessionToken)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}

// EnsureAdminUser creates an admin user if one doesn't exist.
// Returns the generated password if a new admin was created, empty string otherwise.
func (am *AuthManager) EnsureAdminUser(ctx context.Context) (string, error) {
	// Check if admin user already exists
	var count int
	err := am.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE is_admin = 1").Scan(&count)
	if err != nil {
		return "", fmt.Errorf("failed to check admin users: %w", err)
	}

	// If admin already exists, do nothing
	if count > 0 {
		return "", nil
	}

	// Generate random password
	password := generateRandomPassword()

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}

	// Insert admin user
	now := time.Now().UTC().Format(time.RFC3339)
	userID := fmt.Sprintf("user_%d", time.Now().UnixNano())
	_, err = am.db.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, name, phone, address, is_adult, accepted_tos, is_admin, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, userID, "admin@example.com", string(hash), "Admin", "", "", 1, 1, 1, now, now)
	if err != nil {
		return "", fmt.Errorf("failed to insert admin user: %w", err)
	}

	return password, nil
}

// generateRandomPassword generates a random 16-character password.
func generateRandomPassword() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	b := make([]byte, 16)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

// generateSessionToken generates a random session token.
func generateSessionToken() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// GenerateSecureToken generates a cryptographically secure random token.
func GenerateSecureToken(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	tokenBytes := make([]byte, length)
	for i := range tokenBytes {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		tokenBytes[i] = charset[n.Int64()]
	}
	return string(tokenBytes), nil
}

// RequestPasswordReset creates a password reset token for a user.
func (am *AuthManager) RequestPasswordReset(ctx context.Context, email string) (string, error) {
	// Check if user exists
	var userID string
	err := am.db.QueryRowContext(ctx, "SELECT id FROM users WHERE email = ?", email).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		// Don't reveal if user exists or not for security
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to query user: %w", err)
	}

	// Generate secure token
	token, err := GenerateSecureToken(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	// Delete any existing unused tokens for this user
	_, err = am.db.ExecContext(ctx, "DELETE FROM password_reset_tokens WHERE user_id = ? AND used = 0", userID)
	if err != nil {
		return "", fmt.Errorf("failed to delete old tokens: %w", err)
	}

	// Insert new token
	tokenID := fmt.Sprintf("prt_%d", time.Now().UnixNano())
	expiresAt := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = am.db.ExecContext(ctx, `
		INSERT INTO password_reset_tokens (id, user_id, token, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, tokenID, userID, token, expiresAt, now)
	if err != nil {
		return "", fmt.Errorf("failed to insert token: %w", err)
	}

	return token, nil
}

// VerifyPasswordResetToken verifies a password reset token and returns the user ID.
func (am *AuthManager) VerifyPasswordResetToken(ctx context.Context, token string) (string, error) {
	var userID string
	var expiresAt string
	var used int

	err := am.db.QueryRowContext(ctx, `
		SELECT user_id, expires_at, used FROM password_reset_tokens WHERE token = ?
	`, token).Scan(&userID, &expiresAt, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("invalid or expired token")
	}
	if err != nil {
		return "", fmt.Errorf("failed to query token: %w", err)
	}

	// Check if token is already used
	if used == 1 {
		return "", fmt.Errorf("token already used")
	}

	// Check if token is expired
	expiryTime, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return "", fmt.Errorf("failed to parse expiry time: %w", err)
	}
	if time.Now().UTC().After(expiryTime) {
		return "", fmt.Errorf("token expired")
	}

	return userID, nil
}

// ResetPassword resets a user's password using a reset token.
func (am *AuthManager) ResetPassword(ctx context.Context, token, newPassword string) error {
	// Verify token and get user ID
	userID, err := am.VerifyPasswordResetToken(ctx, token)
	if err != nil {
		return err
	}

	// Hash new password
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update user password
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = am.db.ExecContext(ctx, `
		UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?
	`, string(hash), now, userID)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	// Mark token as used
	_, err = am.db.ExecContext(ctx, "UPDATE password_reset_tokens SET used = 1 WHERE token = ?", token)
	if err != nil {
		// Log error but don't fail the request
		log.Printf("Failed to mark token as used: %v", err)
	}

	// Invalidate all user sessions for security
	_, err = am.db.ExecContext(ctx, "DELETE FROM user_sessions WHERE user_id = ?", userID)
	if err != nil {
		// Log error but don't fail the request
		log.Printf("Failed to invalidate sessions: %v", err)
	}

	return nil
}

// UpdateUserProfile updates a user's profile information.
func (am *AuthManager) UpdateUserProfile(ctx context.Context, userID, name, phone, address string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := am.db.ExecContext(ctx, `
		UPDATE users SET name = ?, phone = ?, address = ?, updated_at = ? WHERE id = ?
	`, name, phone, address, now, userID)
	if err != nil {
		return fmt.Errorf("failed to update user profile: %w", err)
	}
	return nil
}

// ChangePassword changes a user's password after verifying the current password.
func (am *AuthManager) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	// Get current password hash
	var currentHash string
	err := am.db.QueryRowContext(ctx, "SELECT password_hash FROM users WHERE id = ?", userID).Scan(&currentHash)
	if err != nil {
		return fmt.Errorf("failed to query user: %w", err)
	}

	// Verify current password
	err = bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(currentPassword))
	if err != nil {
		return fmt.Errorf("current password is incorrect")
	}

	// Hash new password
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update password
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = am.db.ExecContext(ctx, `
		UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?
	`, string(hash), now, userID)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	// Invalidate all user sessions for security
	_, err = am.db.ExecContext(ctx, "DELETE FROM user_sessions WHERE user_id = ?", userID)
	if err != nil {
		// Log error but don't fail the request
		log.Printf("Failed to invalidate sessions: %v", err)
	}

	return nil
}

// GetAllUsers retrieves all users (for admin).
func (am *AuthManager) GetAllUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := am.db.QueryContext(ctx, `
		SELECT id, email, name, phone, address, is_adult, accepted_tos, is_admin,
		       deletion_requested, created_at, updated_at
		FROM users ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var user domain.User
		var isAdmin int
		var createdAtStr, updatedAtStr string
		err := rows.Scan(
			&user.ID, &user.Email, &user.Name, &user.Phone, &user.Address,
			&user.IsAdult, &user.AcceptedTOS, &isAdmin, &user.DeletionRequested,
			&createdAtStr, &updatedAtStr,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}

		user.IsAdmin = isAdmin == 1
		if createdAtStr != "" {
			user.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		}
		if updatedAtStr != "" {
			user.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
		}

		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate users: %w", err)
	}

	return users, nil
}

// AdminResetPassword resets a user's password (admin override).
func (am *AuthManager) AdminResetPassword(ctx context.Context, userID string) (string, error) {
	// Generate random password
	password := generateRandomPassword()

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}

	// Update password
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = am.db.ExecContext(ctx, `
		UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?
	`, string(hash), now, userID)
	if err != nil {
		return "", fmt.Errorf("failed to update password: %w", err)
	}

	// Invalidate all user sessions for security
	_, err = am.db.ExecContext(ctx, "DELETE FROM user_sessions WHERE user_id = ?", userID)
	if err != nil {
		// Log error but don't fail the request
		log.Printf("Failed to invalidate sessions: %v", err)
	}

	return password, nil
}

// AdminUpdateUser updates a user's details (admin override).
func (am *AuthManager) AdminUpdateUser(ctx context.Context, userID, name, email, phone, address string, isAdmin bool) error {
	now := time.Now().UTC().Format(time.RFC3339)
	adminFlag := 0
	if isAdmin {
		adminFlag = 1
	}

	_, err := am.db.ExecContext(ctx, `
		UPDATE users SET name = ?, email = ?, phone = ?, address = ?, is_admin = ?, updated_at = ?
		WHERE id = ?
	`, name, email, phone, address, adminFlag, now, userID)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}
	return nil
}
