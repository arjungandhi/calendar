package calendar

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	ical "github.com/emersion/go-ical"
)

// Source represents a calendar source with a name and iCal URL.
type Source struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Event represents a parsed calendar event.
type Event struct {
	UID         string
	Summary     string
	Description string
	Location    string
	Start       time.Time
	End         time.Time
	Calendar    string
	AllDay      bool
}

// CalendarManager handles calendar source management and event storage.
type CalendarManager struct {
	Config *Config
}

// NewCalendarManager creates a new CalendarManager with default config.
func NewCalendarManager() (*CalendarManager, error) {
	cfg, err := NewConfig()
	if err != nil {
		return nil, err
	}
	if err := cfg.EnsureDir(); err != nil {
		return nil, err
	}
	return &CalendarManager{Config: cfg}, nil
}

// --- Source Management ---

// LoadSources reads the configured calendar sources from disk.
func (m *CalendarManager) LoadSources() ([]Source, error) {
	data, err := os.ReadFile(m.Config.SourcesFile())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var sources []Source
	if err := json.Unmarshal(data, &sources); err != nil {
		return nil, err
	}
	return sources, nil
}

// SaveSources writes the calendar sources to disk.
func (m *CalendarManager) SaveSources(sources []Source) error {
	data, err := json.MarshalIndent(sources, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.Config.SourcesFile(), data, 0644)
}

// AddSource adds a new calendar source.
func (m *CalendarManager) AddSource(name, url string) error {
	sources, err := m.LoadSources()
	if err != nil {
		return err
	}
	for _, s := range sources {
		if s.Name == name {
			return fmt.Errorf("calendar %q already exists", name)
		}
	}
	sources = append(sources, Source{Name: name, URL: url})
	return m.SaveSources(sources)
}

// RemoveSource removes a calendar source and its local events.
func (m *CalendarManager) RemoveSource(name string) error {
	sources, err := m.LoadSources()
	if err != nil {
		return err
	}
	var filtered []Source
	found := false
	for _, s := range sources {
		if s.Name == name {
			found = true
			continue
		}
		filtered = append(filtered, s)
	}
	if !found {
		return fmt.Errorf("calendar %q not found", name)
	}
	os.RemoveAll(m.Config.CalendarDir(name))
	return m.SaveSources(filtered)
}

// --- Sync ---

// SyncAll syncs all configured calendar sources.
func (m *CalendarManager) SyncAll() error {
	sources, err := m.LoadSources()
	if err != nil {
		return err
	}
	if len(sources) == 0 {
		return fmt.Errorf("no calendars configured, use 'add' to add one")
	}
	for _, s := range sources {
		fmt.Printf("syncing %s...\n", s.Name)
		if err := m.syncSource(s); err != nil {
			fmt.Printf("  error: %v\n", err)
			continue
		}
	}
	return nil
}

func (m *CalendarManager) syncSource(s Source) error {
	resp, err := http.Get(s.URL)
	if err != nil {
		return fmt.Errorf("fetching calendar: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching calendar: HTTP %d", resp.StatusCode)
	}

	dec := ical.NewDecoder(resp.Body)
	cal, err := dec.Decode()
	if err != nil {
		return fmt.Errorf("parsing calendar: %w", err)
	}

	dir := m.Config.CalendarDir(s.Name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Clear existing events before writing fresh data
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		os.Remove(filepath.Join(dir, e.Name()))
	}

	count := 0
	for _, event := range cal.Events() {
		uid, err := event.Props.Text(ical.PropUID)
		if err != nil || uid == "" {
			continue
		}

		// Wrap the event in its own calendar object so the .ics file is valid
		eventCal := ical.NewCalendar()
		eventCal.Props.SetText(ical.PropVersion, "2.0")
		eventCal.Props.SetText(ical.PropProductID, "-//arjungandhi/calendar//EN")
		eventCal.Children = append(eventCal.Children, event.Component)

		var buf strings.Builder
		enc := ical.NewEncoder(&buf)
		if err := enc.Encode(eventCal); err != nil {
			continue
		}

		filename := sanitizeFilename(uid) + ".ics"
		if err := os.WriteFile(filepath.Join(dir, filename), []byte(buf.String()), 0644); err != nil {
			continue
		}
		count++
	}
	fmt.Printf("  %d events synced\n", count)
	return nil
}

// --- Event Retrieval ---

// ListEvents returns events within the given time range from all calendars.
func (m *CalendarManager) ListEvents(from, to time.Time) ([]Event, error) {
	sources, err := m.LoadSources()
	if err != nil {
		return nil, err
	}

	var events []Event
	for _, s := range sources {
		calEvents, err := m.loadCalendarEvents(s.Name)
		if err != nil {
			continue
		}
		events = append(events, calEvents...)
	}

	var filtered []Event
	for _, e := range events {
		if !from.IsZero() && e.Start.Before(from) {
			continue
		}
		if !to.IsZero() && e.Start.After(to) {
			continue
		}
		filtered = append(filtered, e)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Start.Before(filtered[j].Start)
	})

	return filtered, nil
}

