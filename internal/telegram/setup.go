package telegram

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"telegram_feed_summarizer/internal/config"
	"telegram_feed_summarizer/internal/domain"
	"telegram_feed_summarizer/internal/llm/gemini"
)

// RunInteractiveSetup conducts an interactive terminal wizard to configure missing
// credentials (Telegram App ID/Hash, optional Gemini API key & model, and MTProto login).
func RunInteractiveSetup(ctx context.Context, repo domain.Repository, cfg *config.Config) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("\n" + strings.Repeat("=", 68))
	fmt.Println(" 📡 Welcome to telegram-feed Setup Guide")
	fmt.Println(strings.Repeat("=", 68))
	fmt.Println("Let's configure your feed aggregator in 2-3 quick steps.")
	fmt.Println("All credentials and sessions are securely stored directly in your")
	fmt.Printf("local database: %s\n\n", cfg.Storage.DSN)

	// Step 1: Telegram App ID & Hash
	if cfg.Telegram.AppID == 0 || strings.TrimSpace(cfg.Telegram.AppHash) == "" {
		fmt.Println(strings.Repeat("-", 68))
		fmt.Println(" 🔑 Step 1: Telegram App ID & App Hash")
		fmt.Println(strings.Repeat("-", 68))
		fmt.Println("Why is this required?")
		fmt.Println("• telegram-feed connects directly to your Telegram account via MTProto")
		fmt.Println("  (the official Telegram protocol) to read your subscribed channels,")
		fmt.Println("  unread messages, and topic feeds.")
		fmt.Println("• Telegram requires every client to identify itself with an App ID and")
		fmt.Println("  Hash to protect your personal account against rate limits and bans.")
		fmt.Println("Instructions to obtain them:")
		fmt.Println("  1. Open https://my.telegram.org in your browser and log in.")
		fmt.Println("  2. Click 'API development tools'.")
		fmt.Println("  3. Enter an application title (e.g. 'TelegramFeed') and short name.")
		fmt.Println("  4. Copy the generated 'App api_id' and 'App api_hash'.")
		fmt.Println()

		for {
			fmt.Print("Enter Telegram App ID: ")
			idStr, _ := reader.ReadString('\n')
			idStr = strings.TrimSpace(idStr)
			id, err := strconv.Atoi(idStr)
			if err != nil || id <= 0 {
				fmt.Println("❌ Please enter a valid numeric App ID (e.g. 12345678).")
				continue
			}
			cfg.Telegram.AppID = id
			break
		}

		for {
			fmt.Print("Enter Telegram App Hash: ")
			hash, _ := reader.ReadString('\n')
			hash = strings.TrimSpace(hash)
			if len(hash) < 10 {
				fmt.Println("❌ Please enter a valid 32-character App Hash.")
				continue
			}
			cfg.Telegram.AppHash = hash
			break
		}

		// Persist to DB
		_ = repo.SetSetting(ctx, domain.SettingKeyTelegramAppID, strconv.Itoa(cfg.Telegram.AppID))
		_ = repo.SetSetting(ctx, domain.SettingKeyTelegramAppHash, cfg.Telegram.AppHash)
		fmt.Println("✓ Telegram credentials saved to database.")
	} else {
		fmt.Println("✓ Telegram App ID & Hash: already configured.")
	}

	// Step 2: Google Gemini AI (Optional)
	geminiKey, _ := repo.GetSetting(ctx, domain.SettingKeyGeminiAPIKey, cfg.Gemini.APIKey)
	geminiKeyLower := strings.ToLower(strings.TrimSpace(geminiKey))
	if geminiKeyLower == "no" || geminiKeyLower == "none" || geminiKeyLower == "false" || geminiKeyLower == "skip" {
		geminiKey = ""
		_ = repo.SetSetting(ctx, domain.SettingKeyGeminiAPIKey, "")
		cfg.Gemini.APIKey = ""
	}

	if strings.TrimSpace(geminiKey) == "" {
		fmt.Println("\n" + strings.Repeat("-", 68))
		fmt.Println(" 💡 Step 2: Google Gemini AI Key (Optional)")
		fmt.Println(strings.Repeat("-", 68))
		fmt.Println("Will you use telegram-feed with Claude Desktop, Cursor, or Antigravity via MCP?")
		fmt.Println("• If YES: You DO NOT need any Gemini API key!")
		fmt.Println("  Your chat LLM handles all summarization and queries directly over MCP.")
		fmt.Println("• If NO (or if you want local CLI digests 'telegram-feed digest' or")
		fmt.Println("  background daemon summaries):")
		fmt.Println("  Enter a Google AI Studio API key (https://aistudio.google.com).")
		fmt.Println()

		for {
			fmt.Print("Enter Gemini API Key (press [Enter] or type 'no' to skip): ")
			keyInput, _ := reader.ReadString('\n')
			keyInput = strings.TrimSpace(keyInput)
			keyLower := strings.ToLower(keyInput)

			if keyInput == "" || keyLower == "no" || keyLower == "n" || keyLower == "skip" || keyLower == "none" || keyLower == "false" {
				fmt.Println("✓ Pure MCP mode selected (no Gemini API key stored).")
				cfg.Gemini.APIKey = ""
				_ = repo.SetSetting(ctx, domain.SettingKeyGeminiAPIKey, "")
				break
			}

			// Validate the provided API key with live verification
			fmt.Print("Validating Gemini API key with Google AI...")
			if err := gemini.ValidateAPIKey(ctx, keyInput); err != nil {
				fmt.Printf("\n❌ Invalid Gemini API key: %v\n", err)
				fmt.Println("Please verify your API key at https://aistudio.google.com or type 'no' to skip.")
				fmt.Println()
				continue
			}

			fmt.Println(" Verified successfully!")
			cfg.Gemini.APIKey = keyInput
			_ = repo.SetSetting(ctx, domain.SettingKeyGeminiAPIKey, keyInput)

			fmt.Println("\nAvailable Gemini models:")
			fmt.Println("  [1] gemini-3.8-flash (Recommended: fast, general purpose)")
			fmt.Println("  [2] gemini-3.8-pro   (Deep reasoning & complex synthesis)")
			fmt.Println("  [3] gemini-3.0-flash (High speed flash model)")
			fmt.Print("Select model [1-3, default 1]: ")

			modelChoice, _ := reader.ReadString('\n')
			modelChoice = strings.TrimSpace(modelChoice)

			selectedModel := "gemini-3.8-flash"
			switch modelChoice {
			case "2":
				selectedModel = "gemini-3.8-pro"
			case "3":
				selectedModel = "gemini-3.0-flash"
			}

			cfg.Gemini.Model = selectedModel
			_ = repo.SetSetting(ctx, domain.SettingKeyGeminiModel, selectedModel)
			fmt.Printf("✓ Gemini configured with model: %s\n", selectedModel)
			break
		}
	} else {
		fmt.Printf("✓ Gemini AI: already configured (Model: %s).\n", cfg.Gemini.Model)
	}

	// Step 3: MTProto Login
	hasSession := false
	if data, err := repo.LoadSession(ctx, "default"); err == nil && len(data) > 0 {
		hasSession = true
	}

	if !hasSession {
		fmt.Println("\n" + strings.Repeat("-", 68))
		fmt.Println(" 🔐 Step 3: Telegram Account Authentication")
		fmt.Println(strings.Repeat("-", 68))
		fmt.Println("We will now connect to Telegram and authenticate your account.")
		fmt.Println("The session will be saved encrypted in your local database.")
		fmt.Println("You only need to do this ONCE.")
		fmt.Println()

		tgClient, err := NewClient(cfg.Telegram, repo)
		if err != nil {
			return fmt.Errorf("creating Telegram client: %w", err)
		}

		if err := tgClient.LoginInteractive(ctx); err != nil {
			return fmt.Errorf("telegram login: %w", err)
		}
	} else {
		fmt.Println("✓ Telegram Session: already authenticated in database.")
	}

	fmt.Println("\n" + strings.Repeat("=", 68))
	fmt.Println(" 🎉 Setup Complete! You are all set.")
	fmt.Println(strings.Repeat("=", 68) + "\n")

	return nil
}
