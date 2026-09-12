package updater

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
)

// maxDownloadSize caps the binary download. The release binary is around
// 8 MB; the cap only guards against a runaway or malicious response.
const maxDownloadSize = 64 << 20

// maxSignatureSize caps the .sig download; a valid file is under 100 bytes.
const maxSignatureSize = 4 << 10

// Progress receives download progress; total is 0 while unknown.
type Progress func(done, total int64)

// download streams url into dst. expectedSize (when > 0) must match the
// number of bytes received; a short or oversized body is an error and the
// partial file is removed.
func download(ctx context.Context, client *http.Client, url, dst string, expectedSize int64, ua string, progress Progress) (err error) {
	total := expectedSize
	if total > maxDownloadSize {
		return fmt.Errorf("release binary of %d bytes exceeds the %d byte limit", total, maxDownloadSize)
	}

	resp, err := get(ctx, client, url, "application/octet-stream", ua)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if total <= 0 && resp.ContentLength > 0 {
		total = resp.ContentLength
	}
	if total > maxDownloadSize {
		return fmt.Errorf("download of %d bytes exceeds the %d byte limit", total, maxDownloadSize)
	}

	f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create download file: %w", err)
	}
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(dst)
		}
	}()

	if progress != nil {
		progress(0, total)
	}
	reader := &progressReader{r: io.LimitReader(resp.Body, maxDownloadSize+1), total: total, progress: progress}
	written, err := io.Copy(f, reader)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	if written > maxDownloadSize {
		return fmt.Errorf("download exceeds the %d byte limit", maxDownloadSize)
	}
	if expectedSize > 0 && written != expectedSize {
		return fmt.Errorf("incomplete download: %d of %d bytes", written, expectedSize)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("flush download: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close download: %w", err)
	}
	return nil
}

type progressReader struct {
	r        io.Reader
	done     int64
	total    int64
	progress Progress
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.done += int64(n)
		if p.progress != nil {
			p.progress(p.done, p.total)
		}
	}
	return n, err
}

// fetchSignature downloads and decodes the detached signature asset.
func fetchSignature(ctx context.Context, client *http.Client, url, ua string) ([]byte, error) {
	resp, err := get(ctx, client, url, "application/octet-stream", ua)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	content, err := io.ReadAll(io.LimitReader(resp.Body, maxSignatureSize))
	if err != nil {
		return nil, fmt.Errorf("read signature: %w", err)
	}
	return DecodeSignature(content)
}
