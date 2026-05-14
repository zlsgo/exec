package exec

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sohaha/zlsgo/zfile"
	"github.com/sohaha/zlsgo/zhttp"
	"github.com/sohaha/zlsgo/zstring"
)

func (e *Engine) downloadToDisk() error {
	if e.options.DownloadUrl == "" {
		return errors.New("download url is empty")
	}

	downloadDir := zfile.RealPathMkdir(filepath.Join(zfile.TmpPath(), zstring.Md5(e.options.DownloadUrl)))
	downloadFile := filepath.Join(downloadDir, filepath.Base(e.options.DownloadUrl))

	if zfile.FileExist(downloadFile) {
		if err := handleDownloadFile(e.options, downloadFile, e.tmpDir); err == nil {
			return nil
		}
	}

	if err := e.download(downloadFile); err != nil {
		return err
	}

	return handleDownloadFile(e.options, downloadFile, e.tmpDir)
}

func (e *Engine) download(downloadFile string) error {
	params := []any{e.ctx}
	if e.options.DownloadProgress != nil {
		params = append(params, zhttp.DownloadProgress(func(current, total int64) {
			if current == 0 {
				e.options.DownloadProgress(0, total)
				return
			}

			e.options.DownloadProgress((float64(current)/float64(total))*100, total)
		}))
	}
	resp, err := zhttp.Get(e.options.DownloadUrl, params...)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(downloadFile), 0o755); err != nil {
		return err
	}

	return resp.ToFile(downloadFile)
}

func handleDownloadFile(opt *Options, file string, dir string) error {
	dir = zfile.RealPathMkdir(dir)
	if opt.DownloadHandlerWithDir != nil {
		return opt.DownloadHandlerWithDir(file, dir)
	}
	if opt.DownloadHandler != nil {
		return opt.DownloadHandler(file)
	}

	switch archiveKind(file) {
	case "tar.gz":
		return ungz(opt, file, dir)
	case "zip":
		return unzip(opt, file, dir)
	case "gz":
		return gunzip(opt, file, dir)
	default:
		if zfile.DirExist(file) {
			return zfile.CopyDir(file, dir)
		}

		dest, err := resolveOutputPath(opt, file, dir)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := zfile.CopyFile(file, dest); err != nil {
			return err
		}

		_ = os.Chmod(dest, 0o777)
		return nil
	}
}

func archiveKind(file string) string {
	lower := strings.ToLower(file)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return "tar.gz"
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	case strings.HasSuffix(lower, ".gz"):
		return "gz"
	default:
		return ""
	}
}

func resolveOutputPath(opt *Options, file string, dir string) (string, error) {
	target := filepath.Base(file)
	if opt != nil && opt.BinFile != "" {
		target = opt.BinFile
	}

	return safeExtractPath(dir, target)
}

func unzip(opt *Options, file string, dir string) error {
	zipReader, err := zip.OpenReader(file)
	if err != nil {
		return err
	}
	defer zipReader.Close()

	dir = zfile.RealPathMkdir(dir)
	for _, f := range zipReader.File {
		fpath, err := safeExtractPath(dir, f.Name)
		if err != nil {
			return err
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fpath, f.Mode()); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), 0o755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		if _, err = io.Copy(outFile, rc); err != nil {
			rc.Close()
			outFile.Close()
			return err
		}
		if err := rc.Close(); err != nil {
			outFile.Close()
			return err
		}
		if err := outFile.Close(); err != nil {
			return err
		}
	}
	return nil
}

func ungz(opt *Options, file string, dir string) error {
	gzipFile, err := os.Open(file)
	if err != nil {
		return err
	}
	defer gzipFile.Close()

	gzipReader, err := gzip.NewReader(gzipFile)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		path, err := safeExtractPath(dir, header.Name)
		if err != nil {
			return err
		}
		info := header.FileInfo()

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, info.Mode()); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
			if err != nil {
				return err
			}

			if _, err = io.Copy(file, tarReader); err != nil {
				file.Close()
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported file type: %v", header.Typeflag)
		}
	}

	return nil
}

func gunzip(opt *Options, file string, dir string) error {
	gzipFile, err := os.Open(file)
	if err != nil {
		return err
	}
	defer gzipFile.Close()

	gzipReader, err := gzip.NewReader(gzipFile)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	dest, err := resolveOutputPath(opt, strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)), dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	outFile, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}

	if _, err = io.Copy(outFile, gzipReader); err != nil {
		outFile.Close()
		return err
	}

	if err := outFile.Close(); err != nil {
		return err
	}

	return nil
}

func safeExtractPath(dir, name string) (string, error) {
	cleanDir := filepath.Clean(dir)
	target := filepath.Join(cleanDir, filepath.Clean(name))
	relative, err := filepath.Rel(cleanDir, target)
	if err != nil {
		return "", err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid archive path: %s", name)
	}

	return target, nil
}
