package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type clientMessage struct {
	Type         string `json:"type"`
	Content      string `json:"content,omitempty"`
	CommandToken string `json:"_internal_cmd_token,omitempty"`
}

type serverMessage struct {
	Type         string `json:"type"`
	Status       string `json:"status"`
	Chunk        string `json:"chunk"`
	Error        string `json:"error"`
	CommandToken string `json:"_internal_cmd_token"`
}

func main() {
	var wsURL string
	var message string
	var timeoutSeconds int
	var expectStatus string
	var stopAfterFirstChunk bool
	var maxPostStopChunks int
	var minAnswerRunes int
	var requireAnyKeywords string
	var verbose bool

	flag.StringVar(&wsURL, "ws-url", "", "WebSocket URL")
	flag.StringVar(&message, "message", "", "chat message to send")
	flag.IntVar(&timeoutSeconds, "timeout-seconds", 30, "timeout in seconds")
	flag.StringVar(&expectStatus, "expect-status", "finished", "expected completion status")
	flag.BoolVar(&stopAfterFirstChunk, "stop-after-first-chunk", false, "send stop command after first chunk")
	flag.IntVar(&maxPostStopChunks, "max-post-stop-chunks", 1, "maximum allowed chunks received after stop command is sent")
	flag.IntVar(&minAnswerRunes, "min-answer-runes", 0, "minimum total answer length in runes")
	flag.StringVar(&requireAnyKeywords, "require-any-keywords", "", "pipe-separated keywords; at least one must appear in final answer")
	flag.BoolVar(&verbose, "verbose", false, "print websocket frames")
	flag.Parse()

	if wsURL == "" {
		fatal("missing -ws-url")
	}
	if message == "" {
		fatal("missing -message")
	}
	if expectStatus != "finished" && expectStatus != "stopped" {
		fatal("unsupported -expect-status, only finished|stopped")
	}
	if _, err := url.Parse(wsURL); err != nil {
		fatalf("invalid ws url: %v", err)
	}

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		fatalf("dial websocket failed: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(clientMessage{
		Type:    "message",
		Content: message,
	}); err != nil {
		fatalf("send chat message failed: %v", err)
	}

	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	var startedToken string
	var sawStarted bool
	var sawChunk bool
	var sawCompletion bool
	var stopSent bool
	var chunkCount int
	var chunksAfterStop int
	var answerBuilder strings.Builder

	for time.Now().Before(deadline) {
		if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
			fatalf("set read deadline failed: %v", err)
		}

		_, payload, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				break
			}
			if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
				continue
			}
			fatalf("read websocket message failed: %v", err)
		}

		if verbose {
			fmt.Printf("[DEBUG] %s\n", string(payload))
		}

		var msg serverMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			fatalf("unmarshal websocket message failed: %v", err)
		}
		if msg.Error != "" {
			fatalf("server returned error: %s", msg.Error)
		}

		switch {
		case msg.Type == "started":
			sawStarted = true
			startedToken = msg.CommandToken
			if startedToken == "" {
				fatal("started message missing _internal_cmd_token")
			}
		case msg.Chunk != "":
			sawChunk = true
			chunkCount++
			answerBuilder.WriteString(msg.Chunk)
			if stopSent {
				chunksAfterStop++
			}
			if stopAfterFirstChunk && !stopSent {
				if err := conn.WriteJSON(clientMessage{
					Type:         "stop",
					CommandToken: startedToken,
				}); err != nil {
					fatalf("send stop command failed: %v", err)
				}
				stopSent = true
			}
		case msg.Type == "completion":
			sawCompletion = true
			if msg.Status != expectStatus {
				fatalf("unexpected completion status: got=%s want=%s", msg.Status, expectStatus)
			}
			if !sawStarted {
				fatal("completion arrived before started")
			}
			if !sawChunk {
				fatal("no chunk received before completion")
			}
			finalAnswer := answerBuilder.String()
			validateFinalAnswer(finalAnswer, minAnswerRunes, requireAnyKeywords)
			if stopAfterFirstChunk && chunksAfterStop > maxPostStopChunks {
				fatalf("too many chunks after stop: got=%d want<=%d", chunksAfterStop, maxPostStopChunks)
			}
			fmt.Printf("[PASS] websocket probe ok: status=%s chunks=%d\n", msg.Status, chunkCount)
			return
		}
	}

	if !sawCompletion {
		fatalf("timeout waiting for completion, sawStarted=%v sawChunk=%v", sawStarted, sawChunk)
	}
}

func fatal(msg string) {
	fmt.Fprintf(os.Stderr, "[FAIL] %s\n", msg)
	os.Exit(1)
}

func fatalf(format string, args ...interface{}) {
	fatal(fmt.Sprintf(format, args...))
}

func validateFinalAnswer(answer string, minAnswerRunes int, requireAnyKeywords string) {
	if minAnswerRunes > 0 && len([]rune(strings.TrimSpace(answer))) < minAnswerRunes {
		fatalf("answer too short: got=%d want>=%d", len([]rune(strings.TrimSpace(answer))), minAnswerRunes)
	}
	if strings.TrimSpace(requireAnyKeywords) == "" {
		return
	}

	answerLower := strings.ToLower(answer)
	for _, keyword := range strings.Split(requireAnyKeywords, "|") {
		keyword = strings.TrimSpace(keyword)
		if keyword == "" {
			continue
		}
		if strings.Contains(answerLower, strings.ToLower(keyword)) {
			return
		}
	}

	fatalf("answer does not contain any required keywords: %s", requireAnyKeywords)
}
