package cli

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type parseProgressReporter struct {
	enabled bool
	label   string
	total   int
	start   time.Time
	spinner int
	lastLen int
}

func newParseProgressReporter(label string, total int, asJSON bool) *parseProgressReporter {
	stat, err := os.Stderr.Stat()
	enabled := err == nil && (stat.Mode()&os.ModeCharDevice) != 0 && !asJSON
	return &parseProgressReporter{
		enabled: enabled,
		label:   label,
		total:   total,
		start:   time.Now(),
	}
}

func (r *parseProgressReporter) Update(file string, count int) {
	if !r.enabled {
		return
	}
	frames := [4]string{"-", "\\", "|", "/"}
	frame := frames[r.spinner%len(frames)]
	r.spinner++
	file = strings.TrimSpace(file)
	if len(file) > 88 {
		file = "..." + file[len(file)-85:]
	}

	status := fmt.Sprintf("%s %s %d parsing %s", frame, r.label, count, file)
	if r.total > 0 {
		status = fmt.Sprintf("%s %s %d/%d parsing %s", frame, r.label, count, r.total, file)
	}
	r.printStatus(status)
}

func (r *parseProgressReporter) Done(count int) {
	if !r.enabled {
		return
	}
	elapsed := time.Since(r.start).Round(time.Millisecond)
	status := fmt.Sprintf("%s complete (%d files in %s)", r.label, count, elapsed)
	r.printStatus(status)
	fmt.Fprintln(os.Stderr)
}

func (r *parseProgressReporter) printStatus(status string) {
	if r.lastLen > len(status) {
		status = status + strings.Repeat(" ", r.lastLen-len(status))
	}
	r.lastLen = len(status)
	fmt.Fprintf(os.Stderr, "\r%s", status)
}
