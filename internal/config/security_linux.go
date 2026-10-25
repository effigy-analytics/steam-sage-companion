package config

import "os"

func ensurePrivateDir(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.Chmod(dir, 0700)
}
func validatePrivateLoad(dir, path string) error { return nil }
func protectPrivateFile(file *os.File) error     { return file.Chmod(0600) }
func replacePrivateFile(temp, dst string) error  { return os.Rename(temp, dst) }
