package notifications_test

import (
	"database/sql"
	"testing"

	"cargo.mleczki.pl/internal/notifications"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetAdminEmails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	notifier := notifications.NewAdminNotifierForTest(db)

	rows := sqlmock.NewRows([]string{"email"}).
		AddRow("admin1@example.com").
		AddRow("admin2@example.com")

	mock.ExpectQuery("SELECT email FROM users WHERE is_admin = 1").
		WillReturnRows(rows)

	emails, err := notifier.GetAdminEmails()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(emails) != 2 {
		t.Fatalf("Expected 2 emails, got %d", len(emails))
	}

	if emails[0] != "admin1@example.com" {
		t.Errorf("Expected admin1@example.com, got %s", emails[0])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unmet expectations: %v", err)
	}
}

func TestGetAdminEmails_NoAdmins(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	notifier := notifications.NewAdminNotifierForTest(db)

	rows := sqlmock.NewRows([]string{"email"})

	mock.ExpectQuery("SELECT email FROM users WHERE is_admin = 1").
		WillReturnRows(rows)

	emails, err := notifier.GetAdminEmails()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(emails) != 0 {
		t.Fatalf("Expected 0 emails, got %d", len(emails))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unmet expectations: %v", err)
	}
}

func TestGetAdminEmails_QueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	notifier := notifications.NewAdminNotifierForTest(db)

	mock.ExpectQuery("SELECT email FROM users WHERE is_admin = 1").
		WillReturnError(sql.ErrConnDone)

	_, err = notifier.GetAdminEmails()
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unmet expectations: %v", err)
	}
}

func TestNotifyOrderRequiringConfirmation_NoAdmins(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	notifier := notifications.NewAdminNotifierForTest(db)

	rows := sqlmock.NewRows([]string{"email"})

	mock.ExpectQuery("SELECT email FROM users WHERE is_admin = 1").
		WillReturnRows(rows)

	err = notifier.NotifyOrderRequiringConfirmation(
		t.Context(),
		"ORD-123",
		"John Doe",
		"john@example.com",
		"cash_pickup",
		100.0,
	)
	if err != nil {
		t.Fatalf("Expected no error when no admins, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unmet expectations: %v", err)
	}
}
