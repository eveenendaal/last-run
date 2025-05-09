package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3" // Import driver for sqlite3
)

// Task represents a tracked task with its last run time
type Task struct {
	ID      string
	LastRun time.Time
	db      *sql.DB
}

// Update updates the last run time for an existing task
func (t *Task) Update() error {
	newValue := t.LastRun
	if err := t.Select(); err != nil {
		return fmt.Errorf("failed to select task before update: %w", err)
	}

	t.LastRun = newValue
	_, err := t.db.Exec("UPDATE tasks SET last_run = ? WHERE id = ?", t.LastRun.Format(time.RFC3339), t.ID)
	if err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	return nil
}

// Insert creates a new task record
func (t *Task) Insert() error {
	_, err := t.db.Exec("INSERT INTO tasks (id, last_run) VALUES (?, ?)", t.ID, t.LastRun.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("failed to insert task: %w", err)
	}

	return nil
}

// Select retrieves a task's information from the database
func (t *Task) Select() error {
	rows, err := t.db.Query("SELECT * FROM tasks WHERE id = ?", t.ID)
	if err != nil {
		return fmt.Errorf("failed to query task: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		// No record found, insert a new one
		log.Printf("No record found for task ID: %s", t.ID)
		return t.Insert()
	}

	var lastRun string
	if err = rows.Scan(&t.ID, &lastRun); err != nil {
		return fmt.Errorf("failed to scan task data: %w", err)
	}

	t.LastRun, err = time.Parse(time.RFC3339, lastRun)
	if err != nil {
		return fmt.Errorf("failed to parse last run time: %w", err)
	}

	return nil
}

// initDB initializes the database connection and creates the tasks table if it doesn't exist
func initDB() (*sql.DB, error) {
	usr, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}

	// Ensure the directory exists
	dbDir := filepath.Join(usr.HomeDir, ".tasks")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	dbPath := filepath.Join(dbDir, "data.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create a table if it doesn't exist
	_, err = db.Exec("CREATE TABLE IF NOT EXISTS tasks (id TEXT PRIMARY KEY, last_run TEXT)")
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create table: %w", err)
	}

	return db, nil
}

// shouldRunTask determines if a task should run based on its last run time and the specified duration
func shouldRunTask(lastRun time.Time, duration time.Duration) (bool, string) {
	// For daily or longer durations, truncate to days for cleaner comparisons
	if duration.Hours() >= 24 {
		lastRun = lastRun.Truncate(24 * time.Hour)
	}

	timeSinceLastRun := time.Since(lastRun)
	if timeSinceLastRun.Hours() >= duration.Hours() {
		return true, fmt.Sprintf("Task is due (last run: %s, %s ago)",
			lastRun.Format(time.RFC3339),
			formatDuration(timeSinceLastRun))
	}

	return false, fmt.Sprintf("Task is not due yet (last run: %s, %s ago, threshold: %s)",
		lastRun.Format(time.RFC3339),
		formatDuration(timeSinceLastRun),
		formatDuration(duration))
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	d = d.Round(time.Minute)

	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour

	hours := d / time.Hour
	d -= hours * time.Hour

	minutes := d / time.Minute

	if days > 0 {
		return fmt.Sprintf("%dd%dh%dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func main() {
	// Define command line flags
	updateCmd := flag.NewFlagSet("update", flag.ExitOnError)
	updateID := updateCmd.String("id", "", "Task ID to update")

	checkCmd := flag.NewFlagSet("check", flag.ExitOnError)
	checkID := checkCmd.String("id", "", "Task ID to check")
	checkDuration := checkCmd.String("duration", "24h", "Duration threshold (e.g., 24h, 7d)")

	// Validate arguments
	if len(os.Args) < 2 {
		fmt.Println("Usage:")
		fmt.Println("  lastrun update -id=<task_id>")
		fmt.Println("  lastrun check -id=<task_id> -duration=<duration>")
		os.Exit(1)
	}

	// Initialize database
	db, err := initDB()
	if err != nil {
		log.Fatalf("Database initialization failed: %v", err)
	}
	defer db.Close()

	// Process commands
	switch os.Args[1] {
	case "update":
		updateCmd.Parse(os.Args[2:])
		if *updateID == "" {
			log.Fatal("Task ID is required")
		}

		task := &Task{ID: *updateID, LastRun: time.Now(), db: db}
		if err := task.Update(); err != nil {
			log.Fatalf("Failed to update task: %v", err)
		}

		fmt.Printf("Task %s updated at %s\n", task.ID, task.LastRun.Format(time.RFC3339))

	case "check":
		checkCmd.Parse(os.Args[2:])
		if *checkID == "" {
			log.Fatal("Task ID is required")
		}

		duration, err := time.ParseDuration(*checkDuration)
		if err != nil {
			log.Fatalf("Invalid duration format: %v", err)
		}

		task := &Task{ID: *checkID, db: db}
		if err := task.Select(); err != nil {
			log.Fatalf("Failed to retrieve task: %v", err)
		}

		shouldRun, message := shouldRunTask(task.LastRun, duration)
		fmt.Println(message)

		// Exit with error code 1 if the task is due to run
		if shouldRun {
			os.Exit(1)
		}

	default:
		log.Fatalf("Unknown command: %s", os.Args[1])
	}
}
