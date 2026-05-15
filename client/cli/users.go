package cli

import (
	"database/sql"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/matinhimself/mattube/client/auth"
)

// CreateUser creates a user in the DB and prints the result.
// isAdmin=true for admin accounts.
func CreateUser(db *sql.DB, username, password string, isAdmin bool) {
	if username == "" || password == "" {
		fatalf("usage: create-admin <username> <password>  or  create-user <username> <password>")
	}
	id, err := auth.CreateUser(db, username, password, isAdmin)
	if err != nil {
		fatalf("create user: %v", err)
	}
	role := "user"
	if isAdmin {
		role = "admin"
	}
	fmt.Printf("created %s  id=%d  username=%s\n", role, id, username)
}

// ListUsers prints all users in a table.
func ListUsers(db *sql.DB) {
	users, err := auth.ListUsers(db)
	if err != nil {
		fatalf("list users: %v", err)
	}
	if len(users) == 0 {
		fmt.Println("no users found")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tUSERNAME\tADMIN\tLAST LOGIN")
	for _, u := range users {
		lastLogin := "-"
		if u.LastLogin != nil {
			lastLogin = *u.LastLogin
		}
		admin := "no"
		if u.IsAdmin {
			admin = "yes"
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", u.ID, u.Username, admin, lastLogin)
	}
	w.Flush()
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
