package telegram

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"golang.org/x/term"
)

// TerminalAuth implements gotd/td's auth.UserAuthenticator for interactive CLI login.
type TerminalAuth struct {
	reader *bufio.Reader
}

// NewTerminalAuth creates an interactive terminal authenticator.
func NewTerminalAuth() *TerminalAuth {
	return &TerminalAuth{
		reader: bufio.NewReader(os.Stdin),
	}
}

func (a *TerminalAuth) Phone(ctx context.Context) (string, error) {
	for {
		fmt.Print("Enter your phone number (with country code, e.g. +1234567890): ")
		line, err := a.reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		cleaned := cleanPhoneNumber(line)
		if len(cleaned) < 6 {
			fmt.Println("❌ Phone number cannot be empty or too short. Please include your international country code (e.g. +1234567890).")
			continue
		}
		return cleaned, nil
	}
}

func cleanPhoneNumber(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	for i, r := range s {
		if r == '+' && i == 0 {
			b.WriteRune(r)
		} else if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	res := b.String()
	if len(res) > 0 && res[0] != '+' {
		res = "+" + res
	}
	return res
}

func (a *TerminalAuth) Password(ctx context.Context) (string, error) {
	for {
		fmt.Print("Enter your Telegram 2FA cloud password: ")
		bytePassword, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", err
		}
		pass := strings.TrimSpace(string(bytePassword))
		if pass == "" {
			fmt.Println("❌ 2FA password cannot be empty. Please try again.")
			continue
		}
		return pass, nil
	}
}

func (a *TerminalAuth) AcceptTermsOfService(ctx context.Context, tos tg.HelpTermsOfService) error {
	return nil
}

func (a *TerminalAuth) Code(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	for {
		fmt.Print("Enter the code sent to your Telegram app or SMS: ")
		line, err := a.reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		code := strings.TrimSpace(line)
		code = strings.ReplaceAll(code, " ", "")
		code = strings.ReplaceAll(code, "-", "")
		if code == "" {
			fmt.Println("❌ Verification code cannot be empty.")
			continue
		}
		return code, nil
	}
}

func (a *TerminalAuth) SignUp(ctx context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, fmt.Errorf("sign-up not supported: please register your account via the official Telegram app first")
}

// LoginInteractive performs the interactive login flow and persists the session to the database.
func (c *Client) LoginInteractive(ctx context.Context) error {
	if !c.IsConfigured() {
		return fmt.Errorf("cannot log in: TELEGRAM_APP_ID and TELEGRAM_APP_HASH must be configured")
	}

	for attempt := 1; attempt <= 5; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		flow := auth.NewFlow(
			NewTerminalAuth(),
			auth.SendCodeOptions{},
		)

		err := c.client.Run(ctx, func(ctx context.Context) error {
			status, err := c.client.Auth().Status(ctx)
			if err != nil {
				return fmt.Errorf("checking auth status: %w", err)
			}

			if status.Authorized {
				user, _ := c.client.Self(ctx)
				username := ""
				if user != nil {
					username = user.Username
				}
				fmt.Printf("✅ Already authenticated as @%s (ID: %d)\n", username, status.User.ID)
				return nil
			}

			if err := c.client.Auth().IfNecessary(ctx, flow); err != nil {
				return err
			}

			fmt.Println("🎉 Successfully authenticated! Session stored securely in your database.")
			return nil
		})

		if err == nil {
			return nil
		}

		errMsg := err.Error()
		if strings.Contains(errMsg, "PHONE_NUMBER_INVALID") {
			fmt.Println("\n❌ Telegram rejected the phone number as invalid (PHONE_NUMBER_INVALID).")
			fmt.Println("Please verify your country code and number format (e.g. +1234567890). Retrying...")
			fmt.Println()
			continue
		} else if strings.Contains(errMsg, "PHONE_CODE_INVALID") {
			fmt.Println("\n❌ The verification code entered was incorrect. Retrying...")
			fmt.Println()
			continue
		} else if strings.Contains(errMsg, "PHONE_CODE_EXPIRED") {
			fmt.Println("\n❌ The verification code has expired. Requesting a new one...")
			fmt.Println()
			continue
		} else if strings.Contains(errMsg, "PASSWORD_HASH_INVALID") {
			fmt.Println("\n❌ The 2FA cloud password was incorrect. Retrying...")
			fmt.Println()
			continue
		}

		return fmt.Errorf("authentication failed: %w", err)
	}

	return fmt.Errorf("maximum login attempts exceeded")
}
