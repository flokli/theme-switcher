package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alecthomas/kong"
	log "github.com/sirupsen/logrus"
)

// watchColorScheme invokes `gsettings monitor org.gnome.desktop.interface color-scheme`,
// and writes the selected scheme to the channel it returns.
func watchColorScheme(ctx context.Context) (chan string, error) {
	v := make(chan string)

	cmd := exec.CommandContext(ctx, "gsettings", "monitor", "org.gnome.desktop.interface", "color-scheme")

	// set up a pipe to receive stdout output.
	pR, pW := io.Pipe()
	cmd.Stdout = pW

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("unable to start gsettings watch command: %w", err)
	}

	go func() {
		// spin off a routine reading from the process
		scanner := bufio.NewScanner(pR)
		for scanner.Scan() {
			text := scanner.Text()
			if text == "color-scheme: 'prefer-dark'" {
				v <- "prefer-dark"
			} else if text == "color-scheme: 'default'" {
				v <- "default"
			} else {
				log.Warnf("got output: %s", scanner.Text())
			}
		}

		if err := scanner.Err(); err != nil {
			log.Fatal(err)
		}
	}()

	// kick off the cmd.Wait(). exit nonzero if we get an error.
	go func() {
		err := cmd.Wait()
		if err != nil {
			os.Exit(1)
		}
	}()

	return v, nil
}

// setKittyTheme invokes kitty to set the color scheme passed in the CLI.
func setKittyTheme(ctx context.Context, theme string) error {
	cmd := exec.CommandContext(ctx, "kitty", "+kitten", "themes", "--reload-in=all", theme)
	return cmd.Run()
}

// setHelixTheme edits the helix config file and sends a -USR1 to all helix instances to reload.
func setHelixTheme(ctx context.Context, theme string) error {
	confDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("unable to determine user config dir: %w", err)
	}

	configPath := filepath.Join(confDir, "helix", "config.toml")

	// open helix config
	f, err := os.Open(configPath)
	if err != nil {
		return fmt.Errorf("unable to open config file %s: %w", configPath, err)
	}

	var themeRegex = regexp.MustCompile(`^theme\s*=\s*"\w+"\s*$`)
	configNew := make([]string, 0)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if themeRegex.Match([]byte(line)) {
			configNew = append(configNew, "theme = \""+theme+"\"")
		} else {
			configNew = append(configNew, line)
		}
	}

	configStr := strings.Join(configNew, "\n")

	if err := os.WriteFile(configPath, []byte(configStr), os.ModePerm); err != nil {
		return fmt.Errorf("unable to write back config file: %w", err)
	}

	// send sigusr1 to all helixes, so they pick up changes
	cmd := exec.CommandContext(ctx, "pkill", "-USR1", "hx")
	return cmd.Run()
}

var cli struct {
	LogLevel    string   `enum:"trace,debug,info,warn,error,fatal,panic" help:"The log level to log with" default:"info"`
	KittyThemes []string `help:"Kitty theme to use in light and dark mode" default:"Catppuccin-Latte,Catppuccin-Mocha"`
	HelixThemes []string `help:"Helix themes to use in light and dark mode" default:"catppuccin_latte,catppuccin_macchiato"`
}

func main() {
	_ = kong.Parse(&cli)

	logLevel, err := log.ParseLevel(cli.LogLevel)
	if err != nil {
		log.Fatal("invalid log level")
	}
	log.SetLevel(logLevel)

	// ensure there's 2 kitty themes set
	if len(cli.KittyThemes) != 2 {
		log.Fatal("need exactly 2 kitty themes to be set")
	}

	// ensure there's 2 helix themes set
	if len(cli.HelixThemes) != 2 {
		log.Fatal("need exactly 2 helix themes to be set")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	chColorScheme, err := watchColorScheme(ctx)
	if err != nil {
		log.WithError(err).Fatal("Unable to watch color scheme")
	}

	for {
		select {
		case colorScheme := <-chColorScheme:
			log.Infof("new color scheme: %s", colorScheme)

			kittyTheme := cli.KittyThemes[0]
			helixTheme := cli.HelixThemes[0]
			if colorScheme == "prefer-dark" {
				kittyTheme = cli.KittyThemes[1]
				helixTheme = cli.HelixThemes[1]
			}

			log.WithField("theme", kittyTheme).Debug("setting kitty theme")
			if err := setKittyTheme(ctx, kittyTheme); err != nil {
				log.WithError(err).Warn("unable to set kitty theme")
			}

			log.WithField("theme", helixTheme).Debug("setting helix theme")
			if err := setHelixTheme(ctx, helixTheme); err != nil {
				log.WithError(err).Warn("unable to set helix theme")
			}
		case <-ctx.Done():
			log.Info("received interrput, stopping")
			return
		}
	}
}
