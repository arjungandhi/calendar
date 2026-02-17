package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/arjungandhi/calendar"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

func validCalendarNames(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	mgr, err := calendar.NewCalendarManager()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	sources, err := mgr.LoadSources()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	prefix := strings.ToLower(toComplete)
	var names []string
	for _, s := range sources {
		if prefix == "" || strings.HasPrefix(strings.ToLower(s.Name), prefix) {
			names = append(names, s.Name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

var rootCmd = &cobra.Command{
	Use:   "calendar",
	Short: "manage calendars and events",
}

var addCmd = &cobra.Command{
	Use:   "add [name] [url]",
	Short: "add a calendar source by iCal URL",
	Args:  cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
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

var removeCmd = &cobra.Command{
	Use:               "remove <name>",
	Short:             "remove a calendar source",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: validCalendarNames,
	RunE: func(cmd *cobra.Command, args []string) error {
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

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "sync all calendars from their iCal URLs",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := calendar.NewCalendarManager()
		if err != nil {
			return err
		}
		return mgr.SyncAll()
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "list configured calendars",
	RunE: func(cmd *cobra.Command, args []string) error {
		format, _ := cmd.Flags().GetString("output")
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

var eventsCmd = &cobra.Command{
	Use:   "events [today|week|month|YYYY-MM-DD [YYYY-MM-DD]]",
	Short: "list upcoming events",
	RunE: func(cmd *cobra.Command, args []string) error {
		format, _ := cmd.Flags().GetString("output")

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

var getCmd = &cobra.Command{
	Use:   "get <uid>",
	Short: "get event details by uid",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		format, _ := cmd.Flags().GetString("output")

		mgr, err := calendar.NewCalendarManager()
		if err != nil {
			return err
		}

		event, raw, err := mgr.GetEvent(args[0])
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

func init() {
	listCmd.Flags().StringP("output", "o", "table", "output format (table, json)")
	eventsCmd.Flags().StringP("output", "o", "table", "output format (table, json, ics)")
	getCmd.Flags().StringP("output", "o", "table", "output format (table, json, ics)")

	rootCmd.AddCommand(addCmd, removeCmd, syncCmd, listCmd, eventsCmd, getCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
