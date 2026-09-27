package pdbcompat

import "net/http"

// prettyIndent is the indent of one level: upstream renders with
// json.dumps(indent=2) (2.83.0 renderers.py:64-73).
const prettyIndent = "  "

// prettyJSON indents the JSON bodies of next when the request has a
// pretty key, with any value, as upstream does (2.83.0
// renderers.py:64-68). The upstream renderer writes every /api/ JSON
// body, the errors included, so every /api/ handler goes through this
// wrapper.
func prettyJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.URL.Query()["pretty"]; !ok {
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(&prettyWriter{ResponseWriter: w}, r)
	})
}

// prettyWriter re-indents the compact JSON that the /api/ writers
// produce, one byte at a time, so a streamed list stays streamed. The
// output has the form of Python json.dumps(indent=2): a line per
// element, ": " after a key, and "[]" and "{}" for an empty array and
// object. White space outside a string is dropped, which removes the
// newline that json.Encoder adds.
type prettyWriter struct {
	http.ResponseWriter
	depth    int
	inString bool
	escaped  bool
	// open holds a '{' or '[' that is not written yet: the next byte
	// tells whether the container is empty.
	open byte
	out  []byte
}

// Write re-indents p and writes the result. It returns len(p) on
// success, because the caller wrote all of p.
func (pw *prettyWriter) Write(p []byte) (int, error) {
	pw.out = pw.out[:0]
	for _, c := range p {
		pw.put(c)
	}
	if _, err := pw.ResponseWriter.Write(pw.out); err != nil {
		return 0, err
	}
	return len(p), nil
}

// put appends the output of one input byte to pw.out.
func (pw *prettyWriter) put(c byte) {
	if pw.inString {
		pw.out = append(pw.out, c)
		switch {
		case pw.escaped:
			pw.escaped = false
		case c == '\\':
			pw.escaped = true
		case c == '"':
			pw.inString = false
		}
		return
	}
	switch c {
	case ' ', '\t', '\n', '\r':
		return
	}
	if pw.open != 0 {
		open := pw.open
		pw.open = 0
		if c == '}' && open == '{' || c == ']' && open == '[' {
			pw.depth--
			pw.out = append(pw.out, open, c)
			return
		}
		pw.out = append(pw.out, open)
		pw.newline()
	}
	switch c {
	case '{', '[':
		pw.depth++
		pw.open = c
	case '}', ']':
		pw.depth--
		pw.newline()
		pw.out = append(pw.out, c)
	case ',':
		pw.out = append(pw.out, ',')
		pw.newline()
	case ':':
		pw.out = append(pw.out, ':', ' ')
	case '"':
		pw.inString = true
		pw.out = append(pw.out, c)
	default:
		pw.out = append(pw.out, c)
	}
}

// newline appends a line break and the indent of the current depth.
func (pw *prettyWriter) newline() {
	pw.out = append(pw.out, '\n')
	for range pw.depth {
		pw.out = append(pw.out, prettyIndent...)
	}
}

// Flush sends the written bytes to the client when the underlying
// writer can flush. A pending '{' or '[' stays in the writer.
func (pw *prettyWriter) Flush() {
	if f, ok := pw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap returns the underlying writer, for http.ResponseController.
func (pw *prettyWriter) Unwrap() http.ResponseWriter {
	return pw.ResponseWriter
}
