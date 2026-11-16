package api

import (
	"strings"
	"testing"
)

func TestParseAnswer(t *testing.T) {
	cases := []struct {
		name, body, question, want string
	}{
		{
			name:     "single line",
			body:     "Q: hello\nA: hi there",
			question: "hello",
			want:     "hi there",
		},
		{
			name:     "multi line answer",
			body:     "Q: list\nA: first\nsecond\nthird",
			question: "list",
			want:     "first\nsecond\nthird",
		},
		{
			name:     "question with A: line",
			body:     "Q: remember\nA: alpha\nB: beta\nA: got it",
			question: "remember\nA: alpha\nB: beta",
			want:     "got it",
		},
		{
			name:     "stray marker",
			body:     "Q: q\nA: A: answer",
			question: "q",
			want:     "answer",
		},
		{
			name:     "no echo",
			body:     "just text\nA: the answer\nmore",
			question: "q",
			want:     "the answer\nmore",
		},
		{
			name: "whole transcript back",
			body: "Q: first question\nA: first answer\n\n" +
				"Q: second question\nA: second answer\n",
			question: "second question",
			want:     "second answer",
		},
		{
			name: "repeated question in history",
			body: "Q: again\nA: old answer\n\n" +
				"Q: again\nA: new answer\n",
			question: "again",
			want:     "new answer",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseAnswer(tc.body, tc.question)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// The streaming path must return the same text as the full-body parser,
// whatever the chunk boundaries are.
func TestStreamMatchesParse(t *testing.T) {
	question := "remember this\nA: alpha\nB: beta"
	body := "Q: remember this\nA: alpha\nB: beta\nA: line one\nline two\n\nA: stray\n"
	want := parseAnswer(body, question)

	for _, size := range []int{1, 2, 3, 7, 64, 4096} {
		var got []byte
		s := newAnswerStream(question, func(p []byte) { got = append(got, p...) })
		for i := 0; i < len(body); i += size {
			end := min(i+size, len(body))
			s.feed([]byte(body[i:end]))
		}
		answer, err := s.finish()
		if err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
		if string(got) != want {
			t.Errorf("size %d: streamed %q, want %q", size, got, want)
		}
		if answer != want {
			t.Errorf("size %d: returned %q, want %q", size, answer, want)
		}
	}
}

func TestStreamHTML(t *testing.T) {
	s := newAnswerStream("hi", func([]byte) {})
	s.feed([]byte("<!DOCTYPE html>\n<html></html>"))
	if _, err := s.finish(); err == nil {
		t.Error("expected an error for an HTML body")
	}
}

// A transcript echo is not streamed: it must still yield the right answer.
func TestStreamTranscriptFallback(t *testing.T) {
	question := "second question"
	body := "Q: first question\nA: first answer\n\nQ: second question\nA: second answer\n"
	var got []byte
	s := newAnswerStream(question, func(p []byte) { got = append(got, p...) })
	for i := 0; i < len(body); i += 5 {
		s.feed([]byte(body[i:min(i+5, len(body))]))
	}
	answer, err := s.finish()
	if err != nil {
		t.Fatal(err)
	}
	if answer != "second answer" || string(got) != "second answer" {
		t.Errorf("got %q emitted %q", answer, got)
	}
}

func TestStreamEmptyAnswer(t *testing.T) {
	body := "Q: hi\nA: "
	s := newAnswerStream("hi", func([]byte) {})
	s.feed([]byte(body))
	answer, err := s.finish()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(answer) != "" {
		t.Errorf("got %q, want empty", answer)
	}
}
