// chatcli is a terminal client for the ch.at service.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/cliccato/chatcli/internal/api"
	"github.com/cliccato/chatcli/internal/session"
	"github.com/cliccato/chatcli/internal/tui"
)

const inputLimit = 1 << 20 // longest line accepted on stdin

func main() {
	name := flag.String("session", "default", "session name")
	once := flag.Bool("once", false, "ask the opening question, print the answer and exit")
	list := flag.Bool("list", false, "list saved sessions")
	clear := flag.Bool("clear", false, "clear the session history and exit")
	flag.Usage = usage
	flag.Parse()

	tui.Init()

	store, err := session.Open()
	if err != nil {
		fatal(err)
	}

	switch {
	case *list:
		listSessions(store, "")
		return
	case *clear:
		sess, err := store.Load(*name)
		if err != nil {
			fatal(err)
		}
		if err := store.Save(&session.Session{Name: sess.Name}); err != nil {
			fatal(err)
		}
		fmt.Printf("session %q cleared\n", sess.Name)
		return
	}

	sess, err := store.Load(*name)
	if err != nil {
		fatal(err)
	}
	if h, trimmed := session.Trim(sess.History, api.MaxHistory); trimmed {
		sess.History = h
	}

	app := &app{client: api.NewClient(), store: store, sess: sess}
	app.run(strings.Join(flag.Args(), " "), *once)
}

type app struct {
	client *api.Client
	store  *session.Store
	sess   *session.Session
}

// run sends the optional opening question, then reads stdin until exit.
func (a *app) run(initial string, once bool) {
	if initial == "" && once {
		tui.Warn("-once needs a question")
		return
	}
	if initial != "" {
		a.ask(initial)
	}
	if once {
		return
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), inputLimit)
	for {
		tui.Prompt()
		if !scanner.Scan() {
			fmt.Println()
			break
		}
		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == "":
		case isQuit(line):
			fmt.Println("Goodbye!")
			return
		case strings.HasPrefix(line, "/"):
			if a.command(line) {
				return
			}
		default:
			a.ask(line)
		}
	}
	if err := scanner.Err(); err != nil {
		tui.Error(fmt.Errorf("input: %w", err))
	}
}

// ask sends one question and streams the answer to stdout.
func (a *app) ask(question string) {
	ctx, stop := cancelOnInterrupt(context.Background())
	defer stop()

	printed := false
	answer, err := a.client.Ask(ctx, question, a.sess.History, func(p []byte) {
		if !printed {
			tui.AnswerPrefix()
			printed = true
		}
		tui.Write(p)
	})
	if printed {
		fmt.Println()
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		tui.Warn("request canceled")
		return
	}
	if err != nil {
		tui.Error(err)
		return
	}
	if answer == "" {
		return
	}
	if trimmed := a.sess.Append(question, answer, api.MaxHistory); trimmed {
		tui.Warn("context trimmed to the newest exchanges")
	}
	if err := a.store.Save(a.sess); err != nil {
		tui.Warn("saving session: %v", err)
	}
}

// command handles "/" lines and reports whether the chat must stop.
func (a *app) command(line string) bool {
	cmd, arg, _ := strings.Cut(line, " ")
	cmd = strings.ToLower(cmd)
	arg = strings.TrimSpace(arg)

	switch cmd {
	case "/help":
		fmt.Print(helpText)
	case "/sessions":
		listSessions(a.store, a.sess.Name)
	case "/session":
		if arg == "" {
			tui.Warn("usage: /session <name>")
			return false
		}
		if err := a.store.Save(a.sess); err != nil {
			tui.Warn("saving session: %v", err)
			return false
		}
		sess, err := a.store.Load(arg)
		if err != nil {
			tui.Warn("%v", err)
			return false
		}
		if h, _ := session.Trim(sess.History, api.MaxHistory); h != sess.History {
			sess.History = h
		}
		a.sess = sess
		fmt.Printf("session %q loaded (%d bytes of context)\n", sess.Name, len(sess.History))
	case "/new":
		a.sess.History = ""
		if err := a.store.Save(a.sess); err != nil {
			tui.Warn("saving session: %v", err)
			return false
		}
		fmt.Printf("session %q cleared\n", a.sess.Name)
	case "/history":
		if strings.TrimSpace(a.sess.History) == "" {
			fmt.Println("empty history")
			return false
		}
		fmt.Print(a.sess.History)
	case "/exit", "/quit":
		return true
	default:
		tui.Warn("unknown command %q, try /help", cmd)
	}
	return false
}

func isQuit(line string) bool {
	switch strings.ToLower(line) {
	case "exit", "quit", "/exit", "/quit":
		return true
	}
	return false
}

func listSessions(store *session.Store, current string) {
	infos, err := store.List()
	if err != nil {
		fatal(err)
	}
	if len(infos) == 0 {
		fmt.Println("no saved sessions")
		return
	}
	for _, info := range infos {
		mark := " "
		if info.Name == current {
			mark = "*"
		}
		fmt.Printf("%s %-24s %8d B  %s\n", mark, info.Name, info.Bytes, info.ModTime.Format("2006-01-02 15:04"))
	}
}

// cancelOnInterrupt aborts the running request on the first Ctrl-C.
func cancelOnInterrupt(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		select {
		case <-sig:
			cancel()
		case <-ctx.Done():
		}
		signal.Stop(sig)
	}()
	return ctx, cancel
}

func fatal(err error) {
	tui.Error(err)
	os.Exit(1)
}

const helpText = `commands:
  /help              show this help
  /sessions          list saved sessions
  /session <name>    switch to another session
  /new               clear the current session context
  /history           print the current transcript
  /exit              quit (also: exit, quit)

usage: chatcli [flags] [question...]
`

func usage() {
	fmt.Fprint(flag.CommandLine.Output(), "chatcli - terminal client for ch.at\n\n")
	flag.PrintDefaults()
	fmt.Fprint(flag.CommandLine.Output(), "\n"+helpText)
}
