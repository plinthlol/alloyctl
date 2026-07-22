package main

import (
	"fmt"

	"alloy/internal/auth"
	"alloy/internal/cli"
	"alloy/internal/config"
)

func cmdAuth(paths config.Paths, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: alloyctl auth online | alloyctl auth offline <username>")
	}

	switch args[0] {
	case "offline":
		if len(args) < 2 {
			return fmt.Errorf("usage: alloyctl auth offline <username>")
		}
		return authOffline(paths, args[1])
	case "online":
		return authOnline(paths)
	default:
		return fmt.Errorf("unknown auth subcommand %q (expected 'online' or 'offline')", args[0])
	}
}

func authOffline(paths config.Paths, username string) error {
	profile := auth.NewOfflineProfile(username)

	g, err := config.Load(paths)
	if err != nil {
		return err
	}
	g.UpsertAccount(config.Account{
		Type:     "offline",
		Username: profile.Username,
		UUID:     profile.UUID,
	})
	g.ActiveAccount = profile.Username
	if err := config.Save(paths, g); err != nil {
		return err
	}

	cli.Info(fmt.Sprintf("Created offline account %q (uuid %s) and set it as active.", profile.Username, profile.UUID))
	return nil
}

func authOnline(paths config.Paths) error {
	dc, err := auth.StartDeviceCode()
	if err != nil {
		return fmt.Errorf("starting Microsoft device code flow: %w", err)
	}

	fmt.Println()
	fmt.Println("  \033[1mOpen:\033[0m", dc.VerificationURI)
	fmt.Println("  \033[1mCode:\033[0m", dc.UserCode)
	fmt.Println()
	cli.Info("Waiting for you to finish signing in...")

	tok, err := auth.PollDeviceCode(dc)
	if err != nil {
		return fmt.Errorf("microsoft sign-in failed: %w", err)
	}

	profile, err := auth.CompleteAuthChain(tok.AccessToken, tok.RefreshToken, tok.ExpiresIn)
	if err != nil {
		return fmt.Errorf("completing Xbox/Minecraft auth chain: %w", err)
	}

	g, err := config.Load(paths)
	if err != nil {
		return err
	}
	g.UpsertAccount(config.Account{
		Type:         "microsoft",
		Username:     profile.Username,
		UUID:         profile.UUID,
		RefreshToken: profile.RefreshToken,
	})
	g.ActiveAccount = profile.Username
	if err := config.Save(paths, g); err != nil {
		return err
	}

	cli.Info(fmt.Sprintf("Signed in as %q (uuid %s) and set it as active.", profile.Username, profile.UUID))
	return nil
}
