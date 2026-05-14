package exec

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func embedWalkDir(fe embed.FS, root, dir string) (int, error) {
	fileTotal := 0

	return fileTotal, fs.WalkDir(fe, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relativePath := strings.TrimPrefix(path, root)
		relativePath = strings.TrimLeft(relativePath, `/\`)
		fp := filepath.Join(dir, relativePath)
		if d.IsDir() {
			if err := os.MkdirAll(fp, 0o755); err != nil && !os.IsExist(err) {
				return err
			}
			return nil
		}

		data, err := fe.ReadFile(path)
		if err != nil {
			return err
		}

		if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
			return err
		}

		if err := os.WriteFile(fp, data, 0o755); err != nil {
			return err
		}

		fileTotal++
		_ = os.Chmod(fp, 0o777)

		return nil
	})
}

func embedToDisk(fe embed.FS, tmpDir string) (int, error) {
	files, err := fe.ReadDir(".")
	if err != nil {
		return 0, err
	}

	if len(files) == 1 {
		if files[0].IsDir() {
			return embedWalkDir(fe, files[0].Name(), tmpDir)
		}
	}

	return embedWalkDir(fe, ".", tmpDir)
}
