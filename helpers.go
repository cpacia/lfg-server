package main

import (
	"crypto/sha1"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func ensureDir(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

func allowedImageExt(ct string) (string, bool) {
	// Only allow common web formats; map content-type to extension
	switch strings.ToLower(ct) {
	case "image/jpeg", "image/jpg":
		return ".jpg", true
	case "image/png":
		return ".png", true
	case "image/webp":
		return ".webp", true
	case "image/gif":
		return ".gif", true
	default:
		return "", false
	}
}

func saveChampionImage(file multipart.File, hdr *multipart.FileHeader, year string, championsUploadDir string) (string, error) {
	// Try content-type, fallback to filename extension
	ct := hdr.Header.Get("Content-Type")
	ext, ok := allowedImageExt(ct)
	if !ok {
		ext = strings.ToLower(filepath.Ext(hdr.Filename))
		switch ext {
		case ".jpg", ".jpeg":
			ext = ".jpg"
			ok = true
		case ".png", ".webp", ".gif":
			ok = true
		}
	}
	if !ok {
		return "", fmt.Errorf("Unsupported image type")
	}

	if err := ensureDir(championsUploadDir); err != nil {
		return "", fmt.Errorf("Cannot create upload dir")
	}

	// Stableish filename: year + short hash + ext to bust caches on updates
	h := sha1.New()
	_, _ = io.WriteString(h, year)
	_, _ = io.WriteString(h, time.Now().UTC().Format(time.RFC3339Nano))
	hash := fmt.Sprintf("%x", h.Sum(nil))[:10]

	filename := fmt.Sprintf("%s-%s%s", year, hash, ext)
	outPath := filepath.Join(championsUploadDir, filename)

	out, err := os.Create(outPath)
	if err != nil {
		return "", fmt.Errorf("Failed to save image")
	}
	defer out.Close()

	if _, err := io.Copy(out, file); err != nil {
		_ = os.Remove(outPath)
		return "", fmt.Errorf("Failed to write image")
	}

	return filename, nil
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "unique violation") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "unique index")
}
