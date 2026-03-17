package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// CLI exit codes.
const (
	exitSuccess   = 0
	exitFailure   = 1
	exitUsage     = 2
	exitNotFound  = 3
	exitForbidden = 4
)

// cliErr signals a desired exit code to main without printing anything extra.
// The command itself is responsible for printing the error before returning it.
type cliErr struct{ code int }

func (e *cliErr) Error() string { return fmt.Sprintf("exit %d", e.code) }

// outFmt is the resolved output format for a command invocation.
type outFmt int

const (
	fmtHuman outFmt = iota
	fmtJSON
	fmtQuiet
)

// resolveFormat picks the output format:
//   - quiet wins over everything
//   - json flag or non-TTY stdout → JSON
//   - otherwise human
func resolveFormat(jsonFlag, quietFlag bool) outFmt {
	if quietFlag {
		return fmtQuiet
	}
	if jsonFlag || !isTTY() {
		return fmtJSON
	}
	return fmtHuman
}

// isTTY reports whether stdout is an interactive terminal.
func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// printJSONError writes a structured error payload to stdout and returns a
// *cliErr with the given code. Always use this in JSON mode so agents get
// machine-readable failures.
func printJSONError(code int, errType, message string, input map[string]any, retryable bool) error {
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"error":     errType,
		"message":   message,
		"input":     input,
		"retryable": retryable,
	})
	return &cliErr{code}
}

// printHumanError writes a plain error to stderr and returns *cliErr.
func printHumanError(code int, message string) error {
	fmt.Fprintln(os.Stderr, "Error:", message)
	return &cliErr{code}
}

// handleAPIError maps an *APIError to the right exit code and output format.
func handleAPIError(err error, fmt_ outFmt, input map[string]any) error {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		if fmt_ == fmtJSON {
			return printJSONError(exitFailure, "request_failed", err.Error(), input, true)
		}
		return printHumanError(exitFailure, err.Error())
	}

	switch apiErr.StatusCode {
	case http.StatusNotFound:
		if fmt_ == fmtJSON {
			return printJSONError(exitNotFound, "not_found", apiErr.Message, input, false)
		}
		return printHumanError(exitNotFound, apiErr.Message)
	case http.StatusUnauthorized, http.StatusForbidden:
		if fmt_ == fmtJSON {
			return printJSONError(exitForbidden, "unauthorized", apiErr.Message, input, false)
		}
		return printHumanError(exitForbidden, apiErr.Message)
	default:
		if fmt_ == fmtJSON {
			return printJSONError(exitFailure, "server_error", apiErr.Message, input, true)
		}
		return printHumanError(exitFailure, apiErr.Message)
	}
}

// uuidRE validates storage IDs (UUIDs).
var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ── Flags ─────────────────────────────────────────────────────────────────────

var (
	clientURL     string
	clientToken   string
	clientProfile string
	jsonFlag      bool
	quietFlag     bool
)

// ── inbox ─────────────────────────────────────────────────────────────────────

var inboxCmd = &cobra.Command{
	Use:   "inbox",
	Short: "Interact with your virtual inbox",
	PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
		return resolveClientFlags()
	},
}

func init() {
	inboxCmd.PersistentFlags().StringVarP(&clientURL, "url", "u", "",
		"Submail server URL (overrides profile, env: SUBMAIL_URL)")
	inboxCmd.PersistentFlags().StringVarP(&clientToken, "token", "t", "",
		"Bearer token for the agent (overrides profile, env: SUBMAIL_TOKEN)")
	inboxCmd.PersistentFlags().StringVarP(&clientProfile, "profile", "p", "",
		"Connection profile to use (env: SUBMAIL_PROFILE, default: \"default\")")
	inboxCmd.PersistentFlags().BoolVar(&jsonFlag, "json", false,
		"Output JSON to stdout (auto-enabled when stdout is not a TTY)")
	inboxCmd.PersistentFlags().BoolVarP(&quietFlag, "quiet", "q", false,
		"Output only IDs, one per line (pipe-friendly)")

	inboxCmd.AddCommand(inboxListCmd)
	inboxCmd.AddCommand(inboxGetCmd)
	rootCmd.AddCommand(inboxCmd)
}

// resolveClientFlags fills clientURL and clientToken using the precedence:
//  1. Explicit flags (--url / --token)
//  2. Environment variables (SUBMAIL_URL / SUBMAIL_TOKEN)
//  3. Named profile (--profile / SUBMAIL_PROFILE, falls back to "default")
func resolveClientFlags() error {
	// 1 & 2 — flags then env vars
	if clientURL == "" {
		clientURL = os.Getenv("SUBMAIL_URL")
	}
	if clientToken == "" {
		clientToken = os.Getenv("SUBMAIL_TOKEN")
	}

	// 3 — profile (only consulted for values still missing)
	if clientURL == "" || clientToken == "" {
		name := clientProfile
		if name == "" {
			name = os.Getenv("SUBMAIL_PROFILE")
		}
		if name == "" {
			name = "default"
		}

		prof, err := loadProfile(name)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return &cliErr{exitFailure}
		}
		if prof != nil {
			if clientURL == "" {
				clientURL = prof.URL
			}
			if clientToken == "" {
				clientToken = prof.Token
			}
		}
	}

	if clientURL == "" {
		fmt.Fprintln(os.Stderr, "Error: server URL is required (--url, SUBMAIL_URL, or a profile)")
		return &cliErr{exitUsage}
	}
	if clientToken == "" {
		fmt.Fprintln(os.Stderr, "Error: agent token is required (--token, SUBMAIL_TOKEN, or a profile)")
		return &cliErr{exitUsage}
	}
	return nil
}

