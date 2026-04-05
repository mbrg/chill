package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/mbrg/chill/internal/store"
)

const defaultDB = "chill.db"

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	dbPath := defaultDB
	// Support --db=path as first arg.
	args := os.Args[1:]
	if len(args) > 0 && strings.HasPrefix(args[0], "--db=") {
		dbPath = strings.TrimPrefix(args[0], "--db=")
		args = args[1:]
	}

	if len(args) < 1 {
		usage()
	}

	switch args[0] {
	case "user":
		if len(args) < 2 {
			fatal("Usage: chill user <add|list|remove>")
		}
		runUser(dbPath, args[1], args[2:])
	case "events":
		if len(args) < 2 || args[1] != "list" {
			fatal("Usage: chill events list")
		}
		runEventsList(dbPath)
	case "deliveries":
		if len(args) < 2 || args[1] != "list" {
			fatal("Usage: chill deliveries list --event-id=X")
		}
		runDeliveriesList(dbPath, args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage: chill [--db=path] <command>

Commands:
  user add --telegram-id=ID --area=AREA
  user list
  user remove --telegram-id=ID
  events list
  deliveries list --event-id=ID`)
	os.Exit(1)
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}

func openStore(dbPath string) *store.Store {
	s, err := store.New(dbPath)
	if err != nil {
		fatal(fmt.Sprintf("open database: %v", err))
	}
	return s
}

func parseFlag(args []string, name string) string {
	prefix := "--" + name + "="
	for _, a := range args {
		if strings.HasPrefix(a, prefix) {
			return strings.TrimPrefix(a, prefix)
		}
	}
	return ""
}

func runUser(dbPath, subcmd string, args []string) {
	switch subcmd {
	case "add":
		tidStr := parseFlag(args, "telegram-id")
		area := parseFlag(args, "area")
		if tidStr == "" || area == "" {
			fatal("Usage: chill user add --telegram-id=ID --area=AREA")
		}
		tid, err := strconv.ParseInt(tidStr, 10, 64)
		if err != nil {
			fatal(fmt.Sprintf("invalid telegram-id: %v", err))
		}

		s := openStore(dbPath)
		defer s.Close()
		if err := s.UpsertUser(tid, area); err != nil {
			fatal(fmt.Sprintf("add user: %v", err))
		}
		fmt.Printf("User %d added (area: %s)\n", tid, area)

	case "list":
		s := openStore(dbPath)
		defer s.Close()
		users, err := s.AllUsers()
		if err != nil {
			fatal(fmt.Sprintf("list users: %v", err))
		}
		if len(users) == 0 {
			fmt.Println("No users.")
			return
		}
		fmt.Printf("%-6s %-14s %-20s %-8s %s\n", "ID", "TELEGRAM_ID", "AREA", "ACTIVE", "UPDATED")
		for _, u := range users {
			active := "yes"
			if !u.Active {
				active = "no"
			}
			fmt.Printf("%-6d %-14d %-20s %-8s %s\n", u.ID, u.TelegramID, u.Area, active, u.UpdatedAt)
		}

	case "remove":
		tidStr := parseFlag(args, "telegram-id")
		if tidStr == "" {
			fatal("Usage: chill user remove --telegram-id=ID")
		}
		tid, err := strconv.ParseInt(tidStr, 10, 64)
		if err != nil {
			fatal(fmt.Sprintf("invalid telegram-id: %v", err))
		}

		s := openStore(dbPath)
		defer s.Close()
		if err := s.DeactivateUser(tid); err != nil {
			fatal(fmt.Sprintf("remove user: %v", err))
		}
		fmt.Printf("User %d deactivated\n", tid)

	default:
		fatal("Usage: chill user <add|list|remove>")
	}
}

func runEventsList(dbPath string) {
	s := openStore(dbPath)
	defer s.Close()
	events, err := s.RecentEvents(50)
	if err != nil {
		fatal(fmt.Sprintf("list events: %v", err))
	}
	if len(events) == 0 {
		fmt.Println("No events.")
		return
	}
	fmt.Printf("%-6s %-20s %-5s %-30s %-30s %s\n", "ID", "OREF_ID", "CAT", "TITLE", "AREAS", "RECEIVED")
	for _, e := range events {
		areas := strings.Join(e.Areas, ", ")
		fmt.Printf("%-6d %-20s %-5d %-30s %-30s %s\n", e.ID, e.OrefID, e.Category, e.Title, areas, e.ReceivedAt)
	}
}

func runDeliveriesList(dbPath string, args []string) {
	eidStr := parseFlag(args, "event-id")
	if eidStr == "" {
		fatal("Usage: chill deliveries list --event-id=ID")
	}
	eid, err := strconv.ParseInt(eidStr, 10, 64)
	if err != nil {
		fatal(fmt.Sprintf("invalid event-id: %v", err))
	}

	s := openStore(dbPath)
	defer s.Close()
	deliveries, err := s.GetDeliveries(eid)
	if err != nil {
		fatal(fmt.Sprintf("list deliveries: %v", err))
	}
	if len(deliveries) == 0 {
		fmt.Println("No deliveries.")
		return
	}
	fmt.Printf("%-6s %-10s %-10s %-8s %-20s %s\n", "ID", "EVENT_ID", "USER_ID", "STATUS", "SENT_AT", "ERROR")
	for _, d := range deliveries {
		fmt.Printf("%-6d %-10d %-10d %-8s %-20s %s\n", d.ID, d.EventID, d.UserID, d.Status, d.SentAt, d.Error)
	}
}
