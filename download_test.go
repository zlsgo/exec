package exec

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"embed"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

//go:embed testdata/embedfs/root/**
var embedFixtureFS embed.FS

func TestHandleDownloadFileUsesCustomHandler(t *testing.T) {
	source := filepath.Join(t.TempDir(), "artifact.bin")
	dest := t.TempDir()
	if err := os.WriteFile(source, []byte("fixture"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var called string
	var calledDir string
	err := handleDownloadFile(&Options{
		DownloadHandlerWithDir: func(file string, dir string) error {
			called = file
			calledDir = dir
			return nil
		},
	}, source, dest)
	if err != nil {
		t.Fatalf("handleDownloadFile() error = %v", err)
	}

	if got, want := called, source; got != want {
		t.Fatalf("DownloadHandler file = %q, want %q", got, want)
	}
	if got, want := filepath.Clean(calledDir), filepath.Clean(dest); got != want {
		t.Fatalf("DownloadHandler dir = %q, want %q", got, want)
	}
}

func TestHandleDownloadFileHandlerCanInstallTargetBinary(t *testing.T) {
	source := filepath.Join(t.TempDir(), "artifact.bin")
	dest := t.TempDir()
	target := filepath.Join(dest, "tool")
	if err := os.WriteFile(source, []byte("fixture"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err := handleDownloadFile(&Options{
		DownloadHandlerWithDir: func(file string, dir string) error {
			data, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dir, "tool"), data, 0o755)
		},
	}, source, dest)
	if err != nil {
		t.Fatalf("handleDownloadFile() error = %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "fixture" {
		t.Fatalf("installed content = %q, want fixture", got)
	}
}

func TestHandleDownloadFileUsesLegacyCustomHandler(t *testing.T) {
	source := filepath.Join(t.TempDir(), "artifact.bin")
	if err := os.WriteFile(source, []byte("fixture"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var called string
	err := handleDownloadFile(&Options{
		DownloadHandler: func(file string) error {
			called = file
			return nil
		},
	}, source, t.TempDir())
	if err != nil {
		t.Fatalf("handleDownloadFile() error = %v", err)
	}

	if got, want := called, source; got != want {
		t.Fatalf("legacy DownloadHandler file = %q, want %q", got, want)
	}
}

func TestHandleDownloadFilePrefersHandlerWithDir(t *testing.T) {
	source := filepath.Join(t.TempDir(), "artifact.bin")
	if err := os.WriteFile(source, []byte("fixture"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	calledLegacy := false
	calledNew := false
	err := handleDownloadFile(&Options{
		DownloadHandler: func(file string) error {
			calledLegacy = true
			return nil
		},
		DownloadHandlerWithDir: func(file string, dir string) error {
			calledNew = true
			return nil
		},
	}, source, t.TempDir())
	if err != nil {
		t.Fatalf("handleDownloadFile() error = %v", err)
	}

	if !calledNew {
		t.Fatal("DownloadHandlerWithDir should be preferred")
	}
	if calledLegacy {
		t.Fatal("legacy DownloadHandler should not run when DownloadHandlerWithDir is set")
	}
}

func TestHandleDownloadFileRejectsZipSlip(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "payload.zip")
	writeZipArchive(t, archive, map[string]string{
		"../escape.txt": "blocked",
	})

	err := handleDownloadFile(&Options{}, archive, t.TempDir())
	if err == nil {
		t.Fatal("handleDownloadFile() expected zip slip error")
	}
	if !strings.Contains(err.Error(), "invalid archive path") {
		t.Fatalf("handleDownloadFile() error = %v", err)
	}
}

func TestHandleDownloadFileExtractsZipNestedFile(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "payload.zip")
	dest := t.TempDir()
	writeZipArchive(t, archive, map[string]string{
		"bin/tool.txt": "payload",
	})

	if err := handleDownloadFile(&Options{}, archive, dest); err != nil {
		t.Fatalf("handleDownloadFile() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dest, "bin", "tool.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "payload" {
		t.Fatalf("extracted zip content = %q, want payload", got)
	}
}

func TestHandleDownloadFileCopiesPlainFileToBinFilePath(t *testing.T) {
	source := filepath.Join(t.TempDir(), "artifact.bin")
	dest := t.TempDir()
	if err := os.WriteFile(source, []byte("fixture"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := handleDownloadFile(&Options{BinFile: "bin/tool"}, source, dest); err != nil {
		t.Fatalf("handleDownloadFile() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dest, "bin", "tool"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "fixture" {
		t.Fatalf("copied content = %q, want fixture", got)
	}
}

func TestHandleDownloadFileGunzipsToBinFilePath(t *testing.T) {
	source := filepath.Join(t.TempDir(), "artifact.gz")
	dest := t.TempDir()
	writeGzipFile(t, source, "payload")

	if err := handleDownloadFile(&Options{BinFile: "bin/tool"}, source, dest); err != nil {
		t.Fatalf("handleDownloadFile() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dest, "bin", "tool"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "payload" {
		t.Fatalf("gunzip content = %q, want payload", got)
	}
}

func TestHandleDownloadFileExtractsTarGz(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "payload.tgz")
	dest := t.TempDir()
	writeTarGzArchive(t, archive, map[string]string{
		"bin/tool.txt": "payload",
	})

	if err := handleDownloadFile(&Options{}, archive, dest); err != nil {
		t.Fatalf("handleDownloadFile() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dest, "bin", "tool.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "payload" {
		t.Fatalf("extracted content = %q, want payload", got)
	}
}

func TestEmbedToDiskCopiesEmbeddedFiles(t *testing.T) {
	dest := t.TempDir()

	total, err := embedToDisk(embedFixtureFS, dest)
	if err != nil {
		t.Fatalf("embedToDisk() error = %v", err)
	}
	if total != 1 {
		t.Fatalf("embedToDisk() total = %d, want 1", total)
	}

	got, err := os.ReadFile(filepath.Join(dest, "embedfs", "root", "bin", "tool.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.TrimSpace(string(got)) != "embedded-tool" {
		t.Fatalf("embedded file content = %q, want embedded-tool", got)
	}
}

func writeZipArchive(t *testing.T, path string, files map[string]string) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	defer file.Close()

	writer := zip.NewWriter(file)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("Create(%q) error = %v", name, err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("Write(%q) error = %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("zip Close() error = %v", err)
	}
}

func writeTarGzArchive(t *testing.T, path string, files map[string]string) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	for name, content := range files {
		header := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("WriteHeader(%q) error = %v", name, err)
		}
		if _, err := tarWriter.Write([]byte(content)); err != nil {
			t.Fatalf("Write(%q) error = %v", name, err)
		}
	}
}

func writeGzipFile(t *testing.T, path string, content string) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	if _, err := gzipWriter.Write([]byte(content)); err != nil {
		t.Fatalf("gzip Write() error = %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("gzip Close() error = %v", err)
	}
}
