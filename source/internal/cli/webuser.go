// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/jim-collier/nano-git-db/internal/core/config"
)

// webUser sets (or replaces) a proxied-mode web login. The password comes from
// NGDB_WEB_PASSWORD when set - so it can be scripted - else it is read from
// stdin. The creds file lives in the user config dir, outside any synced tree.
func webUser(username string) error {
	password := os.Getenv("NGDB_WEB_PASSWORD")
	if password == "" {
		var err error
		if password, err = readPassword(); err != nil {
			return err
		}
	}
	if password == "" {
		return fmt.Errorf("webuser: empty password")
	}
	creds := config.LoadWebCreds()
	if err := creds.Set(username, password); err != nil {
		return err
	}
	if err := creds.Save(); err != nil {
		return err
	}
	fmt.Printf("web login set for %q; the web UI requires web_mode = proxied\n", username)
	return nil
}

// readPassword prompts on stderr and reads without echoing, so the password
// stays out of the terminal and its scrollback. A piped stdin is not a terminal,
// so it is read as a plain line - that is how a setup script feeds one in.
func readPassword() (string, error) {
	fmt.Fprint(os.Stderr, "password: ")
	defer fmt.Fprintln(os.Stderr)
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		if err != nil {
			return "", fmt.Errorf("reading password: %w", err)
		}
		return string(b), nil
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("reading password: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}
