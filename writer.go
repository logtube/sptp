package sptp

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"errors"
	"io"
)

var (
	ErrPayloadTooLarge = errors.New("payload too large, more than 255 chunks are needed")
)

// WriterOptions writer options
type WriterOptions struct {
	// GzipLevel gzip level, 0 for no, 9 for best
	GzipLevel int

	// ChunkThreshold chunk size threshold, any payload larger than this will be sent with chunked message
	ChunkThreshold int
}

type chunkedWriter struct {
	w io.Writer
	t int
}

func (w *chunkedWriter) Write(mode byte, p []byte) (err error) {
	l := len(p)
	t := w.t
	if l > t {
		// check chunks count
		if ChunkedMaxCount*t < l {
			err = ErrPayloadTooLarge
			return
		}
		// chunks count
		c := l / t
		if l%t != 0 {
			c++
		}
		// set mode chunked
		mode |= ModeChunked
		// header
		h := [OverheadMaxSize]byte{Magic, mode}
		h[OverheadMaxSize-2] = byte(c)
		// message id
		if _, err = rand.Read(h[2 : OverheadMaxSize-2]); err != nil {
			return
		}
		// iterate and write
		for i := 0; i < c; i++ {
			// update header
			h[len(h)-1] = byte(i)
			// next boundary
			n := (i + 1) * t
			if n > l {
				n = l
			}
			// build buffer
			b := append(h[:], p[(i*t):n]...)
			// write
			if _, err = w.w.Write(b); err != nil {
				return
			}
		}
	} else {
		// non-chunked
		b := append([]byte{Magic, mode}, p...)
		if _, err = w.w.Write(b); err != nil {
			return
		}
	}
	return
}

type writer struct {
	cw *chunkedWriter
	l  int
}

func compress(p []byte, l int) (o []byte, err error) {
	// output buffer
	buf := &bytes.Buffer{}
	// compressed writer
	var gw *gzip.Writer
	if gw, err = gzip.NewWriterLevel(buf, l); err != nil {
		return
	}
	// compress
	if _, err = gw.Write(p); err != nil {
		return
	}
	if err = gw.Close(); err != nil {
		return
	}
	o = buf.Bytes()
	return
}

func (w *writer) Write(p []byte) (n int, err error) {
	var mode byte
	b := p
	if w.l != gzip.NoCompression {
		// set compression mode
		mode |= ModeGzipped
		// compress
		if b, err = compress(b, w.l); err != nil {
			return
		}
	}
	// send payload via chunked writer
	if err = w.cw.Write(mode, b); err != nil {
		return
	}
	// return len
	n = len(p)
	return
}

func NewWriter(w io.Writer) io.Writer {
	return NewWriterWithOptions(w, WriterOptions{})
}

func NewWriterWithOptions(w io.Writer, opts WriterOptions) io.Writer {
	if opts.ChunkThreshold <= 0 {
		opts.ChunkThreshold = ChunkPayloadSizeDefault
	}
	if opts.GzipLevel < gzip.HuffmanOnly || opts.GzipLevel > gzip.BestCompression {
		opts.GzipLevel = gzip.NoCompression
	}
	o := &writer{
		cw: &chunkedWriter{w: w, t: opts.ChunkThreshold},
		l:  opts.GzipLevel,
	}
	return o
}
