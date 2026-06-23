package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: admin-reset <db_path> [new_password]")
		fmt.Println("If new_password is not provided, the admin user will be deleted (server will recreate with random password)")
		os.Exit(1)
	}

	dbPath := os.Args[1]
	newPassword := ""
	if len(os.Args) >= 3 {
		newPassword = os.Args[2]
	}

	// Open database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	if newPassword == "" {
		// Delete admin user
		result, err := db.ExecContext(ctx, "DELETE FROM users WHERE email = 'admin@example.com'")
		if err != nil {
			log.Fatalf("Failed to delete admin user: %v", err)
		}
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			fmt.Println("No admin user found with email admin@example.com")
		} else {
			fmt.Printf("Admin user deleted (%d row(s) affected). Restart the server to recreate with a new random password.\n", rowsAffected)
		}
	} else {
		// Update admin password
		hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			log.Fatalf("Failed to hash password: %v", err)
		}

		result, err := db.ExecContext(ctx, "UPDATE users SET password_hash = ? WHERE email = 'admin@example.com'", string(hash))
		if err != nil {
			log.Fatalf("Failed to update password: %v", err)
		}
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			fmt.Println("No admin user found with email admin@example.com")
		} else {
			fmt.Printf("Admin password updated successfully for %d user(s).\n", rowsAffected)
			fmt.Println("New password:", newPassword)
		}
	}
}
