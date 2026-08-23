package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gwatch/internal/timeutil"
)

const (
	defaultMaxSizeMB = 20
	mb               = 1024 * 1024
)

type DailyRotationWriter struct {
	mu          sync.Mutex
	dir         string
	baseName    string
	ext         string
	maxSize     int64
	currentDate string
	currentSeq  int
	currentSize int64
	file        *os.File
}

func NewDailyRotationWriter(outputPath string, maxSizeMB int) (*DailyRotationWriter, error) {
	if maxSizeMB <= 0 {
		maxSizeMB = defaultMaxSizeMB
	}

	dir := filepath.Dir(outputPath)
	filename := filepath.Base(outputPath)
	ext := filepath.Ext(filename)
	baseName := strings.TrimSuffix(filename, ext)

	w := &DailyRotationWriter{
		dir:      dir,
		baseName: baseName,
		ext:      ext,
		maxSize:  int64(maxSizeMB) * mb,
	}

	ensureDir(outputPath)

	if err := w.openFile(timeutil.Now()); err != nil {
		return nil, err
	}

	return w, nil
}

func (w *DailyRotationWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	today := timeutil.Format(timeutil.Now(), "2006-01-02")

	if today != w.currentDate {
		if err := w.closeFile(); err != nil {
			return 0, err
		}
		w.currentDate = today
		w.currentSeq = 0
		w.currentSize = 0
		if err := w.openFile(timeutil.Now()); err != nil {
			return 0, err
		}
	}

	if w.currentSize+int64(len(p)) > w.maxSize {
		if err := w.closeFile(); err != nil {
			return 0, err
		}
		w.currentSeq++
		w.currentSize = 0
		if err := w.openFile(timeutil.Now()); err != nil {
			return 0, err
		}
	}

	n, err = w.file.Write(p)
	w.currentSize += int64(n)
	return n, err
}

func (w *DailyRotationWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		return w.file.Sync()
	}
	return nil
}

func (w *DailyRotationWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.closeFile()
}

func (w *DailyRotationWriter) openFile(t time.Time) error {
	dateStr := timeutil.Format(t, "2006-01-02")
	w.currentDate = dateStr

	filename := w.buildFilename(dateStr, w.currentSeq)
	path := filepath.Join(w.dir, filename)

	if w.currentSeq == 0 {
		if info, err := os.Stat(path); err == nil && info.Size() >= w.maxSize {
			w.currentSeq++
			filename = w.buildFilename(dateStr, w.currentSeq)
			path = filepath.Join(w.dir, filename)
		}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开日志文件失败: %w", err)
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	w.currentSize = info.Size()
	w.file = f

	return nil
}

func (w *DailyRotationWriter) closeFile() error {
	if w.file != nil {
		err := w.file.Close()
		w.file = nil
		return err
	}
	return nil
}

func (w *DailyRotationWriter) buildFilename(date string, seq int) string {
	if seq == 0 {
		return fmt.Sprintf("%s_%s%s", w.baseName, date, w.ext)
	}
	return fmt.Sprintf("%s_%s_%02d%s", w.baseName, date, seq, w.ext)
}
