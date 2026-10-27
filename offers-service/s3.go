package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// S3Store — выгрузка/скачивание файлов через rclone CLI. По образцу
// documents-service: aws-sdk-go-v2 на этом Ceph зависал и дестабилизировал
// сервер, rclone работает стабильно.
type S3Store struct {
	bucket     string
	configPath string
}

func NewS3Store() (*S3Store, error) {
	endpoint := os.Getenv("S3_ENDPOINT")
	if endpoint == "" {
		return nil, fmt.Errorf("S3_ENDPOINT is required")
	}
	accessKey := os.Getenv("S3_ACCESS_KEY")
	secretKey := os.Getenv("S3_SECRET_KEY")
	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("S3_ACCESS_KEY and S3_SECRET_KEY are required")
	}
	bucket := os.Getenv("S3_BUCKET")
	if bucket == "" {
		bucket = "sbe-offers"
	}

	configDir := "/root/.config/rclone"
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return nil, err
	}
	configPath := filepath.Join(configDir, "rclone.conf")
	conf := fmt.Sprintf(`[firstvds_offers]
type = s3
provider = Other
access_key_id = %s
secret_access_key = %s
endpoint = %s
`, accessKey, secretKey, endpoint)
	if err := os.WriteFile(configPath, []byte(conf), 0o600); err != nil {
		return nil, err
	}
	return &S3Store{bucket: bucket, configPath: configPath}, nil
}

func (s *S3Store) remote(key string) string {
	return fmt.Sprintf("firstvds_offers:%s/%s", s.bucket, key)
}

func (s *S3Store) rcloneArgs(args ...string) *exec.Cmd {
	cmd := exec.Command("rclone", args...)
	cmd.Env = append(os.Environ(), "RCLONE_CONFIG="+s.configPath)
	return cmd
}

// Put загружает содержимое в S3 по ключу.
func (s *S3Store) Put(ctx context.Context, key string, data []byte) error {
	tmp, err := os.CreateTemp("", "rclone-upload-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	start := time.Now()
	cmd := exec.CommandContext(ctx, "rclone", "copyto", "--log-level", "ERROR", tmp.Name(), s.remote(key))
	cmd.Env = append(os.Environ(), "RCLONE_CONFIG="+s.configPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("rclone copyto failed: %v (%s) out=%s", err, time.Since(start), strings.TrimSpace(string(out)))
		return err
	}
	log.Printf("rclone copyto OK: %s (elapsed %s, %d bytes)", key, time.Since(start), len(data))
	return nil
}

// Link создаёт временную публичную ссылку на объект (presigned GET).
func (s *S3Store) Link(ctx context.Context, key string) (string, error) {
	cmd := exec.CommandContext(ctx, "rclone", "link", "--expire", "48h", s.remote(key))
	cmd.Env = append(os.Environ(), "RCLONE_CONFIG="+s.configPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("rclone link failed: %v out=%s", err, strings.TrimSpace(string(out)))
		return "", err
	}
	link := strings.TrimSpace(string(out))
	if link == "" {
		return "", fmt.Errorf("rclone link: empty url")
	}
	return link, nil
}

// sanitizeKey делает безопасный фрагмент ключа S3.
func sanitizeKey(s string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_", "{", "", "}", "")
	s = replacer.Replace(s)
	for strings.Contains(s, "..") {
		s = strings.ReplaceAll(s, "..", ".")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "file"
	}
	return s
}

func randomID() string {
	b := make([]byte, 12)
	if f, err := os.Open("/dev/urandom"); err == nil {
		_, _ = f.Read(b)
		_ = f.Close()
	}
	return fmt.Sprintf("%x", b)
}
