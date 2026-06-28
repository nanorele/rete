//go:build !windows

package ui

func pickFolderDialog(string) (string, bool) { return "", false }