func (m *CalendarManager) loadCalendarEvents(calName string) ([]Event, error) {
	dir := m.Config.CalendarDir(calName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var events []Event
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".ics") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		event, err := readEvent(path, calName)
		if err != nil {
			continue
		}
		events = append(events, *event)
	}
	return events, nil
}

func readEvent(path, calName string) (*Event, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	dec := ical.NewDecoder(strings.NewReader(string(data)))
	cal, err := dec.Decode()
	if err != nil {
		return nil, err
	}

	icalEvents := cal.Events()
	if len(icalEvents) == 0 {
		return nil, fmt.Errorf("no events in file")
	}

	ie := icalEvents[0]
	uid, _ := ie.Props.Text(ical.PropUID)
	summary, _ := ie.Props.Text(ical.PropSummary)
	description, _ := ie.Props.Text(ical.PropDescription)
	location, _ := ie.Props.Text(ical.PropLocation)

	start, allDay := parseEventTime(&ie, ical.PropDateTimeStart)
	end, _ := parseEventTime(&ie, ical.PropDateTimeEnd)

	return &Event{
		UID:         uid,
		Summary:     summary,
		Description: description,
		Location:    location,
		Start:       start,
		End:         end,
		Calendar:    calName,
		AllDay:      allDay,
	}, nil
}

func parseEventTime(event *ical.Event, prop string) (time.Time, bool) {
	p := event.Props.Get(prop)
	if p == nil {
		return time.Time{}, false
	}

	// Check if it's an all-day event (VALUE=DATE)
	allDay := false
	if values, ok := p.Params["VALUE"]; ok {
		for _, v := range values {
			if v == "DATE" {
				allDay = true
			}
		}
	}

	// Try to resolve timezone from TZID parameter
	loc := time.Local
	if tzids, ok := p.Params["TZID"]; ok && len(tzids) > 0 {
		if l, err := time.LoadLocation(tzids[0]); err == nil {
			loc = l
		}
	}

	if allDay {
		t, err := time.Parse("20060102", p.Value)
		if err != nil {
			return time.Time{}, false
		}
		return t, true
	}

	t, err := p.DateTime(loc)
	if err != nil {
		// Fallback: try parsing as date only
		t, err = time.Parse("20060102", p.Value)
		if err != nil {
			return time.Time{}, false
		}
		return t, true
	}
	return t, false
}

// GetEvent finds an event by UID across all calendars.
func (m *CalendarManager) GetEvent(uid string) (*Event, string, error) {
	sources, err := m.LoadSources()
	if err != nil {
		return nil, "", err
	}

	for _, s := range sources {
		dir := m.Config.CalendarDir(s.Name)
		entries, _ := os.ReadDir(dir)
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".ics") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			event, err := readEvent(path, s.Name)
			if err != nil {
				continue
			}
			if event.UID == uid {
				raw, _ := os.ReadFile(path)
				return event, string(raw), nil
			}
		}
	}
	return nil, "", fmt.Errorf("event %q not found", uid)
}

// FormatEvent returns a human-readable representation of an event.
func FormatEvent(e *Event) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Summary:     %s\n", e.Summary)
	fmt.Fprintf(&b, "Calendar:    %s\n", e.Calendar)
	if e.AllDay {
		fmt.Fprintf(&b, "Date:        %s\n", e.Start.Format("Mon, 02 Jan 2006"))
		if !e.End.IsZero() && !e.End.Equal(e.Start) {
			fmt.Fprintf(&b, "End:         %s\n", e.End.Format("Mon, 02 Jan 2006"))
		}
	} else {
		fmt.Fprintf(&b, "Start:       %s\n", e.Start.Format("Mon, 02 Jan 2006 15:04 MST"))
		if !e.End.IsZero() {
			fmt.Fprintf(&b, "End:         %s\n", e.End.Format("Mon, 02 Jan 2006 15:04 MST"))
		}
	}
	if e.Location != "" {
		fmt.Fprintf(&b, "Location:    %s\n", e.Location)
	}
	if e.Description != "" {
		fmt.Fprintf(&b, "Description: %s\n", e.Description)
	}
	fmt.Fprintf(&b, "UID:         %s\n", e.UID)
	return b.String()
}

func sanitizeFilename(s string) string {
	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_",
		"?", "_", "\"", "_", "<", "_", ">", "_",
		"|", "_", "@", "_at_",
	)
	return replacer.Replace(s)
}
