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
	// defaultMaxSizeMB 日志文件默认最大体积（MB）
	defaultMaxSizeMB = 20
	// mb 1 MB 对应的字节数
	mb = 1024 * 1024
)

// DailyRotationWriter 按天 + 按体积双重轮转的日志写入器。
// 文件命名格式: <base>_YYYY-MM-DD[_NN].<ext>，每日首次写入自动切换到新日期文件，
// 当当前文件体积超过 maxSize 时会新建带序号的文件继续写入。
type DailyRotationWriter struct {
	mu          sync.Mutex // 保护并发写入
	dir         string     // 日志目录
	baseName    string     // 文件名（不含日期、序号与扩展名）
	ext         string     // 文件扩展名（如 .log）
	maxSize     int64      // 单文件最大体积（字节）
	currentDate string     // 当前日志文件对应的日期（YYYY-MM-DD）
	currentSeq  int        // 当前文件的序号（从 0 开始）
	currentSize int64      // 当前文件已写入字节数
	file        *os.File   // 当前打开的文件句柄
}

// NewDailyRotationWriter 创建一个按天 + 按大小轮转的文件写入器。
// outputPath 为日志输出路径（如 ./logs/gwatch.log），maxSizeMB 为单文件最大体积。
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

// Write 实现 io.Writer；执行按天与按大小的轮转判定后写入数据。
func (w *DailyRotationWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	today := timeutil.Format(timeutil.Now(), "2006-01-02")

	// 日期变更 -> 切换到新日期文件
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

	// 文件体积超限 -> 新建同日期的序号文件
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

// Sync 将文件缓冲刷新到磁盘。
func (w *DailyRotationWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		return w.file.Sync()
	}
	return nil
}

// Close 关闭当前打开的日志文件。
func (w *DailyRotationWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.closeFile()
}

// openFile 打开或创建指定日期的日志文件。
// 当序号为 0 且文件已存在且体积超过 maxSize 时，自动递增序号。
func (w *DailyRotationWriter) openFile(t time.Time) error {
	dateStr := timeutil.Format(t, "2006-01-02")
	w.currentDate = dateStr

	filename := w.buildFilename(dateStr, w.currentSeq)
	path := filepath.Join(w.dir, filename)

	// 序号为 0 时，若文件已存在且体积超限，则直接递增序号，避免写入老文件
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

// closeFile 关闭当前文件句柄（若存在）。
func (w *DailyRotationWriter) closeFile() error {
	if w.file != nil {
		err := w.file.Close()
		w.file = nil
		return err
	}
	return nil
}

// buildFilename 根据日期与序号构造文件名。
// 序号为 0 时文件名为 <base>_<date><ext>，否则为 <base>_<date>_<NN><ext>。
func (w *DailyRotationWriter) buildFilename(date string, seq int) string {
	if seq == 0 {
		return fmt.Sprintf("%s_%s%s", w.baseName, date, w.ext)
	}
	return fmt.Sprintf("%s_%s_%02d%s", w.baseName, date, seq, w.ext)
}
