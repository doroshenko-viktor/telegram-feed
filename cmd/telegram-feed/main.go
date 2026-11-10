package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"

	"telegram_feed_summarizer/internal/config"
	"telegram_feed_summarizer/internal/llm"
	"telegram_feed_summarizer/internal/llm/gemini"
	"telegram_feed_summarizer/internal/mcp"
	"telegram_feed_summarizer/internal/service"
	"telegram_feed_summarizer/internal/storage"
	"telegram_feed_summarizer/internal/telegram"
	"telegram_feed_summarizer/internal/tui"
)

func main() {
	configPath := flag.String("config", "", "Path to custom config.yaml file")
	flag.Parse()

	args := flag.Args()
	subcommand := "tui"
	if len(args) > 0 {
		subcommand = args[0]
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	// Bootstrap Database
	repo, err := storage.NewRepository(ctx, cfg.Storage)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing database (%s): %v\n", cfg.Storage.Driver, err)
		os.Exit(1)
	}
	defer repo.Close()

	// Merge with DB settings (fallback)
	cfg.MergeWithDB(ctx, repo)

	// Check if setup is needed
	hasSession := false
	if sess, err := repo.LoadSession(ctx, "default"); err == nil && len(sess) > 0 {
		hasSession = true
	}
	needsSetup := (cfg.Telegram.AppID == 0 || cfg.Telegram.AppHash == "" || !hasSession)

	// Interactive setup for TUI or login when credentials/session are missing
	if needsSetup && (subcommand == "tui" || subcommand == "login") {
		if err := telegram.RunInteractiveSetup(ctx, repo, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Setup failed: %v\n", err)
			os.Exit(1)
		}
	}

	// Initialize MTProto Client
	tgClient, err := telegram.NewClient(cfg.Telegram, repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing Telegram client: %v\n", err)
		os.Exit(1)
	}

	// Initialize LLM Provider (Gemini or No-Op fallback)
	var llmProv llm.Provider
	if cfg.HasLocalAI() {
		prov, err := gemini.NewGeminiProvider(ctx, cfg.Gemini)
		if err != nil {
			fmt.Printf("⚠️ Warning: could not initialize Gemini provider: %v (falling back to No-Op)\n", err)
			llmProv = llm.NewNoOpProvider()
		} else {
			llmProv = prov
		}
	} else {
		llmProv = llm.NewNoOpProvider()
	}

	feedSvc := service.NewFeedService(cfg, repo, tgClient, llmProv)

	// If this was a fresh setup, auto-sync channels once so TUI isn't empty
	if needsSetup && tgClient.IsConfigured() {
		fmt.Println("🔄 Discovered initial subscribed channels...")
		_, _ = feedSvc.SyncSubscribedChannels(ctx)
	}

	switch subcommand {
	case "tui":
		if err := tui.Run(repo); err != nil {
			fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
			os.Exit(1)
		}

	case "login":
		fmt.Println("🔐 Telegram MTProto Login Wizard")
		if !tgClient.IsConfigured() {
			if err := telegram.RunInteractiveSetup(ctx, repo, cfg); err != nil {
				fmt.Fprintf(os.Stderr, "Setup failed: %v\n", err)
				os.Exit(1)
			}
		} else {
			if err := tgClient.LoginInteractive(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "Login error: %v\n", err)
				os.Exit(1)
			}
		}

	case "sync":
		fmt.Println("🔄 Discovering channels and syncing new messages...")
		count, err := feedSvc.SyncSubscribedChannels(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Sync error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ Discovered %d subscribed channels\n", count)

		postsCount, err := feedSvc.SyncTrackedPosts(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Post sync error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ Synced %d new posts from tracked channels\n", postsCount)

	case "mcp":
		server := mcp.NewServer(repo)
		if err := server.RunStdio(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
			os.Exit(1)
		}

	case "digest":
		digestFlags := flag.NewFlagSet("digest", flag.ExitOnError)
		topic := digestFlags.String("topic", "all", "Topic to summarize (e.g. 'go', 'kubernetes', 'all')")
		_ = digestFlags.Parse(args[1:])

		fmt.Printf("✨ Generating digest for topic [%s]...\n", *topic)
		digest, err := feedSvc.GenerateDigest(ctx, *topic)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Digest generation error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("\n" + digest.SummaryMarkdown)

	case "daemon":
		fmt.Printf("⏰ Starting background daemon (Cron: %s)...\n", cfg.Daemon.CronSchedule)
		c := cron.New()
		_, err := c.AddFunc(cfg.Daemon.CronSchedule, func() {
			fmt.Printf("[%s] Running scheduled digest pipeline...\n", time.Now().Format(time.RFC3339))
			runCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			_, _ = feedSvc.SyncTrackedPosts(runCtx)
			if cfg.HasLocalAI() {
				_, err := feedSvc.GenerateDigest(runCtx, "all")
				if err != nil {
					fmt.Printf("[%s] Digest generation error: %v\n", time.Now().Format(time.RFC3339), err)
				}
			}
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cron scheduling error: %v\n", err)
			os.Exit(1)
		}
		c.Start()
		defer c.Stop()

		<-ctx.Done()
		fmt.Println("\nDaemon stopped gracefully.")

	case "config":
		printConfigInfo(cfg)

	case "help", "-h", "--help":
		printHelp()

	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand: %s\n\n", subcommand)
		printHelp()
		os.Exit(1)
	}
}

func printConfigInfo(cfg *config.Config) {
	fmt.Println("📡 Telegram Feed Summarizer Configuration")
	fmt.Println("──────────────────────────────────────────")
	fmt.Printf("Storage Driver:    %s\n", cfg.Storage.Driver)
	fmt.Printf("Database DSN:      %s\n", cfg.Storage.DSN)
	fmt.Printf("Telegram App ID:   %d\n", cfg.Telegram.AppID)
	hasAppHash := "Not set"
	if cfg.Telegram.AppHash != "" {
		hasAppHash = "Configured"
	}
	fmt.Printf("Telegram App Hash: %s\n", hasAppHash)
	fmt.Printf("Local AI Status:   ")
	if cfg.HasLocalAI() {
		fmt.Printf("Enabled (Model: %s)\n", cfg.Gemini.Model)
	} else {
		fmt.Println("Disabled (Default pull-based MCP mode active)")
	}
	fmt.Printf("Bot Delivery:      ")
	if cfg.Telegram.BotToken != "" && cfg.Telegram.BotChatID != "" {
		fmt.Printf("Configured (Chat ID: %s)\n", cfg.Telegram.BotChatID)
	} else {
		fmt.Println("Disabled by default")
	}
	fmt.Printf("Ad Filtering:      %v\n", cfg.Pipeline.FilterAds)
	fmt.Printf("Daemon Cron:       %s\n", cfg.Daemon.CronSchedule)
	fmt.Printf("Output Directory:  %s\n", cfg.Daemon.OutputDir)
}

func printHelp() {
	fmt.Println("Usage: telegram-feed [subcommand] [flags]")
	fmt.Println()
	fmt.Println("Subcommands:")
	fmt.Println("  (no args)  Launch the interactive Terminal User Interface (TUI)")
	fmt.Println("  tui        Launch the interactive Terminal User Interface")
	fmt.Println("  login      Interactive Telegram MTProto terminal login wizard")
	fmt.Println("  mcp        Start the Model Context Protocol (MCP) server over stdio")
	fmt.Println("  sync       One-off sync of all subscribed & tracked channels")
	fmt.Println("  digest     Generate an AI topic digest (e.g. 'telegram-feed digest --topic go')")
	fmt.Println("  daemon     Run scheduled background cron worker")
	fmt.Println("  config     Inspect active configuration and capabilities")
	fmt.Println("  help       Show this help screen")
}
