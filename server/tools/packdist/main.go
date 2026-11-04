// Command packdist packages the komari-web dist directory as the embedded
// dist.tar.zst archive expected by web/public.
package main

import (
	"archive/tar"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

func main() {
	src := flag.String("src", "../komari-web/dist", "frontend dist directory")
	dst := flag.String("dst", "web/public/defaultTheme/dist.tar.zst", "output archive")
	flag.Parse()

	if err := pack(*src, *dst); err != nil {
		fmt.Fprintln(os.Stderr, "packdist:", err)
		os.Exit(1)
	}
}

func pack(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", src)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	enc, err := zstd.NewWriter(out, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	if err != nil {
		return err
	}
	tw := tar.NewWriter(enc)
	names := make([]string, 0, 256)
	err = filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		names = append(names, rel)
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = rel
		if info.IsDir() {
			header.Name += "/"
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
	if err != nil {
		return err
	}
	if !contains(names, "index.html") {
		return fmt.Errorf("dist archive does not contain index.html")
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return out.Close()
}

func contains(list []string, target string) bool {
	for _, item := range list {
		if strings.TrimSuffix(item, "/") == target {
			return true
		}
	}
	return false
}
