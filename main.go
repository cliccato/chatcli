package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const (
	URLStr       = "https://ch.at/"
	MaxHistorySz = 64 * 1024 // 64 KB
)

var (
	history string
	client  = &http.Client{}

	// ANSI color codes
	colorReset  = "\033[0m"
	colorCyan   = "\033[96m"
	colorGreen  = "\033[92m"
	colorYellow = "\033[93m"
	colorRed    = "\033[91m"
)

// sendRequest invia una POST form urlencoded con parametri q e h e ritorna la risposta come stringa.
func sendRequest(q, h string) (string, error) {
	form := url.Values{}
	form.Set("q", q)
	form.Set("h", h)

	req, err := http.NewRequest("POST", URLStr, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "+
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP error: %s", resp.Status)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(bodyBytes)), nil
}

// extractLastAnswer estrae l'ultima risposta (testo dopo ultimo "A:") dalla risposta completa.
func extractLastAnswer(fullResponse string) string {
	lines := strings.Split(fullResponse, "\n")
	var lastAnswerLines []string
	found := false

	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "A:") {
			lastAnswerLines = append([]string{strings.TrimSpace(trimmed[2:])}, lastAnswerLines...)
			found = true
		} else if found {
			break
		}
	}

	if !found {
		return strings.TrimSpace(fullResponse)
	}

	return strings.Join(lastAnswerLines, " ")
}

func printPrompt() {
	fmt.Print(colorCyan + "You: " + colorReset)
}

func printResponse(answer string) {
	// Solo il prefisso 'ch.at >' è verde, risposta senza colore
	fmt.Println(colorGreen + "ch.at: " + colorReset + answer)
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)

	for {
		printPrompt()
		if !scanner.Scan() {
			fmt.Println()
			break
		}
		userInput := strings.TrimSpace(scanner.Text())
		if userInput == "" {
			continue
		}

		lowerInput := strings.ToLower(userInput)
		if lowerInput == "exit" || lowerInput == "quit" {
			fmt.Println("Exiting chat. Goodbye!")
			break
		}

		if len([]byte(history)) > MaxHistorySz {
			fmt.Println(colorYellow + "History too long, resetting chat history..." + colorReset)
			history = ""
		}

		response, err := sendRequest(userInput, history)
		if err != nil {
			fmt.Println(colorRed+"[Error during request:", err, "]"+colorReset)
			continue
		}

		lastAnswer := extractLastAnswer(response)

		historyBuf := bytes.NewBufferString(history)
		historyBuf.WriteString(fmt.Sprintf("Q: %s\nA: %s\n\n", userInput, lastAnswer))
		history = historyBuf.String()

		printResponse(lastAnswer)
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "Error reading standard input:", err)
	}
}
