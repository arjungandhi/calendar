package calendar

import (
	"os"
	"path/filepath"
)

// Config holds the calendar configuration directory path.
type Config struct {
	Dir string
}

// NewConfig creates a new Config. It reads the CALENDAR_DIR environment
// variable or defaults to ~/.config/calendar.
func NewConfig() (*Config, error) {
	dir := os.Getenv("CALENDAR_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, ".config", "calendar")
	}
	return &Config{Dir: dir}, nil
}

// EnsureDir creates the config directory if it doesn't exist.
func (c *Config) EnsureDir() error {
	return os.MkdirAll(c.Dir, 0755)
}

// SourcesFile returns the path to the sources.json file.
func (c *Config) SourcesFile() string {
	return filepath.Join(c.Dir, "sources.json")
}

// EventsDir returns the path to the events directory.
func (c *Config) EventsDir() string {
	return filepath.Join(c.Dir, "events")
}

// CalendarDir returns the path to a specific calendar's events directory.
func (c *Config) CalendarDir(name string) string {
	return filepath.Join(c.EventsDir(), name)
}
