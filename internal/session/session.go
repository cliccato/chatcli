// Package session stores chat transcripts on disk.
package session

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const dirEnv = "CHATCLI_HOME" // overrides the config directory

type Store struct {
	dir string
}

type Session struct {
	Name    string
	History string // "Q: ...\nA: ...\n" transcript sent back as context
}

type Info struct {
	Name    string
	Bytes   int64
	ModTime time.Time
}

// Open creates the session directory if needed.
func Open() (*Store, error) {
	dir, err := baseDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func baseDir() (string, error) {
	if dir := os.Getenv(dirEnv); dir != "" {
		return filepath.Join(dir, "sessions"), nil
	}
	conf, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(".chatcli", "sessions"), nil
	}
	return filepath.Join(conf, "chatcli", "sessions"), nil
}

// Load reads a session; a missing file yields an empty history.
func (s *Store) Load(name string) (*Session, error) {
	name, err := sanitize(name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(s.path(name))
	if errors.Is(err, os.ErrNotExist) {
		return &Session{Name: name}, nil
	}
	if err != nil {
		return nil, err
	}
	return &Session{Name: name, History: string(data)}, nil
}

// Save writes the transcript atomically.
func (s *Store) Save(sess *Session) error {
	name, err := sanitize(sess.Name)
	if err != nil {
		return err
	}
	final := s.path(name)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, []byte(sess.History), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

// List returns the saved sessions, newest first.
func (s *Store) List() ([]Info, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	infos := make([]Info, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".txt") {
			continue
		}
		fi, err := entry.Info()
		if err != nil {
			continue
		}
		infos = append(infos, Info{
			Name:    strings.TrimSuffix(entry.Name(), ".txt"),
			Bytes:   fi.Size(),
			ModTime: fi.ModTime(),
		})
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].ModTime.After(infos[j].ModTime)
	})
	return infos, nil
}

func (s *Store) path(name string) string {
	return filepath.Join(s.dir, name+".txt")
}

// Append records one exchange and trims the transcript to limit bytes.
// It reports whether older exchanges were dropped.
func (sess *Session) Append(question, answer string, limit int) bool {
	sess.History += "Q: " + question + "\nA: " + answer + "\n\n"
	trimmed, changed := Trim(sess.History, limit)
	sess.History = trimmed
	return changed
}

// Trim drops the oldest exchanges until the transcript fits limit bytes.
// It returns "" when even the newest exchange does not fit.
func Trim(history string, limit int) (string, bool) {
	if len(history) <= limit {
		return history, false
	}
	lines := strings.Split(history, "\n")
	for i := 1; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "Q: ") {
			continue
		}
		rest := strings.Join(lines[i:], "\n")
		if len(rest) <= limit {
			return rest, true
		}
	}
	return "", true
}

func sanitize(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || len(name) > 64 {
		return "", fmt.Errorf("invalid session name %q", name)
	}
	for _, r := range name {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			r == '-' || r == '_' || r == '.'
		if !ok {
			return "", fmt.Errorf("invalid session name %q: use letters, digits, '-', '_', '.'", name)
		}
	}
	return name, nil
}
