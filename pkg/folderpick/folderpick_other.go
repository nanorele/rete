//go:build !windows

package folderpick

func PickFolderDialog(string) (string, bool) { return "", false }
