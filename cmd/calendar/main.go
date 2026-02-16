package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/arjungandhi/calendar"
	"github.com/charmbracelet/huh"
	"github.com/rwxrob/bonzai"
	Z "github.com/rwxrob/bonzai/z"
	"github.com/rwxrob/help"
)

// calendarCompleter completes calendar names for commands that take a calendar arg.
type calendarCompleter struct{}

func (calendarCompleter) Complete(_ bonzai.Command, args ...string) []string {
	mgr, err := calendar.NewCalendarManager()
	if err != nil {
		return nil
	}
	sources, err := mgr.LoadSources()
	if err != nil {
		return nil
	}
	prefix := ""
	if len(args) > 0 {
		prefix = strings.ToLower(args[0])
	}
	var names []string
	for _, s := range sources {
		if prefix == "" || strings.HasPrefix(strings.ToLower(s.Name), prefix) {
			names = append(names, s.Name)
		}
	}
	return names
}

var Cmd = &Z.Cmd{
	Name:     "calendar",
	Summary:  "manage calendars and events",
	Commands: []*Z.Cmd{help.Cmd, addCmd, removeCmd, syncCmd, listCmd, eventsCmd, getCmd},
}

var addCmd = &Z.Cmd{
	Name:    "add",
	Summary: "add a calendar source by iCal URL",
	Usage:   "[name] [url]",
	Call: func(x *Z.Cmd, args ...string) error {
		var name, url string

		if len(args) >= 2 {
			name = args[0]
			url = args[1]
		} else {
			form := huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title("Calendar Name").
						Description("A short name for this calendar").
						Value(&name),
					huh.NewInput().
						Title("iCal URL").
						Description("The .ics URL for this calendar").
						Value(&url),
				),
			)
			if err := form.Run(); err != nil {
				return err
			}
		}

		if name == "" || url == "" {
			return fmt.Errorf("name and URL are required")
		}

		mgr, err := calendar.NewCalendarManager()
		if err != nil {
			return err
		}
		if err := mgr.AddSource(name, url); err != nil {
			return err
		}
		fmt.Printf("added calendar %q\n", name)
		return nil
	},
}

var removeCmd = &Z.Cmd{
	Name:    "remove",
	Summary: "remove a calendar source",
	Usage:   "<name>",
	MinArgs: 1,
	Comp:    calendarCompleter{},
	Call: func(x *Z.Cmd, args ...string) error {
		mgr, err := calendar.NewCalendarManager()
		if err != nil {
			return err
		}
		if err := mgr.RemoveSource(args[0]); err != nil {
			return err
		}
		fmt.Printf("removed calendar %q\n", args[0])
		return nil
	},
}

var syncCmd = &Z.Cmd{
	Name:    "sync",
	Summary: "sync all calendars from their iCal URLs",
	Call: func(x *Z.Cmd, args ...string) error {
		mgr, err := calendar.NewCalendarManager()
		if err != nil {
			return err
		}
		return mgr.SyncAll()
	},
}

var listCmd = &Z.Cmd{
	Name:    "list",
	Summary: "list configured calendars",
	Call: func(x *Z.Cmd, args ...string) error {
		mgr, err := calendar.NewCalendarManager()
		if err != nil {
			return err
		}
		sources, err := mgr.LoadSources()
		if err != nil {
			return err
		}
		if len(sources) == 0 {
			fmt.Println("no calendars configured")
			return nil
		}
		for _, s := range sources {
			fmt.Printf("%s\t%s\n", s.Name, s.URL)
		}
		return nil
	},
}

var eventsCmd = &Z.Cmd{
	Name:    "events",
	Summary: "list upcoming events",
	Usage:   "[today|week|month|YYYY-MM-DD [YYYY-MM-DD]]",
	Call: func(x *Z.Cmd, args ...string) error {
		mgr, err := calendar.NewCalendarManager()
		if err != nil {
			return err
		}

		now := time.Now()
		from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		to := from.AddDate(0, 0, 30)

		if len(args) >= 1 {
			switch args[0] {
			case "today":
				to = from.AddDate(0, 0, 1)
			case "week":
				to = from.AddDate(0, 0, 7)
			case "month":
				to = from.AddDate(0, 1, 0)
			default:
				t, err := time.Parse("2006-01-02", args[0])
				if err != nil {
					return fmt.Errorf("invalid date %q (use YYYY-MM-DD, today, week, or month)", args[0])
				}
				from = t
				to = t.AddDate(0, 0, 1)
				if len(args) >= 2 {
					t2, err := time.Parse("2006-01-02", args[1])
					if err != nil {
						return fmt.Errorf("invalid end date %q (use YYYY-MM-DD)", args[1])
					}
					to = t2.AddDate(0, 0, 1)
				}
			}
		}

		events, err := mgr.ListEvents(from, to)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			fmt.Println("no events found")
			return nil
		}

		for _, e := range events {
			var timeStr string
			if e.AllDay {
				timeStr = e.Start.Format("2006-01-02") + " (all day)"
			} else {
				timeStr = e.Start.Format("2006-01-02 15:04")
			}
			fmt.Printf("%s\t%s\t%s\t%s\n", timeStr, e.Summary, e.Location, e.Calendar)
		}
		return nil
	},
}

var getCmd = &Z.Cmd{
	Name:    "get",
	Summary: "get event details by UID",
	Usage:   "[--ics] <uid>",
	MinArgs: 1,
	Call: func(x *Z.Cmd, args ...string) error {
		showICS := false
		if args[0] == "--ics" {
			showICS = true
			args = args[1:]
		}
		if len(args) == 0 {
			return fmt.Errorf("usage: calendar get [--ics] <uid>")
		}

		mgr, err := calendar.NewCalendarManager()
		if err != nil {
			return err
		}

		event, raw, err := mgr.GetEvent(args[0])
		if err != nil {
			return err
		}

		if showICS {
			fmt.Print(raw)
		} else {
			fmt.Print(calendar.FormatEvent(event))
		}
		return nil
	},
}

func main() {
	Cmd.Run()
}