// ── inbox list ────────────────────────────────────────────────────────────────

var (
	listLimit  int
	listOffset int
)

var inboxListCmd = &cobra.Command{
	Use:   "list",
	Short: "List messages in the inbox",
	Long: `List messages delivered to your agent's inbox.

Output defaults to a human-readable table when stdout is a TTY, and JSON
otherwise. Use --json to force JSON, or --quiet for bare IDs.

Examples:
  # Human table (interactive)
  submail inbox list --url http://localhost:8080 --token mytoken

  # JSON (agent / piped)
  submail inbox list --json

  # Paginate
  submail inbox list --limit 10 --offset 20

  # Pipe IDs into another command
  submail inbox list -q | xargs -I{} submail inbox get {}

  # Environment variables
  export SUBMAIL_URL=http://localhost:8080
  export SUBMAIL_TOKEN=mytoken
  submail inbox list`,
	RunE: runInboxList,
}

func init() {
	inboxListCmd.Flags().IntVar(&listLimit, "limit", 50, "Maximum number of messages to return (1–200)")
	inboxListCmd.Flags().IntVar(&listOffset, "offset", 0, "Number of messages to skip")
}

func runInboxList(_ *cobra.Command, _ []string) error {
	if listLimit < 1 || listLimit > 200 {
		fmt.Fprintln(os.Stderr, "Error: --limit must be between 1 and 200")
		return &cliErr{exitUsage}
	}
	if listOffset < 0 {
		fmt.Fprintln(os.Stderr, "Error: --offset must be >= 0")
		return &cliErr{exitUsage}
	}

	fmt_ := resolveFormat(jsonFlag, quietFlag)
	client := NewClient(clientURL, clientToken)

	result, err := client.ListMails(cmd_ctx(), listLimit, listOffset)
	if err != nil {
		return handleAPIError(err, fmt_, map[string]any{"limit": listLimit, "offset": listOffset})
	}

	switch fmt_ {
	case fmtJSON:
		return json.NewEncoder(os.Stdout).Encode(result)

	case fmtQuiet:
		for _, m := range result.Mails {
			fmt.Println(m.ID)
		}

	default: // fmtHuman
		if len(result.Mails) == 0 {
			fmt.Println("No messages found.")
			return nil
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tSUBJECT\tFROM\tRECEIVED AT")
		fmt.Fprintln(tw, strings.Repeat("─", 24)+"\t"+
			strings.Repeat("─", 30)+"\t"+
			strings.Repeat("─", 24)+"\t"+
			strings.Repeat("─", 16))
		for _, m := range result.Mails {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
				m.ID,
				truncate(m.Subject, 30),
				truncate(m.From, 24),
				m.ReceivedAt.Local().Format("2006-01-02 15:04"),
			)
		}
		tw.Flush()
		fmt.Printf("\nShowing %d–%d of %d\n",
			result.Offset+1,
			result.Offset+len(result.Mails),
			result.Total,
		)
	}
	return nil
}

// ── inbox get ─────────────────────────────────────────────────────────────────

var inboxGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a message by ID",
	Long: `Retrieve the full content of a single message by its storage ID.

In human mode the headers and text body are printed to stdout. In JSON mode the
complete message object is written to stdout (text_body and html_body included).

Examples:
  # Human output
  submail inbox get abc12345-6789-...

  # JSON output
  submail inbox get --json abc12345-6789-...

  # Quiet: just confirm the ID exists (exit 0) or exit 3 if not found
  submail inbox get -q abc12345-6789-...

  # Pipe: get the first message
  submail inbox list -q --limit 1 | xargs submail inbox get`,
	Args: cobra.ExactArgs(1),
	RunE: runInboxGet,
}

func runInboxGet(_ *cobra.Command, args []string) error {
	id := args[0]

	// Input validation — reject anything that is not a lowercase UUID.
	if !uuidRE.MatchString(id) {
		msg := fmt.Sprintf("invalid ID %q: must be a UUID (xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx)", id)
		fmt_ := resolveFormat(jsonFlag, quietFlag)
		if fmt_ == fmtJSON {
			return printJSONError(exitUsage, "invalid_input", msg,
				map[string]any{"id": id}, false)
		}
		fmt.Fprintln(os.Stderr, "Error:", msg)
		return &cliErr{exitUsage}
	}

	fmt_ := resolveFormat(jsonFlag, quietFlag)
	client := NewClient(clientURL, clientToken)

	mail, err := client.GetMail(cmd_ctx(), id)
	if err != nil {
		return handleAPIError(err, fmt_, map[string]any{"id": id})
	}

	switch fmt_ {
	case fmtJSON:
		return json.NewEncoder(os.Stdout).Encode(mail)

	case fmtQuiet:
		fmt.Println(mail.ID)

	default: // fmtHuman
		sep := strings.Repeat("─", 60)
		fmt.Printf("ID:         %s\n", mail.ID)
		fmt.Printf("Message-ID: %s\n", mail.MessageID)
		fmt.Printf("Subject:    %s\n", mail.Subject)
		fmt.Printf("From:       %s\n", mail.From)
		fmt.Printf("To:         %s\n", mail.To)
		fmt.Printf("Received:   %s\n", mail.ReceivedAt.UTC().Format("2006-01-02 15:04:05 UTC"))
		fmt.Println(sep)
		body := mail.TextBody
		if body == "" && mail.HTMLBody != "" {
			body = "[HTML body only — use --json for raw content]"
		}
		if body == "" {
			body = "(no body)"
		}
		fmt.Println(body)
	}
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// cmd_ctx returns a context for CLI commands. Signals are handled by the OS/shell.
func cmd_ctx() context.Context {
	return context.Background()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
