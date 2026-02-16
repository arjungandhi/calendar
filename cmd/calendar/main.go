package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/arjungandhi/calendar"
	"github.com/charmbracelet/huh"
	"github.com/rwxrob/bonzai"
	"github.com/rwxrob/bonzai/cmds/help"
	"github.com/rwxrob/bonzai/comp"
)

// calendarCompleter completes calendar names for commands that take a calendar arg.
type calendarCompleter struct{}

func (calendarCompleter) Complete(args ...string) []string {
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

var Cmd = &bonzai.Cmd{
	Name:  "calendar",
	Short: "manage calendars and events",
	Comp:  comp.CmdsOpts,
	Cmds:  []*bonzai.Cmd{help.Cmd, addCmd, removeCmd, syncCmd, listCmd, eventsCmd, getCmd},
}

var addCmd = &bonzai.Cmd{
	Name:  "add",
	Short: "add a calendar source by iCal URL",
	Usage: "[name] [url]",
	Do: func(x *bonzai.Cmd, args ...string) error {
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

var removeCmd = &bonzai.Cmd{
	Name:  "remove",
	Short: "remove a calendar source",
	Usage: "<name>",
	Comp:  calendarCompleter{},
	Do: func(x *bonzai.Cmd, args ...string) error {
		if len(args) < 1 {
			return fmt.Errorf("usage: calendar remove <name>")
		}
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

var syncCmd = &bonzai.Cmd{
	Name:  "sync",
	Short: "sync all calendars from their iCal URLs",
	Do: func(x *bonzai.Cmd, args ...string) error {
		mgr, err := calendar.NewCalendarManager()
		if err != nil {
			return err
		}
		return mgr.SyncAll()
	},
}

var listCmd = &bonzai.Cmd{
	Name:  "list",
	Short: "list configured calendars (-o table|json)",
	Usage: "[-o format]",
	Opts:  "table|json",
	Do: func(x *bonzai.Cmd, args ...string) error {
		format, _, err := parseOutputFlag(args, x.OptsSlice())
		if err != nil {
			return err
		}
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
		switch format {
		case "json":
			out, err := calendar.FormatSourcesJSON(sources)
			if err != nil {
				return err
			}
			fmt.Println(out)
		default: // table
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tURL")
			for _, s := range sources {
				fmt.Fprintf(w, "%s\t%s\n", s.Name, s.URL)
			}
			w.Flush()
		}
		return nil
	},
}

var eventsCmd = &bonzai.Cmd{
	Name:  "events",
	Short: "list upcoming events (-o table|json|ics)",
	Usage: "[-o format] [today|week|month|YYYY-MM-DD [YYYY-MM-DD]]",
	Opts:  "table|json|ics",
	Do: func(x *bonzai.Cmd, args ...string) error {
		format, rest, err := parseOutputFlag(args, x.OptsSlice())
		if err != nil {
			return err
		}

		mgr, err := calendar.NewCalendarManager()
		if err != nil {
			return err
		}

		now := time.Now()
		from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		to := from.AddDate(0, 0, 30)

		if len(rest) >= 1 {
			switch rest[0] {
			case "today":
				to = from.AddDate(0, 0, 1)
			case "week":
				to = from.AddDate(0, 0, 7)
			case "month":
				to = from.AddDate(0, 1, 0)
			default:
				t, err := time.Parse("2006-01-02", rest[0])
				if err != nil {
					return fmt.Errorf("invalid date %q (use YYYY-MM-DD, today, week, or month)", rest[0])
				}
				from = t
				to = t.AddDate(0, 0, 1)
				if len(rest) >= 2 {
					t2, err := time.Parse("2006-01-02", rest[1])
					if err != nil {
						return fmt.Errorf("invalid end date %q (use YYYY-MM-DD)", rest[1])
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

		switch format {
		case "json":
			out, err := calendar.FormatEventsJSON(events)
			if err != nil {
				return err
			}
			fmt.Println(out)
		case "ics":
			for _, e := range events {
				raw, err := mgr.GetEventICS(e.UID)
				if err != nil {
					continue
				}
				fmt.Print(raw)
			}
		default: // table
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "TIME\tSUMMARY\tLOCATION\tCALENDAR")
			for _, e := range events {
				var timeStr string
				if e.AllDay {
					timeStr = e.Start.Format("2006-01-02") + " (all day)"
				} else {
					timeStr = e.Start.Format("2006-01-02 15:04")
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", timeStr, e.Summary, e.Location, e.Calendar)
			}
			w.Flush()
		}
		return nil
	},
}

var getCmd = &bonzai.Cmd{
	Name:  "get",
	Short: "get event details by uid (-o table|json|ics)",
	Usage: "[-o format] <uid>",
	Opts:  "table|json|ics",
	Do: func(x *bonzai.Cmd, args ...string) error {
		format, rest, err := parseOutputFlag(args, x.OptsSlice())
		if err != nil {
			return err
		}
		if len(rest) == 0 {
			return fmt.Errorf("usage: calendar get [-o format] <uid>")
		}

		mgr, err := calendar.NewCalendarManager()
		if err != nil {
			return err
		}

		event, raw, err := mgr.GetEvent(rest[0])
		if err != nil {
			return err
		}

		switch format {
		case "json":
			out, err := calendar.FormatEventJSON(event)
			if err != nil {
				return err
			}
			fmt.Println(out)
		case "ics":
			fmt.Print(raw)
		default: // table
			fmt.Print(calendar.FormatEvent(event))
		}
		return nil
	},
}

// parseOutputFlag extracts -o <format> from args, returning the format
// (defaulting to "table") and the remaining args.
func parseOutputFlag(args []string, valid []string) (string, []string, error) {
	format := "table"
	var rest []string
	for i := 0; i < len(args); i++ {
		if args[i] == "-o" {
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("-o requires a format: %s", strings.Join(valid, ", "))
			}
			format = strings.ToLower(args[i+1])
			found := false
			for _, v := range valid {
				if format == v {
					found = true
					break
				}
			}
			if !found {
				return "", nil, fmt.Errorf("unknown output format %q: use %s", format, strings.Join(valid, ", "))
			}
			i++ // skip the format value
		} else {
			rest = append(rest, args[i])
		}
	}
	return format, rest, nil
}

func main() {
	Cmd.Exec()
}
