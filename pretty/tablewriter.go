package pretty

import (
	"fmt"
	"io"
	"text/tabwriter"
)

// TableWriter is a helper for writing tables
type TableWriter struct {
	*tabwriter.Writer
	indent string
}

func NewTableWriter(out io.Writer) *TableWriter {
	return NewTableWriterIndent(out, "")
}

func NewTableWriterIndent(out io.Writer, indent string) *TableWriter {
	return NewTableWriterPad(out, 1, indent)
}

func NewTableWriterPad(out io.Writer, pad int, indent string) *TableWriter {
	tw := tabwriter.NewWriter(out, 0, 0, pad, ' ', 0)
	return NewTableWriterTab(tw, indent)
}

func NewTableWriterTab(tw *tabwriter.Writer, indent string) *TableWriter {
	return &TableWriter{
		tw,
		indent,
	}
}

// Row emits a row where each argument is a column.
func (w *TableWriter) Row(cols ...interface{}) {
	w.emitRow(false, cols...)
}

// URow calls Row and adds an underline row.
func (w *TableWriter) URow(cols ...interface{}) {
	w.emitRow(false, cols...)
	w.emitRow(true, cols...)
}

// Printf invokes Fprintf on the underlying writer, w/ indent and newline.
func (w *TableWriter) Printf(f string, a ...interface{}) {
	fmt.Fprintf(w.Writer, w.indent)
	fmt.Fprintf(w.Writer, f, a...)
	if f == "" || f[len(f)-1] != '\n' {
		fmt.Fprintln(w.Writer)
	}
}

func (w *TableWriter) emitRow(underline bool, cols ...interface{}) {
	fmt.Fprintf(w, w.indent)
	for i, c := range cols {
		if i != 0 {
			fmt.Fprintf(w, "\t")
		}
		if underline {
			w.Write(dashes[:len(fmt.Sprint(c))])
		} else {
			cs := fmt.Sprintf("%s", c)
			fmt.Fprint(w, cs)
		}
	}
	fmt.Fprintln(w)
}
