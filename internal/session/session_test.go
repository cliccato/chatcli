package session

import "testing"

func TestTrimKeepsNewest(t *testing.T) {
	sess := &Session{Name: "t"}
	for i := 0; i < 10; i++ {
		sess.Append("question number one", "answer number one", 1<<20)
	}
	trimmed, changed := Trim(sess.History, 300)
	if !changed {
		t.Fatal("expected the transcript to be trimmed")
	}
	if len(trimmed) > 300 {
		t.Errorf("got %d bytes, want <= 300", len(trimmed))
	}
	if trimmed == "" {
		t.Fatal("newest exchanges must survive")
	}
	if trimmed[:3] != "Q: " {
		t.Errorf("trimmed transcript must start at an exchange, got %q", trimmed[:20])
	}
}

func TestTrimNoChange(t *testing.T) {
	got, changed := Trim("Q: a\nA: b\n\n", 100)
	if changed || got != "Q: a\nA: b\n\n" {
		t.Errorf("got %q changed=%v", got, changed)
	}
}

func TestSanitize(t *testing.T) {
	if _, err := sanitize("../evil"); err == nil {
		t.Error("path traversal must be rejected")
	}
	if _, err := sanitize("my-session_2.txt"); err != nil {
		t.Errorf("valid name rejected: %v", err)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	t.Setenv(dirEnv, t.TempDir())
	store, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Load("chat")
	if err != nil {
		t.Fatal(err)
	}
	sess.Append("hi", "hello", 1<<20)
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}
	again, err := store.Load("chat")
	if err != nil {
		t.Fatal(err)
	}
	if again.History != sess.History {
		t.Errorf("got %q, want %q", again.History, sess.History)
	}
	infos, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 || infos[0].Name != "chat" {
		t.Errorf("unexpected listing: %+v", infos)
	}
}
