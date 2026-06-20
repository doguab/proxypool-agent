package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/doguab/proxypool-agent/internal/config"
	"github.com/doguab/proxypool-agent/internal/tunnel"
)

func main() {
	configPath := flag.String("config", config.DefaultPath, "path to agent config file")
	showSecret := flag.Bool("show-secret", false, "print secret and exit")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		if os.IsNotExist(err) {
			cfg = config.Default()
		} else {
			log.Fatalf("load config: %v", err)
		}
	}

	generated, err := config.EnsureSecret(&cfg)
	if err != nil {
		log.Fatalf("generate secret: %v", err)
	}
	if generated {
		if err := config.Save(*configPath, cfg); err != nil {
			log.Fatalf("save config: %v", err)
		}
		fmt.Println("==================================================")
		fmt.Println("PROXYPOOL AGENT — SAVE THIS SECRET")
		fmt.Println("Add this secret in your hub panel to register")
		fmt.Println("this server:")
		fmt.Println()
		fmt.Printf("  %s\n", cfg.Secret)
		fmt.Println()
		fmt.Println("==================================================")
	}

	if *showSecret {
		fmt.Println(cfg.Secret)
		return
	}

	if err := config.Validate(cfg); err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client := tunnel.NewClient(cfg)
	log.Printf("connecting to hub %s", cfg.HubURL)
	if err := client.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("agent stopped: %v", err)
	}
}
