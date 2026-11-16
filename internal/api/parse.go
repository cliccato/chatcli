package api

import (
	"bytes"
	"errors"
	"strings"
)

// ch.at answers "Q: <question>\nA: <answer>"; when the whole transcript is
// requested back the body holds the history too. The stream keeps only the
// answer and prints it while it arrives.

type state int

const (
	stateWait     state = iota // waiting for the question echo
	stateStrip                 // dropping the leading "A: " marker
	stateEmit                  // streaming the answer
	stateFallback              // unexpected shape, parse the body when done
)

const answerMarker = "\nA:"

type answerStream struct {
	echo     []byte // "Q: " + question, what the server echoes back
	question string
	state    state
	off      int // bytes of raw already consumed
	html     bool
	raw      bytes.Buffer
	out      bytes.Buffer
	emit     func([]byte)
}

func newAnswerStream(question string, emit func([]byte)) *answerStream {
	question = strings.ReplaceAll(question, "\r\n", "\n")
	return &answerStream{
		question: question,
		echo:     []byte("Q: " + question),
		emit:     emit,
	}
}

func (s *answerStream) feed(p []byte) {
	s.raw.Write(p)

	if s.state == stateWait && s.raw.Len() > 0 && s.raw.Bytes()[0] == '<' {
		s.html = true
		s.state = stateFallback
	}

	switch s.state {
	case stateWait:
		s.classify()
		if s.state == stateStrip {
			s.strip()
		}
		if s.state == stateEmit {
			s.flush()
		}
	case stateStrip:
		s.strip()
		if s.state == stateEmit {
			s.flush()
		}
	case stateEmit:
		s.flush()
	}
}

// classify waits for the echoed question followed by "A:"; anything else
// (transcript echo, HTML page) waits for EOF instead of guessing.
func (s *answerStream) classify() {
	raw := s.raw.Bytes()

	if len(raw) < len(s.echo) {
		if !bytes.HasPrefix(s.echo, raw) {
			s.state = stateFallback
		}
		return
	}
	if !bytes.Equal(raw[:len(s.echo)], s.echo) {
		s.state = stateFallback
		return
	}

	rest := raw[len(s.echo):]
	if len(rest) < len(answerMarker) {
		if !bytes.HasPrefix([]byte(answerMarker), rest) {
			s.state = stateFallback
		}
		return
	}
	if !bytes.Equal(rest[:len(answerMarker)], []byte(answerMarker)) {
		s.state = stateFallback
		return
	}

	s.off = len(s.echo) + len(answerMarker)
	s.state = stateStrip
}

// strip skips spaces and stray "A:" repeats before the answer text.
func (s *answerStream) strip() {
	raw := s.raw.Bytes()
	for s.off < len(raw) {
		switch c := raw[s.off]; {
		case c == ' ' || c == '\t':
			s.off++
		case c == 'A' && s.off+1 < len(raw) && raw[s.off+1] == ':':
			s.off += 2
		case c == 'A' && s.off+1 == len(raw):
			return // might be the start of a marker, wait for more
		default:
			s.state = stateEmit
			return
		}
	}
}

// flush forwards new bytes, holding back trailing blanks until it is clear
// they are not the end of the answer.
func (s *answerStream) flush() {
	data := s.raw.Bytes()[s.off:]
	end := len(data)
	for end > 0 && isBlank(data[end-1]) {
		end--
	}
	if end == 0 {
		return
	}
	chunk := data[:end]
	s.out.Write(chunk)
	s.emit(chunk)
	s.off += end
}

func isBlank(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}

// finish returns the answer; on an unexpected shape it parses the whole
// body and emits it in one go.
func (s *answerStream) finish() (string, error) {
	if s.html {
		return "", errors.New("server returned an HTML page instead of an answer")
	}
	if s.state == stateEmit {
		return strings.TrimRight(s.out.String(), " \t\r\n"), nil
	}
	answer := parseAnswer(s.raw.String(), s.question)
	if answer != "" {
		s.emit([]byte(answer))
	}
	return answer, nil
}

// parseAnswer extracts the answer from a full response body.
func parseAnswer(body, question string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")

	// single exchange or transcript tail: the answer follows our question
	if i := strings.LastIndex(body, "Q: "+question+"\nA:"); i >= 0 {
		return trimMarker(body[i+len("Q: "+question+"\nA:"):])
	}
	// the echo of a multi-line question ends where the answer line starts
	if lines := strings.Split(body, "\n"); strings.Count(question, "\n")+1 < len(lines) {
		if i := strings.Count(question, "\n") + 1; strings.HasPrefix(lines[i], "A:") {
			return trimMarker(strings.Join(lines[i:], "\n"))
		}
	}
	// last resort: text after the last "A:" line
	lines := strings.Split(body, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], "A:") {
			return trimMarker(strings.Join(lines[i:], "\n"))
		}
	}
	return strings.TrimSpace(body)
}

func trimMarker(s string) string {
	for {
		s = strings.TrimLeft(s, " \t")
		if !strings.HasPrefix(s, "A:") {
			break
		}
		s = s[2:]
	}
	return strings.TrimRight(s, " \t\r\n")
}
