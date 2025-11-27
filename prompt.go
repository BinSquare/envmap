package main

import (
	"fmt"
	"golang.org/x/term"
	"os"
)

func readSecretFromPrompt(label string) (string, error) {
	fmt.Fprint(os.Stderr, label)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read secret: %w", err)
	}
	return string(b), nil
}
